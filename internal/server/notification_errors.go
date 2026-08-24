package server

import (
	"net/url"
	"strconv"
	"strings"

	"vocat/internal/store"
)

const redactedNotificationURL = "[REDACTED URL]"

// notificationErrorText removes request URLs before an error reaches slog.
// Provider-aware redaction then covers credentials repeated outside the URL.
func notificationErrorText(err error, providers ...store.SensitiveValuesProvider) string {
	if err == nil {
		return ""
	}
	text := redactNotificationErrorURLs(err.Error(), err, 0)
	return store.RedactText(text, providers...)
}

func redactNotificationErrorURLs(text string, err error, depth int) string {
	if err == nil || depth >= 32 {
		return text
	}
	if requestErr, ok := err.(*url.Error); ok && requestErr.URL != "" {
		text = strings.ReplaceAll(text, strconv.Quote(requestErr.URL), strconv.Quote(redactedNotificationURL))
		text = strings.ReplaceAll(text, requestErr.URL, redactedNotificationURL)
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, nested := range joined.Unwrap() {
			text = redactNotificationErrorURLs(text, nested, depth+1)
		}
		return text
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		return redactNotificationErrorURLs(text, wrapped.Unwrap(), depth+1)
	}
	return text
}
