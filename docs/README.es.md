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

[English](../README.md) | [العربية](README.ar.md) | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-TW.md) | [Français](README.fr.md) | [Русский](README.ru.md) | **Español** | [日本語](README.ja.md)

> [!IMPORTANT]
> Rama reforzada: la instalación remota, las imágenes upstream, los plugins, Export Proxy y la autoactualización en tiempo de ejecución están deshabilitados; los flujos de publicación y Docker fallan de forma cerrada. Compile y despliegue localmente solo un SHA revisado y confirmado siguiendo [security-hardening.md](security-hardening.md); para producción siga [vm-deployment.md](vm-deployment.md).

Vocat es un panel de control web de código abierto y un conjunto de herramientas de ingeniería para módems celulares Quectel de clase EC20/EC25. Combina, en un único servicio autocontenido, el descubrimiento de módems, el estado de radio en vivo, terminales AT y USSD, SMS, WiFi Calling, gestión de eSIM, selección de red, enrutamiento por proxy, notificaciones, registros de auditoría y flujos controlados de compilación y despliegue.

El backend está escrito en Go, la interfaz está construida con React y TypeScript, y el frontend de producción está incrustado en el binario de Go. Un único ejecutable contiene la aplicación web y utiliza SQLite para el estado persistente.

<p align="center">
  <img src="../img/image.png">
  <img src="../img/image-1.png">
</p>

## Funcionalidades

| Área | Lo que proporciona Vocat |
| --- | --- |
| Gestión de dispositivos | Descubrimiento serie/USB automático, soporte para múltiples módems, nombres de dispositivo amigables, actualizaciones en vivo de la vista general, reinicio del módulo, modo avión y controles del modo de red USB. |
| Radio y red | Estado de registro, operador, métricas de señal, RSRP/RSRQ/SINR, modo de red, banda, canal, búsqueda de operadores y selección de red automática o manual. |
| AT y USSD | Terminal AT interactivo, historial de comandos, respuestas sin procesar del módem, flujos de inicio/continuación/cancelación de USSD y reporte claro de errores del módem. |
| SMS | Envío directo de SMS celulares e IMS, sincronización entrante, manejo multiparte, informes de entrega, historial de conversaciones, estado de no leído, marcas de tiempo y estado de entrega por mensaje. |
| WiFi Calling | Establecimiento de túnel IKEv2/ePDG, autenticación EAP-AKA, registro IMS, SMS IMS, controles de reconexión, diagnósticos de estado y enrutamiento por dispositivo. |
| eSIM y eUICC | Descubrimiento de eUICC, EID e información de producción, metadatos de certificados, inventario multi-eUICC, lista de perfiles instalados, operaciones de habilitar/deshabilitar/cambiar, y operaciones de descarga, renombrado y eliminación cuando la tarjeta lo admite. |
| Política de tarjeta | Comportamiento de WiFi Calling y modo avión basado en ICCID con aplicación inmediata de la política. |
| Enrutamiento por proxy | Enrutamiento SOCKS ascendente, vinculaciones de dispositivos, reglas por país, comprobaciones de accesibilidad TCP y comprobaciones UDP Associate para las rutas de datos de WiFi Calling. |
| Notificaciones | Reenvío de nuevos SMS entrantes a través de Telegram, Bark, correo electrónico, Pushplus y webhooks firmados. Cada SMS se entrega como una notificación individual. |
| Bot de Telegram | Estado del dispositivo, lista y cambio de perfiles instalados, controles de WiFi Calling y envío de SMS. Las acciones sensibles requieren confirmación del administrador. |
| Operaciones | Autenticación, protección CSRF, políticas de acceso, eventos de auditoría, registros en vivo, retención de registros, comprobaciones de salud, diseño adaptable, modo oscuro e interfaz de usuario en inglés/chino. |
| Distribución | Artefactos estáticos de Linux compilados localmente desde un SHA revisado y confirmado en contenedores temporales fijados, con verificación del manifest y SHA-256 y rollback de base de datos. Compose es solo para desarrollo; la publicación automática de binarios, GHCR y `latest` está deshabilitada. |

## Hardware compatible

Vocat está dirigido a módulos Quectel basados en Qualcomm que exponen interfaces AT, QMI, serie y de red USB compatibles, incluyendo:

- Quectel EC20
- Quectel EC25
- Familia Quectel EG25
- Módulos EG600 compatibles y relacionados

Las funciones disponibles dependen del firmware del módulo, la composición USB, las capacidades SIM/eSIM, los controladores del host, la red de radio y la configuración del operador.

## Instalación

La instalación remota está deshabilitada. Compile el SHA exacto revisado y
confirmado con imágenes de contenedor fijadas y despliegue el directorio de
artefacto completo. `scripts/install.sh` solo acepta un directorio de artefacto
local verificado; no descarga versiones, URL, GitHub Releases ni imágenes:

```bash
scripts/build-hardened.sh amd64
RELEASE_COMMIT="$(git rev-parse HEAD)"
ARTIFACT_INDEX_SHA256='<hash-SHA256SUMS-de-64-hex-confiable>'
sudo scripts/install.sh --check-env
sudo scripts/install.sh --artifact "dist/hardened/$RELEASE_COMMIT" \
  --expected-commit "$RELEASE_COMMIT" \
  --expected-index-sha256 "$ARTIFACT_INDEX_SHA256"
```

Registre el valor `artifact index sha256` del compilador por un canal confiable
fuera de banda e independiente de la transferencia del artefacto. No calcule el
valor esperado desde `SHA256SUMS` dentro del directorio copiado. El flujo verifica
el commit y el índice esperados, el manifest, SHA-256 y la versión de Go, prueba la migración
SQLite sobre una copia y restaura el binario y la base de datos juntos si falla
readiness. Producción usa la VM KVM dedicada y el servicio systemd descritos en
[vm-deployment.md](vm-deployment.md).

### Docker

El perfil Compose incluido es solo para desarrollo local. Construye
`vocat-hardened:local`, no descarga imágenes de upstream, no usa modo privilegiado
y no monta el árbol `/dev` completo del host:

```bash
docker compose build --pull=false

read -rsp "Admin password: " VOCAT_BOOTSTRAP_PASSWORD; echo
printf '%s\n' "$VOCAT_BOOTSTRAP_PASSWORD" | docker compose run --rm -T \
  --entrypoint /opt/vocat/bin/vocat vocat bootstrap-admin
unset VOCAT_BOOTSTRAP_PASSWORD
docker compose up -d
```

No use un contenedor privilegiado ni monte `/dev` completo en producción.

## Configuración

Vocat lee un archivo de configuración JSON opcional desde `VOCAT_CONFIG` y luego aplica las variables de entorno `VOCAT_*`. Las variables de entorno tienen prioridad.

| Variable de entorno | Predeterminado | Descripción |
| --- | --- | --- |
| `VOCAT_ADDR` | `0.0.0.0:7575` | Dirección de escucha HTTP. |
| `VOCAT_DATABASE_PATH` | `./data/vocat.db` | Ruta de la base de datos SQLite. |
| `VOCAT_SESSION_TTL` | `24h` | Duración de la sesión de autenticación. |
| `VOCAT_SECURE_COOKIES` | `false` | Marca las cookies de sesión como seguras cuando se usa HTTPS. |
| `VOCAT_SHUTDOWN_TIMEOUT` | `10s` | Tiempo de espera de apagado ordenado. |
| `VOCAT_MAX_REQUEST_BODY_BYTES` | `1048576` | Tamaño máximo del cuerpo de solicitud de la API. |

No almacene tokens de Telegram, contraseñas SMTP, secretos de webhook, credenciales SIM u otros datos privados en el repositorio. Configúrelos a través de los ajustes de la aplicación o archivos de entorno protegidos.

## Bot de Telegram

Cuando las notificaciones de Telegram están habilitadas y tanto el Chat ID como el Admin ID están configurados, el bot admite:

```text
/status [dispositivo]
/esim <dispositivo>
/switch <dispositivo> <iccid>
/wfc <dispositivo> <status|on|off|reconnect>
/sms <dispositivo> <número> <mensaje>
```

El cambio de perfil y el envío de SMS usan botones de confirmación de un solo uso. El bot no expone comandos de descarga, eliminación o renombrado de eSIM.

## Actualización

La actualización automática está deshabilitada. Revise los cambios upstream en una rama de integración temporal y despliegue solo un artefacto local verificado, compilado desde un SHA revisado y confirmado en los contenedores fijados.

## Desarrollo

Requisitos:

- Go 1.25 o más reciente
- Node.js 20 o más reciente
- npm

Ejecutar el servidor de desarrollo del frontend:

```bash
cd web
npm install
npm run dev
```

Compilar el frontend incrustado e iniciar el backend:

```bash
cd web
npm run build
cd ..
go run ./cmd/vocat
```

Ejecutar todas las pruebas:

```bash
go test ./...
```

Compilar un binario solo para desarrollo (no es un artefacto de publicación):

```bash
go build -trimpath -ldflags "-s -w" -o vocat ./cmd/vocat
```

## Controles de publicación

Los flujos `release` y `docker` de GitHub Actions están deshabilitados de forma intencionada y fallan de forma cerrada: no publican binarios, imágenes GHCR ni etiquetas `latest`. Una etiqueta Git es solo metadato del código fuente y no inicia un despliegue. Los artefactos se compilan localmente desde un SHA revisado y confirmado con `scripts/build-hardened.sh`; el instalador solo acepta el directorio de artefacto local verificado.

## Estructura del proyecto

```text
cmd/vocat/                  Punto de entrada de la aplicación y CLI
internal/device/            Descubrimiento de módems y control de dispositivos
internal/modem/             Sesión AT y manejo de respuestas
internal/server/            API HTTP, notificaciones y servidor web incrustado
internal/store/             Persistencia SQLite
internal/update/            Compatibilidad deshabilitada; sin actualizador en ejecución
internal/vowifi/            Runtime de IKE, EAP-AKA, IMS y WiFi Calling
scripts/build-hardened.sh   Compila un SHA confirmado en contenedores fijados
scripts/install.sh          Instala solo artefactos locales verificados; no descarga
web/src/                    Frontend en React y TypeScript
.github/workflows/          Controles fail-closed de publicación y Docker
```

## Uso responsable

Las operaciones con módems celulares y eSIM pueden afectar el servicio del abonado, los perfiles almacenados, el registro de red y el estado del hardware. Mantenga copias de seguridad, revise con cuidado las acciones destructivas y use el software solo en entornos legales donde tenga permiso para operar el hardware y los recursos de red conectados.

Vocat no elude la autenticación del operador, la política de red, la seguridad del hardware ni los requisitos de confianza de eSIM. El soporte de una operación significa que Vocat puede solicitarla al módem o al eUICC; el dispositivo, el perfil, la red o el operador aún pueden rechazarla.

## Contribuir

Las incidencias y pull requests son bienvenidas. Mantenga los cambios enfocados, incluya pruebas cuando sea práctico, evite confirmar credenciales o datos de abonados, y documente claramente el comportamiento específico del hardware.

Antes de enviar un cambio:

```bash
go test ./...
cd web && npm run build
```

## Agradecimientos
- [Nodeseek.com](https://www.nodeseek.com) — Una comunidad dedicada a los servidores
- [Linux.do](https://linux.do) — Una comunidad tecnológica inspiradora
- [iniwex5](https://github.com/iniwex5) — Guías de estilo y funcionalidad

## Invítame a un café

| Red | Dirección |
| ------- | ------- |
| USDT-TRON (TRC20) | `TQQAbboBoU8h5xX4YCA1rqWJU2WjK3seSg` |
| USDT-BSC (BEP20) | `0xdbfcd4a462550d6ff06d09cbd89026c6b145d9c4` |
| USDT-Polygon | `0xdbfcd4a462550d6ff06d09cbd89026c6b145d9c4` |

## Licencia

Consulte [LICENSE](../LICENSE).

[![MengMengCode/VoCat Star History](https://mengmeng.meteor-history.com/api/embed/MengMengCode/VoCat.svg?sig=sdeXRVxAoY3yLWgXL7JViY2USYIN3t9neJ6ScPvgUAo&theme=light&style=xkcd&color=dd4528&background=ffffff&textColor=000000&width=900&height=600&lineWidth=3&showTitle=true&showLegend=true&showDots=false&v=0.0.14)](https://meteor-history.com)
