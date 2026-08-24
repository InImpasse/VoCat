package main

import (
	"os"
	"strings"
	"testing"
)

func TestHardenedBuildScriptEnforcesBinaryMetadataBeforePackaging(t *testing.T) {
	contents, err := os.ReadFile("../../scripts/build-hardened.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(contents)
	required := []string{
		`go version -m "/artifact/$binary_name" > "$go_metadata_output/go-version-m.txt"`,
		`[ "$binary_go_version" = "go1.26.7" ]`,
		`[ "$binary_goos" = "linux" ]`,
		`[ "$binary_goarch" = "$target_arch" ]`,
		`npm_test: passedLog("reports/npm-test.txt", "npm test")`,
		`npm_build: passedLog("reports/npm-build.txt", "npm build")`,
	}
	for _, fragment := range required {
		if !strings.Contains(script, fragment) {
			t.Errorf("hardened build script is missing %q", fragment)
		}
	}

	metadata := strings.Index(script, `go version -m "/artifact/$binary_name"`)
	versionGate := strings.Index(script, `[ "$binary_go_version" = "go1.26.7" ]`)
	packageBinary := strings.Index(script, `install -m 0755 "$go_output/$binary_name" "$staging_dir/$binary_name"`)
	if metadata < 0 || versionGate <= metadata || packageBinary <= versionGate {
		t.Fatal("binary metadata must be generated and enforced before the release binary is packaged")
	}
}

func TestHardenedBuildScriptRejectsEmptySBOMs(t *testing.T) {
	contents, err := os.ReadFile("../../scripts/build-hardened.sh")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), `report.components.length === 0`) {
		t.Fatal("hardened build script must reject empty source and binary SBOMs")
	}
}

func TestHardenedBuildScriptSerializesRacePackages(t *testing.T) {
	contents, err := os.ReadFile("../../scripts/build-hardened.sh")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), `go test -race -p 1 ./...`) {
		t.Fatal("hardened build script must serialize race-tested packages on constrained builders")
	}
}

func TestHardenedBuildScriptUsesFreshCacheForSourceVulnerabilityScan(t *testing.T) {
	contents, err := os.ReadFile("../../scripts/build-hardened.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(contents)
	start := strings.Index(script, `run_split_logged "govulncheck source"`)
	if start < 0 {
		t.Fatal("cannot locate source govulncheck container")
	}
	endOffset := strings.Index(script[start:], `assert_exact_files "$go_source_scan_output"`)
	if endOffset < 0 {
		t.Fatal("cannot locate end of source govulncheck container")
	}
	section := script[start : start+endOffset]
	if !strings.Contains(script, `"$temp_root/go-source-scan-cache"`) ||
		!strings.Contains(section, `src=$temp_root/go-source-scan-cache,dst=/cache/go-build`) {
		t.Fatal("source govulncheck must use a fresh build-scoped cache")
	}
	if strings.Contains(section, `GODEBUG=gocacheverify=1`) {
		t.Fatal("source govulncheck cannot use gocacheverify because go/packages requires its generated cache entries")
	}
}

func TestHardenedBuildScriptProvidesPinnedDeploymentTestTools(t *testing.T) {
	contents, err := os.ReadFile("../../scripts/build-hardened.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(contents)
	for _, required := range []string{
		`readonly GO_TEST_IMAGE="golang:1.26.7-bookworm@sha256:6ef6e30f0ea5c384f6d111cf856e024e3086bbdcb1779da3f3b3fbba0aea53d2"`,
		`readonly JQ_IMAGE="ghcr.io/jqlang/jq:1.8.1@sha256:4f34c6d23f4b1372ac789752cc955dc67c2ae177eb1b5860b75cdc5091ce6f91"`,
		`docker create --platform "linux/$build_arch" "$JQ_IMAGE"`,
		`docker cp "$jq_container_id:/jq" "$jq_extract_path"`,
		`[ ! -f "$jq_extract_path" ] || [ -L "$jq_extract_path" ]`,
		`install -m 0555 "$jq_extract_path" "$jq_tool_dir/jq"`,
		`jq --version > "$tool_output/jq-version.txt"`,
		`go version > "$go_test_version_output/go-test-version.txt"`,
		`go_test: { image: process.env.VOCAT_GO_TEST_IMAGE, version: read("reports/go-test-version.txt") }`,
		`jq: { image: process.env.VOCAT_JQ_IMAGE, version: read("reports/jq-version.txt") }`,
	} {
		if !strings.Contains(script, required) {
			t.Errorf("hardened build script is missing pinned test-tool control %q", required)
		}
	}

	sections := map[string][2]string{
		"normal": {`run_logged "go test"`, `assert_exact_files "$go_test_output"`},
		"race":   {`run_logged "go test -race"`, `assert_exact_files "$race_test_output"`},
	}
	for name, markers := range sections {
		start := strings.Index(script, markers[0])
		if start < 0 {
			t.Fatalf("cannot locate %s test container", name)
		}
		endOffset := strings.Index(script[start:], markers[1])
		if endOffset < 0 {
			t.Fatalf("cannot locate end of %s test container", name)
		}
		section := script[start : start+endOffset]
		for _, required := range []string{
			`--platform "linux/$build_arch"`,
			`--read-only`,
			`--network=none`,
			`src=$build_source_dir,dst=/src,readonly`,
			`src=$CACHE_ROOT/go-mod,dst=/cache/go-mod,readonly`,
			`src=$jq_tool_dir/jq,dst=/usr/local/bin/jq,readonly`,
			`src=$test_identity_dir/passwd,dst=/etc/passwd,readonly`,
			`src=$test_identity_dir/group,dst=/etc/group,readonly`,
			`src=$test_identity_dir/os-release,dst=/etc/os-release,readonly`,
			`"$GO_TEST_IMAGE"`,
		} {
			if !strings.Contains(section, required) {
				t.Errorf("%s test container is missing %q", name, required)
			}
		}
	}
}

func TestContainerToolchainsArePinnedAndPublishingStaysDisabled(t *testing.T) {
	dockerfileBytes, err := os.ReadFile("../../Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	dockerfile := string(dockerfileBytes)
	for _, required := range []string{
		"docker/dockerfile:1.7@sha256:a57df69d0ea827fb7266491f2813635de6f17269be881f696fbfdf2d83dda33e",
		"node:24.15.0-alpine3.23@sha256:d1b3b4da11eefd5941e7f0b9cf17783fc99d9c6fc34884a665f40a06dbdfc94f",
		"golang:1.26.7-alpine3.23@sha256:b17af760035fc2f338eed92d448a6c67f2d45438844fc6c60678fa5f99e44b57",
		"alpine:3.23.3@sha256:25109184c71bdad752c8312a8623239686a9a2071e8825f20acb8f2198c3f659",
	} {
		if !strings.Contains(dockerfile, required) {
			t.Errorf("Dockerfile is missing pinned input %q", required)
		}
	}

	for _, path := range []string{
		"../../.github/workflows/docker.yml",
		"../../.github/workflows/release.yml",
		"../../.github/workflows/sync-apple-carrier-bundles.yml",
	} {
		workflowBytes, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		workflow := string(workflowBytes)
		for _, required := range []string{"workflow_dispatch:", "contents: read", "exit 1"} {
			if !strings.Contains(workflow, required) {
				t.Errorf("%s is missing disabled-workflow guard %q", path, required)
			}
		}
		for _, forbidden := range []string{"contents: write", "packages: write", "push: true", "docker/login-action", "docker/build-push-action", "gh release"} {
			if strings.Contains(workflow, forbidden) {
				t.Errorf("%s contains publishing capability %q", path, forbidden)
			}
		}
	}
}
