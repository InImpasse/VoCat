package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGuestPreparationInstallsDeploymentContract(t *testing.T) {
	scriptBytes, err := os.ReadFile("../../scripts/prepare-vocat-guest.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(scriptBytes)
	for _, required := range []string{
		"jq libqmi-utils nftables python3 qemu-guest-agent sqlite3",
		"iproute2",
		"qmicli sqlite3 ss stat systemctl",
		"groupadd --system vocat-modem",
		"groupadd --system vocat",
		"groupadd --system vocat-preflight",
		"useradd --system --gid vocat --groups vocat-modem",
		"useradd --system --gid vocat-preflight --home-dir /nonexistent",
		`$VOCAT_UNIT_SOURCE" /etc/systemd/system/vocat.service`,
		`$DJI_REPAIR_UNIT_SOURCE" /etc/systemd/system/vocat-dji-repair.service`,
		"systemctl restart vocat-firewall.service",
	} {
		if !strings.Contains(script, required) {
			t.Errorf("guest preparation is missing %q", required)
		}
	}
}

func TestDJIRepairUnitRemainsManual(t *testing.T) {
	unitBytes, err := os.ReadFile("../../deploy/vocat-dji-repair.service")
	if err != nil {
		t.Fatal(err)
	}
	unit := string(unitBytes)
	if strings.Contains(unit, "[Install]") {
		t.Fatal("DJI repair unit must not be enableable automatically")
	}
	if !strings.Contains(unit, "doctor --repair-dji-qmi") {
		t.Fatal("DJI repair unit is missing the exact repair command")
	}
}

func TestProductionUnitRejectsLegacyEnvironmentDrift(t *testing.T) {
	unitBytes, err := os.ReadFile("../../deploy/vocat.service")
	if err != nil {
		t.Fatal(err)
	}
	unit := string(unitBytes)
	for _, required := range []string{
		"After=network-online.target vocat-firewall.service",
		"Requires=vocat-firewall.service",
		"Type=exec",
		"Environment=VOCAT_DATABASE_PATH=/var/lib/vocat/vocat.db",
		"Environment=VOCAT_ADDR=0.0.0.0:7575",
		"UnsetEnvironment=GITHUB_TOKEN VOCAT_REPO",
		"ProtectSystem=strict",
		"ReadWritePaths=/var/lib/vocat /run/vocat",
	} {
		if !strings.Contains(unit, required) {
			t.Errorf("production unit is missing %q", required)
		}
	}
	for _, forbidden := range []string{"EnvironmentFile=", "\nEnvironment=GITHUB_TOKEN", "\nEnvironment=VOCAT_REPO"} {
		if strings.Contains(unit, forbidden) {
			t.Errorf("production unit accepts forbidden environment input %q", forbidden)
		}
	}
}

func TestDevelopmentComposeCannotPullOrRunPrivileged(t *testing.T) {
	composeBytes, err := os.ReadFile("../../docker-compose.yml")
	if err != nil {
		t.Fatal(err)
	}
	compose := string(composeBytes)
	for _, required := range []string{
		"image: vocat-hardened:local",
		"pull_policy: never",
		"cap_drop:\n      - ALL",
		"cap_add:\n      - NET_ADMIN\n      - NET_RAW",
		"no-new-privileges:true",
		"read_only: true",
		"/sys:/sys:ro",
	} {
		if !strings.Contains(compose, required) {
			t.Errorf("development Compose is missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"privileged: true",
		"/dev:/dev",
		"GITHUB_TOKEN",
		"VOCAT_REPO",
		":latest",
	} {
		if strings.Contains(compose, forbidden) {
			t.Errorf("development Compose contains forbidden setting %q", forbidden)
		}
	}
}

func TestFirewallAndUSBUnitsFailClosed(t *testing.T) {
	firewallBytes, err := os.ReadFile("../../deploy/vocat-firewall.service")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(firewallBytes), "ConditionPathExists=") {
		t.Fatal("firewall configuration must fail the required unit instead of skipping it")
	}
	for _, name := range []string{"vocat-usb-attach@.service", "vocat-usb-detach@.service"} {
		unitBytes, err := os.ReadFile("../../deploy/" + name)
		if err != nil {
			t.Fatal(err)
		}
		unit := string(unitBytes)
		if !strings.Contains(unit, " %i") || strings.Contains(unit, " %I") {
			t.Errorf("%s does not preserve the escaped USB sysname instance", name)
		}
	}
}

func TestGuestFirewallUsesReversePathAndLivePolicyChecks(t *testing.T) {
	rulesBytes, err := os.ReadFile("../../deploy/vocat-firewall.nft.in")
	if err != nil {
		t.Fatal(err)
	}
	rules := string(rulesBytes)
	for _, required := range []string{
		`fib saddr . iif oif exists`,
		`vocat-policy-v2-loopback`,
		`vocat-policy-v2-tailscale`,
		`vocat-policy-v2-lan-rpf`,
		`vocat-policy-v2-default-reject`,
	} {
		if !strings.Contains(rules, required) {
			t.Errorf("firewall is missing %q", required)
		}
	}

	scriptBytes, err := os.ReadFile("../../scripts/prepare-vocat-guest.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(scriptBytes)
	for _, required := range []string{
		"validate_live_firewall",
		"live VoCat firewall semantics differ from the reviewed policy",
		"assert_unit_profile vocat.service",
		"unit drop-ins are forbidden",
		"systemctl restart vocat-firewall.service",
	} {
		if !strings.Contains(script, required) {
			t.Errorf("guest firewall verification is missing %q", required)
		}
	}
}

func TestGuestFirewallJSONPolicyRejectsSemanticDrift(t *testing.T) {
	scriptBytes, err := os.ReadFile("../../scripts/prepare-vocat-guest.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(scriptBytes)
	startMarker := "  jq -e --arg lan_network \"$lan_network\" --argjson lan_prefix \"$lan_prefix\" '\n"
	start := strings.Index(script, startMarker)
	if start < 0 {
		t.Fatal("cannot locate live firewall jq validator")
	}
	start += len(startMarker)
	endMarker := "\n  ' <<<\"$live_json\""
	end := strings.Index(script[start:], endMarker)
	if end < 0 {
		t.Fatal("live firewall jq validator is unterminated")
	}
	filter := script[start : start+end]
	trusted := trustedFirewallJSON()
	assertFirewallPolicyResult(t, filter, trusted, true)

	mutations := map[string]string{
		"base chain policy": strings.Replace(trusted, `"policy":"accept"`, `"policy":"drop"`, 1),
		"base chain hook":   strings.Replace(trusted, `"hook":"input"`, `"hook":"forward"`, 1),
		"listen port":       strings.Replace(trusted, `"right":7575`, `"right":7576`, 1),
		"trusted subnet":    strings.Replace(trusted, `"addr":"192.0.2.0"`, `"addr":"198.51.100.0"`, 1),
		"reverse path":      strings.Replace(trusted, `"right":true`, `"right":false`, 1),
		"default action":    strings.Replace(trusted, `"type":"tcp reset"`, `"type":"icmpx"`, 1),
		"trusted comment":   strings.Replace(trusted, `"vocat-policy-v2-loopback"`, `"vocat-policy-v2-tailscale"`, 1),
	}
	for name, candidate := range mutations {
		t.Run(name, func(t *testing.T) {
			if candidate == trusted {
				t.Fatal("test mutation did not alter its fixture")
			}
			assertFirewallPolicyResult(t, filter, candidate, false)
		})
	}
}

func TestGuestPreparationChecksRepositoryUnitsAndRuntimeState(t *testing.T) {
	scriptBytes, err := os.ReadFile("../../scripts/prepare-vocat-guest.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(scriptBytes)
	for _, required := range []string{
		`assert_root_file_metadata "$TAILSCALE_KEYRING" 644`,
		`assert_root_file_metadata "$TAILSCALE_LIST" 644`,
		`public_keys ":" primary`,
		`"1:$TAILSCALE_KEY_FINGERPRINT"`,
		`cmp -s -- <(printf '%s\n%s\n' "$TAILSCALE_LIST_COMMENT" "$TAILSCALE_LIST_REPOSITORY")`,
		`modem_state == inactive`,
		`nftables.service qemu-guest-agent.service tailscaled.service vocat-firewall.service`,
		`repair_state == inactive`,
	} {
		if !strings.Contains(script, required) {
			t.Errorf("guest final validation is missing %q", required)
		}
	}

	restart := strings.LastIndex(script, "run systemctl restart vocat-firewall.service")
	if restart < 0 {
		t.Fatal("apply does not restart the reviewed firewall unit")
	}
	profile := strings.LastIndex(script[:restart], "assert_unit_profile vocat-firewall.service")
	finalCheck := strings.LastIndex(script, "validate_guest_state")
	if profile < 0 || finalCheck < restart {
		t.Fatal("apply does not reject unit drop-ins before activation and repeat all checks afterward")
	}
}

func TestGuestFirewallFileTransactionIsRecoverable(t *testing.T) {
	scriptBytes, err := os.ReadFile("../../scripts/prepare-vocat-guest.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(scriptBytes)
	for _, required := range []string{
		"rollback_firewall_transaction",
		"nftables.conf.previous",
		"vocat-firewall.nft.previous",
		`firewall_transaction_active=true`,
		`install -o root -g root -m 0644 "$tmp_dir/nftables.conf.previous" "$NFTABLES_MAIN"`,
		`nft --check --file "$tmp_dir/nftables.conf.candidate"`,
		`nft --check --file "$NFTABLES_MAIN"`,
	} {
		if !strings.Contains(script, required) {
			t.Errorf("recoverable firewall transaction is missing %q", required)
		}
	}
	validated := strings.LastIndex(script, `run nft --check --file "$tmp_dir/nftables.conf.candidate"`)
	transaction := strings.LastIndex(script, "firewall_transaction_active=true")
	firstMove := strings.LastIndex(script, `run mv -T -- "$firewall_staging"`)
	if validated < 0 || transaction < validated || firstMove < transaction {
		t.Fatal("firewall files can be replaced before the staged complete ruleset is validated and rollback is armed")
	}
}

func TestGuestDryRunDescribesApplySecurityChecks(t *testing.T) {
	command := exec.Command("bash", "../../scripts/prepare-vocat-guest.sh", "--dry-run",
		"--lan-cidr", "192.0.2.0/24")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("guest dry-run failed: %v\n%s", err, output)
	}
	for _, required := range []string{
		"primary fingerprint",
		"99-vocat-dji.rules",
		"stage both firewall files",
		"reject any unit drop-in",
		"same file, repository trust, account, unit, service, and live nft JSON semantic checks as --check",
	} {
		if !strings.Contains(string(output), required) {
			t.Errorf("dry-run omitted %q\n%s", required, output)
		}
	}
}

func assertFirewallPolicyResult(t *testing.T, filter, input string, wantSuccess bool) {
	t.Helper()
	command := exec.Command("jq", "-e", "--arg", "lan_network", "192.0.2.0", "--argjson", "lan_prefix", "24", filter)
	command.Stdin = strings.NewReader(input)
	output, err := command.CombinedOutput()
	if wantSuccess && err != nil {
		t.Fatalf("firewall policy rejected trusted nft JSON: %v\n%s", err, output)
	}
	if !wantSuccess && err == nil {
		t.Fatal("firewall policy accepted semantically altered nft JSON")
	}
}

func trustedFirewallJSON() string {
	return `{"nftables":[
{"metainfo":{"json_schema_version":1}},
{"chain":{"family":"inet","table":"vocat_ingress","name":"input","handle":1,"type":"filter","hook":"input","prio":-5,"policy":"accept"}},
{"rule":{"family":"inet","table":"vocat_ingress","chain":"input","handle":2,"expr":[{"match":{"op":"==","left":{"meta":{"key":"iifname"}},"right":"lo"}},{"match":{"op":"==","left":{"payload":{"protocol":"tcp","field":"dport"}},"right":7575}},{"accept":null}],"comment":"vocat-policy-v2-loopback"}},
{"rule":{"family":"inet","table":"vocat_ingress","chain":"input","handle":3,"expr":[{"match":{"op":"==","left":{"meta":{"key":"iifname"}},"right":"tailscale0"}},{"match":{"op":"==","left":{"payload":{"protocol":"tcp","field":"dport"}},"right":7575}},{"accept":null}],"comment":"vocat-policy-v2-tailscale"}},
{"rule":{"family":"inet","table":"vocat_ingress","chain":"input","handle":4,"expr":[{"match":{"op":"==","left":{"payload":{"protocol":"ip","field":"saddr"}},"right":{"prefix":{"addr":"192.0.2.0","len":24}}}},{"match":{"op":"==","left":{"fib":{"result":"oif","flags":["saddr","iif"]}},"right":true}},{"match":{"op":"==","left":{"payload":{"protocol":"tcp","field":"dport"}},"right":7575}},{"accept":null}],"comment":"vocat-policy-v2-lan-rpf"}},
{"rule":{"family":"inet","table":"vocat_ingress","chain":"input","handle":5,"expr":[{"match":{"op":"==","left":{"payload":{"protocol":"tcp","field":"dport"}},"right":7575}},{"reject":{"type":"tcp reset"}}],"comment":"vocat-policy-v2-default-reject"}}
]}`
}

func TestDJIFirmwareReaderPinsUSBIdentityAndResponseShape(t *testing.T) {
	scriptBytes, err := os.ReadFile("../../scripts/read-dji-firmware.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(scriptBytes)
	for _, required := range []string{
		`/sys/class/tty/$tty_name/device`,
		`vendor_id,,} == 2ca3`,
		`product_id,,} == 4006`,
		`interface_number,,} == 02`,
		`if len(lines) != 1 or contains_unsolicited(lines, expected_prefix)`,
	} {
		if !strings.Contains(script, required) {
			t.Errorf("DJI firmware reader is missing %q", required)
		}
	}
}

func TestUSBPassthroughRequiresLibvirtManagedDevice(t *testing.T) {
	for _, name := range []string{"configure-dji-usb-passthrough.sh", "vocat-usb-hotplug.sh"} {
		scriptBytes, err := os.ReadFile("../../scripts/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(scriptBytes), `hostdev.get("managed") != "yes"`) {
			t.Errorf("%s accepts an unmanaged hostdev", name)
		}
	}
}

func TestVMProfileRejectsStorageAndDeviceDrift(t *testing.T) {
	scriptBytes, err := os.ReadFile("../../scripts/create-vocat-vm.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(scriptBytes)
	for _, required := range []string{
		`[[ -f $disk_path && ! -L $disk_path ]]`,
		`virsh --connect qemu:///system dumpxml --inactive "$vm_name"`,
		`virsh --connect qemu:///system dumpxml "$vm_name"`,
		"VM disk directory must be root:kvm with mode 0750",
		"VM qcow2 must be libvirt-qemu:kvm with mode 0640",
		`len(disks) == 1`,
		`len(graphics_devices) == 1`,
		`not root.findall("./devices/filesystem")`,
		`inactive_hostdevs == live_hostdevs`,
		`CD-ROM media must be ejected unless --iso is supplied`,
		`"backing-filename", "full-backing-filename", "data-file"`,
		`--lan-interface "$lan_interface" --iso "$iso_path"`,
	} {
		if !strings.Contains(script, required) {
			t.Errorf("VM fixed-profile check is missing %q", required)
		}
	}
	freeSpaceCheck := strings.Index(script, "free_bytes=$(df")
	checkModeBlock := strings.Index(script, "if [[ $mode != check ]]; then")
	if checkModeBlock < 0 || freeSpaceCheck < checkModeBlock {
		t.Fatal("derived free-space gate is not restricted to VM creation")
	}
}

func TestVMProfilePythonValidatorRejectsLiveAndCDROMDrift(t *testing.T) {
	scriptBytes, err := os.ReadFile("../../scripts/create-vocat-vm.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(scriptBytes)
	startMarker := `  python3 - "$inactive_domain_xml" "$live_domain_xml" "$disk_path" "$lan_interface" "$OVMF_CODE" "$OVMF_VARS" "$iso_path" "$vcpus" "$memory_mib" <<'PY'` + "\n"
	start := strings.Index(script, startMarker)
	if start < 0 {
		t.Fatal("cannot locate the VM profile Python validator")
	}
	start += len(startMarker)
	end := strings.Index(script[start:], "\nPY\n")
	if end < 0 {
		t.Fatal("VM profile Python validator heredoc is unterminated")
	}

	tempDir := t.TempDir()
	validatorPath := filepath.Join(tempDir, "validate_vm.py")
	if err := os.WriteFile(validatorPath, []byte(script[start:start+end]), 0o600); err != nil {
		t.Fatal(err)
	}
	diskPath := filepath.Join(tempDir, "vocat.qcow2")
	isoPath := filepath.Join(tempDir, "ubuntu.iso")
	arbitraryPath := filepath.Join(tempDir, "arbitrary.img")
	for _, path := range []string{diskPath, isoPath, arbitraryPath} {
		if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	inactive := vmProfileXML(diskPath, isoPath, true)
	live := vmProfileXML(diskPath, isoPath, false)
	ejectedInactive := vmProfileXML(diskPath, "", true)
	ejectedLive := vmProfileXML(diskPath, "", false)
	blockCDROM := `<disk type="block" device="cdrom"><driver name="qemu" type="raw"/><source dev="/dev/sr0"/><target dev="sda" bus="sata"/><readonly/></disk>`
	arbitraryFileCDROM := fmt.Sprintf(`<disk type="file" device="cdrom"><driver name="qemu" type="raw"/><source file="%s"/><target dev="sda" bus="sata"/><readonly/></disk>`, arbitraryPath)
	liveWithQEMUArgs := strings.Replace(live, `<domain type="kvm">`, `<domain type="kvm" xmlns:qemu="http://libvirt.org/schemas/domain/qemu/1.0">`, 1)
	liveWithQEMUArgs = strings.Replace(liveWithQEMUArgs, "</domain>", `<qemu:commandline><qemu:arg value="-device"/></qemu:commandline></domain>`, 1)

	tests := []struct {
		name        string
		inactiveXML string
		liveXML     string
		expectedISO string
		wantSuccess bool
	}{
		{name: "valid installed profile", inactiveXML: inactive, liveXML: live, expectedISO: isoPath, wantSuccess: true},
		{name: "valid libvirt install transition", inactiveXML: ejectedInactive, liveXML: live, expectedISO: isoPath, wantSuccess: true},
		{name: "valid ejected profile", inactiveXML: ejectedInactive, liveXML: ejectedLive, wantSuccess: true},
		{name: "live installer media ejected", inactiveXML: ejectedInactive, liveXML: ejectedLive, expectedISO: isoPath},
		{name: "inactive vCPU drift", inactiveXML: strings.Replace(inactive, "<vcpu>2</vcpu>", "<vcpu>3</vcpu>", 1), liveXML: live, expectedISO: isoPath},
		{name: "live memory drift", inactiveXML: inactive, liveXML: strings.Replace(live, "<memory unit=\"KiB\">2097152</memory>", "<memory unit=\"KiB\">3145728</memory>", 1), expectedISO: isoPath},
		{name: "live-only PCI hostdev", inactiveXML: inactive, liveXML: addVMDevice(live, `<hostdev mode="subsystem" type="pci" managed="yes"><source><address domain="0" bus="0" slot="20" function="0"/></source></hostdev>`), expectedISO: isoPath},
		{name: "live-only filesystem", inactiveXML: inactive, liveXML: addVMDevice(live, `<filesystem type="mount"><source dir="/"/><target dir="host"/></filesystem>`), expectedISO: isoPath},
		{name: "live-only redirection", inactiveXML: inactive, liveXML: addVMDevice(live, `<redirdev bus="usb" type="spicevmc"/>`), expectedISO: isoPath},
		{name: "live-only smartcard", inactiveXML: inactive, liveXML: addVMDevice(live, `<smartcard mode="passthrough" type="spicevmc"/>`), expectedISO: isoPath},
		{name: "live-only custom channel", inactiveXML: inactive, liveXML: addVMDevice(live, `<channel type="unix"><target type="virtio" name="custom.channel"/></channel>`), expectedISO: isoPath},
		{name: "live-only custom qemu command line", inactiveXML: inactive, liveXML: liveWithQEMUArgs, expectedISO: isoPath},
		{name: "block CD-ROM", inactiveXML: replaceVMCDROM(inactive, blockCDROM), liveXML: replaceVMCDROM(live, blockCDROM), expectedISO: isoPath},
		{name: "arbitrary file CD-ROM", inactiveXML: replaceVMCDROM(inactive, arbitraryFileCDROM), liveXML: replaceVMCDROM(live, arbitraryFileCDROM), expectedISO: isoPath},
		{name: "attached file without --iso", inactiveXML: inactive, liveXML: live},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertVMProfileValidator(t, validatorPath, diskPath, test.inactiveXML, test.liveXML, test.expectedISO, test.wantSuccess)
		})
	}

	symlinkISO := filepath.Join(tempDir, "linked.iso")
	if err := os.Symlink(isoPath, symlinkISO); err != nil {
		t.Fatal(err)
	}
	assertVMProfileValidator(t, validatorPath, diskPath, vmProfileXML(diskPath, symlinkISO, true),
		vmProfileXML(diskPath, symlinkISO, false), symlinkISO, false)
}

func assertVMProfileValidator(t *testing.T, validatorPath, diskPath, inactiveXML, liveXML, expectedISO string, wantSuccess bool) {
	t.Helper()
	tempDir := t.TempDir()
	inactivePath := filepath.Join(tempDir, "inactive.xml")
	livePath := filepath.Join(tempDir, "live.xml")
	if err := os.WriteFile(inactivePath, []byte(inactiveXML), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(livePath, []byte(liveXML), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("python3", validatorPath, inactivePath, livePath, diskPath, "testlan0",
		"/usr/share/OVMF/OVMF_CODE_4M.secboot.fd", "/usr/share/OVMF/OVMF_VARS_4M.ms.fd", expectedISO,
		"2", "2048")
	output, err := command.CombinedOutput()
	if wantSuccess && err != nil {
		t.Fatalf("validator rejected trusted VM XML: %v\n%s", err, output)
	}
	if !wantSuccess && err == nil {
		t.Fatal("validator accepted untrusted VM XML")
	}
}

func vmProfileXML(diskPath, isoPath string, inactive bool) string {
	startupPolicy := ""
	if inactive {
		startupPolicy = ` startupPolicy="optional"`
	}
	cdrom := `<disk type="file" device="cdrom"><driver name="qemu" type="raw"/><target dev="sda" bus="sata"/><readonly/></disk>`
	if isoPath != "" {
		cdrom = fmt.Sprintf(`<disk type="file" device="cdrom"><driver name="qemu" type="raw"/><source file="%s"/><target dev="sda" bus="sata"/><readonly/></disk>`, isoPath)
	}
	return fmt.Sprintf(`<domain type="kvm">
  <memory unit="KiB">2097152</memory>
  <vcpu>2</vcpu>
  <cpu mode="host-passthrough"/>
  <os>
    <type machine="pc-q35-9.2">hvm</type>
    <loader readonly="yes" secure="yes" type="pflash">/usr/share/OVMF/OVMF_CODE_4M.secboot.fd</loader>
    <nvram template="/usr/share/OVMF/OVMF_VARS_4M.ms.fd">/var/lib/libvirt/qemu/nvram/vocat_VARS.fd</nvram>
  </os>
  <features><smm state="on"/></features>
  <devices>
    <disk type="file" device="disk"><driver name="qemu" type="qcow2" cache="none" discard="unmap" detect_zeroes="unmap"/><source file="%s"/><target dev="sda" bus="scsi"/></disk>
    %s
    <controller type="scsi" model="virtio-scsi"/>
    <interface type="network"><source network="default"/><model type="virtio"/></interface>
    <interface type="direct"><source dev="testlan0" mode="bridge"/><model type="virtio"/></interface>
    <tpm model="tpm-crb"><backend type="emulator" version="2.0"/></tpm>
    <channel type="unix"><target type="virtio" name="org.qemu.guest_agent.0"/></channel>
    <graphics type="spice" listen="127.0.0.1"/>
    <hostdev mode="subsystem" type="usb" managed="yes"><source%s><vendor id="0x2ca3"/><product id="0x4006"/><address bus="1" device="2"/></source><alias name="ua-vocat-dji-usb"/></hostdev>
  </devices>
</domain>`, diskPath, cdrom, startupPolicy)
}

func addVMDevice(domainXML, deviceXML string) string {
	return strings.Replace(domainXML, "</devices>", deviceXML+"</devices>", 1)
}

func replaceVMCDROM(domainXML, replacement string) string {
	start := strings.Index(domainXML, `<disk type="file" device="cdrom">`)
	if start < 0 {
		return domainXML
	}
	end := strings.Index(domainXML[start:], "</disk>")
	if end < 0 {
		return domainXML
	}
	end += start + len("</disk>")
	return domainXML[:start] + replacement + domainXML[end:]
}
