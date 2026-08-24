package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"vocat/internal/store"
)

func TestParseAccessConfigValidation(t *testing.T) {
	if _, err := parseAccessConfig(accessConfig{Mode: "bogus"}); err == nil {
		t.Fatal("accepted an invalid mode")
	}
	if _, err := parseAccessConfig(accessConfig{Mode: "internal", AllowedCIDRs: []string{"not-a-cidr"}}); err == nil {
		t.Fatal("accepted an invalid CIDR")
	}
	if _, err := parseAccessConfig(accessConfig{Mode: "internal", TrustedProxyCIDRs: []string{"not-a-proxy-cidr"}}); err == nil {
		t.Fatal("accepted an invalid trusted proxy CIDR")
	}
	parsed, err := parseAccessConfig(accessConfig{Mode: "internal", AllowedCIDRs: []string{"203.0.113.0/24", "198.51.100.7"}})
	if err != nil {
		t.Fatalf("parseAccessConfig: %v", err)
	}
	if len(parsed.cidrs) != 2 {
		t.Fatalf("cidrs = %v", parsed.cidrs)
	}
}

func TestAccessControlMiddleware(t *testing.T) {
	server := &Server{logger: regionTestLogger()}
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	handler := server.accessControl(ok)

	check := func(config parsedAccessConfig, remoteAddr string, forwardedFor string) int {
		server.accessMu.Lock()
		server.access = config
		server.accessMu.Unlock()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = remoteAddr
		if forwardedFor != "" {
			req.Header.Set("X-Forwarded-For", forwardedFor)
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)
		return recorder.Code
	}

	internal := parsedAccessConfig{mode: "internal"}
	if got := check(internal, "192.168.2.10:5000", ""); got != http.StatusOK {
		t.Fatalf("private IP denied: %d", got)
	}
	if got := check(internal, "127.0.0.1:5000", ""); got != http.StatusOK {
		t.Fatalf("loopback denied: %d", got)
	}
	if got := check(internal, "100.100.100.100:5000", ""); got != http.StatusOK {
		t.Fatalf("Tailscale IPv4 denied: %d", got)
	}
	if got := check(internal, "8.8.8.8:5000", ""); got != http.StatusForbidden {
		t.Fatalf("public IP allowed in internal mode: %d", got)
	}
	// Custom CIDR admits an otherwise-public range.
	withCIDR := parsedAccessConfig{mode: "internal"}
	parsed, _ := parseAccessConfig(accessConfig{Mode: "internal", AllowedCIDRs: []string{"8.8.8.0/24"}})
	withCIDR = parsed
	if got := check(withCIDR, "8.8.8.8:5000", ""); got != http.StatusOK {
		t.Fatalf("custom CIDR not honored: %d", got)
	}
	// Public mode allows anything.
	public := parsedAccessConfig{mode: "public"}
	if got := check(public, "8.8.8.8:5000", ""); got != http.StatusOK {
		t.Fatalf("public mode denied a public IP: %d", got)
	}
	// The legacy boolean alone is intentionally fail-closed after upgrade.
	legacyTrust := parsedAccessConfig{mode: "internal", trustProxy: true}
	if got := check(legacyTrust, "8.8.8.8:5000", "192.168.1.20"); got != http.StatusForbidden {
		t.Fatalf("proxy header without a trusted proxy CIDR was honored: %d", got)
	}
	if got := check(internal, "8.8.8.8:5000", "192.168.1.20"); got != http.StatusForbidden {
		t.Fatalf("untrusted X-Forwarded-For was honored: %d", got)
	}
	trustedProxy, err := parseAccessConfig(accessConfig{
		Mode:              "internal",
		TrustProxyHeaders: true,
		TrustedProxyCIDRs: []string{"203.0.113.0/24"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := check(trustedProxy, "203.0.113.10:5000", "192.168.1.20"); got != http.StatusOK {
		t.Fatalf("header from trusted proxy not honored: %d", got)
	}
	if got := check(trustedProxy, "8.8.8.8:5000", "192.168.1.20"); got != http.StatusForbidden {
		t.Fatalf("header from untrusted direct peer was honored: %d", got)
	}
	if got := check(trustedProxy, "203.0.113.10:5000", "127.0.0.1, 8.8.8.8"); got != http.StatusForbidden {
		t.Fatalf("left-side spoof crossed an untrusted forwarding hop: %d", got)
	}
	if got := check(trustedProxy, "203.0.113.10", "192.168.1.20"); got != http.StatusForbidden {
		t.Fatalf("non-TCP RemoteAddr was accepted: %d", got)
	}
	privateProxy, err := parseAccessConfig(accessConfig{
		Mode:              "internal",
		TrustProxyHeaders: true,
		TrustedProxyCIDRs: []string{"192.168.2.0/24"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := check(privateProxy, "192.168.2.10:5000", "invalid"); got != http.StatusForbidden {
		t.Fatalf("malformed header from an internal trusted proxy failed open: %d", got)
	}
}

func TestClientIPPeelsTrustedProxyChainFromTheRight(t *testing.T) {
	config, err := parseAccessConfig(accessConfig{
		Mode:              "internal",
		TrustProxyHeaders: true,
		TrustedProxyCIDRs: []string{"203.0.113.0/24", "198.51.100.0/24"},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "203.0.113.10:5000"
	request.Header.Set("X-Forwarded-For", "127.0.0.1, 8.8.8.8, 198.51.100.20")
	if got := config.clientIP(request).String(); got != "8.8.8.8" {
		t.Fatalf("client IP = %s, want first untrusted address from the right", got)
	}

	request.Header.Set("X-Forwarded-For", "192.168.1.20, invalid")
	if got := config.clientIP(request); got.IsValid() {
		t.Fatalf("invalid forwarding chain produced client IP %s", got)
	}

	request.Header.Del("X-Forwarded-For")
	request.Header.Set("X-Real-IP", "192.168.1.20")
	if got := config.clientIP(request).String(); got != "192.168.1.20" {
		t.Fatalf("trusted X-Real-IP fallback = %s", got)
	}

	request.Header.Set("X-Real-IP", "invalid")
	if got := config.clientIP(request); got.IsValid() {
		t.Fatalf("invalid X-Real-IP produced client IP %s", got)
	}
}

func TestLoginRateLimiterLocksAndResets(t *testing.T) {
	limiter := newLoginRateLimiter()
	now := time.Now()
	limiter.now = func() time.Time { return now }
	ip := "192.168.1.1"
	username := "admin"

	for i := 0; i < loginPairMaxFailures-1; i++ {
		if _, locked := limiter.recordFailure(ip, username); locked {
			t.Fatalf("locked after %d failures, below threshold", i+1)
		}
	}
	if _, locked := limiter.recordFailure(ip, username); !locked {
		t.Fatal("not locked at the failure threshold")
	}
	if _, admitted := limiter.begin(ip, username); admitted {
		limiter.end(ip, username)
		t.Fatal("begin admitted a locked login")
	}
	// Success clears the track record.
	limiter.recordSuccess(ip, username)
	if _, admitted := limiter.begin(ip, username); !admitted {
		t.Fatal("still locked after a success")
	} else {
		limiter.end(ip, username)
	}
	// Lockout expires after the lockout duration.
	for i := 0; i < loginPairMaxFailures; i++ {
		limiter.recordFailure(ip, username)
	}
	now = now.Add(limiter.lockout + time.Second)
	if _, admitted := limiter.begin(ip, username); !admitted {
		t.Fatal("lock did not expire after the lockout window")
	} else {
		limiter.end(ip, username)
	}
}

func TestLoginRateLimiterSeparatesAccountAndIPThresholds(t *testing.T) {
	accountLimiter := newLoginRateLimiter()
	for i := 0; i < loginAccountMaxFailures; i++ {
		accountLimiter.recordFailure(fmt.Sprintf("192.0.2.%d", i+1), "admin")
	}
	if _, admitted := accountLimiter.begin("198.51.100.1", "admin"); admitted {
		accountLimiter.end("198.51.100.1", "admin")
		t.Fatal("account limit did not apply across source IPs")
	}

	ipLimiter := newLoginRateLimiter()
	for i := 0; i < loginIPMaxFailures; i++ {
		ipLimiter.recordFailure("198.51.100.2", fmt.Sprintf("candidate-%d", i))
	}
	if _, admitted := ipLimiter.begin("198.51.100.2", "another-candidate"); admitted {
		ipLimiter.end("198.51.100.2", "another-candidate")
		t.Fatal("IP limit did not apply across usernames")
	}
}

func TestLoginRateLimiterSuccessPreservesBroadIPFailures(t *testing.T) {
	limiter := newLoginRateLimiter()
	ip := "198.51.100.20"

	for i := 0; i < loginIPMaxFailures-1; i++ {
		username := fmt.Sprintf("candidate-%d", i)
		if _, admitted := limiter.begin(ip, username); !admitted {
			t.Fatalf("failure %d was rejected below the IP threshold", i+1)
		}
		if _, locked := limiter.recordFailure(ip, username); locked {
			t.Fatalf("failure %d locked below the IP threshold", i+1)
		}
		limiter.end(ip, username)
	}

	if _, admitted := limiter.begin(ip, "admin"); !admitted {
		t.Fatal("valid login was rejected below the IP threshold")
	}
	limiter.recordSuccess(ip, "admin")
	limiter.end(ip, "admin")

	username := "candidate-after-success"
	if _, admitted := limiter.begin(ip, username); !admitted {
		t.Fatal("threshold request was rejected before password verification")
	}
	if _, locked := limiter.recordFailure(ip, username); !locked {
		t.Fatal("successful login reset failures against unrelated usernames from the same IP")
	}
	limiter.end(ip, username)
	if _, admitted := limiter.begin(ip, "another-candidate"); admitted {
		limiter.end(ip, "another-candidate")
		t.Fatal("IP flood was admitted after reaching the threshold")
	}
}

func TestLoginRateLimiterSuccessClearsAccountAndPairFailures(t *testing.T) {
	limiter := newLoginRateLimiter()
	username := "admin"

	for i := 0; i < loginAccountMaxFailures-1; i++ {
		limiter.recordFailure(fmt.Sprintf("192.0.2.%d", i+1), username)
	}
	pairIP := "203.0.113.10"
	for i := 0; i < loginPairMaxFailures-1; i++ {
		limiter.recordFailure(pairIP, "pair-user")
	}

	limiter.recordSuccess("198.51.100.30", username)
	limiter.recordSuccess(pairIP, "pair-user")
	if _, locked := limiter.recordFailure("198.51.100.31", username); locked {
		t.Fatal("successful login did not clear the account failure history")
	}
	if _, locked := limiter.recordFailure(pairIP, "pair-user"); locked {
		t.Fatal("successful login did not clear the source/account failure history")
	}
}

func TestLoginRateLimiterCapsConcurrentPasswordChecks(t *testing.T) {
	limiter := newLoginRateLimiter()
	for i := 0; i < loginBcryptConcurrency; i++ {
		ip := fmt.Sprintf("192.0.2.%d", i+1)
		if _, admitted := limiter.begin(ip, "admin"); !admitted {
			t.Fatalf("password check %d was not admitted", i+1)
		}
	}
	if retryAfter, admitted := limiter.begin("192.0.2.99", "admin"); admitted {
		limiter.end("192.0.2.99", "admin")
		t.Fatal("password check beyond the concurrency cap was admitted")
	} else if retryAfter != loginBcryptBusyRetry {
		t.Fatalf("busy retry = %s, want %s", retryAfter, loginBcryptBusyRetry)
	}
	limiter.end("192.0.2.1", "admin")
	if _, admitted := limiter.begin("192.0.2.99", "admin"); !admitted {
		t.Fatal("password check was not admitted after a slot was released")
	}
	limiter.end("192.0.2.99", "admin")
	limiter.end("192.0.2.2", "admin")
}

func TestLoginRateLimiterStopsThousandUsernameFloodBeforeBcrypt(t *testing.T) {
	limiter := newLoginRateLimiter()
	const attempts = 1000
	admitted := 0
	rejected := 0
	for i := 0; i < attempts; i++ {
		username := fmt.Sprintf("candidate-%d", i)
		if _, ok := limiter.begin("198.51.100.10", username); !ok {
			rejected++
			continue
		}
		admitted++
		limiter.recordFailure("198.51.100.10", username)
		limiter.end("198.51.100.10", username)
	}
	if admitted != loginIPMaxFailures {
		t.Fatalf("bcrypt admissions = %d, want %d", admitted, loginIPMaxFailures)
	}
	if rejected != attempts-loginIPMaxFailures {
		t.Fatalf("fast rejections = %d, want %d", rejected, attempts-loginIPMaxFailures)
	}
	if len(limiter.attempts) > loginLimiterMaxEntries {
		t.Fatalf("limiter state = %d, exceeds %d", len(limiter.attempts), loginLimiterMaxEntries)
	}
	for key := range limiter.attempts {
		if strings.Contains(key, "candidate-") {
			t.Fatalf("raw username retained in limiter key %q", key)
		}
	}
}

func TestLoginRateLimiterFailsClosedAndPreservesActiveStateAtCapacity(t *testing.T) {
	limiter := newLoginRateLimiter()
	limiter.maxEntries = 6
	for i := 0; i < loginPairMaxFailures; i++ {
		limiter.recordFailure("203.0.113.1", "locked-user")
	}
	limiter.recordFailure("203.0.113.2", "counting-user")

	if _, admitted := limiter.begin("203.0.113.3", "new-user"); admitted {
		limiter.end("203.0.113.3", "new-user")
		t.Fatal("capacity pressure reached bcrypt instead of failing closed")
	}
	if len(limiter.bcryptSlots) != 0 {
		t.Fatal("capacity rejection leaked a bcrypt slot")
	}
	if len(limiter.attempts) != limiter.maxEntries || limiter.recency.Len() != len(limiter.attempts) {
		t.Fatalf("bounded state = %d, LRU = %d, capacity = %d", len(limiter.attempts), limiter.recency.Len(), limiter.maxEntries)
	}
	if _, admitted := limiter.begin("203.0.113.1", "locked-user"); admitted {
		limiter.end("203.0.113.1", "locked-user")
		t.Fatal("capacity pressure evicted an active lock")
	}
	for i := 1; i < loginPairMaxFailures; i++ {
		limiter.recordFailure("203.0.113.2", "counting-user")
	}
	if _, admitted := limiter.begin("203.0.113.2", "counting-user"); admitted {
		limiter.end("203.0.113.2", "counting-user")
		t.Fatal("capacity pressure evicted an accumulating counter")
	}
}

func newSettingsTestServer(t *testing.T) *Server {
	t.Helper()
	database, err := store.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return &Server{
		store:               database,
		logger:              regionTestLogger(),
		maxRequestBodyBytes: 4096,
		access:              defaultAccessConfig(),
	}
}

func TestHandleSecuritySettingsRoundTrip(t *testing.T) {
	server := newSettingsTestServer(t)
	body := `{"mode":"internal","allowed_cidrs":["203.0.113.0/24"],"trust_proxy_headers":true,"trusted_proxy_cidrs":["192.168.2.0/24"]}`
	request := httptest.NewRequest(http.MethodPut, "/api/settings/security", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.RemoteAddr = "192.168.2.20:5000"
	recorder := httptest.NewRecorder()
	server.handleSecuritySettings(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if server.currentAccessConfig().trustProxy != true {
		t.Fatal("runtime access config was not updated")
	}
	// Persisted?
	setting, err := server.store.AppSetting(context.Background(), accessSettingKey)
	if err != nil || !strings.Contains(string(setting.Value), "203.0.113.0/24") || !strings.Contains(string(setting.Value), "192.168.2.0/24") {
		t.Fatalf("access policy not persisted: %v %v", setting, err)
	}
	// GET reflects it.
	getRec := httptest.NewRecorder()
	getReq := httptest.NewRequest(http.MethodGet, "/api/settings/security", nil)
	getReq.RemoteAddr = "192.168.2.20:5000"
	server.handleSecuritySettings(getRec, getReq)
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(getRec.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data["trust_proxy_headers"] != true || envelope.Data["client_allowed"] != true {
		t.Fatalf("GET data = %v", envelope.Data)
	}
	trustedCIDRs, ok := envelope.Data["trusted_proxy_cidrs"].([]any)
	if !ok || len(trustedCIDRs) != 1 || trustedCIDRs[0] != "192.168.2.0/24" {
		t.Fatalf("GET trusted_proxy_cidrs = %v", envelope.Data["trusted_proxy_cidrs"])
	}
}

func TestHandleSecuritySettingsRejectsBadPolicy(t *testing.T) {
	server := newSettingsTestServer(t)
	request := httptest.NewRequest(http.MethodPut, "/api/settings/security", strings.NewReader(`{"mode":"nowhere"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	server.handleSecuritySettings(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
}

func TestHandleLoggingSettingsRoundTripAndEnforceCount(t *testing.T) {
	server := newSettingsTestServer(t)
	// Seed 10 log rows.
	for i := 0; i < 10; i++ {
		if _, err := server.store.AppendLogEvent(context.Background(), store.LogEvent{
			Level: "info", Message: "entry",
		}); err != nil {
			t.Fatal(err)
		}
	}
	// Keep only the newest 4.
	request := httptest.NewRequest(http.MethodPut, "/api/settings/logging", strings.NewReader(`{"mode":"count","count":4}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	server.handleLoggingSettings(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	count, err := server.store.CountLogEvents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if count != 4 {
		t.Fatalf("stored log count = %d, want 4 after retention", count)
	}
}

func TestLoggingCountIsClampedToHardLimit(t *testing.T) {
	config, err := parseLoggingConfig(loggingConfig{Mode: "count", Count: store.MaxLogEvents + 500})
	if err != nil {
		t.Fatal(err)
	}
	if config.Count != store.MaxLogEvents {
		t.Fatalf("count = %d, want %d", config.Count, store.MaxLogEvents)
	}
}

func TestPeriodicRetentionPrunesExpiredAuditEvents(t *testing.T) {
	server := newSettingsTestServer(t)
	now := time.Now().UTC()
	if _, err := server.store.AppendAuditEvent(context.Background(), store.AuditEvent{
		Action: "retention-test", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := server.applyPeriodicRetention(context.Background(), now.Add(store.AuditRetention+time.Second)); err != nil {
		t.Fatal(err)
	}
	events, err := server.store.ListAuditEvents(context.Background(), store.AuditFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("expired audit events retained: %#v", events)
	}
}

func TestLoginLockoutViaHTTP(t *testing.T) {
	app := newTestApplication(t)
	for i := 0; i < 4; i++ {
		response, err := app.client.Post(app.server.URL+"/api/auth/login", "application/json",
			strings.NewReader(`{"username":"admin","password":"wrong"}`))
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d, want 401", i+1, response.StatusCode)
		}
	}
	// Fifth consecutive failure crosses the threshold and locks.
	response, err := app.client.Post(app.server.URL+"/api/auth/login", "application/json",
		strings.NewReader(`{"username":"admin","password":"wrong"}`))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("5th failure status = %d, want 429", response.StatusCode)
	}
	// Even the correct password is refused while locked.
	response, err = app.client.Post(app.server.URL+"/api/auth/login", "application/json",
		strings.NewReader(`{"username":"admin","password":"correct-password"}`))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("locked login status = %d, want 429", response.StatusCode)
	}
}
