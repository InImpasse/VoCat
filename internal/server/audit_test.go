package server

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"

	"vocat/internal/auth"
	"vocat/internal/store"
)

func newAuditTestServer(t *testing.T) (*Server, *store.Store, string) {
	t.Helper()
	database, err := store.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	authService, err := auth.New(database, auth.Options{
		SessionTTL: time.Hour,
		BcryptCost: bcrypt.MinCost,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := authService.EnsureAdmin(context.Background(), "operator", "operator-secure-password"); err != nil {
		t.Fatal(err)
	}
	credentials, err := authService.Login(context.Background(), "operator", "operator-secure-password")
	if err != nil {
		t.Fatal(err)
	}
	access, err := parseAccessConfig(accessConfig{
		Mode:              "internal",
		TrustProxyHeaders: true,
		TrustedProxyCIDRs: []string{"203.0.113.10/32"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return &Server{
		store:  database,
		auth:   authService,
		logger: regionTestLogger(),
		access: access,
	}, database, credentials.SessionToken
}

func TestHTTPRequestAuditUsesSessionActorAndTrustedClientIP(t *testing.T) {
	server, database, sessionToken := newAuditTestServer(t)
	request := httptest.NewRequest(http.MethodPost, "/api/settings/developer", nil)
	request.RemoteAddr = "203.0.113.10:5000"
	request.Header.Set("X-Forwarded-For", "198.51.100.25")
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionToken})

	server.audit(request, "settings.developer.device_limit", "settings", "developer", "success")
	events, err := database.ListAuditEvents(context.Background(), store.AuditFilter{
		Action: "settings.developer.device_limit",
		Limit:  1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("audit events = %d, want 1", len(events))
	}
	if events[0].Actor != "operator" || events[0].RemoteAddr != "198.51.100.25" {
		t.Fatalf("audit actor/remote = %q/%q", events[0].Actor, events[0].RemoteAddr)
	}
}

func TestHTTPRequestAuditIgnoresProxyHeaderFromUntrustedPeer(t *testing.T) {
	server, database, sessionToken := newAuditTestServer(t)
	request := httptest.NewRequest(http.MethodPost, "/api/extensions/upload", nil)
	request.RemoteAddr = "192.0.2.44:5000"
	request.Header.Set("X-Forwarded-For", "127.0.0.1")
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionToken})

	server.audit(request, "plugin.upload", "plugin", "example", "success")
	events, err := database.ListAuditEvents(context.Background(), store.AuditFilter{
		Action: "plugin.upload",
		Limit:  1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].RemoteAddr != "192.0.2.44" {
		t.Fatalf("audit events = %#v", events)
	}
}

func TestAuthenticationAuditUsesAttemptedActorAndTrustedClientIP(t *testing.T) {
	server, database, _ := newAuditTestServer(t)
	request := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	request.RemoteAddr = "203.0.113.10:5000"
	request.Header.Set("X-Forwarded-For", "198.51.100.30")

	server.auditAuth(request, "candidate", "failure")
	events, err := database.ListAuditEvents(context.Background(), store.AuditFilter{
		Action: "auth.login",
		Limit:  1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Actor != "candidate" || events[0].RemoteAddr != "198.51.100.30" {
		t.Fatalf("audit events = %#v", events)
	}
}

func TestNonHTTPAuditRetainsTransport(t *testing.T) {
	server, database, _ := newAuditTestServer(t)
	server.recordAudit(
		context.Background(),
		"telegram:42",
		"telegram.at.execute",
		"device",
		"device-1",
		"success",
		"telegram",
	)
	events, err := database.ListAuditEvents(context.Background(), store.AuditFilter{
		Action: "telegram.at.execute",
		Limit:  1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Actor != "telegram:42" || events[0].RemoteAddr != "telegram" {
		t.Fatalf("audit events = %#v", events)
	}
}

func TestAuthenticationAuditBoundsUntrustedUsernameBeforeLogAndStore(t *testing.T) {
	server, database, _ := newAuditTestServer(t)
	var logOutput bytes.Buffer
	server.logger = slog.New(slog.NewJSONHandler(&logOutput, nil))
	request := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	username := strings.Repeat("界", 5000)

	server.auditAuth(request, username, "failure")
	events, err := database.ListAuditEvents(context.Background(), store.AuditFilter{
		Action: "auth.login",
		Limit:  1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("audit events = %d, want 1", len(events))
	}
	if len(events[0].Actor) > store.MaxAuditActorBytes || len(events[0].EntityID) > store.MaxAuditEntityIDBytes {
		t.Fatalf("oversized audit fields persisted: actor=%d entity_id=%d", len(events[0].Actor), len(events[0].EntityID))
	}
	if !utf8.ValidString(events[0].Actor) || !utf8.ValidString(events[0].EntityID) {
		t.Fatal("audit field truncation produced invalid UTF-8")
	}
	if strings.Contains(logOutput.String(), username) {
		t.Fatal("raw oversized username was written to the audit log")
	}
}

func TestLockedLoginFloodUsesBoundedAuditSampling(t *testing.T) {
	server, database, _ := newAuditTestServer(t)
	server.loginLimiter = newLoginRateLimiter()
	server.maxRequestBodyBytes = 4096
	now := time.Now().UTC()
	server.loginLimiter.now = func() time.Time { return now }

	if _, err := database.AppendAuditEvent(context.Background(), store.AuditEvent{
		Action:    "historical.security.event",
		Outcome:   "success",
		CreatedAt: now.Add(-time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	login := func(username string) int {
		t.Helper()
		request := httptest.NewRequest(
			http.MethodPost,
			"/api/auth/login",
			strings.NewReader(`{"username":"`+username+`","password":"wrong-password"}`),
		)
		request.RemoteAddr = "198.51.100.40:5000"
		response := httptest.NewRecorder()
		server.handleLogin(response, request)
		return response.Code
	}

	// Unique usernames exercise the IP bucket rather than the pair or account
	// bucket. The threshold request itself must remain visible as a lock event.
	for i := 0; i < loginIPMaxFailures; i++ {
		status := login(fmt.Sprintf("candidate-%d", i))
		want := http.StatusUnauthorized
		if i == loginIPMaxFailures-1 {
			want = http.StatusTooManyRequests
		}
		if status != want {
			t.Fatalf("threshold request %d status = %d, want %d", i+1, status, want)
		}
	}

	const rejectedRequests = 1000
	for i := 0; i < rejectedRequests; i++ {
		if status := login(fmt.Sprintf("flood-%d", i)); status != http.StatusTooManyRequests {
			t.Fatalf("locked request %d status = %d, want 429", i+1, status)
		}
	}

	events, err := database.ListAuditEvents(context.Background(), store.AuditFilter{
		Action: "auth.login",
		Limit:  loginIPMaxFailures + 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != loginIPMaxFailures {
		t.Fatalf("audit writes after %d locked requests = %d, want %d", rejectedRequests, len(events), loginIPMaxFailures)
	}
	locked := 0
	for _, event := range events {
		if event.Outcome == "locked" {
			locked++
		}
	}
	if locked != 1 {
		t.Fatalf("initial lock audit count = %d, want 1", locked)
	}

	// A sustained attack remains visible without making writes request-linear.
	now = now.Add(loginLockoutAuditPeriod)
	if status := login("periodic-sample"); status != http.StatusTooManyRequests {
		t.Fatalf("periodic sample status = %d, want 429", status)
	}
	events, err = database.ListAuditEvents(context.Background(), store.AuditFilter{
		Action: "auth.login",
		Limit:  loginIPMaxFailures + 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != loginIPMaxFailures+1 || events[0].Outcome != "locked" {
		t.Fatalf("periodic audit events = %#v", events)
	}

	historical, err := database.ListAuditEvents(context.Background(), store.AuditFilter{
		Action: "historical.security.event",
		Limit:  1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(historical) != 1 {
		t.Fatal("locked-request flood removed unrelated audit history")
	}
}
