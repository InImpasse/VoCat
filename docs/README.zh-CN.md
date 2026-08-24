<p align="center">
  <img src="../web/public/favicon.svg" width="96" alt="Vocat">
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
  <img alt="Linux" src="https://img.shields.io/badge/Linux-amd64_%7C_386_%7C_arm64_%7C_armv7-FCC624?style=flat-square&logo=linux&logoColor=111111">
  <img alt="Docker" src="https://img.shields.io/badge/Docker-Pinned_Build-2496ED?style=flat-square&logo=docker&logoColor=white">
  <img alt="WiFi Calling" src="https://img.shields.io/badge/WiFi_Calling-IMS_SMS-7B1FA2?style=flat-square">
  <img alt="eSIM" src="https://img.shields.io/badge/eSIM-LPA_%2F_eUICC-009688?style=flat-square">
  <img alt="Telegram" src="https://img.shields.io/badge/Telegram-Bot-26A5E4?style=flat-square&logo=telegram&logoColor=white">
  <img alt="Release" src="https://img.shields.io/badge/Release-Local_Artifact-2E7D32?style=flat-square">
</p>

[English](../README.md) | [العربية](README.ar.md) | **简体中文** | [繁體中文](README.zh-TW.md) | [Français](README.fr.md) | [Русский](README.ru.md) | [Español](README.es.md) | [日本語](README.ja.md)

> [!IMPORTANT]
> 加固分支已禁用远程安装、上游容器镜像、插件、Export Proxy 和运行时自更新；发布与 Docker 工作流采用 fail-closed 策略。仅按照[安全流程](security-hardening.md)从经过审查且已提交的 SHA 本地构建、部署；生产部署使用[独立 VM 流程](vm-deployment.md)。

Vocat 是一款面向 Quectel EC20/EC25 系列蜂窝模组的开源 Web 控制面板与工程工具套件。它在一个自包含的服务中整合了模组发现、实时射频状态、AT 与 USSD 终端、短信、WiFi Calling(WiFi 通话)、eSIM 管理、网络选择、代理路由、通知、审计日志以及受控构建与部署流程。

后端使用 Go 编写,界面采用 React 与 TypeScript 构建,生产环境前端被嵌入进 Go 二进制中。单个可执行文件即包含完整的 Web 应用,并使用 SQLite 进行持久化存储。

<p align="center">
  <img src="../img/image.png">
  <img src="../img/image-1.png">
</p>

## 功能

| 领域 | Vocat 提供的能力 |
| --- | --- |
| 设备管理 | 自动串口/USB 发现、多模组支持、设备友好名称、概览实时刷新、模组重启、飞行模式以及 USB 网卡模式控制。 |
| 射频与网络 | 注册状态、运营商、信号指标、RSRP/RSRQ/SINR、网络模式、频段、信道、运营商扫描以及自动/手动选网。 |
| AT 与 USSD | 交互式 AT 终端、命令历史、原始模组响应、USSD 发起/继续/取消流程以及清晰的模组错误上报。 |
| 短信 | 蜂窝与 IMS 短信直接发送、入站同步、长短信合并、送达报告、会话历史、未读状态、时间戳以及逐条消息的送达状态。 |
| WiFi Calling | IKEv2/ePDG 隧道建立、EAP-AKA 鉴权、IMS 注册、IMS 短信、重连控制、状态诊断以及按设备路由。 |
| eSIM 与 eUICC | eUICC 发现、EID 与生产信息、证书元数据、多 eUICC 清单、已安装配置文件列表、启用/禁用/切换操作,以及在卡片支持时进行下载、重命名和删除。 |
| 卡策略 | 基于 ICCID 的 WiFi Calling 与飞行模式行为,策略即时应用。 |
| 代理路由 | 上游 SOCKS 路由、设备绑定、国家规则、TCP 可达性检查以及面向 WiFi Calling 数据路径的 UDP Associate 检查。 |
| 通知 | 通过 Telegram、Bark、邮件、Pushplus 以及签名 Webhook 转发新入站短信,每条短信单独推送。 |
| Telegram 机器人 | 设备状态、已安装配置文件列表与切换、WiFi Calling 控制以及短信发送。敏感操作需要管理员确认。 |
| 运维 | 鉴权、CSRF 防护、访问策略、审计事件、实时日志、日志留存、健康检查、响应式布局、深色模式以及中英文应用界面。 |
| 分发 | 使用固定版本临时容器，从经过审查且已提交的 SHA 本地构建静态 Linux 产物，并验证清单与 SHA-256，支持数据库感知回滚。Compose 仅供开发；二进制、GHCR 和 `latest` 自动发布均已禁用。 |

## 支持的硬件

Vocat 面向基于高通芯片、并暴露兼容 AT、QMI、串口与 USB 网络接口的 Quectel 模组,包括:

- Quectel EC20
- Quectel EC25
- Quectel EG25 系列
- 兼容的 EG600 及相关模组

可用功能取决于模组固件、USB 复合设备配置、SIM/eSIM 能力、主机驱动、无线网络以及运营商配置。

## 加固安装

远程安装已禁用。使用固定版本的临时容器构建经过审查且已提交的确切 SHA，再部署完整产物目录。`scripts/install.sh` 只接受本地已验证的产物目录，不会下载版本、URL、GitHub Release 或容器镜像：

```bash
scripts/build-hardened.sh amd64
RELEASE_COMMIT="$(git rev-parse HEAD)"
ARTIFACT_INDEX_SHA256='<通过可信带外渠道取得的64位SHA256SUMS散列>'
sudo scripts/install.sh --check-env
sudo scripts/install.sh --artifact "dist/hardened/$RELEASE_COMMIT" \
  --expected-commit "$RELEASE_COMMIT" \
  --expected-index-sha256 "$ARTIFACT_INDEX_SHA256"
```

构建器输出的 `artifact index sha256` 必须通过独立于产物传输的可信带外渠道记录，再作为 `ARTIFACT_INDEX_SHA256` 使用；不得从复制到目标机的产物目录内重新计算预期值。部署脚本会验证预期 commit、产物索引、清单、SHA-256 与 Go 1.26.7 构建信息，在副本上预检 SQLite 迁移，并在 readiness 失败时同时恢复二进制和数据库。生产环境使用独立 KVM 来宾与 `deploy/vocat.service`。

仓库中的 Compose 仅供本地开发：构建 `vocat-hardened:local`，不拉取上游镜像；除 `NET_ADMIN`/`NET_RAW` 外删除所有 capability，也不挂载宿主机完整的 `/dev`：

```bash
docker compose build --pull=false
read -rsp "管理员密码: " VOCAT_BOOTSTRAP_PASSWORD; echo
printf "%s\n" "$VOCAT_BOOTSTRAP_PASSWORD" | docker compose run --rm -T \
  --entrypoint /opt/vocat/bin/vocat vocat bootstrap-admin
unset VOCAT_BOOTSTRAP_PASSWORD
docker compose up -d
```

参见[安全加固流程](security-hardening.md)与[VM 部署](vm-deployment.md)。

### USB SIM 读卡器

USB SIM 读卡器通过 Linux PC/SC 服务访问。本地产物安装器不会安装操作系统软件包。
需要该功能时，先在 Debian/Ubuntu 上执行 `apt install pcscd libccid` 安装可选的
读卡器支持。如果 USB 已识别 CCID 读卡器但 PC/SC 尚未就绪，VoCat 会继续在添加
设备窗口显示该硬件，并明确提示缺少服务或驱动，不再静默隐藏。

### QMI 命令行工具

VoCat 使用 `qmicli` 验证 QMI 控制通道是否就绪，并通过 `qmi-proxy` 复用控制
通道；分组数据会话由内置的 QMI WDS 客户端管理，不再依赖 `qmi-network` 的
临时 CID/PDH 状态文件。来宾准备脚本负责安装并验证对应工具，产物
安装器只检查准备好的环境。手动部署时，Debian/Ubuntu 使用
`apt install libqmi-utils`；Arch Linux 使用
`pacman -S libqmi`，Alpine 使用 `apk add qmi-utils`，OpenWrt 使用 `opkg install qmi-utils`。

`vocat doctor --repair-dji-qmi` 会在修改 USB 驱动绑定或触发 DTR 之前检查
`qmicli`。如果工具不可用，命令会给出安装提示并停止，保持设备当前状态不变。

## 配置

Vocat 先从 `VOCAT_CONFIG` 读取可选的 JSON 配置文件,再应用 `VOCAT_*` 环境变量。环境变量优先级更高。

| 环境变量 | 默认值 | 说明 |
| --- | --- | --- |
| `VOCAT_ADDR` | `0.0.0.0:7575` | HTTP 监听地址。 |
| `VOCAT_DATABASE_PATH` | `./data/vocat.db` | SQLite 数据库路径。 |
| `VOCAT_SESSION_TTL` | `24h` | 鉴权会话有效期。 |
| `VOCAT_SECURE_COOKIES` | `false` | 在使用 HTTPS 时将会话 Cookie 标记为安全。 |
| `VOCAT_SHUTDOWN_TIMEOUT` | `10s` | 优雅关闭超时时间。 |
| `VOCAT_MAX_REQUEST_BODY_BYTES` | `1048576` | API 请求体最大字节数。 |

管理员账号和密码只保存在 SQLite 数据库中。空数据库需要执行一次
`vocat bootstrap-admin` 完成初始化；环境变量和 JSON 配置都不能设置或覆盖管理员凭据。

请勿将 Telegram token、SMTP 密码、Webhook 密钥、SIM 凭据或其他私密数据存放在仓库中。请通过应用设置或受保护的环境文件来配置它们。

## Apple IPCC 运营商规则导入

VoCat 可以离线解析用户提供的 `.ipcc`，将 Apple 的 XML/二进制 plist
转换为可审查的运营商 Profile。默认只预览，不会修改配置：

```bash
vocat carrier import-ipcc Carrier_iPhone.ipcc
```

确认警告和匹配范围后，使用 `--install` 安装；重启 VoCat 后生效：

```bash
vocat carrier import-ipcc --install Carrier_iPhone.ipcc
```

导入器不会复制关闭证书验证、绕过运营商授权、APN 凭据、紧急呼叫或
设备型号专属媒体参数。完整字段和冲突处理说明见
[CARRIER_IPCC_IMPORT.md](CARRIER_IPCC_IMPORT.md)。

## Telegram 机器人

启用 Telegram 通知并配置好 Chat ID 与 Admin ID 后,机器人支持:

```text
/status [设备]
/esim <设备>
/switch <设备> <iccid>
/wfc <设备> <status|on|off|reconnect>
/sms <设备> <号码> <内容>
```

配置文件切换与短信提交使用一次性确认按钮。机器人不暴露 eSIM 下载、删除或重命名命令。

## 更新

Web、CLI、安装器和容器运行时自更新均已禁用。上游更新必须先合并到临时审查分支，通过完整安全门禁后，从已提交 SHA 构建并使用支持数据库回滚的流程部署。禁止直接部署上游构建。

## 开发

依赖要求:

- Go 1.25 或更新版本
- Node.js 20 或更新版本
- npm

运行前端开发服务器:

```bash
cd web
npm install
npm run dev
```

构建嵌入的前端并启动后端:

```bash
cd web
npm run build
cd ..
go run ./cmd/vocat
```

运行全部测试:

```bash
go test ./...
```

构建仅供开发使用的二进制（不得作为发布产物）：

```bash
go build -trimpath -ldflags "-s -w" -o vocat ./cmd/vocat
```

## 发布控制

GitHub Actions 中的 `release` 与 `docker` 工作流刻意采用 fail-closed 策略：不会发布二进制、GHCR 镜像或 `latest` 标签。Git 标签只作为源码元数据，不会触发部署。发布产物必须使用 `scripts/build-hardened.sh` 从经过审查且已提交的 SHA 在本地构建，安装器只接受本地已验证的产物目录。

## 项目结构

```text
cmd/vocat/                  应用入口与 CLI
internal/device/            模组发现与设备控制
internal/modem/             AT 会话与响应处理
internal/server/            HTTP API、通知与内嵌 Web 服务器
internal/store/             SQLite 持久化
internal/update/            已禁用的兼容占位代码；无运行时更新器
internal/vowifi/            IKE、EAP-AKA、IMS 与 WiFi Calling 运行时
scripts/build-hardened.sh   使用固定容器构建已提交 SHA
scripts/install.sh          仅安装本地已验证产物；不下载
web/src/                    React 与 TypeScript 前端
.github/workflows/          fail-closed 的发布与 Docker 门禁
```

## 合规使用

蜂窝模组与 eSIM 操作可能影响用户服务、已存储的配置文件、网络注册以及硬件状态。请做好备份,谨慎审视破坏性操作,并仅在您被允许操作所连接的硬件与网络资源的合法环境中使用本软件。

Vocat 不会绕过运营商鉴权、网络策略、硬件安全或 eSIM 信任要求。支持某项操作意味着 Vocat 能够向模组或 eUICC 发起该请求;但设备、配置文件、网络或运营商仍可能拒绝。

## 贡献

欢迎提交 Issue 与 Pull Request。请保持改动聚焦,在可行处附带测试,避免提交凭据或用户数据,并清晰地说明硬件相关行为。

提交改动前:

```bash
go test ./...
cd web && npm run build
```

## 致谢
- [Nodeseek.com](https://www.nodeseek.com) — 专注服务器的社群
- [Linux.do](https://linux.do) — 富有启发的技术社群
- [iniwex5](https://github.com/iniwex5) — 风格与功能指南

## 许可证

参见 [LICENSE](../LICENSE)。
