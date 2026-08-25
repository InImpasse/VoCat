package main

import (
	"os"
	"strings"
	"testing"
)

func TestKVMHostPreparationRequiresSPICEBackend(t *testing.T) {
	scriptBytes, err := os.ReadFile("../../scripts/prepare-kvm-host.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(scriptBytes)
	for _, required := range []string{
		"qemu-system-modules-spice",
		`qemu-system-x86_64 -spice help`,
	} {
		if !strings.Contains(script, required) {
			t.Errorf("KVM host preparation is missing %q", required)
		}
	}
}
