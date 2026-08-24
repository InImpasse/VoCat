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

[English](../README.md) | **العربية** | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-TW.md) | [Français](README.fr.md) | [Русский](README.ru.md) | [Español](README.es.md) | [日本語](README.ja.md)

> [!IMPORTANT]
> الفرع المحصّن: التثبيت عن بُعد وصور upstream والإضافات وExport Proxy والتحديث الذاتي أثناء التشغيل معطّلة، كما أن مسارَي الإصدار وDocker مغلقان افتراضيًا (fail-closed). ابنِ وانشر محليًا فقط معرّف SHA خضع للمراجعة وتم إيداعه، وفق [security-hardening.md](security-hardening.md)؛ وللإنتاج اتبع [vm-deployment.md](vm-deployment.md).

Vocat هي لوحة تحكم ويب مفتوحة المصدر ومجموعة أدوات هندسية لمودمات Quectel الخلوية من فئة EC20/EC25. تجمع في خدمة واحدة مكتفية ذاتيًا بين اكتشاف المودم، وحالة الراديو المباشرة، وطرفيات AT وUSSD، والرسائل القصيرة SMS، وWiFi Calling، وإدارة eSIM، واختيار الشبكة، والتوجيه عبر البروكسي، والإشعارات، وسجلات التدقيق، ومسارات البناء والنشر المنضبطة.

الواجهة الخلفية مكتوبة بلغة Go، والواجهة مبنية باستخدام React وTypeScript، وتُضمَّن واجهة الإنتاج الأمامية داخل الملف الثنائي لـ Go. يحتوي ملف تنفيذي واحد على تطبيق الويب ويستخدم SQLite للحالة الدائمة.

<p align="center">
  <img src="../img/image.png">
  <img src="../img/image-1.png">
</p>

## الميزات

| المجال | ما يوفره Vocat |
| --- | --- |
| إدارة الأجهزة | اكتشاف تلقائي عبر المنفذ التسلسلي/USB، دعم عدة مودمات، أسماء أجهزة مألوفة، تحديثات مباشرة للنظرة العامة، إعادة تشغيل الوحدة، وضع الطيران، وضوابط وضع شبكة USB. |
| الراديو والشبكة | حالة التسجيل، المشغّل، مقاييس الإشارة، RSRP/RSRQ/SINR، وضع الشبكة، النطاق، القناة، فحص المشغّلين، والاختيار التلقائي أو اليدوي للشبكة. |
| AT وUSSD | طرفية AT تفاعلية، سجل الأوامر، استجابات المودم الخام، تدفقات بدء/متابعة/إلغاء USSD، والإبلاغ الواضح عن أخطاء المودم. |
| الرسائل القصيرة | إرسال مباشر للرسائل الخلوية ورسائل IMS، المزامنة الواردة، التعامل مع الرسائل متعددة الأجزاء، تقارير التسليم، سجل المحادثات، حالة عدم القراءة، الطوابع الزمنية، وحالة التسليم لكل رسالة. |
| WiFi Calling | إنشاء نفق IKEv2/ePDG، مصادقة EAP-AKA، تسجيل IMS، رسائل IMS القصيرة، ضوابط إعادة الاتصال، تشخيصات الحالة، والتوجيه لكل جهاز. |
| eSIM وeUICC | اكتشاف eUICC، ومعلومات EID والإنتاج، والبيانات الوصفية للشهادات، وجرد متعدد لـ eUICC، وقائمة الملفات الشخصية المثبتة، وعمليات التمكين/التعطيل/التبديل، وعمليات التنزيل وإعادة التسمية والحذف عندما تدعمها البطاقة. |
| سياسة البطاقة | سلوك WiFi Calling ووضع الطيران بناءً على ICCID مع تطبيق فوري للسياسة. |
| التوجيه عبر البروكسي | توجيه SOCKS صاعد، ربط الأجهزة، قواعد الدول، فحوصات الوصول عبر TCP، وفحوصات UDP Associate لمسارات بيانات WiFi Calling. |
| الإشعارات | إعادة توجيه الرسائل القصيرة الواردة الجديدة عبر Telegram وBark والبريد الإلكتروني وPushplus وwebhooks الموقّعة. يتم تسليم كل رسالة كإشعار منفصل. |
| بوت Telegram | حالة الجهاز، قائمة الملفات الشخصية المثبتة وتبديلها، ضوابط WiFi Calling، وإرسال الرسائل القصيرة. تتطلب الإجراءات الحساسة تأكيد المسؤول. |
| العمليات | المصادقة، الحماية من CSRF، سياسات الوصول، أحداث التدقيق، السجلات المباشرة، الاحتفاظ بالسجلات، فحوصات الصحة، تخطيط متجاوب، الوضع الداكن، وواجهة مستخدم بالإنجليزية/الصينية. |
| التوزيع | آثار Linux ثابتة تُبنى محليًا من معرّف SHA مُراجَع ومودَع داخل حاويات مؤقتة مثبتة الإصدارات، مع التحقق من manifest وSHA-256 ودعم استرجاع قاعدة البيانات. Compose للتطوير فقط؛ ونشر الملفات الثنائية وGHCR ووسم `latest` آليًا معطّل. |

## الأجهزة المدعومة

يستهدف Vocat وحدات Quectel المبنية على Qualcomm والتي توفر واجهات AT وQMI والمنفذ التسلسلي وشبكة USB المتوافقة، بما في ذلك:

- Quectel EC20
- Quectel EC25
- عائلة Quectel EG25
- وحدات EG600 المتوافقة وذات الصلة

تعتمد الميزات المتاحة على برنامج الوحدة الثابت (firmware)، وتكوين USB، وقدرات SIM/eSIM، وتعريفات المضيف، والشبكة اللاسلكية، وإعدادات المشغّل.

## التثبيت

التثبيت عن بُعد معطّل. ابنِ معرّف SHA الدقيق بعد مراجعته وإيداعه باستخدام
صور الحاويات المثبتة، ثم انشر دليل artifact الكامل. لا يقبل
`scripts/install.sh` إلا دليل artifact محليًا تم التحقق منه؛ ولا ينزّل إصدارًا
أو URL أو GitHub Release أو صورة حاوية:

```bash
scripts/build-hardened.sh amd64
RELEASE_COMMIT="$(git rev-parse HEAD)"
ARTIFACT_INDEX_SHA256='<trusted-64-hex-SHA256SUMS-hash>'
sudo scripts/install.sh --check-env
sudo scripts/install.sh --artifact "dist/hardened/$RELEASE_COMMIT" \
  --expected-commit "$RELEASE_COMMIT" \
  --expected-index-sha256 "$ARTIFACT_INDEX_SHA256"
```

سجّل قيمة `artifact index sha256` التي يطبعها مسار البناء عبر قناة موثوقة خارج
النطاق ومستقلة عن نقل artifact. لا تستخرج القيمة المتوقعة من `SHA256SUMS` داخل
الدليل المنسوخ. يتحقق مسار النشر من commit والفهرس المتوقعين ومن manifest وSHA-256 وإصدار Go، ويختبر ترحيل SQLite على
نسخة، ويستعيد الملف الثنائي وقاعدة البيانات معًا إذا فشل فحص الجاهزية.
يستخدم الإنتاج آلة KVM الافتراضية المخصصة وخدمة systemd الموضحة في
[vm-deployment.md](vm-deployment.md).

### Docker

ملف Compose الموجود في المستودع مخصص للتطوير المحلي فقط. يبني صورة
`vocat-hardened:local` محليًا، ولا يسحب صورة من upstream، ولا يستخدم الوضع
المميز، ولا يركب شجرة `/dev` الكاملة للمضيف:

```bash
docker compose build --pull=false

read -rsp "Admin password: " VOCAT_BOOTSTRAP_PASSWORD; echo
printf '%s\n' "$VOCAT_BOOTSTRAP_PASSWORD" | docker compose run --rm -T \
  --entrypoint /opt/vocat/bin/vocat vocat bootstrap-admin
unset VOCAT_BOOTSTRAP_PASSWORD
docker compose up -d
```

لا تستخدم حاوية مميزة أو ربطًا كاملًا لـ `/dev` في الإنتاج.

## الإعدادات

يقرأ Vocat ملف إعدادات JSON اختياريًا من `VOCAT_CONFIG`، ثم يطبق متغيرات البيئة `VOCAT_*`. متغيرات البيئة لها الأولوية.

| متغير البيئة | الافتراضي | الوصف |
| --- | --- | --- |
| `VOCAT_ADDR` | `0.0.0.0:7575` | عنوان الاستماع HTTP. |
| `VOCAT_DATABASE_PATH` | `./data/vocat.db` | مسار قاعدة بيانات SQLite. |
| `VOCAT_SESSION_TTL` | `24h` | مدة صلاحية جلسة المصادقة. |
| `VOCAT_SECURE_COOKIES` | `false` | يضع علامة آمنة على ملفات تعريف ارتباط الجلسة عند استخدام HTTPS. |
| `VOCAT_SHUTDOWN_TIMEOUT` | `10s` | مهلة الإيقاف السلس. |
| `VOCAT_MAX_REQUEST_BODY_BYTES` | `1048576` | الحد الأقصى لحجم جسم طلب API. |

لا تخزّن رموز Telegram، أو كلمات مرور SMTP، أو أسرار webhook، أو بيانات اعتماد SIM، أو بيانات خاصة أخرى في المستودع. قم بإعدادها عبر إعدادات التطبيق أو ملفات البيئة المحمية.

## بوت Telegram

عند تفعيل إشعارات Telegram وإعداد كلٍّ من Chat ID وAdmin ID، يدعم البوت:

```text
/status [الجهاز]
/esim <الجهاز>
/switch <الجهاز> <iccid>
/wfc <الجهاز> <status|on|off|reconnect>
/sms <الجهاز> <الرقم> <الرسالة>
```

تستخدم عمليتا تبديل الملفات الشخصية وإرسال الرسائل القصيرة أزرار تأكيد لمرة واحدة. لا يعرض البوت أوامر تنزيل أو حذف أو إعادة تسمية eSIM.

## التحديث

التحديث الذاتي أثناء التشغيل معطّل. راجع تغييرات upstream في فرع دمج مؤقت، ولا تنشر إلا artifact محليًا تم التحقق منه وبناؤه داخل الحاويات المثبتة من معرّف SHA مُراجَع ومودَع.

## التطوير

المتطلبات:

- Go 1.25 أو أحدث
- Node.js 20 أو أحدث
- npm

تشغيل خادم تطوير الواجهة الأمامية:

```bash
cd web
npm install
npm run dev
```

بناء الواجهة الأمامية المضمّنة وتشغيل الواجهة الخلفية:

```bash
cd web
npm run build
cd ..
go run ./cmd/vocat
```

تشغيل جميع الاختبارات:

```bash
go test ./...
```

بناء ملف ثنائي للتطوير فقط (ليس artifact للإصدار):

```bash
go build -trimpath -ldflags "-s -w" -o vocat ./cmd/vocat
```

## ضوابط الإصدار

مسارا `release` و`docker` في GitHub Actions معطّلان عمدًا ويعملان وفق fail-closed: لا ينشران ملفات ثنائية أو صور GHCR أو وسوم `latest`. وسم Git ليس إلا بيانات وصفية للمصدر ولا يشغّل النشر. ابنِ artifacts الإصدار محليًا من معرّف SHA مُراجَع ومودَع باستخدام `scripts/build-hardened.sh`، ولا تمرر إلى المثبّت إلا دليل artifact المحلي الذي تم التحقق منه.

## بنية المشروع

```text
cmd/vocat/                  نقطة دخول التطبيق وCLI
internal/device/            اكتشاف المودم والتحكم في الأجهزة
internal/modem/             جلسة AT ومعالجة الاستجابات
internal/server/            واجهة HTTP API والإشعارات وخادم الويب المضمّن
internal/store/             التخزين الدائم SQLite
internal/update/            كود توافق معطّل؛ لا يوجد محدّث أثناء التشغيل
internal/vowifi/            بيئة تشغيل IKE وEAP-AKA وIMS وWiFi Calling
scripts/build-hardened.sh   بناء SHA مودَع داخل حاويات مثبتة الإصدارات
scripts/install.sh          artifact محلي متحقق منه فقط؛ بلا تنزيل
web/src/                    الواجهة الأمامية React وTypeScript
.github/workflows/          حواجز fail-closed للإصدار وDocker
```

## الاستخدام المسؤول

يمكن أن تؤثر عمليات المودم الخلوي وeSIM في خدمة المشترك، والملفات الشخصية المخزنة، وتسجيل الشبكة، وحالة الأجهزة. احتفظ بنسخ احتياطية، وراجع الإجراءات المدمّرة بعناية، واستخدم البرنامج فقط في بيئات قانونية يُسمح لك فيها بتشغيل الأجهزة وموارد الشبكة المتصلة.

لا يتجاوز Vocat مصادقة المشغّل، أو سياسة الشبكة، أو أمان الأجهزة، أو متطلبات الثقة لـ eSIM. إن دعم عملية ما يعني أن Vocat يمكن أن يطلبها من المودم أو eUICC؛ وقد يرفضها الجهاز أو الملف الشخصي أو الشبكة أو المشغّل مع ذلك.

## المساهمة

نرحّب بالمسائل (issues) وطلبات السحب (pull requests). حافظ على التغييرات مركّزة، وأضِف الاختبارات حيثما أمكن، وتجنّب إيداع بيانات الاعتماد أو بيانات المشتركين، ووثّق بوضوح السلوك الخاص بالأجهزة.

قبل إرسال تغيير:

```bash
go test ./...
cd web && npm run build
```

## شكر وتقدير
- [Nodeseek.com](https://www.nodeseek.com) — مجتمع مكرّس للخوادم
- [Linux.do](https://linux.do) — مجتمع تقني ملهم
- [iniwex5](https://github.com/iniwex5) — إرشادات الأسلوب والوظائف

## ادعُني إلى فنجان قهوة

| الشبكة | العنوان |
| ------- | ------- |
| USDT-TRON (TRC20) | `TQQAbboBoU8h5xX4YCA1rqWJU2WjK3seSg` |
| USDT-BSC (BEP20) | `0xdbfcd4a462550d6ff06d09cbd89026c6b145d9c4` |
| USDT-Polygon | `0xdbfcd4a462550d6ff06d09cbd89026c6b145d9c4` |

## الرخصة

انظر [LICENSE](../LICENSE).

[![MengMengCode/VoCat Star History](https://mengmeng.meteor-history.com/api/embed/MengMengCode/VoCat.svg?sig=sdeXRVxAoY3yLWgXL7JViY2USYIN3t9neJ6ScPvgUAo&theme=light&style=xkcd&color=dd4528&background=ffffff&textColor=000000&width=900&height=600&lineWidth=3&showTitle=true&showLegend=true&showDots=false&v=0.0.14)](https://meteor-history.com)
