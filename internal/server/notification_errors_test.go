package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"strings"
	"testing"
	"time"

	"vocat/internal/loghub"
	"vocat/internal/store"
)

func TestNotificationErrorTextProtectsMemoryAndPersistence(t *testing.T) {
	const (
		secret       = "123456:abcdefghijklmnopqrstuvwxyz"
		sensitiveURL = "https://user:password@notify.example.test/bot123456:abcdefghijklmnopqrstuvwxyz/send?key=private"
	)
	provider := store.NotificationSetting{
		Channel:         "telegram",
		Config:          json.RawMessage(`{"bot_token":"` + secret + `"}`),
		SensitiveFields: []string{"bot_token"},
	}
	requestErr := &url.Error{Op: "Post", URL: sensitiveURL, Err: errors.New("dial failed")}
	err := fmt.Errorf("delivery with token %s failed: %w", secret, errors.Join(requestErr))
	errorText := notificationErrorText(err, provider)
	if strings.Contains(errorText, secret) || strings.Contains(errorText, sensitiveURL) || strings.Contains(errorText, "password") {
		t.Fatalf("notification error was not redacted: %q", errorText)
	}
	if !strings.Contains(errorText, redactedNotificationURL) || !strings.Contains(errorText, "dial failed") {
		t.Fatalf("notification error lost its safe diagnostic detail: %q", errorText)
	}

	hub := loghub.New(slog.NewTextHandler(io.Discard, nil), 100)
	entries, cancel := hub.Subscribe(1)
	defer cancel()
	slog.New(hub).Warn("notification failed", "error", errorText)
	var entry loghub.Entry
	select {
	case entry = <-entries:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for notification log entry")
	}
	assertNoNotificationSecret(t, entry.Message, secret, sensitiveURL)
	encodedFields, err := json.Marshal(entry.Fields)
	if err != nil {
		t.Fatal(err)
	}
	assertNoNotificationSecret(t, string(encodedFields), secret, sensitiveURL)
	history := hub.History(1, slog.LevelDebug, "")
	if len(history) != 1 {
		t.Fatalf("history length = %d, want 1", len(history))
	}
	historyFields, _ := json.Marshal(history[0].Fields)
	assertNoNotificationSecret(t, string(historyFields), secret, sensitiveURL)

	database, err := store.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.AppendLogEvent(context.Background(), store.LogEvent{
		Time: entry.Time, Level: entry.Level, Message: entry.Message, Fields: encodedFields,
	}); err != nil {
		t.Fatal(err)
	}
	persisted, err := database.ListLogEvents(context.Background(), store.LogFilter{Limit: 1, Since: time.Unix(1, 0)})
	if err != nil || len(persisted) != 1 {
		t.Fatalf("persisted logs = %#v, error = %v", persisted, err)
	}
	assertNoNotificationSecret(t, persisted[0].Message+string(persisted[0].Fields), secret, sensitiveURL)
}

func assertNoNotificationSecret(t *testing.T, text string, secrets ...string) {
	t.Helper()
	for _, secret := range secrets {
		if strings.Contains(text, secret) {
			t.Fatalf("notification log contains sensitive value %q: %s", secret, text)
		}
	}
}
