#!/usr/bin/env bash
set -Eeuo pipefail
export LC_ALL=C

readonly REQUIRED_PACKAGES=(
  qemu-system-x86
  qemu-utils
  libvirt-daemon-system
  libvirt-clients
  virtinst
  ovmf
  python3
  swtpm
  swtpm-tools
  dnsmasq-base
  virt-viewer
)

mode=check
admin_user=${SUDO_USER:-}
lan_interface=

usage() {
  cat <<'EOF'
Usage: prepare-kvm-host.sh [--check | --dry-run | --apply] [options]

Prepare an Ubuntu 24.04 production host for a libvirt/KVM VoCat guest.
The default --check mode is read-only. --apply must be run as root from a
private local terminal; the script never invokes sudo itself.

Options:
  --admin-user USER       Add USER to the libvirt and kvm groups on --apply
  --lan-interface IFACE   Verify the explicitly selected macvtap parent
  -h, --help              Show this help

This script does not remove LXD, create a VM, or change existing guests.
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

valid_name() {
  [[ $1 =~ ^[a-z_][a-z0-9_-]{0,31}$ ]]
}

while (($#)); do
  case "$1" in
    --check|--dry-run|--apply)
      mode=${1#--}
      shift
      ;;
    --admin-user)
      (($# >= 2)) || die '--admin-user requires a value'
      admin_user=$2
      shift 2
      ;;
    --lan-interface)
      (($# >= 2)) || die '--lan-interface requires a value'
      lan_interface=$2
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

[[ $(uname -m) == x86_64 ]] || die 'this VM profile requires an x86_64 host'
[[ -n $lan_interface ]] || die '--lan-interface is required'
[[ $lan_interface =~ ^[A-Za-z0-9_.:-]{1,32}$ ]] || die 'invalid LAN interface name'
if [[ -n $admin_user ]]; then
  valid_name "$admin_user" || die 'invalid admin user name'
  id "$admin_user" >/dev/null 2>&1 || die 'admin user does not exist'
fi

if ! grep -Eqm1 '(^|[[:space:]])(vmx|svm)($|[[:space:]])' /proc/cpuinfo; then
  die 'CPU virtualization extensions are unavailable or disabled in firmware'
fi
ip link show dev "$lan_interface" >/dev/null 2>&1 || die "LAN interface not found: $lan_interface"

if command -v apt-cache >/dev/null 2>&1; then
  for package_name in "${REQUIRED_PACKAGES[@]}"; do
    package_candidate=$(apt-cache policy "$package_name" | awk '/Candidate:/ && !found { print $2; found=1 }')
    [[ -n $package_candidate && $package_candidate != '(none)' ]] || die "no installable Ubuntu 24.04 candidate for package: $package_name"
  done
fi

if [[ $mode == apply ]]; then
  ((EUID == 0)) || die '--apply must be run as root from a private terminal'
  run env DEBIAN_FRONTEND=noninteractive apt-get update
  run env DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends "${REQUIRED_PACKAGES[@]}"
  run systemctl enable --now libvirtd.service

  if [[ -n $admin_user ]]; then
    run usermod --append --groups libvirt,kvm "$admin_user"
    log "Group membership for $admin_user takes effect at the next login."
  fi

  if ! virsh --connect qemu:///system net-info default >/dev/null 2>&1; then
    die 'libvirt default NAT network was not created; stop and inspect package configuration'
  fi
  if ! virsh --connect qemu:///system net-info default | grep -Eq '^Active:[[:space:]]+yes$'; then
    run virsh --connect qemu:///system net-start default
  fi
  run virsh --connect qemu:///system net-autostart default
fi

if [[ $mode == dry-run ]]; then
  run env DEBIAN_FRONTEND=noninteractive apt-get update
  run env DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends "${REQUIRED_PACKAGES[@]}"
  run systemctl enable --now libvirtd.service
  if [[ -n $admin_user ]]; then
    run usermod --append --groups libvirt,kvm "$admin_user"
  fi
  run virsh --connect qemu:///system net-start default
  run virsh --connect qemu:///system net-autostart default
  exit 0
fi

missing=0
for command_name in python3 qemu-system-x86_64 qemu-img virsh virt-install swtpm virt-host-validate; do
  if ! command -v "$command_name" >/dev/null 2>&1; then
    printf 'MISSING: %s\n' "$command_name" >&2
    missing=1
  fi
done
((missing == 0)) || die 'required KVM tools are not installed'

[[ -c /dev/kvm ]] || die '/dev/kvm is unavailable'
[[ -r /dev/kvm && -w /dev/kvm ]] || die 'the current user cannot access /dev/kvm'

ovmf_code=/usr/share/OVMF/OVMF_CODE_4M.secboot.fd
ovmf_vars=/usr/share/OVMF/OVMF_VARS_4M.ms.fd
[[ -r $ovmf_code ]] || die "Secure Boot firmware is missing: $ovmf_code"
[[ -r $ovmf_vars ]] || die "Microsoft-enrolled OVMF variables are missing: $ovmf_vars"

virsh --connect qemu:///system list --all >/dev/null || die 'cannot connect to qemu:///system'
network_info=$(virsh --connect qemu:///system net-info default 2>/dev/null) || die 'libvirt default NAT network is missing'
grep -Eq '^Active:[[:space:]]+yes$' <<<"$network_info" || die 'libvirt default NAT network is inactive'
grep -Eq '^Autostart:[[:space:]]+yes$' <<<"$network_info" || die 'libvirt default NAT network is not set to autostart'
virt-host-validate qemu || die 'libvirt host validation reported a required capability failure'

if command -v snap >/dev/null 2>&1 && snap list lxd >/dev/null 2>&1; then
  log 'WARNING: an LXD snap is installed. Confirm it has no instances, storage, or networks before any manual removal.'
fi

log 'KVM host checks passed. No VM or USB passthrough was created.'
