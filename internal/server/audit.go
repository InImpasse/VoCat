package server

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"vocat/internal/store"
)

// recordAudit writes one security-relevant event to the audit trail. Failures
// are logged but never block the request being audited.
func (s *Server) recordAudit(
	ctx context.Context,
	actor string,
	action string,
	entityType string,
	entityID string,
	outcome string,
	remoteAddr string,
) {
	event, err := store.NormalizeAuditEvent(store.AuditEvent{
		Actor:      actor,
		Action:     action,
		EntityType: entityType,
		EntityID:   entityID,
		Outcome:    outcome,
		RemoteAddr: remoteAddr,
		CreatedAt:  time.Now().UTC(),
	})
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("discard invalid audit event", "category", "system", "raw_error", err)
		}
		return
	}
	level := slog.LevelInfo
	if !strings.EqualFold(event.Outcome, "success") {
		level = slog.LevelWarn
	}
	if s.logger != nil {
		s.logger.Log(ctx, level, "user operation",
			"category", auditLogCategory(event.Action),
			"event", event.Action,
			"actor", event.Actor,
			"entity_type", event.EntityType,
			"entity_id", event.EntityID,
			"outcome", event.Outcome,
		)
	}
	if s.store == nil {
		return
	}
	_, err = s.store.AppendAuditEvent(ctx, event)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("write audit event failed", "category", "system", "action", event.Action, "raw_error", err)
		}
	}
}

func auditLogCategory(action string) string {
	action = strings.ToLower(strings.TrimSpace(action))
	switch {
	case strings.Contains(action, ".sms") || strings.HasPrefix(action, "sms."):
		return "sms"
	case strings.Contains(action, ".call") || strings.HasPrefix(action, "call."):
		return "call"
	case strings.Contains(action, "vowifi") || strings.Contains(action, "ims"):
		return "vowifi"
	case strings.Contains(action, "device") || strings.Contains(action, "esim") ||
		strings.Contains(action, ".at.") || strings.Contains(action, ".ussd"):
		return "hardware"
	default:
		return "operation"
	}
}

// audit records an event for an already-authenticated request, resolving the
// actor from the session and the source address through the configured trusted
// proxy policy.
func (s *Server) audit(r *http.Request, action string, entityType string, entityID string, outcome string) {
	actor := ""
	if s.auth != nil {
		if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
			if session, authErr := s.auth.Authenticate(r.Context(), cookie.Value); authErr == nil {
				actor = session.Principal.Username
			}
		}
	}
	s.recordAudit(r.Context(), actor, action, entityType, entityID, outcome, s.requestClientIP(r))
}

// auditAuth records an authentication event where no session exists yet (the
// actor is the username that was attempted).
func (s *Server) auditAuth(r *http.Request, username string, outcome string) {
	s.recordAudit(r.Context(), username, "auth.login", "session", username, outcome, s.requestClientIP(r))
}

func (s *Server) requestClientIP(r *http.Request) string {
	address := s.currentAccessConfig().clientIP(r)
	if !address.IsValid() {
		return ""
	}
	return address.String()
}
