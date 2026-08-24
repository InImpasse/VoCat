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

[English](../README.md) | [العربية](README.ar.md) | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-TW.md) | **Français** | [Русский](README.ru.md) | [Español](README.es.md) | [日本語](README.ja.md)

> [!IMPORTANT]
> Fork renforcé : l'installation distante, les images upstream, les plugins, Export Proxy et l'auto-mise à jour à l'exécution sont désactivés ; les workflows de publication et Docker échouent en mode fermé. Compilez et déployez localement uniquement un SHA relu et commité selon [security-hardening.md](security-hardening.md) ; pour la production, suivez [vm-deployment.md](vm-deployment.md).

Vocat est un panneau de contrôle web open-source et une boîte à outils d'ingénierie pour les modems cellulaires Quectel de classe EC20/EC25. Il réunit, dans un service autonome unique, la découverte de modems, l'état radio en direct, les terminaux AT et USSD, les SMS, la WiFi Calling, la gestion eSIM, la sélection de réseau, le routage par proxy, les notifications, les journaux d'audit et des processus contrôlés de compilation et de déploiement.

Le backend est écrit en Go, l'interface est construite avec React et TypeScript, et le frontend de production est intégré dans le binaire Go. Un seul exécutable contient l'application web et utilise SQLite pour l'état persistant.

<p align="center">
  <img src="../img/image.png">
  <img src="../img/image-1.png">
</p>

## Fonctionnalités

| Domaine | Ce que Vocat fournit |
| --- | --- |
| Gestion des appareils | Découverte série/USB automatique, prise en charge de plusieurs modems, noms d'appareils conviviaux, mises à jour en direct de la vue d'ensemble, redémarrage du module, mode avion et contrôles du mode réseau USB. |
| Radio et réseau | État d'enregistrement, opérateur, métriques de signal, RSRP/RSRQ/SINR, mode réseau, bande, canal, recherche d'opérateurs et sélection de réseau automatique ou manuelle. |
| AT et USSD | Terminal AT interactif, historique des commandes, réponses brutes du modem, flux de démarrage/poursuite/annulation USSD et rapport d'erreurs modem clair. |
| SMS | Envoi direct de SMS cellulaires et IMS, synchronisation entrante, gestion des messages multiparties, rapports de livraison, historique des conversations, état non lu, horodatages et statut de livraison par message. |
| WiFi Calling | Établissement de tunnel IKEv2/ePDG, authentification EAP-AKA, enregistrement IMS, SMS IMS, contrôles de reconnexion, diagnostics d'état et routage par appareil. |
| eSIM et eUICC | Découverte eUICC, EID et informations de production, métadonnées de certificat, inventaire multi-eUICC, liste des profils installés, opérations d'activation/désactivation/commutation, ainsi que téléchargement, renommage et suppression lorsque la carte le permet. |
| Politique de carte | Comportement WiFi Calling et mode avion basé sur l'ICCID avec application immédiate de la politique. |
| Routage par proxy | Routage SOCKS amont, liaisons d'appareils, règles par pays, vérifications d'accessibilité TCP et vérifications UDP Associate pour les chemins de données WiFi Calling. |
| Notifications | Transfert des nouveaux SMS entrants via Telegram, Bark, e-mail, Pushplus et webhooks signés. Chaque SMS est livré comme une notification individuelle. |
| Bot Telegram | État de l'appareil, liste et commutation des profils installés, contrôles WiFi Calling et envoi de SMS. Les actions sensibles nécessitent une confirmation de l'administrateur. |
| Exploitation | Authentification, protection CSRF, politiques d'accès, événements d'audit, journaux en direct, rétention des journaux, vérifications de santé, mise en page réactive, mode sombre et interface utilisateur en anglais/chinois. |
| Distribution | Artefacts Linux statiques compilés localement depuis un SHA relu et commité dans des conteneurs temporaires épinglés, avec vérification du manifest et de SHA-256 et rollback de base de données. Compose est réservé au développement ; la publication automatique de binaires, de GHCR et de `latest` est désactivée. |

## Matériel pris en charge

Vocat cible les modules Quectel à base Qualcomm qui exposent des interfaces AT, QMI, série et réseau USB compatibles, notamment :

- Quectel EC20
- Quectel EC25
- Famille Quectel EG25
- Modules EG600 compatibles et apparentés

Les fonctionnalités disponibles dépendent du firmware du module, de la composition USB, des capacités SIM/eSIM, des pilotes hôtes, du réseau radio et de la configuration de l'opérateur.

## Installation

L'installation distante est désactivée. Construisez le SHA exact, relu et commité,
avec les images de conteneur épinglées, puis déployez le répertoire d'artefact
complet. `scripts/install.sh` accepte uniquement un répertoire d'artefact local
vérifié ; il ne télécharge ni version, ni URL, ni GitHub Release, ni image :

```bash
scripts/build-hardened.sh amd64
RELEASE_COMMIT="$(git rev-parse HEAD)"
ARTIFACT_INDEX_SHA256='<hachage-SHA256SUMS-fiable-sur-64-hex>'
sudo scripts/install.sh --check-env
sudo scripts/install.sh --artifact "dist/hardened/$RELEASE_COMMIT" \
  --expected-commit "$RELEASE_COMMIT" \
  --expected-index-sha256 "$ARTIFACT_INDEX_SHA256"
```

Consignez la valeur `artifact index sha256` produite par le compilateur via un
canal fiable hors bande, indépendant du transfert de l'artefact. Ne calculez pas
la valeur attendue depuis `SHA256SUMS` dans le répertoire copié. Le déploiement
vérifie le commit et l'index attendus, le manifest, SHA-256 et la version de Go, teste la migration
SQLite sur une copie, puis restaure ensemble le binaire et la base de données si
readiness échoue. La production utilise la VM KVM dédiée et le service systemd
décrits dans [vm-deployment.md](vm-deployment.md).

### Docker

Le profil Compose fourni est réservé au développement local. Il construit
`vocat-hardened:local`, ne télécharge aucune image upstream, n'utilise pas le mode
privilégié et ne monte pas l'arborescence `/dev` complète de l'hôte :

```bash
docker compose build --pull=false

read -rsp "Admin password: " VOCAT_BOOTSTRAP_PASSWORD; echo
printf '%s\n' "$VOCAT_BOOTSTRAP_PASSWORD" | docker compose run --rm -T \
  --entrypoint /opt/vocat/bin/vocat vocat bootstrap-admin
unset VOCAT_BOOTSTRAP_PASSWORD
docker compose up -d
```

N'utilisez pas de conteneur privilégié et ne montez pas l'arborescence `/dev`
complète en production.

## Configuration

Vocat lit un fichier de configuration JSON optionnel depuis `VOCAT_CONFIG`, puis applique les variables d'environnement `VOCAT_*`. Les variables d'environnement ont la priorité.

| Variable d'environnement | Par défaut | Description |
| --- | --- | --- |
| `VOCAT_ADDR` | `0.0.0.0:7575` | Adresse d'écoute HTTP. |
| `VOCAT_DATABASE_PATH` | `./data/vocat.db` | Chemin de la base de données SQLite. |
| `VOCAT_SESSION_TTL` | `24h` | Durée de vie de la session d'authentification. |
| `VOCAT_SECURE_COOKIES` | `false` | Marque les cookies de session comme sécurisés lorsque HTTPS est utilisé. |
| `VOCAT_SHUTDOWN_TIMEOUT` | `10s` | Délai d'arrêt gracieux. |
| `VOCAT_MAX_REQUEST_BODY_BYTES` | `1048576` | Taille maximale du corps de requête API. |

Ne stockez pas de jetons Telegram, mots de passe SMTP, secrets de webhook, identifiants SIM ou autres données privées dans le dépôt. Configurez-les via les paramètres de l'application ou des fichiers d'environnement protégés.

## Bot Telegram

Lorsque les notifications Telegram sont activées et que le Chat ID et l'Admin ID sont configurés, le bot prend en charge :

```text
/status [appareil]
/esim <appareil>
/switch <appareil> <iccid>
/wfc <appareil> <status|on|off|reconnect>
/sms <appareil> <numéro> <message>
```

La commutation de profil et l'envoi de SMS utilisent des boutons de confirmation à usage unique. Le bot n'expose pas les commandes de téléchargement, de suppression ou de renommage eSIM.

## Mise à jour

La mise à jour automatique est désactivée. Examinez les changements upstream dans une branche d'intégration temporaire et ne déployez qu'un artefact local vérifié, compilé depuis un SHA relu et commité dans les conteneurs épinglés.

## Développement

Prérequis :

- Go 1.25 ou plus récent
- Node.js 20 ou plus récent
- npm

Lancer le serveur de développement frontend :

```bash
cd web
npm install
npm run dev
```

Construire le frontend intégré et démarrer le backend :

```bash
cd web
npm run build
cd ..
go run ./cmd/vocat
```

Exécuter tous les tests :

```bash
go test ./...
```

Construire un binaire réservé au développement (ce n'est pas un artefact de publication) :

```bash
go build -trimpath -ldflags "-s -w" -o vocat ./cmd/vocat
```

## Contrôles de publication

Les workflows GitHub Actions `release` et `docker` sont volontairement désactivés et échouent en mode fermé : ils ne publient ni binaires, ni images GHCR, ni tags `latest`. Un tag Git est uniquement une métadonnée du code source et ne déclenche aucun déploiement. Compilez localement les artefacts depuis un SHA relu et commité avec `scripts/build-hardened.sh` ; l'installeur accepte uniquement le répertoire d'artefact local vérifié.

## Structure du projet

```text
cmd/vocat/                  Point d'entrée de l'application et CLI
internal/device/            Découverte de modems et contrôle des appareils
internal/modem/             Session AT et gestion des réponses
internal/server/            API HTTP, notifications et serveur web intégré
internal/store/             Persistance SQLite
internal/update/            Compatibilité désactivée ; aucun updater à l'exécution
internal/vowifi/            Runtime IKE, EAP-AKA, IMS et WiFi Calling
scripts/build-hardened.sh   Compile un SHA commité dans des conteneurs épinglés
scripts/install.sh          Installe uniquement l'artefact local vérifié ; aucun téléchargement
web/src/                    Frontend React et TypeScript
.github/workflows/          Garde-fous fail-closed de publication et Docker
```

## Utilisation responsable

Les opérations sur les modems cellulaires et les eSIM peuvent affecter le service de l'abonné, les profils stockés, l'enregistrement réseau et l'état du matériel. Effectuez des sauvegardes, examinez attentivement les actions destructrices et n'utilisez le logiciel que dans des environnements légaux où vous êtes autorisé à exploiter le matériel et les ressources réseau connectés.

Vocat ne contourne ni l'authentification de l'opérateur, ni la politique réseau, ni la sécurité matérielle, ni les exigences de confiance eSIM. La prise en charge d'une opération signifie que Vocat peut la demander au modem ou à l'eUICC ; l'appareil, le profil, le réseau ou l'opérateur peut toujours la refuser.

## Contribution

Les issues et pull requests sont les bienvenues. Gardez des changements ciblés, incluez des tests lorsque c'est possible, évitez de committer des identifiants ou des données d'abonnés, et documentez clairement les comportements spécifiques au matériel.

Avant de soumettre un changement :

```bash
go test ./...
cd web && npm run build
```

## Remerciements
- [Nodeseek.com](https://www.nodeseek.com) — Une communauté dédiée aux serveurs
- [Linux.do](https://linux.do) — Une communauté technologique inspirante
- [iniwex5](https://github.com/iniwex5) — Directives de style et de fonctionnalité

## Offrez-moi un café

| Réseau | Adresse |
| ------- | ------- |
| USDT-TRON (TRC20) | `TQQAbboBoU8h5xX4YCA1rqWJU2WjK3seSg` |
| USDT-BSC (BEP20) | `0xdbfcd4a462550d6ff06d09cbd89026c6b145d9c4` |
| USDT-Polygon | `0xdbfcd4a462550d6ff06d09cbd89026c6b145d9c4` |

## Licence

Voir [LICENSE](../LICENSE).

[![MengMengCode/VoCat Star History](https://mengmeng.meteor-history.com/api/embed/MengMengCode/VoCat.svg?sig=sdeXRVxAoY3yLWgXL7JViY2USYIN3t9neJ6ScPvgUAo&theme=light&style=xkcd&color=dd4528&background=ffffff&textColor=000000&width=900&height=600&lineWidth=3&showTitle=true&showLegend=true&showDots=false&v=0.0.14)](https://meteor-history.com)
