# VoCat VM 部署手册

[English](VM_DEPLOYMENT.md) | **简体中文**

这是加固分支的生产部署主文档。生产环境使用最小化 Ubuntu Server 24.04
来宾、systemd、单张 libvirt NAT 网卡和宿主机反向代理。Docker 只用于可复现
构建，不在生产 VM 内运行。

> [!IMPORTANT]
> 构建通过只能说明它是候选版本。生产验收还必须通过 VM 配置、完整产物校验、
> 宿主反向代理边界和真实调制解调器验收。不得直接部署上游构建。

## 安全模型

- VM qcow2、当前 release 和在线 SQLite 必须位于 SSD。
- 大容量存储只保存 ISO、构建缓存和加密备份。
- 来宾必须恰好只有一张连接 libvirt `default` NAT 的 virtio 网卡。
- 不允许来宾直连 LAN，也不在来宾内安装 Tailscale。
- Web 页面只通过宿主反向代理发布。来宾 TCP/7575 只允许 loopback 和一个
  私密配置的宿主代理源 IPv4。
- VoCat 使用加固 systemd 单元直接运行，不在来宾中运行特权 Docker。
- 密码、恢复密钥、SIM 标识、私有地址、USB 身份和通知凭据不得进入 Git、
  命令输出或日志。
- `CAP_NET_ADMIN` 和 `CAP_NET_RAW` 是保留风险。来宾 nftables 只限制正常入口，
  VoCat 被攻陷后不能把它当作隔离边界。

## 1. 私密保存宿主配置

宿主专用设置放在项目内已忽略的 `.vocat-local/host-deployment.json`，权限必须
为 `0600`，不得提交真实配置。schema 3 结构如下：

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

使用前确认本地保护：

```bash
chmod 0600 .vocat-local/host-deployment.json
git check-ignore .vocat-local/host-deployment.json
./.vocat-local/vocat-hostctl validate
```

ISO 散列必须来自 Ubuntu 已签名的 `SHA256SUMS`，不能只信任未认证镜像返回的
散列。控制器只接受 2-4 vCPU、2048-8192 MiB 内存（256 MiB 步进）和
24-64 GiB thin qcow2。

## 2. 准备 KVM 宿主

先执行只读检查，只在私密本地终端应用变更：

```bash
./.vocat-local/vocat-hostctl host-check
sudo ./.vocat-local/vocat-hostctl host-apply
```

组成员变更后退出并重新登录，确认已加入 `libvirt` 和 `kvm`，再执行一次
`host-check`。`qemu-guest-agent.service` 是静态单元时出现无法 enable 的提示是
正常现象；来宾中的 `systemctl is-active qemu-guest-agent` 必须返回 `active`。

创建 VM 前，SSD 可用空间必须至少为 `max(48 GiB, 2 x 虚拟磁盘大小)`。
不得使用 `docker system prune --volumes` 腾出空间。

## 3. 创建并安装 VM

先审阅 dry run，再创建：

```bash
sudo ./.vocat-local/vocat-hostctl vm-dry-run
sudo ./.vocat-local/vocat-hostctl vm-create
```

轻量配置为 q35/KVM、UEFI Secure Boot、vTPM 2.0、2 vCPU、2048 MiB 内存、
24 GiB thin qcow2、virtio-scsi/discard，以及一张 libvirt `default` NAT 网卡。

只从宿主连接控制台：

```bash
virt-viewer --connect qemu:///system vocat
# 串口控制台配置完成后：
virsh --connect qemu:///system console vocat
```

安装器中执行：

1. 选择 `Ubuntu Server (minimized)`。
2. 不安装 GUI、第三方驱动或可选 snap。
3. 整个虚拟磁盘使用 LVM，并启用 LUKS 加密。
4. LUKS 密码只在私密控制台输入，恢复密钥离线保存。
5. 只有需要宿主向来宾传输文件时才安装 OpenSSH，并且只能在 libvirt 私有网络使用。

重启后从控制台解锁 LUKS，从 inactive 和 live 域配置中弹出 ISO，然后执行：

```bash
sudo ./.vocat-local/vocat-hostctl vm-check-installed
```

检查必须确认资源规格、SSD qcow2、单张 NAT 网卡、没有安装介质，也没有额外
存储、网卡、PCI 或 USB 设备。只有通过后才能删除已验证的 installer snapshot。

如果 VM 已经安装并且该检查通过，直接从第 4 节继续。

## 4. 构建并记录候选版本

只能构建干净、已审阅且已提交的 SHA。推送或 PR 的 CI 必须在生产部署前通过。
本地候选构建命令为：

```bash
git status --short
git rev-parse HEAD
VOCAT_BUILD_CACHE_ROOT="$(jq -er '.bulk_storage_root' \
  .vocat-local/host-deployment.json)/cache/vocat" \
  ./scripts/build-hardened.sh amd64
```

该命令只为本次构建进程从私有 JSON 读取缓存目录，不要求 `export`，也不会把
宿主路径写入受版本控制文件。

构建使用固定临时容器，输出到 `dist/hardened/<40_HEX_COMMIT>/`。在复制产物前，
通过独立于产物传输的可信渠道记录：

```text
reviewed commit:        <40_HEX_COMMIT>
SHA256SUMS index hash:  <64_HEX_SHA256_OF_SHA256SUMS>
```

不得从到达 VM 后的目录名、manifest 或 `SHA256SUMS` 推导这两个预期值。完整
产物必须包含 schema 2、Gitleaks、完整和生产 npm audit、普通/race Go 测试、
vet、源码与二进制 govulncheck、双 SBOM、Go 1.26.7 二进制元数据及全文件校验。

## 5. 准备来宾

通过宿主到来宾的私有路径传输精确 commit 的 `git archive` 和完整产物目录，
不要复制脏工作树。在宿主替换两个占位符后执行：

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

然后进入来宾源码快照目录并执行：

```bash
cd "$HOME/vocat-transfer"
sudo ./scripts/prepare-vocat-guest.sh --dry-run \
  --proxy-source-ipv4 <HOST_PROXY_SOURCE_IPV4>
sudo ./scripts/prepare-vocat-guest.sh --apply \
  --proxy-source-ipv4 <HOST_PROXY_SOURCE_IPV4>
sudo ./scripts/prepare-vocat-guest.sh --check \
  --proxy-source-ipv4 <HOST_PROXY_SOURCE_IPV4>
```

这里必须填写宿主反向代理连接来宾时实际使用的源地址，不是 LAN 客户端地址。
真实值只保存在私密部署记录中。

脚本会安装 guest agent、QMI 工具、SQLite、nftables 和服务账号，清除旧
Tailscale 状态，mask ModemManager，安装加固 systemd 单元，并把 TCP/7575
限制为 loopback 和精确代理源。脚本不会认证 SIM，也不会升级固件。

## 6. 部署精确产物

在来宾私密 TTY 中运行。首次部署会要求输入并确认初始管理员密码；密码不接受
命令行参数，也不得写入仓库。

```bash
sudo ./scripts/deploy-hardened.sh \
  --expected-commit <40_HEX_COMMIT> \
  --expected-index-sha256 <64_HEX_SHA256_OF_SHA256SUMS> \
  <GUEST_PATH_TO_COMPLETE_ARTIFACT_DIRECTORY>
```

部署器会先认证完整库存再解析 JSON，重新检查所有证据，在隔离环境预演 SQLite
迁移，为在线数据库生成一致性快照，切换 `/opt/vocat/current`；readiness 失败时
同时恢复旧二进制和旧数据库。

在来宾内验证：

```bash
sudo systemctl status vocat.service --no-pager
sudo systemctl is-active vocat.service vocat-firewall.service
curl --fail --silent --show-error http://127.0.0.1:7575/readyz
sudo ss -ltnp 'sport = :7575'
```

7575 只能由 VoCat 的 MainPID 独占监听。

## 7. 通过宿主反向代理发布

宿主代理是唯一面向 LAN 的入口。生产环境使用 HTTPS，只监听指定 LAN 接口并
限制客户端 CIDR；关闭或重定向明文 HTTP；保留 WebSocket upgrade；关闭流式
API 的响应缓冲；增加 HSTS，并给上游 Cookie 增加 `Secure` 标志。VoCat 保持
`trust_proxy_headers=false`，LAN 客户端归因使用反向代理自己的访问日志。

网络关系必须是：

```text
LAN 客户端 --HTTPS--> 宿主反向代理 --HTTP/libvirt NAT--> VM:7575
```

使用 Nginx 时，只提交带占位符的模板，替换后的配置仅保存在宿主。`map` 必须
位于 Nginx 的 `http` 上下文：

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

不中断现有代理地检查并重新加载：

```bash
sudo nginx -t
sudo systemctl reload nginx
```

应用代理配置后验证三条路径：

1. 通过宿主 HTTPS 代理可以访问 `/readyz` 和登录页。
2. 宿主使用已配置的代理源访问来宾 TCP/7575 成功。
3. 任何非代理来源都不能直接连接来宾 TCP/7575。

不得为来宾增加 LAN 路由、macvtap 网卡、端口转发或直接防火墙例外。

## 8. 可选 DJI USB 直通

USB 验收与应用部署是两个独立门禁。当前加固登记允许所有已连接的 `2ca3:4006`
设备；每台都必须具有唯一且稳定的 udev `ID_PATH`。部分已审查设备不提供 USB
序列号，因此序列号可以为空；设备一旦提供序列号，后续热插拔就必须同时匹配。
无序列号设备会绑定到登记时的物理 USB 端口，换到其他端口会被拒绝。不得把
白名单弱化为只匹配 VID/PID。

不输出身份信息，只列出匹配的 sysfs 名称：

```bash
for device_path in /sys/bus/usb/devices/*; do
  [ -r "$device_path/idVendor" ] && [ -r "$device_path/idProduct" ] || continue
  [ "$(<"$device_path/idVendor")" = 2ca3 ] && \
    [ "$(<"$device_path/idProduct")" = 4006 ] && basename "$device_path"
done
```

自动发现并登记当前全部匹配设备：

```bash
sudo ./scripts/configure-dji-usb-passthrough.sh --check
sudo ./scripts/configure-dji-usb-passthrough.sh --dry-run
sudo ./scripts/configure-dji-usb-passthrough.sh --apply
```

仅需登记部分设备时才重复传入 `--sysname`。脚本只把真实身份写入宿主 root-only
文件，为每个已登记的受管理 USB hostdev 分配独立 alias 和可恢复状态，并拒绝
重复路径、意外 alias 及 PCI/xHCI 直通。新增物理设备后需要再次运行 `--apply`；
已登记设备后续会自动热插拔。开始调制解调器测试前，必须对每台设备分别验证
冷启动、热插拔、拔出/重插和 VM 重启。

来宾收到精确 `2ca3:4006` USB add 事件后，会启动隔离的自动修复实例。修复器
串行处理并发事件，把接口 0-3 绑定到 `option`，执行 DTR 唤醒，把接口 4 绑定到
`qmi_wwan`，并对每台匹配设备执行只读 QMI DMS 检查；不会写入 modem NV、
固件或 SIM 状态。如果安装自动化时设备已经连接，需要精确触发一次：

```bash
sudo udevadm trigger --action=add --subsystem-match=usb \
  --attr-match=idVendor=2ca3 --attr-match=idProduct=4006
```

私有诊断可运行 `sudo /opt/vocat/current/vocat doctor --repair-dji-qmi
--timeout 60s`。自动实例不会把详细拓扑写入 journald；手工命令只能在私有控制台
使用。

固件只在维护窗读取。先停止 VoCat，再使用 `scripts/read-dji-firmware.sh`；它只
发送 `ATI`、`AT+CGMM` 和 `AT+CGMR`。首轮部署不升级固件。

## 9. 验收与回滚

生产验收必须全部满足：

- 精确部署 SHA 的候选 CI 和本地产物门禁通过；
- `vm-check-installed` 和来宾准备检查通过；
- 宿主代理访问成功，来宾直连被拒绝；
- SIM 为 Ready，且不披露 IMSI、ICCID、IMEI、PIN 或短信内容；
- 运营商配置正确，随后 ePDG/IKE 建链和 IMS 注册成功；
- 完成一次授权短信测试；仅在确有需要时执行通话测试；
- 重启、LUKS 解锁、USB 重枚举和坏版本回滚均实测通过；
- 与宿主原有服务并行浸泡 24-48 小时且没有引入变化。

SQLite 只能在停服状态或通过 SQLite online backup API 一致性备份。VM 和
SQLite 备份写入大容量存储前必须再次加密，密钥放在其他位置，并保留一份已校验
的异机副本。单块大容量存储盘不构成备份。
