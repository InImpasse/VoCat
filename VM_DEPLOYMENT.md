# VoCat VM Deployment

**English** | [简体中文](VM_DEPLOYMENT.zh-CN.md)

This is the canonical production deployment guide for the hardened branch. It
uses a minimized Ubuntu Server 24.04 guest, systemd, one libvirt NAT interface,
and a host-side reverse proxy. Docker is used only by the reproducible build;
Docker does not run inside the production guest.

> [!IMPORTANT]
> A passing build is only a candidate. Production acceptance also requires the
> reviewed VM profile, the exact artifact checks below, the host reverse-proxy
> boundary, and real modem acceptance. Never deploy an upstream build directly.

## Security model

- Keep the VM qcow2, current release, and live SQLite database on SSD storage.
- Use bulk storage only for ISO files, build caches, and encrypted backups.
- Give the guest exactly one virtio interface on libvirt's `default` NAT network.
- Do not attach the guest directly to the LAN and do not install Tailscale in it.
- Publish the web UI only through a host reverse proxy. Guest TCP/7575 accepts
  loopback and one privately configured host-proxy source IPv4 address.
- Run VoCat directly under the hardened systemd unit. Do not run privileged
  Docker in the guest.
- Keep passwords, recovery keys, SIM identifiers, private addresses, USB
  identity, and notification credentials out of Git, command output, and logs.
- `CAP_NET_ADMIN` and `CAP_NET_RAW` are residual risks. Guest nftables limits
  normal ingress but is not a containment boundary after a VoCat compromise.

## 1. Keep host settings private

Host-specific settings belong in the ignored project-local file
`.vocat-local/host-deployment.json`, mode `0600`. Do not commit a populated
copy. Schema 3 has this shape:

```json
{
  "schema": 3,
  "admin_user": "<HOST_ADMIN_USER>",
  "bulk_storage_root": "<ABSOLUTE_BULK_STORAGE_PATH>",
  "vm": {
    "name": "vocat",
    "disk_dir": "<ABSOLUTE_SSD_VM_DIRECTORY>",
    "vcpus": 2,
    "memory_mib": 2048,
    "disk_size_gib": 24
  },
  "installer": {
    "iso": "<ABSOLUTE_UBUNTU_SERVER_ISO_PATH>",
    "sha256": "<SHA256_FROM_UBUNTU_SIGNED_CHECKSUMS>"
  }
}
```

Verify the local protection before using it:

```bash
chmod 0600 .vocat-local/host-deployment.json
git check-ignore .vocat-local/host-deployment.json
./.vocat-local/vocat-hostctl validate
```

The ISO digest must come from Ubuntu's signed `SHA256SUMS`, not from an
unauthenticated mirror response. The controller accepts only 2-4 vCPUs,
2048-8192 MiB RAM in 256 MiB increments, and a 24-64 GiB thin qcow2 disk.

## 2. Prepare the KVM host

Run the read-only check first. Apply changes only from a private local terminal:

```bash
./.vocat-local/vocat-hostctl host-check
sudo ./.vocat-local/vocat-hostctl host-apply
```

Log out and back in after group changes, then confirm membership in `libvirt`
and `kvm` and rerun `host-check`. Warnings about static
`qemu-guest-agent.service` enablement are expected; `systemctl is-active
qemu-guest-agent` must report `active` in the guest.

Before VM creation, the SSD must have at least `max(48 GiB, 2 x disk size)`
available. Do not use `docker system prune --volumes` to make room.

## 3. Create and install the VM

Review the generated command before creating anything:

```bash
sudo ./.vocat-local/vocat-hostctl vm-dry-run
sudo ./.vocat-local/vocat-hostctl vm-create
```

The reviewed lightweight profile is q35/KVM, UEFI Secure Boot, vTPM 2.0,
2 vCPUs, 2048 MiB RAM, a 24 GiB thin qcow2 disk, virtio-scsi/discard, and one
libvirt `default` NAT NIC.

Connect only from the host:

```bash
virt-viewer --connect qemu:///system vocat
# After the serial console is configured:
virsh --connect qemu:///system console vocat
```

In the installer:

1. Select `Ubuntu Server (minimized)`.
2. Do not install a GUI, third-party drivers, or optional snaps.
3. Use LVM on the whole virtual disk and enable LUKS encryption.
4. Enter the LUKS passphrase only in the private console and keep the recovery
   key offline.
5. Install the OpenSSH server only if it is needed for host-to-guest transfer;
   expose it only on the private libvirt network.

After reboot, unlock LUKS from the console, eject the installer ISO from both
inactive and live domain state, and run:

```bash
sudo ./.vocat-local/vocat-hostctl vm-check-installed
```

The check must confirm the resource profile, SSD-backed qcow2, one NAT NIC, no
attached installer, and no unexpected storage, network, PCI, or USB devices.
Only then may the verified installer snapshot be removed.

If the VM is already installed and this check passes, continue at section 4.

## 4. Build and record the candidate

Build only a clean, reviewed, committed SHA. Push/PR CI must pass before
production deployment. A local candidate is built with:

```bash
git status --short
git rev-parse HEAD
VOCAT_BUILD_CACHE_ROOT="$(jq -er '.bulk_storage_root' \
  .vocat-local/host-deployment.json)/cache/vocat" \
  ./scripts/build-hardened.sh amd64
```

This reads the cache location from the private JSON for this one process; it
does not require an exported shell variable or a tracked host path.

The build uses pinned temporary containers and produces
`dist/hardened/<40_HEX_COMMIT>/`. Record these two values through a trusted
channel independent of artifact transfer:

```text
reviewed commit:        <40_HEX_COMMIT>
SHA256SUMS index hash:  <64_HEX_SHA256_OF_SHA256SUMS>
```

Do not derive either expected value from files after they arrive in the guest.
The artifact must contain schema 2 evidence, Gitleaks, full and production npm
audits, normal/race Go tests, vet, source and binary govulncheck reports, source
and binary SBOMs, Go 1.26.7 binary metadata, and whole-artifact checksums.

## 5. Prepare the guest

Transfer a `git archive` of the exact reviewed commit plus the complete artifact
directory over the private host-to-guest path. Do not copy a dirty working tree.
For example, run on the host after replacing both placeholders:

```bash
reviewed_commit='<40_HEX_COMMIT>'
guest_target='<GUEST_LOGIN>@<VM_PRIVATE_IPV4>'
ssh "$guest_target" 'install -d -m 0700 "$HOME/vocat-transfer"'
git archive --format=tar "$reviewed_commit" |
  ssh "$guest_target" 'tar -xf - -C "$HOME/vocat-transfer"'
scp -r "dist/hardened/$reviewed_commit" \
  "$guest_target:vocat-transfer/artifact"
unset reviewed_commit guest_target
```

Then enter the source snapshot in the guest and run:

```bash
cd "$HOME/vocat-transfer"
sudo ./scripts/prepare-vocat-guest.sh --dry-run \
  --proxy-source-ipv4 <HOST_PROXY_SOURCE_IPV4>
sudo ./scripts/prepare-vocat-guest.sh --apply \
  --proxy-source-ipv4 <HOST_PROXY_SOURCE_IPV4>
sudo ./scripts/prepare-vocat-guest.sh --check \
  --proxy-source-ipv4 <HOST_PROXY_SOURCE_IPV4>
```

Use the source address that the host reverse proxy actually uses when connecting
to the guest, not the LAN client address. Keep it in private deployment records.

The script installs the guest agent, QMI tools, SQLite, nftables, and service
accounts; removes legacy Tailscale state; masks ModemManager; installs the
hardened systemd units; and limits TCP/7575 to loopback and the exact proxy
source. It does not authenticate a SIM or upgrade modem firmware.

## 6. Deploy the exact artifact

Run deployment from the guest's private TTY. The first deployment prompts twice
for the initial administrator password; the password is not accepted as a
command argument and must never be stored in the repository.

```bash
sudo ./scripts/deploy-hardened.sh \
  --expected-commit <40_HEX_COMMIT> \
  --expected-index-sha256 <64_HEX_SHA256_OF_SHA256SUMS> \
  <GUEST_PATH_TO_COMPLETE_ARTIFACT_DIRECTORY>
```

The deployer authenticates the complete inventory before parsing JSON, reruns
the evidence checks, preflights SQLite migration in isolation, snapshots the
live database consistently, switches `/opt/vocat/current`, and restores both
the previous binary and database if readiness fails.

Verify in the guest:

```bash
sudo systemctl status vocat.service --no-pager
sudo systemctl is-active vocat.service vocat-firewall.service
curl --fail --silent --show-error http://127.0.0.1:7575/readyz
sudo ss -ltnp 'sport = :7575'
```

Only the VoCat MainPID may own the 7575 listener.

## 7. Publish through the host reverse proxy

The host proxy is the only LAN-facing endpoint. Use HTTPS, restrict the listener
to the intended LAN interface and client CIDR, redirect or close plain HTTP,
preserve WebSocket upgrades, disable response buffering for streaming APIs, add
HSTS, and add the `Secure` flag to upstream cookies. Keep VoCat
`trust_proxy_headers=false`; use the proxy's own access log for LAN client
attribution.

The essential routing relationship is:

```text
LAN client --HTTPS--> host reverse proxy --HTTP/libvirt NAT--> VM:7575
```

For Nginx, use placeholders in the reviewed template and keep the populated
file only on the host. The `map` belongs in the `http` context:

```nginx
map $http_upgrade $vocat_connection_upgrade {
    default upgrade;
    ''      close;
}

server {
    listen <HOST_LAN_IPV4>:443 ssl;
    server_name <INTERNAL_DNS_NAME>;

    ssl_certificate     <CERTIFICATE_PATH>;
    ssl_certificate_key <PRIVATE_KEY_PATH>;
    add_header Strict-Transport-Security "max-age=31536000" always;

    allow <TRUSTED_LAN_CIDR>;
    deny all;

    location / {
        proxy_pass http://<VM_PRIVATE_IPV4>:7575;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $vocat_connection_upgrade;
        proxy_set_header X-Forwarded-Proto https;
        proxy_buffering off;
        proxy_cookie_flags ~ secure;
    }
}
```

Validate and reload without interrupting a working proxy:

```bash
sudo nginx -t
sudo systemctl reload nginx
```

After applying the proxy configuration, verify all three paths:

1. HTTPS through the host proxy reaches `/readyz` and the login page.
2. Host-to-guest TCP/7575 succeeds from the configured proxy source.
3. A non-proxy source cannot connect directly to guest TCP/7575.

Do not add a LAN route, macvtap NIC, port forward, or direct firewall exception
to the guest.

## 8. Optional DJI USB passthrough

USB acceptance is separate from application deployment. The current hardened
enrollment accepts all connected `2ca3:4006` devices. Each device must have a
unique, stable udev `ID_PATH`. A USB serial is optional because some reviewed
devices do not expose one; when present, it is an additional required match.
Serial-less devices are therefore bound to their enrolled physical USB ports.
Moving one to another port is rejected. Never weaken the allowlist to
VID/PID-only matching.

List matching sysfs names without printing identifiers:

```bash
for device_path in /sys/bus/usb/devices/*; do
  [ -r "$device_path/idVendor" ] && [ -r "$device_path/idProduct" ] || continue
  [ "$(<"$device_path/idVendor")" = 2ca3 ] && \
    [ "$(<"$device_path/idProduct")" = 4006 ] && basename "$device_path"
done
```

To discover and enroll every currently connected matching device:

```bash
sudo ./scripts/configure-dji-usb-passthrough.sh --check
sudo ./scripts/configure-dji-usb-passthrough.sh --dry-run
sudo ./scripts/configure-dji-usb-passthrough.sh --apply
```

Use repeated `--sysname` only to enroll an explicit subset. The script stores
real identities only in a root-only host file, assigns independent aliases and
recoverable state to every enrolled managed USB hostdev, and rejects duplicate
paths, unexpected aliases, and PCI/xHCI passthrough. Run `--apply` again after
adding a new physical device; already enrolled devices hotplug automatically.
Validate each device independently after cold boot, hotplug, unplug/replug, and
VM restart before modem testing.

Inside the guest, an exact `2ca3:4006` USB add event starts an isolated repair
instance. The repair serializes concurrent events, binds interfaces 0-3 to
`option`, asserts DTR, binds interface 4 to `qmi_wwan`, and performs a read-only
QMI DMS check for every connected matching device. It does not write modem NV,
firmware, or SIM state. After first installing this automation while devices
are already attached, trigger only the reviewed composition once:

```bash
sudo udevadm trigger --action=add --subsystem-match=usb \
  --attr-match=idVendor=2ca3 --attr-match=idProduct=4006
```

For private diagnostics, run `sudo /opt/vocat/current/vocat doctor
--repair-dji-qmi --timeout 60s`. Automatic instances suppress detailed topology
from journald; the manual command is intended only for a private console.

Read firmware only during a maintenance window. Stop VoCat first and use
`scripts/read-dji-firmware.sh`; it sends only `ATI`, `AT+CGMM`, and `AT+CGMR`.
Do not upgrade firmware during the initial deployment.

## 9. Acceptance and rollback

Production acceptance requires all of the following:

- candidate CI and the local artifact gate pass for the exact deployed SHA;
- `vm-check-installed` and guest preparation checks pass;
- host proxy access succeeds and direct guest access is rejected;
- SIM is Ready without exposing IMSI, ICCID, IMEI, PIN, or SMS content;
- the carrier profile is correct, then ePDG/IKE and IMS registration succeed;
- one authorized SMS test succeeds; call testing is performed only if required;
- reboot, LUKS unlock, USB re-enumeration, and a failed-release rollback work;
- the deployment soaks for 24-48 hours without changing unrelated host services.

Back up SQLite only with the service stopped or through SQLite's online backup
API. Encrypt VM and SQLite backups before writing them to bulk storage, keep the
key elsewhere, and retain a verified off-host copy. A single bulk-storage disk
is not a backup.
