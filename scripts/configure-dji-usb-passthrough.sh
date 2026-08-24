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
sysname=
tmp_dir=

usage() {
  cat <<'EOF'
Usage: configure-dji-usb-passthrough.sh [--check | --dry-run | --apply] \
       --sysname USB_SYSNAME [--domain LIBVIRT_DOMAIN]

Generate a host-local USB allowlist only after selecting one physically
observed DJI device. The selected device must match all of:

  VID:PID 2ca3:4006, a non-empty USB serial, and a non-empty udev ID_PATH.

The serial and physical path are never printed. --apply stores them only in
root-owned /etc/vocat/dji-usb.conf and installs udev/systemd hotplug handling.
The generated libvirt hostdev is USB-only, address-specific, managed, and uses
startupPolicy=optional. PCI/xHCI controller passthrough is refused.
EOF
}

log() {
  printf '%s\n' "$*"
}

die() {
  printf 'ERROR: %s\n' "$*" >&2
  exit 1
}

validate_installed_allowlist() {
  local candidate_file=$1
  local installed_file=$2
  local expected_metadata=$3
  local actual_metadata

  [[ -e $installed_file || -L $installed_file ]] || return 0
  [[ -f $installed_file && ! -L $installed_file ]] || die 'installed USB allowlist is not a regular file'
  actual_metadata=$(stat -c '%U:%G:%a:%h' -- "$installed_file") || die 'cannot inspect installed USB allowlist metadata'
  [[ $actual_metadata == "$expected_metadata" ]] || die 'installed USB allowlist has unsafe ownership, mode, or link count'
  cmp -s -- "$candidate_file" "$installed_file" || die 'installed USB allowlist does not match the selected device identity'
}

cleanup() {
  if [[ -n $tmp_dir && -d $tmp_dir ]]; then
    rm -rf -- "$tmp_dir"
  fi
}
trap cleanup EXIT

while (($#)); do
  case "$1" in
    --check|--dry-run|--apply)
      mode=${1#--}
      shift
      ;;
    --sysname)
      (($# >= 2)) || die '--sysname requires a value'
      sysname=$2
      shift 2
      ;;
    --domain)
      (($# >= 2)) || die '--domain requires a value'
      domain=$2
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

[[ $sysname =~ ^[0-9]+-[0-9]+([.][0-9]+)*$ ]] || die '--sysname must identify one USB device such as 1-2'
[[ $domain =~ ^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$ ]] || die 'invalid libvirt domain name'

for source_file in "$RULE_TEMPLATE" "$ATTACH_UNIT_SOURCE" "$DETACH_UNIT_SOURCE" "$HOTPLUG_SOURCE"; do
  [[ -r $source_file ]] || die "deployment source is missing: $source_file"
done
for command_name in cmp python3 stat udevadm virsh; do
  command -v "$command_name" >/dev/null 2>&1 || die "required command is missing: $command_name"
done

sysfs_path=/sys/bus/usb/devices/$sysname
[[ -r $sysfs_path/idVendor && -r $sysfs_path/idProduct ]] || die 'selected USB device is not present'
vendor_id=$(<"$sysfs_path/idVendor")
product_id=$(<"$sysfs_path/idProduct")
[[ $vendor_id == "$EXPECTED_VENDOR" && $product_id == "$EXPECTED_PRODUCT" ]] || die 'selected device is not the reviewed DJI 2ca3:4006 composition'

properties=$(udevadm info --query=property --path="$sysfs_path") || die 'cannot read udev properties for selected device'
property_value() {
  local key=$1
  awk -F= -v wanted="$key" '$1 == wanted { print substr($0, index($0, "=") + 1); exit }' <<<"$properties"
}
serial=$(property_value ID_SERIAL_SHORT)
id_path=$(property_value ID_PATH)
[[ $serial =~ ^[A-Za-z0-9._:-]{1,128}$ ]] || die 'a stable USB serial is required and must use conservative characters'
[[ $id_path =~ ^[A-Za-z0-9._:/+-]{1,256}$ ]] || die 'a stable udev ID_PATH is required and must use conservative characters'

match_count=0
for candidate_path in /sys/bus/usb/devices/*; do
  candidate=${candidate_path##*/}
  [[ $candidate =~ ^[0-9]+-[0-9]+([.][0-9]+)*$ ]] || continue
  [[ -r $candidate_path/idVendor && -r $candidate_path/idProduct ]] || continue
  [[ $(<"$candidate_path/idVendor") == "$vendor_id" && $(<"$candidate_path/idProduct") == "$product_id" ]] || continue
  candidate_properties=$(udevadm info --query=property --path="$candidate_path" 2>/dev/null) || continue
  candidate_serial=$(awk -F= '$1 == "ID_SERIAL_SHORT" { print substr($0, index($0, "=") + 1); exit }' <<<"$candidate_properties")
  candidate_id_path=$(awk -F= '$1 == "ID_PATH" { print substr($0, index($0, "=") + 1); exit }' <<<"$candidate_properties")
  if [[ $candidate_serial == "$serial" && $candidate_id_path == "$id_path" ]]; then
    ((match_count += 1))
  fi
done
((match_count == 1)) || die 'the complete USB identity must match exactly one connected device'

tmp_dir=$(mktemp -d)
candidate_allowlist=$tmp_dir/dji-usb.conf
printf '%s\n' \
  "DOMAIN=$domain" \
  "VENDOR_ID=$vendor_id" \
  "PRODUCT_ID=$product_id" \
  "SERIAL=$serial" \
  "ID_PATH=$id_path" >"$candidate_allowlist"
if [[ -e $CONFIG_TARGET || -L $CONFIG_TARGET ]]; then
  ((EUID == 0)) || die 'checking an installed USB allowlist requires root'
fi
validate_installed_allowlist "$candidate_allowlist" "$CONFIG_TARGET" root:root:600:1

domain_xml=$tmp_dir/domain.xml
virsh --connect qemu:///system dumpxml --inactive "$domain" >"$domain_xml" || die 'libvirt domain does not exist or is inaccessible'
live_domain_xml=-
domain_state=$(virsh --connect qemu:///system domstate "$domain" 2>/dev/null | tr -d '\r') || die 'cannot determine libvirt domain state'
if [[ $domain_state != 'shut off' && $domain_state != 'crashed' ]]; then
  live_domain_xml=$tmp_dir/domain-live.xml
  virsh --connect qemu:///system dumpxml "$domain" >"$live_domain_xml" || die 'cannot inspect live libvirt domain XML'
fi
python3 - "$domain_xml" "$live_domain_xml" "$EXPECTED_VENDOR" "$EXPECTED_PRODUCT" <<'PY'
import sys
import xml.etree.ElementTree as ET

inactive_path, live_path, vendor_id, product_id = sys.argv[1:]


def validate(path, scope):
    root = ET.parse(path).getroot()
    hostdevs = root.findall("./devices/hostdev")
    if len(hostdevs) > 1:
        raise SystemExit(f"ERROR: unexpected extra {scope} hostdev passthrough is present")
    for hostdev in hostdevs:
        alias = hostdev.find("alias")
        source = hostdev.find("source")
        vendor = source.find("vendor") if source is not None else None
        product = source.find("product") if source is not None else None
        address = source.find("address") if source is not None else None
        startup_policy = source.get("startupPolicy") if source is not None else None
        startup_ok = startup_policy == "optional" if scope == "inactive" else startup_policy in (None, "optional")
        try:
            bus = int(address.get("bus", ""), 10) if address is not None else -1
            device = int(address.get("device", ""), 10) if address is not None else -1
        except ValueError:
            bus = device = -1
        if (
            hostdev.get("mode") != "subsystem"
            or hostdev.get("type") != "usb"
            or hostdev.get("managed") != "yes"
            or alias is None
            or alias.get("name") != "ua-vocat-dji-usb"
            or source is None
            or not startup_ok
            or vendor is None
            or vendor.get("id", "").lower() != f"0x{vendor_id}"
            or product is None
            or product.get("id", "").lower() != f"0x{product_id}"
            or not (1 <= bus <= 255 and 1 <= device <= 255)
        ):
            raise SystemExit(f"ERROR: unauthorized USB or PCI/controller passthrough is present in {scope} XML")
    os_type = root.find("./os/type")
    if os_type is None or "q35" not in os_type.get("machine", ""):
        raise SystemExit(f"ERROR: {scope} domain is not the reviewed q35 profile")


validate(inactive_path, "inactive")
if live_path != "-":
    validate(live_path, "live")
PY

log 'Observed exactly one DJI 2ca3:4006 device with both serial and physical-path identity; values were not displayed.'
if [[ $mode == check ]]; then
  log 'Read-only USB passthrough preflight passed. No allowlist or libvirt device was created.'
  exit 0
fi
if [[ $mode == dry-run ]]; then
  log 'DRY RUN: would install a root-only exact allowlist, two hardened hotplug units, and a VID/PID trigger rule.'
  log 'DRY RUN: would attach only this USB device to the persistent and live domain; no PCI controller operation is generated.'
  exit 0
fi

((EUID == 0)) || die '--apply must run as root from a private local terminal'
for command_name in install sed systemctl; do
  command -v "$command_name" >/dev/null 2>&1 || die "required command is missing: $command_name"
done

sed \
  -e "s|@VENDOR_ID@|$vendor_id|g" \
  -e "s|@PRODUCT_ID@|$product_id|g" \
  "$RULE_TEMPLATE" >"$tmp_dir/70-vocat-dji-passthrough.rules"

install -d -o root -g root -m 0700 /etc/vocat
install -d -o root -g root -m 0755 /usr/local/libexec
install -d -o root -g root -m 0700 "$STATE_DIR"
install -o root -g root -m 0600 "$candidate_allowlist" "$CONFIG_TARGET"
install -o root -g root -m 0644 "$tmp_dir/70-vocat-dji-passthrough.rules" "$RULE_TARGET"
install -o root -g root -m 0755 "$HOTPLUG_SOURCE" "$HOTPLUG_TARGET"
install -o root -g root -m 0644 "$ATTACH_UNIT_SOURCE" /etc/systemd/system/vocat-usb-attach@.service
install -o root -g root -m 0644 "$DETACH_UNIT_SOURCE" /etc/systemd/system/vocat-usb-detach@.service
systemctl daemon-reload
udevadm control --reload-rules
"$HOTPLUG_TARGET" attach "$sysname"

log 'USB allowlist and optional hotplug passthrough are active. Production identifiers remain only in the root-only host config.'
