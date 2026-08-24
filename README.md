<p align="center">
  <img src="web/public/favicon.svg" width="96" alt="Vocat">
</p>

<h1 align="center">VoCat</h1>

<p align="center">
  <img alt="Go" src="https://img.shields.io/badge/Go-1.25-00ADD8?style=flat-square&logo=go&logoColor=white">
  <img alt="React" src="https://img.shields.io/badge/React-19-61DAFB?style=flat-square&logo=react&logoColor=111111">
  <img alt="TypeScript" src="https://img.shields.io/badge/TypeScript-5.8-3178C6?style=flat-square&logo=typescript&logoColor=white">
  <img alt="Vite" src="https://img.shields.io/badge/Vite-7-646CFF?style=flat-square&logo=vite&logoColor=white">
  <img alt="Tailwind CSS" src="https://img.shields.io/badge/Tailwind_CSS-3-06B6D4?style=flat-square&logo=tailwindcss&logoColor=white">
  <img alt="SQLite" src="https://img.shields.io/badge/SQLite-Embedded-003B57?style=flat-square&logo=sqlite&logoColor=white">
</p>

<p align="center">
  <img alt="Linux" src="https://img.shields.io/badge/Linux-amd64_%7C_386_%7C_arm64_%7C_aarch64_%7C_armv7-FCC624?style=flat-square&logo=linux&logoColor=111111">
  <img alt="Docker" src="https://img.shields.io/badge/Docker-Pinned_Build-2496ED?style=flat-square&logo=docker&logoColor=white">
  <img alt="WiFi Calling" src="https://img.shields.io/badge/WiFi_Calling-IMS_SMS-7B1FA2?style=flat-square">
  <img alt="eSIM" src="https://img.shields.io/badge/eSIM-LPA_%2F_eUICC-009688?style=flat-square">
  <img alt="Telegram" src="https://img.shields.io/badge/Telegram-Bot-26A5E4?style=flat-square&logo=telegram&logoColor=white">
  <img alt="Release" src="https://img.shields.io/badge/Release-Local_Artifact-2E7D32?style=flat-square">
</p>

**English** | [العربية](docs/README.ar.md) | [简体中文](docs/README.zh-CN.md) | [繁體中文](docs/README.zh-TW.md) | [Français](docs/README.fr.md) | [Русский](docs/README.ru.md) | [Español](docs/README.es.md) | [日本語](docs/README.ja.md)

> [!IMPORTANT]
> Hardened fork: remote installation, upstream container images, plugins, Export Proxy, and runtime self-update are disabled; release and Docker workflows are fail-closed. Build and deploy locally only from a reviewed, committed SHA in pinned containers using [the security workflow](docs/security-hardening.md); production uses [the dedicated VM procedure](docs/vm-deployment.md).

Vocat is an open-source web control panel and engineering toolkit for Quectel EC20/EC25-class cellular modems. It combines modem discovery, live radio status, AT and USSD terminals, SMS, WiFi Calling, eSIM management, network selection, proxy routing, notifications, audit logs, and controlled build and deployment workflows in one self-contained service.

The backend is written in Go, the interface is built with React and TypeScript, and the production frontend is embedded into the Go binary. A single executable contains the web application and uses SQLite for persistent state.

<p align="center">
  <img src="img\image.png">
  <img src="img\image-1.png">
</p>

## Features

| Area | What Vocat provides |
| --- | --- |
| Device management | Automatic serial/USB discovery, multiple modem support, friendly device names, live overview updates, module restart, flight mode, and USB networking mode controls. |
| Radio and network | Registration status, operator, signal metrics, RSRP/RSRQ/SINR, network mode, band, channel, operator scanning, and automatic or manual network selection. |
| AT and USSD | Interactive AT terminal, command history, raw modem responses, USSD start/continue/cancel flows, and clear modem error reporting. |
| SMS | Direct cellular and IMS SMS transmission, inbound synchronization, multipart handling, delivery reports, conversation history, unread state, timestamps, and per-message delivery status. |
| WiFi Calling | IKEv2/ePDG tunnel setup, EAP-AKA authentication, IMS registration, IMS SMS, reconnect controls, status diagnostics, and per-device routing. |
| eSIM and eUICC | eUICC discovery, EID and production information, certificate metadata, multi-eUICC inventory, installed profile listing, enable/disable/switch operations, download, rename, and delete operations when supported by the card. |
| Card policy | ICCID-based WiFi Calling and flight-mode behavior with immediate policy application. |
| Proxy routing | Upstream SOCKS routing, device bindings, country rules, TCP reachability checks, and UDP Associate checks for WiFi Calling data paths. |
| Notifications | New inbound SMS forwarding through Telegram, Bark, email, Pushplus, and signed webhooks. Each SMS is delivered as an individual notification. |
| Telegram bot | Device status, installed-profile listing and switching, WiFi Calling controls, and SMS sending. Sensitive actions require administrator confirmation. |
| Operations | Authentication, CSRF protection, access policies, audit events, live logs, log retention, health checks, responsive layout, dark mode, and English/Chinese application UI. |
| Distribution | Local static Linux artifacts built from a reviewed, committed SHA in pinned temporary containers, with manifest and SHA-256 verification plus database-aware rollback. Compose is development-only; automated binary, GHCR, and `latest` publishing are disabled. |

## Supported hardware

Vocat targets Qualcomm-based Quectel modules that expose compatible AT, QMI, serial, and USB networking interfaces, including:

- Quectel EC20
- Quectel EC25
- Quectel EG25 family
- Compatible EG600 and related modules

Available features depend on the module firmware, USB composition, SIM/eSIM capabilities, host drivers, radio network, and carrier configuration.

## Hardened installation

Remote installation is disabled. Build the exact reviewed, committed SHA in pinned temporary containers, then deploy the complete artifact directory. `scripts/install.sh` accepts only a verified local artifact directory; it does not download a version, URL, GitHub release, or container image:

```bash
scripts/build-hardened.sh amd64
RELEASE_COMMIT="$(git rev-parse HEAD)"
ARTIFACT_INDEX_SHA256='<trusted-64-hex-SHA256SUMS-hash>'
sudo scripts/install.sh --check-env
sudo scripts/install.sh --artifact "dist/hardened/$RELEASE_COMMIT" \
  --expected-commit "$RELEASE_COMMIT" \
  --expected-index-sha256 "$ARTIFACT_INDEX_SHA256"
```

Record the builder's `artifact index sha256` value through a trusted out-of-band channel that is independent of the artifact transfer, then use it as `ARTIFACT_INDEX_SHA256`. Never derive the expected value from `SHA256SUMS` inside the copied artifact directory. The deployment script verifies the expected commit, artifact index, manifest, SHA-256, and Go 1.26.7 build metadata, preflights SQLite migration on a copy, and restores both binary and database if readiness fails. Production uses the dedicated KVM guest and `deploy/vocat.service`.

The checked-in Compose profile is for local development only. It builds `vocat-hardened:local`, never pulls an upstream image, drops all capabilities except `NET_ADMIN`/`NET_RAW`, and does not mount the host's complete `/dev` tree:

```bash
docker compose build --pull=false
read -rsp "Admin password: " VOCAT_BOOTSTRAP_PASSWORD; echo
printf "%s\n" "$VOCAT_BOOTSTRAP_PASSWORD" | docker compose run --rm -T \
  --entrypoint /opt/vocat/bin/vocat vocat bootstrap-admin
unset VOCAT_BOOTSTRAP_PASSWORD
docker compose up -d
```

See [security hardening](docs/security-hardening.md) and [VM deployment](docs/vm-deployment.md).

### USB SIM readers

USB SIM readers use the Linux PC/SC service. The local artifact installer never
installs operating-system packages. On Debian/Ubuntu, install the optional reader
support with `apt install pcscd libccid` before using this feature. If USB sees a
CCID reader but PC/SC is unavailable, VoCat keeps the reader visible in the
add-device dialog and reports the missing service or driver instead of silently
hiding it.

### QMI command-line utilities

VoCat uses `qmicli` to verify that a QMI control channel is ready and
`qmi-proxy` to multiplex access to it. Packet-data sessions are managed by the
built-in QMI WDS client instead of `qmi-network` CID/PDH state files. The
The guest preparation script installs and verifies these utilities; the
artifact installer only checks that the prepared environment is ready. For manual
deployment, Debian/Ubuntu uses `apt install libqmi-utils`; Arch Linux uses
`pacman -S libqmi`, Alpine uses `apk add qmi-utils`, and OpenWrt uses
`opkg install qmi-utils`.

`vocat doctor --repair-dji-qmi` checks for `qmicli` before changing any USB
driver binding or asserting DTR. If the utility is unavailable, the command
stops with an installation hint and leaves the current device state untouched.

## Configuration

Vocat reads an optional JSON configuration file from `VOCAT_CONFIG`, then applies `VOCAT_*` environment variables. Environment variables take precedence.

| Environment variable | Default | Description |
| --- | --- | --- |
| `VOCAT_ADDR` | `0.0.0.0:7575` | HTTP listen address. |
| `VOCAT_DATABASE_PATH` | `./data/vocat.db` | SQLite database path. |
| `VOCAT_SESSION_TTL` | `24h` | Authentication session lifetime. |
| `VOCAT_SECURE_COOKIES` | `false` | Marks session cookies as secure when HTTPS is used. |
| `VOCAT_SHUTDOWN_TIMEOUT` | `10s` | Graceful shutdown timeout. |
| `VOCAT_MAX_REQUEST_BODY_BYTES` | `1048576` | Maximum API request body size. |

User-supplied Apple carrier bundles can be converted into reviewable,
allow-listed carrier profiles with `vocat carrier import-ipcc`; see
[docs/CARRIER_IPCC_IMPORT.md](docs/CARRIER_IPCC_IMPORT.md).

Administrator credentials are stored only in SQLite. Initialize an empty
database once with `vocat bootstrap-admin`; environment variables and JSON
configuration cannot set or overwrite the administrator username or password.

Do not store Telegram tokens, SMTP passwords, webhook secrets, SIM credentials, or other private data in the repository. Configure them through the application settings or protected environment files.

## Telegram bot

When Telegram notifications are enabled and both Chat ID and Admin ID are configured, the bot supports:

```text
/status [device]
/esim <device>
/switch <device> <iccid>
/wfc <device> <status|on|off|reconnect>
/sms <device> <number> <message>
```

Profile switching and SMS submission use one-time confirmation buttons. The bot does not expose eSIM download, delete, or rename commands.

## Updating

Runtime Web, CLI, installer, and container self-update are disabled. Merge upstream into a temporary review branch, pass the full security gate, build the committed SHA, and deploy its verified artifact with database-aware rollback. Never deploy an upstream build directly.

## Development

Requirements:

- Go 1.25 or newer
- Node.js 20 or newer
- npm

Run the frontend development server:

```bash
cd web
npm install
npm run dev
```

Build the embedded frontend and start the backend:

```bash
cd web
npm run build
cd ..
go run ./cmd/vocat
```

Run all tests:

```bash
go test ./...
```

Build a development-only binary (not a release artifact):

```bash
go build -trimpath -ldflags "-s -w" -o vocat ./cmd/vocat
```

## Release controls

The `release` and `docker` GitHub Actions workflows are intentionally fail-closed: they do not publish binaries, GHCR images, or `latest` tags. A Git tag is source metadata only and is not a deployment trigger. Build release artifacts locally from a reviewed, committed SHA with `scripts/build-hardened.sh`, then install only the verified local artifact directory.

## Project layout

```text
cmd/vocat/                  Application entry point and CLI
internal/device/            Modem discovery and device control
internal/modem/             AT session and response handling
internal/server/            HTTP API, notifications, and embedded web server
internal/store/             SQLite persistence
internal/update/            disabled compatibility stubs; no runtime updater
internal/vowifi/            IKE, EAP-AKA, IMS, and WiFi Calling runtime
scripts/build-hardened.sh   committed-SHA builder using pinned containers
scripts/install.sh          verified local-artifact installer; no downloads
web/src/                    React and TypeScript frontend
.github/workflows/          fail-closed release and Docker guardrails
```

## Responsible use

Cellular modem and eSIM operations can affect subscriber service, stored profiles, network registration, and hardware state. Keep backups, review destructive actions carefully, and use the software only in lawful environments where you are permitted to operate the connected hardware and network resources.

Vocat does not bypass carrier authentication, network policy, hardware security, or eSIM trust requirements. Support for an operation means that Vocat can request it from the modem or eUICC; the device, profile, network, or carrier may still reject it.

## Contributing

Issues and pull requests are welcome. Keep changes focused, include tests where practical, avoid committing credentials or subscriber data, and document hardware-specific behavior clearly.

Before submitting a change:

```bash
go test ./...
cd web && npm run build
```

## Thanks
- [Nodeseek.com](https://www.nodeseek.com) — A community dedicated to servers
- [Linux.do](https://linux.do) — An inspiring tech community
- [iniwex5](https://github.com/iniwex5) — Style and Functionality Guidelines

## Buy me a coffee

| Network | Address |
| ------- | ------- |
| USDT-TRON (TRC20) | `TQQAbboBoU8h5xX4YCA1rqWJU2WjK3seSg` |
| USDT-BSC (BEP20) | `0xdbfcd4a462550d6ff06d09cbd89026c6b145d9c4` |
| USDT-Polygon | `0xdbfcd4a462550d6ff06d09cbd89026c6b145d9c4` |

## License

See [LICENSE](LICENSE).

[![MengMengCode/VoCat Star History](https://mengmeng.meteor-history.com/api/embed/MengMengCode/VoCat.svg?sig=sdeXRVxAoY3yLWgXL7JViY2USYIN3t9neJ6ScPvgUAo&theme=light&style=xkcd&color=dd4528&background=ffffff&textColor=000000&width=900&height=600&lineWidth=3&showTitle=true&showLegend=true&showDots=false&v=0.0.14)](https://meteor-history.com)
