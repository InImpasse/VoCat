package update

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRunRequiresExplicitRepository(t *testing.T) {
	t.Setenv("VOCAT_REPO", "")
	if err := Run(slog.Default(), nil); !errors.Is(err, errRepositoryRequired) {
		t.Fatalf("Run() error = %v, want %v", err, errRepositoryRequired)
	}
}

func TestApplyLatestRequiresExplicitRepository(t *testing.T) {
	if _, err := ApplyLatest(context.Background(), slog.Default(), Options{}, false); !errors.Is(err, errRepositoryRequired) {
		t.Fatalf("ApplyLatest() error = %v, want %v", err, errRepositoryRequired)
	}
}

func TestValidateExecutableRejectsNonExecutableFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-vocat")
	if err := os.WriteFile(path, []byte("not an executable"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := validateExecutable(context.Background(), path); err == nil {
		t.Fatal("validateExecutable accepted invalid file")
	}
}

func TestBackupAndReplaceRetainsPreviousBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Linux replacement behavior")
	}
	directory := t.TempDir()
	target := filepath.Join(directory, "vocat")
	replacement := filepath.Join(directory, "replacement")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(replacement, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := backupAndReplace(target, replacement); err != nil {
		t.Fatal(err)
	}
	old, err := os.ReadFile(target + ".previous")
	if err != nil {
		t.Fatalf("read retained backup: %v", err)
	}
	if string(old) != "old" {
		t.Fatalf("backup = %q", old)
	}
}
