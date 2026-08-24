package main

import (
	"context"
	"errors"
	"log/slog"

	"vocat/internal/store"
)

func runDevelop(_ []string, _ *slog.Logger) error {
	return errors.New("developer mode, third-party plugins, and Export Proxy are disabled in hardened builds")
}

func isDeveloperEnabled(context.Context, *store.Store) bool {
	return false
}
