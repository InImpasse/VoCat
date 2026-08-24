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

[English](../README.md) | [العربية](README.ar.md) | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-TW.md) | [Français](README.fr.md) | [Русский](README.ru.md) | [Español](README.es.md) | **日本語**

> [!IMPORTANT]
> 強化ブランチでは、リモートインストール、upstream イメージ、プラグイン、Export Proxy、実行時の自己更新を無効化し、リリースおよび Docker ワークフローを fail-closed にしています。[security-hardening.md](security-hardening.md) に従い、レビュー済みかつコミット済みの SHA だけをローカルでビルド、デプロイしてください。本番環境には [vm-deployment.md](vm-deployment.md) を使用します。

Vocat は、Quectel EC20/EC25 クラスのセルラーモデム向けのオープンソース Web コントロールパネル兼エンジニアリングツールキットです。モデムの検出、ライブの無線ステータス、AT / USSD ターミナル、SMS、WiFi Calling、eSIM 管理、ネットワーク選択、プロキシルーティング、通知、監査ログ、管理されたビルドとデプロイの手順を、自己完結型の単一サービスに統合しています。

バックエンドは Go で書かれ、インターフェースは React と TypeScript で構築され、本番フロントエンドは Go バイナリに埋め込まれています。単一の実行ファイルに Web アプリケーション全体が含まれ、永続的な状態には SQLite を使用します。

<p align="center">
  <img src="../img/image.png">
  <img src="../img/image-1.png">
</p>

## 機能

| 領域 | Vocat が提供する機能 |
| --- | --- |
| デバイス管理 | シリアル/USB の自動検出、複数モデムのサポート、わかりやすいデバイス名、概要のライブ更新、モジュールの再起動、機内モード、USB ネットワークモードの制御。 |
| 無線とネットワーク | 登録ステータス、オペレーター、信号指標、RSRP/RSRQ/SINR、ネットワークモード、バンド、チャネル、オペレータースキャン、自動または手動のネットワーク選択。 |
| AT と USSD | 対話型 AT ターミナル、コマンド履歴、モデムの生出力、USSD の開始/継続/キャンセルフロー、明確なモデムエラーレポート。 |
| SMS | セルラーおよび IMS SMS の直接送信、受信同期、マルチパート処理、配信レポート、会話履歴、未読状態、タイムスタンプ、メッセージごとの配信ステータス。 |
| WiFi Calling | IKEv2/ePDG トンネルの確立、EAP-AKA 認証、IMS 登録、IMS SMS、再接続制御、ステータス診断、デバイスごとのルーティング。 |
| eSIM と eUICC | eUICC の検出、EID と製造情報、証明書メタデータ、複数 eUICC のインベントリ、インストール済みプロファイルの一覧、有効化/無効化/切り替え操作、およびカードが対応している場合のダウンロード、名前変更、削除操作。 |
| カードポリシー | ICCID ベースの WiFi Calling および機内モードの動作で、ポリシーが即時に適用されます。 |
| プロキシルーティング | アップストリーム SOCKS ルーティング、デバイスバインディング、国別ルール、TCP 到達性チェック、WiFi Calling データパス向けの UDP Associate チェック。 |
| 通知 | Telegram、Bark、メール、Pushplus、署名付き Webhook を介した新着 SMS の転送。各 SMS は個別の通知として配信されます。 |
| Telegram ボット | デバイスステータス、インストール済みプロファイルの一覧と切り替え、WiFi Calling 制御、SMS 送信。機密性の高い操作には管理者の確認が必要です。 |
| 運用 | 認証、CSRF 保護、アクセスポリシー、監査イベント、ライブログ、ログ保持、ヘルスチェック、レスポンシブレイアウト、ダークモード、英語/中国語のアプリケーション UI。 |
| 配布 | 固定された一時コンテナ内で、レビュー済みかつコミット済みの SHA からローカルビルドする静的 Linux アーティファクト。manifest と SHA-256 の検証およびデータベースを含むロールバックに対応します。Compose は開発専用で、バイナリ、GHCR、`latest` の自動公開は無効です。 |

## 対応ハードウェア

Vocat は、互換性のある AT、QMI、シリアル、USB ネットワークインターフェースを公開する Qualcomm ベースの Quectel モジュールを対象としています。対象には以下が含まれます:

- Quectel EC20
- Quectel EC25
- Quectel EG25 ファミリー
- 互換性のある EG600 および関連モジュール

利用可能な機能は、モジュールのファームウェア、USB 構成、SIM/eSIM の機能、ホストドライバー、無線ネットワーク、キャリア設定によって異なります。

## 強化されたインストール

リモートインストールは無効です。レビュー済みかつコミット済みの SHA から完全なアーティファクトディレクトリをビルドしてデプロイします。

```bash
scripts/build-hardened.sh amd64
RELEASE_COMMIT="$(git rev-parse HEAD)"
ARTIFACT_INDEX_SHA256='<trusted-64-hex-SHA256SUMS-hash>'
sudo scripts/install.sh --check-env
sudo scripts/install.sh --artifact "dist/hardened/$RELEASE_COMMIT" \
  --expected-commit "$RELEASE_COMMIT" \
  --expected-index-sha256 "$ARTIFACT_INDEX_SHA256"
```

ビルダーが出力する `artifact index sha256` は、アーティファクト転送とは独立した信頼できる帯域外チャネルで記録してください。コピー先のアーティファクトディレクトリにある `SHA256SUMS` から期待値を算出してはいけません。`scripts/install.sh` が受け付けるのは検証済みのローカルアーティファクトディレクトリだけで、バージョン、URL、GitHub Release、コンテナイメージはダウンロードしません。本番環境では専用 KVM ゲストを使用します。Compose はローカル開発専用で、upstream イメージの取得、privileged mode、ホストの `/dev` 全体のマウントを行いません。詳細は [security-hardening.md](security-hardening.md) を参照してください。

## 設定

Vocat は `VOCAT_CONFIG` からオプションの JSON 設定ファイルを読み込み、次に `VOCAT_*` 環境変数を適用します。環境変数が優先されます。

| 環境変数 | デフォルト | 説明 |
| --- | --- | --- |
| `VOCAT_ADDR` | `0.0.0.0:7575` | HTTP リッスンアドレス。 |
| `VOCAT_DATABASE_PATH` | `./data/vocat.db` | SQLite データベースパス。 |
| `VOCAT_SESSION_TTL` | `24h` | 認証セッションの有効期間。 |
| `VOCAT_SECURE_COOKIES` | `false` | HTTPS 使用時にセッション Cookie をセキュアとしてマークします。 |
| `VOCAT_SHUTDOWN_TIMEOUT` | `10s` | グレースフルシャットダウンのタイムアウト。 |
| `VOCAT_MAX_REQUEST_BODY_BYTES` | `1048576` | API リクエストボディの最大サイズ。 |

Telegram トークン、SMTP パスワード、Webhook シークレット、SIM 認証情報、その他のプライベートデータをリポジトリに保存しないでください。アプリケーション設定または保護された環境ファイルを通じて設定してください。

## Telegram ボット

Telegram 通知が有効で、Chat ID と Admin ID の両方が設定されている場合、ボットは以下をサポートします:

```text
/status [デバイス]
/esim <デバイス>
/switch <デバイス> <iccid>
/wfc <デバイス> <status|on|off|reconnect>
/sms <デバイス> <番号> <メッセージ>
```

プロファイルの切り替えと SMS の送信には、ワンタイム確認ボタンが使用されます。ボットは eSIM のダウンロード、削除、名前変更コマンドを公開しません。

## 更新

実行時の自己更新は無効です。一時的な統合ブランチで upstream の変更をレビューし、レビュー済みかつコミット済みの SHA を固定コンテナでビルドした、検証済みローカルアーティファクトだけをデプロイしてください。

## 開発

要件:

- Go 1.25 以降
- Node.js 20 以降
- npm

フロントエンド開発サーバーを実行する:

```bash
cd web
npm install
npm run dev
```

埋め込みフロントエンドをビルドしてバックエンドを起動する:

```bash
cd web
npm run build
cd ..
go run ./cmd/vocat
```

すべてのテストを実行する:

```bash
go test ./...
```

開発専用バイナリをビルドする（リリースアーティファクトには使用不可）:

```bash
go build -trimpath -ldflags "-s -w" -o vocat ./cmd/vocat
```

## リリース制御

GitHub Actions の `release` と `docker` ワークフローは意図的に無効化され、fail-closed です。バイナリ、GHCR イメージ、`latest` タグを公開しません。Git タグはソースのメタデータにすぎず、デプロイのトリガーにはなりません。リリースアーティファクトは `scripts/build-hardened.sh` を使ってレビュー済みかつコミット済みの SHA からローカルでビルドし、インストーラーには検証済みローカルアーティファクトディレクトリだけを渡してください。

## プロジェクト構成

```text
cmd/vocat/                  アプリケーションのエントリポイントと CLI
internal/device/            モデム検出とデバイス制御
internal/modem/             AT セッションと応答処理
internal/server/            HTTP API、通知、埋め込み Web サーバー
internal/store/             SQLite 永続化
internal/update/            無効化済み互換コード。実行時 updater なし
internal/vowifi/            IKE、EAP-AKA、IMS、WiFi Calling ランタイム
scripts/build-hardened.sh   固定コンテナでコミット済み SHA をビルド
scripts/install.sh          検証済みローカルアーティファクト専用。ダウンロードなし
web/src/                    React と TypeScript のフロントエンド
.github/workflows/          fail-closed のリリースおよび Docker ガード
```

## 責任ある使用

セルラーモデムおよび eSIM の操作は、加入者サービス、保存されたプロファイル、ネットワーク登録、ハードウェア状態に影響を与える可能性があります。バックアップを保持し、破壊的な操作を慎重に確認し、接続されたハードウェアとネットワークリソースを操作することが許可されている合法的な環境でのみソフトウェアを使用してください。

Vocat は、キャリア認証、ネットワークポリシー、ハードウェアセキュリティ、eSIM の信頼要件をバイパスしません。操作のサポートは、Vocat がモデムまたは eUICC にそれを要求できることを意味します。デバイス、プロファイル、ネットワーク、またはキャリアがそれを拒否する場合があります。

## コントリビューション

Issue や Pull Request を歓迎します。変更は焦点を絞り、可能な場合はテストを含め、認証情報や加入者データをコミットしないようにし、ハードウェア固有の動作を明確に文書化してください。

変更を提出する前に:

```bash
go test ./...
cd web && npm run build
```

## 謝辞
- [Nodeseek.com](https://www.nodeseek.com) — サーバーに特化したコミュニティ
- [Linux.do](https://linux.do) — 刺激的なテックコミュニティ
- [iniwex5](https://github.com/iniwex5) — スタイルと機能のガイドライン

## コーヒーをおごってください

| ネットワーク | アドレス |
| ------- | ------- |
| USDT-TRON (TRC20) | `TQQAbboBoU8h5xX4YCA1rqWJU2WjK3seSg` |
| USDT-BSC (BEP20) | `0xdbfcd4a462550d6ff06d09cbd89026c6b145d9c4` |
| USDT-Polygon | `0xdbfcd4a462550d6ff06d09cbd89026c6b145d9c4` |

## ライセンス

[LICENSE](../LICENSE) を参照してください。

[![MengMengCode/VoCat Star History](https://mengmeng.meteor-history.com/api/embed/MengMengCode/VoCat.svg?sig=sdeXRVxAoY3yLWgXL7JViY2USYIN3t9neJ6ScPvgUAo&theme=light&style=xkcd&color=dd4528&background=ffffff&textColor=000000&width=900&height=600&lineWidth=3&showTitle=true&showLegend=true&showDots=false&v=0.0.14)](https://meteor-history.com)
