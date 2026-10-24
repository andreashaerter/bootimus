# Auto-Install Guide

Run unattended installs across Windows, Ubuntu, Debian, and Red Hat-family distros. Drop a config file in once, attach it to an image, and every PXE boot finishes installation without a single keystroke.

## Table of Contents

- [Overview](#overview)
- [Supported Formats](#supported-formats)
- [The File Library](#the-file-library)
- [Attaching Files](#attaching-files)
- [Resolution Order](#resolution-order)
- [Placeholders](#placeholders)
- [Variables](#variables)
- [Preview](#preview)
- [Validation](#validation)
- [Examples](#examples)
- [Reporting Install Completion](#reporting-install-completion)
- [Windows Notes](#windows-notes)
- [REST API](#rest-api)
- [Troubleshooting](#troubleshooting)

## Overview

Auto-install configs are stored in a per-distro library under `data/autoinstall/`. You can:

- **Manage files in the UI** under the **Auto-Install** tab — create, edit, upload, download, and delete configs without touching the filesystem.
- **Set a default** per image (every client booting that image gets the same config).
- **Override per client** or **per client group** when one machine needs a different config (different hostname, disk layout, role, etc.).
- **Templatise** with placeholders like `{{HOSTNAME}}` and `{{IP}}` resolved at serve time using the booting client's identity, plus your own `{{VAR.name}}` variables set per client or per group.
- **Preview** the rendered result for any client and image, and have YAML, XML and JSON files checked for well-formedness when you save them.

Bootimus serves the rendered file over HTTP on the auto-install endpoint. Linux installers fetch it through a kernel parameter; for Windows, wimboot fetches the same URL as an extra `initrd` and injects it into the WinPE RAM disk as `AutoUnattend.xml`, so `setup.exe /unattend:` picks it up automatically (see [Windows Notes](#windows-notes)).

## Supported Formats

| Distro family | Format | File extension | Detected as |
|--------------|--------|----------------|-------------|
| Windows (10/11/Server) | `autounattend.xml` | `.xml` | `autounattend` |
| Ubuntu (Server live, 20.04+) | cloud-init / autoinstall | `.yaml`, `.yml` | `autoinstall` |
| Debian | preseed | `.cfg` | `preseed` |
| Red Hat / Rocky / Fedora / Alma | kickstart | `.ks` | `kickstart` |
| Anything else | raw | any | `generic` |

The extension drives both the in-UI label and the `Content-Type` header when the file is served.

## The File Library

All auto-install files live under `data/autoinstall/<distro>/<filename>`:

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

The `<distro>` segment must match a known distro profile ID (see [Distro Profiles](distro-profiles.md)). The directory is created automatically on first start.

### Add files via the UI

**Auto-Install** tab → **New File** opens the editor with a distro picker, filename field, and a syntax-friendly textarea. **Upload File** takes any local file and drops it into the chosen distro folder.

### Add files manually

Just drop them in:

```bash
mkdir -p data/autoinstall/ubuntu
cp my-autoinstall.yaml data/autoinstall/ubuntu/default.yaml
```

They appear in the UI immediately — no restart, no scan.

## Attaching Files

Auto-install files have no effect until you wire them up. There are three places to attach one:

### Image (default)

**Images** tab → open an image's **Properties** → **Auto-Install** section → pick a file. Every client that boots this image gets this config unless something more specific overrides it.

### Client (per-machine override)

**Clients** tab → open a client → **Auto-Install File** dropdown. Use this when one specific machine needs a different config (e.g., a build server vs the rest of the desk fleet).

### Client Group (per-fleet override)

**Groups** tab → open a group → **Auto-Install File**. Applies to every client in the group. Useful for "all the workstations in lab 3" scenarios.

## Resolution Order

When a client requests its auto-install file, Bootimus walks this hierarchy:

```
1. Per-client override        (Client.AutoInstallFile)
2. Per-group override         (ClientGroup.AutoInstallFile, if client is in a group)
3. Image default              (Image.AutoInstallFile)
4. Inline legacy script       (Image.AutoInstallScript — pre-0.1.58 setups)
5. → 404 (no auto-install configured)
```

The first non-empty match wins. The endpoint logs the source it served from:

```
Served auto-install script for ubuntu-24.04-live-server-amd64.iso \
  (source: client:b4:2e:99:01:5f:a3, type: autoinstall, size: 1247 bytes)
```

## Placeholders

These tokens are substituted per-client at serve time:

| Token | Replaced with |
|-------|---------------|
| `{{MAC}}` | Client MAC address (lowercase, colon-separated) |
| `{{CLIENT_NAME}}` | Friendly name from the Clients table |
| `{{HOSTNAME}}` | Same as `{{CLIENT_NAME}}` (alias for clarity in configs) |
| `{{CLIENT_GROUP}}` | Name of the client's group, or empty when it has none |
| `{{IP}}` | Client IP that issued the request |
| `{{SERVER_ADDR}}` | Bootimus server address |
| `{{SERVER_PORT}}` | Bootimus HTTP boot port |
| `{{SERVER_URL}}` | `http://<SERVER_ADDR>:<SERVER_PORT>`, the same base URL the boot menu uses |
| `{{IMAGE_NAME}}` | Display name of the booting image |
| `{{IMAGE_FILENAME}}` | ISO filename of the booting image |
| `{{IMAGE_URL}}` | URL of the ISO as served under `/isos/`, the same value the boot menu passes as `iso-url=` |
| `{{CALLBACK_URL}}` | Full URL the installer calls to report completion, including a one-time token (see [Reporting Install Completion](#reporting-install-completion)) |
| `{{INSTALL_TOKEN}}` | The bare token behind `{{CALLBACK_URL}}`, for building your own request |
| `{{VAR.<name>}}` | The client or group variable called `<name>` (see [Variables](#variables)) |

Placeholders are plain string substitution — no escaping. Quote them appropriately for the target format (XML, YAML, etc.). A placeholder with no value is left exactly as written and logged as unresolved when the file is served; use [Preview](#preview) to spot these before a machine boots.

## Variables

Built-in placeholders cover the client's identity, but most fleets also need values Bootimus cannot know: a role, a rack number, a disk device, a mirror URL. Variables fill that gap. Each client and each client group carries a set of named string values, and every one of them is available in auto-install files as `{{VAR.<name>}}`.

- Names use letters, digits and underscores only (`role`, `rack_id`, `DISK`); the server rejects anything else when the client or group is saved.
- **Group variables are defaults** for every member. **Client variables override** them one name at a time, so a group can set `role: worker` and a single machine can set `role: control-plane` while still inheriting the group's `rack`.
- A `{{VAR.<name>}}` token with no value at either level is left unresolved.

Set them in the UI under **Clients** → open a client → **Auto-install variables**, or **Groups** → open a group → **Auto-install variables**: each row is a name and a value, with **Add variable** and a remove button per row. Over the API, `variables` is an object of strings on both the client and the group payloads:

```bash
# Group defaults
curl -u admin:pw -X PUT "http://localhost:8081/api/client-groups/update?id=3" \
  -H "Content-Type: application/json" \
  -d '{"name":"lab-3","variables":{"role":"worker","rack":"3","disk":"/dev/sda"}}'

# One machine overrides a single value
curl -u admin:pw -X PUT "http://localhost:8081/api/clients?mac=b4:2e:99:01:5f:a3" \
  -H "Content-Type: application/json" \
  -d '{"variables":{"role":"control-plane"}}'
```

Sending `"variables": null` on a client update clears them; leaving the key out leaves them unchanged. `GET /api/clients` and `GET /api/client-groups` return them under the same key.

A kickstart that uses both levels:

```
network --bootproto=dhcp --hostname={{HOSTNAME}}
ignoredisk --only-use={{VAR.disk}}
%post
echo "{{VAR.role}} in rack {{VAR.rack}} ({{CLIENT_GROUP}})" > /etc/machine-role
%end
```

The CSV import under **Clients** does not carry variables; existing variables are kept when a CSV row updates a client.

## Preview

The editor under **Auto-Install** has a **Preview** action for any saved file. Pick a client and, optionally, an image, and the panel shows the exact text Bootimus would serve to that machine, read-only, with a warning listing every placeholder that stayed unresolved. Without a client, the client and group values render empty; without an image, the `IMAGE_*` placeholders stay unresolved. `{{CALLBACK_URL}}` and `{{INSTALL_TOKEN}}` render with the fixed token `preview`, because no install record is created.

The same rendering is available over the API:

```bash
curl -u admin:pw "http://localhost:8081/api/autoinstall-files/render?distro=ubuntu&filename=default.yaml&mac=b4:2e:99:01:5f:a3&image=ubuntu-24.04-live-server-amd64.iso"
```

```json
{"success":true,"data":{"content":"#cloud-config\n...","unresolved":["VAR.disk"]}}
```

`mac` and `image` are optional. Rendering never writes anything: it does not create an install record or move an install through its states.

## Validation

When a file is saved from the editor or uploaded, Bootimus checks it for well-formedness by extension before writing it:

| Extension | Check |
|-----------|-------|
| `.yaml`, `.yml` | Parses as YAML (multi-document files are allowed) |
| `.xml` | Well-formed XML |
| `.json` | Valid JSON |
| anything else | Not checked |

Every placeholder is replaced with a harmless dummy value first, so `hostname: {{HOSTNAME}}` passes even though a bare `{{HOSTNAME}}` is not valid YAML on its own. A failure answers `400` with the parser's message and, where the parser provides it, the line number; the editor shows it inline under the content and offers **Save anyway** for the cases where you know better than the parser (for instance a file that is deliberately a fragment). Over the API, pass `"force": true` in the save body or `force=true` as a multipart field on upload to skip the check.

Substitution happens on the auto-install endpoint, so it applies to every format. Windows `autounattend.xml` is rendered for the booting client too, because wimboot fetches it from that endpoint at boot (see [Windows Notes](#windows-notes)); only the unrendered SMB fallback copy described there bypasses it.

## Examples

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

Bootimus adds the boot parameter itself whenever an auto-install file is attached, so there is nothing to add to the image's boot params. The generated parameter looks like this:

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

`data/autoinstall/windows/kiosk.xml`: standard `<unattend>` document — see [Microsoft's autounattend reference](https://learn.microsoft.com/en-us/windows-hardware/customize/desktop/unattend/). Placeholders work inside any text node when the client boots through wimboot, which is how Bootimus boots Windows images:

```xml
<ComputerName>{{HOSTNAME}}</ComputerName>
```

## Reporting Install Completion

Bootimus tracks every unattended install from the moment the client loads the image's kernel. The record moves through four states:

| State | Set when |
|-------|----------|
| `booted` | The client fetched the image's boot files and the image has an auto-install file or inline script |
| `installing` | The installer fetched the auto-install file |
| `completed` | The installed system called `{{CALLBACK_URL}}` |
| `failed` | The callback reported `status=failed`, or nothing reported back within the timeout |

The current state shows in the **Install** column of the Clients table and the full history sits under **Install History** in the client editor. Installs that never report are marked failed after six hours; change that with `--install-timeout-hours` or `install_timeout_hours` in `bootimus.yaml`. Each transition to `completed` or `failed` fires an `install.completed` or `install.failed` webhook event when that event is enabled under **Settings → Webhook**.

The callback is a plain HTTP request, `GET` or `POST`, to the boot port:

```
http://<server>:8080/install/callback?token=<token>&status=completed|failed&message=<optional text>
```

`{{CALLBACK_URL}}` already contains the token, so appending `&status=failed&message=...` is all a script needs. Only the first report for a token counts; later calls answer `already recorded`.

### Kickstart (`%post`)

```
%post --log=/root/ks-post.log
curl -fsS "{{CALLBACK_URL}}" || curl -fsS "{{CALLBACK_URL}}&status=failed&message=post-script"
%end
```

### Preseed (`late_command`)

```
d-i preseed/late_command string in-target sh -c 'wget -qO- "{{CALLBACK_URL}}" || curl -fsS "{{CALLBACK_URL}}"'
```

### Ubuntu autoinstall / cloud-init

The `phone_home` module posts after first boot, which is the most reliable signal:

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

Or report from the installer environment with `late-commands`:

```yaml
  late-commands:
    - curtin in-target --target=/target -- curl -fsS "{{CALLBACK_URL}}"
```

### Windows (`autounattend.xml`)

Add a first-logon command under `Microsoft-Windows-Shell-Setup`:

```xml
<FirstLogonCommands>
  <SynchronousCommand wcm:action="add">
    <Order>1</Order>
    <CommandLine>powershell -NoProfile -Command "Invoke-WebRequest -UseBasicParsing '{{CALLBACK_URL}}'"</CommandLine>
    <Description>Report install to Bootimus</Description>
  </SynchronousCommand>
</FirstLogonCommands>
```

Remember that `&` inside XML attributes or text must be written as `&amp;` if you append `status` or `message` parameters.

### Talos and Flatcar nodes

Kubernetes nodes enrolled through the **Kubernetes** tab already report through the machine config flow and the `node.install_started` / `node.install_completed` events. They do not need a callback.

### Querying the history

```bash
# History for one client
curl -H "Authorization: Bearer $TOKEN" "http://localhost:8081/api/clients/installs?mac=b4:2e:99:01:5f:a3"

# Recent installs across the fleet
curl -H "Authorization: Bearer $TOKEN" "http://localhost:8081/api/installs?limit=50"
```

## Windows Notes

Windows images boot through [wimboot](https://ipxe.org/wimboot), which injects every extra `initrd` file in the iPXE script into the WinPE RAM disk. Bootimus uses that to deliver the auto-install file per client:

1. When the booting client resolves to an autounattend file (through the usual [resolution order](#resolution-order): client, then group, then image), the generated iPXE boot section adds `initrd http://<server>:<port>/autoinstall/<iso>?mac=<mac> AutoUnattend.xml` before `boot.wim`.
2. The auto-install endpoint renders the file with the client's placeholders, exactly as it does for Linux installers.
3. wimboot places the file at `X:\Windows\System32\AutoUnattend.xml` inside WinPE.
4. The patched `startnet.cmd` finds it there and launches `setup.exe /unattend:X:\Windows\System32\AutoUnattend.xml`.

Because the file is resolved and rendered per client, per-client and per-group overrides and placeholders such as `{{HOSTNAME}}` work for Windows just as they do elsewhere. Boot parameters are still passed to wimboot; avoid `rawwim`, which turns file injection off.

**SMB fallback.** When `boot.wim` is patched for an image that has an autounattend file attached at image level, Bootimus also stages that file, unrendered, as `AutoUnattend.xml` on the image's SMB share. `startnet.cmd` only copies it to `X:\AutoUnattend.xml` when wimboot injected nothing, for example when the client booted the patched `boot.wim` by some route other than the Bootimus iPXE menu. The fallback carries no placeholder substitution and no per-client or per-group overrides: placeholders in it reach Setup literally.

Without an autounattend file at any level, Setup runs interactively as before.

**Data directory requirements.** Bootimus runs its own `smbd` with its state under `data/smb`. Samba refuses to start if that directory sits on a filesystem that cannot store Unix permissions (exFAT, NTFS, or a CIFS mount), because its lock directory then reads back as `0777` instead of `0755`. Keep the data directory on a native Linux filesystem such as ext4 or XFS, and make sure no other SMB server on the host is listening on port 445. Bootimus checks for both conditions at startup and reports them under **Server Info** → **Windows SMB**.

**Reboot resilience.** WinPE reboots mid-install and reconnects from the same client IP. The bundled Samba config sets `reset on zero vc = yes` and disables oplocks so the second `net use` doesn't trip on stale session state. If you've replaced `data/smb/smb.conf` with your own, mirror these settings.

**Undoing the patch.** The first SMB patch keeps a pristine copy of `boot.wim` as `boot.wim.orig` next to it. **Images** → **Unpatch SMB** restores that original and removes the image's SMB share, so the ISO boots exactly as uploaded. Images patched by older Bootimus versions have no backup — re-extract the image first (which produces a fresh pristine copy), then unpatch.

## REST API

Everything in the UI is also a REST call.

```bash
# List all auto-install files
curl -u admin:pw http://localhost:8081/api/autoinstall-files

# Read a file
curl -u admin:pw "http://localhost:8081/api/autoinstall-files/get?distro=ubuntu&filename=default.yaml"

# Create or overwrite a file (answers 400 when YAML, XML or JSON is malformed; add "force":true to save anyway)
curl -u admin:pw -X POST http://localhost:8081/api/autoinstall-files/save \
  -H "Content-Type: application/json" \
  -d '{"distro":"ubuntu","filename":"default.yaml","content":"#cloud-config\n..."}'

# Render a file for a client and image, listing unresolved placeholders
curl -u admin:pw "http://localhost:8081/api/autoinstall-files/render?distro=ubuntu&filename=default.yaml&mac=b4:2e:99:01:5f:a3&image=ubuntu-24.04-live-server-amd64.iso"

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

Attach a file to an image:

```bash
curl -u admin:pw -X PUT http://localhost:8081/api/images/update \
  -H "Content-Type: application/json" \
  -d '{"filename":"ubuntu-24.04-live-server-amd64.iso","auto_install_file":"ubuntu/default.yaml"}'
```

Attach a file to a client:

```bash
curl -u admin:pw -X PUT http://localhost:8081/api/clients/b4:2e:99:01:5f:a3 \
  -H "Content-Type: application/json" \
  -d '{"auto_install_file":"ubuntu/lab-bench.yaml"}'
```

The auto-install endpoint clients hit at boot:

```
GET /autoinstall/<image-filename>?mac=<mac>             # preseed, kickstart
GET /autoinstall/<image-filename>/mac/<mac>/user-data   # Ubuntu autoinstall
GET /autoinstall/<image-filename>/mac/<mac>/meta-data   # Ubuntu autoinstall
```

The boot menu builds these URLs with the client's MAC address so per-client overrides resolve correctly; for Ubuntu, cloud-init appends `user-data` and `meta-data` itself. Requests without the MAC address, or in another shape such as `/autoinstall/<image-filename>/user-data`, return 404.

## Troubleshooting

### 404 from `/autoinstall/...`

`no auto-install configuration for this image/client` — nothing is attached at any level of the resolution chain. Either attach a file to the image, the client, or its group, or check that `auto_install_file` actually points at a file that exists under `data/autoinstall/`.

### Placeholders rendered literally

`{{HOSTNAME}}` showing up as the literal string in the installed system means the file was served before the substitution ran — usually because the client booted by IP only and the request didn't include a `mac` query param. Confirm the boot menu is generating URLs of the form `/autoinstall/<iso>?mac=<mac>` or `/autoinstall/<iso>/mac/<mac>/`.

On Windows, literal placeholders mean Setup used the unrendered copy staged on the SMB share rather than the file wimboot injects. Preview the client's boot menu (see [Previewing a Client's Boot Menu](clients.md#previewing-a-clients-boot-menu)) and check the Windows section contains an `initrd .../autoinstall/<iso>?mac=<mac> AutoUnattend.xml` line, and that the image's boot parameters don't include `rawwim`.

A `{{VAR.<name>}}` left literal means neither the client nor its group defines that variable, or the name is spelt differently (names are case-sensitive). Every unresolved placeholder is logged when the file is served, and the editor's **Preview** lists them for a chosen client before anything boots.

### Wrong file served

Resolution is most-specific-first. If a client has its own override and you don't expect it, that's why the image-level default isn't being used. Check the server log line:

```
Served auto-install script for ... (source: client:..., type: ..., size: ...)
```

The `source:` field tells you exactly which slot won.

### Windows Setup runs interactively

- An autounattend file must resolve for the booting client: attach one to the image (image properties → Auto-Install), to the client, or to its group.
- Preview the client's boot menu and confirm the Windows section contains the `initrd .../autoinstall/<iso>?mac=<mac> AutoUnattend.xml` line. If it doesn't, no autounattend file resolved for that MAC.
- Make sure the image's boot parameters don't include `rawwim`; it stops wimboot injecting files into WinPE.
- Re-patch `boot.wim` after upgrading Bootimus so `startnet.cmd` knows to look for the injected file: **Images** → **Patch SMB** (or it re-patches automatically on next boot).
- Confirm the SMB share is reachable from the client (`net view \\<server>` from WinPE); the installer itself still comes from the share.

### "AutoUnattend.xml neither injected by wimboot nor staged on the share, running interactive setup"

Logged by `startnet.cmd` when the image has an autounattend file attached but neither delivery path produced one. Check the Bootimus log for the client's `/autoinstall/<iso>?mac=...` request at boot time (a 404 means no file resolved for that client) and for the staging step around the time of the patch. Re-run the SMB patch from the image properties.

### `Z:\setup.exe not found on the share`

`startnet.cmd` mapped `Z:` but the listing it prints shows the image folder (`iso\`, `metadata.txt`) rather than the installer files. Bootimus always shares the image's `iso` subfolder, so the client has connected to a different SMB server on the same address. Check the Bootimus log for `SMB: smbd exited` and the **Windows SMB** line under **Server Info**. The usual cause is the data directory living on a filesystem without Unix permissions, which makes `smbd` exit with `invalid permissions on directory '.../smb/locks/msg.lock': has 0777 should be 0755`, leaving a system Samba to answer on port 445 instead. Move the data directory to ext4 or XFS, stop the other Samba, and restart Bootimus.

### `net use fails after VM reboot`

Fixed in 0.1.58 by enabling `reset on zero vc = yes` in the bundled Samba config. If you maintain a custom `smb.conf`, add:

```
reset on zero vc = yes
oplocks = no
kernel oplocks = no
level2 oplocks = no
strict locking = no
deadtime = 1
```

## Next Steps

- See [Image Management](images.md) for attaching files to images.
- See [Client Management](clients.md) for per-client overrides.
- See [Distro Profiles](distro-profiles.md) for the underlying profile IDs that map to library subdirectories.
