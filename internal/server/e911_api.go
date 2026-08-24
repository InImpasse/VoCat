package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"vocat/internal/auth"
	"vocat/internal/store"
)

const (
	maxWebsheetSessions  = 1024
	websheetSessionTTL   = 15 * time.Minute
	websheetDoneGrace    = 2 * time.Minute
	websheetTombstoneTTL = 2 * time.Minute
	maxE911FieldBytes    = 256
)

var errWebsheetCapacity = errors.New("websheet session capacity is exhausted")

type websheetCallbackResult uint8

const (
	websheetCallbackUnavailable websheetCallbackResult = iota
	websheetCallbackClaimed
	websheetCallbackReplay
)

// websheetSession is a short-lived E911 address provisioning session. Access
// is authorized by the normal application session; the random ID only locates
// the in-memory websheet state.
type websheetSession struct {
	id                string
	deviceID          string
	createdAt         time.Time
	expiresAt         time.Time
	callbackInFlight  bool
	callbackComplete  bool
	callbackDigest    [sha256.Size]byte
	callbackHasDigest bool
	doneAt            time.Time
}

type websheetManager struct {
	mu       sync.Mutex
	sessions map[string]*websheetSession
}

func newWebsheetManager() *websheetManager {
	return &websheetManager{sessions: make(map[string]*websheetSession)}
}

func websheetID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate websheet ID: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

func (m *websheetManager) create(deviceID string) (*websheetSession, error) {
	id, err := websheetID()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	session := &websheetSession{
		id:        id,
		deviceID:  deviceID,
		createdAt: now,
		expiresAt: now.Add(websheetSessionTTL),
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneExpiredLocked(now)
	if len(m.sessions) >= maxWebsheetSessions {
		victimID := m.oldestEvictableLocked(true)
		if victimID == "" {
			victimID = m.oldestEvictableLocked(false)
		}
		if victimID == "" {
			return nil, errWebsheetCapacity
		}
		delete(m.sessions, victimID)
	}
	m.sessions[session.id] = session
	return cloneWebsheetSession(session), nil
}

func (m *websheetManager) get(id string) *websheetSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	session := m.activeLocked(id, time.Now().UTC())
	if session == nil {
		return nil
	}
	return cloneWebsheetSession(session)
}

func (m *websheetManager) beginCallback(id string, digest [sha256.Size]byte) (*websheetSession, websheetCallbackResult) {
	m.mu.Lock()
	defer m.mu.Unlock()
	session := m.activeLocked(id, time.Now().UTC())
	if session == nil || session.callbackInFlight {
		return nil, websheetCallbackUnavailable
	}
	if session.callbackComplete {
		if session.callbackHasDigest && session.callbackDigest == digest {
			return cloneWebsheetSession(session), websheetCallbackReplay
		}
		return nil, websheetCallbackUnavailable
	}
	session.callbackInFlight = true
	session.callbackDigest = digest
	session.callbackHasDigest = true
	return cloneWebsheetSession(session), websheetCallbackClaimed
}

func (m *websheetManager) finishCallback(id string, digest [sha256.Size]byte, succeeded bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC()
	if session := m.sessions[id]; session != nil && session.callbackInFlight &&
		session.callbackHasDigest && session.callbackDigest == digest {
		session.callbackInFlight = false
		if succeeded {
			session.callbackComplete = true
			session.expiresAt = now.Add(websheetDoneGrace)
			return
		}
		session.callbackDigest = [sha256.Size]byte{}
		session.callbackHasDigest = false
		if !session.expiresAt.After(now) {
			delete(m.sessions, id)
		}
	}
}

func (m *websheetManager) complete(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	session := m.activeLocked(id, time.Now().UTC())
	if session == nil || session.callbackInFlight || !session.callbackComplete {
		return false
	}
	if session.doneAt.IsZero() {
		session.doneAt = time.Now().UTC()
		session.expiresAt = session.doneAt.Add(websheetTombstoneTTL)
	}
	return true
}

func (m *websheetManager) activeLocked(id string, now time.Time) *websheetSession {
	session := m.sessions[id]
	if session == nil {
		return nil
	}
	if !session.expiresAt.After(now) && !session.callbackInFlight {
		delete(m.sessions, id)
		return nil
	}
	return session
}

func (m *websheetManager) pruneExpiredLocked(now time.Time) {
	for id, session := range m.sessions {
		if !session.callbackInFlight && !session.expiresAt.After(now) {
			delete(m.sessions, id)
		}
	}
}

func (m *websheetManager) oldestEvictableLocked(tombstoneOnly bool) string {
	oldestID := ""
	var oldest time.Time
	for id, session := range m.sessions {
		isTombstone := !session.doneAt.IsZero()
		if tombstoneOnly != isTombstone {
			continue
		}
		if session.callbackInFlight || (!isTombstone && session.callbackComplete) {
			continue
		}
		if oldestID == "" || session.createdAt.Before(oldest) {
			oldestID = id
			oldest = session.createdAt
		}
	}
	return oldestID
}

func cloneWebsheetSession(session *websheetSession) *websheetSession {
	if session == nil {
		return nil
	}
	copy := *session
	return &copy
}

// handleE911Websheet creates a self-hosted E911 websheet session for a device
// and returns the embeddable form URL (VoHive: POST /devices/{id}/vowifi/e911/websheet).
func (s *Server) handleE911Websheet(
	w http.ResponseWriter,
	r *http.Request,
	config store.Device,
) bool {
	if !requireMethod(w, r, http.MethodPost) {
		return true
	}
	session, err := s.websheets.create(config.ID)
	if err != nil {
		if errors.Is(err, errWebsheetCapacity) {
			writeError(w, http.StatusServiceUnavailable, "websheet_capacity_exhausted", "too many active websheet sessions")
			return true
		}
		s.logger.Error("create E911 websheet session", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "an internal error occurred")
		return true
	}
	embedURL := fmt.Sprintf("/websheets/%s", session.id)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"id":         session.id,
			"embed_url":  embedURL,
			"expires_at": session.expiresAt,
		},
	})
	return true
}

// handleWebsheet serves and drives the self-hosted E911 address form. These
// paths live outside the /api tree because the GET response is HTML, so this
// handler applies the same session and CSRF checks explicitly.
//
//	GET  /websheets/{id}                  -> the address form
//	POST /websheets/{id}/callback         -> relay the entered address
//	POST /websheets/{id}/done             -> complete the session
func (s *Server) handleWebsheet(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuthenticated(w, r) {
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
		if !s.validateWebsheetCSRF(w, r) {
			return
		}
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/websheets/"), "/")
	segments := splitAPIPath(rest)
	if len(segments) == 0 || segments[0] == "" {
		writeError(w, http.StatusNotFound, "not_found", "websheet was not found")
		return
	}
	session := s.websheets.get(segments[0])
	if session == nil {
		writeError(w, http.StatusNotFound, "websheet_not_found", "websheet session was not found or has expired")
		return
	}
	action := ""
	if len(segments) > 1 {
		action = segments[1]
	}
	switch {
	case action == "" && r.Method == http.MethodGet:
		s.serveWebsheetForm(w, r, session)
	case action == "callback" && r.Method == http.MethodPost:
		s.handleWebsheetCallback(w, r, session)
	case action == "done" && r.Method == http.MethodPost:
		s.handleWebsheetDone(w, r, session)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	}
}

func (s *Server) validateWebsheetCSRF(w http.ResponseWriter, r *http.Request) bool {
	sessionToken, ok := s.sessionToken(w, r)
	if !ok {
		return false
	}
	csrfToken, ok := s.validateDoubleSubmitCSRF(w, r)
	if !ok {
		return false
	}
	if _, err := s.auth.ValidateCSRF(r.Context(), sessionToken, csrfToken); err != nil {
		switch {
		case errors.Is(err, auth.ErrUnauthorized):
			s.authenticationRequired(w, r)
		case errors.Is(err, auth.ErrInvalidCSRF):
			writeError(w, http.StatusForbidden, "invalid_csrf", "CSRF validation failed")
		default:
			s.logger.Error("websheet CSRF validation failed", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "an internal error occurred")
		}
		return false
	}
	return true
}

func (s *Server) serveWebsheetForm(w http.ResponseWriter, r *http.Request, session *websheetSession) {
	csrfCookie, err := r.Cookie(csrfCookieName)
	if err != nil || csrfCookie.Value == "" {
		writeError(w, http.StatusForbidden, "invalid_csrf", "CSRF validation failed")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, websheetFormHTML(session, csrfCookie.Value))
}

func (s *Server) handleWebsheetCallback(w http.ResponseWriter, r *http.Request, session *websheetSession) {
	var address e911Address
	if err := s.decodeJSON(w, r, &address); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	clean, err := normalizeE911Address(address)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_e911_address", err.Error())
		return
	}
	payload, err := json.Marshal(clean)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "an internal error occurred")
		return
	}
	digest := sha256.Sum256(payload)
	claimed, result := s.websheets.beginCallback(session.id, digest)
	switch result {
	case websheetCallbackReplay:
		writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"received": true}})
		return
	case websheetCallbackClaimed:
		// Continue with the only code path that is allowed to persist the address.
	default:
		writeError(w, http.StatusConflict, "websheet_already_submitted", "websheet callback is already in progress or was submitted with different data")
		return
	}
	succeeded := false
	defer func() { s.websheets.finishCallback(session.id, digest, succeeded) }()
	if err := s.persistE911Address(r, claimed.deviceID, payload); err != nil {
		s.logger.Warn("persist e911 address failed", "device_id", claimed.deviceID, "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "an internal error occurred")
		return
	}
	succeeded = true
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"received": true}})
}

func (s *Server) handleWebsheetDone(w http.ResponseWriter, r *http.Request, session *websheetSession) {
	if !s.websheets.complete(session.id) {
		writeError(w, http.StatusConflict, "websheet_not_submitted", "websheet callback has not completed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"done": true}})
}

// persistE911Address stores the provisioned E911 address against the device so
// the VoWiFi IMS registration can reference it later.
func (s *Server) persistE911Address(r *http.Request, deviceID string, payload json.RawMessage) error {
	if err := s.store.UpsertAppSetting(r.Context(), store.AppSetting{
		Key:       "e911_address:" + deviceID,
		Value:     payload,
		Sensitive: true,
	}); err != nil {
		return err
	}
	return nil
}

type e911Address struct {
	Name    string `json:"name"`
	Street  string `json:"street"`
	City    string `json:"city"`
	State   string `json:"state"`
	ZIP     string `json:"zip"`
	Country string `json:"country"`
}

func (address *e911Address) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || fields == nil {
		return errors.New("E911 address must be a JSON object")
	}
	for key, raw := range fields {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return fmt.Errorf("E911 address field %q must be a string", key)
		}
		switch key {
		case "name":
			address.Name = value
		case "street":
			address.Street = value
		case "city":
			address.City = value
		case "state":
			address.State = value
		case "zip":
			address.ZIP = value
		case "country":
			address.Country = value
		default:
			return fmt.Errorf("unknown E911 address field %q", key)
		}
	}
	return nil
}

func normalizeE911Address(address e911Address) (e911Address, error) {
	fields := []struct {
		name  string
		value *string
	}{
		{"name", &address.Name},
		{"street", &address.Street},
		{"city", &address.City},
		{"state", &address.State},
		{"zip", &address.ZIP},
		{"country", &address.Country},
	}
	for _, field := range fields {
		if len(*field.value) > maxE911FieldBytes {
			return e911Address{}, fmt.Errorf("E911 address field %q exceeds %d bytes", field.name, maxE911FieldBytes)
		}
		*field.value = strings.TrimSpace(*field.value)
	}
	for _, required := range []struct {
		name  string
		value string
	}{
		{"street", address.Street},
		{"city", address.City},
		{"country", address.Country},
	} {
		if required.value == "" {
			return e911Address{}, fmt.Errorf("E911 address field %q is required", required.name)
		}
	}
	return address, nil
}

// websheetFormHTML renders the self-contained E911 address form embedded by the
// frontend. On submit it relays the address to the callback, notifies the
// parent frame (vohive-websheet-callback), and completes the session.
func websheetFormHTML(session *websheetSession, csrfToken string) string {
	callbackURL := fmt.Sprintf("/websheets/%s/callback", session.id)
	doneURL := fmt.Sprintf("/websheets/%s/done", session.id)
	callbackJSON, _ := json.Marshal(callbackURL)
	doneJSON, _ := json.Marshal(doneURL)
	csrfJSON, _ := json.Marshal(csrfToken)
	return `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>E911 Address</title>
<style>
body{font-family:-apple-system,Segoe UI,Roboto,sans-serif;background:#f6f7f9;margin:0;padding:20px;color:#111}
.card{max-width:430px;margin:0 auto;background:#fff;border:1px solid #e5e7eb;border-radius:12px;padding:20px}
h1{font-size:16px;margin:0 0 4px}p.sub{font-size:12px;color:#6b7280;margin:0 0 16px}
label{display:block;font-size:12px;color:#4b5563;margin:10px 0 4px}
input{width:100%;height:34px;padding:0 10px;border:1px solid #dcdfe6;border-radius:8px;font-size:13px;box-sizing:border-box}
input:focus{outline:none;border-color:#5b5bd6;box-shadow:0 0 0 3px rgba(91,91,214,.18)}
button{margin-top:16px;width:100%;height:38px;border:0;border-radius:8px;background:#5b5bd6;color:#fff;font-size:14px;font-weight:600;cursor:pointer}
button:disabled{opacity:.6;cursor:not-allowed}.row{display:flex;gap:10px}.row>div{flex:1}
.msg{margin-top:12px;font-size:13px;text-align:center}.ok{color:#16a34a}.err{color:#dc2626}
</style></head><body>
<div class="card">
<h1>E911 紧急地址登记</h1>
<p class="sub">为 VoWiFi 服务登记紧急呼叫地址（Emergency Address）。</p>
<form id="f">
<label>姓名 / Name</label><input name="name" autocomplete="name">
<label>街道地址 / Street</label><input name="street" required autocomplete="street-address">
<label>城市 / City</label><input name="city" required>
<div class="row"><div><label>州/省 / State</label><input name="state"></div>
<div><label>邮编 / ZIP</label><input name="zip"></div></div>
<label>国家 / Country</label><input name="country" required>
<button type="submit" id="btn">提交地址</button>
<div class="msg" id="msg"></div>
</form></div>
<script>
var CALLBACK=` + string(callbackJSON) + `,DONE=` + string(doneJSON) + `,CSRF=` + string(csrfJSON) + `;
document.getElementById('f').addEventListener('submit',async function(e){
e.preventDefault();var btn=document.getElementById('btn'),msg=document.getElementById('msg');
btn.disabled=true;msg.textContent='';var data={};
new FormData(e.target).forEach(function(v,k){data[k]=v});
try{
var headers={'Content-Type':'application/json','X-CSRF-Token':CSRF};
var r=await fetch(CALLBACK,{method:'POST',headers:headers,body:JSON.stringify(data)});
if(!r.ok)throw new Error('callback '+r.status);
var completed=await fetch(DONE,{method:'POST',headers:{'X-CSRF-Token':CSRF}});
if(!completed.ok)throw new Error('done '+completed.status);
try{if(window.parent)window.parent.postMessage({type:'vohive-websheet-callback'},window.location.origin)}catch(_){}
msg.className='msg ok';msg.textContent='地址已提交 / Address saved';
}catch(err){btn.disabled=false;msg.className='msg err';msg.textContent='提交失败，请重试 / Submit failed';}
});
</script></body></html>`
}
