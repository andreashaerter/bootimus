# Auto-Install-Leitfaden

Unbeaufsichtigte Installationen für Windows, Ubuntu, Debian und die Red-Hat-Familie. Einmal eine Config-Datei reinwerfen, an ein Image anhängen — und jeder PXE-Boot zieht die Installation ohne einen einzigen Tastendruck durch.

## Inhaltsverzeichnis

- [Überblick](#überblick)
- [Unterstützte Formate](#unterstützte-formate)
- [Die Dateibibliothek](#die-dateibibliothek)
- [Dateien anhängen](#dateien-anhängen)
- [Auflösungsreihenfolge](#auflösungsreihenfolge)
- [Platzhalter](#platzhalter)
- [Beispiele](#beispiele)
- [Installationsabschluss melden](#installationsabschluss-melden)
- [Windows-Hinweise](#windows-hinweise)
- [REST-API](#rest-api)
- [Fehlersuche](#fehlersuche)

## Überblick

Auto-Install-Konfigurationen werden in einer Per-Distro-Bibliothek unter `data/autoinstall/` gespeichert. Du kannst:

- **Dateien im UI verwalten** im Tab **Auto-Install** — Configs anlegen, editieren, hochladen, herunterladen und löschen, ohne das Dateisystem anzufassen.
- **Default pro Image setzen** (jeder Client, der dieses Image bootet, bekommt dieselbe Config).
- **Pro Client oder Client-Gruppe überschreiben**, wenn eine Maschine eine andere Config braucht (anderer Hostname, anderes Disk-Layout, andere Rolle etc.).
- **Templates bauen** mit Platzhaltern wie `{{HOSTNAME}}` und `{{IP}}`, die beim Ausliefern anhand der Identität des bootenden Clients aufgelöst werden.

Bootimus liefert die gerenderte Datei über HTTP am Auto-Install-Endpunkt aus. Linux-Installer holen sie über einen Kernel-Parameter; für Windows holt wimboot dieselbe URL als zusätzliche `initrd` und injiziert sie als `AutoUnattend.xml` in die WinPE-RAM-Disk, sodass `setup.exe /unattend:` sie automatisch aufgreift (siehe [Windows-Hinweise](#windows-hinweise)).

## Unterstützte Formate

| Distro-Familie | Format | Datei-Endung | Erkannt als |
|--------------|--------|----------------|-------------|
| Windows (10/11/Server) | `autounattend.xml` | `.xml` | `autounattend` |
| Ubuntu (Server live, 20.04+) | cloud-init / autoinstall | `.yaml`, `.yml` | `autoinstall` |
| Debian | preseed | `.cfg` | `preseed` |
| Red Hat / Rocky / Fedora / Alma | kickstart | `.ks` | `kickstart` |
| Alles andere | raw | beliebig | `generic` |

Die Endung steuert sowohl das Label im UI als auch den `Content-Type`-Header beim Ausliefern.

## Die Dateibibliothek

Alle Auto-Install-Dateien liegen unter `data/autoinstall/<distro>/<filename>`:

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

Das Segment `<distro>` muss zu einer bekannten Distro-Profil-ID passen (siehe [Distro-Profile](distro-profiles.md)). Das Verzeichnis wird beim ersten Start automatisch angelegt.

### Dateien über das UI hinzufügen

Im Tab **Auto-Install** → **New File** öffnet sich der Editor mit einem Distro-Picker, einem Dateinamen-Feld und einer Syntax-freundlichen Textarea. **Upload File** nimmt eine beliebige lokale Datei und legt sie im gewählten Distro-Ordner ab.

### Dateien manuell hinzufügen

Einfach reinwerfen:

```bash
mkdir -p data/autoinstall/ubuntu
cp my-autoinstall.yaml data/autoinstall/ubuntu/default.yaml
```

Sie tauchen sofort im UI auf — kein Neustart, kein Scan.

## Dateien anhängen

Auto-Install-Dateien tun nichts, bis du sie verdrahtest. Es gibt drei Stellen zum Anhängen:

### Image (Default)

Tab **Images** → **Eigenschaften** eines Images öffnen → Abschnitt **Auto-Install** → Datei wählen. Jeder Client, der dieses Image bootet, bekommt diese Config, sofern nichts Spezifischeres überschreibt.

### Client (Per-Maschine-Override)

Tab **Clients** → Client öffnen → Dropdown **Auto-Install File**. Nutze das, wenn eine bestimmte Maschine eine andere Config braucht (z.B. ein Build-Server vs. der Rest der Schreibtisch-Flotte).

### Client-Gruppe (Per-Flotten-Override)

Tab **Groups** → Gruppe öffnen → **Auto-Install File**. Gilt für jeden Client in der Gruppe. Nützlich für "alle Workstations in Lab 3"-Szenarien.

## Auflösungsreihenfolge

Wenn ein Client seine Auto-Install-Datei anfordert, läuft Bootimus diese Hierarchie ab:

```
1. Per-Client-Override        (Client.AutoInstallFile)
2. Per-Group-Override         (ClientGroup.AutoInstallFile, falls Client in einer Gruppe)
3. Image-Default              (Image.AutoInstallFile)
4. Inline-Legacy-Skript       (Image.AutoInstallScript — Pre-0.1.58-Setups)
5. → 404 (kein Auto-Install konfiguriert)
```

Der erste nicht-leere Treffer gewinnt. Der Endpunkt loggt die Quelle, von der er ausgeliefert hat:

```
Served auto-install script for ubuntu-24.04-live-server-amd64.iso \
  (source: client:b4:2e:99:01:5f:a3, type: autoinstall, size: 1247 bytes)
```

## Platzhalter

Diese Tokens werden beim Ausliefern pro Client ersetzt:

| Token | Wird ersetzt durch |
|-------|---------------|
| `{{MAC}}` | Client-MAC-Adresse (lowercase, mit Doppelpunkten) |
| `{{CLIENT_NAME}}` | Anzeigename aus der Clients-Tabelle |
| `{{HOSTNAME}}` | Wie `{{CLIENT_NAME}}` (Alias zur Klarheit in Configs) |
| `{{IP}}` | Client-IP, die den Request gestellt hat |
| `{{SERVER_ADDR}}` | Adresse des Bootimus-Servers |
| `{{IMAGE_NAME}}` | Anzeigename des bootenden Images |
| `{{IMAGE_FILENAME}}` | ISO-Dateiname des bootenden Images |
| `{{CALLBACK_URL}}` | Vollständige URL, die der Installer zum Melden des Abschlusses aufruft, inklusive Einmal-Token (siehe [Installationsabschluss melden](#installationsabschluss-melden)) |
| `{{INSTALL_TOKEN}}` | Der nackte Token hinter `{{CALLBACK_URL}}`, um einen eigenen Request zu bauen |

Platzhalter sind plain String-Substitution — kein Escaping. Quote sie passend zum Zielformat (XML, YAML etc.).

Die Substitution passiert am Auto-Install-Endpunkt und gilt damit für jedes Format. Auch Windows-`autounattend.xml` wird für den bootenden Client gerendert, weil wimboot sie beim Boot von genau diesem Endpunkt holt (siehe [Windows-Hinweise](#windows-hinweise)); nur die dort beschriebene ungerenderte SMB-Fallback-Kopie umgeht sie.

## Beispiele

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

Bootimus fügt den Boot-Parameter selbst hinzu, sobald eine Auto-Install-Datei angehängt ist; in den Boot-Parametern des Images musst du also nichts eintragen. Der erzeugte Parameter sieht so aus:

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

`data/autoinstall/windows/kiosk.xml`: Standard-`<unattend>`-Dokument — siehe [Microsofts autounattend-Referenz](https://learn.microsoft.com/en-us/windows-hardware/customize/desktop/unattend/). Platzhalter funktionieren in jedem Text-Node, wenn der Client über wimboot bootet — so bootet Bootimus Windows-Images:

```xml
<ComputerName>{{HOSTNAME}}</ComputerName>
```

## Installationsabschluss melden

Bootimus verfolgt jede unbeaufsichtigte Installation ab dem Moment, in dem der Client den Kernel des Images lädt. Der Eintrag durchläuft vier Zustände:

| Zustand | Gesetzt, wenn |
|---------|---------------|
| `booted` | Der Client hat die Boot-Dateien des Images geholt und das Image hat eine Auto-Install-Datei oder ein Inline-Skript |
| `installing` | Der Installer hat die Auto-Install-Datei geholt |
| `completed` | Das installierte System hat `{{CALLBACK_URL}}` aufgerufen |
| `failed` | Der Callback hat `status=failed` gemeldet, oder innerhalb des Timeouts kam keine Rückmeldung |

Der aktuelle Zustand steht in der Spalte **Install** der Clients-Tabelle, die vollständige Historie unter **Install History** im Client-Editor. Installationen, die sich nie melden, werden nach sechs Stunden als fehlgeschlagen markiert; ändere das mit `--install-timeout-hours` oder `install_timeout_hours` in `bootimus.yaml`. Jeder Übergang nach `completed` oder `failed` löst ein `install.completed`- bzw. `install.failed`-Webhook-Event aus, wenn dieses Event unter **Settings → Webhook** aktiviert ist.

Der Callback ist ein einfacher HTTP-Request, `GET` oder `POST`, an den Boot-Port:

```
http://<server>:8080/install/callback?token=<token>&status=completed|failed&message=<optionaler Text>
```

`{{CALLBACK_URL}}` enthält den Token bereits, ein Skript muss also nur `&status=failed&message=...` anhängen. Nur die erste Meldung pro Token zählt; spätere Aufrufe antworten mit `already recorded`.

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

Das Modul `phone_home` meldet sich nach dem ersten Boot, was das zuverlässigste Signal ist:

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

Oder melde aus der Installer-Umgebung mit `late-commands`:

```yaml
  late-commands:
    - curtin in-target --target=/target -- curl -fsS "{{CALLBACK_URL}}"
```

### Windows (`autounattend.xml`)

Füge unter `Microsoft-Windows-Shell-Setup` einen First-Logon-Befehl hinzu:

```xml
<FirstLogonCommands>
  <SynchronousCommand wcm:action="add">
    <Order>1</Order>
    <CommandLine>powershell -NoProfile -Command "Invoke-WebRequest -UseBasicParsing '{{CALLBACK_URL}}'"</CommandLine>
    <Description>Report install to Bootimus</Description>
  </SynchronousCommand>
</FirstLogonCommands>
```

Denke daran, dass `&` in XML-Attributen oder -Text als `&amp;` geschrieben werden muss, wenn du `status`- oder `message`-Parameter anhängst.

### Talos- und Flatcar-Nodes

Kubernetes-Nodes, die über den Tab **Kubernetes** aufgenommen wurden, melden sich bereits über den Machine-Config-Ablauf und die Events `node.install_started` / `node.install_completed`. Sie brauchen keinen Callback.

### Historie abfragen

```bash
# Historie eines Clients
curl -H "Authorization: Bearer $TOKEN" "http://localhost:8081/api/clients/installs?mac=b4:2e:99:01:5f:a3"

# Aktuelle Installationen der gesamten Flotte
curl -H "Authorization: Bearer $TOKEN" "http://localhost:8081/api/installs?limit=50"
```

## Windows-Hinweise

Windows-Images booten über [wimboot](https://ipxe.org/wimboot), das jede zusätzliche `initrd`-Datei aus dem iPXE-Skript in die WinPE-RAM-Disk injiziert. Bootimus nutzt das, um die Auto-Install-Datei pro Client auszuliefern:

1. Wenn für den bootenden Client eine autounattend-Datei aufgelöst wird (über die übliche [Auflösungsreihenfolge](#auflösungsreihenfolge): Client, dann Gruppe, dann Image), fügt der generierte iPXE-Boot-Abschnitt vor `boot.wim` die Zeile `initrd http://<server>:<port>/autoinstall/<iso>?mac=<mac> AutoUnattend.xml` ein.
2. Der Auto-Install-Endpunkt rendert die Datei mit den Platzhaltern des Clients, genau wie für Linux-Installer.
3. wimboot legt die Datei in WinPE unter `X:\Windows\System32\AutoUnattend.xml` ab.
4. Die gepatchte `startnet.cmd` findet sie dort und startet `setup.exe /unattend:X:\Windows\System32\AutoUnattend.xml`.

Weil die Datei pro Client aufgelöst und gerendert wird, funktionieren Per-Client- und Per-Gruppen-Overrides sowie Platzhalter wie `{{HOSTNAME}}` unter Windows genauso wie überall sonst. Boot-Parameter werden weiterhin an wimboot durchgereicht; vermeide `rawwim`, das schaltet die Datei-Injektion ab.

**SMB-Fallback.** Wenn `boot.wim` für ein Image gepatcht wird, an dem auf Image-Ebene eine autounattend-Datei hängt, stellt Bootimus diese Datei zusätzlich ungerendert als `AutoUnattend.xml` auf der SMB-Freigabe des Images bereit. `startnet.cmd` kopiert sie nur dann nach `X:\AutoUnattend.xml`, wenn wimboot nichts injiziert hat, etwa weil der Client die gepatchte `boot.wim` auf einem anderen Weg als über das Bootimus-iPXE-Menü gebootet hat. Der Fallback kennt keine Platzhalter-Substitution und keine Per-Client- oder Per-Gruppen-Overrides: Platzhalter darin erreichen Setup wörtlich.

Ohne autounattend-Datei auf irgendeiner Ebene läuft Setup wie gehabt interaktiv.

**Anforderungen an das Datenverzeichnis.** Bootimus betreibt einen eigenen `smbd`, dessen Zustand unter `data/smb` liegt. Samba startet nicht, wenn dieses Verzeichnis auf einem Dateisystem liegt, das keine Unix-Berechtigungen speichern kann (exFAT, NTFS oder ein CIFS-Mount), weil sein Lock-Verzeichnis dann als `0777` statt `0755` zurückgemeldet wird. Lass das Datenverzeichnis auf einem nativen Linux-Dateisystem wie ext4 oder XFS liegen und stell sicher, dass kein anderer SMB-Server auf dem Host Port 445 belegt. Bootimus prüft beides beim Start und meldet es unter **Server Info** → **Windows SMB**.

**Reboot-Resilienz.** WinPE startet mitten in der Installation neu und verbindet sich von derselben Client-IP wieder. Die mitgelieferte Samba-Config setzt `reset on zero vc = yes` und deaktiviert Oplocks, damit das zweite `net use` nicht an einem stale Session-State scheitert. Wenn du `data/smb/smb.conf` durch deine eigene ersetzt hast, übernimm diese Einstellungen.

**Patch rückgängig machen.** Beim ersten SMB-Patch wird eine unveränderte Kopie der `boot.wim` als `boot.wim.orig` daneben abgelegt. **Images** → **SMB-Patch entfernen** stellt das Original wieder her und entfernt die SMB-Freigabe des Images — die ISO bootet dann exakt wie hochgeladen. Images, die von älteren Bootimus-Versionen gepatcht wurden, haben keine Sicherung: erst das Image neu extrahieren (das erzeugt eine frische unveränderte Kopie), dann den Patch entfernen.

## REST-API

Alles im UI ist auch ein REST-Aufruf.

```bash
# Alle Auto-Install-Dateien auflisten
curl -u admin:pw http://localhost:8081/api/autoinstall-files

# Datei lesen
curl -u admin:pw "http://localhost:8081/api/autoinstall-files/get?distro=ubuntu&filename=default.yaml"

# Datei anlegen oder überschreiben
curl -u admin:pw -X POST http://localhost:8081/api/autoinstall-files/save \
  -H "Content-Type: application/json" \
  -d '{"distro":"ubuntu","filename":"default.yaml","content":"#cloud-config\n..."}'

# Datei hochladen
curl -u admin:pw -X POST http://localhost:8081/api/autoinstall-files/upload \
  -F "distro=windows" \
  -F "filename=kiosk.xml" \
  -F "file=@./kiosk.xml"

# Herunterladen
curl -u admin:pw "http://localhost:8081/api/autoinstall-files/download?distro=ubuntu&filename=default.yaml" -o default.yaml

# Löschen
curl -u admin:pw -X POST "http://localhost:8081/api/autoinstall-files/delete?distro=ubuntu&filename=default.yaml"
```

Datei an ein Image anhängen:

```bash
curl -u admin:pw -X PUT http://localhost:8081/api/images/update \
  -H "Content-Type: application/json" \
  -d '{"filename":"ubuntu-24.04-live-server-amd64.iso","auto_install_file":"ubuntu/default.yaml"}'
```

Datei an einen Client anhängen:

```bash
curl -u admin:pw -X PUT http://localhost:8081/api/clients/b4:2e:99:01:5f:a3 \
  -H "Content-Type: application/json" \
  -d '{"auto_install_file":"ubuntu/lab-bench.yaml"}'
```

Den Auto-Install-Endpunkt, den Clients beim Boot ansprechen:

```
GET /autoinstall/<image-filename>?mac=<mac>             # preseed, kickstart
GET /autoinstall/<image-filename>/mac/<mac>/user-data   # Ubuntu autoinstall
GET /autoinstall/<image-filename>/mac/<mac>/meta-data   # Ubuntu autoinstall
```

Das Boot-Menü baut diese URLs mit der MAC-Adresse des Clients, damit Per-Client-Overrides korrekt aufgelöst werden; bei Ubuntu hängt cloud-init `user-data` und `meta-data` selbst an. Anfragen ohne MAC-Adresse oder in anderer Form, etwa `/autoinstall/<image-filename>/user-data`, liefern 404.

## Fehlersuche

### 404 von `/autoinstall/...`

`no auto-install configuration for this image/client` — auf keiner Ebene der Auflösungskette ist etwas angehängt. Entweder hänge eine Datei an das Image, den Client oder seine Gruppe — oder prüfe, ob `auto_install_file` tatsächlich auf eine existierende Datei unter `data/autoinstall/` zeigt.

### Platzhalter werden wörtlich ausgespielt

Wenn `{{HOSTNAME}}` als literaler String im installierten System auftaucht, wurde die Datei vor dem Substitutions-Lauf ausgeliefert — meistens, weil der Client nur per IP gebootet hat und der Request keinen `mac`-Query-Parameter enthielt. Stelle sicher, dass das Boot-Menü URLs der Form `/autoinstall/<iso>?mac=<mac>` oder `/autoinstall/<iso>/mac/<mac>/` erzeugt.

Unter Windows bedeuten wörtliche Platzhalter, dass Setup die ungerenderte Kopie von der SMB-Freigabe statt der von wimboot injizierten Datei benutzt hat. Zeige das Boot-Menü des Clients in der Vorschau an (siehe [Boot-Menü eines Clients in der Vorschau](clients.md#previewing-a-clients-boot-menu)) und prüfe, dass der Windows-Abschnitt eine Zeile `initrd .../autoinstall/<iso>?mac=<mac> AutoUnattend.xml` enthält und die Boot-Parameter des Images kein `rawwim` enthalten.

### Falsche Datei ausgeliefert

Die Auflösung läuft nach Most-Specific-First. Wenn ein Client einen eigenen Override hat und du das nicht erwartest, ist das der Grund, warum das Image-Level-Default nicht greift. Prüfe die Server-Log-Zeile:

```
Served auto-install script for ... (source: client:..., type: ..., size: ...)
```

Das Feld `source:` sagt dir genau, welcher Slot gewonnen hat.

### Windows-Setup läuft interaktiv

- Für den bootenden Client muss eine autounattend-Datei aufgelöst werden: hänge eine ans Image (Image-Eigenschaften → Auto-Install), an den Client oder an seine Gruppe.
- Zeige das Boot-Menü des Clients in der Vorschau an und prüfe, dass der Windows-Abschnitt die Zeile `initrd .../autoinstall/<iso>?mac=<mac> AutoUnattend.xml` enthält. Fehlt sie, wurde für diese MAC keine autounattend-Datei aufgelöst.
- Stelle sicher, dass die Boot-Parameter des Images kein `rawwim` enthalten; das verhindert, dass wimboot Dateien in WinPE injiziert.
- `boot.wim` nach einem Bootimus-Upgrade neu patchen, damit `startnet.cmd` die injizierte Datei sucht: **Images** → **Patch SMB** (oder beim nächsten Boot wird automatisch neu gepatcht).
- Erreichbarkeit der SMB-Freigabe vom Client prüfen (`net view \\<server>` aus WinPE); der Installer selbst kommt weiterhin von der Freigabe.

### "AutoUnattend.xml neither injected by wimboot nor staged on the share, running interactive setup"

Wird von `startnet.cmd` geloggt, wenn am Image eine autounattend-Datei hängt, aber keiner der beiden Auslieferungswege eine Datei geliefert hat. Prüfe im Bootimus-Log den `/autoinstall/<iso>?mac=...`-Request des Clients zum Boot-Zeitpunkt (ein 404 heißt, dass für diesen Client keine Datei aufgelöst wurde) und den Staging-Schritt um den Patch-Zeitpunkt herum. SMB-Patch aus den Image-Eigenschaften erneut ausführen.

### `Z:\setup.exe not found on the share`

`startnet.cmd` hat `Z:` eingebunden, aber die ausgegebene Auflistung zeigt den Image-Ordner (`iso\`, `metadata.txt`) statt der Installationsdateien. Bootimus gibt immer den `iso`-Unterordner des Images frei, der Client hat sich also mit einem anderen SMB-Server unter derselben Adresse verbunden. Prüfe das Bootimus-Log auf `SMB: smbd exited` und die Zeile **Windows SMB** unter **Server Info**. Die übliche Ursache ist ein Datenverzeichnis auf einem Dateisystem ohne Unix-Berechtigungen; `smbd` beendet sich dann mit `invalid permissions on directory '.../smb/locks/msg.lock': has 0777 should be 0755`, und stattdessen antwortet ein System-Samba auf Port 445. Verschiebe das Datenverzeichnis auf ext4 oder XFS, stoppe das andere Samba und starte Bootimus neu.

### `net use fails after VM reboot`

In 0.1.58 behoben, indem `reset on zero vc = yes` in der mitgelieferten Samba-Config aktiviert wurde. Wenn du eine eigene `smb.conf` pflegst, ergänze:

```
reset on zero vc = yes
oplocks = no
kernel oplocks = no
level2 oplocks = no
strict locking = no
deadtime = 1
```

## Nächste Schritte

- Siehe [Image-Verwaltung](images.md), um Dateien an Images zu hängen.
- Siehe [Client-Verwaltung](clients.md) für Per-Client-Overrides.
- Siehe [Distro-Profile](distro-profiles.md) für die zugrunde liegenden Profil-IDs, die auf die Bibliotheks-Unterverzeichnisse mappen.
