#!/usr/bin/env bash
set -Eeuo pipefail
export LC_ALL=C
umask 077

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
readonly SCRIPT_DIR
REPO_ROOT=$(cd -- "$SCRIPT_DIR/.." && pwd)
readonly REPO_ROOT
readonly RULE_TEMPLATE=$REPO_ROOT/deploy/vocat-dji-usb.rules.in
readonly ATTACH_UNIT_SOURCE=$REPO_ROOT/deploy/vocat-usb-attach@.service
readonly DETACH_UNIT_SOURCE=$REPO_ROOT/deploy/vocat-usb-detach@.service
readonly HOTPLUG_SOURCE=$SCRIPT_DIR/vocat-usb-hotplug.sh
readonly CONFIG_TARGET=/etc/vocat/dji-usb.conf
readonly RULE_TARGET=/etc/udev/rules.d/70-vocat-dji-passthrough.rules
readonly HOTPLUG_TARGET=/usr/local/libexec/vocat-usb-hotplug
readonly STATE_DIR=/var/lib/vocat-usb
readonly EXPECTED_VENDOR=2ca3
readonly EXPECTED_PRODUCT=4006

mode=check
domain=vocat
sysnames=()
tmp_dir=

usage() {
  cat <<'EOF'
Usage: configure-dji-usb-passthrough.sh [--check | --dry-run | --apply] \
       [--sysname USB_SYSNAME ...] [--domain LIBVIRT_DOMAIN]

With no --sysname, discover and enroll every connected DJI 2ca3:4006 device.
Every device must have a unique stable udev ID_PATH. A USB serial is optional;
when present it is also required on every hotplug. With no serial, identity is
bound to VID/PID and the enrolled physical USB path, so moving the module to
another port is rejected.

Production identities are never printed. --apply stores them only in root-owned
/etc/vocat/dji-usb.conf and installs recoverable udev/systemd hotplug handling.
Only managed USB hostdevs are generated; PCI/xHCI passthrough remains forbidden.
EOF
}

log() { printf '%s\n' "$*"; }
die() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }

validate_installed_allowlist() {
  local installed_file=$1 expected_metadata=$2 actual_metadata
  [[ -e $installed_file || -L $installed_file ]] || return 0
  [[ -f $installed_file && ! -L $installed_file ]] || die 'installed USB allowlist is not a regular file'
  actual_metadata=$(stat -c '%U:%G:%a:%h' -- "$installed_file") || die 'cannot inspect installed USB allowlist metadata'
  [[ $actual_metadata == "$expected_metadata" ]] || die 'installed USB allowlist has unsafe ownership, mode, or link count'
}

installed_value() {
  local key=$1 values=()
  mapfile -t values < <(awk -F= -v wanted="$key" '$1 == wanted { print substr($0, index($0, "=") + 1) }' "$CONFIG_TARGET")
  ((${#values[@]} == 1)) || die "installed allowlist must contain exactly one $key value"
  printf '%s' "${values[0]}"
}

preserve_installed_slots() {
  [[ -e $CONFIG_TARGET || -L $CONFIG_TARGET ]] || return 0
  ((EUID == 0)) || die 'checking an installed USB allowlist requires root'
  validate_installed_allowlist "$CONFIG_TARGET" root:root:600:1

  local installed_count slot existing_serial existing_path index
  [[ $(installed_value SCHEMA) == 2 && $(installed_value DOMAIN) == "$domain" &&
     $(installed_value VENDOR_ID) == "$EXPECTED_VENDOR" &&
     $(installed_value PRODUCT_ID) == "$EXPECTED_PRODUCT" ]] || die 'installed USB allowlist uses an incompatible profile'
  installed_count=$(installed_value DEVICE_COUNT)
  [[ $installed_count =~ ^[1-9][0-9]{0,2}$ ]] && ((installed_count <= 255)) || die 'installed USB allowlist has an invalid device count'
  [[ $(wc -l <"$CONFIG_TARGET") == $((5 + installed_count * 2)) ]] || die 'installed USB allowlist contains unexpected entries'

  declare -A selected_by_path=() used=()
  for index in "${!id_paths[@]}"; do selected_by_path[${id_paths[$index]}]=$index; done
  ordered_sysnames=(); ordered_serials=(); ordered_paths=()
  for slot in $(seq 1 "$installed_count"); do
    existing_serial=$(installed_value "DEVICE_${slot}_SERIAL")
    existing_path=$(installed_value "DEVICE_${slot}_ID_PATH")
    [[ -n ${selected_by_path[$existing_path]+x} ]] || die 'every previously enrolled DJI device must be connected when updating the allowlist'
    index=${selected_by_path[$existing_path]}
    [[ ${serials[$index]} == "$existing_serial" ]] || die 'an enrolled DJI USB serial changed'
    ordered_sysnames+=("${sysnames[$index]}"); ordered_serials+=("$existing_serial"); ordered_paths+=("$existing_path"); used[$index]=1
  done
  for index in "${!id_paths[@]}"; do
    [[ -n ${used[$index]+x} ]] && continue
    ordered_sysnames+=("${sysnames[$index]}"); ordered_serials+=("${serials[$index]}"); ordered_paths+=("${id_paths[$index]}")
  done
  sysnames=("${ordered_sysnames[@]}"); serials=("${ordered_serials[@]}"); id_paths=("${ordered_paths[@]}")
}

cleanup() { [[ -n $tmp_dir && -d $tmp_dir ]] && rm -rf -- "$tmp_dir" || true; }
trap cleanup EXIT

while (($#)); do
  case "$1" in
    --check|--dry-run|--apply) mode=${1#--}; shift ;;
    --sysname) (($# >= 2)) || die '--sysname requires a value'; sysnames+=("$2"); shift 2 ;;
    --domain) (($# >= 2)) || die '--domain requires a value'; domain=$2; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
done

[[ $domain =~ ^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$ ]] || die 'invalid libvirt domain name'
if ((${#sysnames[@]} == 0)); then
  for candidate_path in /sys/bus/usb/devices/*; do
    candidate=${candidate_path##*/}
    [[ $candidate =~ ^[0-9]+-[0-9]+([.][0-9]+)*$ ]] || continue
    [[ -r $candidate_path/idVendor && -r $candidate_path/idProduct ]] || continue
    [[ $(<"$candidate_path/idVendor") == "$EXPECTED_VENDOR" &&
       $(<"$candidate_path/idProduct") == "$EXPECTED_PRODUCT" ]] || continue
    sysnames+=("$candidate")
  done
fi
device_count=${#sysnames[@]}
((device_count >= 1 && device_count <= 255)) || die 'expected between 1 and 255 connected or selected DJI USB devices'
declare -A seen_sysnames=()
for sysname in "${sysnames[@]}"; do
  [[ $sysname =~ ^[0-9]+-[0-9]+([.][0-9]+)*$ ]] || die '--sysname must identify a USB device such as 1-2'
  [[ -z ${seen_sysnames[$sysname]+x} ]] || die 'duplicate --sysname is forbidden'
  seen_sysnames[$sysname]=1
done

for source_file in "$RULE_TEMPLATE" "$ATTACH_UNIT_SOURCE" "$DETACH_UNIT_SOURCE" "$HOTPLUG_SOURCE"; do
  [[ -r $source_file ]] || die "deployment source is missing: $source_file"
done
for command_name in python3 stat udevadm virsh; do
  command -v "$command_name" >/dev/null 2>&1 || die "required command is missing: $command_name"
done

property_value() {
  local properties=$1 key=$2
  awk -F= -v wanted="$key" '$1 == wanted { print substr($0, index($0, "=") + 1); exit }' <<<"$properties"
}

serials=()
id_paths=()
declare -A seen_id_paths=()
for sysname in "${sysnames[@]}"; do
  sysfs_path=/sys/bus/usb/devices/$sysname
  [[ -r $sysfs_path/idVendor && -r $sysfs_path/idProduct ]] || die 'selected USB device is not present'
  [[ $(<"$sysfs_path/idVendor") == "$EXPECTED_VENDOR" && $(<"$sysfs_path/idProduct") == "$EXPECTED_PRODUCT" ]] ||
    die 'selected device is not the reviewed DJI 2ca3:4006 composition'
  properties=$(udevadm info --query=property --path="$sysfs_path") || die 'cannot read udev properties for selected device'
  serial=$(property_value "$properties" ID_SERIAL_SHORT)
  id_path=$(property_value "$properties" ID_PATH)
  [[ -z $serial || $serial =~ ^[A-Za-z0-9._:-]{1,128}$ ]] || die 'USB serial uses unsupported characters'
  [[ $id_path =~ ^[A-Za-z0-9._:/+-]{1,256}$ ]] || die 'a stable udev ID_PATH is required and must use conservative characters'
  [[ -z ${seen_id_paths[$id_path]+x} ]] || die 'selected devices must have unique physical paths'
  seen_id_paths[$id_path]=1
  serials+=("$serial")
  id_paths+=("$id_path")
done

preserve_installed_slots

for index in "${!sysnames[@]}"; do
  match_count=0
  for candidate_path in /sys/bus/usb/devices/*; do
    candidate=${candidate_path##*/}
    [[ $candidate =~ ^[0-9]+-[0-9]+([.][0-9]+)*$ ]] || continue
    [[ -r $candidate_path/idVendor && -r $candidate_path/idProduct ]] || continue
    [[ $(<"$candidate_path/idVendor") == "$EXPECTED_VENDOR" && $(<"$candidate_path/idProduct") == "$EXPECTED_PRODUCT" ]] || continue
    candidate_properties=$(udevadm info --query=property --path="$candidate_path" 2>/dev/null) || continue
    candidate_serial=$(property_value "$candidate_properties" ID_SERIAL_SHORT)
    candidate_id_path=$(property_value "$candidate_properties" ID_PATH)
    if [[ $candidate_id_path == "${id_paths[$index]}" &&
          ( -z ${serials[$index]} || $candidate_serial == "${serials[$index]}" ) ]]; then
      ((match_count += 1))
    fi
  done
  ((match_count == 1)) || die 'each complete USB identity must match exactly one connected device'
done

tmp_dir=$(mktemp -d)
candidate_allowlist=$tmp_dir/dji-usb.conf
{
  printf 'SCHEMA=2\nDOMAIN=%s\nVENDOR_ID=%s\nPRODUCT_ID=%s\nDEVICE_COUNT=%s\n' \
    "$domain" "$EXPECTED_VENDOR" "$EXPECTED_PRODUCT" "$device_count"
  for index in "${!sysnames[@]}"; do
    slot=$((index + 1))
    printf 'DEVICE_%s_SERIAL=%s\nDEVICE_%s_ID_PATH=%s\n' \
      "$slot" "${serials[$index]}" "$slot" "${id_paths[$index]}"
  done
} >"$candidate_allowlist"
validate_installed_allowlist "$CONFIG_TARGET" root:root:600:1

domain_xml=$tmp_dir/domain.xml
virsh --connect qemu:///system dumpxml --inactive "$domain" >"$domain_xml" || die 'libvirt domain does not exist or is inaccessible'
live_domain_xml=-
domain_state=$(virsh --connect qemu:///system domstate "$domain" 2>/dev/null | tr -d '\r') || die 'cannot determine libvirt domain state'
if [[ $domain_state != 'shut off' && $domain_state != 'crashed' ]]; then
  live_domain_xml=$tmp_dir/domain-live.xml
  virsh --connect qemu:///system dumpxml "$domain" >"$live_domain_xml" || die 'cannot inspect live libvirt domain XML'
fi
python3 - "$domain_xml" "$live_domain_xml" "$EXPECTED_VENDOR" "$EXPECTED_PRODUCT" "$device_count" <<'PY'
import sys
import xml.etree.ElementTree as ET

inactive_path, live_path, vendor_id, product_id, device_count = sys.argv[1:]
device_count = int(device_count)

def managed_alias(name):
    prefix = "ua-vocat-dji-usb-"
    suffix = name.removeprefix(prefix)
    return name.startswith(prefix) and suffix.isascii() and suffix.isdigit() and 1 <= int(suffix) <= device_count

def validate(path, scope):
    root = ET.parse(path).getroot()
    hostdevs = root.findall("./devices/hostdev")
    if len(hostdevs) > device_count:
        raise SystemExit(f"ERROR: unexpected extra {scope} hostdev passthrough is present")
    seen = set()
    for hostdev in hostdevs:
        sources = hostdev.findall("source"); aliases = hostdev.findall("alias")
        source = sources[0] if len(sources) == 1 else None
        alias = aliases[0] if len(aliases) == 1 else None
        vendors = source.findall("vendor") if source is not None else []
        products = source.findall("product") if source is not None else []
        addresses = source.findall("address") if source is not None else []
        vendor = vendors[0] if len(vendors) == 1 else None
        product = products[0] if len(products) == 1 else None
        address = addresses[0] if len(addresses) == 1 else None
        name = alias.get("name", "") if alias is not None else ""
        startup = source.get("startupPolicy") if source is not None else None
        startup_ok = startup == "optional" if scope == "inactive" else startup in (None, "optional")
        try:
            bus = int(address.get("bus", ""), 10); device = int(address.get("device", ""), 10)
        except (AttributeError, ValueError):
            bus = device = -1
        if (set(hostdev.attrib) != {"mode", "type", "managed"} or
            hostdev.get("mode") != "subsystem" or hostdev.get("type") != "usb" or
            hostdev.get("managed") != "yes" or source is None or alias is None or
            alias.attrib != {"name": name} or list(alias) or
            not managed_alias(name) or name in seen or
            set(source.attrib) - {"startupPolicy"} or
            [child.tag for child in source].count("vendor") != 1 or
            [child.tag for child in source].count("product") != 1 or
            [child.tag for child in source].count("address") != 1 or
            any(child.tag not in {"vendor", "product", "address"} for child in source) or
            not startup_ok or vendor is None or
            {key: value.lower() for key, value in vendor.attrib.items()} != {"id": f"0x{vendor_id}"} or
            list(vendor) or product is None or
            {key: value.lower() for key, value in product.attrib.items()} != {"id": f"0x{product_id}"} or
            list(product) or address is None or set(address.attrib) != {"bus", "device"} or
            list(address) or not (1 <= bus <= 255 and 1 <= device <= 255) or
            any(child.tag not in {"source", "alias", "address"} for child in hostdev) or
            any(item.get("type") != "usb" for item in hostdev.findall("address"))):
            raise SystemExit(f"ERROR: unauthorized USB or PCI/controller passthrough is present in {scope} XML")
        seen.add(name)

validate(inactive_path, "inactive")
if live_path != "-":
    validate(live_path, "live")
PY

log "Observed $device_count path-bound DJI 2ca3:4006 device(s); production identities were not displayed."
if [[ $mode == check ]]; then
  log 'Read-only USB passthrough preflight passed. No allowlist or libvirt device was created.'
  exit 0
fi
if [[ $mode == dry-run ]]; then
  log "DRY RUN: would install a root-only $device_count-device allowlist and hardened hotplug handling."
  log 'DRY RUN: would attach only the selected USB devices; no PCI controller operation is generated.'
  exit 0
fi

((EUID == 0)) || die '--apply must run as root from a private local terminal'
for command_name in install sed systemctl; do command -v "$command_name" >/dev/null || die "required command is missing: $command_name"; done
sed -e "s|@VENDOR_ID@|$EXPECTED_VENDOR|g" -e "s|@PRODUCT_ID@|$EXPECTED_PRODUCT|g" \
  "$RULE_TEMPLATE" >"$tmp_dir/70-vocat-dji-passthrough.rules"
install -d -o root -g root -m 0700 /etc/vocat "$STATE_DIR"
for slot in $(seq 1 "$device_count"); do
  install -d -o root -g root -m 0700 "$STATE_DIR/slot-$slot"
done
install -d -o root -g root -m 0755 /usr/local/libexec
install -o root -g root -m 0600 "$candidate_allowlist" "$CONFIG_TARGET"
install -o root -g root -m 0644 "$tmp_dir/70-vocat-dji-passthrough.rules" "$RULE_TARGET"
install -o root -g root -m 0755 "$HOTPLUG_SOURCE" "$HOTPLUG_TARGET"
install -o root -g root -m 0644 "$ATTACH_UNIT_SOURCE" /etc/systemd/system/vocat-usb-attach@.service
install -o root -g root -m 0644 "$DETACH_UNIT_SOURCE" /etc/systemd/system/vocat-usb-detach@.service
systemctl daemon-reload
udevadm control --reload-rules

attached=()
for sysname in "${sysnames[@]}"; do
  if "$HOTPLUG_TARGET" attach "$sysname"; then
    attached+=("$sysname")
  else
    rollback_failed=false
    for attached_sysname in "${attached[@]}"; do
      "$HOTPLUG_TARGET" detach "$attached_sysname" || rollback_failed=true
    done
    [[ $rollback_failed == false ]] ||
      die 'multi-device attachment failed and rollback was incomplete; manual review is required'
    die 'multi-device attachment failed and completed attachments were rolled back'
  fi
done
log "$device_count path-bound DJI USB device(s) are enrolled with optional hotplug passthrough."
