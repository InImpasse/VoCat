package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHandleWebsheetCallbackRejectsFieldsOverRawByteLimit(t *testing.T) {
	tests := []struct {
		name   string
		street string
	}{
		{
			name:   "surrounding whitespace",
			street: strings.Repeat(" ", 126) + "Main" + strings.Repeat(" ", 127),
		},
		{
			name:   "all whitespace",
			street: strings.Repeat(" ", maxE911FieldBytes+1),
		},
		{
			name:   "multibyte",
			street: strings.Repeat("界", 85) + "ab",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := len(test.street); got != maxE911FieldBytes+1 {
				t.Fatalf("test field length = %d, want %d", got, maxE911FieldBytes+1)
			}
			body, err := json.Marshal(e911Address{
				Street:  test.street,
				City:    "Springfield",
				Country: "US",
			})
			if err != nil {
				t.Fatal(err)
			}
			server := &Server{
				websheets:           newWebsheetManager(),
				maxRequestBodyBytes: 4096,
			}
			request := httptest.NewRequest(http.MethodPost, "/websheets/missing/callback", bytes.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()

			server.handleWebsheetCallback(response, request, &websheetSession{id: "missing"})

			if response.Code != http.StatusBadRequest {
				t.Fatalf("callback status = %d, want 400; body=%s", response.Code, response.Body.String())
			}
			var envelope errorEnvelope
			if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.Error.Code != "invalid_e911_address" {
				t.Fatalf("error code = %q, want invalid_e911_address", envelope.Error.Code)
			}
			wantMessage := `E911 address field "street" exceeds 256 bytes`
			if envelope.Error.Message != wantMessage {
				t.Fatalf("error message = %q, want %q", envelope.Error.Message, wantMessage)
			}
		})
	}
}

func TestNormalizeE911AddressRawByteBoundaries(t *testing.T) {
	tests := []struct {
		name       string
		street     string
		wantBytes  int
		wantStreet string
		wantError  string
	}{
		{
			name:       "exact ASCII boundary with surrounding whitespace",
			street:     strings.Repeat(" ", 126) + "Main" + strings.Repeat(" ", 126),
			wantBytes:  maxE911FieldBytes,
			wantStreet: "Main",
		},
		{
			name:       "exact multibyte boundary",
			street:     strings.Repeat("界", 85) + "a",
			wantBytes:  maxE911FieldBytes,
			wantStreet: strings.Repeat("界", 85) + "a",
		},
		{
			name:      "exact boundary all whitespace",
			street:    strings.Repeat(" ", maxE911FieldBytes),
			wantBytes: maxE911FieldBytes,
			wantError: `E911 address field "street" is required`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := len(test.street); got != test.wantBytes {
				t.Fatalf("test field length = %d, want %d", got, test.wantBytes)
			}
			clean, err := normalizeE911Address(e911Address{
				Street:  test.street,
				City:    "Springfield",
				Country: "US",
			})
			if test.wantError != "" {
				if err == nil || err.Error() != test.wantError {
					t.Fatalf("normalize error = %v, want %q", err, test.wantError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if clean.Street != test.wantStreet {
				t.Fatalf("normalized street = %q, want %q", clean.Street, test.wantStreet)
			}
		})
	}
}

func TestWebsheetManagerEnforcesSessionLimit(t *testing.T) {
	manager := newWebsheetManager()
	first, err := manager.create("device-0")
	if err != nil {
		t.Fatal(err)
	}
	manager.mu.Lock()
	manager.sessions[first.id].createdAt = time.Unix(0, 0).UTC()
	manager.mu.Unlock()
	for index := 1; index <= maxWebsheetSessions; index++ {
		if _, err := manager.create(fmt.Sprintf("device-%d", index)); err != nil {
			t.Fatal(err)
		}
	}
	if got := len(manager.sessions); got != maxWebsheetSessions {
		t.Fatalf("session count = %d, want %d", got, maxWebsheetSessions)
	}
	if manager.get(first.id) != nil {
		t.Fatal("oldest websheet session was not evicted")
	}
}

func TestWebsheetManagerRequiresCallbackBeforeCompletion(t *testing.T) {
	manager := newWebsheetManager()
	session, err := manager.create("device-1")
	if err != nil {
		t.Fatal(err)
	}
	if manager.complete(session.id) {
		t.Fatal("websheet completed before its callback")
	}
	digest := sha256.Sum256([]byte("address-a"))
	otherDigest := sha256.Sum256([]byte("address-b"))
	if _, result := manager.beginCallback(session.id, digest); result != websheetCallbackClaimed {
		t.Fatal("websheet callback was not claimed")
	}
	if _, result := manager.beginCallback(session.id, digest); result != websheetCallbackUnavailable {
		t.Fatal("concurrent websheet callback was accepted")
	}
	manager.finishCallback(session.id, digest, true)
	if _, result := manager.beginCallback(session.id, digest); result != websheetCallbackReplay {
		t.Fatal("identical completed callback was not accepted idempotently")
	}
	if _, result := manager.beginCallback(session.id, otherDigest); result != websheetCallbackUnavailable {
		t.Fatal("different callback payload was accepted after persistence")
	}
	if !manager.complete(session.id) {
		t.Fatal("submitted websheet could not be completed")
	}
	if !manager.complete(session.id) {
		t.Fatal("completed websheet was not idempotent")
	}
	if manager.get(session.id) == nil {
		t.Fatal("completed websheet tombstone was not retained")
	}
	if _, result := manager.beginCallback(session.id, digest); result != websheetCallbackReplay {
		t.Fatal("identical callback was not recoverable after done")
	}
}

func TestWebsheetManagerProtectsActiveSessionsAtCapacity(t *testing.T) {
	manager := newWebsheetManager()
	var firstID string
	for index := 0; index < maxWebsheetSessions; index++ {
		session, err := manager.create(fmt.Sprintf("device-%d", index))
		if err != nil {
			t.Fatal(err)
		}
		if index == 0 {
			firstID = session.id
		}
		manager.mu.Lock()
		manager.sessions[session.id].callbackComplete = true
		manager.sessions[session.id].callbackHasDigest = true
		manager.sessions[session.id].callbackDigest = sha256.Sum256([]byte(session.id))
		manager.mu.Unlock()
	}
	if _, err := manager.create("overflow"); !errors.Is(err, errWebsheetCapacity) {
		t.Fatalf("create at protected capacity error = %v, want %v", err, errWebsheetCapacity)
	}
	if manager.get(firstID) == nil {
		t.Fatal("waiting-done session was evicted")
	}

	manager.mu.Lock()
	manager.sessions[firstID].doneAt = time.Now().UTC()
	manager.mu.Unlock()
	if _, err := manager.create("replacement"); err != nil {
		t.Fatalf("create with tombstone victim: %v", err)
	}
	if manager.get(firstID) != nil {
		t.Fatal("completed tombstone was not evicted before protected sessions")
	}
}

func TestWebsheetManagerKeepsCallbackAcrossOriginalExpiry(t *testing.T) {
	manager := newWebsheetManager()
	session, err := manager.create("device-1")
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte("address"))
	if _, result := manager.beginCallback(session.id, digest); result != websheetCallbackClaimed {
		t.Fatal("websheet callback was not claimed")
	}
	manager.mu.Lock()
	manager.sessions[session.id].expiresAt = time.Now().UTC().Add(-time.Second)
	manager.mu.Unlock()
	if manager.get(session.id) == nil {
		t.Fatal("in-flight callback was removed after its original expiry")
	}
	manager.finishCallback(session.id, digest, true)
	if !manager.complete(session.id) {
		t.Fatal("callback could not complete after crossing its original expiry")
	}
}
