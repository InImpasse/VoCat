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
  <img alt="Linux" src="https://img.shields.io/badge/Linux-amd64_%7C_386_%7C_arm64_%7C_aarch64_%7C_armv7-FCC624?style=flat-square&logo=linux&logoColor=111111">
  <img alt="Docker" src="https://img.shields.io/badge/Docker-Pinned_Build-2496ED?style=flat-square&logo=docker&logoColor=white">
  <img alt="WiFi Calling" src="https://img.shields.io/badge/WiFi_Calling-IMS_SMS-7B1FA2?style=flat-square">
  <img alt="eSIM" src="https://img.shields.io/badge/eSIM-LPA_%2F_eUICC-009688?style=flat-square">
  <img alt="Telegram" src="https://img.shields.io/badge/Telegram-Bot-26A5E4?style=flat-square&logo=telegram&logoColor=white">
  <img alt="Release" src="https://img.shields.io/badge/Release-Local_Artifact-2E7D32?style=flat-square">
</p>

[English](../README.md) | [العربية](README.ar.md) | [简体中文](README.zh-CN.md) | **繁體中文** | [Français](README.fr.md) | [Русский](README.ru.md) | [Español](README.es.md) | [日本語](README.ja.md)

> [!IMPORTANT]
> 加固分支已停用遠端安裝、上游映像、外掛、Export Proxy 與執行期自動更新；發佈與 Docker 工作流程採 fail-closed 策略。只可依照 [security-hardening.md](security-hardening.md) 從已審查且已提交的 SHA 在本機建置與部署；生產部署請遵循 [vm-deployment.md](vm-deployment.md)。

Vocat 是一款面向 Quectel EC20/EC25 系列行動通訊模組的開源 Web 控制面板與工程工具套件。它在單一自包含的服務中整合了模組探索、即時射頻狀態、AT 與 USSD 終端、簡訊、WiFi Calling(WiFi 通話)、eSIM 管理、網路選擇、代理路由、通知、稽核日誌以及受控建置與部署流程。

後端使用 Go 撰寫,介面採用 React 與 TypeScript 建構,生產環境前端被嵌入進 Go 二進位檔中。單一可執行檔即包含完整的 Web 應用,並使用 SQLite 進行持久化儲存。

<p align="center">
  <img src="../img/image.png">
  <img src="../img/image-1.png">
</p>

## 功能

| 領域 | Vocat 提供的能力 |
| --- | --- |
| 裝置管理 | 自動序列埠/USB 探索、多模組支援、裝置友善名稱、概覽即時更新、模組重新啟動、飛航模式以及 USB 網路卡模式控制。 |
| 射頻與網路 | 註冊狀態、電信業者、訊號指標、RSRP/RSRQ/SINR、網路模式、頻段、通道、電信業者掃描以及自動/手動選網。 |
| AT 與 USSD | 互動式 AT 終端、指令歷史、原始模組回應、USSD 發起/繼續/取消流程以及清晰的模組錯誤回報。 |
| 簡訊 | 行動通訊與 IMS 簡訊直接傳送、接收同步、長簡訊合併、送達報告、對話歷史、未讀狀態、時間戳以及逐則訊息的送達狀態。 |
| WiFi Calling | IKEv2/ePDG 隧道建立、EAP-AKA 驗證、IMS 註冊、IMS 簡訊、重新連線控制、狀態診斷以及依裝置路由。 |
| eSIM 與 eUICC | eUICC 探索、EID 與生產資訊、憑證中繼資料、多 eUICC 清單、已安裝設定檔列表、啟用/停用/切換操作,以及在卡片支援時進行下載、重新命名與刪除。 |
| 卡片策略 | 基於 ICCID 的 WiFi Calling 與飛航模式行為,策略即時套用。 |
| 代理路由 | 上游 SOCKS 路由、裝置綁定、國家規則、TCP 可達性檢查以及面向 WiFi Calling 資料路徑的 UDP Associate 檢查。 |
| 通知 | 透過 Telegram、Bark、電子郵件、Pushplus 以及簽章 Webhook 轉發新接收簡訊,每則簡訊個別推送。 |
| Telegram 機器人 | 裝置狀態、已安裝設定檔列表與切換、WiFi Calling 控制以及簡訊傳送。敏感操作需要管理員確認。 |
| 維運 | 驗證、CSRF 防護、存取策略、稽核事件、即時日誌、日誌保留、健康檢查、響應式版面、深色模式以及中英文應用介面。 |
| 發佈 | 使用固定版本的臨時容器，從已審查且已提交的 SHA 在本機建置靜態 Linux 產物，並驗證清單與 SHA-256，支援資料庫感知回復。Compose 僅供開發；二進位檔、GHCR 與 `latest` 自動發佈均已停用。 |

## 支援的硬體

Vocat 面向基於高通晶片、並暴露相容 AT、QMI、序列埠與 USB 網路介面的 Quectel 模組,包括:

- Quectel EC20
- Quectel EC25
- Quectel EG25 系列
- 相容的 EG600 及相關模組

可用功能取決於模組韌體、USB 複合裝置配置、SIM/eSIM 能力、主機驅動、無線網路以及電信業者配置。

## 加固安裝

遠端安裝已停用。請從已審查且已提交的 SHA 建置並部署完整產物目錄：

```bash
scripts/build-hardened.sh amd64
RELEASE_COMMIT="$(git rev-parse HEAD)"
ARTIFACT_INDEX_SHA256='<由可信帶外管道取得的64位SHA256SUMS雜湊>'
sudo scripts/install.sh --check-env
sudo scripts/install.sh --artifact "dist/hardened/$RELEASE_COMMIT" \
  --expected-commit "$RELEASE_COMMIT" \
  --expected-index-sha256 "$ARTIFACT_INDEX_SHA256"
```

建置器輸出的 `artifact index sha256` 必須透過獨立於產物傳輸的可信帶外管道記錄，不得從複製到目標機的產物目錄內重新計算預期值。`scripts/install.sh` 只接受本機已驗證的產物目錄，不會下載版本、URL、GitHub Release 或容器映像。生產環境使用獨立 KVM VM。Compose 僅供本機開發，不拉取上游映像、不使用 privileged mode，也不掛載完整 `/dev`。參見 [security-hardening.md](security-hardening.md)。

## 配置

Vocat 先從 `VOCAT_CONFIG` 讀取可選的 JSON 配置檔,再套用 `VOCAT_*` 環境變數。環境變數優先級更高。

| 環境變數 | 預設值 | 說明 |
| --- | --- | --- |
| `VOCAT_ADDR` | `0.0.0.0:7575` | HTTP 監聽位址。 |
| `VOCAT_DATABASE_PATH` | `./data/vocat.db` | SQLite 資料庫路徑。 |
| `VOCAT_SESSION_TTL` | `24h` | 驗證工作階段有效期。 |
| `VOCAT_SECURE_COOKIES` | `false` | 在使用 HTTPS 時將工作階段 Cookie 標記為安全。 |
| `VOCAT_SHUTDOWN_TIMEOUT` | `10s` | 優雅關閉逾時時間。 |
| `VOCAT_MAX_REQUEST_BODY_BYTES` | `1048576` | API 請求主體最大位元組數。 |

請勿將 Telegram token、SMTP 密碼、Webhook 金鑰、SIM 憑證或其他私密資料存放在倉庫中。請透過應用設定或受保護的環境檔來配置它們。

## Telegram 機器人

啟用 Telegram 通知並配置好 Chat ID 與 Admin ID 後,機器人支援:

```text
/status [裝置]
/esim <裝置>
/switch <裝置> <iccid>
/wfc <裝置> <status|on|off|reconnect>
/sms <裝置> <號碼> <內容>
```

設定檔切換與簡訊提交使用一次性確認按鈕。機器人不暴露 eSIM 下載、刪除或重新命名命令。

## 更新

執行期自動更新已停用。請在臨時整合分支審查 upstream 變更，只部署由已審查且已提交的 SHA 在固定容器中建置並驗證的本機產物。

## 開發

依賴要求:

- Go 1.25 或更新版本
- Node.js 20 或更新版本
- npm

執行前端開發伺服器:

```bash
cd web
npm install
npm run dev
```

建構嵌入的前端並啟動後端:

```bash
cd web
npm run build
cd ..
go run ./cmd/vocat
```

執行全部測試:

```bash
go test ./...
```

建構僅供開發使用的二進位檔（不得作為發佈產物）：

```bash
go build -trimpath -ldflags "-s -w" -o vocat ./cmd/vocat
```

## 發佈控制

GitHub Actions 中的 `release` 與 `docker` 工作流程刻意採 fail-closed 策略：不會發佈二進位檔、GHCR 映像或 `latest` 標籤。Git 標籤僅是原始碼中繼資料，不會觸發部署。發佈產物必須使用 `scripts/build-hardened.sh` 從已審查且已提交的 SHA 在本機建置，安裝器只接受本機已驗證的產物目錄。

## 專案結構

```text
cmd/vocat/                  應用入口與 CLI
internal/device/            模組探索與裝置控制
internal/modem/             AT 工作階段與回應處理
internal/server/            HTTP API、通知與內嵌 Web 伺服器
internal/store/             SQLite 持久化
internal/update/            已停用的相容占位程式碼；無執行期更新器
internal/vowifi/            IKE、EAP-AKA、IMS 與 WiFi Calling 執行時
scripts/build-hardened.sh   使用固定容器建置已提交 SHA
scripts/install.sh          僅安裝本機已驗證產物；不下載
web/src/                    React 與 TypeScript 前端
.github/workflows/          fail-closed 的發佈與 Docker 閘門
```

## 合規使用

行動通訊模組與 eSIM 操作可能影響用戶服務、已儲存的設定檔、網路註冊以及硬體狀態。請做好備份,謹慎審視破壞性操作,並僅在您被允許操作所連接的硬體與網路資源的合法環境中使用本軟體。

Vocat 不會繞過電信業者驗證、網路策略、硬體安全或 eSIM 信任要求。支援某項操作意味著 Vocat 能夠向模組或 eUICC 發起該請求;但裝置、設定檔、網路或電信業者仍可能拒絕。

## 貢獻

歡迎提交 Issue 與 Pull Request。請保持改動聚焦,在可行處附帶測試,避免提交憑證或用戶資料,並清晰地說明硬體相關行為。

提交改動前:

```bash
go test ./...
cd web && npm run build
```

## 致謝
- [Nodeseek.com](https://www.nodeseek.com) — 專注伺服器的社群
- [Linux.do](https://linux.do) — 富有啟發的技術社群
- [iniwex5](https://github.com/iniwex5) — 風格與功能指南

## 請我喝杯咖啡

| 網路 | 位址 |
| ------- | ------- |
| USDT-TRON (TRC20) | `TQQAbboBoU8h5xX4YCA1rqWJU2WjK3seSg` |
| USDT-BSC (BEP20) | `0xdbfcd4a462550d6ff06d09cbd89026c6b145d9c4` |
| USDT-Polygon | `0xdbfcd4a462550d6ff06d09cbd89026c6b145d9c4` |

## 授權條款

參見 [LICENSE](../LICENSE)。

[![MengMengCode/VoCat Star History](https://mengmeng.meteor-history.com/api/embed/MengMengCode/VoCat.svg?sig=sdeXRVxAoY3yLWgXL7JViY2USYIN3t9neJ6ScPvgUAo&theme=light&style=xkcd&color=dd4528&background=ffffff&textColor=000000&width=900&height=600&lineWidth=3&showTitle=true&showLegend=true&showDots=false&v=0.0.14)](https://meteor-history.com)
