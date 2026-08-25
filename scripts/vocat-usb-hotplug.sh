#!/usr/bin/env bash
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly CONFIG_FILE=/etc/vocat/dji-usb.conf
readonly STATE_DIR=/var/lib/vocat-usb
readonly LIBVIRT_URI=qemu:///system

action=${1:-}
sysname=${2:-}
temp_files=()
alias_in_config=false
alias_in_live=false
domain_live_snapshot=false

die() {
  printf 'vocat-usb-hotplug: %s\n' "$*" >&2
  exit 1
}

cleanup() {
  local path
  for path in "${temp_files[@]}"; do
    [[ -n $path ]] && rm -f -- "$path"
  done
}
trap cleanup EXIT

[[ $action == attach || $action == detach || $action == check ]] || die 'usage: vocat-usb-hotplug {attach|detach|check} USB_SYSNAME'
[[ $sysname =~ ^[0-9]+-[0-9]+([.][0-9]+)*$ ]] || die 'invalid USB device sysname'
((EUID == 0)) || die 'must run as root'

for command_name in find flock install mv python3 rmdir seq stat udevadm virsh wc; do
  command -v "$command_name" >/dev/null 2>&1 || die "required command is missing: $command_name"
done
[[ -f $CONFIG_FILE && ! -L $CONFIG_FILE ]] || die 'USB allowlist is not configured as a regular file'
[[ $(stat -c '%U:%G:%a:%h' "$CONFIG_FILE") == root:root:600:1 ]] ||
  die 'USB allowlist must be root:root with mode 0600 and one link'

config_value() {
  local key=$1
  local values=()
  mapfile -t values < <(awk -F= -v wanted="$key" '$1 == wanted { print substr($0, index($0, "=") + 1) }' "$CONFIG_FILE")
  ((${#values[@]} == 1)) || die "allowlist must contain exactly one $key value"
  printf '%s' "${values[0]}"
}

load_allowlist() {
  schema=$(config_value SCHEMA)
  domain=$(config_value DOMAIN)
  vendor_id=$(config_value VENDOR_ID)
  product_id=$(config_value PRODUCT_ID)
  device_count=$(config_value DEVICE_COUNT)

  [[ $schema == 2 ]] || die 'allowlist schema is unsupported'
  [[ $domain =~ ^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$ ]] || die 'invalid domain in allowlist'
  [[ $vendor_id == 2ca3 && $product_id == 4006 ]] || die 'allowlist is not the reviewed DJI 2ca3:4006 device composition'
  [[ $device_count =~ ^[1-9][0-9]{0,2}$ ]] && ((device_count <= 255)) ||
    die 'allowlist must contain between 1 and 255 devices'

  serials=()
  id_paths=()
  for slot in $(seq 1 "$device_count"); do
    serial=$(config_value "DEVICE_${slot}_SERIAL")
    id_path=$(config_value "DEVICE_${slot}_ID_PATH")
    [[ -z $serial || $serial =~ ^[A-Za-z0-9._:-]{1,128}$ ]] || die 'device serial contains unsupported characters'
    [[ $id_path =~ ^[A-Za-z0-9._:/+-]{1,256}$ ]] || die 'device ID_PATH contains unsupported characters'
    serials+=("$serial")
    id_paths+=("$id_path")
  done
  declare -A seen_paths=()
  for id_path in "${id_paths[@]}"; do
    [[ -z ${seen_paths[$id_path]+x} ]] || die 'allowlist physical paths must be unique'
    seen_paths[$id_path]=1
  done
  expected_lines=$((5 + device_count * 2))
  [[ $(wc -l <"$CONFIG_FILE") == "$expected_lines" ]] || die 'allowlist contains unexpected or missing entries'
}
load_allowlist

select_slot() {
  local selected=$1
  ((selected >= 1 && selected <= device_count)) || return 1
  slot=$selected
  serial=${serials[selected - 1]}
  id_path=${id_paths[selected - 1]}
  HOSTDEV_ALIAS=ua-vocat-dji-usb-$selected
  SLOT_STATE=$STATE_DIR/slot-$selected
  CURRENT_STATE=$SLOT_STATE/current
  PENDING_STATE=$SLOT_STATE/pending
}
select_slot 1

[[ -d $STATE_DIR && ! -L $STATE_DIR ]] || die 'USB passthrough state directory is missing or unsafe'
[[ $(stat -c '%U:%G:%a' "$STATE_DIR") == root:root:700 ]] || die 'USB passthrough state directory must be root:root with mode 0700'
exec 9>"$STATE_DIR/hotplug.lock"
flock -x 9

virsh --connect "$LIBVIRT_URI" dominfo "$domain" >/dev/null 2>&1 || die 'configured libvirt domain does not exist'

property_value() {
  local properties=$1
  local key=$2
  awk -F= -v wanted="$key" '$1 == wanted { print substr($0, index($0, "=") + 1); exit }' <<<"$properties"
}

identity_matches() {
  local candidate_serial=$1
  local candidate_path=$2
  [[ $candidate_path == "$id_path" && ( -z $serial || $candidate_serial == "$serial" ) ]]
}

matches_allowlist() {
  local candidate=$1
  local sysfs_path=/sys/bus/usb/devices/$candidate
  local properties candidate_vendor candidate_product candidate_serial candidate_path
  [[ -r $sysfs_path/idVendor && -r $sysfs_path/idProduct ]] || return 1
  candidate_vendor=$(<"$sysfs_path/idVendor")
  candidate_product=$(<"$sysfs_path/idProduct")
  [[ $candidate_vendor == "$vendor_id" && $candidate_product == "$product_id" ]] || return 1
  properties=$(udevadm info --query=property --path="$sysfs_path" 2>/dev/null) || return 1
  candidate_serial=$(property_value "$properties" ID_SERIAL_SHORT)
  candidate_path=$(property_value "$properties" ID_PATH)
  identity_matches "$candidate_serial" "$candidate_path"
}

alias_present() {
  local xml_file=$1
  python3 - "$xml_file" "$HOSTDEV_ALIAS" <<'PY'
import sys
import xml.etree.ElementTree as ET

root = ET.parse(sys.argv[1]).getroot()
alias_name = sys.argv[2]
hostdevs = [root] if root.tag == "hostdev" else root.findall("./devices/hostdev")
for hostdev in hostdevs:
    alias = hostdev.find("alias")
    if alias is not None and alias.get("name") == alias_name:
        raise SystemExit(0)
raise SystemExit(1)
PY
}

assert_managed_passthrough() {
  local xml_file=$1
  local scope=$2
  local require_one=$3
  python3 - "$xml_file" "$HOSTDEV_ALIAS" "$vendor_id" "$product_id" "$scope" "$require_one" "$device_count" <<'PY'
import sys
import xml.etree.ElementTree as ET

xml_file, alias_name, vendor_id, product_id, scope, require_one, device_count = sys.argv[1:]
root = ET.parse(xml_file).getroot()
hostdevs = [root] if root.tag == "hostdev" else root.findall("./devices/hostdev")
device_count = int(device_count)

def managed_alias(name):
    prefix = "ua-vocat-dji-usb-"
    suffix = name.removeprefix(prefix)
    return name.startswith(prefix) and suffix.isascii() and suffix.isdigit() and 1 <= int(suffix) <= device_count
if require_one == "yes" and len(hostdevs) != 1:
    raise SystemExit("trusted USB state must contain exactly one hostdev")
if len(hostdevs) > device_count:
    raise SystemExit("unexpected extra hostdev passthrough is present")
seen = set()
for hostdev in hostdevs:
    aliases = hostdev.findall("alias")
    sources = hostdev.findall("source")
    alias = aliases[0] if len(aliases) == 1 else None
    source = sources[0] if len(sources) == 1 else None
    vendors = source.findall("vendor") if source is not None else []
    products = source.findall("product") if source is not None else []
    addresses = source.findall("address") if source is not None else []
    vendor = vendors[0] if len(vendors) == 1 else None
    product = products[0] if len(products) == 1 else None
    address = addresses[0] if len(addresses) == 1 else None
    startup_policy = source.get("startupPolicy") if source is not None else None
    if scope == "live":
        startup_ok = startup_policy in (None, "optional")
    else:
        startup_ok = startup_policy == "optional"
    try:
        bus = int(address.get("bus", ""), 10) if address is not None else -1
        device = int(address.get("device", ""), 10) if address is not None else -1
    except ValueError:
        bus = device = -1
    actual_alias = alias.get("name", "") if alias is not None else ""
    alias_ok = actual_alias == alias_name if require_one == "yes" else managed_alias(actual_alias)
    if (
        set(hostdev.attrib) != {"mode", "type", "managed"}
        or hostdev.get("mode") != "subsystem"
        or hostdev.get("type") != "usb"
        or hostdev.get("managed") != "yes"
        or alias is None
        or alias.attrib != {"name": actual_alias}
        or list(alias)
        or not alias_ok
        or actual_alias in seen
        or source is None
        or set(source.attrib) - {"startupPolicy"}
        or [child.tag for child in source].count("vendor") != 1
        or [child.tag for child in source].count("product") != 1
        or [child.tag for child in source].count("address") != 1
        or any(child.tag not in {"vendor", "product", "address"} for child in source)
        or not startup_ok
        or vendor is None
        or {key: value.lower() for key, value in vendor.attrib.items()} != {"id": f"0x{vendor_id}"}
        or list(vendor)
        or product is None
        or {key: value.lower() for key, value in product.attrib.items()} != {"id": f"0x{product_id}"}
        or list(product)
        or address is None
        or set(address.attrib) != {"bus", "device"}
        or list(address)
        or not (1 <= bus <= 255 and 1 <= device <= 255)
        or any(child.tag not in {"source", "alias", "address"} for child in hostdev)
        or any(item.get("type") != "usb" for item in hostdev.findall("address"))
    ):
        raise SystemExit("unauthorized USB or PCI/controller passthrough is present")
    seen.add(actual_alias)
PY
}

assert_passthrough_identity() {
  local xml_file=$1
  local scope=$2
  local expected_bus=$3
  local expected_device=$4

  assert_managed_passthrough "$xml_file" "$scope" no || return 1
  python3 - "$xml_file" "$HOSTDEV_ALIAS" "$vendor_id" "$product_id" "$scope" "$expected_bus" "$expected_device" <<'PY'
import sys
import xml.etree.ElementTree as ET

xml_file, alias_name, vendor_id, product_id, scope, expected_bus, expected_device = sys.argv[1:]
root = ET.parse(xml_file).getroot()
hostdevs = [root] if root.tag == "hostdev" else root.findall("./devices/hostdev")
hostdevs = [item for item in hostdevs if item.find("alias") is not None and item.find("alias").get("name") == alias_name]
if len(hostdevs) != 1:
    raise SystemExit("complete USB identity requires exactly one matching hostdev")
try:
    expected_address = (int(expected_bus, 10), int(expected_device, 10))
except ValueError:
    raise SystemExit("expected USB address is invalid")
if not all(1 <= value <= 255 for value in expected_address):
    raise SystemExit("expected USB address is outside the libvirt range")

hostdev = hostdevs[0]
aliases = hostdev.findall("alias")
sources = hostdev.findall("source")
alias = aliases[0] if len(aliases) == 1 else None
source = sources[0] if len(sources) == 1 else None
vendors = source.findall("vendor") if source is not None else []
products = source.findall("product") if source is not None else []
addresses = source.findall("address") if source is not None else []
vendor = vendors[0] if len(vendors) == 1 else None
product = products[0] if len(products) == 1 else None
address = addresses[0] if len(addresses) == 1 else None
startup_policy = source.get("startupPolicy") if source is not None else None
startup_ok = startup_policy in (None, "optional") if scope == "live" else startup_policy == "optional"
try:
    actual_address = (
        int(address.get("bus", ""), 10),
        int(address.get("device", ""), 10),
    ) if address is not None else (-1, -1)
except ValueError:
    actual_address = (-1, -1)
if (
    hostdev.get("mode") != "subsystem"
    or hostdev.get("type") != "usb"
    or hostdev.get("managed") != "yes"
    or alias is None
    or alias.attrib != {"name": alias_name}
    or source is None
    or not startup_ok
    or vendor is None
    or {key: value.lower() for key, value in vendor.attrib.items()} != {"id": f"0x{vendor_id}"}
    or product is None
    or {key: value.lower() for key, value in product.attrib.items()} != {"id": f"0x{product_id}"}
    or address is None
    or set(address.attrib) != {"bus", "device"}
    or actual_address != expected_address
):
    raise SystemExit("trusted USB identity does not match the connected device")
PY
}

read_domain_state() {
  local state
  state=$(virsh --connect "$LIBVIRT_URI" domstate "$domain" 2>/dev/null) || return 1
  state=${state//$'\r'/}
  [[ -n $state ]] || return 1
  printf '%s' "$state"
}

dump_domain_xml() {
  local scope=$1
  local destination=$2
  if [[ $scope == config ]]; then
    virsh --connect "$LIBVIRT_URI" dumpxml --inactive "$domain" >"$destination"
  else
    virsh --connect "$LIBVIRT_URI" dumpxml "$domain" >"$destination"
  fi
}

validate_domain_passthrough() {
  local config_dump live_dump domain_state
  config_dump=$(mktemp)
  temp_files+=("$config_dump")
  dump_domain_xml config "$config_dump" || return 1
  assert_managed_passthrough "$config_dump" config no || return 1
  domain_state=$(read_domain_state) || return 1
  if [[ $domain_state != 'shut off' && $domain_state != 'crashed' ]]; then
    live_dump=$(mktemp)
    temp_files+=("$live_dump")
    dump_domain_xml live "$live_dump" || return 1
    assert_managed_passthrough "$live_dump" live no || return 1
  fi
}

refresh_alias_status() {
  local config_dump live_dump domain_state
  alias_in_config=false
  alias_in_live=false
  domain_live_snapshot=false

  config_dump=$(mktemp)
  temp_files+=("$config_dump")
  dump_domain_xml config "$config_dump" || return 1
  if alias_present "$config_dump"; then
    alias_in_config=true
  fi
  domain_state=$(read_domain_state) || return 1
  if [[ $domain_state != 'shut off' && $domain_state != 'crashed' ]]; then
    domain_live_snapshot=true
    live_dump=$(mktemp)
    temp_files+=("$live_dump")
    dump_domain_xml live "$live_dump" || return 1
    if alias_present "$live_dump"; then
      alias_in_live=true
    fi
  fi
}

detach_persistent() {
  local xml_file=$1
  refresh_alias_status || return 1
  if [[ $alias_in_config == false && $alias_in_live == false ]]; then
    return 0
  fi

  if ! virsh --connect "$LIBVIRT_URI" detach-device "$domain" "$xml_file" --persistent; then
    # A physical removal can leave live and inactive XML temporarily out of
    # sync. Use a scoped operation only to recover that already-divergent state.
    refresh_alias_status || return 1
    if [[ $alias_in_config == true && $alias_in_live == false ]]; then
      virsh --connect "$LIBVIRT_URI" detach-device "$domain" "$xml_file" --config || return 1
    elif [[ $alias_in_config == false && $alias_in_live == true ]]; then
      virsh --connect "$LIBVIRT_URI" detach-device "$domain" "$xml_file" --live || return 1
    elif [[ $alias_in_config == true || $alias_in_live == true ]]; then
      return 1
    fi
  fi

  refresh_alias_status || return 1
  [[ $alias_in_config == false && $alias_in_live == false ]]
}

validate_state_directory() {
  local state_path=$1
  local state_xml=$state_path/device.xml
  local state_sysname=$state_path/sysname
  local entry_count recorded_sysname

  [[ -d $state_path && ! -L $state_path ]] || return 1
  [[ $(stat -c '%U:%G:%a' "$state_path") == root:root:700 ]] || return 1
  [[ -f $state_xml && ! -L $state_xml && -f $state_sysname && ! -L $state_sysname ]] || return 1
  [[ $(stat -c '%U:%G:%a' "$state_xml") == root:root:600 ]] || return 1
  [[ $(stat -c '%U:%G:%a' "$state_sysname") == root:root:600 ]] || return 1
  entry_count=$(find "$state_path" -mindepth 1 -maxdepth 1 -printf '.' | wc -c)
  [[ $entry_count == 2 ]] || return 1
  recorded_sysname=$(<"$state_sysname")
  [[ $recorded_sysname =~ ^[0-9]+-[0-9]+([.][0-9]+)*$ ]] || return 1
  assert_managed_passthrough "$state_xml" state yes
}

validate_check_identity() {
  local expected_bus=$1
  local expected_device=$2
  local config_dump live_dump recorded_sysname domain_state

  recorded_sysname=$(<"$CURRENT_STATE/sysname")
  [[ $recorded_sysname == "$sysname" ]] || return 1
  assert_passthrough_identity "$CURRENT_STATE/device.xml" state "$expected_bus" "$expected_device" || return 1

  config_dump=$(mktemp)
  temp_files+=("$config_dump")
  dump_domain_xml config "$config_dump" || return 1
  assert_passthrough_identity "$config_dump" config "$expected_bus" "$expected_device" || return 1

  domain_state=$(read_domain_state) || return 1
  if [[ $domain_state != 'shut off' && $domain_state != 'crashed' ]]; then
    live_dump=$(mktemp)
    temp_files+=("$live_dump")
    dump_domain_xml live "$live_dump" || return 1
    assert_passthrough_identity "$live_dump" live "$expected_bus" "$expected_device" || return 1
  fi
}

remove_state_directory() {
  local state_path=$1
  rm -f -- "$state_path/device.xml" "$state_path/sysname"
  rmdir -- "$state_path"
}

recover_pending_state() {
  [[ -e $PENDING_STATE || -L $PENDING_STATE ]] || return 0
  validate_state_directory "$PENDING_STATE" || die 'pending USB state is incomplete or unsafe; manual review is required'
  if ! detach_persistent "$PENDING_STATE/device.xml"; then
    die 'failed to recover a pending USB attachment; the root-only marker was retained'
  fi
  remove_state_directory "$PENDING_STATE" || die 'failed to clear recovered pending USB state'
}

for candidate_slot in $(seq 1 "$device_count"); do
  select_slot "$candidate_slot"
  [[ -d $SLOT_STATE && ! -L $SLOT_STATE ]] || die 'USB slot state directory is missing or unsafe'
  [[ $(stat -c '%U:%G:%a' "$SLOT_STATE") == root:root:700 ]] || die 'USB slot state directory must be root:root with mode 0700'
  if [[ -e $PENDING_STATE || -L $PENDING_STATE ]]; then
    [[ $action != check ]] || die 'a pending USB transaction requires recovery before a read-only check can pass'
    recover_pending_state
  fi
done

validate_domain_passthrough || die 'unauthorized hostdev passthrough is present; refusing USB operation'

for candidate_slot in $(seq 1 "$device_count"); do
  select_slot "$candidate_slot"
  refresh_alias_status || die 'cannot reconcile a managed USB slot with libvirt state'
  if [[ -e $CURRENT_STATE || -L $CURRENT_STATE ]]; then
    validate_state_directory "$CURRENT_STATE" || die 'current USB state is incomplete or unsafe; manual review is required'
    [[ $alias_in_config == true ]] || die 'trusted USB state is missing from persistent domain XML'
  elif [[ $alias_in_config == true || $alias_in_live == true ]]; then
    die 'domain has a managed VoCat USB alias without trusted local state'
  fi
done

event_slots=()
if [[ $action == attach || $action == check ]]; then
  for candidate_slot in $(seq 1 "$device_count"); do
    select_slot "$candidate_slot"
    matches_allowlist "$sysname" && event_slots+=("$candidate_slot")
  done
  ((${#event_slots[@]} == 1)) || die 'USB event must match exactly one path-bound allowlist entry'
else
  for candidate_slot in $(seq 1 "$device_count"); do
    select_slot "$candidate_slot"
    if [[ -f $CURRENT_STATE/sysname && ! -L $CURRENT_STATE/sysname && $(<"$CURRENT_STATE/sysname") == "$sysname" ]]; then
      event_slots+=("$candidate_slot")
    fi
  done
  ((${#event_slots[@]} <= 1)) || die 'USB detach event matches multiple trusted states'
  if ((${#event_slots[@]} == 0)); then
    exit 0
  fi
fi
select_slot "${event_slots[0]}"

current_state_exists=false
if [[ -e $CURRENT_STATE || -L $CURRENT_STATE ]]; then
  validate_state_directory "$CURRENT_STATE" || die 'current USB state is incomplete or unsafe; manual review is required'
  current_state_exists=true
fi

if [[ $action == check ]]; then
  [[ $current_state_exists == true ]] || die 'allowlisted USB device has no trusted attachment state'
  refresh_alias_status || die 'cannot inspect current hostdev aliases'
  [[ $alias_in_config == true ]] || die 'trusted USB state is not present in persistent domain XML'
  if [[ $domain_live_snapshot == true && $alias_in_live != true ]]; then
    die 'trusted USB state is not present in live domain XML'
  fi
fi

if [[ $action == check || $action == attach ]]; then
  matches_allowlist "$sysname" || die 'USB event does not match the complete allowlist'
  match_count=0
  for candidate_path in /sys/bus/usb/devices/*; do
    candidate=${candidate_path##*/}
    if [[ $candidate =~ ^[0-9]+-[0-9]+([.][0-9]+)*$ ]] && matches_allowlist "$candidate"; then
      ((match_count += 1))
    fi
  done
  ((match_count == 1)) || die 'expected exactly one connected USB device matching the complete allowlist'

  if [[ $action == check ]]; then
    bus_raw=$(</sys/bus/usb/devices/"$sysname"/busnum)
    device_raw=$(</sys/bus/usb/devices/"$sysname"/devnum)
    [[ $bus_raw =~ ^[0-9]{1,3}$ && $device_raw =~ ^[0-9]{1,3}$ ]] || die 'invalid USB bus or device number'
    bus_number=$((10#$bus_raw))
    device_number=$((10#$device_raw))
    ((1 <= bus_number && bus_number <= 255 && 1 <= device_number && device_number <= 255)) ||
      die 'USB bus or device number is outside the libvirt address range'
    validate_check_identity "$bus_number" "$device_number" ||
      die 'trusted USB state and libvirt hostdev addresses do not match the connected allowlisted device'
    printf 'USB allowlist check passed for slot %s (%s:%s); physical identity was matched but not displayed.\n' "$slot" "$vendor_id" "$product_id"
    exit 0
  fi
fi

if [[ $action == detach ]]; then
  if [[ $current_state_exists != true ]]; then
    refresh_alias_status || die 'cannot inspect hostdev aliases during detach'
    if [[ $alias_in_config == true || $alias_in_live == true ]]; then
      die 'domain has a VoCat USB alias but no trusted local state; manual review is required'
    fi
    exit 0
  fi
  recorded_sysname=$(<"$CURRENT_STATE/sysname")
  [[ $recorded_sysname == "$sysname" ]] || exit 0
  detach_persistent "$CURRENT_STATE/device.xml" || die 'failed to detach the allowlisted USB device from persistent domain state'
  remove_state_directory "$CURRENT_STATE" || die 'USB detached but trusted state could not be removed'
  validate_domain_passthrough || die 'hostdev validation failed after detach'
  printf 'Detached allowlisted DJI USB device from domain %s.\n' "$domain"
  exit 0
fi

if [[ $current_state_exists == true ]]; then
  detach_persistent "$CURRENT_STATE/device.xml" || die 'failed to replace the existing allowlisted USB attachment'
  remove_state_directory "$CURRENT_STATE" || die 'old USB attachment was removed but trusted state could not be cleared'
else
  refresh_alias_status || die 'cannot inspect existing hostdev aliases'
  if [[ $alias_in_config == true || $alias_in_live == true ]]; then
    die 'domain has a VoCat USB alias but no trusted local state; manual review is required'
  fi
fi

bus_raw=$(</sys/bus/usb/devices/"$sysname"/busnum)
device_raw=$(</sys/bus/usb/devices/"$sysname"/devnum)
[[ $bus_raw =~ ^[0-9]{1,3}$ && $device_raw =~ ^[0-9]{1,3}$ ]] || die 'invalid USB bus or device number'
bus_number=$((10#$bus_raw))
device_number=$((10#$device_raw))
((bus_number <= 255 && device_number <= 255)) || die 'USB bus or device number is outside the libvirt address range'

new_xml=$(mktemp)
new_sysname=$(mktemp)
temp_files+=("$new_xml" "$new_sysname")
printf '%s\n' \
  "<hostdev mode='subsystem' type='usb' managed='yes'>" \
  "  <source startupPolicy='optional'>" \
  "    <vendor id='0x$vendor_id'/>" \
  "    <product id='0x$product_id'/>" \
  "    <address bus='$bus_number' device='$device_number'/>" \
  '  </source>' \
  "  <alias name='$HOSTDEV_ALIAS'/>" \
  '</hostdev>' >"$new_xml"
printf '%s\n' "$sysname" >"$new_sysname"
assert_managed_passthrough "$new_xml" state yes || die 'generated USB hostdev XML failed validation'

install -d -o root -g root -m 0700 "$PENDING_STATE"
install -o root -g root -m 0600 "$new_xml" "$PENDING_STATE/device.xml"
install -o root -g root -m 0600 "$new_sysname" "$PENDING_STATE/sysname"
validate_state_directory "$PENDING_STATE" || die 'failed to stage a complete recoverable USB transaction'

if ! virsh --connect "$LIBVIRT_URI" attach-device "$domain" "$new_xml" --persistent; then
  if ! refresh_alias_status; then
    die 'USB attach outcome is unknown because libvirt state could not be reconciled; the root-only pending marker was retained'
  fi
  if [[ $alias_in_config == true || $alias_in_live == true ]]; then
    if ! detach_persistent "$new_xml"; then
      die 'USB attach returned an error after changing domain state; the root-only pending marker was retained'
    fi
  fi
  remove_state_directory "$PENDING_STATE" || true
  die 'failed to attach the allowlisted USB device persistently; no attachment remains'
fi

attachment_valid=true
validate_domain_passthrough || attachment_valid=false
refresh_alias_status || attachment_valid=false
if [[ $alias_in_config != true || ($domain_live_snapshot == true && $alias_in_live != true) ]]; then
  attachment_valid=false
fi
if [[ $attachment_valid != true ]]; then
  if detach_persistent "$new_xml"; then
    remove_state_directory "$PENDING_STATE" || true
    die 'USB attachment did not converge in live and persistent XML and was rolled back'
  fi
  die 'USB attachment validation failed and rollback failed; the root-only pending marker was retained'
fi

if ! mv -T -- "$PENDING_STATE" "$CURRENT_STATE"; then
  if detach_persistent "$new_xml"; then
    remove_state_directory "$PENDING_STATE" || true
    die 'USB state commit failed; the libvirt attachment was rolled back'
  fi
  die 'USB state commit and rollback failed; the root-only pending marker was retained'
fi

printf 'Attached path-bound DJI USB slot %s (%s:%s) to domain %s; production identity was not logged.\n' "$slot" "$vendor_id" "$product_id" "$domain"
