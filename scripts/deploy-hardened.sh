#!/usr/bin/env bash
set -Eeuo pipefail
export LC_ALL=C
umask 077

# Production deployment is deliberately boring: all state lives below the
# fixed system paths. The explicit test root only exists for the regression
# harness and is rejected unless it is an absolute path.
test_mode=false
if [ -n "${VOCAT_DEPLOY_TEST_ROOT:-}" ]; then
  [[ "$VOCAT_DEPLOY_TEST_ROOT" = /* ]] || { echo "vocat-deploy: test root must be absolute" >&2; exit 1; }
  test_mode=true
  readonly DEPLOY_ROOT="${VOCAT_DEPLOY_TEST_ROOT%/}"
  readonly DATA_DIR="$DEPLOY_ROOT/var/lib/vocat"
  readonly BACKUP_DIR="$DEPLOY_ROOT/var/backups/vocat"
  readonly PREFLIGHT_ROOT="$DEPLOY_ROOT/var/lib/vocat-preflight"
  readonly RELEASE_ROOT="$DEPLOY_ROOT/opt/vocat/releases"
  readonly CURRENT_LINK="$DEPLOY_ROOT/opt/vocat/current"
  readonly LOCK_DIR="$DEPLOY_ROOT/run/vocat-deploy"
  readonly LOCK_FILE="$LOCK_DIR/deploy.lock"
  readonly SERVICE_USER="${VOCAT_DEPLOY_TEST_USER:-$(id -un)}"
  readonly SERVICE_GROUP="${VOCAT_DEPLOY_TEST_GROUP:-$(id -gn)}"
  readonly PREFLIGHT_USER="${VOCAT_DEPLOY_TEST_PREFLIGHT_USER:-$(id -un)}"
  readonly PREFLIGHT_GROUP="${VOCAT_DEPLOY_TEST_PREFLIGHT_GROUP:-$(id -gn)}"
  readonly RELEASE_OWNER="$(id -un)"
  readonly RELEASE_GROUP="$(id -gn)"
  readonly BACKUP_OWNER="$(id -un)"
  readonly BACKUP_GROUP="$(id -gn)"
else
  readonly DATA_DIR="/var/lib/vocat"
  readonly BACKUP_DIR="/var/backups/vocat"
  readonly PREFLIGHT_ROOT="/var/lib/vocat-preflight"
  readonly RELEASE_ROOT="/opt/vocat/releases"
  readonly CURRENT_LINK="/opt/vocat/current"
  readonly LOCK_DIR="/run/vocat-deploy"
  readonly LOCK_FILE="$LOCK_DIR/deploy.lock"
  readonly SERVICE_USER="vocat"
  readonly SERVICE_GROUP="vocat"
  readonly PREFLIGHT_USER="vocat-preflight"
  readonly PREFLIGHT_GROUP="vocat-preflight"
  readonly RELEASE_OWNER="root"
  readonly RELEASE_GROUP="root"
  readonly BACKUP_OWNER="root"
  readonly BACKUP_GROUP="root"
fi
readonly SERVICE="vocat.service"
readonly EXPECTED_GO_IMAGE="golang:1.26.7-alpine3.23@sha256:b17af760035fc2f338eed92d448a6c67f2d45438844fc6c60678fa5f99e44b57"
readonly EXPECTED_GO_TEST_IMAGE="golang:1.26.7-bookworm@sha256:6ef6e30f0ea5c384f6d111cf856e024e3086bbdcb1779da3f3b3fbba0aea53d2"
readonly EXPECTED_NODE_IMAGE="node:24.15.0-alpine3.23@sha256:d1b3b4da11eefd5941e7f0b9cf17783fc99d9c6fc34884a665f40a06dbdfc94f"
readonly EXPECTED_JQ_IMAGE="ghcr.io/jqlang/jq:1.8.1@sha256:4f34c6d23f4b1372ac789752cc955dc67c2ae177eb1b5860b75cdc5091ce6f91"
readonly EXPECTED_GITLEAKS_IMAGE="zricethezav/gitleaks:v8.30.1@sha256:c00b6bd0aeb3071cbcb79009cb16a60dd9e0a7c60e2be9ab65d25e6bc8abbb7f"
readonly EXPECTED_SYFT_IMAGE="anchore/syft:v1.51.0@sha256:678bfa565b60f747aac0f8e964fe5588a24445b8d0a480e91f6efd70020dfbb0"

die() {
  echo "vocat-deploy: $*" >&2
  exit 1
}

if [ "$test_mode" = false ] && [ "$(id -u)" -ne 0 ]; then
  die "run as root"
fi
if [ "$#" -ne 5 ] || [ "$1" != "--expected-commit" ] || [ "$3" != "--expected-index-sha256" ]; then
  die "usage: $0 --expected-commit <40-hex-sha> --expected-index-sha256 <64-hex-sha256> /path/to/dist/hardened/<commit>"
fi

expected_commit="${2,,}"
expected_index_sha256="${4,,}"
input_artifact_dir="$5"
[[ "$expected_commit" =~ ^[0-9a-f]{40}$ ]] || die "expected commit is invalid"
[[ "$expected_index_sha256" =~ ^[0-9a-f]{64}$ ]] || die "expected artifact index SHA-256 is invalid"
[ -d "$input_artifact_dir" ] || die "artifact directory is missing"
input_artifact_dir="$(readlink -f -- "$input_artifact_dir")"
[ -f "$input_artifact_dir/manifest.json" ] || die "artifact manifest is missing"
[ -f "$input_artifact_dir/SHA256SUMS" ] || die "artifact checksum file is missing"

artifact_snapshot=""
cleanup_artifact_snapshot() {
  if [ -n "$artifact_snapshot" ] && [ -d "$artifact_snapshot" ]; then
    rm -rf -- "$artifact_snapshot"
  fi
}
trap cleanup_artifact_snapshot EXIT

artifact_dir=""
manifest="$artifact_dir/manifest.json"
sums="$artifact_dir/SHA256SUMS"

for command in awk cp curl find flock grep install jq mkdir rmdir runuser sha256sum sqlite3 ss stat systemctl systemd-run tr; do
  command -v "$command" >/dev/null 2>&1 || die "missing required command: $command"
done
if [ "$test_mode" = true ]; then
  [ -n "${VOCAT_DEPLOY_TEST_PROC_ROOT:-}" ] || die "test proc root is required in test mode"
  [[ "$VOCAT_DEPLOY_TEST_PROC_ROOT" = /* ]] || die "test proc root must be absolute"
  readonly PROC_ROOT="${VOCAT_DEPLOY_TEST_PROC_ROOT%/}"
else
  [ -z "${VOCAT_DEPLOY_TEST_PROC_ROOT:-}" ] || die "test proc root requires test mode"
  readonly PROC_ROOT="/proc"
fi
[ -d "$PROC_ROOT" ] && [ ! -L "$PROC_ROOT" ] || die "process metadata root is missing or unsafe"
readiness_attempts=30
if [ "$test_mode" = true ]; then
  readiness_attempts="${VOCAT_DEPLOY_TEST_READY_ATTEMPTS:-1}"
fi
[[ "$readiness_attempts" =~ ^[1-9][0-9]*$ ]] || die "invalid readiness attempt count"
readonly readiness_attempts
systemctl cat "$SERVICE" >/dev/null 2>&1 || die "vocat.service is not installed; prepare the guest first"
getent passwd "$SERVICE_USER" >/dev/null || die "service account is missing: $SERVICE_USER"
getent group "$SERVICE_GROUP" >/dev/null || die "service group is missing: $SERVICE_GROUP"
getent passwd "$PREFLIGHT_USER" >/dev/null || die "preflight account is missing: $PREFLIGHT_USER"
getent group "$PREFLIGHT_GROUP" >/dev/null || die "preflight group is missing: $PREFLIGHT_GROUP"

install -d -o "$RELEASE_OWNER" -g "$RELEASE_GROUP" -m 0755 "${RELEASE_ROOT%/releases}" "$RELEASE_ROOT"
install -d -o "$SERVICE_USER" -g "$SERVICE_GROUP" -m 0700 "$DATA_DIR"
install -d -o "$BACKUP_OWNER" -g "$BACKUP_GROUP" -m 0700 "$BACKUP_DIR"
install -d -o "$BACKUP_OWNER" -g "$PREFLIGHT_GROUP" -m 0710 "$PREFLIGHT_ROOT"
[ -d "$PREFLIGHT_ROOT" ] && [ ! -L "$PREFLIGHT_ROOT" ] || die "preflight root is unsafe"
[ "$(stat -c '%U:%G:%a' "$PREFLIGHT_ROOT")" = "$BACKUP_OWNER:$PREFLIGHT_GROUP:710" ] || die "preflight root has unsafe ownership or mode"
if [ "$test_mode" = true ]; then
  install -d -o "$BACKUP_OWNER" -g "$BACKUP_GROUP" -m 0755 "$(dirname "$LOCK_DIR")"
fi
if [ ! -e "$LOCK_DIR" ] && [ ! -L "$LOCK_DIR" ]; then
  mkdir -m 0700 -- "$LOCK_DIR" || die "cannot create the private deployment lock directory"
fi
[ -d "$LOCK_DIR" ] && [ ! -L "$LOCK_DIR" ] || die "deployment lock directory is unsafe"
[ "$(stat -c '%U:%G:%a' "$LOCK_DIR")" = "$BACKUP_OWNER:$BACKUP_GROUP:700" ] || die "deployment lock directory has unsafe ownership or mode"
if [ -e "$LOCK_FILE" ] || [ -L "$LOCK_FILE" ]; then
  [ -f "$LOCK_FILE" ] && [ ! -L "$LOCK_FILE" ] || die "deployment lock file is unsafe"
  [ "$(stat -c '%U:%G:%a' "$LOCK_FILE")" = "$BACKUP_OWNER:$BACKUP_GROUP:600" ] || die "deployment lock file has unsafe ownership or mode"
else
  install -o "$BACKUP_OWNER" -g "$BACKUP_GROUP" -m 0600 /dev/null "$LOCK_FILE"
fi
# The private directory prevents unprivileged users from replacing this file
# between validation and open. Keep descriptor 9 locked for the entire run.
exec 9<>"$LOCK_FILE"
flock -n 9 || die "another deployment is running"

# The caller-provided artifact may be writable by an unprivileged account.
# Copy it once into a root-only directory, then perform every trust decision on
# that immutable snapshot so validation and installation cannot see different
# bytes.
artifact_snapshot="$(mktemp -d "$RELEASE_ROOT/.incoming.XXXXXX")"
chmod 0700 "$artifact_snapshot"
if ! cp -R --no-dereference --no-preserve=all -- "$input_artifact_dir/." "$artifact_snapshot/"; then
  die "failed to snapshot the artifact directory"
fi
snapshot_unsafe_entry="$(find "$artifact_snapshot" -xdev -mindepth 1 ! -type d ! -type f -print -quit)"
[ -z "$snapshot_unsafe_entry" ] || die "artifact contains a symbolic link or special file"
find "$artifact_snapshot" -xdev -type d -exec chmod 0700 -- {} +
find "$artifact_snapshot" -xdev -type f -exec chmod 0600 -- {} +
snapshot_unsafe_metadata="$(find "$artifact_snapshot" -xdev \
  \( ! -user "$RELEASE_OWNER" -o ! -group "$RELEASE_GROUP" \
  -o \( -type d ! -perm 0700 \) -o \( -type f ! -perm 0600 \) \) \
  -print -quit)"
[ -z "$snapshot_unsafe_metadata" ] || die "artifact snapshot is not root-only"
artifact_dir="$artifact_snapshot"
manifest="$artifact_dir/manifest.json"
sums="$artifact_dir/SHA256SUMS"
[ -f "$manifest" ] && [ ! -L "$manifest" ] || die "missing or unsafe manifest.json"
[ -f "$sums" ] && [ ! -L "$sums" ] || die "missing or unsafe SHA256SUMS"
actual_index_sha256="$(sha256sum "$sums" | awk '{print $1}')"
[ "$actual_index_sha256" = "$expected_index_sha256" ] || die "artifact index SHA-256 does not match the reviewed out-of-band value"

declare -A checksum_by_path=()
while IFS= read -r checksum_line || [ -n "$checksum_line" ]; do
  if [[ "$checksum_line" =~ ^([0-9a-fA-F]{64})[[:space:]][[:space:]](\./[A-Za-z0-9][A-Za-z0-9._-]*(/[A-Za-z0-9][A-Za-z0-9._-]*)*)$ ]]; then
    checksum_sha="${BASH_REMATCH[1],,}"
    checksum_path="${BASH_REMATCH[2]}"
  else
    die "SHA256SUMS entry is malformed"
  fi
  case "/${checksum_path#./}/" in
    */../*|*/./*) die "SHA256SUMS contains an unsafe path" ;;
  esac
  [ -z "${checksum_by_path[$checksum_path]+present}" ] || die "SHA256SUMS contains a duplicate path"
  artifact_entry="$artifact_dir/${checksum_path#./}"
  [ -f "$artifact_entry" ] && [ ! -L "$artifact_entry" ] || die "SHA256SUMS names a missing or unsafe file"
  checksum_by_path["$checksum_path"]="$checksum_sha"
done < "$sums"

mapfile -d '' -t artifact_files < <(
  cd "$artifact_dir"
  find . -type f ! -path './SHA256SUMS' -print0 | sort -z
)
[ "${#artifact_files[@]}" -eq "${#checksum_by_path[@]}" ] || die "SHA256SUMS inventory does not match artifact contents"
for artifact_file in "${artifact_files[@]}"; do
  [ -n "${checksum_by_path[$artifact_file]+present}" ] || die "SHA256SUMS omits an artifact file"
done
[ -n "${checksum_by_path[./manifest.json]+present}" ] || die "SHA256SUMS does not cover manifest.json"
(
  cd "$artifact_dir"
  sha256sum --check --strict --quiet -- SHA256SUMS
) || die "artifact checksum verification failed"

schema="$(jq -er '.schema' "$manifest")"
source_type="$(jq -er '.source.type | select(type == "string")' "$manifest")"
commit="$(jq -er '.source.commit | select(type == "string")' "$manifest")"
binary_name="$(jq -er '.binary.file | select(type == "string")' "$manifest")"
target="$(jq -er '.target' "$manifest")"
go_version="$(jq -er '.toolchain.go.version | select(type == "string")' "$manifest")"
go_version_m_path="$(jq -er '.binary.go_version_m | select(type == "string")' "$manifest")"
manifest_sha="$(jq -er '.binary.sha256 | select(type == "string")' "$manifest" | tr '[:upper:]' '[:lower:]')"

[ "$schema" = "2" ] || die "manifest schema is unsupported"
[ "$source_type" = "git archive" ] || die "manifest source type is unsupported"
[[ "$commit" =~ ^[0-9a-f]{40}$ ]] || die "manifest commit is invalid"
[ "$commit" = "$expected_commit" ] || die "manifest commit does not match the reviewed commit"
[ "$binary_name" = "vocat-linux-amd64" ] || die "manifest binary must be vocat-linux-amd64"
[ "$target" = "linux/amd64" ] || die "artifact target must be linux/amd64"
[ "$go_version" = "go version go1.26.7 linux/amd64" ] || die "artifact was not built with Go 1.26.7 for linux/amd64"
[ "$go_version_m_path" = "reports/go-version-m.txt" ] || die "manifest binary Go metadata path is invalid"
[[ "$manifest_sha" =~ ^[0-9a-f]{64}$ ]] || die "manifest binary SHA-256 is invalid"
[ -f "$artifact_dir/$binary_name" ] && [ ! -L "$artifact_dir/$binary_name" ] || die "manifest binary is missing or unsafe"
[ -n "${checksum_by_path[./$binary_name]+present}" ] || die "SHA256SUMS does not cover the manifest binary"
[ "${checksum_by_path[./$binary_name]}" = "$manifest_sha" ] || die "SHA256SUMS does not match manifest binary SHA-256"

required_evidence=(
  go-version.txt
  node-version.txt
  npm-version.txt
  reports/gitleaks-version.txt
  reports/gitleaks.json
  reports/go-build.txt
  reports/go-mod-download.txt
  reports/go-mod-verify.txt
  reports/go-race-version.txt
  reports/go-test.txt
  reports/go-test-race.txt
  reports/go-test-version.txt
  reports/go-vet.txt
  reports/go-version-m.txt
  reports/govulncheck-version.txt
  reports/govulncheck-source.sarif.json
  reports/govulncheck-binary.sarif.json
  reports/jq-version.txt
  reports/npm-audit-full.json
  reports/npm-audit-full.stderr.txt
  reports/npm-audit-production.json
  reports/npm-audit-production.stderr.txt
  reports/npm-build.txt
  reports/npm-ci.txt
  reports/npm-test.txt
  reports/syft-version.json
  sbom/source.cdx.json
  sbom/binary.cdx.json
)
for evidence_path in "${required_evidence[@]}"; do
  [ -s "$artifact_dir/$evidence_path" ] && [ ! -L "$artifact_dir/$evidence_path" ] || die "required build evidence is missing or empty: $evidence_path"
done

for gate_spec in \
  'reports/npm-ci.txt|PASS: npm ci' \
  'reports/npm-ci.txt|PASS: npm lifecycle rebuild' \
  'reports/npm-audit-full.stderr.txt|PASS: npm audit (all dependencies, high+)' \
  'reports/npm-audit-production.stderr.txt|PASS: npm audit (production dependencies, high+)' \
  'reports/npm-build.txt|PASS: npm build' \
  'reports/npm-test.txt|PASS: npm test' \
  'reports/go-build.txt|PASS: static Go build' \
  'reports/go-mod-download.txt|PASS: go mod download' \
  'reports/go-mod-verify.txt|PASS: go mod verify' \
  'reports/go-test.txt|PASS: go test' \
  'reports/go-test-race.txt|PASS: go test -race' \
  'reports/go-vet.txt|PASS: go vet'; do
  IFS='|' read -r gate_path gate_marker <<< "$gate_spec"
  grep -Fqx "$gate_marker" "$artifact_dir/$gate_path" || die "build evidence does not record a passing gate: $gate_path"
done

single_line() {
  tr -d '\r' < "$1"
}

release_go_version="$(single_line "$artifact_dir/go-version.txt")"
test_go_version="$(single_line "$artifact_dir/reports/go-test-version.txt")"
race_go_version="$(single_line "$artifact_dir/reports/go-race-version.txt")"
node_version="$(single_line "$artifact_dir/node-version.txt")"
npm_version="$(single_line "$artifact_dir/npm-version.txt")"
jq_version="$(single_line "$artifact_dir/reports/jq-version.txt")"
gitleaks_version="$(single_line "$artifact_dir/reports/gitleaks-version.txt")"
[ "$release_go_version" = "go version go1.26.7 linux/amd64" ] || die "release Go tool evidence is invalid"
[ "$test_go_version" = "go version go1.26.7 linux/amd64" ] || die "test Go tool evidence is invalid"
[ "$race_go_version" = "go version go1.26.7 linux/amd64" ] || die "race Go tool evidence is invalid"
[ "$node_version" = "v24.15.0" ] || die "Node tool evidence is invalid"
[ "$npm_version" = "11.12.1" ] || die "npm tool evidence is invalid"
[ "$jq_version" = "jq-1.8.1" ] || die "jq tool evidence is invalid"
[ "$gitleaks_version" = "v8.30.1" ] || die "Gitleaks tool evidence is invalid"
grep -Fq 'Scanner: govulncheck@v1.7.0' "$artifact_dir/reports/govulncheck-version.txt" || die "govulncheck tool evidence is invalid"
jq -e -s 'length == 1 and .[0].version == "1.51.0"' \
  "$artifact_dir/reports/syft-version.json" >/dev/null || die "Syft tool evidence is invalid"

jq -e -s 'length == 1 and (.[0] | type == "array" and length == 0)' \
  "$artifact_dir/reports/gitleaks.json" >/dev/null || die "Gitleaks evidence contains findings or is malformed"

validate_npm_audit() {
  local report=$1
  jq -e -s '
    def safe_nat: type == "number" and . >= 0 and . <= 9007199254740991 and floor == .;
    length == 1 and (.[0] |
      .auditReportVersion == 2 and
      (.metadata.vulnerabilities as $v |
        ($v.info | safe_nat) and ($v.low | safe_nat) and
        ($v.moderate | safe_nat) and ($v.high | safe_nat) and
        ($v.critical | safe_nat) and ($v.total | safe_nat) and
        $v.total == ($v.info + $v.low + $v.moderate + $v.high + $v.critical) and
        $v.high == 0 and $v.critical == 0) and
      (.metadata.dependencies as $d |
        ($d.prod | safe_nat) and ($d.dev | safe_nat) and
        ($d.optional | safe_nat) and ($d.peer | safe_nat) and
        ($d.peerOptional | safe_nat) and ($d.total | safe_nat)))
  ' "$report" >/dev/null
}

validate_govulncheck() {
  local report=$1
  jq -e -s '
    length == 1 and (.[0] |
      .version == "2.1.0" and
      (.runs | type == "array" and length > 0) and
      all(.runs[]; .tool.driver.name == "govulncheck" and (.results | type == "array")) and
      all(.runs[].results[]?; .level == "error" or .level == "warning" or .level == "note") and
      ([.runs[].results[]? | select(.level == "error")] | length) == 0)
  ' "$report" >/dev/null
}

validate_sbom() {
  local report=$1
  jq -e -s '
    length == 1 and (.[0] |
      .bomFormat == "CycloneDX" and
      (.components | type == "array" and length > 0))
  ' "$report" >/dev/null
}

validate_npm_audit "$artifact_dir/reports/npm-audit-full.json" || die "full npm audit evidence is malformed or unsafe"
validate_npm_audit "$artifact_dir/reports/npm-audit-production.json" || die "production npm audit evidence is malformed or unsafe"
validate_govulncheck "$artifact_dir/reports/govulncheck-source.sarif.json" || die "source govulncheck evidence is malformed or reachable"
validate_govulncheck "$artifact_dir/reports/govulncheck-binary.sarif.json" || die "binary govulncheck evidence is malformed or reachable"
validate_sbom "$artifact_dir/sbom/source.cdx.json" || die "source SBOM evidence is malformed or empty"
validate_sbom "$artifact_dir/sbom/binary.cdx.json" || die "binary SBOM evidence is malformed or empty"

npm_full_high="$(jq -er '.metadata.vulnerabilities.high' "$artifact_dir/reports/npm-audit-full.json")"
npm_full_critical="$(jq -er '.metadata.vulnerabilities.critical' "$artifact_dir/reports/npm-audit-full.json")"
npm_production_high="$(jq -er '.metadata.vulnerabilities.high' "$artifact_dir/reports/npm-audit-production.json")"
npm_production_critical="$(jq -er '.metadata.vulnerabilities.critical' "$artifact_dir/reports/npm-audit-production.json")"
source_vuln_total="$(jq -er '[.runs[].results[]?] | length' "$artifact_dir/reports/govulncheck-source.sarif.json")"
binary_vuln_total="$(jq -er '[.runs[].results[]?] | length' "$artifact_dir/reports/govulncheck-binary.sarif.json")"
source_component_count="$(jq -er '.components | length' "$artifact_dir/sbom/source.cdx.json")"
binary_component_count="$(jq -er '.components | length' "$artifact_dir/sbom/binary.cdx.json")"
syft_version="$(jq -er '.version' "$artifact_dir/reports/syft-version.json")"

jq -e \
  --arg expectedVersion "hardened-${commit:0:12}" \
  --arg binarySHA "$manifest_sha" \
  --arg goImage "$EXPECTED_GO_IMAGE" \
  --arg goTestImage "$EXPECTED_GO_TEST_IMAGE" \
  --arg nodeImage "$EXPECTED_NODE_IMAGE" \
  --arg jqImage "$EXPECTED_JQ_IMAGE" \
  --arg gitleaksImage "$EXPECTED_GITLEAKS_IMAGE" \
  --arg syftImage "$EXPECTED_SYFT_IMAGE" \
  --arg releaseGoVersion "$release_go_version" \
  --arg testGoVersion "$test_go_version" \
  --arg raceGoVersion "$race_go_version" \
  --arg nodeVersion "$node_version" \
  --arg npmVersion "$npm_version" \
  --arg jqVersion "$jq_version" \
  --arg gitleaksVersion "$gitleaks_version" \
  --arg syftVersion "$syft_version" \
  --argjson npmFullHigh "$npm_full_high" \
  --argjson npmFullCritical "$npm_full_critical" \
  --argjson npmProductionHigh "$npm_production_high" \
  --argjson npmProductionCritical "$npm_production_critical" \
  --argjson sourceVulnTotal "$source_vuln_total" \
  --argjson binaryVulnTotal "$binary_vuln_total" \
  --argjson sourceComponentCount "$source_component_count" \
  --argjson binaryComponentCount "$binary_component_count" '
    def safe_nat: type == "number" and . >= 0 and . <= 9007199254740991 and floor == .;
    .schema == 2 and
    .source.type == "git archive" and
    (.source.frontend_dist.isolated_build == true) and
    (.source.frontend_dist.files | safe_nat) and .source.frontend_dist.files > 0 and
    (.source.frontend_dist.size_bytes | safe_nat) and .source.frontend_dist.size_bytes <= 52428800 and
    .target == "linux/amd64" and .build_platform == "linux/amd64" and
    .version == $expectedVersion and (.build_time | type == "string" and length > 0) and
    .binary == {file:"vocat-linux-amd64", sha256:$binarySHA, go_version_m:"reports/go-version-m.txt"} and
    .toolchain == {
      go:{image:$goImage, version:$releaseGoVersion},
      go_test:{image:$goTestImage, version:$testGoVersion},
      go_race:{image:$goTestImage, version:$raceGoVersion},
      node:{image:$nodeImage, version:$nodeVersion},
      npm:{version:$npmVersion},
      jq:{image:$jqImage, version:$jqVersion},
      govulncheck:{version:"v1.7.0", report:"reports/govulncheck-version.txt"},
      gitleaks:{image:$gitleaksImage, version:$gitleaksVersion},
      syft:{image:$syftImage, version:$syftVersion}
    } and
    .gates == {
      secret_scan:{status:"passed", findings:0, report:"reports/gitleaks.json"},
      npm_audit_full:{status:"passed", threshold:"high", high:$npmFullHigh, critical:$npmFullCritical, report:"reports/npm-audit-full.json"},
      npm_audit_production:{status:"passed", threshold:"high", high:$npmProductionHigh, critical:$npmProductionCritical, report:"reports/npm-audit-production.json"},
      npm_test:{status:"passed", report:"reports/npm-test.txt"},
      npm_build:{status:"passed", report:"reports/npm-build.txt"},
      go_mod_download:{status:"passed", report:"reports/go-mod-download.txt"},
      go_mod_verify:{status:"passed", report:"reports/go-mod-verify.txt"},
      go_test:{status:"passed", report:"reports/go-test.txt"},
      go_test_race:{status:"passed", report:"reports/go-test-race.txt"},
      go_vet:{status:"passed", report:"reports/go-vet.txt"},
      govulncheck_source:{status:"passed", scan:"symbol", reachable_findings:0, total_findings:$sourceVulnTotal, report:"reports/govulncheck-source.sarif.json"},
      govulncheck_binary:{status:"passed", scan:"symbol", reachable_findings:0, total_findings:$binaryVulnTotal, report:"reports/govulncheck-binary.sarif.json"}
    } and
    .sbom == {
      source:{format:"CycloneDX JSON", components:$sourceComponentCount, file:"sbom/source.cdx.json"},
      binary:{format:"CycloneDX JSON", components:$binaryComponentCount, file:"sbom/binary.cdx.json"}
    } and
    .integrity == {checksums:"SHA256SUMS", scope:"all regular artifact files except SHA256SUMS"}
  ' "$manifest" >/dev/null || die "manifest and build evidence are malformed or inconsistent"

binary_go_version="$(awk 'NR == 1 { print $NF; exit }' "$artifact_dir/$go_version_m_path")"
binary_goos="$(awk '$1 == "build" && $2 ~ /^GOOS=/ { sub(/^GOOS=/, "", $2); print $2 }' "$artifact_dir/$go_version_m_path")"
binary_goarch="$(awk '$1 == "build" && $2 ~ /^GOARCH=/ { sub(/^GOARCH=/, "", $2); print $2 }' "$artifact_dir/$go_version_m_path")"
[ "$binary_go_version" = "go1.26.7" ] && [ "$binary_goos" = "linux" ] && [ "$binary_goarch" = "amd64" ] || \
  die "binary Go metadata does not prove go1.26.7 linux/amd64"

release_dir="$RELEASE_ROOT/$commit"
validate_release_entry() {
  local entry=$1
  [ -f "$entry" ] && [ ! -L "$entry" ] || die "existing release is incomplete or contains a symlink"
  [ "$(stat -c '%U:%G' "$entry")" = "$RELEASE_OWNER:$RELEASE_GROUP" ] || die "existing release file has unsafe ownership"
  local entry_mode
  entry_mode="$(stat -c '%a' "$entry")"
  (( (8#$entry_mode & 0022) == 0 )) || die "existing release file is group/world writable"
}
if [ -L "$release_dir" ]; then
  die "release path must not be a symbolic link"
fi
if [ -e "$release_dir" ] && [ ! -d "$release_dir" ]; then
  die "release path exists but is not a directory"
fi
if [ -d "$release_dir" ]; then
  [ "$(stat -c '%U:%G' "$release_dir")" = "$RELEASE_OWNER:$RELEASE_GROUP" ] || die "existing release directory has unsafe ownership"
  mode="$(stat -c '%a' "$release_dir")"
  (( (8#$mode & 0022) == 0 )) || die "existing release directory is group/world writable"
  validate_release_entry "$release_dir/vocat"
  validate_release_entry "$release_dir/manifest.json"
  validate_release_entry "$release_dir/SHA256SUMS"
  cmp -s "$artifact_dir/$binary_name" "$release_dir/vocat" || die "existing release binary differs from artifact"
  cmp -s "$manifest" "$release_dir/manifest.json" || die "existing release manifest differs from artifact"
  cmp -s "$sums" "$release_dir/SHA256SUMS" || die "existing release checksums differ from artifact"
else
  release_staging="$(mktemp -d "$RELEASE_ROOT/.${commit}.XXXXXX")"
  if ! (
    chown "$RELEASE_OWNER:$RELEASE_GROUP" "$release_staging" &&
      chmod 0755 "$release_staging" &&
      install -o "$RELEASE_OWNER" -g "$RELEASE_GROUP" -m 0755 "$artifact_dir/$binary_name" "$release_staging/vocat" &&
      install -o "$RELEASE_OWNER" -g "$RELEASE_GROUP" -m 0644 "$manifest" "$sums" "$release_staging/" &&
      [ "$(sha256sum "$release_staging/vocat" | awk '{print $1}')" = "$manifest_sha" ] &&
      mv -T "$release_staging" "$release_dir"
  ); then
    rm -rf -- "$release_staging"
    die "failed to install immutable release"
  fi
fi

rm -rf -- "$artifact_snapshot"
artifact_snapshot=""

database="$DATA_DIR/vocat.db"
preflight_dir="$(mktemp -d "$PREFLIGHT_ROOT/run.XXXXXX")"
chown "$PREFLIGHT_USER:$PREFLIGHT_GROUP" "$preflight_dir"
chmod 0700 "$preflight_dir"
preflight_db="$preflight_dir/vocat.db"
previous_target=""
previous_link_exists=false
database_existed=false
rollback_db=""
rollback_staging=""
service_was_active=false
activation_started=false
database_may_have_changed=false
live_snapshot=""
rollback_workdir=""
restore_staging=""

validate_live_database_file() {
  local path=$1
  local label=$2
  local allow_absent=${3:-false}
  if [ ! -e "$path" ] && [ ! -L "$path" ]; then
    [ "$allow_absent" = true ] && return 0
    die "$label is missing"
  fi
  local metadata
  metadata="$(stat -c '%F:%U:%G:%a:%h' -- "$path")" || die "cannot inspect $label"
  [ "$metadata" = "regular file:$SERVICE_USER:$SERVICE_GROUP:600:1" ] || \
    die "$label has unsafe type, ownership, mode, or link count"
}

validate_live_database_state() {
  validate_live_database_file "$database" "live database"
  validate_live_database_file "$database-wal" "live database WAL" true
  validate_live_database_file "$database-shm" "live database SHM" true
  [ -s "$database" ] || die "database is empty; restore it or remove it before deployment"
}

snapshot_live_database() {
  local destination=$1
  local failure_message=$2
  live_snapshot="$(mktemp "$DATA_DIR/.vocat-snapshot.XXXXXX")"
  chown --no-dereference "$SERVICE_USER:$SERVICE_GROUP" "$live_snapshot"
  chmod 0600 "$live_snapshot"
  if ! runuser -u "$SERVICE_USER" -- sqlite3 "$database" ".backup '$live_snapshot'"; then
    die "$failure_message"
  fi
  mv -T -- "$live_snapshot" "$destination"
  live_snapshot=""
  local snapshot_metadata
  snapshot_metadata="$(stat -c '%F:%U:%G:%a:%h' -- "$destination")" || die "cannot inspect live database snapshot"
  [ "$snapshot_metadata" = "regular file:$SERVICE_USER:$SERVICE_GROUP:600:1" ] || \
    die "live database snapshot has unsafe type, ownership, mode, or link count"
}

purge_candidate_database_state() {
  # The candidate service has been stopped and its cgroup is gone. Remove only
  # the three fixed paths it could have replaced; --one-file-system prevents a
  # hostile directory from crossing into another mounted filesystem.
  rm -rf --one-file-system -- "$database" "$database-wal" "$database-shm"
  [ ! -e "$database" ] && [ ! -L "$database" ] || return 1
  [ ! -e "$database-wal" ] && [ ! -L "$database-wal" ] || return 1
  [ ! -e "$database-shm" ] && [ ! -L "$database-shm" ] || return 1
}

restore_live_database() {
  local backup=$1
  local backup_metadata
  backup_metadata="$(stat -c '%F:%U:%G:%a:%h' -- "$backup")" || return 1
  [ "$backup_metadata" = "regular file:$BACKUP_OWNER:$BACKUP_GROUP:600:1" ] || return 1
  purge_candidate_database_state || return 1
  restore_staging="$(mktemp "$DATA_DIR/.vocat-restore.XXXXXX")" || return 1
  install -o "$SERVICE_USER" -g "$SERVICE_GROUP" -m 0600 "$backup" "$restore_staging" || return 1
  local restore_metadata
  restore_metadata="$(stat -c '%F:%U:%G:%a:%h' -- "$restore_staging")" || return 1
  [ "$restore_metadata" = "regular file:$SERVICE_USER:$SERVICE_GROUP:600:1" ] || return 1
  mv -Tf -- "$restore_staging" "$database" || return 1
  restore_staging=""
}

release_check_error=""
release_check_pid=""
inspect_release_process() {
  local expected_release=$1
  local label=$2
  local expected_pid=${3:-}
  local state pid executable listener_output listener_line listener_remaining
  local line_has_pid

  release_check_error=""
  release_check_pid=""
  state="$(systemctl show --property=ActiveState --value "$SERVICE")" || {
    release_check_error="cannot determine $label service state"
    return 1
  }
  if [ "$state" != "active" ]; then
    release_check_error="$label did not remain active after readiness"
    return 1
  fi
  pid="$(systemctl show --property=MainPID --value "$SERVICE")" || {
    release_check_error="cannot determine $label main PID"
    return 1
  }
  if ! [[ "$pid" =~ ^[1-9][0-9]*$ ]]; then
    release_check_error="$label has no live main process after readiness"
    return 1
  fi
  if [ -n "$expected_pid" ] && [ "$pid" != "$expected_pid" ]; then
    release_check_error="$label process changed during readiness request"
    return 1
  fi
  executable="$(readlink -f -- "$PROC_ROOT/$pid/exe")" || {
    release_check_error="cannot verify $label process executable"
    return 1
  }
  if [ "$executable" != "$expected_release/vocat" ]; then
    release_check_error="$label main process is not running the activated release"
    return 1
  fi
  listener_output="$(ss -H -ltnp 'sport = :7575')" || {
    release_check_error="cannot inspect the $label readiness listener"
    return 1
  }
  if [ -z "$listener_output" ]; then
    release_check_error="$label has no TCP listener on port 7575"
    return 1
  fi
  while IFS= read -r listener_line; do
    listener_remaining=$listener_line
    line_has_pid=false
    while [[ "$listener_remaining" =~ pid=([0-9]+),fd= ]]; do
      line_has_pid=true
      if [ "${BASH_REMATCH[1]}" != "$pid" ]; then
        release_check_error="port 7575 listener is not owned exclusively by the $label process"
        return 1
      fi
      listener_remaining="${listener_remaining#*"${BASH_REMATCH[0]}"}"
    done
    if [ "$line_has_pid" != true ]; then
      release_check_error="cannot attribute the port 7575 listener to the $label process"
      return 1
    fi
  done <<<"$listener_output"
  release_check_pid=$pid
}

verify_release_readiness() {
  local expected_release=$1
  local label=$2
  local attempt request_pid

  release_check_error="$label failed readiness"
  for attempt in $(seq 1 "$readiness_attempts"); do
    if ! inspect_release_process "$expected_release" "$label"; then
      [ "$attempt" -lt "$readiness_attempts" ] && sleep 1
      continue
    fi
    request_pid=$release_check_pid
    release_check_error="$label failed readiness"
    if curl --noproxy '*' --fail --silent --show-error --max-time 2 http://127.0.0.1:7575/readyz >/dev/null; then
      # Accept the response only if the same expected binary still owns every
      # matching listener after the request.
      inspect_release_process "$expected_release" "$label" "$request_pid" || return 1
      return 0
    fi
    [ "$attempt" -lt "$readiness_attempts" ] && sleep 1
  done
  return 1
}

stop_managed_service() {
  if ! systemctl stop "$SERVICE" >/dev/null 2>&1 ||
    ! systemctl show --property=ActiveState --value "$SERVICE" 2>/dev/null | grep -Eq '^(inactive|failed)$'; then
    systemctl kill --kill-who=all --signal=SIGKILL "$SERVICE" >/dev/null 2>&1
    systemctl stop "$SERVICE" >/dev/null 2>&1
  fi
  systemctl show --property=ActiveState --value "$SERVICE" 2>/dev/null | grep -Eq '^(inactive|failed)$'
}

rollback() {
  set +e
  local rollback_failed=false
  local rollback_readiness_error=""
  echo "vocat-deploy: activation failed; restoring previous release" >&2
  if ! stop_managed_service; then
    echo "vocat-deploy: rollback could not stop candidate; database was not overwritten" >&2
    return 1
  fi
  if [ "$database_may_have_changed" = true ]; then
    if [ "$database_existed" = true ]; then
      if [ -z "$rollback_db" ] || [ ! -s "$rollback_db" ]; then
        echo "vocat-deploy: no validated database backup is available; refusing to overwrite the live database" >&2
        rollback_failed=true
      elif ! restore_live_database "$rollback_db"; then
        echo "vocat-deploy: failed to restore the validated database backup" >&2
        rollback_failed=true
      fi
    elif ! purge_candidate_database_state; then
      echo "vocat-deploy: failed to remove the candidate database" >&2
      rollback_failed=true
    fi
  fi
  rm -f -- "$CURRENT_LINK.new"
  if [ "$previous_link_exists" = true ]; then
    if ! ln -s "$previous_target" "$CURRENT_LINK.new" || ! mv -Tf "$CURRENT_LINK.new" "$CURRENT_LINK"; then
      echo "vocat-deploy: failed to restore the previous release link" >&2
      rollback_failed=true
    fi
  else
    rm -f -- "$CURRENT_LINK" || rollback_failed=true
  fi
  if [ "$service_was_active" = true ]; then
    if [ "$previous_link_exists" != true ]; then
      echo "vocat-deploy: no previous release is available to restart" >&2
      rollback_failed=true
    elif [ "$rollback_failed" = false ]; then
      systemctl start "$SERVICE" >/dev/null 2>&1 || {
        echo "vocat-deploy: rollback could not restart previous service" >&2
        rollback_failed=true
      }
      if [ "$rollback_failed" = false ] && ! verify_release_readiness "$previous_target" "previous service"; then
        rollback_readiness_error=$release_check_error
        echo "vocat-deploy: rollback readiness verification failed: $rollback_readiness_error" >&2
        if ! stop_managed_service; then
          echo "vocat-deploy: unverified previous service could not be stopped; isolate the VM immediately" >&2
        fi
        rollback_failed=true
      fi
    else
      echo "vocat-deploy: previous service was not restarted because rollback is incomplete" >&2
    fi
  fi
  [ "$rollback_failed" = false ]
}

cleanup() {
  status=$?
  trap - EXIT
  if [ "$status" -ne 0 ] && [ "$activation_started" = true ]; then
    rollback || status=1
  fi
  if [ -n "$rollback_staging" ]; then
    rm -f -- "$rollback_staging"
  fi
  if [ -n "$rollback_workdir" ]; then
    rm -rf -- "$rollback_workdir"
  fi
  if [ -n "$live_snapshot" ]; then
    rm -f -- "$live_snapshot"
  fi
  if [ -n "$restore_staging" ]; then
    rm -f -- "$restore_staging"
  fi
  if [ -n "$artifact_snapshot" ] && [ -d "$artifact_snapshot" ]; then
    rm -rf -- "$artifact_snapshot"
  fi
  rm -rf -- "$preflight_dir"
  exit "$status"
}
trap cleanup EXIT

run_candidate_preflight() {
  local password=$1
  # /run is replaced by the private TemporaryFileSystem below, so the
  # deployment lock path must not be listed as an inaccessible path here.
  printf '%s\n' "$password" | systemd-run \
    --quiet \
    --wait \
    --pipe \
    --collect \
    --service-type=exec \
    --property="User=$PREFLIGHT_USER" \
    --property="Group=$PREFLIGHT_GROUP" \
    --property="WorkingDirectory=$preflight_dir" \
    --property="ReadWritePaths=$preflight_dir" \
    --property="InaccessiblePaths=$DATA_DIR $BACKUP_DIR" \
    --property="TemporaryFileSystem=/run:rw,nosuid,nodev,noexec,mode=0700" \
    --property=PrivateNetwork=yes \
    --property=PrivateIPC=yes \
    --property=PrivateDevices=yes \
    --property=PrivateTmp=yes \
    --property=PrivateMounts=yes \
    --property=ProtectSystem=strict \
    --property=ProtectHome=yes \
    --property=ProtectProc=invisible \
    --property=ProtectClock=yes \
    --property=ProtectHostname=yes \
    --property=ProtectKernelLogs=yes \
    --property=ProtectKernelModules=yes \
    --property=ProtectKernelTunables=yes \
    --property=ProtectControlGroups=yes \
    --property=NoNewPrivileges=yes \
    --property=CapabilityBoundingSet= \
    --property=RestrictAddressFamilies=AF_UNIX \
    --property=RestrictSUIDSGID=yes \
    --property=LockPersonality=yes \
    --property=MemoryDenyWriteExecute=yes \
    --property=RemoveIPC=yes \
    --property=SystemCallArchitectures=native \
    --property=RuntimeMaxSec=2min \
    --property=TimeoutStopSec=15s \
    --property=MemoryHigh=384M \
    --property=MemoryMax=512M \
    --property=MemorySwapMax=0 \
    --property=TasksMax=64 \
    --property=LimitNOFILE=1024 \
    --property=LimitNPROC=64 \
    --property=LimitCORE=0 \
    --property=CPUQuota=200% \
    --property=UMask=0077 \
    -- \
    "$release_dir/vocat" bootstrap-admin --database "$preflight_db" --username admin \
    >/dev/null 2>&1
}

seal_preflight_output() {
  # The transient service has exited. Remove traversal rights from its account
  # before inspecting or opening anything it produced.
  [ -d "$preflight_dir" ] && [ ! -L "$preflight_dir" ] || die "candidate preflight directory is unsafe"
  chown --no-dereference "$BACKUP_OWNER:$BACKUP_GROUP" "$preflight_dir"
  chmod 0700 "$preflight_dir"

  local entries=()
  mapfile -d '' -t entries < <(find "$preflight_dir" -xdev -mindepth 1 -maxdepth 1 -printf '%f\0')
  [ "${#entries[@]}" -eq 1 ] && [ "${entries[0]}" = "vocat.db" ] || die "candidate preflight produced unexpected output"

  local output_metadata
  output_metadata="$(stat -c '%F:%U:%G:%a:%h' -- "$preflight_db")" || die "cannot lstat candidate database"
  [ "$output_metadata" = "regular file:$PREFLIGHT_USER:$PREFLIGHT_GROUP:600:1" ] || die "candidate database has unsafe type, ownership, mode, or link count"

  chown --no-dereference "$BACKUP_OWNER:$BACKUP_GROUP" "$preflight_db"
  chmod 0600 "$preflight_db"
  output_metadata="$(stat -c '%F:%U:%G:%a:%h' -- "$preflight_db")" || die "cannot verify sealed candidate database"
  [ "$output_metadata" = "regular file:$BACKUP_OWNER:$BACKUP_GROUP:600:1" ] || die "candidate database could not be sealed for root-only validation"
}

if [ -e "$database" ] || [ -L "$database" ]; then
  validate_live_database_state
  database_existed=true
  snapshot_live_database "$preflight_db" "live database snapshot failed"
  chown "$PREFLIGHT_USER:$PREFLIGHT_GROUP" "$preflight_db"
  chmod 0600 "$preflight_db"
  probe_password="$(od -An -N24 -tx1 /dev/urandom | tr -d ' \n')"
  run_candidate_preflight "$probe_password" || die "candidate database preflight failed"
  unset probe_password
else
  if [ "$test_mode" = true ] && [ -n "${VOCAT_DEPLOY_BOOTSTRAP_PASSWORD:-}" ]; then
    admin_password="$VOCAT_DEPLOY_BOOTSTRAP_PASSWORD"
  else
    [ -t 0 ] || die "initial administrator bootstrap requires a private TTY"
    read -r -s -p "Initial VoCat administrator password: " admin_password
    echo
    read -r -s -p "Confirm password: " admin_password_confirm
    echo
    [ "$admin_password" = "$admin_password_confirm" ] || die "passwords do not match"
    unset admin_password_confirm
  fi
  run_candidate_preflight "$admin_password" || die "candidate database preflight failed"
  unset admin_password
fi
seal_preflight_output
[ "$(sqlite3 "$preflight_db" 'PRAGMA quick_check;')" = "ok" ] || die "candidate migration failed SQLite quick_check"

if [ -e "$CURRENT_LINK" ] && [ ! -L "$CURRENT_LINK" ]; then
  die "current path exists but is not a symbolic link"
fi
if [ -L "$CURRENT_LINK" ]; then
  previous_target="$(readlink -f -- "$CURRENT_LINK")" || die "current release link is broken"
  case "$previous_target" in
    "$RELEASE_ROOT"/*) ;;
    *) die "current release points outside the immutable release root" ;;
  esac
  previous_link_exists=true
fi
service_state="$(systemctl show --property=ActiveState --value "$SERVICE")" || die "cannot determine current service state"
case "$service_state" in
  active|activating|reloading|deactivating)
    service_was_active=true
    # A failed stop can still leave the old unit inactive or half-stopped. From
    # the first stop request onward every error must attempt to restore it.
    activation_started=true
    systemctl stop "$SERVICE" || die "failed to stop current service; refusing to activate candidate"
    stopped_state="$(systemctl show --property=ActiveState --value "$SERVICE")" || die "cannot verify that the current service stopped"
    case "$stopped_state" in
      inactive|failed) ;;
      *) die "current service did not reach an inactive state; refusing to activate candidate" ;;
    esac
    ;;
  inactive|failed) ;;
  *) die "unexpected current service state: $service_state" ;;
esac

# An already-inactive service has nothing to restart, but failures from this
# point must still restore the release link and any candidate database state.
activation_started=true

if [ "$database_existed" = true ]; then
  validate_live_database_state
  rollback_workdir="$(mktemp -d "$BACKUP_DIR/.pre-${commit}.XXXXXX")"
  chmod 0700 "$rollback_workdir"
  rollback_staging="$rollback_workdir/vocat.db"
  snapshot_live_database "$rollback_staging" "rollback database backup failed"
  chown --no-dereference "$BACKUP_OWNER:$BACKUP_GROUP" "$rollback_staging"
  chmod 0600 "$rollback_staging"
  [ "$(sqlite3 "$rollback_staging" 'PRAGMA quick_check;')" = "ok" ] || die "rollback database backup failed SQLite quick_check"
  rollback_suffix="${rollback_workdir##*.}"
  rollback_db="$BACKUP_DIR/pre-${commit}-$(date -u +%Y%m%dT%H%M%SZ)-${rollback_suffix}.db"
  mv -T -- "$rollback_staging" "$rollback_db"
  rollback_staging=""
  rmdir -- "$rollback_workdir"
  rollback_workdir=""
fi

if [ "$database_existed" = false ]; then
  database_may_have_changed=true
  install -o "$SERVICE_USER" -g "$SERVICE_GROUP" -m 0600 "$preflight_db" "$database"
fi
rm -f -- "$CURRENT_LINK.new"
ln -s "$release_dir" "$CURRENT_LINK.new"
mv -Tf "$CURRENT_LINK.new" "$CURRENT_LINK"

systemctl daemon-reload
database_may_have_changed=true
systemctl start "$SERVICE"
verify_release_readiness "$release_dir" "candidate" || die "$release_check_error"

echo "deployed commit $commit"
echo "service: active"
