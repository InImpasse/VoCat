package main

import (
	"os"
	"strings"
	"testing"
)

func TestInstallerUsesOnlyVerifiedLocalArtifacts(t *testing.T) {
	scriptBytes, err := os.ReadFile("../../scripts/install.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(scriptBytes)
	for _, forbidden := range []string{
		"releases/latest",
		"GITHUB_TOKEN",
		"MengMengCode/VoCat",
		"ghcr.io/mengmengcode",
		"curl -fsSL https://",
	} {
		if strings.Contains(script, forbidden) {
			t.Errorf("installer contains forbidden remote update reference %q", forbidden)
		}
	}
	for _, required := range []string{
		"--artifact",
		"--expected-commit",
		"--expected-index-sha256",
		"deploy-hardened.sh",
		`exec "$deploy_script" --expected-commit "$4" --expected-index-sha256 "$6" "$2"`,
		"remote installation and self-update are disabled",
	} {
		if !strings.Contains(script, required) {
			t.Errorf("installer is missing hardened artifact handling %q", required)
		}
	}
}

func TestInstallerProvidesRequiredQMIUtilities(t *testing.T) {
	scriptBytes, err := os.ReadFile("../../scripts/install.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(scriptBytes)
	for _, required := range []string{
		"curl ip jq qmicli sha256sum sqlite3 ss systemctl",
		`command -v "$command"`,
		"command -v qmi-proxy",
		"/usr/libexec/qmi-proxy",
		"/usr/lib/qmi-proxy",
		"ip xfrm state list",
		"environment preflight failed",
	} {
		if !strings.Contains(script, required) {
			t.Errorf("installer is missing required QMI handling %q", required)
		}
	}
}
