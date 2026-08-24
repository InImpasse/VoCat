package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDJIRemoveRuleQueuesDetachWithoutDeviceActivation(t *testing.T) {
	rules := readUSBPassthroughAsset(t, "../../deploy/vocat-dji-usb.rules.in")
	var removeRule string
	for _, line := range strings.Split(rules, "\n") {
		if strings.Contains(line, `ACTION=="remove"`) {
			removeRule = line
			break
		}
	}
	if removeRule == "" {
		t.Fatal("DJI USB rules do not contain a remove handler")
	}
	if !strings.Contains(removeRule, `RUN{program}+="/usr/bin/systemctl --no-block start vocat-usb-detach@%k.service"`) {
		t.Fatalf("remove handler does not queue the detach unit with a short systemctl request: %s", removeRule)
	}
	if strings.Contains(removeRule, "SYSTEMD_WANTS") {
		t.Fatal("remove handler uses SYSTEMD_WANTS, which is only acted on while a device becomes active")
	}
}

func TestDJIHotplugUnitsPreserveKernelUSBName(t *testing.T) {
	for _, path := range []string{
		"../../deploy/vocat-usb-attach@.service",
		"../../deploy/vocat-usb-detach@.service",
	} {
		unit := readUSBPassthroughAsset(t, path)
		if !strings.Contains(unit, "%i") {
			t.Errorf("%s does not pass the escaped instance literally", path)
		}
		if strings.Contains(unit, "%I") {
			t.Errorf("%s unescapes a USB name such as 1-2 into the invalid path 1/2", path)
		}
	}
}

func TestDJIHotplugUsesPersistentRecoverableTransaction(t *testing.T) {
	script := readUSBPassthroughAsset(t, "../../scripts/vocat-usb-hotplug.sh")
	for _, required := range []string{
		`readonly CURRENT_STATE=$STATE_DIR/current`,
		`readonly PENDING_STATE=$STATE_DIR/pending`,
		`attach-device "$domain" "$new_xml" --persistent`,
		`detach-device "$domain" "$xml_file" --persistent`,
		`validate_state_directory "$PENDING_STATE"`,
		`mv -T -- "$PENDING_STATE" "$CURRENT_STATE"`,
		`the root-only pending marker was retained`,
	} {
		if !strings.Contains(script, required) {
			t.Errorf("hotplug transaction is missing %q", required)
		}
	}
	for _, forbidden := range []string{
		`attach-device "$domain" "$new_xml" --config`,
		`attach-device "$domain" "$new_xml" --live`,
		`readonly CURRENT_XML=$STATE_DIR/current.xml`,
	} {
		if strings.Contains(script, forbidden) {
			t.Errorf("hotplug transaction retains unsafe split-state operation %q", forbidden)
		}
	}

	stage := strings.Index(script, `install -d -o root -g root -m 0700 "$PENDING_STATE"`)
	attach := strings.Index(script, `attach-device "$domain" "$new_xml" --persistent`)
	commit := strings.Index(script, `mv -T -- "$PENDING_STATE" "$CURRENT_STATE"`)
	if stage < 0 || attach < 0 || commit < 0 || !(stage < attach && attach < commit) {
		t.Fatalf("recoverable state must be staged before attach and atomically committed afterwards: stage=%d attach=%d commit=%d", stage, attach, commit)
	}
}

func TestDJIHotplugValidatesInactiveAndLiveHostdevs(t *testing.T) {
	hotplug := readUSBPassthroughAsset(t, "../../scripts/vocat-usb-hotplug.sh")
	for _, required := range []string{
		`dump_domain_xml config "$config_dump"`,
		`assert_managed_passthrough "$config_dump" config no`,
		`dump_domain_xml live "$live_dump"`,
		`assert_managed_passthrough "$live_dump" live no`,
		`hostdev.get("mode") != "subsystem"`,
		`hostdev.get("managed") != "yes"`,
		`startup_policy == "optional"`,
	} {
		if !strings.Contains(hotplug, required) {
			t.Errorf("hotplug validator is missing %q", required)
		}
	}

	configure := readUSBPassthroughAsset(t, "../../scripts/configure-dji-usb-passthrough.sh")
	for _, required := range []string{
		`dumpxml --inactive "$domain"`,
		`dumpxml "$domain"`,
		`validate(inactive_path, "inactive")`,
		`validate(live_path, "live")`,
		`hostdev.get("mode") != "subsystem"`,
		`hostdev.get("managed") != "yes"`,
	} {
		if !strings.Contains(configure, required) {
			t.Errorf("configuration preflight is missing %q", required)
		}
	}
}

func TestDJIHostdevValidatorRejectsUnmanagedAndExtraDevices(t *testing.T) {
	script := readUSBPassthroughAsset(t, "../../scripts/vocat-usb-hotplug.sh")
	startMarker := `python3 - "$xml_file" "$HOSTDEV_ALIAS" "$vendor_id" "$product_id" "$scope" "$require_one" <<'PY'` + "\n"
	start := strings.Index(script, startMarker)
	if start < 0 {
		t.Fatal("cannot locate the hostdev validator in the hotplug script")
	}
	start += len(startMarker)
	end := strings.Index(script[start:], "\nPY\n")
	if end < 0 {
		t.Fatal("hostdev validator heredoc is unterminated")
	}
	validatorPath := filepath.Join(t.TempDir(), "validate.py")
	if err := os.WriteFile(validatorPath, []byte(script[start:start+end]), 0o600); err != nil {
		t.Fatal(err)
	}

	valid := `<domain type="kvm"><devices><hostdev mode="subsystem" type="usb" managed="yes"><source startupPolicy="optional"><vendor id="0x2ca3"/><product id="0x4006"/><address bus="1" device="2"/></source><alias name="ua-vocat-dji-usb"/></hostdev></devices></domain>`
	unmanaged := strings.Replace(valid, `managed="yes"`, `managed="no"`, 1)
	extraPCI := strings.Replace(valid, `</devices>`, `<hostdev mode="subsystem" type="pci" managed="yes"><source><address domain="0" bus="0" slot="20" function="0"/></source></hostdev></devices>`, 1)
	liveWithoutStartupPolicy := strings.Replace(valid, ` startupPolicy="optional"`, "", 1)

	assertValidatorResult(t, validatorPath, valid, "config", true)
	assertValidatorResult(t, validatorPath, unmanaged, "config", false)
	assertValidatorResult(t, validatorPath, extraPCI, "config", false)
	assertValidatorResult(t, validatorPath, liveWithoutStartupPolicy, "live", true)
}

func assertValidatorResult(t *testing.T, validatorPath, xml, scope string, wantSuccess bool) {
	t.Helper()
	xmlPath := filepath.Join(t.TempDir(), "domain.xml")
	if err := os.WriteFile(xmlPath, []byte(xml), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("python3", validatorPath, xmlPath, "ua-vocat-dji-usb", "2ca3", "4006", scope, "no")
	err := command.Run()
	if wantSuccess && err != nil {
		t.Fatalf("validator rejected trusted %s XML: %v", scope, err)
	}
	if !wantSuccess && err == nil {
		t.Fatalf("validator accepted untrusted %s XML", scope)
	}
}

func readUSBPassthroughAsset(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
