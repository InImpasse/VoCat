# 生产宿主机隔离部署手册

本文只描述经过审阅后才能执行的部署流程。仓库脚本默认使用只读的 `--check`；本次代码加固不会自动调用 `sudo`、安装软件、创建虚拟机、修改宿主服务或升级调制解调器固件。

## 安全边界

- 所有 `sudo`、LUKS 密码、恢复密钥和 Tailscale 登录都必须在宿主机或 VM 的私密本地控制台中完成。不得把密码、auth key、SIM 信息或设备序列号粘贴到聊天、命令参数、日志或仓库文件。
- VM 中使用 systemd 直接运行 VoCat，不运行特权 Docker。服务只保留 `CAP_NET_ADMIN` 和 `CAP_NET_RAW`，但这两项长期能力仍允许已攻陷的服务修改来宾网络、路由/XFRM 状态并使用原始套接字，属于明确保留的残余风险。来宾内 nftables 只提供正常运行时的入口控制，不能作为服务发生 RCE 后的安全边界；需要抗来宾内服务失陷的隔离时，必须在宿主或上游路由器实施独立 ACL，长期应把特权网络操作拆到窄接口 helper。
- 大容量存储只保存 ISO、构建缓存和加密备份。VM 的 qcow2、当前 release 和运行中的 SQLite 必须位于 SSD。
- LAN 上保留的 HTTP 会话可能被同网段监听。7575 仅允许内部 readiness 使用的 loopback、私密提供的可信 LAN CIDR 与 `tailscale0`，远程管理优先使用 Tailscale。
- USB 直通只允许一个经过 VID/PID、序列号和 `ID_PATH` 联合确认的 USB 设备。不得直通宿主机唯一的 xHCI/PCI 控制器。
- 识别到 USB、SIM Ready、ePDG 建链和 IMS 注册是不同验收层级，不能互相替代。

## 1. 宿主变更前检查

先在只读模式记录当前 Docker 容器、端口、路由、DNS、磁盘和健康状态，保存到受控的本地运维记录中，不提交到仓库：

```bash
docker ps --no-trunc
ss -lntup
ip -brief address
ip route
findmnt -T / -o SOURCE,TARGET,FSTYPE,OPTIONS
findmnt -T "$BULK_STORAGE_ROOT" -o SOURCE,TARGET,FSTYPE,OPTIONS
lsblk -o NAME,TYPE,ROTA,SIZE,MOUNTPOINTS
df -h / "$BULK_STORAGE_ROOT"
```

创建 VM 前，SSD 必须至少有 120 GiB 可用空间。`create-vocat-vm.sh` 会解析实际物理后端，遇到旋转盘、私密提供的大容量存储根目录或不足 120 GiB 时直接停止。清理时只能删除已确认没有进程、挂载或 Git worktree 引用的旧临时文件，以及超过 14 天且没有使用者的 BuildKit 缓存。禁止运行 `docker system prune --volumes`。

### 检查意外安装的 LXD

先检查 snap 事务，不要再次运行 Ubuntu 的 `lxc` 包装器：

```bash
snap changes
snap list lxd
```

如果 LXD 安装仍处于 `Doing`，只能在私密 sudo 终端核对对应事务后执行 `sudo snap abort <change-id>`。如果已经安装，先使用 `/snap/bin/lxc` 核实没有实例、需要保留的存储池或网络：

```bash
sudo /snap/bin/lxc list
sudo /snap/bin/lxc storage list
sudo /snap/bin/lxc network list
```

只有确认它是本次误触发且没有任何消费者后，才可执行 `sudo snap remove lxd --purge`。这一步具有破坏性，不能由本仓库脚本自动完成。

## 2. 准备 KVM 宿主

先只读检查，再在宿主机的私密终端中执行安装。脚本本身不会调用 sudo。以下变量只在私密终端赋值，不写入仓库或共享日志：

```bash
./scripts/prepare-kvm-host.sh --check --lan-interface "$LAN_INTERFACE"
sudo ./scripts/prepare-kvm-host.sh --apply --admin-user "$LOCAL_ADMIN" --lan-interface "$LAN_INTERFACE"
```

安装后退出并重新登录，使 `libvirt`/`kvm` 组生效，然后再次运行 `--check`。脚本安装 QEMU/KVM、libvirt、OVMF、virt-install、swtpm、guest-agent 所需宿主组件，启用 libvirt `default` NAT 网络，并验证私密提供的 LAN 接口。它不会创建 VM 或 USB 直通。

从 Ubuntu 官方 HTTPS 站点取得 24.04.x live-server amd64 ISO、`SHA256SUMS` 和 `SHA256SUMS.gpg`，放入私密提供的大容量存储目录。先按 Ubuntu 官方发布密钥流程验证签名，再取得该 ISO 对应的 SHA-256。不能只信任同一次未认证下载中的散列值。

## 3. 创建并加密安装 VM

先执行 dry run，再创建固定规格 VM。`<verified-sha256>` 不是秘密，但必须来自已验证签名的校验文件：

```bash
sudo ./scripts/create-vocat-vm.sh --dry-run \
  --disk-dir /var/lib/libvirt/images/vocat \
  --bulk-storage-root "$BULK_STORAGE_ROOT" \
  --lan-interface "$LAN_INTERFACE" \
  --iso "$ISO_PATH" \
  --iso-sha256 <verified-sha256>

sudo ./scripts/create-vocat-vm.sh --create \
  --disk-dir /var/lib/libvirt/images/vocat \
  --bulk-storage-root "$BULK_STORAGE_ROOT" \
  --lan-interface "$LAN_INTERFACE" \
  --iso "$ISO_PATH" \
  --iso-sha256 <verified-sha256>
```

`--create` 不会把大容量存储上的原始 ISO 直接交给 QEMU。它先把 ISO 复制到 SSD 的 root 管理目录，建立只对 root/libvirt 身份开放的 installer snapshot，检查普通文件、所有权、权限和单链接属性，再按 Ubuntu 签名校验文件中的预期 SHA-256 重新计算并核对副本，最后才调用 `virt-install`。创建失败且新域可安全撤销时会清理临时文件、snapshot 和新 qcow2；若域无法安全 undefine，则保留相关文件并要求人工恢复，避免删除仍被引用的数据。

生成规格为 q35/KVM、UEFI Secure Boot、vTPM 2.0、4 vCPU、8 GiB 内存、64 GiB thin qcow2、virtio-scsi/discard。第一张 virtio 网卡连接 libvirt NAT，供宿主机管理；第二张通过 macvtap 直连私密提供的 LAN 接口。macvtap 默认不能让宿主直接访问来宾，因此 NAT 管理网必须保留。

连接本地控制台：

```bash
virt-viewer --connect qemu:///system vocat
# 安装后也可使用串口控制台：
virsh --connect qemu:///system console vocat
```

SPICE 只监听宿主 loopback，不向 LAN 暴露。首次安装和每次宿主重启后的 LUKS 解锁优先使用本地 `virt-viewer`；确认来宾已配置串口控制台后，也可使用 `virsh console`。

在 Ubuntu 安装器中选择使用整个 64 GiB 虚拟磁盘、建立 LVM，并启用 LUKS 加密。LUKS 密码只在安装器控制台输入，恢复密钥离线保存，不使用 cloud-init、命令参数或仓库文件传递。保留人工解锁：宿主重启时域会自动启动，但来宾在控制台输入 LUKS 密码前不会启动 VoCat。

安装完成后确认两张网卡都获得地址，LAN DHCP 地址无冲突后再到路由器设置固定租约；不要把 MAC、租约或实际地址提交到仓库。随后验证固定配置。日常 `--check` 会针对 qcow2 文件的真实后端重新确认 SSD、所有权、权限、无 backing/external data file 以及受限的 libvirt 设备集合；120 GiB 空闲空间只作为创建前门槛：

```bash
sudo ./scripts/create-vocat-vm.sh --check \
  --disk-dir /var/lib/libvirt/images/vocat \
  --bulk-storage-root "$BULK_STORAGE_ROOT" \
  --lan-interface "$LAN_INTERFACE"
```

成功创建后，SSD 上已校验的 installer snapshot 会保留到系统安装完成并弹出介质，不能在 QEMU 仍引用它时手工删除。系统安装完成后必须从 inactive 与 live 域配置中弹出 ISO；保留的 CD-ROM 只能是只读 raw 设备且不得挂载介质。先用 `virsh domblklist vocat --details` 确认实际 CD-ROM target，再分别处理持久和运行中配置，最后不带 `--iso` 运行上述 `--check`。检查会同时拒绝 live/config 中残留或漂移的安装介质及未授权存储设备。确认 inactive/live 配置均已弹出且 `--check` 通过后，才可删除该 installer snapshot。

## 4. 准备来宾并部署 VoCat

在 VM 私密控制台中，从同一受审阅 commit 提供脚本和 `deploy/` 文件，然后执行：

```bash
sudo ./scripts/prepare-vocat-guest.sh --dry-run --lan-cidr "$LAN_CIDR"
sudo ./scripts/prepare-vocat-guest.sh --apply --lan-cidr "$LAN_CIDR"
sudo tailscale up
sudo ./scripts/prepare-vocat-guest.sh --check --lan-cidr "$LAN_CIDR"
```

来宾脚本验证 Tailscale 仓库签名密钥指纹，安装 `qemu-guest-agent`、`libqmi-utils`、nftables 和 Tailscale，mask ModemManager，并加载独立的 `inet vocat_ingress` 表。规则只处理 TCP/7575，不刷新或接管其他 nftables 表；LAN 放行同时要求 `fib saddr . iif oif exists` 反向路径成立，阻止从错误接口伪造可信 LAN CIDR 的源地址。脚本会先验证包含 staged 文件的完整 ruleset，再以可回滚事务替换配置，并用 nft JSON 复核 live 规则语义。`vocat-firewall.service` 使用 `RequiredBy=vocat.service`，防火墙加载失败时 VoCat 不应启动。应用的内网策略默认包含 Tailscale IPv4 `100.64.0.0/10` 和 IPv6 ULA，因此无需改成公网模式。

Tailscale 登录是独立的交互步骤；不要向脚本传 auth key。分别从可信 LAN、Tailscale 和一个不可信入口实测：前两者可以访问 7575，不可信入口必须被拒绝。还要确认宿主 NAT 管理地址不能绕过该规则。

构建产物必须由已提交且通过发布门禁的 SHA 产生。构建器输出的 `artifact index sha256` 是 `SHA256SUMS` 文件本身的 SHA-256；在复制 release 目录前，将 reviewed commit 和该 index hash 记录到独立于产物传输的可信带外渠道。不得从到达 VM 的 manifest、目录名或 `SHA256SUMS` 重新推导这两个预期值。将完整 release 目录复制到 VM 后执行：

```bash
sudo ./scripts/deploy-hardened.sh \
  --expected-commit "<reviewed-40-hex-commit>" \
  --expected-index-sha256 "<trusted-64-hex-SHA256SUMS-hash>" \
  "/path/to/dist/hardened/<reviewed-40-hex-commit>"
```

两个 `--expected-*` 参数均为强制参数。部署器先用带外 index 认证 `SHA256SUMS`，在解析任何 JSON 前核对安全路径、完整文件库存和全部文件散列；随后重新解析原始 Gitleaks、npm audit、govulncheck SARIF 和 CycloneDX SBOM，并要求推导结果、固定工具链、报告映射、阈值、扫描模式和完整性范围与 schema 2 manifest 完全一致。它还会确认实际二进制元数据为 Go 1.26.7/linux/amd64。部署由私有 `flock` 串行化；在线 SQLite 只由 `vocat` 身份通过 backup API 读取，主库及现存 WAL/SHM 必须是 `vocat:vocat`、0600、单链接普通文件。生成的副本被原子移入受保护目录并重新校验后，迁移副本才交给无登录、无补充组的 `vocat-preflight` 账号，在隐藏宿主 `/run` socket、无网络和严格资源限制的 transient systemd 单元中执行。运行数据库位于 VM SSD 的 `/var/lib/vocat`，root-only 的一致性回滚快照位于 `/var/backups/vocat`，服务账号不能改写或删除。部署前后的 SQLite 迁移、readiness 以及二进制和数据库联合回滚都必须通过。

## 5. USB 枚举与精确直通

使用已知良好的 USB 数据线把模块直接连接宿主。当前只有在 `lsusb` 出现真实设备、并且 sysfs 同时提供 USB 序列号和稳定 `ID_PATH` 时才能继续。以下命令只列出匹配 `2ca3:4006` 的 sysfs 名称，不打印序列号：

```bash
for p in /sys/bus/usb/devices/*; do
  [ -r "$p/idVendor" ] && [ -r "$p/idProduct" ] || continue
  [ "$(<"$p/idVendor")" = 2ca3 ] && [ "$(<"$p/idProduct")" = 4006 ] && basename "$p"
done
```

把输出的单个名称作为 `<usb-sysname>`。先只读检查，再在私密宿主终端生成 root-only 白名单：

```bash
sudo ./scripts/configure-dji-usb-passthrough.sh --check --sysname <usb-sysname>
sudo ./scripts/configure-dji-usb-passthrough.sh --dry-run --sysname <usb-sysname>
sudo ./scripts/configure-dji-usb-passthrough.sh --apply --sysname <usb-sysname>
```

脚本不会显示序列号或 `ID_PATH`。真实值只写入宿主 `/etc/vocat/dji-usb.conf`，权限为 root:root 0600；仓库中的 udev 文件只是占位模板。热插拔处理器每次重新核对完整身份，只向 libvirt 的 inactive 与 live 配置添加同一个 `type='usb'` 设备，设置 `managed='yes'` 和 `startupPolicy='optional'`，并拒绝两种配置的漂移、任何非本工具管理的 USB hostdev 或 PCI/xHCI 直通。

在 VM 中验证冷启动、热插拔、拔出、重新插入和 VM 重启。预期一代模块提供 0-3 串口（接口 2 通常是 AT）及接口 4 QMI；实际节点必须以本机枚举为准。禁用 ModemManager 后，再检查 `/dev/ttyUSB*`、`/dev/cdc-wdm*` 和 QMI 只读状态。固件读取脚本会在发送命令前确认所选 tty 的 sysfs 接口号严格为 `02`。只有设备精确匹配时才手工启动 `vocat-dji-repair.service`，不得把修复单元改为自动或宽泛匹配。

## 6. 固件与 T-Mobile 验收

固件采集应在维护窗内先停 VoCat，避免它与只读采集竞争 AT 端口；采集结束后立即恢复服务：

```bash
sudo systemctl stop vocat.service
./scripts/read-dji-firmware.sh --check --device /dev/ttyUSB2
./scripts/read-dji-firmware.sh --read --device /dev/ttyUSB2
sudo systemctl start vocat.service
```

脚本只发送 `ATI`、`AT+CGMM`、`AT+CGMR`，原始响应不会输出；最终仅显示经过限制的厂商、型号和固件版本。若响应看起来像账户或设备标识，整项显示为 `unavailable`。不要把原始 AT 会话、IMEI、ICCID、IMSI、SIM PIN、短信或运营商凭据写入日志或工单。

T-Mobile/Wi-Fi Calling 按以下顺序验收：

1. SIM 状态为 Ready，且不披露 SIM 标识。
2. 内部运营商配置匹配 `310260`。
3. ePDG/IKE 隧道成功建立。
4. IMS 完成注册。
5. 最后在已授权窗口完成一次通话和短信测试。

首轮部署不升级固件。只有官方签名包与硬件、地区和运营商分支完全匹配，并已准备 NV/QCN 加密备份、回滚和救砖路径时，才能另开维护窗。

## 7. 备份、回滚与浸泡

- SQLite 必须在停服或使用 SQLite online backup API 的一致性状态下备份。数据库升级失败时，同时恢复旧 release 链接和升级前数据库，不能只回滚二进制。
- 大容量存储上的 SQLite 或 VM 备份必须在写入前加密，密钥不得保存在同一存储；同时保留一份通过校验的异机副本。单盘存储不构成可靠备份。
- 整机 qcow2 备份应在来宾关机或受控快照状态完成；即使来宾磁盘已使用 LUKS，外部备份仍按敏感数据再次加密。
- 上线后与宿主既有负载并行浸泡 24-48 小时，复核变更前私下记录的容器、监听端口、DNS、路由和出网状态。
- 浸泡期必须实测坏构建联合回滚、来宾重启后的人工 LUKS 解锁、USB 重枚举、热拔插以及 Tailscale/LAN 防火墙路径。

只有代码门禁、VM 安全配置、真实 USB/QMI、IMS 与授权通话测试全部通过，才能把该 release 标记为生产部署完成。
