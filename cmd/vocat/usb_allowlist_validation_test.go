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
	if !strings.Contains(script, `validate_installed_allowlist "$CONFIG_TARGET" root:root:600:1`) {
		t.Fatal("production allowlist validation does not require root:root mode 0600 with one link")
	}

	identity := "SCHEMA=2\nDOMAIN=vocat\nVENDOR_ID=2ca3\nPRODUCT_ID=4006\nDEVICE_COUNT=2\nDEVICE_1_SERIAL=TEST-SERIAL-42\nDEVICE_1_ID_PATH=test-usb-port-a\nDEVICE_2_SERIAL=\nDEVICE_2_ID_PATH=test-usb-port-b\n"
	metadata := currentFileMetadata(t)

	tests := []struct {
		name        string
		installed   string
		mode        os.FileMode
		mutate      func(*testing.T, string)
		wantSuccess bool
	}{
		{name: "not installed", wantSuccess: true},
		{name: "regular root-only candidate", installed: identity, mode: 0o600, wantSuccess: true},
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
			if strings.Contains(output, "TEST-SERIAL-42") || strings.Contains(output, "test-usb-port-a") {
				t.Fatalf("allowlist validator disclosed a device identity: %s", output)
			}
		})
	}
}

func TestDJIAllowlistSchemaRejectsMalformedOrAmbiguousEntries(t *testing.T) {
	script := readUSBPassthroughAsset(t, "../../scripts/vocat-usb-hotplug.sh")
	configValue := extractShellFunction(t, script, "config_value")
	loadAllowlist := extractShellFunction(t, script, "load_allowlist")
	validTwo := "SCHEMA=2\nDOMAIN=vocat\nVENDOR_ID=2ca3\nPRODUCT_ID=4006\nDEVICE_COUNT=2\nDEVICE_1_SERIAL=\nDEVICE_1_ID_PATH=test-usb-port-a\nDEVICE_2_SERIAL=TEST-SERIAL-42\nDEVICE_2_ID_PATH=test-usb-port-b\n"
	validOne := "SCHEMA=2\nDOMAIN=vocat\nVENDOR_ID=2ca3\nPRODUCT_ID=4006\nDEVICE_COUNT=1\nDEVICE_1_SERIAL=\nDEVICE_1_ID_PATH=test-usb-port-a\n"
	validThree := "SCHEMA=2\nDOMAIN=vocat\nVENDOR_ID=2ca3\nPRODUCT_ID=4006\nDEVICE_COUNT=3\nDEVICE_1_SERIAL=\nDEVICE_1_ID_PATH=test-usb-port-a\nDEVICE_2_SERIAL=TEST-SERIAL-42\nDEVICE_2_ID_PATH=test-usb-port-b\nDEVICE_3_SERIAL=\nDEVICE_3_ID_PATH=test-usb-port-c\n"

	tests := []struct {
		name        string
		config      string
		wantSuccess bool
	}{
		{name: "one serialless device", config: validOne, wantSuccess: true},
		{name: "two devices with optional serial", config: validTwo, wantSuccess: true},
		{name: "three devices", config: validThree, wantSuccess: true},
		{name: "old schema", config: strings.Replace(validTwo, "SCHEMA=2", "SCHEMA=1", 1)},
		{name: "zero devices", config: strings.Replace(validTwo, "DEVICE_COUNT=2", "DEVICE_COUNT=0", 1)},
		{name: "too many devices", config: strings.Replace(validTwo, "DEVICE_COUNT=2", "DEVICE_COUNT=256", 1)},
		{name: "duplicate path", config: strings.Replace(validTwo, "test-usb-port-b", "test-usb-port-a", 1)},
		{name: "invalid serial", config: strings.Replace(validTwo, "TEST-SERIAL-42", "invalid serial", 1)},
		{name: "missing key", config: strings.Replace(validTwo, "DEVICE_2_SERIAL=TEST-SERIAL-42\n", "", 1)},
		{name: "duplicate key", config: validTwo + "DEVICE_2_SERIAL=TEST-SERIAL-42\n"},
		{name: "unknown key", config: validTwo + "UNEXPECTED=value\n"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "dji-usb.conf")
			if err := os.WriteFile(configPath, []byte(testCase.config), 0o600); err != nil {
				t.Fatal(err)
			}
			harness := "set -Eeuo pipefail\nCONFIG_FILE=$1\n" +
				"die() { printf 'ERROR: %s\\n' \"$*\" >&2; exit 1; }\n" +
				configValue + "\n" + loadAllowlist + "\nload_allowlist\n"
			command := exec.Command("bash", "-c", harness, "allowlist-schema-test", configPath)
			output, err := command.CombinedOutput()
			if testCase.wantSuccess && err != nil {
				t.Fatalf("valid schema was rejected: %v\n%s", err, output)
			}
			if !testCase.wantSuccess && err == nil {
				t.Fatal("malformed or ambiguous schema was accepted")
			}
			if strings.Contains(string(output), "TEST-SERIAL-42") || strings.Contains(string(output), "test-usb-port-a") {
				t.Fatalf("schema validator disclosed a device identity: %s", output)
			}
		})
	}
}

func TestDJIPathBindingRequiresSerialOnlyWhenEnrolled(t *testing.T) {
	script := readUSBPassthroughAsset(t, "../../scripts/vocat-usb-hotplug.sh")
	matcher := extractShellFunction(t, script, "identity_matches")
	tests := []struct {
		name            string
		enrolledSerial  string
		enrolledPath    string
		candidateSerial string
		candidatePath   string
		wantMatch       bool
	}{
		{name: "serialless exact path", enrolledPath: "test-usb-port-a", candidatePath: "test-usb-port-a", wantMatch: true},
		{name: "serialless moved port", enrolledPath: "test-usb-port-a", candidatePath: "test-usb-port-b"},
		{name: "serial exact identity", enrolledSerial: "TEST-SERIAL-42", enrolledPath: "test-usb-port-a", candidateSerial: "TEST-SERIAL-42", candidatePath: "test-usb-port-a", wantMatch: true},
		{name: "serial mismatch", enrolledSerial: "TEST-SERIAL-42", enrolledPath: "test-usb-port-a", candidateSerial: "OTHER-SERIAL-99", candidatePath: "test-usb-port-a"},
		{name: "serial device moved port", enrolledSerial: "TEST-SERIAL-42", enrolledPath: "test-usb-port-a", candidateSerial: "TEST-SERIAL-42", candidatePath: "test-usb-port-b"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			harness := "set -Eeuo pipefail\nserial=$1\nid_path=$2\n" + matcher +
				"\nidentity_matches \"$3\" \"$4\"\n"
			command := exec.Command("bash", "-c", harness, "identity-test",
				testCase.enrolledSerial, testCase.enrolledPath, testCase.candidateSerial, testCase.candidatePath)
			err := command.Run()
			if testCase.wantMatch && err != nil {
				t.Fatalf("matching identity was rejected: %v", err)
			}
			if !testCase.wantMatch && err == nil {
				t.Fatal("mismatched identity was accepted")
			}
		})
	}
}

func TestDJIHotplugRequiresSingleLinkAllowlist(t *testing.T) {
	script := readUSBPassthroughAsset(t, "../../scripts/vocat-usb-hotplug.sh")
	if !strings.Contains(script, `stat -c '%U:%G:%a:%h' "$CONFIG_FILE"`) ||
		!strings.Contains(script, `root:root:600:1`) {
		t.Fatal("hotplug does not preserve the root-only single-link allowlist boundary")
	}
}

func TestDJIAllowlistUpdatePreservesExistingSlots(t *testing.T) {
	script := readUSBPassthroughAsset(t, "../../scripts/configure-dji-usb-passthrough.sh")
	for _, required := range []string{
		`preserve_installed_slots`,
		`selected_by_path[${id_paths[$index]}]=$index`,
		`every previously enrolled DJI device must be connected when updating the allowlist`,
		`[[ ${serials[$index]} == "$existing_serial" ]]`,
		`ordered_sysnames+=("${sysnames[$index]}")`,
	} {
		if !strings.Contains(script, required) {
			t.Errorf("allowlist growth does not preserve existing slots: missing %q", required)
		}
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
		"validate_installed_allowlist \"$2\" \"$3\"\n"
	command := exec.Command("bash", "-c", harness, "allowlist-test", candidate, installed, metadata)
	output, err := command.CombinedOutput()
	return string(output), err
}
