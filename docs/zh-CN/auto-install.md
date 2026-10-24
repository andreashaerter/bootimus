# 无人值守安装指南

跨 Windows、Ubuntu、Debian 和 Red Hat 系列发行版运行无人值守安装。配置文件丢进去一次,挂到镜像上,之后每次 PXE 引导都能在不按一次键的情况下完成安装。

## 目录

- [概述](#概述)
- [支持的格式](#支持的格式)
- [文件库](#文件库)
- [挂载文件](#挂载文件)
- [解析顺序](#解析顺序)
- [占位符](#占位符)
- [示例](#示例)
- [报告安装完成](#报告安装完成)
- [Windows 注意事项](#windows-注意事项)
- [REST API](#rest-api)
- [故障排查](#故障排查)

## 概述

无人值守安装配置存储在 `data/autoinstall/` 下按发行版分类的文件库中。你可以:

- **在 UI 里管理文件** — 在 **Auto-Install** 标签页中创建、编辑、上传、下载、删除配置,无需触碰文件系统。
- **为镜像设置默认值** — 引导该镜像的每个客户端都得到同一份配置。
- **按客户端覆盖** 或 **按客户端组覆盖** — 当某台机器需要不同的配置时(不同的 hostname、磁盘布局、角色等)。
- **模板化** — 使用类似 `{{HOSTNAME}}` 和 `{{IP}}` 的占位符,在分发时根据正在引导的客户端身份解析。

Bootimus 通过 HTTP 在无人值守安装端点上提供渲染后的文件。Linux 安装程序通过内核参数获取它;对 Windows 而言,wimboot 会把同一个 URL 作为额外的 `initrd` 下载并以 `AutoUnattend.xml` 的名字注入 WinPE 的 RAM 盘,这样 `setup.exe /unattend:` 就能自动取用(见 [Windows 注意事项](#windows-注意事项))。

## 支持的格式

| 发行版家族 | 格式 | 文件扩展名 | 识别为 |
|--------------|--------|----------------|-------------|
| Windows (10/11/Server) | `autounattend.xml` | `.xml` | `autounattend` |
| Ubuntu (Server live, 20.04+) | cloud-init / autoinstall | `.yaml`, `.yml` | `autoinstall` |
| Debian | preseed | `.cfg` | `preseed` |
| Red Hat / Rocky / Fedora / Alma | kickstart | `.ks` | `kickstart` |
| 其他任何 | 原始格式 | 任意 | `generic` |

扩展名同时决定了 UI 内的标签和文件被服务时的 `Content-Type` 头。

## 文件库

所有无人值守安装文件都位于 `data/autoinstall/<distro>/<filename>` 下:

```
data/autoinstall/
├── windows/
│   ├── kiosk.xml
│   └── server-2022.xml
├── ubuntu/
│   ├── default.yaml
│   └── lab-bench.yaml
├── debian/
│   └── server.cfg
└── rocky/
    └── workstation.ks
```

`<distro>` 段必须匹配一个已知的发行版 profile ID(详见 [发行版 Profile](distro-profiles.md))。该目录在首次启动时自动创建。

### 通过 UI 添加文件

**Auto-Install** 标签 → **New File** 打开编辑器,内含发行版选择器、文件名输入框,以及对语法友好的文本区。**Upload File** 接受任意本地文件并放入所选的发行版文件夹。

### 手动添加文件

直接丢进去就行:

```bash
mkdir -p data/autoinstall/ubuntu
cp my-autoinstall.yaml data/autoinstall/ubuntu/default.yaml
```

它们会立即出现在 UI 中 — 无需重启,无需扫描。

## 挂载文件

无人值守安装文件在你将其挂到某处之前不会产生任何效果。有三处可以挂载:

### 镜像(默认值)

**Images** 标签 → 打开镜像的 **Properties** → **Auto-Install** 部分 → 选择一个文件。引导该镜像的每个客户端都会得到这份配置,除非有更具体的覆盖。

### 客户端(按机器覆盖)

**Clients** 标签 → 打开一个客户端 → **Auto-Install File** 下拉框。当某台特定机器需要不同配置时使用(例如,一台构建服务器 vs. 桌面机群的其余机器)。

### 客户端组(按机群覆盖)

**Groups** 标签 → 打开一个组 → **Auto-Install File**。适用于该组内的每个客户端。适合"实验室 3 的所有工作站"这类场景。

## 解析顺序

当客户端请求其无人值守安装文件时,Bootimus 会沿以下层级查找:

```
1. 按客户端覆盖              (Client.AutoInstallFile)
2. 按组覆盖                  (ClientGroup.AutoInstallFile, if client is in a group)
3. 镜像默认值                (Image.AutoInstallFile)
4. 内联旧版脚本              (Image.AutoInstallScript — pre-0.1.58 setups)
5. → 404 (no auto-install configured)
```

第一个非空匹配胜出。端点会记录它服务来源的日志:

```
Served auto-install script for ubuntu-24.04-live-server-amd64.iso \
  (source: client:b4:2e:99:01:5f:a3, type: autoinstall, size: 1247 bytes)
```

## 占位符

这些占位符会在分发时按客户端替换:

| 占位符 | 替换为 |
|-------|---------------|
| `{{MAC}}` | 客户端 MAC 地址(小写,冒号分隔) |
| `{{CLIENT_NAME}}` | Clients 表中的友好名称 |
| `{{HOSTNAME}}` | 同 `{{CLIENT_NAME}}`(在配置中更清晰的别名) |
| `{{IP}}` | 发起请求的客户端 IP |
| `{{SERVER_ADDR}}` | Bootimus 服务器地址 |
| `{{IMAGE_NAME}}` | 正在引导的镜像的显示名 |
| `{{IMAGE_FILENAME}}` | 正在引导的镜像的 ISO 文件名 |
| `{{CALLBACK_URL}}` | 安装程序用来报告完成的完整 URL,包含一次性令牌(见[报告安装完成](#报告安装完成)) |
| `{{INSTALL_TOKEN}}` | `{{CALLBACK_URL}}` 背后的裸令牌,用于构造你自己的请求 |

占位符是纯字符串替换 — 不做转义。请按目标格式(XML、YAML 等)适当地引用它们。

替换发生在无人值守安装端点上,因此对所有格式都生效。Windows 的 `autounattend.xml` 同样会按正在引导的客户端渲染,因为 wimboot 在引导时正是从该端点获取它(见 [Windows 注意事项](#windows-注意事项));只有那里描述的未渲染 SMB 回退副本会绕过替换。

## 示例

### Ubuntu Server (cloud-init)

`data/autoinstall/ubuntu/default.yaml`:

```yaml
#cloud-config
autoinstall:
  version: 1
  identity:
    hostname: {{HOSTNAME}}
    username: ubuntu
    password: "$6$rounds=4096$..."  # mkpasswd -m sha-512
  ssh:
    install-server: true
    allow-pw: false
    authorized-keys:
      - ssh-ed25519 AAAA...
  storage:
    layout:
      name: lvm
  late-commands:
    - curtin in-target -- systemctl enable --now serial-getty@ttyS0.service
```

只要附加了无人值守安装文件,Bootimus 就会自动添加该启动参数,无需在镜像的启动参数中手动添加。生成的参数如下:

```
autoinstall ds=nocloud-net;s=http://<server>:8080/autoinstall/<image-filename>/mac/<mac>/
```

### Debian (preseed)

`data/autoinstall/debian/server.cfg`:

```
d-i debian-installer/locale string en_GB.UTF-8
d-i keyboard-configuration/xkb-keymap select gb
d-i netcfg/get_hostname string {{HOSTNAME}}
d-i netcfg/get_domain string lan
d-i partman-auto/method string lvm
d-i partman-auto/choose_recipe select atomic
d-i passwd/root-login boolean false
d-i passwd/user-fullname string Admin
d-i passwd/username string admin
d-i passwd/user-password-crypted password $6$rounds=4096$...
d-i pkgsel/include string openssh-server
d-i grub-installer/bootdev string default
d-i finish-install/reboot_in_progress note
```

### Rocky / Fedora (kickstart)

`data/autoinstall/rocky/workstation.ks`:

```
text
lang en_GB.UTF-8
keyboard gb
timezone Europe/London --utc
network --bootproto=dhcp --hostname={{HOSTNAME}}
rootpw --lock
user --name=admin --groups=wheel --password=$6$rounds=4096$... --iscrypted
sshkey --username=admin "ssh-ed25519 AAAA..."
bootloader --location=mbr
clearpart --all --initlabel
autopart --type=lvm
%packages
@^minimal-environment
openssh-server
%end
```

### Windows 11 / Server (autounattend)

`data/autoinstall/windows/kiosk.xml`:标准 `<unattend>` 文档 — 参见 [微软的 autounattend 参考](https://learn.microsoft.com/en-us/windows-hardware/customize/desktop/unattend/)。当客户端通过 wimboot 引导时(Bootimus 正是这样引导 Windows 镜像的),占位符在任何文本节点内都生效:

```xml
<ComputerName>{{HOSTNAME}}</ComputerName>
```

## 报告安装完成

Bootimus 从客户端加载镜像内核的那一刻起就跟踪每一次无人值守安装。记录会经历四种状态:

| 状态 | 设置时机 |
|------|----------|
| `booted` | 客户端已获取镜像的引导文件,且该镜像带有自动安装文件或内联脚本 |
| `installing` | 安装程序已获取自动安装文件 |
| `completed` | 已安装的系统调用了 `{{CALLBACK_URL}}` |
| `failed` | 回调报告了 `status=failed`,或在超时内没有任何回报 |

当前状态显示在 Clients 表的 **Install** 列,完整历史位于客户端编辑器的 **Install History** 下。从未回报的安装会在六小时后标记为失败;可用 `--install-timeout-hours` 或 `bootimus.yaml` 中的 `install_timeout_hours` 更改。只要在 **Settings → Webhook** 下启用了相应事件,每次转为 `completed` 或 `failed` 都会触发 `install.completed` 或 `install.failed` webhook 事件。

回调是一个发往引导端口的普通 HTTP 请求,`GET` 或 `POST` 均可:

```
http://<server>:8080/install/callback?token=<token>&status=completed|failed&message=<optional text>
```

`{{CALLBACK_URL}}` 已经包含了令牌,所以脚本只需追加 `&status=failed&message=...`。每个令牌只有第一次报告算数;之后的调用会返回 `already recorded`。

### Kickstart(`%post`)

```
%post --log=/root/ks-post.log
curl -fsS "{{CALLBACK_URL}}" || curl -fsS "{{CALLBACK_URL}}&status=failed&message=post-script"
%end
```

### Preseed(`late_command`)

```
d-i preseed/late_command string in-target sh -c 'wget -qO- "{{CALLBACK_URL}}" || curl -fsS "{{CALLBACK_URL}}"'
```

### Ubuntu autoinstall / cloud-init

`phone_home` 模块会在首次启动后发送请求,这是最可靠的信号:

```yaml
#cloud-config
autoinstall:
  version: 1
  user-data:
    phone_home:
      url: "{{CALLBACK_URL}}"
      post: [instance_id, hostname]
      tries: 10
```

或者用 `late-commands` 从安装程序环境中报告:

```yaml
  late-commands:
    - curtin in-target --target=/target -- curl -fsS "{{CALLBACK_URL}}"
```

### Windows(`autounattend.xml`)

在 `Microsoft-Windows-Shell-Setup` 下添加一条首次登录命令:

```xml
<FirstLogonCommands>
  <SynchronousCommand wcm:action="add">
    <Order>1</Order>
    <CommandLine>powershell -NoProfile -Command "Invoke-WebRequest -UseBasicParsing '{{CALLBACK_URL}}'"</CommandLine>
    <Description>Report install to Bootimus</Description>
  </SynchronousCommand>
</FirstLogonCommands>
```

请记住,如果追加 `status` 或 `message` 参数,XML 属性或文本中的 `&` 必须写成 `&amp;`。

### Talos 和 Flatcar 节点

通过 **Kubernetes** 标签页注册的 Kubernetes 节点已经通过机器配置流程以及 `node.install_started` / `node.install_completed` 事件进行报告。它们不需要回调。

### 查询历史

```bash
# History for one client
curl -H "Authorization: Bearer $TOKEN" "http://localhost:8081/api/clients/installs?mac=b4:2e:99:01:5f:a3"

# Recent installs across the fleet
curl -H "Authorization: Bearer $TOKEN" "http://localhost:8081/api/installs?limit=50"
```

## Windows 注意事项

Windows 镜像通过 [wimboot](https://ipxe.org/wimboot) 引导,它会把 iPXE 脚本中每个额外的 `initrd` 文件注入 WinPE 的 RAM 盘。Bootimus 借此按客户端分发无人值守安装文件:

1. 当正在引导的客户端解析到某个 autounattend 文件时(按常规的[解析顺序](#解析顺序):客户端、然后客户端组、然后镜像),生成的 iPXE 引导段会在 `boot.wim` 之前加入 `initrd http://<server>:<port>/autoinstall/<iso>?mac=<mac> AutoUnattend.xml`。
2. 无人值守安装端点用该客户端的占位符渲染文件,与 Linux 安装程序完全一致。
3. wimboot 把文件放在 WinPE 内的 `X:\Windows\System32\AutoUnattend.xml`。
4. 打过补丁的 `startnet.cmd` 在那里找到它,并以 `setup.exe /unattend:X:\Windows\System32\AutoUnattend.xml` 启动 Setup。

由于文件是按客户端解析和渲染的,按客户端和按客户端组的覆盖以及 `{{HOSTNAME}}` 之类的占位符在 Windows 上和其他地方一样有效。引导参数仍会传给 wimboot;请避免 `rawwim`,它会关闭文件注入。

**SMB 回退。** 当为一个在镜像级别挂载了 autounattend 文件的镜像打补丁 `boot.wim` 时,Bootimus 还会把该文件以未渲染的形式作为 `AutoUnattend.xml` 暂存到该镜像的 SMB 共享上。只有当 wimboot 没有注入任何文件时,例如客户端通过 Bootimus iPXE 菜单之外的其他途径引导了打过补丁的 `boot.wim`,`startnet.cmd` 才会把它复制到 `X:\AutoUnattend.xml`。该回退没有占位符替换,也没有按客户端或按客户端组的覆盖:其中的占位符会原样传给 Setup。

如果任何级别都没有挂载 autounattend 文件,Setup 会和以前一样以交互模式运行。

**数据目录要求。** Bootimus 运行自己的 `smbd`,其状态保存在 `data/smb` 下。如果该目录位于无法保存 Unix 权限的文件系统上(exFAT、NTFS 或 CIFS 挂载),Samba 会拒绝启动,因为其锁目录会被读成 `0777` 而不是 `0755`。请把数据目录放在 ext4 或 XFS 等原生 Linux 文件系统上,并确保主机上没有其他 SMB 服务监听 445 端口。Bootimus 会在启动时检查这两个条件,并在 **Server Info** → **Windows SMB** 中报告。

**重启韧性。** WinPE 在安装中途会重启并从同一个客户端 IP 重新连接。内置的 Samba 配置设置了 `reset on zero vc = yes` 并禁用了 oplocks,这样第二次 `net use` 不会被陈旧的会话状态卡住。如果你用自己的 `data/smb/smb.conf` 替换了默认配置,请同步这些设置。

**撤销补丁。** 第一次打 SMB 补丁时,会在旁边保留一份原始的 `boot.wim`,命名为 `boot.wim.orig`。**Images** → **移除 SMB 补丁** 会恢复原始文件并删除该镜像的 SMB 共享——之后 ISO 的引导行为与刚上传时完全一致。由旧版 Bootimus 打补丁的镜像没有备份:请先重新提取镜像(会生成一份新的原始副本),再移除补丁。

## REST API

UI 里的每件事也都是一次 REST 调用。

```bash
# List all auto-install files
curl -u admin:pw http://localhost:8081/api/autoinstall-files

# Read a file
curl -u admin:pw "http://localhost:8081/api/autoinstall-files/get?distro=ubuntu&filename=default.yaml"

# Create or overwrite a file
curl -u admin:pw -X POST http://localhost:8081/api/autoinstall-files/save \
  -H "Content-Type: application/json" \
  -d '{"distro":"ubuntu","filename":"default.yaml","content":"#cloud-config\n..."}'

# Upload a file
curl -u admin:pw -X POST http://localhost:8081/api/autoinstall-files/upload \
  -F "distro=windows" \
  -F "filename=kiosk.xml" \
  -F "file=@./kiosk.xml"

# Download
curl -u admin:pw "http://localhost:8081/api/autoinstall-files/download?distro=ubuntu&filename=default.yaml" -o default.yaml

# Delete
curl -u admin:pw -X POST "http://localhost:8081/api/autoinstall-files/delete?distro=ubuntu&filename=default.yaml"
```

将文件挂载到镜像:

```bash
curl -u admin:pw -X PUT http://localhost:8081/api/images/update \
  -H "Content-Type: application/json" \
  -d '{"filename":"ubuntu-24.04-live-server-amd64.iso","auto_install_file":"ubuntu/default.yaml"}'
```

将文件挂载到客户端:

```bash
curl -u admin:pw -X PUT http://localhost:8081/api/clients/b4:2e:99:01:5f:a3 \
  -H "Content-Type: application/json" \
  -d '{"auto_install_file":"ubuntu/lab-bench.yaml"}'
```

客户端在启动时访问的无人值守安装端点:

```
GET /autoinstall/<image-filename>?mac=<mac>             # preseed, kickstart
GET /autoinstall/<image-filename>/mac/<mac>/user-data   # Ubuntu autoinstall
GET /autoinstall/<image-filename>/mac/<mac>/meta-data   # Ubuntu autoinstall
```

引导菜单会用客户端的 MAC 地址构造这些 URL,以便按客户端覆盖能正确解析;对于 Ubuntu,cloud-init 会自行追加 `user-data` 和 `meta-data`。不带 MAC 地址或其他形式的请求(例如 `/autoinstall/<image-filename>/user-data`)会返回 404。

## 故障排查

### `/autoinstall/...` 返回 404

`no auto-install configuration for this image/client` — 解析链的任何层级都没有挂载文件。要么挂载到镜像、客户端或其所属组,要么检查 `auto_install_file` 是否真的指向 `data/autoinstall/` 下存在的某个文件。

### 占位符按字面输出

`{{HOSTNAME}}` 在已安装系统中显示为字面字符串,意味着文件在替换运行之前就被分发了 — 通常是因为客户端仅通过 IP 启动而请求未携带 `mac` 查询参数。确认引导菜单生成的 URL 形如 `/autoinstall/<iso>?mac=<mac>` 或 `/autoinstall/<iso>/mac/<mac>/`。

在 Windows 上,占位符按字面输出意味着 Setup 使用了 SMB 共享上暂存的未渲染副本,而不是 wimboot 注入的文件。预览该客户端的引导菜单(见[预览客户端的引导菜单](clients.md#previewing-a-clients-boot-menu)),检查 Windows 段中是否包含 `initrd .../autoinstall/<iso>?mac=<mac> AutoUnattend.xml` 这一行,并确认镜像的引导参数中没有 `rawwim`。

### 分发了错误的文件

解析是"最具体优先"。如果一个客户端有自己的覆盖而你并没意识到,这就是为什么镜像级默认没有被使用。检查服务器日志这一行:

```
Served auto-install script for ... (source: client:..., type: ..., size: ...)
```

`source:` 字段告诉你究竟是哪个槽位胜出。

### Windows Setup 交互式运行

- 正在引导的客户端必须能解析到一个 autounattend 文件:把它挂载到镜像(镜像属性 → Auto-Install)、客户端或其所属客户端组。
- 预览该客户端的引导菜单,确认 Windows 段包含 `initrd .../autoinstall/<iso>?mac=<mac> AutoUnattend.xml` 这一行。若没有,说明该 MAC 没有解析到任何 autounattend 文件。
- 确保镜像的引导参数中没有 `rawwim`;它会阻止 wimboot 向 WinPE 注入文件。
- 升级 Bootimus 后重新打补丁 `boot.wim`,让 `startnet.cmd` 知道去查找注入的文件:**Images** → **Patch SMB**(或下次启动时会自动重新打补丁)。
- 确认 SMB 共享在客户端可达(在 WinPE 中执行 `net view \\<server>`);安装程序本身仍来自共享。

### "AutoUnattend.xml neither injected by wimboot nor staged on the share, running interactive setup"

由 `startnet.cmd` 在镜像挂载了 autounattend 文件但两条分发路径都没有产生文件时记录。检查 Bootimus 日志中客户端引导时的 `/autoinstall/<iso>?mac=...` 请求(404 表示该客户端没有解析到文件),以及打补丁时段附近的暂存步骤。从镜像属性重新执行 SMB 补丁。

### `Z:\setup.exe not found on the share`

`startnet.cmd` 已映射 `Z:`,但它打印的目录列表显示的是镜像文件夹(`iso\`、`metadata.txt`)而不是安装文件。Bootimus 始终共享镜像的 `iso` 子文件夹,所以客户端连接到了同一地址上的另一个 SMB 服务。检查 Bootimus 日志中是否有 `SMB: smbd exited`,并查看 **Server Info** 下的 **Windows SMB** 一行。常见原因是数据目录位于没有 Unix 权限的文件系统上,这会让 `smbd` 以 `invalid permissions on directory '.../smb/locks/msg.lock': has 0777 should be 0755` 退出,转而由系统的 Samba 在 445 端口应答。把数据目录移到 ext4 或 XFS,停止另一个 Samba,然后重启 Bootimus。

### `net use fails after VM reboot`

在 0.1.58 通过在内置 Samba 配置中启用 `reset on zero vc = yes` 已修复。如果你维护着自定义 `smb.conf`,请加入:

```
reset on zero vc = yes
oplocks = no
kernel oplocks = no
level2 oplocks = no
strict locking = no
deadtime = 1
```

## 下一步

- 参见 [镜像管理](images.md) 了解如何将文件挂载到镜像。
- 参见 [客户端管理](clients.md) 了解按客户端覆盖。
- 参见 [发行版 Profile](distro-profiles.md) 了解映射到文件库子目录的底层 profile ID。
