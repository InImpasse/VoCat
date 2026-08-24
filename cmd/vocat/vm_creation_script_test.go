package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestVMCreationRejectsUnsafeResourceValues(t *testing.T) {
	tests := []struct {
		name     string
		resource []string
		message  string
	}{
		{name: "missing vcpus", resource: []string{"--memory-mib", "2048", "--disk-size-gib", "24"}, message: "--vcpus must be an integer from 2 to 4"},
		{name: "missing memory", resource: []string{"--vcpus", "2", "--disk-size-gib", "24"}, message: "--memory-mib must be an integer from 2048 to 8192 in 256 MiB increments"},
		{name: "missing disk", resource: []string{"--vcpus", "2", "--memory-mib", "2048"}, message: "--disk-size-gib must be an integer from 24 to 64"},
		{name: "non-integer vcpus", resource: []string{"--vcpus", "two", "--memory-mib", "2048", "--disk-size-gib", "24"}, message: "--vcpus must be an integer from 2 to 4"},
		{name: "too few vcpus", resource: []string{"--vcpus", "1", "--memory-mib", "2048", "--disk-size-gib", "24"}, message: "--vcpus must be an integer from 2 to 4"},
		{name: "too many vcpus", resource: []string{"--vcpus", "5", "--memory-mib", "2048", "--disk-size-gib", "24"}, message: "--vcpus must be an integer from 2 to 4"},
		{name: "low memory", resource: []string{"--vcpus", "2", "--memory-mib", "1792", "--disk-size-gib", "24"}, message: "--memory-mib must be an integer from 2048 to 8192 in 256 MiB increments"},
		{name: "non-integer memory", resource: []string{"--vcpus", "2", "--memory-mib", "small", "--disk-size-gib", "24"}, message: "--memory-mib must be an integer from 2048 to 8192 in 256 MiB increments"},
		{name: "unaligned memory", resource: []string{"--vcpus", "2", "--memory-mib", "2300", "--disk-size-gib", "24"}, message: "--memory-mib must be an integer from 2048 to 8192 in 256 MiB increments"},
		{name: "high memory", resource: []string{"--vcpus", "2", "--memory-mib", "8448", "--disk-size-gib", "24"}, message: "--memory-mib must be an integer from 2048 to 8192 in 256 MiB increments"},
		{name: "small disk", resource: []string{"--vcpus", "2", "--memory-mib", "2048", "--disk-size-gib", "23"}, message: "--disk-size-gib must be an integer from 24 to 64"},
		{name: "large disk", resource: []string{"--vcpus", "2", "--memory-mib", "2048", "--disk-size-gib", "65"}, message: "--disk-size-gib must be an integer from 24 to 64"},
		{name: "non-integer disk", resource: []string{"--vcpus", "2", "--memory-mib", "2048", "--disk-size-gib", "small"}, message: "--disk-size-gib must be an integer from 24 to 64"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			args := []string{"../../scripts/create-vocat-vm.sh", "--dry-run", "--lan-interface", "test0", "--bulk-storage-root", "/tmp"}
			args = append(args, test.resource...)
			output, err := exec.Command("bash", args...).CombinedOutput()
			if err == nil {
				t.Fatal("unsafe resource profile was accepted")
			}
			if !strings.Contains(string(output), test.message) {
				t.Fatalf("unexpected validation error: %s", output)
			}
		})
	}
}

func TestVMCreationBindsConfiguredResourcesToCreationAndChecks(t *testing.T) {
	scriptBytes, err := os.ReadFile("../../scripts/create-vocat-vm.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(scriptBytes)
	for _, required := range []string{
		`"${disk_size_gib}G"`,
		`--vcpus "$vcpus"`,
		`--memory "$memory_mib"`,
		`expected_vcpus = int(expected_vcpus)`,
		`expected_memory_mib = int(expected_memory_mib)`,
		`expected_size = int(sys.argv[1]) * 1024**3`,
		`min_free_gib=$((disk_size_gib * 2))`,
		`((min_free_gib >= 48)) || min_free_gib=48`,
		`--memory-mib "$memory_mib" --disk-size-gib "$disk_size_gib"`,
	} {
		if !strings.Contains(script, required) {
			t.Errorf("VM resource profile is not bound to creation and validation: missing %q", required)
		}
	}
}

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
