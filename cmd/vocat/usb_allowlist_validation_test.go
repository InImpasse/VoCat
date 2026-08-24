package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDJIInstalledAllowlistValidation(t *testing.T) {
	script := readUSBPassthroughAsset(t, "../../scripts/configure-dji-usb-passthrough.sh")
	validator := extractShellFunction(t, script, "validate_installed_allowlist")
	if !strings.Contains(script, `validate_installed_allowlist "$candidate_allowlist" "$CONFIG_TARGET" root:root:600:1`) {
		t.Fatal("production allowlist validation does not require root:root mode 0600 with one link")
	}

	identity := "DOMAIN=vocat\nVENDOR_ID=2ca3\nPRODUCT_ID=4006\nSERIAL=TEST-SERIAL-42\nID_PATH=pci-0000:00:14.0-usb-0:1\n"
	metadata := currentFileMetadata(t)

	tests := []struct {
		name        string
		installed   string
		mode        os.FileMode
		mutate      func(*testing.T, string)
		wantSuccess bool
	}{
		{name: "not installed", wantSuccess: true},
		{name: "exact identity", installed: identity, mode: 0o600, wantSuccess: true},
		{name: "serial differs", installed: strings.Replace(identity, "TEST-SERIAL-42", "OTHER-SERIAL-99", 1), mode: 0o600},
		{name: "physical path differs", installed: strings.Replace(identity, "usb-0:1", "usb-0:2", 1), mode: 0o600},
		{name: "missing key", installed: strings.Replace(identity, "SERIAL=TEST-SERIAL-42\n", "", 1), mode: 0o600},
		{name: "duplicate key", installed: identity + "SERIAL=TEST-SERIAL-42\n", mode: 0o600},
		{name: "world readable", installed: identity, mode: 0o644},
		{
			name:      "symbolic link",
			installed: identity,
			mode:      0o600,
			mutate: func(t *testing.T, installed string) {
				t.Helper()
				target := installed + ".target"
				if err := os.Rename(installed, target); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, installed); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name:      "hard link",
			installed: identity,
			mode:      0o600,
			mutate: func(t *testing.T, installed string) {
				t.Helper()
				if err := os.Link(installed, installed+".alias"); err != nil {
					t.Fatal(err)
				}
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			directory := t.TempDir()
			candidate := filepath.Join(directory, "candidate.conf")
			installed := filepath.Join(directory, "installed.conf")
			if err := os.WriteFile(candidate, []byte(identity), 0o600); err != nil {
				t.Fatal(err)
			}
			if testCase.installed != "" {
				if err := os.WriteFile(installed, []byte(testCase.installed), testCase.mode); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(installed, testCase.mode); err != nil {
					t.Fatal(err)
				}
			}
			if testCase.mutate != nil {
				testCase.mutate(t, installed)
			}

			output, err := runAllowlistValidator(t, validator, candidate, installed, metadata)
			if testCase.wantSuccess && err != nil {
				t.Fatalf("trusted allowlist was rejected: %v\n%s", err, output)
			}
			if !testCase.wantSuccess && err == nil {
				t.Fatal("unsafe or mismatched allowlist was accepted")
			}
			if strings.Contains(output, "TEST-SERIAL-42") || strings.Contains(output, "pci-0000:00:14.0") {
				t.Fatalf("allowlist validator disclosed a device identity: %s", output)
			}
		})
	}
}

func extractShellFunction(t *testing.T, script, name string) string {
	t.Helper()
	startMarker := name + "() {\n"
	start := strings.Index(script, startMarker)
	if start < 0 {
		t.Fatalf("cannot locate shell function %s", name)
	}
	end := strings.Index(script[start:], "\n}\n")
	if end < 0 {
		t.Fatalf("shell function %s is unterminated", name)
	}
	return script[start : start+end+3]
}

func currentFileMetadata(t *testing.T) string {
	t.Helper()
	owner, err := exec.Command("id", "-un").Output()
	if err != nil {
		t.Fatal(err)
	}
	group, err := exec.Command("id", "-gn").Output()
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%s:%s:600:1", strings.TrimSpace(string(owner)), strings.TrimSpace(string(group)))
}

func runAllowlistValidator(t *testing.T, validator, candidate, installed, metadata string) (string, error) {
	t.Helper()
	harness := "set -Eeuo pipefail\n" +
		"die() { printf 'ERROR: %s\\n' \"$*\" >&2; exit 1; }\n" +
		validator + "\n" +
		"validate_installed_allowlist \"$1\" \"$2\" \"$3\"\n"
	command := exec.Command("bash", "-c", harness, "allowlist-test", candidate, installed, metadata)
	output, err := command.CombinedOutput()
	return string(output), err
}
