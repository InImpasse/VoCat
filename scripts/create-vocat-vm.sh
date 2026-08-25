#!/usr/bin/env bash
set -Eeuo pipefail
export LC_ALL=C

readonly GIB=$((1024 * 1024 * 1024))
readonly OVMF_CODE=/usr/share/OVMF/OVMF_CODE_4M.secboot.fd
readonly OVMF_VARS=/usr/share/OVMF/OVMF_VARS_4M.ms.fd

mode=check
vm_name=vocat
disk_dir=/var/lib/libvirt/images/vocat
lan_interface=
bulk_storage_root=
iso_path=
iso_sha256=
vcpus=
memory_mib=
disk_size_gib=
created_disk=0
domain_was_absent=0
disk_path=
inactive_domain_xml=
live_domain_xml=
iso_snapshot_staging=
verified_iso_snapshot=
created_iso_snapshot=0

usage() {
  cat <<'EOF'
Usage: create-vocat-vm.sh [--check | --dry-run | --create] [options]

Create or verify the constrained VoCat VM profile:
  Ubuntu Server 24.04, q35/KVM, UEFI Secure Boot, vTPM 2.0,
  configurable resources within reviewed bounds, virtio-scsi/discard,
  libvirt default NAT plus an explicitly selected macvtap interface.

Options:
  --name NAME             Libvirt domain name (default: vocat)
  --disk-dir PATH         SSD-backed qcow2 directory
  --lan-interface IFACE   macvtap parent (required)
  --bulk-storage-root PATH
                           non-SSD storage tree forbidden for VM disks
  --iso PATH              Ubuntu Server 24.04 amd64 ISO
  --iso-sha256 HEX        Expected SHA-256 from Ubuntu's signed checksum file
  --vcpus COUNT           2-4 virtual CPUs (required)
  --memory-mib MIB        2048-8192 MiB in 256 MiB increments (required)
  --disk-size-gib GIB     24-64 GiB thin qcow2 size (required)
  -h, --help              Show this help

--check verifies inactive and live domain state plus its qcow2. Pass --iso to
verify attached installation media; omit it to require an ejected CD-ROM.
--dry-run validates storage and ISO inputs and prints commands. --create must be
run as root. This script does not automate disk encryption: enter the LUKS
passphrase only in the local Ubuntu installer console so it never appears in a
file, argument, or log.
EOF
}

log() {
  printf '%s\n' "$*"
}

die() {
  printf 'ERROR: %s\n' "$*" >&2
  exit 1
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

cleanup_failed_create() {
  local status=$?
  local can_remove_disk=1
  [[ -z $inactive_domain_xml || ! -f $inactive_domain_xml ]] || rm -f -- "$inactive_domain_xml"
  [[ -z $live_domain_xml || ! -f $live_domain_xml ]] || rm -f -- "$live_domain_xml"
  [[ -z $iso_snapshot_staging || ! -f $iso_snapshot_staging ]] || rm -f -- "$iso_snapshot_staging"
  if ((status != 0 && domain_was_absent == 1)) && command -v virsh >/dev/null 2>&1 && \
      virsh --connect qemu:///system dominfo "$vm_name" >/dev/null 2>&1; then
    virsh --connect qemu:///system destroy "$vm_name" >/dev/null 2>&1 || true
    if ! virsh --connect qemu:///system undefine "$vm_name" --nvram --tpm >/dev/null 2>&1; then
      virsh --connect qemu:///system undefine "$vm_name" --nvram >/dev/null 2>&1 || true
    fi
    if virsh --connect qemu:///system dominfo "$vm_name" >/dev/null 2>&1; then
      can_remove_disk=0
      printf 'Creation failed and the new domain could not be undefined; preserving its qcow2 for manual recovery.\n' >&2
    fi
  fi
  if ((status != 0 && created_disk == 1 && can_remove_disk == 1)) && [[ -n $disk_path && -f $disk_path ]]; then
    printf 'Creation failed; removing the new, unused qcow2: %s\n' "$disk_path" >&2
    rm -f -- "$disk_path"
  fi
  if ((status != 0 && created_iso_snapshot == 1 && can_remove_disk == 1)) &&
      [[ -n $verified_iso_snapshot && -f $verified_iso_snapshot ]]; then
    printf 'Creation failed; removing the verified installer snapshot: %s\n' "$verified_iso_snapshot" >&2
    rm -f -- "$verified_iso_snapshot"
  fi
  exit "$status"
}
trap cleanup_failed_create EXIT

while (($#)); do
  case "$1" in
    --check|--dry-run|--create)
      mode=${1#--}
      shift
      ;;
    --name)
      (($# >= 2)) || die '--name requires a value'
      vm_name=$2
      shift 2
      ;;
    --disk-dir)
      (($# >= 2)) || die '--disk-dir requires a value'
      disk_dir=$2
      shift 2
      ;;
    --lan-interface)
      (($# >= 2)) || die '--lan-interface requires a value'
      lan_interface=$2
      shift 2
      ;;
    --bulk-storage-root)
      (($# >= 2)) || die '--bulk-storage-root requires a value'
      bulk_storage_root=$2
      shift 2
      ;;
    --iso)
      (($# >= 2)) || die '--iso requires a value'
      iso_path=$2
      shift 2
      ;;
    --iso-sha256)
      (($# >= 2)) || die '--iso-sha256 requires a value'
      iso_sha256=${2,,}
      shift 2
      ;;
    --vcpus)
      (($# >= 2)) || die '--vcpus requires a value'
      vcpus=$2
      shift 2
      ;;
    --memory-mib)
      (($# >= 2)) || die '--memory-mib requires a value'
      memory_mib=$2
      shift 2
      ;;
    --disk-size-gib)
      (($# >= 2)) || die '--disk-size-gib requires a value'
      disk_size_gib=$2
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

[[ $vm_name =~ ^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$ ]] || die 'invalid VM name'
[[ -n $lan_interface ]] || die '--lan-interface is required'
[[ $lan_interface =~ ^[A-Za-z0-9_.:-]{1,32}$ ]] || die 'invalid LAN interface name'
[[ -n $bulk_storage_root && $bulk_storage_root == /* ]] || die '--bulk-storage-root must be an absolute path'
[[ $vcpus =~ ^[0-9]{1,2}$ ]] || die '--vcpus must be an integer from 2 to 4'
vcpus=$((10#$vcpus))
((vcpus >= 2 && vcpus <= 4)) || die '--vcpus must be an integer from 2 to 4'
[[ $memory_mib =~ ^[0-9]{1,5}$ ]] || die '--memory-mib must be an integer from 2048 to 8192 in 256 MiB increments'
memory_mib=$((10#$memory_mib))
((memory_mib >= 2048 && memory_mib <= 8192 && memory_mib % 256 == 0)) ||
  die '--memory-mib must be an integer from 2048 to 8192 in 256 MiB increments'
[[ $disk_size_gib =~ ^[0-9]{1,3}$ ]] || die '--disk-size-gib must be an integer from 24 to 64'
disk_size_gib=$((10#$disk_size_gib))
((disk_size_gib >= 24 && disk_size_gib <= 64)) || die '--disk-size-gib must be an integer from 24 to 64'
min_free_gib=$((disk_size_gib * 2))
((min_free_gib >= 48)) || min_free_gib=48
min_free_bytes=$((min_free_gib * GIB))
for command_name in df findmnt lsblk realpath stat; do
  command -v "$command_name" >/dev/null 2>&1 || die "required storage command is missing: $command_name"
done
disk_dir=$(realpath -m -- "$disk_dir")
bulk_storage_root=$(realpath -m -- "$bulk_storage_root")
disk_path=$disk_dir/$vm_name.qcow2
[[ $disk_dir != "$bulk_storage_root" && $disk_dir != "$bulk_storage_root"/* ]] || die 'VM disks are forbidden on the configured bulk-storage tree; choose an SSD-backed path'

if [[ $mode == check ]]; then
  [[ -f $disk_path && ! -L $disk_path ]] || die "expected qcow2 is missing or is a symbolic link: $disk_path"
  storage_probe=$disk_path
else
  storage_probe=$disk_dir
  while [[ ! -e $storage_probe ]]; do
    next_probe=$(dirname -- "$storage_probe")
    [[ $next_probe != "$storage_probe" ]] || die 'cannot find an existing parent for the disk directory'
    storage_probe=$next_probe
  done
fi

mount_record=$(findmnt -n -o SOURCE,TARGET,FSTYPE -T "$storage_probe") || die 'cannot resolve the disk directory filesystem'
read -r mount_source mount_target _ <<<"$mount_record"
[[ $mount_target != "$bulk_storage_root" && $mount_target != "$bulk_storage_root"/* ]] || die 'VM disks are forbidden on the configured bulk-storage filesystem'

mapfile -t backing_rotational < <(lsblk -srno TYPE,ROTA "$mount_source" | awk '$1 == "disk" { print $2 }')
((${#backing_rotational[@]} > 0)) || die "cannot resolve physical backing disks for $mount_source"
for rotational in "${backing_rotational[@]}"; do
  [[ $rotational == 0 ]] || die "disk directory is not entirely SSD-backed ($mount_source)"
done

if [[ $mode != check ]]; then
  free_bytes=$(df -B1 --output=avail "$storage_probe" | awk 'NR == 2 { print $1 }')
  [[ $free_bytes =~ ^[0-9]+$ ]] || die 'cannot determine free SSD space'
  ((free_bytes >= min_free_bytes)) ||
    die "at least $min_free_gib GiB free is required before creating this VM"
fi

if [[ -n $iso_path ]]; then
  [[ -f $iso_path && ! -L $iso_path && -r $iso_path ]] || die 'ISO must be a readable, non-symlink regular file'
  iso_path=$(realpath -- "$iso_path")
fi

if [[ $mode != check ]]; then
  [[ -n $iso_path ]] || die '--iso is required for --dry-run and --create'
  [[ $iso_sha256 =~ ^[0-9a-f]{64}$ ]] || die '--iso-sha256 must be exactly 64 hexadecimal characters'
  actual_iso_sha256=$(sha256sum -- "$iso_path" | awk '{ print $1 }')
  [[ $actual_iso_sha256 == "$iso_sha256" ]] || die 'Ubuntu ISO SHA-256 mismatch'
fi

if [[ $mode == check ]]; then
  for command_name in virsh qemu-img python3; do
    command -v "$command_name" >/dev/null 2>&1 || die "required command is missing: $command_name"
  done
  virsh --connect qemu:///system dominfo "$vm_name" >/dev/null 2>&1 || die "libvirt domain does not exist: $vm_name"
  [[ $(stat -c '%U:%G:%a' "$disk_dir") == root:kvm:750 ]] || die 'VM disk directory must be root:kvm with mode 0750'
  [[ $(stat -c '%U:%G:%a' "$disk_path") == libvirt-qemu:kvm:640 ]] || die 'VM qcow2 must be libvirt-qemu:kvm with mode 0640'

  inactive_domain_xml=$(mktemp)
  live_domain_xml=$(mktemp)
  virsh --connect qemu:///system dumpxml --inactive "$vm_name" >"$inactive_domain_xml"
  virsh --connect qemu:///system dumpxml "$vm_name" >"$live_domain_xml"
  python3 - "$inactive_domain_xml" "$live_domain_xml" "$disk_path" "$lan_interface" "$OVMF_CODE" "$OVMF_VARS" "$iso_path" "$vcpus" "$memory_mib" <<'PY'
import os
import stat
import sys
import xml.etree.ElementTree as ET

inactive_xml_path, live_xml_path, disk_path, lan_interface, ovmf_code, ovmf_vars, expected_iso, expected_vcpus, expected_memory_mib = sys.argv[1:]
expected_vcpus = int(expected_vcpus)
expected_memory_mib = int(expected_memory_mib)

def require(condition, message):
    if not condition:
        raise SystemExit(f"ERROR: {message}")

def validate(xml_path, scope):
    root = ET.parse(xml_path).getroot()

    def scoped_require(condition, message):
        require(condition, f"{scope}: {message}")

    scoped_require(root.get("type") == "kvm", "domain type is not KVM")
    scoped_require(root.findtext("vcpu", "").strip() == str(expected_vcpus),
                   "vCPU count differs from the reviewed profile")
    memory = root.find("memory")
    scoped_require(memory is not None and memory.get("unit") == "KiB" and
                   int(memory.text or 0) == expected_memory_mib * 1024,
                   "memory differs from the reviewed profile")
    cpu = root.find("cpu")
    scoped_require(cpu is not None and cpu.get("mode") == "host-passthrough",
                   "CPU mode is not host-passthrough")

    os_type = root.find("./os/type")
    scoped_require(os_type is not None and "q35" in os_type.get("machine", ""), "machine type is not q35")
    loader = root.find("./os/loader")
    scoped_require(loader is not None and loader.get("type") == "pflash", "UEFI pflash loader is missing")
    scoped_require(loader.get("readonly") == "yes" and loader.get("secure") == "yes",
                   "UEFI Secure Boot is not enforced")
    scoped_require(os.path.realpath((loader.text or "").strip()) == os.path.realpath(ovmf_code),
                   "unexpected OVMF code image")
    nvram = root.find("./os/nvram")
    scoped_require(nvram is not None and os.path.realpath(nvram.get("template", "")) == os.path.realpath(ovmf_vars),
                   "Microsoft-enrolled OVMF variables template is missing")
    smm = root.find("./features/smm")
    scoped_require(smm is not None and smm.get("state") == "on", "SMM is not enabled for Secure Boot")

    storage_devices = root.findall("./devices/disk")
    disks = root.findall("./devices/disk[@device='disk']")
    cdroms = root.findall("./devices/disk[@device='cdrom']")
    scoped_require(len(disks) == 1, "domain must have exactly one guest disk")
    scoped_require(all(device.get("device") in {"disk", "cdrom"} for device in storage_devices),
                   "unauthorized storage device is present")
    scoped_require(len(cdroms) <= 1, "domain has more than one CD-ROM device")
    disk = disks[0]
    source = disk.find("source")
    driver = disk.find("driver")
    target = disk.find("target")
    source_file = source.get("file", "") if source is not None else ""
    scoped_require(source is not None and source_file == disk_path and
                   os.path.realpath(source_file) == os.path.realpath(disk_path),
                   "guest disk source is unexpected")
    scoped_require(driver is not None and driver.get("name") == "qemu" and driver.get("type") == "qcow2" and
                   driver.get("cache") == "none" and driver.get("discard") == "unmap" and
                   driver.get("detect_zeroes") == "unmap", "guest disk driver hardening is incomplete")
    scoped_require(target is not None and target.get("bus") == "scsi", "guest disk is not on virtio-scsi")

    if expected_iso:
        scoped_require(len(cdroms) == 1, "expected installation ISO is not attached")
    for cdrom in cdroms:
        cdrom_source = cdrom.find("source")
        cdrom_driver = cdrom.find("driver")
        scoped_require(cdrom.get("type") == "file" and cdrom.find("readonly") is not None,
                       "CD-ROM must be a read-only file device")
        scoped_require(cdrom_driver is not None and cdrom_driver.get("name") == "qemu" and
                       cdrom_driver.get("type") == "raw", "CD-ROM driver must use the raw format")
        cdrom_ejected = (cdrom_source is None or
                         (not cdrom_source.attrib and not list(cdrom_source)))
        if expected_iso and scope == "inactive" and cdrom_ejected:
            continue
        if not expected_iso:
            scoped_require(cdrom_ejected,
                           "CD-ROM media must be ejected unless --iso is supplied")
            continue
        cdrom_file = cdrom_source.get("file", "") if cdrom_source is not None else ""
        scoped_require(cdrom_source is not None and set(cdrom_source.attrib) == {"file"} and
                       cdrom_file == expected_iso and os.path.realpath(cdrom_file) == expected_iso,
                       "CD-ROM source is not the expected ISO")
        try:
            iso_stat = os.lstat(cdrom_file)
        except OSError as error:
            raise SystemExit(f"ERROR: {scope}: cannot inspect attached ISO: {error}") from error
        scoped_require(stat.S_ISREG(iso_stat.st_mode) and not stat.S_ISLNK(iso_stat.st_mode),
                       "CD-ROM source must be a non-symlink regular ISO file")

    controller = root.find("./devices/controller[@type='scsi']")
    scoped_require(controller is not None and controller.get("model") == "virtio-scsi",
                   "virtio-scsi controller is missing")

    interfaces = root.findall("./devices/interface")
    scoped_require(len(interfaces) == 2, "domain must have exactly two network interfaces")
    has_nat = any(i.get("type") == "network" and i.find("model") is not None and
                  i.find("model").get("type") == "virtio" and i.find("source") is not None and
                  i.find("source").get("network") == "default" for i in interfaces)
    has_macvtap = any(i.get("type") == "direct" and i.find("model") is not None and
                      i.find("model").get("type") == "virtio" and i.find("source") is not None and
                      i.find("source").get("dev") == lan_interface and
                      i.find("source").get("mode") == "bridge" for i in interfaces)
    scoped_require(has_nat, "default NAT management interface is missing")
    scoped_require(has_macvtap, "LAN macvtap interface is missing or incorrect")

    tpm = root.find("./devices/tpm")
    scoped_require(tpm is not None and tpm.get("model") == "tpm-crb", "TPM 2.0 CRB device is missing")
    tpm_backend = tpm.find("backend") if tpm is not None else None
    scoped_require(tpm_backend is not None and tpm_backend.get("type") == "emulator" and
                   tpm_backend.get("version") == "2.0", "TPM backend is not an emulated TPM 2.0")
    channels = root.findall("./devices/channel")
    allowed_channels = {
        "org.qemu.guest_agent.0": "unix",
        "com.redhat.spice.0": "spicevmc",
    }
    seen_channels = set()
    for channel_device in channels:
        channel_target = channel_device.find("target")
        channel_name = channel_target.get("name", "") if channel_target is not None else ""
        scoped_require(channel_name in allowed_channels and channel_name not in seen_channels and
                       channel_device.get("type") == allowed_channels[channel_name] and
                       channel_target.get("type") == "virtio", "unauthorized guest channel is present")
        seen_channels.add(channel_name)
    scoped_require("org.qemu.guest_agent.0" in seen_channels, "QEMU guest-agent channel is missing")
    serials = root.findall("./devices/serial")
    consoles = root.findall("./devices/console")
    scoped_require(len(serials) <= 1 and all(device.get("type") == "pty" for device in serials),
                   "unauthorized serial device is present")
    scoped_require(len(consoles) <= 1 and all(device.get("type") == "pty" for device in consoles),
                   "unauthorized console device is present")
    scoped_require(not root.findall("./devices/parallel"), "parallel devices are forbidden")

    graphics_devices = root.findall("./devices/graphics")
    scoped_require(len(graphics_devices) == 1, "domain must have exactly one graphics device")
    graphics = graphics_devices[0]
    listen_addresses = [graphics.get("listen", "")]
    listen_addresses.extend(item.get("address", "") for item in graphics.findall("listen"))
    listen_addresses = [address for address in listen_addresses if address]
    scoped_require(graphics.get("type") == "spice" and listen_addresses and
                   all(address == "127.0.0.1" for address in listen_addresses),
                   "SPICE console is not restricted to loopback")
    scoped_require(not root.findall("./devices/filesystem"), "host filesystem passthrough is forbidden")
    scoped_require(not root.findall("./devices/redirdev"), "USB redirection devices are forbidden")
    scoped_require(not root.findall("./devices/smartcard"), "smartcard passthrough is forbidden")
    scoped_require(not any(str(element.tag) == "commandline" or str(element.tag).endswith("}commandline")
                           for element in root.iter()), "custom QEMU command-line arguments are forbidden")

    hostdevs = root.findall("./devices/hostdev")
    scoped_require(len(hostdevs) <= 1, "unexpected extra hostdev passthrough is present")
    signatures = []
    for hostdev in hostdevs:
        source = hostdev.find("source")
        alias = hostdev.find("alias")
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
        scoped_require(set(hostdev.attrib) == {"mode", "type", "managed"} and
                       hostdev.get("mode") == "subsystem" and hostdev.get("type") == "usb" and
                       hostdev.get("managed") == "yes" and source is not None and alias is not None and
                       len(hostdev.findall("source")) == 1 and len(hostdev.findall("alias")) == 1 and
                       all(child.tag in {"source", "alias", "address"} for child in hostdev) and
                       startup_ok and set(source.attrib) <= {"startupPolicy"} and
                       [child.tag for child in source].count("vendor") == 1 and
                       [child.tag for child in source].count("product") == 1 and
                       [child.tag for child in source].count("address") == 1 and
                       all(child.tag in {"vendor", "product", "address"} for child in source) and
                       vendor is not None and vendor.attrib == {"id": "0x2ca3"} and
                       product is not None and product.attrib == {"id": "0x4006"} and
                       address is not None and set(address.attrib) == {"bus", "device"} and
                       1 <= bus <= 255 and 1 <= device <= 255 and
                       alias.attrib == {"name": "ua-vocat-dji-usb"},
                       "unauthorized USB or PCI/controller passthrough is present")
        guest_addresses = hostdev.findall("address")
        scoped_require(len(guest_addresses) <= 1 and
                       all(item.get("type") == "usb" for item in guest_addresses),
                       "USB hostdev has an unauthorized guest address")
        signatures.append(("2ca3", "4006", bus, device, "ua-vocat-dji-usb"))
    return tuple(signatures)

inactive_hostdevs = validate(inactive_xml_path, "inactive")
live_hostdevs = validate(live_xml_path, "live")
require(inactive_hostdevs == live_hostdevs, "inactive and live USB hostdev definitions differ")
PY

  qemu-img info --force-share --output=json "$disk_path" | python3 -c '
import json, sys
info = json.load(sys.stdin)
expected_size = int(sys.argv[1]) * 1024**3
if info.get("format") != "qcow2" or info.get("virtual-size") != expected_size:
    raise SystemExit("ERROR: disk size differs from the reviewed profile")
if any(info.get(key) for key in ("backing-filename", "full-backing-filename", "data-file")):
    raise SystemExit("ERROR: qcow2 backing or external data files are forbidden")
format_data = info.get("format-specific", {}).get("data", {})
if any(format_data.get(key) for key in ("backing-filename", "data-file", "data-file-raw")):
    raise SystemExit("ERROR: qcow2 backing or external data files are forbidden")
' "$disk_size_gib"
  virsh --connect qemu:///system dominfo "$vm_name" | grep -Eq '^Autostart:[[:space:]]+enable' || die 'domain autostart is not enabled'
  rm -f -- "$inactive_domain_xml" "$live_domain_xml"
  inactive_domain_xml=
  live_domain_xml=
  log 'VM profile checks passed. Guest LUKS unlock and service readiness are separate gates.'
  exit 0
fi

for command_name in qemu-img runuser virt-install virsh; do
  if [[ $mode == create ]]; then
    command -v "$command_name" >/dev/null 2>&1 || die "required command is missing: $command_name"
  fi
done
[[ -r $OVMF_CODE ]] || [[ $mode == dry-run ]] || die "Secure Boot firmware is missing: $OVMF_CODE"
[[ -r $OVMF_VARS ]] || [[ $mode == dry-run ]] || die "Secure Boot variables template is missing: $OVMF_VARS"
ip link show dev "$lan_interface" >/dev/null 2>&1 || die "LAN interface not found: $lan_interface"

if [[ $mode == create ]]; then
  ((EUID == 0)) || die '--create must be run as root from a private local terminal'
  [[ ! -e $disk_path ]] || die "refusing to overwrite existing disk: $disk_path"
  ! virsh --connect qemu:///system dominfo "$vm_name" >/dev/null 2>&1 || die "refusing to overwrite existing domain: $vm_name"
  domain_was_absent=1
  install -d -m 0750 -o root -g kvm "$disk_dir"

  verified_iso_snapshot="$disk_dir/$vm_name-installer-$iso_sha256.iso"
  [[ ! -e $verified_iso_snapshot && ! -L $verified_iso_snapshot ]] ||
    die "refusing to overwrite existing installer snapshot: $verified_iso_snapshot"
  iso_snapshot_staging=$(mktemp "$disk_dir/.${vm_name}-installer.XXXXXX.iso")
  install -o root -g kvm -m 0640 "$iso_path" "$iso_snapshot_staging"
  snapshot_metadata=$(stat -c '%F:%U:%G:%a:%h' -- "$iso_snapshot_staging") ||
    die 'cannot inspect installer snapshot'
  [[ $snapshot_metadata == 'regular file:root:kvm:640:1' ]] ||
    die 'installer snapshot has unsafe type, ownership, mode, or link count'
  snapshot_sha256=$(sha256sum -- "$iso_snapshot_staging" | awk '{ print $1 }')
  [[ $snapshot_sha256 == "$iso_sha256" ]] || die 'verified installer snapshot SHA-256 mismatch'
  mv -T -- "$iso_snapshot_staging" "$verified_iso_snapshot"
  iso_snapshot_staging=
  created_iso_snapshot=1
  iso_path=$verified_iso_snapshot
  runuser -u libvirt-qemu -- test -r "$iso_path" || die 'libvirt-qemu cannot read the verified installer snapshot'
else
  [[ ! -e $disk_path ]] || die "dry-run target already exists: $disk_path"
  verified_iso_snapshot="$disk_dir/$vm_name-installer-$iso_sha256.iso"
  log "DRY RUN: --create would copy and re-hash the ISO as $verified_iso_snapshot before virt-install."
  iso_path=$verified_iso_snapshot
fi

run qemu-img create -f qcow2 -o preallocation=metadata,lazy_refcounts=on "$disk_path" "${disk_size_gib}G"
[[ $mode == dry-run ]] || created_disk=1
run chown libvirt-qemu:kvm "$disk_path"
run chmod 0640 "$disk_path"

virt_install_args=(
  --connect qemu:///system
  --name "$vm_name"
  --machine q35
  --virt-type kvm
  --cpu host-passthrough
  --vcpus "$vcpus"
  --memory "$memory_mib"
  --features smm=on
  --boot "loader=$OVMF_CODE,loader.readonly=yes,loader.type=pflash,loader.secure=yes,nvram.template=$OVMF_VARS"
  --tpm "backend.type=emulator,backend.version=2.0,model=tpm-crb"
  --controller "type=scsi,model=virtio-scsi"
  --disk "path=$disk_path,format=qcow2,bus=scsi,cache=none,discard=unmap,detect_zeroes=unmap"
  --cdrom "$iso_path"
  --os-variant ubuntu24.04
  --network "network=default,model=virtio"
  --network "type=direct,source=$lan_interface,source_mode=bridge,model=virtio"
  --graphics "spice,listen=127.0.0.1"
  --video virtio
  --console "pty,target.type=serial"
  --channel "unix,target.type=virtio,target.name=org.qemu.guest_agent.0"
  --rng /dev/urandom
  --autostart
  --noautoconsole
)
run virt-install "${virt_install_args[@]}"

if [[ $mode == dry-run ]]; then
  log 'Dry run only. No disk or domain was created.'
  exit 0
fi

if ! "$0" --check --name "$vm_name" --disk-dir "$disk_dir" --bulk-storage-root "$bulk_storage_root" \
  --lan-interface "$lan_interface" --iso "$iso_path" --vcpus "$vcpus" \
  --memory-mib "$memory_mib" --disk-size-gib "$disk_size_gib"; then
  die 'new domain failed reviewed-profile validation and will be removed'
fi
created_disk=0
domain_was_absent=0
log "VM created. Open its local console and install Ubuntu 24.04 using encrypted LVM/LUKS."
log "Verified installer snapshot retained until ISO ejection: $verified_iso_snapshot"
log 'Do not place the LUKS passphrase in shell history, cloud-init, or tracked files.'
