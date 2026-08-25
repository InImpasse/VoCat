#!/usr/bin/env bash
set -Eeuo pipefail
export LC_ALL=C
umask 077

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
readonly SCRIPT_DIR
REPO_ROOT=$(cd -- "$SCRIPT_DIR/.." && pwd)
readonly REPO_ROOT
readonly LEGACY_TAILSCALE_KEYRING=/usr/share/keyrings/tailscale-archive-keyring.gpg
readonly LEGACY_TAILSCALE_LIST=/etc/apt/sources.list.d/tailscale.list
readonly FIREWALL_TEMPLATE=$REPO_ROOT/deploy/vocat-firewall.nft.in
readonly FIREWALL_UNIT_SOURCE=$REPO_ROOT/deploy/vocat-firewall.service
readonly VOCAT_UNIT_SOURCE=$REPO_ROOT/deploy/vocat.service
readonly DJI_REPAIR_UNIT_SOURCE=$REPO_ROOT/deploy/vocat-dji-repair@.service
readonly DJI_RULES_SOURCE=$REPO_ROOT/deploy/99-vocat-dji.rules
readonly NFTABLES_MAIN=/etc/nftables.conf
readonly NFTABLES_INCLUDE='include "/etc/vocat/vocat-firewall.nft"'

mode=check
proxy_source_ipv4=
FIREWALL_SOURCE=
tmp_dir=
firewall_staging=
nftables_staging=
firewall_transaction_active=false
firewall_target_existed=false

usage() {
  cat <<'EOF'
Usage: prepare-vocat-guest.sh [--check | --dry-run | --apply] \
       --proxy-source-ipv4 ADDRESS

Prepare an installed Ubuntu Server 24.04 VoCat guest. The default --check is
read-only. --apply must run as root from a private console and will:

  - install qemu-guest-agent, libqmi tools, deployment dependencies, and nftables;
  - mask ModemManager so it cannot claim the passed-through modem;
  - allow TCP/7575 only from local loopback or the supplied host-proxy IPv4 address;
  - remove legacy Tailscale package and repository state;
  - install least-privilege udev access for the supported DJI USB device.
EOF
}

log() {
  printf '%s\n' "$*"
}

die() {
  printf 'ERROR: %s\n' "$*" >&2
  exit 1
}

validate_vocat_account() {
  local account_record
  local account_fields=()
  account_record=$(getent passwd vocat) || die 'vocat service account is missing'
  IFS=: read -r -a account_fields <<<"$account_record"
  ((${#account_fields[@]} == 7)) || die 'vocat service account record is malformed'
  [[ ${account_fields[0]} == vocat && ${account_fields[2]} =~ ^[0-9]+$ && ${account_fields[2]} != 0 ]] || die 'vocat service account identity is invalid'
  [[ $(id -gn vocat) == vocat ]] || die 'vocat service account has an unexpected primary group'
  [[ ${account_fields[5]} == /var/lib/vocat && ${account_fields[6]} == /usr/sbin/nologin ]] || die 'vocat service account home or shell is unexpected'
}

validate_preflight_account() {
  local account_record
  local account_fields=()
  account_record=$(getent passwd vocat-preflight) || die 'vocat-preflight account is missing'
  IFS=: read -r -a account_fields <<<"$account_record"
  ((${#account_fields[@]} == 7)) || die 'vocat-preflight account record is malformed'
  [[ ${account_fields[0]} == vocat-preflight && ${account_fields[2]} =~ ^[0-9]+$ && ${account_fields[2]} != 0 ]] || die 'vocat-preflight account identity is invalid'
  [[ $(id -gn vocat-preflight) == vocat-preflight ]] || die 'vocat-preflight account has an unexpected primary group'
  [[ ${account_fields[5]} == /nonexistent && ${account_fields[6]} == /usr/sbin/nologin ]] || die 'vocat-preflight account home or shell is unexpected'
  [[ $(id -nG vocat-preflight) == vocat-preflight ]] || die 'vocat-preflight account must not have supplementary groups'
}

assert_installed_file() {
  local source=$1
  local target=$2
  local expected_mode=$3
  [[ -f $target && ! -L $target ]] || die "installed file is missing or unsafe: $target"
  [[ $(stat -c '%U:%G:%a' "$target") == "root:root:$expected_mode" ]] || die "installed file has unsafe ownership or mode: $target"
  cmp -s -- "$source" "$target" || die "installed file differs from reviewed source: $target"
}

assert_root_file_metadata() {
  local target=$1
  local expected_mode=$2
  [[ -f $target && ! -L $target ]] || die "installed file is missing or unsafe: $target"
  [[ $(stat -c '%U:%G:%a' "$target") == "root:root:$expected_mode" ]] || die "installed file has unsafe ownership or mode: $target"
}

assert_root_file_secure() {
  local target=$1
  local mode
  [[ -f $target && ! -L $target ]] || die "installed file is missing or unsafe: $target"
  [[ $(stat -c '%U:%G' "$target") == root:root ]] || die "installed file has unsafe ownership: $target"
  mode=$(stat -c '%a' "$target")
  (( (8#$mode & 0022) == 0 )) || die "installed file is group/world writable: $target"
}

assert_unit_profile() {
  local unit=$1
  local expected_fragment=$2
  local fragment dropins
  fragment=$(systemctl show --property=FragmentPath --value "$unit") || die "cannot inspect unit fragment: $unit"
  [[ $fragment == "$expected_fragment" ]] || die "unit uses an unexpected fragment: $unit"
  dropins=$(systemctl show --property=DropInPaths --value "$unit") || die "cannot inspect unit drop-ins: $unit"
  [[ -z $dropins ]] || die "unit drop-ins are forbidden by the fixed guest profile: $unit"
}

validate_live_firewall() {
  local live_json
  live_json=$(nft --json list chain inet vocat_ingress input) || die 'cannot inspect the live VoCat firewall chain'
  jq -e --arg proxy_source_ipv4 "$proxy_source_ipv4" '
    def equals($left; $right):
      {"match": {"op": "==", "left": $left, "right": $right}};
    def interface($name):
      equals({"meta": {"key": "iifname"}}; $name);
    def tcp_7575:
      equals({"payload": {"protocol": "tcp", "field": "dport"}}; 7575);
    def proxy_source:
      equals({"payload": {"protocol": "ip", "field": "saddr"}}; $proxy_source_ipv4);
    def accept: {"accept": null};
    def reject_reset: {"reject": {"type": "tcp reset"}};

    [.nftables[] | select(has("chain")) | .chain] as $chains |
    [.nftables[] | select(has("rule")) | .rule] as $rules |
    ($chains | length) == 1 and
    $chains[0].family == "inet" and
    $chains[0].table == "vocat_ingress" and
    $chains[0].name == "input" and
    $chains[0].type == "filter" and
    $chains[0].hook == "input" and
    $chains[0].prio == -5 and
    $chains[0].policy == "accept" and
    ($rules | length) == 3 and
    ($rules | all(.family == "inet" and .table == "vocat_ingress" and .chain == "input")) and
    $rules[0].comment == "vocat-policy-v3-loopback" and
    $rules[0].expr == [interface("lo"), tcp_7575, accept] and
    $rules[1].comment == "vocat-policy-v3-host-proxy" and
    $rules[1].expr == [proxy_source, tcp_7575, accept] and
    $rules[2].comment == "vocat-policy-v3-default-reject" and
    $rules[2].expr == [tcp_7575, reject_reset]
  ' <<<"$live_json" >/dev/null || die 'live VoCat firewall semantics differ from the reviewed policy'
}

validate_guest_state() {
  local include_count modem_state service_name
  for service_name in nftables.service vocat-firewall.service; do
    systemctl is-enabled --quiet "$service_name" || die "required service is not enabled: $service_name"
    systemctl is-active --quiet "$service_name" || die "required service is not active: $service_name"
  done
  systemctl is-active --quiet qemu-guest-agent.service || die 'qemu-guest-agent.service is not active'
  [[ $(systemctl is-enabled ModemManager.service 2>/dev/null) == masked ]] || die 'ModemManager is not masked'
  modem_state=$(systemctl show --property=ActiveState --value ModemManager.service 2>/dev/null || true)
  [[ $modem_state == inactive ]] || die 'ModemManager is not inactive'

  ! dpkg-query -W -f='${Status}\n' tailscale 2>/dev/null | grep -Fxq 'install ok installed' ||
    die 'Tailscale package must not be installed'
  ! systemctl is-active --quiet tailscaled.service || die 'tailscaled.service must not be active'
  [[ ! -e $LEGACY_TAILSCALE_KEYRING && ! -L $LEGACY_TAILSCALE_KEYRING ]] ||
    die 'legacy Tailscale repository keyring is present'
  [[ ! -e $LEGACY_TAILSCALE_LIST && ! -L $LEGACY_TAILSCALE_LIST ]] ||
    die 'legacy Tailscale repository source is present'
  ! ip link show dev tailscale0 >/dev/null 2>&1 || die 'tailscale0 must not exist in the fixed guest profile'
  assert_root_file_metadata "$NFTABLES_MAIN" 644
  include_count=$(grep -Fxc "$NFTABLES_INCLUDE" "$NFTABLES_MAIN" || true)
  [[ $include_count == 1 ]] || die 'persistent nftables ruleset must contain exactly one VoCat include'
  assert_installed_file "$FIREWALL_SOURCE" /etc/vocat/vocat-firewall.nft 600
  assert_installed_file "$FIREWALL_UNIT_SOURCE" /etc/systemd/system/vocat-firewall.service 644
  assert_installed_file "$VOCAT_UNIT_SOURCE" /etc/systemd/system/vocat.service 644
  assert_installed_file "$DJI_REPAIR_UNIT_SOURCE" /etc/systemd/system/vocat-dji-repair@.service 644
  [[ ! -e /etc/systemd/system/vocat-dji-repair.service && ! -L /etc/systemd/system/vocat-dji-repair.service ]] ||
    die 'legacy manual-only DJI repair unit is present'
  assert_installed_file "$DJI_RULES_SOURCE" /etc/udev/rules.d/99-vocat-dji.rules 644
  assert_unit_profile vocat-firewall.service /etc/systemd/system/vocat-firewall.service
  assert_unit_profile vocat.service /etc/systemd/system/vocat.service
  assert_unit_profile vocat-dji-repair@probe.service /etc/systemd/system/vocat-dji-repair@.service
  nft --check --file "$NFTABLES_MAIN" || die 'persistent nftables ruleset does not validate'
  nft list table inet vocat_ingress >/dev/null || die 'VoCat nftables table is missing'
  validate_live_firewall

  getent group vocat-modem >/dev/null || die 'vocat-modem group is missing'
  getent group vocat >/dev/null || die 'vocat service group is missing'
  getent group vocat-preflight >/dev/null || die 'vocat-preflight group is missing'
  validate_vocat_account
  validate_preflight_account
  id -nG vocat | tr ' ' '\n' | grep -Fxq vocat-modem || die 'vocat service account lacks the modem group'
  systemctl is-enabled --quiet vocat.service || die 'vocat.service is not enabled'
}

shell_join() {
  printf '%q ' "$@"
  printf '\n'
}

run() {
  if [[ $mode == dry-run ]]; then
    printf '+ '
    shell_join "$@"
    return 0
  fi
  "$@"
}

rollback_firewall_transaction() {
  set +e
  log 'ERROR: restoring the previous persistent VoCat firewall configuration.' >&2
  install -o root -g root -m 0644 "$tmp_dir/nftables.conf.previous" "$NFTABLES_MAIN" ||
    log 'ERROR: could not restore the previous nftables main configuration.' >&2
  if [[ $firewall_target_existed == true ]]; then
    install -o root -g root -m 0600 "$tmp_dir/vocat-firewall.nft.previous" /etc/vocat/vocat-firewall.nft ||
      log 'ERROR: could not restore the previous VoCat firewall include.' >&2
    if nft --check --file /etc/vocat/vocat-firewall.nft >/dev/null 2>&1; then
      nft --file /etc/vocat/vocat-firewall.nft >/dev/null 2>&1 ||
        log 'ERROR: could not restore the previous live VoCat firewall table.' >&2
    fi
  else
    rm -f -- /etc/vocat/vocat-firewall.nft
    nft delete table inet vocat_ingress >/dev/null 2>&1 || true
  fi
  firewall_transaction_active=false
}

cleanup() {
  local status=$?
  trap - EXIT
  if [[ $firewall_transaction_active == true ]]; then
    rollback_firewall_transaction
    status=1
  fi
  if [[ -n $firewall_staging ]]; then
    rm -f -- "$firewall_staging"
  fi
  if [[ -n $nftables_staging ]]; then
    rm -f -- "$nftables_staging"
  fi
  if [[ -n $tmp_dir && -d $tmp_dir ]]; then
    rm -rf -- "$tmp_dir"
  fi
  exit "$status"
}
trap cleanup EXIT

while (($#)); do
  case "$1" in
    --check|--dry-run|--apply)
      mode=${1#--}
      shift
      ;;
    --proxy-source-ipv4)
      (($# >= 2)) || die '--proxy-source-ipv4 requires a value'
      proxy_source_ipv4=$2
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      die "unknown argument: $1"
      ;;
  esac
done

proxy_source_ipv4=$(python3 - "$proxy_source_ipv4" <<'PY'
import ipaddress
import sys

try:
    address = ipaddress.ip_address(sys.argv[1])
except ValueError as exc:
    raise SystemExit(f"ERROR: invalid --proxy-source-ipv4: {exc}")
if address.version != 4:
    raise SystemExit("ERROR: --proxy-source-ipv4 must be IPv4")
private_networks = tuple(ipaddress.ip_network(network) for network in ("10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"))
if not any(address in network for network in private_networks):
    raise SystemExit("ERROR: --proxy-source-ipv4 must be a usable private IPv4 address")
print(address)
PY
)
[[ -n $proxy_source_ipv4 ]] || die '--proxy-source-ipv4 validation failed'

[[ -r /etc/os-release ]] || die '/etc/os-release is unavailable'
# shellcheck disable=SC1091
source /etc/os-release
[[ ${ID:-} == ubuntu && ${VERSION_ID:-} == 24.04 ]] || die 'guest must be Ubuntu 24.04'
[[ $(uname -m) == x86_64 ]] || die 'guest must be amd64/x86_64'

for source_file in "$FIREWALL_TEMPLATE" "$FIREWALL_UNIT_SOURCE" "$VOCAT_UNIT_SOURCE" "$DJI_REPAIR_UNIT_SOURCE" "$DJI_RULES_SOURCE"; do
  [[ -f $source_file && ! -L $source_file && -r $source_file ]] || die "deployment source is missing or unsafe: $source_file"
done

tmp_dir=$(mktemp -d)
FIREWALL_SOURCE=$tmp_dir/vocat-firewall.nft
firewall_contents=$(<"$FIREWALL_TEMPLATE")
firewall_contents=${firewall_contents//@PROXY_SOURCE_IPV4@/$proxy_source_ipv4}
printf '%s\n' "$firewall_contents" >"$FIREWALL_SOURCE"

if [[ $mode == dry-run ]]; then
  log 'DRY RUN: would remove any legacy Tailscale package, service, repository, and interface state.'
  run systemctl disable --now tailscaled.service
  run apt-get purge -y tailscale
  run rm -f -- "$LEGACY_TAILSCALE_KEYRING" "$LEGACY_TAILSCALE_LIST"
  run apt-get update
  run apt-get install -y --no-install-recommends ca-certificates curl iproute2 jq libqmi-utils nftables python3 qemu-guest-agent sqlite3 usbutils
  if systemctl list-unit-files ModemManager.service --no-legend 2>/dev/null | grep -q '^ModemManager.service'; then
    run systemctl disable --now ModemManager.service
  else
    log 'DRY RUN: ModemManager.service is not installed; no running service would need to be stopped.'
  fi
  run systemctl mask ModemManager.service
  for group_name in vocat-modem vocat vocat-preflight; do
    if getent group "$group_name" >/dev/null; then
      log "DRY RUN: would validate existing group: $group_name."
    else
      run groupadd --system "$group_name"
    fi
  done
  if getent passwd vocat >/dev/null; then
    log 'DRY RUN: would validate the existing vocat account and append only the vocat-modem supplementary group.'
    run usermod --append --groups vocat-modem vocat
  else
    run useradd --system --gid vocat --groups vocat-modem --home-dir /var/lib/vocat --no-create-home --shell /usr/sbin/nologin vocat
  fi
  if getent passwd vocat-preflight >/dev/null; then
    log 'DRY RUN: would validate the existing isolated vocat-preflight account.'
  else
    run useradd --system --gid vocat-preflight --home-dir /nonexistent --no-create-home --shell /usr/sbin/nologin vocat-preflight
  fi
  run install -o root -g root -m 0644 "$DJI_RULES_SOURCE" /etc/udev/rules.d/99-vocat-dji.rules
  run udevadm control --reload-rules
  run install -d -o root -g root -m 0700 /etc/vocat
  run nft --check --file "$FIREWALL_SOURCE"
  log "DRY RUN: would stage both firewall files, validate the complete ruleset against the staged include, and restore both files and the live table on any failure."
  run install -o root -g root -m 0600 "$FIREWALL_SOURCE" /etc/vocat/vocat-firewall.nft
  run install -o root -g root -m 0644 "$FIREWALL_UNIT_SOURCE" /etc/systemd/system/vocat-firewall.service
  run install -o root -g root -m 0644 "$VOCAT_UNIT_SOURCE" /etc/systemd/system/vocat.service
  run rm -f -- /etc/systemd/system/vocat-dji-repair.service
  run install -o root -g root -m 0644 "$DJI_REPAIR_UNIT_SOURCE" /etc/systemd/system/vocat-dji-repair@.service
  log "DRY RUN: would ensure exactly one VoCat include in $NFTABLES_MAIN."
  run nft --check --file "$NFTABLES_MAIN"
  run systemctl daemon-reload
  log 'DRY RUN: would reject any unit drop-in before activating the reviewed units.'
  run systemctl enable --now nftables.service
  run systemctl start qemu-guest-agent.service
  run systemctl enable vocat-firewall.service
  run systemctl restart vocat-firewall.service
  run systemctl enable vocat.service
  log 'DRY RUN: would install udev-triggered DJI repair template without enabling a persistent service.'
  log 'DRY RUN: would finish with the same file, legacy-state absence, account, unit, service, and live nft JSON semantic checks as --check.'
  log 'Dry run only. No guest state was changed.'
  exit 0
fi

if [[ $mode == check ]]; then
  ((EUID == 0)) || die '--check requires root only to inspect the active nftables ruleset'
  for command_name in awk cmp curl dpkg-query getent grep id ip jq nft qmicli sqlite3 ss stat systemctl tr; do
    command -v "$command_name" >/dev/null 2>&1 || die "required guest command is missing: $command_name"
  done
  validate_guest_state
  log 'Guest package, service, and firewall checks passed.'
  exit 0
fi

((EUID == 0)) || die '--apply must be run as root from a private guest console'
for command_name in apt-get curl install systemctl; do
  command -v "$command_name" >/dev/null 2>&1 || die "required command is missing: $command_name"
done

if systemctl list-unit-files tailscaled.service --no-legend 2>/dev/null | grep -q '^tailscaled.service'; then
  run systemctl disable --now tailscaled.service
fi
if dpkg-query -W -f='${Status}\n' tailscale 2>/dev/null | grep -Fxq 'install ok installed'; then
  run apt-get purge -y tailscale
fi
run rm -f -- "$LEGACY_TAILSCALE_KEYRING" "$LEGACY_TAILSCALE_LIST"
run apt-get update
run apt-get install -y --no-install-recommends ca-certificates curl iproute2 jq libqmi-utils nftables python3 qemu-guest-agent sqlite3 usbutils
for command_name in awk cmp cp getent grep groupadd id ip jq mv nft ss stat tr udevadm useradd usermod; do
  command -v "$command_name" >/dev/null 2>&1 || die "required command is missing after base package installation: $command_name"
done

if systemctl list-unit-files ModemManager.service --no-legend 2>/dev/null | grep -q '^ModemManager.service'; then
  run systemctl disable --now ModemManager.service
fi
run systemctl mask ModemManager.service

if ! getent group vocat-modem >/dev/null; then
  run groupadd --system vocat-modem
fi
if ! getent group vocat >/dev/null; then
  run groupadd --system vocat
fi
if ! getent group vocat-preflight >/dev/null; then
  run groupadd --system vocat-preflight
fi
if ! getent passwd vocat >/dev/null; then
  run useradd --system --gid vocat --groups vocat-modem --home-dir /var/lib/vocat --no-create-home --shell /usr/sbin/nologin vocat
else
  validate_vocat_account
  run usermod --append --groups vocat-modem vocat
fi
if ! getent passwd vocat-preflight >/dev/null; then
  run useradd --system --gid vocat-preflight --home-dir /nonexistent --no-create-home --shell /usr/sbin/nologin vocat-preflight
else
  validate_preflight_account
fi
run install -o root -g root -m 0644 "$DJI_RULES_SOURCE" /etc/udev/rules.d/99-vocat-dji.rules
run udevadm control --reload-rules

run install -d -o root -g root -m 0700 /etc/vocat
run nft --check --file "$FIREWALL_SOURCE"
assert_root_file_secure "$NFTABLES_MAIN"
cp --no-preserve=all -- "$NFTABLES_MAIN" "$tmp_dir/nftables.conf.previous"
chmod 0600 "$tmp_dir/nftables.conf.previous"
if [[ -e /etc/vocat/vocat-firewall.nft || -L /etc/vocat/vocat-firewall.nft ]]; then
  assert_root_file_metadata /etc/vocat/vocat-firewall.nft 600
  run nft --check --file /etc/vocat/vocat-firewall.nft
  cp --no-preserve=all -- /etc/vocat/vocat-firewall.nft "$tmp_dir/vocat-firewall.nft.previous"
  chmod 0600 "$tmp_dir/vocat-firewall.nft.previous"
  firewall_target_existed=true
fi

firewall_staging=$(mktemp /etc/vocat/.vocat-firewall.nft.XXXXXX)
run install -o root -g root -m 0600 "$FIREWALL_SOURCE" "$firewall_staging"
run install -o root -g root -m 0644 "$FIREWALL_UNIT_SOURCE" /etc/systemd/system/vocat-firewall.service
run install -o root -g root -m 0644 "$VOCAT_UNIT_SOURCE" /etc/systemd/system/vocat.service
run rm -f -- /etc/systemd/system/vocat-dji-repair.service
run install -o root -g root -m 0644 "$DJI_REPAIR_UNIT_SOURCE" /etc/systemd/system/vocat-dji-repair@.service
[[ -f $NFTABLES_MAIN ]] || die 'nftables package did not create /etc/nftables.conf'
include_count=$(grep -Fxc "$NFTABLES_INCLUDE" "$NFTABLES_MAIN" || true)
[[ $include_count == 0 || $include_count == 1 ]] || die 'persistent nftables ruleset contains duplicate VoCat includes'
cp --no-preserve=all -- "$NFTABLES_MAIN" "$tmp_dir/nftables.conf.final"
if [[ $include_count == 0 ]]; then
  printf '\n%s\n' "$NFTABLES_INCLUDE" >>"$tmp_dir/nftables.conf.final"
fi
awk -v reviewed_include="$NFTABLES_INCLUDE" -v staged_include="include \"$firewall_staging\"" '
  $0 == reviewed_include { $0=staged_include }
  { print }
' "$tmp_dir/nftables.conf.final" >"$tmp_dir/nftables.conf.candidate"
run nft --check --file "$tmp_dir/nftables.conf.candidate"
nftables_staging=$(mktemp /etc/.nftables.conf.XXXXXX)
run install -o root -g root -m 0644 "$tmp_dir/nftables.conf.final" "$nftables_staging"

firewall_transaction_active=true
run mv -T -- "$firewall_staging" /etc/vocat/vocat-firewall.nft
firewall_staging=
run mv -T -- "$nftables_staging" "$NFTABLES_MAIN"
nftables_staging=
run nft --check --file "$NFTABLES_MAIN"
run systemctl daemon-reload
assert_installed_file "$FIREWALL_UNIT_SOURCE" /etc/systemd/system/vocat-firewall.service 644
assert_installed_file "$VOCAT_UNIT_SOURCE" /etc/systemd/system/vocat.service 644
assert_installed_file "$DJI_REPAIR_UNIT_SOURCE" /etc/systemd/system/vocat-dji-repair@.service 644
assert_unit_profile vocat-firewall.service /etc/systemd/system/vocat-firewall.service
assert_unit_profile vocat.service /etc/systemd/system/vocat.service
assert_unit_profile vocat-dji-repair@probe.service /etc/systemd/system/vocat-dji-repair@.service
run systemctl enable --now nftables.service
run systemctl start qemu-guest-agent.service
run systemctl enable vocat-firewall.service
run systemctl restart vocat-firewall.service
validate_live_firewall
run systemctl enable vocat.service
validate_guest_state
firewall_transaction_active=false

log 'Guest base preparation complete.'
