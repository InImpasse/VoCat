#!/usr/bin/env bash
set -euo pipefail
umask 077

readonly NODE_IMAGE="node:24.15.0-alpine3.23@sha256:d1b3b4da11eefd5941e7f0b9cf17783fc99d9c6fc34884a665f40a06dbdfc94f"
readonly GO_IMAGE="golang:1.26.7-alpine3.23@sha256:b17af760035fc2f338eed92d448a6c67f2d45438844fc6c60678fa5f99e44b57"
readonly GO_TEST_IMAGE="golang:1.26.7-bookworm@sha256:6ef6e30f0ea5c384f6d111cf856e024e3086bbdcb1779da3f3b3fbba0aea53d2"
readonly JQ_IMAGE="ghcr.io/jqlang/jq:1.8.1@sha256:4f34c6d23f4b1372ac789752cc955dc67c2ae177eb1b5860b75cdc5091ce6f91"
readonly GITLEAKS_IMAGE="zricethezav/gitleaks:v8.30.1@sha256:c00b6bd0aeb3071cbcb79009cb16a60dd9e0a7c60e2be9ab65d25e6bc8abbb7f"
readonly SYFT_IMAGE="anchore/syft:v1.51.0@sha256:678bfa565b60f747aac0f8e964fe5588a24445b8d0a480e91f6efd70020dfbb0"
readonly GOVULNCHECK_VERSION="v1.7.0"
readonly CACHE_ROOT="${VOCAT_BUILD_CACHE_ROOT:-/var/cache/vocat-build}"

target_arch="${1:-amd64}"
case "$target_arch" in
  amd64|arm64) ;;
  *)
    echo "usage: $0 [amd64|arm64]" >&2
    exit 2
    ;;
esac

case "$(uname -m)" in
  x86_64) build_arch="amd64" ;;
  aarch64|arm64) build_arch="arm64" ;;
  *)
    echo "unsupported build host architecture: $(uname -m)" >&2
    exit 2
    ;;
esac

for command in awk cmp cp docker du find git install sha256sum sort tar xargs; do
  if ! command -v "$command" >/dev/null 2>&1; then
    echo "missing required command: $command" >&2
    exit 1
  fi
done

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"
commit="$(git rev-parse --verify HEAD^{commit})"
short_commit="$(git rev-parse --short=12 "$commit")"
build_time="$(git show -s --format=%cI "$commit")"
version="hardened-${short_commit}"
binary_name="vocat-linux-${target_arch}"
final_dir="$repo_root/dist/hardened/$commit"
tool_cache="$CACHE_ROOT/tools/go1.26.7-${build_arch}"

if [ -e "$final_dir" ]; then
  echo "artifact directory already exists: $final_dir" >&2
  exit 1
fi
if [ -n "$(git status --porcelain=v1 --untracked-files=normal)" ]; then
  echo "note: working-tree changes are ignored; building committed HEAD $commit" >&2
fi

mkdir -p \
  "$CACHE_ROOT/npm" \
  "$CACHE_ROOT/go-build" \
  "$CACHE_ROOT/go-mod" \
  "$CACHE_ROOT/syft" \
  "$CACHE_ROOT/tmp" \
  "$tool_cache/bin" \
  "$tool_cache/gopath" \
  "$repo_root/dist/hardened"

temp_root="$(mktemp -d "$CACHE_ROOT/tmp/build-${short_commit}.XXXXXX")"
source_dir="$temp_root/source"
build_source_dir="$temp_root/build-source"
web_work_dir="$temp_root/web-work"
web_build_dir="$temp_root/web-build"
tool_output="$temp_root/output-tool"
jq_tool_dir="$temp_root/jq-tool"
jq_extract_path="$temp_root/jq-extracted"
test_identity_dir="$temp_root/test-identity"
gitleaks_output="$temp_root/output-gitleaks"
npm_output="$temp_root/output-npm"
npm_fetch_output="$temp_root/output-npm-fetch"
npm_audit_full_output="$temp_root/output-npm-audit-full"
npm_audit_production_output="$temp_root/output-npm-audit-production"
npm_lifecycle_output="$temp_root/output-npm-lifecycle"
npm_test_output="$temp_root/output-npm-test"
npm_build_output="$temp_root/output-npm-build"
go_deps_output="$temp_root/output-go-deps"
go_output="$temp_root/output-go"
go_version_output="$temp_root/output-go-version"
go_test_version_output="$temp_root/output-go-test-version"
go_test_output="$temp_root/output-go-test"
go_vet_output="$temp_root/output-go-vet"
go_source_scan_output="$temp_root/output-go-source-scan"
go_build_binary_output="$temp_root/output-go-build-binary"
go_build_log_output="$temp_root/output-go-build-log"
go_metadata_output="$temp_root/output-go-metadata"
go_binary_scan_output="$temp_root/output-go-binary-scan"
race_output="$temp_root/output-race"
race_version_output="$temp_root/output-race-version"
race_test_output="$temp_root/output-race-test"
syft_version_output="$temp_root/output-syft-version"
syft_source_output="$temp_root/output-syft-source"
syft_binary_output="$temp_root/output-syft-binary"
manifest_output="$temp_root/output-manifest"
staging_dir="$(mktemp -d "$repo_root/dist/hardened/.build-${short_commit}.XXXXXX")"
mkdir -p \
  "$source_dir" \
  "$build_source_dir" \
  "$web_work_dir" \
  "$web_build_dir" \
  "$temp_root/gitleaks-home" \
  "$temp_root/gitleaks-tmp" \
  "$temp_root/go-deps-home" \
  "$temp_root/go-deps-gopath" \
  "$temp_root/go-deps-tmp" \
  "$temp_root/go-version-home" \
  "$temp_root/go-version-tmp" \
  "$temp_root/go-test-version-home" \
  "$temp_root/go-test-version-tmp" \
  "$temp_root/go-test-home" \
  "$temp_root/go-test-gopath" \
  "$temp_root/go-test-tmp" \
  "$temp_root/go-vet-home" \
  "$temp_root/go-vet-gopath" \
  "$temp_root/go-vet-tmp" \
  "$temp_root/go-source-scan-home" \
  "$temp_root/go-source-scan-cache" \
  "$temp_root/go-source-scan-gopath" \
  "$temp_root/go-source-scan-tmp" \
  "$temp_root/go-build-home" \
  "$temp_root/go-build-gopath" \
  "$temp_root/go-build-tmp" \
  "$temp_root/go-metadata-home" \
  "$temp_root/go-metadata-tmp" \
  "$temp_root/go-binary-scan-home" \
  "$temp_root/go-binary-scan-gopath" \
  "$temp_root/go-binary-scan-tmp" \
  "$temp_root/manifest-home" \
  "$temp_root/manifest-tmp" \
  "$temp_root/node-fetch-home" \
  "$temp_root/node-fetch-tmp" \
  "$temp_root/node-audit-full-home" \
  "$temp_root/node-audit-full-tmp" \
  "$temp_root/node-audit-production-home" \
  "$temp_root/node-audit-production-tmp" \
  "$temp_root/node-lifecycle-home" \
  "$temp_root/node-lifecycle-tmp" \
  "$temp_root/node-test-home" \
  "$temp_root/node-test-tmp" \
  "$temp_root/node-build-home" \
  "$temp_root/node-build-tmp" \
  "$temp_root/race-version-home" \
  "$temp_root/race-version-tmp" \
  "$temp_root/race-test-home" \
  "$temp_root/race-test-gopath" \
  "$temp_root/race-test-tmp" \
  "$temp_root/syft-binary-home" \
  "$temp_root/syft-binary-tmp" \
  "$temp_root/syft-source-home" \
  "$temp_root/syft-source-tmp" \
  "$temp_root/syft-version-home" \
  "$temp_root/syft-version-tmp" \
  "$temp_root/tool-home" \
  "$temp_root/tool-tmp" \
  "$jq_tool_dir" \
  "$test_identity_dir" \
  "$tool_output" \
  "$gitleaks_output" \
  "$npm_output" \
  "$npm_fetch_output" \
  "$npm_audit_full_output" \
  "$npm_audit_production_output" \
  "$npm_lifecycle_output" \
  "$npm_test_output" \
  "$npm_build_output" \
  "$go_deps_output" \
  "$go_output" \
  "$go_version_output" \
  "$go_test_version_output" \
  "$go_test_output" \
  "$go_vet_output" \
  "$go_source_scan_output" \
  "$go_build_binary_output" \
  "$go_build_log_output" \
  "$go_metadata_output" \
  "$go_binary_scan_output" \
  "$race_output" \
  "$race_version_output" \
  "$race_test_output" \
  "$syft_version_output" \
  "$syft_source_output" \
  "$syft_binary_output" \
  "$manifest_output" \
  "$staging_dir/reports" \
  "$staging_dir/sbom"

mkdir -p \
  "$CACHE_ROOT/go-build/tool-$build_arch" \
  "$CACHE_ROOT/go-build/deps-$build_arch" \
  "$CACHE_ROOT/go-build/test-$build_arch" \
  "$CACHE_ROOT/go-build/vet-$build_arch" \
  "$CACHE_ROOT/go-build/source-scan-$build_arch" \
  "$CACHE_ROOT/go-build/release-$target_arch" \
  "$CACHE_ROOT/go-build/binary-scan-$build_arch" \
  "$CACHE_ROOT/go-build/race-$build_arch"

jq_container_id=""
cleanup() {
  if [ -n "${jq_container_id:-}" ]; then
    docker rm "$jq_container_id" >/dev/null 2>&1 || true
  fi
  if [ -n "${temp_root:-}" ] && [ -d "$temp_root" ]; then
    rm -rf -- "$temp_root"
  fi
  if [ -n "${staging_dir:-}" ] && [ -d "$staging_dir" ]; then
    rm -rf -- "$staging_dir"
  fi
}
trap cleanup EXIT

printf 'root:x:0:0:root:/root:/bin/bash\n' > "$test_identity_dir/passwd"
if [ "$(id -u)" -ne 0 ]; then
  printf 'vocat-build:x:%s:%s:VoCat build test:/phase-home:/usr/sbin/nologin\n' \
    "$(id -u)" "$(id -g)" >> "$test_identity_dir/passwd"
fi
printf 'root:x:0:\n' > "$test_identity_dir/group"
if [ "$(id -g)" -ne 0 ]; then
  printf 'vocat-build:x:%s:vocat-build\n' "$(id -g)" >> "$test_identity_dir/group"
fi
printf 'ID=ubuntu\nVERSION_ID="24.04"\n' > "$test_identity_dir/os-release"
chmod 0444 \
  "$test_identity_dir/group" \
  "$test_identity_dir/os-release" \
  "$test_identity_dir/passwd"

assert_exact_files() {
  local root="$1"
  shift
  local unsafe_entry
  unsafe_entry="$(find "$root" -mindepth 1 ! -type d ! -type f -print -quit)"
  if [ -n "$unsafe_entry" ]; then
    echo "phase output contains a symbolic link or special file: $unsafe_entry" >&2
    exit 1
  fi

  local -a actual=()
  local -a expected=("$@")
  mapfile -d '' -t actual < <(
    cd "$root"
    find . -type f -printf '%P\0' | LC_ALL=C sort -z
  )
  if [ "${#actual[@]}" -ne "${#expected[@]}" ]; then
    echo "unexpected phase output inventory under $root" >&2
    printf 'expected: %s\n' "${expected[*]}" >&2
    printf 'actual: %s\n' "${actual[*]}" >&2
    exit 1
  fi
  local index
  for index in "${!expected[@]}"; do
    if [ "${actual[$index]}" != "${expected[$index]}" ]; then
      echo "unexpected phase output inventory under $root" >&2
      printf 'expected: %s\n' "${expected[*]}" >&2
      printf 'actual: %s\n' "${actual[*]}" >&2
      exit 1
    fi
  done
}

# Capture command output on the host. Containers that execute repository or
# dependency code never receive the report directory as a writable mount.
run_logged() {
  local label="$1"
  local report="$2"
  shift 2
  if "$@" > "$report" 2>&1; then
    printf 'PASS: %s\n' "$label" >> "$report"
  else
    local status=$?
    cat "$report" >&2
    printf 'FAIL: %s\n' "$label" >&2
    return "$status"
  fi
}

run_split_logged() {
  local label="$1"
  local report="$2"
  local error_report="$3"
  shift 3
  if "$@" > "$report" 2> "$error_report"; then
    printf 'PASS: %s\n' "$label" >> "$error_report"
  else
    local status=$?
    cat "$error_report" >&2
    printf 'FAIL: %s\n' "$label" >&2
    return "$status"
  fi
}

git archive --format=tar "$commit" | tar -xf - -C "$source_dir"
if [ ! -f "$source_dir/.gitleaks.toml" ]; then
  echo "committed source is missing .gitleaks.toml" >&2
  exit 1
fi
unsafe_source_entry="$(find "$source_dir" -mindepth 1 ! -type d ! -type f -print -quit)"
if [ -n "$unsafe_source_entry" ]; then
  echo "committed source contains a symbolic link or special file: $unsafe_source_entry" >&2
  exit 1
fi
source_inventory_before="$temp_root/source-inventory.before"
source_inventory_after="$temp_root/source-inventory.after"
(
  cd "$source_dir"
  find . -type f -print0 | LC_ALL=C sort -z | xargs -0 sha256sum
) > "$source_inventory_before"
cp -a -- "$source_dir/." "$build_source_dir/"
cp -a -- "$source_dir/web/." "$web_work_dir/"
if [ -e "$web_work_dir/node_modules" ] || [ -e "$web_work_dir/dist" ]; then
  echo "committed frontend source unexpectedly contains generated output" >&2
  exit 1
fi
mkdir "$web_work_dir/node_modules"

# Install the exact govulncheck module into an architecture-specific persistent
# cache. Reinstalling on every run verifies the pinned module through Go's
# checksum database while keeping downloaded modules and compiled objects.
docker run --rm \
  --read-only \
  --cap-drop=ALL \
  --security-opt=no-new-privileges \
  --user "$(id -u):$(id -g)" \
  --workdir /phase-tmp \
  --env HOME=/phase-home \
  --env TMPDIR=/phase-tmp \
  --env GOPATH=/cache/tools/gopath \
  --env GOBIN=/cache/tools/bin \
  --env GOCACHE=/cache/go-build \
  --env GOMODCACHE=/cache/go-mod \
  --env GOTOOLCHAIN=local \
  --env GODEBUG=gocacheverify=1 \
  --mount "type=bind,src=$temp_root/tool-home,dst=/phase-home" \
  --mount "type=bind,src=$temp_root/tool-tmp,dst=/phase-tmp" \
  --mount "type=bind,src=$CACHE_ROOT/go-build/tool-$build_arch,dst=/cache/go-build" \
  --mount "type=bind,src=$CACHE_ROOT/go-mod,dst=/cache/go-mod" \
  --mount "type=bind,src=$tool_cache,dst=/cache/tools" \
  --mount "type=bind,src=$tool_output,dst=/out" \
  "$GO_IMAGE" \
  sh -ceu '
    CGO_ENABLED=0 go install "golang.org/x/vuln/cmd/govulncheck@'"$GOVULNCHECK_VERSION"'"
    /cache/tools/bin/govulncheck -version > /out/govulncheck-version.txt
    grep -F "Scanner: govulncheck@'"$GOVULNCHECK_VERSION"'" /out/govulncheck-version.txt >/dev/null
  '
assert_exact_files "$tool_output" govulncheck-version.txt

# The deployment-script tests need bash, Python, and jq. The pinned Bookworm Go
# image supplies the first two; copy the exact jq binary from its shell-less,
# digest-pinned image and mount it read-only into the native test containers.
jq_container_id="$(docker create --platform "linux/$build_arch" "$JQ_IMAGE")"
if [[ ! "$jq_container_id" =~ ^[0-9a-f]{64}$ ]]; then
  echo "could not create the pinned jq extraction container" >&2
  exit 1
fi
docker cp "$jq_container_id:/jq" "$jq_extract_path"
docker rm "$jq_container_id" >/dev/null
jq_container_id=""
if [ ! -f "$jq_extract_path" ] || [ -L "$jq_extract_path" ]; then
  echo "pinned jq image did not provide a regular /jq binary" >&2
  exit 1
fi
install -m 0555 "$jq_extract_path" "$jq_tool_dir/jq"
docker run --rm \
  --platform "linux/$build_arch" \
  --read-only \
  --network=none \
  --cap-drop=ALL \
  --security-opt=no-new-privileges \
  --user "$(id -u):$(id -g)" \
  --mount "type=bind,src=$jq_tool_dir/jq,dst=/usr/local/bin/jq,readonly" \
  "$GO_TEST_IMAGE" \
  jq --version > "$tool_output/jq-version.txt"
if [ "$(tr -d '\r\n' < "$tool_output/jq-version.txt")" != "jq-1.8.1" ]; then
  echo "unexpected jq version" >&2
  exit 1
fi
assert_exact_files "$tool_output" govulncheck-version.txt jq-version.txt

docker run --rm \
  --read-only \
  --network=none \
  --cap-drop=ALL \
  --security-opt=no-new-privileges \
  --user "$(id -u):$(id -g)" \
  --env HOME=/phase-home \
  --env TMPDIR=/phase-tmp \
  --mount "type=bind,src=$temp_root/gitleaks-home,dst=/phase-home" \
  --mount "type=bind,src=$temp_root/gitleaks-tmp,dst=/phase-tmp" \
  "$GITLEAKS_IMAGE" \
  version > "$gitleaks_output/gitleaks-version.txt"
if [ "$(tr -d '\r\n' < "$gitleaks_output/gitleaks-version.txt")" != "v8.30.1" ]; then
  echo "unexpected Gitleaks version" >&2
  exit 1
fi

if ! docker run --rm \
  --read-only \
  --network=none \
  --cap-drop=ALL \
  --security-opt=no-new-privileges \
  --user "$(id -u):$(id -g)" \
  --workdir /src \
  --env HOME=/phase-home \
  --env TMPDIR=/phase-tmp \
  --mount "type=bind,src=$source_dir,dst=/src,readonly" \
  --mount "type=bind,src=$temp_root/gitleaks-home,dst=/phase-home" \
  --mount "type=bind,src=$temp_root/gitleaks-tmp,dst=/phase-tmp" \
  --mount "type=bind,src=$gitleaks_output,dst=/out" \
  "$GITLEAKS_IMAGE" \
  dir /src \
    --config /src/.gitleaks.toml \
    --exit-code 1 \
    --log-level warn \
    --no-banner \
    --no-color \
    --redact=100 \
    --report-format json \
    --report-path /out/gitleaks.json; then
  if [ -s "$gitleaks_output/gitleaks.json" ]; then
    cat "$gitleaks_output/gitleaks.json" >&2
  fi
  echo "secret scan failed; inspect the redacted Gitleaks report in the failed build logs" >&2
  exit 1
fi
assert_exact_files "$gitleaks_output" gitleaks-version.txt gitleaks.json

# Resolve the exact lockfile without running dependency lifecycle code. The
# committed frontend is read-only; only node_modules and the content-addressed
# npm cache are writable in this networked phase.
docker run --rm \
  --read-only \
  --network=none \
  --cap-drop=ALL \
  --security-opt=no-new-privileges \
  --user "$(id -u):$(id -g)" \
  --env HOME=/phase-home \
  --env TMPDIR=/phase-tmp \
  --mount "type=bind,src=$temp_root/node-fetch-home,dst=/phase-home" \
  --mount "type=bind,src=$temp_root/node-fetch-tmp,dst=/phase-tmp" \
  "$NODE_IMAGE" \
  node --version > "$npm_fetch_output/node-version.txt"
docker run --rm \
  --read-only \
  --network=none \
  --cap-drop=ALL \
  --security-opt=no-new-privileges \
  --user "$(id -u):$(id -g)" \
  --env HOME=/phase-home \
  --env TMPDIR=/phase-tmp \
  --mount "type=bind,src=$temp_root/node-fetch-home,dst=/phase-home" \
  --mount "type=bind,src=$temp_root/node-fetch-tmp,dst=/phase-tmp" \
  "$NODE_IMAGE" \
  npm --version > "$npm_fetch_output/npm-version.txt"
run_logged "npm ci" "$npm_fetch_output/npm-ci.txt" \
  docker run --rm \
    --read-only \
    --cap-drop=ALL \
    --security-opt=no-new-privileges \
    --user "$(id -u):$(id -g)" \
    --workdir /web \
    --env HOME=/phase-home \
    --env TMPDIR=/phase-tmp \
    --env NPM_CONFIG_CACHE=/cache/npm \
    --env NPM_CONFIG_AUDIT=false \
    --env NPM_CONFIG_FUND=false \
    --env NPM_CONFIG_UPDATE_NOTIFIER=false \
    --mount "type=bind,src=$web_work_dir,dst=/web,readonly" \
    --mount "type=bind,src=$web_work_dir/node_modules,dst=/web/node_modules" \
    --mount "type=bind,src=$CACHE_ROOT/npm,dst=/cache/npm" \
    --mount "type=bind,src=$temp_root/node-fetch-home,dst=/phase-home" \
    --mount "type=bind,src=$temp_root/node-fetch-tmp,dst=/phase-tmp" \
    "$NODE_IMAGE" \
    npm ci --ignore-scripts --no-audit --no-fund
assert_exact_files "$npm_fetch_output" node-version.txt npm-ci.txt npm-version.txt

# Audit the committed lockfile in clean, read-only views. Neither audit phase
# can observe node_modules or any other phase's report directory.
run_split_logged "npm audit (all dependencies, high+)" \
  "$npm_audit_full_output/npm-audit-full.json" \
  "$npm_audit_full_output/npm-audit-full.stderr.txt" \
  docker run --rm \
    --read-only \
    --cap-drop=ALL \
    --security-opt=no-new-privileges \
    --user "$(id -u):$(id -g)" \
    --workdir /web \
    --env HOME=/phase-home \
    --env TMPDIR=/phase-tmp \
    --env NPM_CONFIG_CACHE=/cache/npm \
    --env NPM_CONFIG_FUND=false \
    --env NPM_CONFIG_UPDATE_NOTIFIER=false \
    --mount "type=bind,src=$source_dir/web,dst=/web,readonly" \
    --mount "type=bind,src=$CACHE_ROOT/npm,dst=/cache/npm" \
    --mount "type=bind,src=$temp_root/node-audit-full-home,dst=/phase-home" \
    --mount "type=bind,src=$temp_root/node-audit-full-tmp,dst=/phase-tmp" \
    "$NODE_IMAGE" \
    npm audit --package-lock-only --audit-level=high --json
assert_exact_files "$npm_audit_full_output" npm-audit-full.json npm-audit-full.stderr.txt

run_split_logged "npm audit (production dependencies, high+)" \
  "$npm_audit_production_output/npm-audit-production.json" \
  "$npm_audit_production_output/npm-audit-production.stderr.txt" \
  docker run --rm \
    --read-only \
    --cap-drop=ALL \
    --security-opt=no-new-privileges \
    --user "$(id -u):$(id -g)" \
    --workdir /web \
    --env HOME=/phase-home \
    --env TMPDIR=/phase-tmp \
    --env NPM_CONFIG_CACHE=/cache/npm \
    --env NPM_CONFIG_FUND=false \
    --env NPM_CONFIG_UPDATE_NOTIFIER=false \
    --mount "type=bind,src=$source_dir/web,dst=/web,readonly" \
    --mount "type=bind,src=$CACHE_ROOT/npm,dst=/cache/npm" \
    --mount "type=bind,src=$temp_root/node-audit-production-home,dst=/phase-home" \
    --mount "type=bind,src=$temp_root/node-audit-production-tmp,dst=/phase-tmp" \
    "$NODE_IMAGE" \
    npm audit --package-lock-only --omit=dev --audit-level=high --json
assert_exact_files "$npm_audit_production_output" \
  npm-audit-production.json \
  npm-audit-production.stderr.txt

# Lifecycle scripts run offline and can write only the installed dependency
# tree plus their private HOME/TMP. Tests get that tree read-only; the frontend
# build gets its own copy and cannot rewrite audit or test evidence.
run_logged "npm lifecycle rebuild" "$npm_lifecycle_output/npm-lifecycle.txt" \
  docker run --rm \
    --read-only \
    --network=none \
    --cap-drop=ALL \
    --security-opt=no-new-privileges \
    --user "$(id -u):$(id -g)" \
    --workdir /web \
    --env HOME=/phase-home \
    --env TMPDIR=/phase-tmp \
    --env NPM_CONFIG_AUDIT=false \
    --env NPM_CONFIG_FUND=false \
    --env NPM_CONFIG_UPDATE_NOTIFIER=false \
    --mount "type=bind,src=$web_work_dir,dst=/web,readonly" \
    --mount "type=bind,src=$web_work_dir/node_modules,dst=/web/node_modules" \
    --mount "type=bind,src=$temp_root/node-lifecycle-home,dst=/phase-home" \
    --mount "type=bind,src=$temp_root/node-lifecycle-tmp,dst=/phase-tmp" \
    "$NODE_IMAGE" \
    npm rebuild --offline --no-audit --no-fund
assert_exact_files "$npm_lifecycle_output" npm-lifecycle.txt

run_logged "npm test" "$npm_test_output/npm-test.txt" \
  docker run --rm \
    --read-only \
    --network=none \
    --cap-drop=ALL \
    --security-opt=no-new-privileges \
    --user "$(id -u):$(id -g)" \
    --workdir /web \
    --env HOME=/phase-home \
    --env TMPDIR=/phase-tmp \
    --env NPM_CONFIG_AUDIT=false \
    --env NPM_CONFIG_FUND=false \
    --env NPM_CONFIG_UPDATE_NOTIFIER=false \
    --mount "type=bind,src=$web_work_dir,dst=/web,readonly" \
    --mount "type=bind,src=$temp_root/node-test-home,dst=/phase-home" \
    --mount "type=bind,src=$temp_root/node-test-tmp,dst=/phase-tmp" \
    "$NODE_IMAGE" \
    npm test
assert_exact_files "$npm_test_output" npm-test.txt

cp -a -- "$web_work_dir/." "$web_build_dir/"
run_logged "npm build" "$npm_build_output/npm-build.txt" \
  docker run --rm \
    --read-only \
    --network=none \
    --cap-drop=ALL \
    --security-opt=no-new-privileges \
    --user "$(id -u):$(id -g)" \
    --workdir /web \
    --env HOME=/phase-home \
    --env TMPDIR=/phase-tmp \
    --env NPM_CONFIG_AUDIT=false \
    --env NPM_CONFIG_FUND=false \
    --env NPM_CONFIG_UPDATE_NOTIFIER=false \
    --mount "type=bind,src=$web_build_dir,dst=/web" \
    --mount "type=bind,src=$temp_root/node-build-home,dst=/phase-home" \
    --mount "type=bind,src=$temp_root/node-build-tmp,dst=/phase-tmp" \
    "$NODE_IMAGE" \
    npm run build
assert_exact_files "$npm_build_output" npm-build.txt

install -m 0644 "$npm_fetch_output/node-version.txt" "$npm_output/node-version.txt"
install -m 0644 "$npm_fetch_output/npm-version.txt" "$npm_output/npm-version.txt"
install -m 0644 "$npm_audit_full_output/npm-audit-full.json" "$npm_output/npm-audit-full.json"
install -m 0644 "$npm_audit_full_output/npm-audit-full.stderr.txt" "$npm_output/npm-audit-full.stderr.txt"
install -m 0644 "$npm_audit_production_output/npm-audit-production.json" "$npm_output/npm-audit-production.json"
install -m 0644 "$npm_audit_production_output/npm-audit-production.stderr.txt" "$npm_output/npm-audit-production.stderr.txt"
install -m 0644 "$npm_test_output/npm-test.txt" "$npm_output/npm-test.txt"
install -m 0644 "$npm_build_output/npm-build.txt" "$npm_output/npm-build.txt"
{
  cat "$npm_fetch_output/npm-ci.txt"
  cat "$npm_lifecycle_output/npm-lifecycle.txt"
} > "$npm_output/npm-ci.txt"
chmod 0644 "$npm_output/npm-ci.txt"
assert_exact_files "$npm_output" \
  node-version.txt \
  npm-audit-full.json \
  npm-audit-full.stderr.txt \
  npm-audit-production.json \
  npm-audit-production.stderr.txt \
  npm-build.txt \
  npm-ci.txt \
  npm-test.txt \
  npm-version.txt

web_dist="$web_build_dir/dist"
if [ ! -d "$web_dist" ] || [ -L "$web_dist" ] || [ ! -f "$web_dist/index.html" ]; then
  echo "frontend build did not produce a safe dist directory with index.html" >&2
  exit 1
fi
unsafe_web_entry="$(find "$web_dist" -mindepth 1 ! -type d ! -type f -print -quit)"
if [ -n "$unsafe_web_entry" ]; then
  echo "frontend dist contains a symbolic link or special file: $unsafe_web_entry" >&2
  exit 1
fi
mapfile -d '' -t web_dist_files < <(find "$web_dist" -type f -print0)
web_dist_file_count="${#web_dist_files[@]}"
web_dist_size="$(du -sb -- "$web_dist" | awk '{print $1}')"
if [ "$web_dist_file_count" -lt 1 ] || [ "$web_dist_file_count" -gt 2000 ]; then
  echo "frontend dist file count is outside the accepted range: $web_dist_file_count" >&2
  exit 1
fi
if [ "$web_dist_size" -gt 52428800 ]; then
  echo "frontend dist exceeds the 50 MiB release limit: $web_dist_size bytes" >&2
  exit 1
fi
if [ -e "$build_source_dir/web/dist" ]; then
  echo "committed source unexpectedly contains web/dist" >&2
  exit 1
fi
mkdir "$build_source_dir/web/dist"
cp -a -- "$web_dist/." "$build_source_dir/web/dist/"

docker run --rm \
  --read-only \
  --cap-drop=ALL \
  --security-opt=no-new-privileges \
  --user "$(id -u):$(id -g)" \
  --workdir /src \
  --env HOME=/phase-home \
  --env TMPDIR=/phase-tmp \
  --env GOPATH=/phase-gopath \
  --env GOCACHE=/cache/go-build \
  --env GOMODCACHE=/cache/go-mod \
  --env GOTOOLCHAIN=local \
  --mount "type=bind,src=$build_source_dir,dst=/src,readonly" \
  --mount "type=bind,src=$temp_root/go-deps-home,dst=/phase-home" \
  --mount "type=bind,src=$temp_root/go-deps-tmp,dst=/phase-tmp" \
  --mount "type=bind,src=$temp_root/go-deps-gopath,dst=/phase-gopath" \
  --mount "type=bind,src=$CACHE_ROOT/go-build/deps-$build_arch,dst=/cache/go-build" \
  --mount "type=bind,src=$CACHE_ROOT/go-mod,dst=/cache/go-mod" \
  --mount "type=bind,src=$go_deps_output,dst=/out" \
  "$GO_IMAGE" \
  sh -ceu '
    run_logged() {
      label="$1"
      report="$2"
      shift 2
      if "$@" > "$report" 2>&1; then
        printf "PASS: %s\n" "$label" >> "$report"
      else
        status="$?"
        cat "$report" >&2
        printf "FAIL: %s\n" "$label" >&2
        return "$status"
      fi
    }
    run_logged "go mod download" /out/go-mod-download.txt go mod download all
    run_logged "go mod verify" /out/go-mod-verify.txt go mod verify
  '
assert_exact_files "$go_deps_output" go-mod-download.txt go-mod-verify.txt

# Every source-consuming Go command gets a private HOME, TMPDIR, GOPATH,
# GOCACHE, and host-captured report. After the dependency phase, the verified
# module cache is read-only. Tests and builds are offline; only govulncheck can
# reach the vulnerability database.
docker run --rm \
  --read-only \
  --network=none \
  --cap-drop=ALL \
  --security-opt=no-new-privileges \
  --user "$(id -u):$(id -g)" \
  --env HOME=/phase-home \
  --env TMPDIR=/phase-tmp \
  --mount "type=bind,src=$temp_root/go-version-home,dst=/phase-home" \
  --mount "type=bind,src=$temp_root/go-version-tmp,dst=/phase-tmp" \
  "$GO_IMAGE" \
  go version > "$go_version_output/go-version.txt"
assert_exact_files "$go_version_output" go-version.txt

docker run --rm \
  --platform "linux/$build_arch" \
  --read-only \
  --network=none \
  --cap-drop=ALL \
  --security-opt=no-new-privileges \
  --user "$(id -u):$(id -g)" \
  --env HOME=/phase-home \
  --env TMPDIR=/phase-tmp \
  --mount "type=bind,src=$temp_root/go-test-version-home,dst=/phase-home" \
  --mount "type=bind,src=$temp_root/go-test-version-tmp,dst=/phase-tmp" \
  "$GO_TEST_IMAGE" \
  go version > "$go_test_version_output/go-test-version.txt"
assert_exact_files "$go_test_version_output" go-test-version.txt

run_logged "go test" "$go_test_output/go-test.txt" \
  docker run --rm \
    --platform "linux/$build_arch" \
    --read-only \
    --network=none \
    --cap-drop=ALL \
    --security-opt=no-new-privileges \
    --user "$(id -u):$(id -g)" \
    --workdir /src \
    --env HOME=/phase-home \
    --env TMPDIR=/phase-tmp \
    --env GOPATH=/phase-gopath \
    --env GOCACHE=/cache/go-build \
    --env GOMODCACHE=/cache/go-mod \
    --env GOTOOLCHAIN=local \
    --env GOPROXY=off \
    --env GODEBUG=gocacheverify=1 \
    --env CGO_ENABLED=0 \
    --mount "type=bind,src=$build_source_dir,dst=/src,readonly" \
    --mount "type=bind,src=$temp_root/go-test-home,dst=/phase-home" \
    --mount "type=bind,src=$temp_root/go-test-tmp,dst=/phase-tmp" \
    --mount "type=bind,src=$temp_root/go-test-gopath,dst=/phase-gopath" \
    --mount "type=bind,src=$CACHE_ROOT/go-build/test-$build_arch,dst=/cache/go-build" \
    --mount "type=bind,src=$CACHE_ROOT/go-mod,dst=/cache/go-mod,readonly" \
    --mount "type=bind,src=$jq_tool_dir/jq,dst=/usr/local/bin/jq,readonly" \
    --mount "type=bind,src=$test_identity_dir/passwd,dst=/etc/passwd,readonly" \
    --mount "type=bind,src=$test_identity_dir/group,dst=/etc/group,readonly" \
    --mount "type=bind,src=$test_identity_dir/os-release,dst=/etc/os-release,readonly" \
    "$GO_TEST_IMAGE" \
    go test -timeout=30m ./...
assert_exact_files "$go_test_output" go-test.txt

run_logged "go vet" "$go_vet_output/go-vet.txt" \
  docker run --rm \
    --read-only \
    --network=none \
    --cap-drop=ALL \
    --security-opt=no-new-privileges \
    --user "$(id -u):$(id -g)" \
    --workdir /src \
    --env HOME=/phase-home \
    --env TMPDIR=/phase-tmp \
    --env GOPATH=/phase-gopath \
    --env GOCACHE=/cache/go-build \
    --env GOMODCACHE=/cache/go-mod \
    --env GOTOOLCHAIN=local \
    --env GOPROXY=off \
    --env GODEBUG=gocacheverify=1 \
    --env CGO_ENABLED=0 \
    --mount "type=bind,src=$build_source_dir,dst=/src,readonly" \
    --mount "type=bind,src=$temp_root/go-vet-home,dst=/phase-home" \
    --mount "type=bind,src=$temp_root/go-vet-tmp,dst=/phase-tmp" \
    --mount "type=bind,src=$temp_root/go-vet-gopath,dst=/phase-gopath" \
    --mount "type=bind,src=$CACHE_ROOT/go-build/vet-$build_arch,dst=/cache/go-build" \
    --mount "type=bind,src=$CACHE_ROOT/go-mod,dst=/cache/go-mod,readonly" \
    "$GO_IMAGE" \
    go vet ./...
assert_exact_files "$go_vet_output" go-vet.txt

# govulncheck's go/packages loader must read export data that the go command
# writes to GOCACHE. gocacheverify bypasses those entries, so use a fresh,
# build-scoped cache instead of reusing an unverifiable persistent cache.
run_split_logged "govulncheck source" \
  "$go_source_scan_output/govulncheck-source.sarif.json" \
  "$go_source_scan_output/govulncheck-source.stderr.txt" \
  docker run --rm \
    --read-only \
    --cap-drop=ALL \
    --security-opt=no-new-privileges \
    --user "$(id -u):$(id -g)" \
    --workdir /src \
    --env HOME=/phase-home \
    --env TMPDIR=/phase-tmp \
    --env GOPATH=/phase-gopath \
    --env GOCACHE=/cache/go-build \
    --env GOMODCACHE=/cache/go-mod \
    --env GOTOOLCHAIN=local \
    --env GOPROXY=off \
    --env CGO_ENABLED=0 \
    --mount "type=bind,src=$build_source_dir,dst=/src,readonly" \
    --mount "type=bind,src=$temp_root/go-source-scan-home,dst=/phase-home" \
    --mount "type=bind,src=$temp_root/go-source-scan-tmp,dst=/phase-tmp" \
    --mount "type=bind,src=$temp_root/go-source-scan-gopath,dst=/phase-gopath" \
    --mount "type=bind,src=$temp_root/go-source-scan-cache,dst=/cache/go-build" \
    --mount "type=bind,src=$CACHE_ROOT/go-mod,dst=/cache/go-mod,readonly" \
    --mount "type=bind,src=$tool_cache,dst=/cache/tools,readonly" \
    "$GO_IMAGE" \
    /cache/tools/bin/govulncheck -scan=symbol -format=sarif ./...
assert_exact_files "$go_source_scan_output" \
  govulncheck-source.sarif.json \
  govulncheck-source.stderr.txt

run_logged "static Go build" "$go_build_log_output/go-build.txt" \
  docker run --rm \
    --read-only \
    --network=none \
    --cap-drop=ALL \
    --security-opt=no-new-privileges \
    --user "$(id -u):$(id -g)" \
    --workdir /src \
    --env HOME=/phase-home \
    --env TMPDIR=/phase-tmp \
    --env GOPATH=/phase-gopath \
    --env GOCACHE=/cache/go-build \
    --env GOMODCACHE=/cache/go-mod \
    --env GOTOOLCHAIN=local \
    --env GOPROXY=off \
    --env GODEBUG=gocacheverify=1 \
    --env CGO_ENABLED=0 \
    --env GOOS=linux \
    --env GOARCH="$target_arch" \
    --mount "type=bind,src=$build_source_dir,dst=/src,readonly" \
    --mount "type=bind,src=$temp_root/go-build-home,dst=/phase-home" \
    --mount "type=bind,src=$temp_root/go-build-tmp,dst=/phase-tmp" \
    --mount "type=bind,src=$temp_root/go-build-gopath,dst=/phase-gopath" \
    --mount "type=bind,src=$CACHE_ROOT/go-build/release-$target_arch,dst=/cache/go-build" \
    --mount "type=bind,src=$CACHE_ROOT/go-mod,dst=/cache/go-mod,readonly" \
    --mount "type=bind,src=$go_build_binary_output,dst=/out" \
    "$GO_IMAGE" \
    go build -trimpath \
      -ldflags "-w -X vocat/internal/buildinfo.Version=$version -X vocat/internal/buildinfo.BuildTime=$build_time" \
      -o "/out/$binary_name" \
      ./cmd/vocat
assert_exact_files "$go_build_log_output" go-build.txt
assert_exact_files "$go_build_binary_output" "$binary_name"

docker run --rm \
  --read-only \
  --network=none \
  --cap-drop=ALL \
  --security-opt=no-new-privileges \
  --user "$(id -u):$(id -g)" \
  --env HOME=/phase-home \
  --env TMPDIR=/phase-tmp \
  --mount "type=bind,src=$temp_root/go-metadata-home,dst=/phase-home" \
  --mount "type=bind,src=$temp_root/go-metadata-tmp,dst=/phase-tmp" \
  --mount "type=bind,src=$go_build_binary_output,dst=/artifact,readonly" \
  "$GO_IMAGE" \
  go version -m "/artifact/$binary_name" > "$go_metadata_output/go-version-m.txt"
assert_exact_files "$go_metadata_output" go-version-m.txt
binary_go_version="$(awk 'NR == 1 { print $NF; exit }' "$go_metadata_output/go-version-m.txt")"
binary_goos="$(awk '$1 == "build" && $2 ~ /^GOOS=/ { sub(/^GOOS=/, "", $2); print $2 }' "$go_metadata_output/go-version-m.txt")"
binary_goarch="$(awk '$1 == "build" && $2 ~ /^GOARCH=/ { sub(/^GOARCH=/, "", $2); print $2 }' "$go_metadata_output/go-version-m.txt")"
[ "$binary_go_version" = "go1.26.7" ] || {
  echo "release binary was not built by Go 1.26.7" >&2
  exit 1
}
[ "$binary_goos" = "linux" ] || {
  echo "release binary GOOS metadata is not linux" >&2
  exit 1
}
[ "$binary_goarch" = "$target_arch" ] || {
  echo "release binary GOARCH metadata does not match target $target_arch" >&2
  exit 1
}

run_split_logged "govulncheck binary" \
  "$go_binary_scan_output/govulncheck-binary.sarif.json" \
  "$go_binary_scan_output/govulncheck-binary.stderr.txt" \
  docker run --rm \
    --read-only \
    --cap-drop=ALL \
    --security-opt=no-new-privileges \
    --user "$(id -u):$(id -g)" \
    --env HOME=/phase-home \
    --env TMPDIR=/phase-tmp \
    --env GOPATH=/phase-gopath \
    --env GOCACHE=/cache/go-build \
    --env GOMODCACHE=/cache/go-mod \
    --env GOTOOLCHAIN=local \
    --env GOPROXY=off \
    --mount "type=bind,src=$temp_root/go-binary-scan-home,dst=/phase-home" \
    --mount "type=bind,src=$temp_root/go-binary-scan-tmp,dst=/phase-tmp" \
    --mount "type=bind,src=$temp_root/go-binary-scan-gopath,dst=/phase-gopath" \
    --mount "type=bind,src=$CACHE_ROOT/go-build/binary-scan-$build_arch,dst=/cache/go-build" \
    --mount "type=bind,src=$CACHE_ROOT/go-mod,dst=/cache/go-mod,readonly" \
    --mount "type=bind,src=$tool_cache,dst=/cache/tools,readonly" \
    --mount "type=bind,src=$go_build_binary_output,dst=/artifact,readonly" \
    "$GO_IMAGE" \
    /cache/tools/bin/govulncheck -mode=binary -scan=symbol -format=sarif "/artifact/$binary_name"
assert_exact_files "$go_binary_scan_output" \
  govulncheck-binary.sarif.json \
  govulncheck-binary.stderr.txt

install -m 0755 "$go_build_binary_output/$binary_name" "$go_output/$binary_name"
install -m 0644 "$go_build_log_output/go-build.txt" "$go_output/go-build.txt"
install -m 0644 "$go_test_output/go-test.txt" "$go_output/go-test.txt"
install -m 0644 "$go_metadata_output/go-version-m.txt" "$go_output/go-version-m.txt"
install -m 0644 "$go_version_output/go-version.txt" "$go_output/go-version.txt"
install -m 0644 "$go_vet_output/go-vet.txt" "$go_output/go-vet.txt"
install -m 0644 "$go_binary_scan_output/govulncheck-binary.sarif.json" "$go_output/govulncheck-binary.sarif.json"
install -m 0644 "$go_binary_scan_output/govulncheck-binary.stderr.txt" "$go_output/govulncheck-binary.stderr.txt"
install -m 0644 "$go_source_scan_output/govulncheck-source.sarif.json" "$go_output/govulncheck-source.sarif.json"
install -m 0644 "$go_source_scan_output/govulncheck-source.stderr.txt" "$go_output/govulncheck-source.stderr.txt"
assert_exact_files "$go_output" \
  go-build.txt \
  go-test.txt \
  go-version-m.txt \
  go-version.txt \
  go-vet.txt \
  govulncheck-binary.sarif.json \
  govulncheck-binary.stderr.txt \
  govulncheck-source.sarif.json \
  govulncheck-source.stderr.txt \
  "$binary_name"

# The race detector requires CGO and a native C compiler. Its test container is
# offline and receives no writable report mount or cache shared with a later
# phase. Only the final static build is cross-compiled.
docker run --rm \
  --platform "linux/$build_arch" \
  --read-only \
  --network=none \
  --cap-drop=ALL \
  --security-opt=no-new-privileges \
  --user "$(id -u):$(id -g)" \
  --env HOME=/phase-home \
  --env TMPDIR=/phase-tmp \
  --mount "type=bind,src=$temp_root/race-version-home,dst=/phase-home" \
  --mount "type=bind,src=$temp_root/race-version-tmp,dst=/phase-tmp" \
  "$GO_TEST_IMAGE" \
  go version > "$race_version_output/go-race-version.txt"
assert_exact_files "$race_version_output" go-race-version.txt

run_logged "go test -race" "$race_test_output/go-test-race.txt" \
  docker run --rm \
    --platform "linux/$build_arch" \
    --read-only \
    --network=none \
    --cap-drop=ALL \
    --security-opt=no-new-privileges \
    --user "$(id -u):$(id -g)" \
    --workdir /src \
    --env HOME=/phase-home \
    --env TMPDIR=/phase-tmp \
    --env GOPATH=/phase-gopath \
    --env GOCACHE=/cache/go-build \
    --env GOMODCACHE=/cache/go-mod \
    --env GOTOOLCHAIN=local \
    --env GOPROXY=off \
    --env GODEBUG=gocacheverify=1 \
    --env CGO_ENABLED=1 \
    --mount "type=bind,src=$build_source_dir,dst=/src,readonly" \
    --mount "type=bind,src=$temp_root/race-test-home,dst=/phase-home" \
    --mount "type=bind,src=$temp_root/race-test-tmp,dst=/phase-tmp" \
    --mount "type=bind,src=$temp_root/race-test-gopath,dst=/phase-gopath" \
    --mount "type=bind,src=$CACHE_ROOT/go-build/race-$build_arch,dst=/cache/go-build" \
    --mount "type=bind,src=$CACHE_ROOT/go-mod,dst=/cache/go-mod,readonly" \
    --mount "type=bind,src=$jq_tool_dir/jq,dst=/usr/local/bin/jq,readonly" \
    --mount "type=bind,src=$test_identity_dir/passwd,dst=/etc/passwd,readonly" \
    --mount "type=bind,src=$test_identity_dir/group,dst=/etc/group,readonly" \
    --mount "type=bind,src=$test_identity_dir/os-release,dst=/etc/os-release,readonly" \
    "$GO_TEST_IMAGE" \
    go test -race -p 1 -timeout=30m ./...
assert_exact_files "$race_test_output" go-test-race.txt
install -m 0644 "$race_version_output/go-race-version.txt" "$race_output/go-race-version.txt"
install -m 0644 "$race_test_output/go-test-race.txt" "$race_output/go-test-race.txt"
assert_exact_files "$race_output" go-race-version.txt go-test-race.txt

docker run --rm \
  --read-only \
  --network=none \
  --cap-drop=ALL \
  --security-opt=no-new-privileges \
  --user "$(id -u):$(id -g)" \
  --env SYFT_CHECK_FOR_APP_UPDATE=false \
  --env SYFT_CACHE_DIR=/cache/syft \
  --env HOME=/phase-home \
  --env TMPDIR=/phase-tmp \
  --mount "type=bind,src=$CACHE_ROOT/syft,dst=/cache/syft" \
  --mount "type=bind,src=$temp_root/syft-version-home,dst=/phase-home" \
  --mount "type=bind,src=$temp_root/syft-version-tmp,dst=/phase-tmp" \
  "$SYFT_IMAGE" \
  version -o json > "$syft_version_output/syft-version.json"
assert_exact_files "$syft_version_output" syft-version.json

docker run --rm \
  --read-only \
  --network=none \
  --cap-drop=ALL \
  --security-opt=no-new-privileges \
  --user "$(id -u):$(id -g)" \
  --env SYFT_CHECK_FOR_APP_UPDATE=false \
  --env SYFT_CACHE_DIR=/cache/syft \
  --env HOME=/phase-home \
  --env TMPDIR=/phase-tmp \
  --mount "type=bind,src=$build_source_dir,dst=/src,readonly" \
  --mount "type=bind,src=$syft_source_output,dst=/out" \
  --mount "type=bind,src=$CACHE_ROOT/syft,dst=/cache/syft" \
  --mount "type=bind,src=$temp_root/syft-source-home,dst=/phase-home" \
  --mount "type=bind,src=$temp_root/syft-source-tmp,dst=/phase-tmp" \
  "$SYFT_IMAGE" \
  scan dir:/src \
    --source-name VoCat \
    --source-version "$version" \
    -o cyclonedx-json=/out/source.cdx.json
assert_exact_files "$syft_source_output" source.cdx.json

docker run --rm \
  --read-only \
  --network=none \
  --cap-drop=ALL \
  --security-opt=no-new-privileges \
  --user "$(id -u):$(id -g)" \
  --env SYFT_CHECK_FOR_APP_UPDATE=false \
  --env SYFT_CACHE_DIR=/cache/syft \
  --env HOME=/phase-home \
  --env TMPDIR=/phase-tmp \
  --mount "type=bind,src=$go_output,dst=/artifact,readonly" \
  --mount "type=bind,src=$syft_binary_output,dst=/out" \
  --mount "type=bind,src=$CACHE_ROOT/syft,dst=/cache/syft" \
  --mount "type=bind,src=$temp_root/syft-binary-home,dst=/phase-home" \
  --mount "type=bind,src=$temp_root/syft-binary-tmp,dst=/phase-tmp" \
  "$SYFT_IMAGE" \
  scan "file:/artifact/$binary_name" \
    --source-name "$binary_name" \
    --source-version "$version" \
    -o cyclonedx-json=/out/binary.cdx.json
assert_exact_files "$syft_binary_output" binary.cdx.json

(
  cd "$source_dir"
  find . -type f -print0 | LC_ALL=C sort -z | xargs -0 sha256sum
) > "$source_inventory_after"
if ! cmp -s "$source_inventory_before" "$source_inventory_after"; then
  echo "committed source changed during the isolated build" >&2
  exit 1
fi

install -m 0755 "$go_output/$binary_name" "$staging_dir/$binary_name"
install -m 0644 "$go_output/go-version.txt" "$staging_dir/go-version.txt"
install -m 0644 "$go_test_version_output/go-test-version.txt" "$staging_dir/reports/go-test-version.txt"
install -m 0644 "$npm_output/node-version.txt" "$staging_dir/node-version.txt"
install -m 0644 "$npm_output/npm-version.txt" "$staging_dir/npm-version.txt"
install -m 0644 "$tool_output/govulncheck-version.txt" "$staging_dir/reports/govulncheck-version.txt"
install -m 0644 "$tool_output/jq-version.txt" "$staging_dir/reports/jq-version.txt"
install -m 0644 "$gitleaks_output/gitleaks-version.txt" "$staging_dir/reports/gitleaks-version.txt"
install -m 0644 "$gitleaks_output/gitleaks.json" "$staging_dir/reports/gitleaks.json"
install -m 0644 "$syft_version_output/syft-version.json" "$staging_dir/reports/syft-version.json"
install -m 0644 "$syft_source_output/source.cdx.json" "$staging_dir/sbom/source.cdx.json"
install -m 0644 "$syft_binary_output/binary.cdx.json" "$staging_dir/sbom/binary.cdx.json"
for report in \
  npm-audit-full.json \
  npm-audit-full.stderr.txt \
  npm-audit-production.json \
  npm-audit-production.stderr.txt \
  npm-build.txt \
  npm-ci.txt \
  npm-test.txt; do
  install -m 0644 "$npm_output/$report" "$staging_dir/reports/$report"
done
for report in go-mod-download.txt go-mod-verify.txt; do
  install -m 0644 "$go_deps_output/$report" "$staging_dir/reports/$report"
done
for report in \
  go-build.txt \
  go-test.txt \
  go-version-m.txt \
  go-vet.txt \
  govulncheck-binary.sarif.json \
  govulncheck-binary.stderr.txt \
  govulncheck-source.sarif.json \
  govulncheck-source.stderr.txt; do
  install -m 0644 "$go_output/$report" "$staging_dir/reports/$report"
done
install -m 0644 "$race_output/go-race-version.txt" "$staging_dir/reports/go-race-version.txt"
install -m 0644 "$race_output/go-test-race.txt" "$staging_dir/reports/go-test-race.txt"

binary_sha256="$(sha256sum "$staging_dir/$binary_name" | awk '{print $1}')"

# Validate every structured report, enforce the vulnerability thresholds, and
# generate the manifest with the same pinned Node runtime used for the UI.
docker run --rm \
  --read-only \
  --network=none \
  --cap-drop=ALL \
  --security-opt=no-new-privileges \
  --user "$(id -u):$(id -g)" \
  --env HOME=/phase-home \
  --env TMPDIR=/phase-tmp \
  --env VOCAT_BINARY="$binary_name" \
  --env VOCAT_BINARY_SHA256="$binary_sha256" \
  --env VOCAT_BUILD_ARCH="$build_arch" \
  --env VOCAT_BUILD_TIME="$build_time" \
  --env VOCAT_COMMIT="$commit" \
  --env VOCAT_FRONTEND_FILE_COUNT="$web_dist_file_count" \
  --env VOCAT_FRONTEND_SIZE_BYTES="$web_dist_size" \
  --env VOCAT_GITLEAKS_IMAGE="$GITLEAKS_IMAGE" \
  --env VOCAT_GO_IMAGE="$GO_IMAGE" \
  --env VOCAT_GO_TEST_IMAGE="$GO_TEST_IMAGE" \
  --env VOCAT_GOVULNCHECK_VERSION="$GOVULNCHECK_VERSION" \
  --env VOCAT_JQ_IMAGE="$JQ_IMAGE" \
  --env VOCAT_NODE_IMAGE="$NODE_IMAGE" \
  --env VOCAT_SYFT_IMAGE="$SYFT_IMAGE" \
  --env VOCAT_TARGET_ARCH="$target_arch" \
  --env VOCAT_VERSION="$version" \
  --mount "type=bind,src=$staging_dir,dst=/artifact,readonly" \
  --mount "type=bind,src=$manifest_output,dst=/out" \
  --mount "type=bind,src=$temp_root/manifest-home,dst=/phase-home" \
  --mount "type=bind,src=$temp_root/manifest-tmp,dst=/phase-tmp" \
  "$NODE_IMAGE" \
  node -e '
    const fs = require("node:fs");
    const path = require("node:path");
    const read = (name) => fs.readFileSync(path.join("/artifact", name), "utf8").trim();
    const json = (name) => JSON.parse(read(name));

    const leaks = json("reports/gitleaks.json");
    if (!Array.isArray(leaks) || leaks.length !== 0) throw new Error("secret scan contains findings");

    const auditGate = (name) => {
      const report = json(name);
      const counts = report.metadata?.vulnerabilities;
      const dependencies = report.metadata?.dependencies;
      const validCount = (value) => Number.isSafeInteger(value) && value >= 0;
      const severityFields = ["info", "low", "moderate", "high", "critical"];
      const dependencyFields = ["prod", "dev", "optional", "peer", "peerOptional", "total"];
      if (report.auditReportVersion !== 2 || !counts ||
          severityFields.some((field) => !validCount(counts[field])) ||
          !validCount(counts.total) ||
          counts.total !== severityFields.reduce((total, field) => total + counts[field], 0) ||
          !dependencies || dependencyFields.some((field) => !validCount(dependencies[field]))) {
        throw new Error(`${name} is not a complete npm audit v2 report`);
      }
      const { high, critical } = counts;
      if (high + critical !== 0) throw new Error(`${name} contains high or critical vulnerabilities`);
      return { high, critical, report: name };
    };
    const sarifGate = (name) => {
      const report = json(name);
      if (report.version !== "2.1.0" || !Array.isArray(report.runs) || report.runs.length === 0 ||
          report.runs.some((run) => run.tool?.driver?.name !== "govulncheck" || !Array.isArray(run.results))) {
        throw new Error(`${name} is not a complete govulncheck SARIF report`);
      }
      const results = report.runs.flatMap((run) => run.results);
      if (results.some((result) => !["error", "warning", "note"].includes(result.level))) {
        throw new Error(`${name} contains an unknown SARIF result level`);
      }
      const reachable = results.filter((result) => result.level === "error");
      if (reachable.length !== 0) throw new Error(`${name} contains reachable vulnerabilities`);
      return { reachable_findings: reachable.length, total_findings: results.length, report: name };
    };
    const sbom = (name) => {
      const report = json(name);
      if (report.bomFormat !== "CycloneDX" || !Array.isArray(report.components) ||
          report.components.length === 0) {
        throw new Error(`${name} is not a CycloneDX component inventory`);
      }
      return { format: "CycloneDX JSON", components: report.components.length, file: name };
    };
    const passedLog = (name, label) => {
      if (!read(name).split(/\r?\n/).includes(`PASS: ${label}`)) {
        throw new Error(`${name} does not record a passing ${label} gate`);
      }
      return { status: "passed", report: name };
    };

    const manifest = {
      schema: 2,
      source: {
        type: "git archive",
        commit: process.env.VOCAT_COMMIT,
        frontend_dist: {
          isolated_build: true,
          files: Number(process.env.VOCAT_FRONTEND_FILE_COUNT),
          size_bytes: Number(process.env.VOCAT_FRONTEND_SIZE_BYTES),
        },
      },
      target: `linux/${process.env.VOCAT_TARGET_ARCH}`,
      build_platform: `linux/${process.env.VOCAT_BUILD_ARCH}`,
      version: process.env.VOCAT_VERSION,
      build_time: process.env.VOCAT_BUILD_TIME,
      binary: {
        file: process.env.VOCAT_BINARY,
        sha256: process.env.VOCAT_BINARY_SHA256,
        go_version_m: "reports/go-version-m.txt",
      },
      toolchain: {
        go: { image: process.env.VOCAT_GO_IMAGE, version: read("go-version.txt") },
        go_test: { image: process.env.VOCAT_GO_TEST_IMAGE, version: read("reports/go-test-version.txt") },
        go_race: { image: process.env.VOCAT_GO_TEST_IMAGE, version: read("reports/go-race-version.txt") },
        node: { image: process.env.VOCAT_NODE_IMAGE, version: read("node-version.txt") },
        npm: { version: read("npm-version.txt") },
        jq: { image: process.env.VOCAT_JQ_IMAGE, version: read("reports/jq-version.txt") },
        govulncheck: { version: process.env.VOCAT_GOVULNCHECK_VERSION, report: "reports/govulncheck-version.txt" },
        gitleaks: { image: process.env.VOCAT_GITLEAKS_IMAGE, version: read("reports/gitleaks-version.txt") },
        syft: { image: process.env.VOCAT_SYFT_IMAGE, version: json("reports/syft-version.json").version },
      },
      gates: {
        secret_scan: { status: "passed", findings: leaks.length, report: "reports/gitleaks.json" },
        npm_audit_full: { status: "passed", threshold: "high", ...auditGate("reports/npm-audit-full.json") },
        npm_audit_production: { status: "passed", threshold: "high", ...auditGate("reports/npm-audit-production.json") },
        npm_test: passedLog("reports/npm-test.txt", "npm test"),
        npm_build: passedLog("reports/npm-build.txt", "npm build"),
        go_mod_download: passedLog("reports/go-mod-download.txt", "go mod download"),
        go_mod_verify: passedLog("reports/go-mod-verify.txt", "go mod verify"),
        go_test: { status: "passed", report: "reports/go-test.txt" },
        go_test_race: { status: "passed", report: "reports/go-test-race.txt" },
        go_vet: { status: "passed", report: "reports/go-vet.txt" },
        govulncheck_source: { status: "passed", scan: "symbol", ...sarifGate("reports/govulncheck-source.sarif.json") },
        govulncheck_binary: { status: "passed", scan: "symbol", ...sarifGate("reports/govulncheck-binary.sarif.json") },
      },
      sbom: {
        source: sbom("sbom/source.cdx.json"),
        binary: sbom("sbom/binary.cdx.json"),
      },
      integrity: { checksums: "SHA256SUMS", scope: "all regular artifact files except SHA256SUMS" },
    };
    fs.writeFileSync("/out/manifest.json", `${JSON.stringify(manifest, null, 2)}\n`);
  '
assert_exact_files "$manifest_output" manifest.json
install -m 0644 "$manifest_output/manifest.json" "$staging_dir/manifest.json"

find "$staging_dir/reports" "$staging_dir/sbom" -type f -exec chmod 0644 {} +
chmod 0644 \
  "$staging_dir/go-version.txt" \
  "$staging_dir/manifest.json" \
  "$staging_dir/node-version.txt" \
  "$staging_dir/npm-version.txt"
(
  cd "$staging_dir"
  find . -type f ! -name SHA256SUMS -print0 \
    | LC_ALL=C sort -z \
    | xargs -0 sha256sum
) > "$staging_dir/SHA256SUMS"
chmod 0644 "$staging_dir/SHA256SUMS"
(
  cd "$staging_dir"
  sha256sum -c SHA256SUMS >/dev/null
)
artifact_index_sha256="$(sha256sum "$staging_dir/SHA256SUMS" | awk '{print $1}')"
[[ "$artifact_index_sha256" =~ ^[0-9a-f]{64}$ ]] || {
  echo "could not calculate artifact index SHA-256" >&2
  exit 1
}

# -T makes a concurrent publisher fail instead of nesting its staging
# directory inside an artifact that another build already published.
mv -T -- "$staging_dir" "$final_dir"
staging_dir=""
echo "hardened artifact: $final_dir"
echo "sha256: $binary_sha256"
echo "artifact index sha256: $artifact_index_sha256"
