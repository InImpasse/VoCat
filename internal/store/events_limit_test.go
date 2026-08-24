package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestNormalizeAuditEventBoundsUntrustedFields(t *testing.T) {
	value, err := NormalizeAuditEvent(AuditEvent{
		Actor:      strings.Repeat("界", MaxAuditActorBytes),
		Action:     strings.Repeat("a", MaxAuditActionBytes+10),
		EntityType: strings.Repeat("t", MaxAuditEntityTypeBytes+10),
		EntityID:   strings.Repeat("界", MaxAuditEntityIDBytes),
		Outcome:    strings.Repeat("o", MaxAuditOutcomeBytes+10),
		RemoteAddr: strings.Repeat("r", MaxAuditRemoteAddrBytes+10),
	})
	if err != nil {
		t.Fatal(err)
	}
	fields := []struct {
		name string
		text string
		max  int
	}{
		{"actor", value.Actor, MaxAuditActorBytes},
		{"action", value.Action, MaxAuditActionBytes},
		{"entity type", value.EntityType, MaxAuditEntityTypeBytes},
		{"entity ID", value.EntityID, MaxAuditEntityIDBytes},
		{"outcome", value.Outcome, MaxAuditOutcomeBytes},
		{"remote address", value.RemoteAddr, MaxAuditRemoteAddrBytes},
	}
	for _, field := range fields {
		if len(field.text) > field.max {
			t.Errorf("%s length = %d, want <= %d", field.name, len(field.text), field.max)
		}
		if !utf8.ValidString(field.text) {
			t.Errorf("%s is not valid UTF-8", field.name)
		}
	}
}

func TestNormalizeAuditEventRejectsOversizedDetails(t *testing.T) {
	details, err := json.Marshal(map[string]string{"value": strings.Repeat("x", MaxAuditDetailsBytes)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NormalizeAuditEvent(AuditEvent{Action: "test", Details: details}); err == nil {
		t.Fatal("oversized audit details were accepted")
	}
}

func TestNormalizeAuditEventTrimsAfterTruncation(t *testing.T) {
	value, err := NormalizeAuditEvent(AuditEvent{
		Action: strings.Repeat("a", MaxAuditActionBytes-1) + " trailing",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasSuffix(value.Action, " ") {
		t.Fatalf("truncated action retains trailing whitespace: %q", value.Action)
	}
	if len(value.Action) != MaxAuditActionBytes-1 {
		t.Fatalf("truncated action length = %d, want %d", len(value.Action), MaxAuditActionBytes-1)
	}
}

func TestAppendAuditEventEnforcesAgeAndCountLimits(t *testing.T) {
	database, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().UTC()
	if _, err := database.db.ExecContext(context.Background(), `
		WITH RECURSIVE sequence(value) AS (
			SELECT 1 UNION ALL SELECT value + 1 FROM sequence WHERE value <= ?
		)
		INSERT INTO audit_events(
			actor, action, entity_type, entity_id, outcome, remote_addr,
			details_json, created_at
		)
		SELECT 'admin', 'seed', '', '', 'success', '', '{}', ?
		FROM sequence
	`, MaxAuditEvents, now.Add(-time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	if _, err := database.db.ExecContext(context.Background(), `
		INSERT INTO audit_events(
			actor, action, entity_type, entity_id, outcome, remote_addr,
			details_json, created_at
		) VALUES ('admin', 'expired', '', '', 'success', '', '{}', ?)
	`, now.Add(-AuditRetention-time.Second).Unix()); err != nil {
		t.Fatal(err)
	}
	if _, err := database.AppendAuditEvent(context.Background(), AuditEvent{
		Actor: "admin", Action: "newest", Outcome: "success", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	var count int
	if err := database.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM audit_events`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != MaxAuditEvents {
		t.Fatalf("audit count = %d, want %d", count, MaxAuditEvents)
	}
	var expired int
	if err := database.db.QueryRowContext(
		context.Background(),
		`SELECT COUNT(*) FROM audit_events WHERE action = 'expired'`,
	).Scan(&expired); err != nil {
		t.Fatal(err)
	}
	if expired != 0 {
		t.Fatalf("expired audit count = %d, want 0", expired)
	}
	audits, err := database.ListAuditEvents(context.Background(), AuditFilter{Limit: 1})
	if err != nil || len(audits) != 1 || audits[0].Action != "newest" {
		t.Fatalf("newest audit = %#v, %v", audits, err)
	}
}

func TestAppendLogEventEnforcesHardLimit(t *testing.T) {
	database, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	if _, err := database.db.ExecContext(context.Background(), `
		WITH RECURSIVE sequence(value) AS (
			SELECT 1 UNION ALL SELECT value + 1 FROM sequence WHERE value <= ?
		)
		INSERT INTO log_events(event_time, level, message, caller, fields_json)
		SELECT value, 'info', 'seed-' || value, '', '{}' FROM sequence
	`, MaxLogEvents); err != nil {
		t.Fatal(err)
	}
	if _, err := database.AppendLogEvent(context.Background(), LogEvent{
		Level: "info", Message: "newest", Time: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	count, err := database.CountLogEvents(context.Background())
	if err != nil || count != MaxLogEvents {
		t.Fatalf("CountLogEvents = %d, %v; want %d", count, err, MaxLogEvents)
	}
	logs, err := database.ListLogEvents(context.Background(), LogFilter{Limit: 1})
	if err != nil || len(logs) != 1 || logs[0].Message != "newest" {
		t.Fatalf("newest log = %#v, %v", logs, err)
	}
}

func TestClearLogEventsRejectsAlreadyQueuedEntries(t *testing.T) {
	database, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	cutoff := time.Now().UTC()
	if _, err := database.AppendLogEvent(context.Background(), LogEvent{
		Level: "info", Message: "existing", Time: cutoff.Add(-time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	deleted, err := database.ClearLogEvents(context.Background(), cutoff)
	if err != nil || deleted != 1 {
		t.Fatalf("ClearLogEvents = %d, %v", deleted, err)
	}
	late, err := database.AppendLogEvent(context.Background(), LogEvent{
		Level: "info", Message: "queued-before-clear", Time: cutoff.Add(-time.Millisecond),
	})
	if err != nil || late.ID != 0 {
		t.Fatalf("old queued append = %+v, %v", late, err)
	}
	if _, err := database.AppendLogEvent(context.Background(), LogEvent{
		Level: "info", Message: fmt.Sprintf("new-%d", MaxLogEvents), Time: cutoff.Add(time.Millisecond),
	}); err != nil {
		t.Fatal(err)
	}
	count, err := database.CountLogEvents(context.Background())
	if err != nil || count != 1 {
		t.Fatalf("CountLogEvents = %d, %v; want 1", count, err)
	}
}
