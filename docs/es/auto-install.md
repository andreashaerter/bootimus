# Guía de auto-instalación

Lanza instalaciones desatendidas en Windows, Ubuntu, Debian y las distros de la familia Red Hat. Suelta un archivo de configuración una vez, adjúntalo a una imagen, y cada arranque PXE termina la instalación sin una sola pulsación de tecla.

## Tabla de contenidos

- [Visión general](#visión-general)
- [Formatos soportados](#formatos-soportados)
- [La biblioteca de archivos](#la-biblioteca-de-archivos)
- [Adjuntar archivos](#adjuntar-archivos)
- [Orden de resolución](#orden-de-resolución)
- [Placeholders](#placeholders)
- [Ejemplos](#ejemplos)
- [Informar de la finalización de la instalación](#informar-de-la-finalización-de-la-instalación)
- [Notas sobre Windows](#notas-sobre-windows)
- [API REST](#api-rest)
- [Solución de problemas](#solución-de-problemas)

## Visión general

Los configs de auto-instalación se almacenan en una biblioteca por distro bajo `data/autoinstall/`. Puedes:

- **Gestionar archivos en la UI** bajo la pestaña **Auto-Install** — crear, editar, subir, descargar y borrar configs sin tocar el filesystem.
- **Definir un default** por imagen (cada cliente que arranque esa imagen recibe el mismo config).
- **Override por cliente** o **por grupo de cliente** cuando una máquina necesita un config diferente (hostname distinto, layout de disco, rol, etc.).
- **Plantillar** con placeholders como `{{HOSTNAME}}` e `{{IP}}` resueltos al servir, usando la identidad del cliente que arranca.

Bootimus sirve el archivo renderizado por HTTP en el endpoint de auto-instalación. Los instaladores Linux lo obtienen mediante un parámetro del kernel; para Windows, wimboot descarga la misma URL como un `initrd` adicional y lo inyecta en el RAM disk de WinPE como `AutoUnattend.xml`, de modo que `setup.exe /unattend:` lo recoge automáticamente (ver [Notas sobre Windows](#notas-sobre-windows)).

## Formatos soportados

| Familia de distro | Formato | Extensión | Detectado como |
|--------------|--------|----------------|-------------|
| Windows (10/11/Server) | `autounattend.xml` | `.xml` | `autounattend` |
| Ubuntu (Server live, 20.04+) | cloud-init / autoinstall | `.yaml`, `.yml` | `autoinstall` |
| Debian | preseed | `.cfg` | `preseed` |
| Red Hat / Rocky / Fedora / Alma | kickstart | `.ks` | `kickstart` |
| Cualquier otro | raw | cualquiera | `generic` |

La extensión determina tanto la etiqueta en la UI como la cabecera `Content-Type` cuando se sirve el archivo.

## La biblioteca de archivos

Todos los archivos de auto-instalación viven bajo `data/autoinstall/<distro>/<filename>`:

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

El segmento `<distro>` debe coincidir con un ID de perfil de distro conocido (mira [Perfiles de distro](distro-profiles.md)). El directorio se crea automáticamente en el primer arranque.

### Añadir archivos vía la UI

Pestaña **Auto-Install** → **New File** abre el editor con un selector de distro, campo de nombre de archivo y un textarea amigable para sintaxis. **Upload File** acepta cualquier archivo local y lo suelta en la carpeta de distro elegida.

### Añadir archivos manualmente

Simplemente suéltalos:

```bash
mkdir -p data/autoinstall/ubuntu
cp my-autoinstall.yaml data/autoinstall/ubuntu/default.yaml
```

Aparecen en la UI inmediatamente — sin reinicio, sin scan.

## Adjuntar archivos

Los archivos de auto-instalación no tienen efecto hasta que los conectes. Hay tres lugares donde adjuntar uno:

### Imagen (default)

Pestaña **Images** → abre las **Properties** de una imagen → sección **Auto-Install** → elige un archivo. Cada cliente que arranque esta imagen recibe este config a menos que algo más específico lo sobrescriba.

### Cliente (override por máquina)

Pestaña **Clients** → abre un cliente → desplegable **Auto-Install File**. Usa esto cuando una máquina específica necesita un config diferente (p. ej., un servidor de build vs el resto de la flota del escritorio).

### Grupo de clientes (override por flota)

Pestaña **Groups** → abre un grupo → **Auto-Install File**. Se aplica a cada cliente del grupo. Útil para escenarios tipo "todos los workstations del laboratorio 3".

## Orden de resolución

Cuando un cliente pide su archivo de auto-instalación, Bootimus recorre esta jerarquía:

```
1. Per-client override        (Client.AutoInstallFile)
2. Per-group override         (ClientGroup.AutoInstallFile, if client is in a group)
3. Image default              (Image.AutoInstallFile)
4. Inline legacy script       (Image.AutoInstallScript — pre-0.1.58 setups)
5. → 404 (no auto-install configured)
```

La primera coincidencia no vacía gana. El endpoint registra la fuente desde la que sirvió:

```
Served auto-install script for ubuntu-24.04-live-server-amd64.iso \
  (source: client:b4:2e:99:01:5f:a3, type: autoinstall, size: 1247 bytes)
```

## Placeholders

Estos tokens se sustituyen por cliente al servir:

| Token | Reemplazado con |
|-------|---------------|
| `{{MAC}}` | Dirección MAC del cliente (minúsculas, separada por dos puntos) |
| `{{CLIENT_NAME}}` | Nombre amigable de la tabla de Clients |
| `{{HOSTNAME}}` | Igual que `{{CLIENT_NAME}}` (alias por claridad en los configs) |
| `{{IP}}` | IP del cliente que emitió la petición |
| `{{SERVER_ADDR}}` | Dirección del servidor Bootimus |
| `{{IMAGE_NAME}}` | Nombre de display de la imagen que arranca |
| `{{IMAGE_FILENAME}}` | Filename del ISO de la imagen que arranca |
| `{{CALLBACK_URL}}` | URL completa a la que llama el instalador para informar de la finalización, incluido un token de un solo uso (ver [Informar de la finalización de la instalación](#informar-de-la-finalización-de-la-instalación)) |
| `{{INSTALL_TOKEN}}` | El token a secas que hay detrás de `{{CALLBACK_URL}}`, para construir tu propia petición |

Los placeholders son sustitución de strings plana — sin escape. Cítalos apropiadamente para el formato objetivo (XML, YAML, etc.).

La sustitución ocurre en el endpoint de auto-instalación, así que aplica a todos los formatos. El `autounattend.xml` de Windows también se renderiza para el cliente que arranca, porque wimboot lo descarga de ese endpoint al arrancar (ver [Notas sobre Windows](#notas-sobre-windows)); solo la copia de respaldo SMB sin renderizar que se describe allí se la salta.

## Ejemplos

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

Bootimus añade el parámetro de arranque por sí mismo cuando hay un archivo de auto-instalación asociado, así que no hay que añadir nada a los parámetros de arranque de la imagen. El parámetro generado tiene este aspecto:

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

`data/autoinstall/windows/kiosk.xml`: documento `<unattend>` estándar — mira la [referencia de autounattend de Microsoft](https://learn.microsoft.com/en-us/windows-hardware/customize/desktop/unattend/). Los placeholders funcionan dentro de cualquier nodo de texto cuando el cliente arranca mediante wimboot, que es como Bootimus arranca las imágenes de Windows:

```xml
<ComputerName>{{HOSTNAME}}</ComputerName>
```

## Informar de la finalización de la instalación

Bootimus sigue cada instalación desatendida desde el momento en que el cliente carga el kernel de la imagen. El registro pasa por cuatro estados:

| Estado | Se establece cuando |
|--------|---------------------|
| `booted` | El cliente descargó los archivos de arranque de la imagen y la imagen tiene un archivo de autoinstalación o un script inline |
| `installing` | El instalador descargó el archivo de autoinstalación |
| `completed` | El sistema instalado llamó a `{{CALLBACK_URL}}` |
| `failed` | El callback informó `status=failed`, o nada informó de vuelta dentro del timeout |

El estado actual se muestra en la columna **Install** de la tabla de Clients y el historial completo está en **Install History** dentro del editor de cliente. Las instalaciones que nunca informan se marcan como fallidas a las seis horas; cámbialo con `--install-timeout-hours` o `install_timeout_hours` en `bootimus.yaml`. Cada transición a `completed` o `failed` dispara un evento de webhook `install.completed` o `install.failed` cuando ese evento está habilitado en **Settings → Webhook**.

El callback es una petición HTTP normal, `GET` o `POST`, al puerto de arranque:

```
http://<server>:8080/install/callback?token=<token>&status=completed|failed&message=<optional text>
```

`{{CALLBACK_URL}}` ya contiene el token, así que a un script le basta con añadir `&status=failed&message=...`. Solo cuenta el primer informe de cada token; las llamadas posteriores responden `already recorded`.

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

El módulo `phone_home` hace el POST tras el primer arranque, que es la señal más fiable:

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

O informa desde el entorno del instalador con `late-commands`:

```yaml
  late-commands:
    - curtin in-target --target=/target -- curl -fsS "{{CALLBACK_URL}}"
```

### Windows (`autounattend.xml`)

Añade un comando de primer inicio de sesión bajo `Microsoft-Windows-Shell-Setup`:

```xml
<FirstLogonCommands>
  <SynchronousCommand wcm:action="add">
    <Order>1</Order>
    <CommandLine>powershell -NoProfile -Command "Invoke-WebRequest -UseBasicParsing '{{CALLBACK_URL}}'"</CommandLine>
    <Description>Report install to Bootimus</Description>
  </SynchronousCommand>
</FirstLogonCommands>
```

Recuerda que `&` dentro de atributos o texto XML debe escribirse como `&amp;` si añades los parámetros `status` o `message`.

### Nodos Talos y Flatcar

Los nodos de Kubernetes inscritos a través de la pestaña **Kubernetes** ya informan mediante el flujo de machine config y los eventos `node.install_started` / `node.install_completed`. No necesitan callback.

### Consultar el historial

```bash
# History for one client
curl -H "Authorization: Bearer $TOKEN" "http://localhost:8081/api/clients/installs?mac=b4:2e:99:01:5f:a3"

# Recent installs across the fleet
curl -H "Authorization: Bearer $TOKEN" "http://localhost:8081/api/installs?limit=50"
```

## Notas sobre Windows

Las imágenes de Windows arrancan mediante [wimboot](https://ipxe.org/wimboot), que inyecta cada archivo `initrd` adicional del script iPXE en el RAM disk de WinPE. Bootimus lo aprovecha para entregar el archivo de auto-instalación por cliente:

1. Cuando el cliente que arranca resuelve a un archivo autounattend (mediante el [orden de resolución](#orden-de-resolución) habitual: cliente, luego grupo, luego imagen), la sección de arranque iPXE generada añade `initrd http://<server>:<port>/autoinstall/<iso>?mac=<mac> AutoUnattend.xml` antes de `boot.wim`.
2. El endpoint de auto-instalación renderiza el archivo con los placeholders del cliente, exactamente igual que para los instaladores Linux.
3. wimboot deja el archivo en `X:\Windows\System32\AutoUnattend.xml` dentro de WinPE.
4. El `startnet.cmd` parcheado lo encuentra ahí y lanza `setup.exe /unattend:X:\Windows\System32\AutoUnattend.xml`.

Como el archivo se resuelve y renderiza por cliente, los overrides por cliente y por grupo y los placeholders como `{{HOSTNAME}}` funcionan en Windows igual que en el resto. Los parámetros de arranque siguen pasándose a wimboot; evita `rawwim`, que desactiva la inyección de archivos.

**Respaldo SMB.** Cuando se parchea `boot.wim` para una imagen que tiene un archivo autounattend adjunto a nivel de imagen, Bootimus además coloca ese archivo, sin renderizar, como `AutoUnattend.xml` en el share SMB de la imagen. `startnet.cmd` solo lo copia a `X:\AutoUnattend.xml` cuando wimboot no inyectó nada, por ejemplo cuando el cliente arrancó el `boot.wim` parcheado por otra vía distinta del menú iPXE de Bootimus. El respaldo no tiene sustitución de placeholders ni overrides por cliente o por grupo: los placeholders que contenga llegan a Setup literalmente.

Sin un archivo autounattend en ningún nivel, Setup corre interactivamente como antes.

**Requisitos del directorio de datos.** Bootimus ejecuta su propio `smbd` con su estado bajo `data/smb`. Samba se niega a arrancar si ese directorio está en un sistema de archivos que no puede almacenar permisos Unix (exFAT, NTFS o un montaje CIFS), porque su directorio de locks se lee entonces como `0777` en vez de `0755`. Mantén el directorio de datos en un sistema de archivos Linux nativo como ext4 o XFS y asegúrate de que ningún otro servidor SMB del host escuche en el puerto 445. Bootimus comprueba ambas condiciones al arrancar y las muestra en **Server Info** → **Windows SMB**.

**Resiliencia ante reinicios.** WinPE reinicia a mitad de instalación y se reconecta desde la misma IP de cliente. La config Samba incluida define `reset on zero vc = yes` y desactiva oplocks para que el segundo `net use` no tropiece con estado de sesión obsoleto. Si has reemplazado `data/smb/smb.conf` con el tuyo, replica estos settings.

**Deshacer el parche.** El primer parche SMB guarda una copia intacta de `boot.wim` como `boot.wim.orig` a su lado. **Images** → **Unpatch SMB** restaura el original y elimina el share SMB de la imagen — la ISO arranca entonces exactamente como se subió. Las imágenes parcheadas por versiones antiguas de Bootimus no tienen copia de seguridad: re-extrae primero la imagen (lo que produce una copia intacta fresca) y luego quita el parche.

## API REST

Todo lo de la UI es también una llamada REST.

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

Adjuntar un archivo a una imagen:

```bash
curl -u admin:pw -X PUT http://localhost:8081/api/images/update \
  -H "Content-Type: application/json" \
  -d '{"filename":"ubuntu-24.04-live-server-amd64.iso","auto_install_file":"ubuntu/default.yaml"}'
```

Adjuntar un archivo a un cliente:

```bash
curl -u admin:pw -X PUT http://localhost:8081/api/clients/b4:2e:99:01:5f:a3 \
  -H "Content-Type: application/json" \
  -d '{"auto_install_file":"ubuntu/lab-bench.yaml"}'
```

El endpoint de auto-instalación que los clientes consultan al arrancar:

```
GET /autoinstall/<image-filename>?mac=<mac>             # preseed, kickstart
GET /autoinstall/<image-filename>/mac/<mac>/user-data   # Ubuntu autoinstall
GET /autoinstall/<image-filename>/mac/<mac>/meta-data   # Ubuntu autoinstall
```

El menú de arranque construye estas URLs con la dirección MAC del cliente para que los overrides por cliente se resuelvan correctamente; en Ubuntu, cloud-init añade `user-data` y `meta-data` por su cuenta. Las peticiones sin dirección MAC, o con otra forma como `/autoinstall/<image-filename>/user-data`, devuelven 404.

## Solución de problemas

### 404 de `/autoinstall/...`

`no auto-install configuration for this image/client` — no hay nada adjuntado en ningún nivel de la cadena de resolución. O adjunta un archivo a la imagen, al cliente o a su grupo, o verifica que `auto_install_file` realmente apunta a un archivo que existe bajo `data/autoinstall/`.

### Placeholders renderizados literalmente

`{{HOSTNAME}}` apareciendo como la string literal en el sistema instalado significa que el archivo se sirvió antes de que se ejecutara la sustitución — normalmente porque el cliente arrancó solo por IP y la petición no incluyó un query param `mac`. Confirma que el menú de arranque está generando URLs del estilo `/autoinstall/<iso>?mac=<mac>` o `/autoinstall/<iso>/mac/<mac>/`.

En Windows, los placeholders literales significan que Setup usó la copia sin renderizar colocada en el share SMB en lugar del archivo que inyecta wimboot. Previsualiza el menú de arranque del cliente (ver [Previsualizar el menú de arranque de un cliente](clients.md#previewing-a-clients-boot-menu)) y comprueba que la sección de Windows contiene una línea `initrd .../autoinstall/<iso>?mac=<mac> AutoUnattend.xml`, y que los parámetros de arranque de la imagen no incluyen `rawwim`.

### Archivo equivocado servido

La resolución es de lo más específico a lo más general. Si un cliente tiene su propio override y no lo esperabas, ese es el motivo por el que el default a nivel de imagen no se usa. Revisa la línea de log del servidor:

```
Served auto-install script for ... (source: client:..., type: ..., size: ...)
```

El campo `source:` te dice exactamente qué slot ganó.

### Windows Setup corre interactivamente

- Debe resolverse un archivo autounattend para el cliente que arranca: adjunta uno a la imagen (propiedades de imagen → Auto-Install), al cliente o a su grupo.
- Previsualiza el menú de arranque del cliente y confirma que la sección de Windows contiene la línea `initrd .../autoinstall/<iso>?mac=<mac> AutoUnattend.xml`. Si no está, ningún archivo autounattend resolvió para esa MAC.
- Asegúrate de que los parámetros de arranque de la imagen no incluyen `rawwim`; impide que wimboot inyecte archivos en WinPE.
- Re-parchea `boot.wim` tras actualizar Bootimus para que `startnet.cmd` sepa buscar el archivo inyectado: **Images** → **Patch SMB** (o se re-parchea automáticamente en el siguiente arranque).
- Confirma que el share SMB es accesible desde el cliente (`net view \\<server>` desde WinPE); el instalador en sí sigue viniendo del share.

### "AutoUnattend.xml neither injected by wimboot nor staged on the share, running interactive setup"

Registrado por `startnet.cmd` cuando la imagen tiene un archivo autounattend adjunto pero ninguna de las dos vías de entrega produjo uno. Revisa en el log de Bootimus la petición `/autoinstall/<iso>?mac=...` del cliente en el momento del arranque (un 404 significa que no resolvió ningún archivo para ese cliente) y el paso de staging alrededor de la hora del patch. Vuelve a correr el patch SMB desde las propiedades de imagen.

### `Z:\setup.exe not found on the share`

`startnet.cmd` montó `Z:`, pero el listado que imprime muestra la carpeta de la imagen (`iso\`, `metadata.txt`) en lugar de los archivos del instalador. Bootimus siempre comparte la subcarpeta `iso` de la imagen, así que el cliente se ha conectado a otro servidor SMB en la misma dirección. Busca `SMB: smbd exited` en el log de Bootimus y revisa la línea **Windows SMB** en **Server Info**. La causa habitual es que el directorio de datos esté en un sistema de archivos sin permisos Unix, lo que hace que `smbd` termine con `invalid permissions on directory '.../smb/locks/msg.lock': has 0777 should be 0755` y que un Samba del sistema responda en el puerto 445 en su lugar. Mueve el directorio de datos a ext4 o XFS, detén el otro Samba y reinicia Bootimus.

### `net use falla tras reiniciar la VM`

Solucionado en 0.1.58 habilitando `reset on zero vc = yes` en la config Samba incluida. Si mantienes un `smb.conf` custom, añade:

```
reset on zero vc = yes
oplocks = no
kernel oplocks = no
level2 oplocks = no
strict locking = no
deadtime = 1
```

## Siguientes pasos

- Mira la [Gestión de imágenes](images.md) para adjuntar archivos a imágenes.
- Mira la [Gestión de clientes](clients.md) para overrides por cliente.
- Mira [Perfiles de distro](distro-profiles.md) para los IDs de perfil subyacentes que mapean a los subdirectorios de la biblioteca.
