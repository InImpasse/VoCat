package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestUSBHotplugIdentityValidatorBindsAddressAndIdentity(t *testing.T) {
	validatorPath := extractUSBHotplugIdentityValidator(t)
	validState := usbIdentityHostdev("2ca3", "4006", "ua-vocat-dji-usb", 1, 2, true)
	validLive := usbIdentityDomain(usbIdentityHostdev("2ca3", "4006", "ua-vocat-dji-usb", 1, 2, false))

	tests := []struct {
		name        string
		xml         string
		scope       string
		wantSuccess bool
	}{
		{name: "trusted state", xml: validState, scope: "state", wantSuccess: true},
		{name: "trusted inactive domain", xml: usbIdentityDomain(validState), scope: "config", wantSuccess: true},
		{name: "trusted live domain", xml: validLive, scope: "live", wantSuccess: true},
		{name: "bus drift", xml: usbIdentityDomain(usbIdentityHostdev("2ca3", "4006", "ua-vocat-dji-usb", 3, 2, true)), scope: "config"},
		{name: "device drift", xml: usbIdentityDomain(usbIdentityHostdev("2ca3", "4006", "ua-vocat-dji-usb", 1, 4, true)), scope: "config"},
		{name: "vendor drift", xml: usbIdentityDomain(usbIdentityHostdev("ffff", "4006", "ua-vocat-dji-usb", 1, 2, true)), scope: "config"},
		{name: "product drift", xml: usbIdentityDomain(usbIdentityHostdev("2ca3", "ffff", "ua-vocat-dji-usb", 1, 2, true)), scope: "config"},
		{name: "alias drift", xml: usbIdentityDomain(usbIdentityHostdev("2ca3", "4006", "ua-other", 1, 2, true)), scope: "config"},
		{name: "duplicate alias", xml: usbIdentityDomain(strings.Replace(validState, `</hostdev>`, `<alias name="ua-vocat-dji-usb"/></hostdev>`, 1)), scope: "config"},
		{name: "duplicate source address", xml: usbIdentityDomain(strings.Replace(validState, `</source>`, `<address bus="1" device="2"/></source>`, 1)), scope: "config"},
		{name: "missing hostdev", xml: usbIdentityDomain(""), scope: "config"},
		{name: "extra hostdev", xml: usbIdentityDomain(validState + validState), scope: "config"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			xmlPath := filepath.Join(t.TempDir(), "device.xml")
			if err := os.WriteFile(xmlPath, []byte(test.xml), 0o600); err != nil {
				t.Fatal(err)
			}
			command := exec.Command("python3", validatorPath, xmlPath, "ua-vocat-dji-usb", "2ca3", "4006", test.scope, "1", "2")
			output, err := command.CombinedOutput()
			if test.wantSuccess && err != nil {
				t.Fatalf("validator rejected matching identity: %v\n%s", err, output)
			}
			if !test.wantSuccess && err == nil {
				t.Fatal("validator accepted drifted USB identity")
			}
		})
	}
}

func TestUSBHotplugCheckBindsAllSnapshotsAfterAllowlistMatch(t *testing.T) {
	scriptBytes, err := os.ReadFile("../../scripts/vocat-usb-hotplug.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(scriptBytes)
	ordered := []string{
		`matches_allowlist "$sysname"`,
		`((match_count == 1))`,
		`bus_raw=$(</sys/bus/usb/devices/"$sysname"/busnum)`,
		`validate_check_identity "$bus_number" "$device_number"`,
		`USB allowlist check passed`,
	}
	previous := -1
	for _, fragment := range ordered {
		position := strings.Index(script, fragment)
		if position < 0 {
			t.Fatalf("USB check is missing identity-binding step %q", fragment)
		}
		if position <= previous {
			t.Fatalf("USB check performs %q out of order", fragment)
		}
		previous = position
	}

	for _, required := range []string{
		`[[ $recorded_sysname == "$sysname" ]]`,
		`assert_passthrough_identity "$CURRENT_STATE/device.xml" state "$expected_bus" "$expected_device"`,
		`dump_domain_xml config "$config_dump"`,
		`assert_passthrough_identity "$config_dump" config "$expected_bus" "$expected_device"`,
		`domain_state=$(read_domain_state) || return 1`,
		`dump_domain_xml live "$live_dump"`,
		`assert_passthrough_identity "$live_dump" live "$expected_bus" "$expected_device"`,
	} {
		if !strings.Contains(script, required) {
			t.Errorf("USB check does not bind snapshot identity with %q", required)
		}
	}

	callStart := strings.Index(script, `validate_check_identity "$bus_number" "$device_number"`)
	if callStart < 0 {
		t.Fatal("cannot inspect USB identity checker invocation")
	}
	callEnd := strings.Index(script[callStart:], "\n")
	if callEnd < 0 {
		t.Fatal("USB identity checker invocation is unterminated")
	}
	identityCall := script[callStart : callStart+callEnd]
	if strings.Contains(identityCall, "$serial") || strings.Contains(identityCall, "$id_path") {
		t.Fatalf("USB identity checker receives sensitive allowlist values as arguments: %s", identityCall)
	}
}

func TestUSBHotplugDomainStateErrorsFailClosed(t *testing.T) {
	script := readUSBPassthroughAsset(t, "../../scripts/vocat-usb-hotplug.sh")
	if strings.Contains(script, "domain_is_live") {
		t.Fatal("hotplug script still conflates an offline domain with a domstate query failure")
	}
	if count := strings.Count(script, `domain_state=$(read_domain_state) || return 1`); count != 3 {
		t.Fatalf("domain state must fail closed in all three validators, found %d guarded reads", count)
	}
	if count := strings.Count(script, `if [[ $domain_state != 'shut off' && $domain_state != 'crashed' ]]; then`); count != 3 {
		t.Fatalf("live XML must be skipped only for explicit offline states, found %d guarded decisions", count)
	}

	validator := extractShellFunction(t, script, "read_domain_state")
	binDir := t.TempDir()
	virshPath := filepath.Join(binDir, "virsh")
	if err := os.WriteFile(virshPath, []byte(`#!/bin/sh
case "${VOCAT_TEST_DOMSTATE:-running}" in
  error) exit 42;;
  empty) exit 0;;
  *) printf '%s\r\n' "$VOCAT_TEST_DOMSTATE";;
esac
`), 0o755); err != nil {
		t.Fatal(err)
	}

	for _, testCase := range []struct {
		name        string
		state       string
		want        string
		wantSuccess bool
	}{
		{name: "running", state: "running", want: "running", wantSuccess: true},
		{name: "offline", state: "shut off", want: "shut off", wantSuccess: true},
		{name: "query error", state: "error"},
		{name: "empty response", state: "empty"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			harness := "set -Eeuo pipefail\nreadonly LIBVIRT_URI=qemu:///system\ndomain=vocat\n" + validator + "\nread_domain_state\n"
			command := exec.Command("bash", "-c", harness)
			command.Env = append(os.Environ(), "PATH="+binDir+":"+os.Getenv("PATH"), "VOCAT_TEST_DOMSTATE="+testCase.state)
			output, err := command.CombinedOutput()
			if testCase.wantSuccess && err != nil {
				t.Fatalf("valid domain state was rejected: %v\n%s", err, output)
			}
			if !testCase.wantSuccess && err == nil {
				t.Fatalf("invalid domain state response was accepted: %q", output)
			}
			if testCase.wantSuccess && string(output) != testCase.want {
				t.Fatalf("domain state = %q, want %q", output, testCase.want)
			}
		})
	}
}

func extractUSBHotplugIdentityValidator(t *testing.T) string {
	t.Helper()
	scriptBytes, err := os.ReadFile("../../scripts/vocat-usb-hotplug.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(scriptBytes)
	startMarker := `python3 - "$xml_file" "$HOSTDEV_ALIAS" "$vendor_id" "$product_id" "$scope" "$expected_bus" "$expected_device" <<'PY'` + "\n"
	start := strings.Index(script, startMarker)
	if start < 0 {
		t.Fatal("cannot locate USB identity validator")
	}
	start += len(startMarker)
	end := strings.Index(script[start:], "\nPY\n")
	if end < 0 {
		t.Fatal("USB identity validator heredoc is unterminated")
	}
	validatorPath := filepath.Join(t.TempDir(), "validate_usb_identity.py")
	if err := os.WriteFile(validatorPath, []byte(script[start:start+end]), 0o600); err != nil {
		t.Fatal(err)
	}
	return validatorPath
}

func usbIdentityHostdev(vendor, product, alias string, bus, device int, startupPolicy bool) string {
	startup := ""
	if startupPolicy {
		startup = ` startupPolicy="optional"`
	}
	return fmt.Sprintf(`<hostdev mode="subsystem" type="usb" managed="yes"><source%s><vendor id="0x%s"/><product id="0x%s"/><address bus="%d" device="%d"/></source><alias name="%s"/></hostdev>`, startup, vendor, product, bus, device, alias)
}

func usbIdentityDomain(hostdev string) string {
	return `<domain type="kvm"><devices>` + hostdev + `</devices></domain>`
}
