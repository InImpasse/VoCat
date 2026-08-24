package main

import (
	"os"
	"strings"
	"testing"
)

func TestVMCreationUsesVerifiedSSDISOSnapshot(t *testing.T) {
	scriptBytes, err := os.ReadFile("../../scripts/create-vocat-vm.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(scriptBytes)

	ordered := []string{
		`actual_iso_sha256=$(sha256sum -- "$iso_path"`,
		`verified_iso_snapshot="$disk_dir/$vm_name-installer-$iso_sha256.iso"`,
		`iso_snapshot_staging=$(mktemp "$disk_dir/.${vm_name}-installer.XXXXXX.iso")`,
		`install -o root -g kvm -m 0640 "$iso_path" "$iso_snapshot_staging"`,
		`snapshot_metadata=$(stat -c '%F:%U:%G:%a:%h' -- "$iso_snapshot_staging")`,
		`[[ $snapshot_metadata == 'regular file:root:kvm:640:1' ]]`,
		`snapshot_sha256=$(sha256sum -- "$iso_snapshot_staging"`,
		`[[ $snapshot_sha256 == "$iso_sha256" ]]`,
		`mv -T -- "$iso_snapshot_staging" "$verified_iso_snapshot"`,
		`iso_path=$verified_iso_snapshot`,
		`runuser -u libvirt-qemu -- test -r "$iso_path"`,
		`--cdrom "$iso_path"`,
	}
	previous := -1
	for _, fragment := range ordered {
		position := strings.Index(script, fragment)
		if position < 0 {
			t.Fatalf("VM creation is missing ISO snapshot safeguard %q", fragment)
		}
		if position <= previous {
			t.Fatalf("VM creation performs ISO snapshot step %q out of order", fragment)
		}
		previous = position
	}

}

func TestVMCreationCleansFailedISOSnapshots(t *testing.T) {
	scriptBytes, err := os.ReadFile("../../scripts/create-vocat-vm.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(scriptBytes)

	for _, required := range []string{
		`trap cleanup_failed_create EXIT`,
		`[[ -z $iso_snapshot_staging || ! -f $iso_snapshot_staging ]] || rm -f -- "$iso_snapshot_staging"`,
		`status != 0 && created_iso_snapshot == 1 && can_remove_disk == 1`,
		`rm -f -- "$verified_iso_snapshot"`,
		`log "Verified installer snapshot retained until ISO ejection: $verified_iso_snapshot"`,
	} {
		if !strings.Contains(script, required) {
			t.Errorf("VM creation failure handling is missing %q", required)
		}
	}
}
