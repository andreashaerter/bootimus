# Руководство по автоустановке

Запускайте unattended-установки для Windows, Ubuntu, Debian и дистрибутивов семейства Red Hat. Положите конфиг один раз, прикрепите его к образу — и каждая PXE-загрузка доводит установку до конца без единого нажатия клавиши.

## Оглавление

- [Обзор](#обзор)
- [Поддерживаемые форматы](#поддерживаемые-форматы)
- [Библиотека файлов](#библиотека-файлов)
- [Прикрепление файлов](#прикрепление-файлов)
- [Порядок разрешения](#порядок-разрешения)
- [Плейсхолдеры](#плейсхолдеры)
- [Примеры](#примеры)
- [Отчёт о завершении установки](#отчёт-о-завершении-установки)
- [Особенности Windows](#особенности-windows)
- [REST API](#rest-api)
- [Диагностика](#диагностика)

## Обзор

Конфиги автоустановки хранятся в библиотеке по дистрибутивам в `data/autoinstall/`. Вы можете:

- **Управлять файлами в UI** во вкладке **Auto-Install** — создавать, редактировать, загружать, скачивать и удалять конфиги без копания в файловой системе.
- **Задавать дефолт** для каждого образа (каждый клиент, грузящий этот образ, получит одинаковый конфиг).
- **Переопределять для клиента** или **для группы клиентов**, когда одной машине нужен другой конфиг (другой hostname, разметка диска, роль и т. д.).
- **Шаблонизировать** с помощью плейсхолдеров типа `{{HOSTNAME}}` и `{{IP}}`, которые подставляются на лету по личности грузящегося клиента.

Bootimus раздаёт скрипт по HTTP на эндпоинте автоустановки, а для Windows — стейджит `AutoUnattend.xml` на SMB-шару установки, чтобы `setup.exe /unattend:` подхватил его автоматически.

## Поддерживаемые форматы

| Семейство | Формат | Расширение | Определяется как |
|--------------|--------|----------------|-------------|
| Windows (10/11/Server) | `autounattend.xml` | `.xml` | `autounattend` |
| Ubuntu (Server live, 20.04+) | cloud-init / autoinstall | `.yaml`, `.yml` | `autoinstall` |
| Debian | preseed | `.cfg` | `preseed` |
| Red Hat / Rocky / Fedora / Alma | kickstart | `.ks` | `kickstart` |
| Что-то другое | raw | любое | `generic` |

Расширение определяет и метку в UI, и заголовок `Content-Type` при раздаче файла.

## Библиотека файлов

Все файлы автоустановки лежат под `data/autoinstall/<distro>/<filename>`:

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

Сегмент `<distro>` должен совпадать с ID известного профиля дистрибутива (см. [Профили дистрибутивов](distro-profiles.md)). Директория создаётся автоматически при первом запуске.

### Добавление файлов через UI

Вкладка **Auto-Install** → **New File** открывает редактор с выбором дистрибутива, полем имени файла и удобной текстовой областью. **Upload File** берёт любой локальный файл и кладёт его в выбранную папку дистрибутива.

### Добавление файлов вручную

Просто положите их:

```bash
mkdir -p data/autoinstall/ubuntu
cp my-autoinstall.yaml data/autoinstall/ubuntu/default.yaml
```

Они сразу появятся в UI — без перезапуска, без сканирования.

## Прикрепление файлов

Файлы автоустановки бесполезны, пока вы их не привяжете. Привязать можно в трёх местах:

### Образ (дефолт)

Вкладка **Images** → откройте **Properties** образа → раздел **Auto-Install** → выберите файл. Каждый клиент, грузящий этот образ, получит этот конфиг — если ничего более специфичного не переопределит.

### Клиент (переопределение для машины)

Вкладка **Clients** → откройте клиента → выпадающий список **Auto-Install File**. Используйте, когда одной конкретной машине нужен другой конфиг (например, сборочный сервер vs остальной парк рабочих столов).

### Группа клиентов (переопределение для парка)

Вкладка **Groups** → откройте группу → **Auto-Install File**. Применяется ко всем клиентам группы. Полезно для сценариев «все рабочие станции в лаборатории 3».

## Порядок разрешения

Когда клиент запрашивает свой файл автоустановки, Bootimus идёт по этой иерархии:

```
1. Переопределение клиента      (Client.AutoInstallFile)
2. Переопределение группы       (ClientGroup.AutoInstallFile, если клиент в группе)
3. Дефолт образа                (Image.AutoInstallFile)
4. Старый инлайн-скрипт         (Image.AutoInstallScript — установки до 0.1.58)
5. → 404 (автоустановка не настроена)
```

Побеждает первое непустое совпадение. Эндпоинт логирует источник, из которого раздал:

```
Served auto-install script for ubuntu-24.04-live-server-amd64.iso \
  (source: client:b4:2e:99:01:5f:a3, type: autoinstall, size: 1247 bytes)
```

## Плейсхолдеры

Эти токены подставляются для каждого клиента в момент раздачи:

| Токен | Заменяется на |
|-------|---------------|
| `{{MAC}}` | MAC-адрес клиента (в нижнем регистре, через двоеточие) |
| `{{CLIENT_NAME}}` | Дружественное имя из таблицы Clients |
| `{{HOSTNAME}}` | То же, что `{{CLIENT_NAME}}` (алиас для понятности в конфигах) |
| `{{IP}}` | IP клиента, отправившего запрос |
| `{{SERVER_ADDR}}` | Адрес сервера Bootimus |
| `{{IMAGE_NAME}}` | Отображаемое имя грузящегося образа |
| `{{IMAGE_FILENAME}}` | Имя ISO-файла грузящегося образа |
| `{{CALLBACK_URL}}` | Полный URL, который установщик вызывает, чтобы сообщить о завершении, включая одноразовый токен (см. [Отчёт о завершении установки](#отчёт-о-завершении-установки)) |
| `{{INSTALL_TOKEN}}` | Сам токен из `{{CALLBACK_URL}}` для сборки собственного запроса |

Плейсхолдеры — это обычная подстановка строки, без экранирования. Заключайте их в кавычки соответственно целевому формату (XML, YAML и т. д.).

## Примеры

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

Bootimus сам добавляет параметр загрузки, когда к образу привязан файл автоустановки, поэтому в параметры загрузки образа ничего добавлять не нужно. Сгенерированный параметр выглядит так:

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

`data/autoinstall/windows/kiosk.xml`: стандартный документ `<unattend>` — см. [справочник autounattend от Microsoft](https://learn.microsoft.com/en-us/windows-hardware/customize/desktop/unattend/). Плейсхолдеры работают в любом текстовом узле:

```xml
<ComputerName>{{HOSTNAME}}</ComputerName>
```

## Отчёт о завершении установки

Bootimus отслеживает каждую автоматическую установку с момента, когда клиент загружает ядро образа. Запись проходит через четыре состояния:

| Состояние | Устанавливается, когда |
|-----------|------------------------|
| `booted` | Клиент получил загрузочные файлы образа, а у образа есть файл автоустановки или встроенный скрипт |
| `installing` | Установщик получил файл автоустановки |
| `completed` | Установленная система вызвала `{{CALLBACK_URL}}` |
| `failed` | Обратный вызов сообщил `status=failed`, либо в течение таймаута никто не отчитался |

Текущее состояние показывается в столбце **Install** таблицы Clients, а полная история — в разделе **Install History** в редакторе клиента. Установки, которые так и не отчитались, помечаются как неудачные через шесть часов; измените это флагом `--install-timeout-hours` или параметром `install_timeout_hours` в `bootimus.yaml`. Каждый переход в `completed` или `failed` порождает событие вебхука `install.completed` или `install.failed`, если оно включено в **Settings → Webhook**.

Обратный вызов — это обычный HTTP-запрос, `GET` или `POST`, на загрузочный порт:

```
http://<server>:8080/install/callback?token=<token>&status=completed|failed&message=<optional text>
```

`{{CALLBACK_URL}}` уже содержит токен, так что скрипту достаточно дописать `&status=failed&message=...`. Засчитывается только первый отчёт по токену; последующие вызовы отвечают `already recorded`.

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

Модуль `phone_home` отправляет запрос после первой загрузки — это самый надёжный сигнал:

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

Либо отчитайтесь из окружения установщика через `late-commands`:

```yaml
  late-commands:
    - curtin in-target --target=/target -- curl -fsS "{{CALLBACK_URL}}"
```

### Windows (`autounattend.xml`)

Добавьте команду первого входа в `Microsoft-Windows-Shell-Setup`:

```xml
<FirstLogonCommands>
  <SynchronousCommand wcm:action="add">
    <Order>1</Order>
    <CommandLine>powershell -NoProfile -Command "Invoke-WebRequest -UseBasicParsing '{{CALLBACK_URL}}'"</CommandLine>
    <Description>Report install to Bootimus</Description>
  </SynchronousCommand>
</FirstLogonCommands>
```

Помните, что `&` внутри атрибутов или текста XML нужно записывать как `&amp;`, если вы дописываете параметры `status` или `message`.

### Узлы Talos и Flatcar

Узлы Kubernetes, зарегистрированные через вкладку **Kubernetes**, уже отчитываются через процесс машинной конфигурации и события `node.install_started` / `node.install_completed`. Обратный вызов им не нужен.

### Запрос истории

```bash
# История одного клиента
curl -H "Authorization: Bearer $TOKEN" "http://localhost:8081/api/clients/installs?mac=b4:2e:99:01:5f:a3"

# Недавние установки по всему парку
curl -H "Authorization: Bearer $TOKEN" "http://localhost:8081/api/installs?limit=50"
```

## Особенности Windows

Установки Windows идут через SMB. Когда к образу прикреплён файл autounattend, Bootimus:

1. Стейджит `AutoUnattend.xml` на SMB-шару установки при патчинге `boot.wim`.
2. Патчит `startnet.cmd` так, чтобы WinPE копировал его в `X:\AutoUnattend.xml` (локальный RAM-диск) при загрузке.
3. Запускает Setup как `setup.exe /unattend:X:\AutoUnattend.xml`.

Без autounattend-файла на образе Setup как и раньше работает интерактивно.

**Требования к каталогу данных.** Bootimus запускает собственный `smbd`, состояние которого хранится в `data/smb`. Samba отказывается стартовать, если этот каталог лежит на файловой системе, не умеющей хранить права Unix (exFAT, NTFS или CIFS-монтирование), потому что её каталог блокировок тогда читается как `0777` вместо `0755`. Держите каталог данных на родной файловой системе Linux, например ext4 или XFS, и убедитесь, что никакой другой SMB-сервер на хосте не слушает порт 445. Bootimus проверяет оба условия при запуске и показывает результат в **Server Info** → **Windows SMB**.

**Устойчивость к перезагрузкам.** WinPE перезагружается посреди установки и переподключается с того же IP клиента. В прилагаемом конфиге Samba стоит `reset on zero vc = yes` и отключены oplocks, чтобы второй `net use` не споткнулся об устаревшее состояние сессии. Если вы заменили `data/smb/smb.conf` своим, продублируйте эти настройки.

**Откат патча.** При первом SMB-патче рядом сохраняется нетронутая копия `boot.wim` под именем `boot.wim.orig`. **Images** → **Убрать патч SMB** восстанавливает оригинал и удаляет SMB-шару образа — ISO после этого грузится ровно так, как была загружена. У образов, пропатченных старыми версиями Bootimus, резервной копии нет: сначала повторно извлеките образ (это создаст свежую нетронутую копию), затем уберите патч.

## REST API

Всё, что есть в UI, — это также REST-вызов.

```bash
# Список всех файлов автоустановки
curl -u admin:pw http://localhost:8081/api/autoinstall-files

# Прочитать файл
curl -u admin:pw "http://localhost:8081/api/autoinstall-files/get?distro=ubuntu&filename=default.yaml"

# Создать или перезаписать файл
curl -u admin:pw -X POST http://localhost:8081/api/autoinstall-files/save \
  -H "Content-Type: application/json" \
  -d '{"distro":"ubuntu","filename":"default.yaml","content":"#cloud-config\n..."}'

# Загрузить файл
curl -u admin:pw -X POST http://localhost:8081/api/autoinstall-files/upload \
  -F "distro=windows" \
  -F "filename=kiosk.xml" \
  -F "file=@./kiosk.xml"

# Скачать
curl -u admin:pw "http://localhost:8081/api/autoinstall-files/download?distro=ubuntu&filename=default.yaml" -o default.yaml

# Удалить
curl -u admin:pw -X POST "http://localhost:8081/api/autoinstall-files/delete?distro=ubuntu&filename=default.yaml"
```

Прикрепить файл к образу:

```bash
curl -u admin:pw -X PUT http://localhost:8081/api/images/update \
  -H "Content-Type: application/json" \
  -d '{"filename":"ubuntu-24.04-live-server-amd64.iso","auto_install_file":"ubuntu/default.yaml"}'
```

Прикрепить файл к клиенту:

```bash
curl -u admin:pw -X PUT http://localhost:8081/api/clients/b4:2e:99:01:5f:a3 \
  -H "Content-Type: application/json" \
  -d '{"auto_install_file":"ubuntu/lab-bench.yaml"}'
```

Эндпоинт автоустановки, на который ходят клиенты при загрузке:

```
GET /autoinstall/<image-filename>?mac=<mac>             # preseed, kickstart
GET /autoinstall/<image-filename>/mac/<mac>/user-data   # Ubuntu autoinstall
GET /autoinstall/<image-filename>/mac/<mac>/meta-data   # Ubuntu autoinstall
```

Загрузочное меню формирует эти URL с MAC-адресом клиента, чтобы переопределения для конкретного клиента разрешались правильно; для Ubuntu cloud-init сам добавляет `user-data` и `meta-data`. Запросы без MAC-адреса или в другом виде, например `/autoinstall/<image-filename>/user-data`, возвращают 404.

## Диагностика

### 404 от `/autoinstall/...`

`no auto-install configuration for this image/client` — ни на одном уровне цепочки разрешения ничего не привязано. Прикрепите файл к образу, клиенту или его группе — или проверьте, что `auto_install_file` действительно указывает на существующий файл под `data/autoinstall/`.

### Плейсхолдеры выводятся буквально

`{{HOSTNAME}}` появляется в установленной системе как сама строка — значит, файл раздался до того, как сработала подстановка. Обычно потому, что клиент пришёл только по IP, и в запросе не было параметра `mac`. Убедитесь, что загрузочное меню формирует URL вида `/autoinstall/<iso>?mac=<mac>` или `/autoinstall/<iso>/mac/<mac>/`.

### Раздался не тот файл

Разрешение идёт от самого специфичного. Если у клиента есть собственное переопределение, а вы его не ожидали — вот почему дефолт уровня образа не используется. Посмотрите строку в логе сервера:

```
Served auto-install script for ... (source: client:..., type: ..., size: ...)
```

Поле `source:` точно скажет, какой слот победил.

### Windows Setup идёт интерактивно

- К образу должен быть прикреплён autounattend-файл (свойства образа → Auto-Install).
- Перепатчите `boot.wim` после привязки: **Images** → **Patch SMB** (или это произойдёт автоматически при следующей загрузке).
- Убедитесь, что SMB-шара доступна с клиента (`net view \\<server>` из WinPE).

### «AutoUnattend.xml not on share, running interactive setup»

Логируется `startnet.cmd`, когда файла нет там, где ожидается. Либо упал шаг стейджинга (посмотрите лог Bootimus в момент патча), либо SMB-шара потеряла файл. Перезапустите SMB-патч из свойств образа.

### `Z:\setup.exe not found on the share`

`startnet.cmd` подключил `Z:`, но в выведенном списке видна папка образа (`iso\`, `metadata.txt`), а не файлы установщика. Bootimus всегда расшаривает подпапку `iso` образа, значит клиент подключился к другому SMB-серверу по тому же адресу. Поищите в логе Bootimus `SMB: smbd exited` и посмотрите строку **Windows SMB** в **Server Info**. Обычная причина — каталог данных на файловой системе без прав Unix: `smbd` завершается с `invalid permissions on directory '.../smb/locks/msg.lock': has 0777 should be 0755`, а на порту 445 вместо него отвечает системная Samba. Перенесите каталог данных на ext4 или XFS, остановите другую Samba и перезапустите Bootimus.

### `net use fails after VM reboot`

Исправлено в 0.1.58 — в прилагаемый конфиг Samba добавлено `reset on zero vc = yes`. Если вы держите свой `smb.conf`, добавьте:

```
reset on zero vc = yes
oplocks = no
kernel oplocks = no
level2 oplocks = no
strict locking = no
deadtime = 1
```

## Дальше

- См. [Управление образами](images.md) — как прикреплять файлы к образам.
- См. [Управление клиентами](clients.md) — переопределения для конкретных клиентов.
- См. [Профили дистрибутивов](distro-profiles.md) — какие ID профилей мапятся на подкаталоги библиотеки.
