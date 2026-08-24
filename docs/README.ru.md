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

[English](../README.md) | [العربية](README.ar.md) | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-TW.md) | [Français](README.fr.md) | **Русский** | [Español](README.es.md) | [日本語](README.ja.md)

> [!IMPORTANT]
> Защищённая ветка: удалённая установка, образы upstream, плагины, Export Proxy и самообновление во время работы отключены; workflows релиза и Docker намеренно работают по принципу fail-closed. Локально собирайте и развёртывайте только проверенный и зафиксированный SHA по [security-hardening.md](security-hardening.md); для production используйте [vm-deployment.md](vm-deployment.md).

Vocat — это веб-панель управления с открытым исходным кодом и набор инженерных инструментов для сотовых модемов Quectel класса EC20/EC25. Она объединяет в одном автономном сервисе обнаружение модемов, состояние радиосвязи в реальном времени, терминалы AT и USSD, SMS, WiFi Calling, управление eSIM, выбор сети, маршрутизацию через прокси, уведомления, журналы аудита и контролируемые процессы сборки и развёртывания.

Бэкенд написан на Go, интерфейс построен на React и TypeScript, а производственный фронтенд встроен в бинарный файл Go. Один исполняемый файл содержит веб-приложение и использует SQLite для постоянного хранения состояния.

<p align="center">
  <img src="../img/image.png">
  <img src="../img/image-1.png">
</p>

## Возможности

| Область | Что предоставляет Vocat |
| --- | --- |
| Управление устройствами | Автоматическое обнаружение по последовательному порту/USB, поддержка нескольких модемов, понятные имена устройств, обновление обзора в реальном времени, перезапуск модуля, авиарежим и управление режимом USB-сети. |
| Радио и сеть | Статус регистрации, оператор, метрики сигнала, RSRP/RSRQ/SINR, режим сети, диапазон, канал, сканирование операторов и автоматический или ручной выбор сети. |
| AT и USSD | Интерактивный AT-терминал, история команд, необработанные ответы модема, потоки запуска/продолжения/отмены USSD и понятные сообщения об ошибках модема. |
| SMS | Прямая отправка сотовых и IMS SMS, входящая синхронизация, обработка составных сообщений, отчёты о доставке, история диалогов, статус непрочитанных, метки времени и статус доставки каждого сообщения. |
| WiFi Calling | Установка туннеля IKEv2/ePDG, аутентификация EAP-AKA, регистрация IMS, IMS SMS, управление переподключением, диагностика состояния и маршрутизация по устройствам. |
| eSIM и eUICC | Обнаружение eUICC, EID и производственная информация, метаданные сертификатов, инвентарь нескольких eUICC, список установленных профилей, операции включения/отключения/переключения, а также загрузка, переименование и удаление при поддержке картой. |
| Политика карты | Поведение WiFi Calling и авиарежима на основе ICCID с немедленным применением политики. |
| Маршрутизация через прокси | Восходящая маршрутизация SOCKS, привязки устройств, правила по странам, проверки доступности TCP и проверки UDP Associate для путей передачи данных WiFi Calling. |
| Уведомления | Пересылка новых входящих SMS через Telegram, Bark, электронную почту, Pushplus и подписанные вебхуки. Каждое SMS доставляется как отдельное уведомление. |
| Telegram-бот | Статус устройства, список и переключение установленных профилей, управление WiFi Calling и отправка SMS. Чувствительные действия требуют подтверждения администратора. |
| Эксплуатация | Аутентификация, защита CSRF, политики доступа, события аудита, журналы в реальном времени, хранение журналов, проверки работоспособности, адаптивная вёрстка, тёмный режим и интерфейс на английском/китайском. |
| Дистрибуция | Локальные статические артефакты Linux, собранные из проверенного и зафиксированного SHA во временных закреплённых контейнерах, с проверкой манифеста и SHA-256 и откатом базы данных. Compose предназначен только для разработки; автоматическая публикация бинарных файлов, GHCR и `latest` отключена. |

## Поддерживаемое оборудование

Vocat ориентирован на модули Quectel на базе Qualcomm, которые предоставляют совместимые интерфейсы AT, QMI, последовательный порт и USB-сеть, включая:

- Quectel EC20
- Quectel EC25
- Семейство Quectel EG25
- Совместимые модули EG600 и родственные

Доступные функции зависят от прошивки модуля, конфигурации USB, возможностей SIM/eSIM, драйверов хоста, радиосети и настроек оператора.

## Установка

Удалённая установка отключена. Соберите точный проверенный и зафиксированный SHA
в закреплённых контейнерных образах и разверните полный каталог артефакта.
`scripts/install.sh` принимает только проверенный локальный каталог артефакта;
он не загружает версии, URL, GitHub Releases или образы контейнеров:

```bash
scripts/build-hardened.sh amd64
RELEASE_COMMIT="$(git rev-parse HEAD)"
ARTIFACT_INDEX_SHA256='<doverennyy-64-hex-hesh-SHA256SUMS>'
sudo scripts/install.sh --check-env
sudo scripts/install.sh --artifact "dist/hardened/$RELEASE_COMMIT" \
  --expected-commit "$RELEASE_COMMIT" \
  --expected-index-sha256 "$ARTIFACT_INDEX_SHA256"
```

Значение `artifact index sha256`, выведенное сборщиком, необходимо получить по
доверенному внешнему каналу, независимому от передачи артефакта. Нельзя вычислять
ожидаемое значение из `SHA256SUMS` внутри скопированного каталога. Процесс проверяет
ожидаемые commit и индекс, manifest, SHA-256 и версию Go, тестирует миграцию SQLite
на копии и вместе восстанавливает бинарный файл и базу данных при ошибке
readiness. В production используется отдельная KVM VM и systemd-служба из
[vm-deployment.md](vm-deployment.md).

### Docker

Профиль Compose в репозитории предназначен только для локальной разработки. Он
собирает `vocat-hardened:local`, не загружает upstream-образы, не использует
привилегированный режим и не монтирует всё дерево `/dev` хоста:

```bash
docker compose build --pull=false

read -rsp "Admin password: " VOCAT_BOOTSTRAP_PASSWORD; echo
printf '%s\n' "$VOCAT_BOOTSTRAP_PASSWORD" | docker compose run --rm -T \
  --entrypoint /opt/vocat/bin/vocat vocat bootstrap-admin
unset VOCAT_BOOTSTRAP_PASSWORD
docker compose up -d
```

Не используйте привилегированный контейнер и не монтируйте всё дерево `/dev`
в production.

## Конфигурация

Vocat читает необязательный JSON-файл конфигурации из `VOCAT_CONFIG`, затем применяет переменные окружения `VOCAT_*`. Переменные окружения имеют приоритет.

| Переменная окружения | По умолчанию | Описание |
| --- | --- | --- |
| `VOCAT_ADDR` | `0.0.0.0:7575` | Адрес прослушивания HTTP. |
| `VOCAT_DATABASE_PATH` | `./data/vocat.db` | Путь к базе данных SQLite. |
| `VOCAT_SESSION_TTL` | `24h` | Время жизни сессии аутентификации. |
| `VOCAT_SECURE_COOKIES` | `false` | Помечает cookie сессии как безопасные при использовании HTTPS. |
| `VOCAT_SHUTDOWN_TIMEOUT` | `10s` | Тайм-аут корректного завершения работы. |
| `VOCAT_MAX_REQUEST_BODY_BYTES` | `1048576` | Максимальный размер тела запроса API. |

Не храните токены Telegram, пароли SMTP, секреты вебхуков, учётные данные SIM или другие приватные данные в репозитории. Настраивайте их через параметры приложения или защищённые файлы окружения.

## Telegram-бот

Когда уведомления Telegram включены и настроены Chat ID и Admin ID, бот поддерживает:

```text
/status [устройство]
/esim <устройство>
/switch <устройство> <iccid>
/wfc <устройство> <status|on|off|reconnect>
/sms <устройство> <номер> <сообщение>
```

Переключение профилей и отправка SMS используют одноразовые кнопки подтверждения. Бот не предоставляет команды загрузки, удаления или переименования eSIM.

## Обновление

Самообновление отключено. Проверяйте изменения upstream во временной интеграционной ветке и развёртывайте только проверенный локальный артефакт, собранный из проверенного и зафиксированного SHA в закреплённых контейнерах.

## Разработка

Требования:

- Go 1.25 или новее
- Node.js 20 или новее
- npm

Запустить сервер разработки фронтенда:

```bash
cd web
npm install
npm run dev
```

Собрать встроенный фронтенд и запустить бэкенд:

```bash
cd web
npm run build
cd ..
go run ./cmd/vocat
```

Запустить все тесты:

```bash
go test ./...
```

Собрать бинарный файл только для разработки (не релизный артефакт):

```bash
go build -trimpath -ldflags "-s -w" -o vocat ./cmd/vocat
```

## Контроль релизов

Workflows `release` и `docker` в GitHub Actions намеренно отключены и работают по принципу fail-closed: они не публикуют бинарные файлы, образы GHCR или теги `latest`. Git-тег является только метаданными исходного кода и не запускает развёртывание. Собирайте релизные артефакты локально из проверенного и зафиксированного SHA с помощью `scripts/build-hardened.sh`; установщик принимает только проверенный локальный каталог артефакта.

## Структура проекта

```text
cmd/vocat/                  Точка входа приложения и CLI
internal/device/            Обнаружение модемов и управление устройствами
internal/modem/             Сессия AT и обработка ответов
internal/server/            HTTP API, уведомления и встроенный веб-сервер
internal/store/             Постоянное хранение SQLite
internal/update/            Отключённый код совместимости; нет runtime updater
internal/vowifi/            Среда выполнения IKE, EAP-AKA, IMS и WiFi Calling
scripts/build-hardened.sh   Сборка зафиксированного SHA в закреплённых контейнерах
scripts/install.sh          Только проверенный локальный артефакт; без загрузок
web/src/                    Фронтенд на React и TypeScript
.github/workflows/          Fail-closed ограничения релиза и Docker
```

## Ответственное использование

Операции с сотовыми модемами и eSIM могут влиять на обслуживание абонента, сохранённые профили, регистрацию в сети и состояние оборудования. Делайте резервные копии, внимательно проверяйте деструктивные действия и используйте программное обеспечение только в законных средах, где вам разрешено работать с подключённым оборудованием и сетевыми ресурсами.

Vocat не обходит аутентификацию оператора, сетевую политику, аппаратную безопасность или требования доверия eSIM. Поддержка операции означает, что Vocat может запросить её у модема или eUICC; устройство, профиль, сеть или оператор всё равно могут её отклонить.

## Участие в разработке

Мы приветствуем issues и pull request'ы. Делайте изменения сфокусированными, по возможности добавляйте тесты, избегайте коммита учётных данных или данных абонентов и чётко документируйте поведение, специфичное для оборудования.

Перед отправкой изменения:

```bash
go test ./...
cd web && npm run build
```

## Благодарности
- [Nodeseek.com](https://www.nodeseek.com) — Сообщество, посвящённое серверам
- [Linux.do](https://linux.do) — Вдохновляющее технологическое сообщество
- [iniwex5](https://github.com/iniwex5) — Руководства по стилю и функциональности

## Угостите меня кофе

| Сеть | Адрес |
| ------- | ------- |
| USDT-TRON (TRC20) | `TQQAbboBoU8h5xX4YCA1rqWJU2WjK3seSg` |
| USDT-BSC (BEP20) | `0xdbfcd4a462550d6ff06d09cbd89026c6b145d9c4` |
| USDT-Polygon | `0xdbfcd4a462550d6ff06d09cbd89026c6b145d9c4` |

## Лицензия

См. [LICENSE](../LICENSE).

[![MengMengCode/VoCat Star History](https://mengmeng.meteor-history.com/api/embed/MengMengCode/VoCat.svg?sig=sdeXRVxAoY3yLWgXL7JViY2USYIN3t9neJ6ScPvgUAo&theme=light&style=xkcd&color=dd4528&background=ffffff&textColor=000000&width=900&height=600&lineWidth=3&showTitle=true&showLegend=true&showDots=false&v=0.0.14)](https://meteor-history.com)
