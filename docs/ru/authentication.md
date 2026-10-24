# Руководство по аутентификации

Bootimus использует JWT (JSON Web Token) для аутентификации в админ-панели. По желанию можно подключить LDAP- или Active Directory-сервер либо провайдер идентификации OpenID Connect (OIDC), например Authentik или Keycloak, для единого входа (SSO).

## Оглавление

- [Локальная аутентификация](#локальная-аутентификация)
- [Процесс входа](#процесс-входа)
- [Аутентификация API](#аутентификация-api)
- [LDAP / Active Directory](#ldap--active-directory)
- [OIDC / единый вход (SSO)](#oidc--единый-вход-sso)
- [Пользователи из LDAP и OIDC](#пользователи-из-ldap-и-oidc)
- [Справочник по конфигурации](#справочник-по-конфигурации)
- [Диагностика](#диагностика)

## Локальная аутентификация

По умолчанию Bootimus использует локальные учётные записи, хранящиеся в базе данных (SQLite или PostgreSQL).

### Учётка администратора по умолчанию

При первом запуске генерируется случайный пароль и печатается в логи сервера:

```
╔════════════════════════════════════════════════════════════════╗
║                    ADMIN PASSWORD GENERATED                    ║
╠════════════════════════════════════════════════════════════════╣
║  Username: admin                                               ║
║  Password: AbCdEfGh1234567890-_XyZ123456                       ║
╠════════════════════════════════════════════════════════════════╣
║  This password will NOT be shown again!                        ║
║  Save it now or reset it using --reset-admin-password flag     ║
╚════════════════════════════════════════════════════════════════╝
```

### Сброс пароля администратора

Сбросить на новый случайный пароль (запускает полноценный экземпляр сервера):
```bash
./bootimus serve --reset-admin-password
# или через Docker
docker exec bootimus /bootimus serve --reset-admin-password
```

Либо задать конкретный пароль напрямую в базе данных, не привязываясь к портам
(идеально для аварийного восстановления):
```bash
# Интерактивный ввод (скрытый ввод, пароль не попадает в историю shell)
./bootimus user set-password admin

# Или передать неинтерактивно для скриптов
./bootimus user set-password admin --password 'новый-пароль'
```

### Управление пользователями

Дополнительных пользователей можно создать во вкладке **Users** админ-панели. У каждого пользователя есть:
- **Username**: уникальное имя для входа
- **Password**: хранится как bcrypt-хеш
- **Admin**: есть ли у пользователя права администратора
- **Enabled**: можно отключить без удаления
- **Source**: `Local`, `LDAP` или `OIDC` — откуда взята учётная запись (см. [Пользователи из LDAP и OIDC](#пользователи-из-ldap-и-oidc))

Пользователями также можно управлять из CLI, не запуская сервер (удобно для
восстановления). Эти команды работают напрямую с настроенной базой данных
(SQLite или PostgreSQL):
```bash
./bootimus user list                       # список всех пользователей и их источник
./bootimus user enable <username>          # включить учётную запись
./bootimus user disable <username>         # отключить учётную запись
./bootimus user set-admin <username>       # выдать права администратора
./bootimus user unset-admin <username>     # снять права администратора
./bootimus user set-password <username>    # задать пароль (запрос ввода или --password)
```

## Процесс входа

1. Откройте `http://your-server:8081`
2. Отображается страница входа с полями имени пользователя и пароля
3. Если настроен LDAP, появляется выпадающий список для выбора бэкенда
4. Если настроен OIDC, под формой появляется кнопка **Sign in with …**
5. При успешном входе выдаётся JWT-токен (действует 24 часа)
6. Токен сохраняется в браузере и отправляется со всеми API-запросами
7. При выходе или истечении токена снова показывается страница входа

## Аутентификация API

Все API-эндпоинты (кроме `/api/login`, `/api/auth-info` и эндпоинтов входа `/api/oidc/`) требуют действительный JWT-токен.

### Получить токен

```bash
# Логин и получение токена
TOKEN=$(curl -s -X POST http://localhost:8081/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"your-password"}' | jq -r '.data.token')

echo $TOKEN
```

### Использовать токен

```bash
# Включайте в каждый API-запрос
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/clients

# Пример: список образов
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/images
```

### Детали токена

- **Алгоритм**: HMAC-SHA256
- **Срок жизни**: 24 часа с момента выдачи
- **Секрет**: генерируется случайно при каждом запуске сервера (все токены инвалидируются при перезапуске)
- **Claims**: имя пользователя, статус администратора, время выдачи, срок истечения

### Проверка доступных auth-бэкендов

```bash
# Аутентификация не требуется
curl http://localhost:8081/api/auth-info
```

Ответ:
```json
{
  "success": true,
  "data": [
    {"id": "local", "name": "Local"},
    {"id": "ldap", "name": "LDAP (dc.example.com)"},
    {"id": "oidc", "name": "Authentik", "type": "redirect", "url": "/api/oidc/login"}
  ]
}
```

## LDAP / Active Directory

Bootimus поддерживает LDAP как дополнительный бэкенд аутентификации. Когда он настроен, пользователи могут выбрать на странице входа между локальной и LDAP-аутентификацией. Локальные учётки всегда работают как резерв.

### Как это работает

1. Пользователь выбирает «LDAP» на странице входа и вводит креды
2. Bootimus подключается к LDAP-серверу через сервисную учётку (bind DN)
3. Ищет пользователя по заданному фильтру
4. Пытается забиндиться от имени найденного пользователя с указанным паролем
5. При успехе проверяет членство в группе для админ-доступа
6. Создаёт или обновляет для него локальную запись пользователя (см. [Пользователи из LDAP и OIDC](#пользователи-из-ldap-и-oidc))
7. Выдаёт JWT-токен (так же, как при локальной аутентификации)

### Пример Active Directory

```bash
# Переменные окружения
export BOOTIMUS_LDAP_HOST=dc.example.com
export BOOTIMUS_LDAP_BASE_DN="dc=example,dc=com"
export BOOTIMUS_LDAP_BIND_DN="cn=svc-bootimus,ou=Service Accounts,dc=example,dc=com"
export BOOTIMUS_LDAP_BIND_PASSWORD="service-account-password"
export BOOTIMUS_LDAP_USER_FILTER="(sAMAccountName=%s)"
export BOOTIMUS_LDAP_GROUP_FILTER="cn=bootimus-admins"
```

### Пример OpenLDAP

```bash
export BOOTIMUS_LDAP_HOST=ldap.example.com
export BOOTIMUS_LDAP_BASE_DN="dc=example,dc=com"
export BOOTIMUS_LDAP_BIND_DN="cn=readonly,dc=example,dc=com"
export BOOTIMUS_LDAP_BIND_PASSWORD="readonly-password"
export BOOTIMUS_LDAP_USER_FILTER="(uid=%s)"
```

### LDAPS (TLS)

```bash
export BOOTIMUS_LDAP_HOST=ldaps.example.com
export BOOTIMUS_LDAP_PORT=636
export BOOTIMUS_LDAP_TLS=true

# Или StartTLS на порту 389
export BOOTIMUS_LDAP_HOST=ldap.example.com
export BOOTIMUS_LDAP_STARTTLS=true

# Пропустить проверку сертификата (не рекомендуется для продакшена)
export BOOTIMUS_LDAP_SKIP_VERIFY=true
```

### Пример Docker Compose

```yaml
services:
  bootimus:
    image: garybowers/bootimus:latest
    environment:
      BOOTIMUS_LDAP_HOST: dc.example.com
      BOOTIMUS_LDAP_BASE_DN: dc=example,dc=com
      BOOTIMUS_LDAP_BIND_DN: cn=svc-bootimus,ou=Service Accounts,dc=example,dc=com
      BOOTIMUS_LDAP_BIND_PASSWORD: service-account-password
      BOOTIMUS_LDAP_USER_FILTER: (sAMAccountName=%s)
      BOOTIMUS_LDAP_GROUP_FILTER: cn=bootimus-admins
```

### Членство в админ-группе

Если задан `BOOTIMUS_LDAP_GROUP_FILTER`, права администратора получают только пользователи — члены подходящей группы. Членство проверяется через:

1. Атрибут `memberOf` на объекте пользователя
2. Запрос поиска по группе, если `memberOf` недоступен

Если `BOOTIMUS_LDAP_GROUP_FILTER` **не задан**, все LDAP-пользователи получают права администратора.

Права администратора пересчитываются при каждом входе, поэтому если убрать человека из группы, он потеряет их при следующем входе.

### Логин через API с LDAP

```bash
# Укажите auth_method: "ldap"
TOKEN=$(curl -s -X POST http://localhost:8081/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"jdoe","password":"ldap-password","auth_method":"ldap"}' | jq -r '.data.token')
```

## OIDC / единый вход (SSO)

Bootimus может авторизовать пользователей через любой провайдер OpenID Connect — Authentik, Keycloak, Zitadel, Authelia, Entra ID, Google Workspace и другие. Пользователь нажимает **Sign in with …** на странице входа, проходит аутентификацию у провайдера и возвращается в Bootimus уже авторизованным. Учётные записи создаются автоматически при первом входе, а права администратора определяются группами пользователя у провайдера.

### Как это работает

1. Пользователь нажимает **Sign in with …** на странице входа
2. Bootimus перенаправляет его к провайдеру (поток authorisation code с PKCE)
3. Пользователь входит у провайдера, который перенаправляет обратно на `/api/oidc/callback`
4. Bootimus обменивает код на ID-токен, проверяет его подпись, audience, срок действия и nonce и считывает claims с именем пользователя и группами
5. Если заданы разрешённые группы, пользователь должен состоять в одной из них (или в админ-группе)
6. Создаётся или обновляется локальная запись пользователя (см. [Пользователи из LDAP и OIDC](#пользователи-из-ldap-и-oidc)); права администратора выдаются, если пользователь состоит в админ-группе
7. Браузер возвращается в админ-панель с JWT-токеном (так же, как при локальной аутентификации)

Если в ID-токене нет claim с именем пользователя или группами, Bootimus дополнительно обращается к userinfo-эндпоинту провайдера.

### Настройка провайдера

Создайте у провайдера OIDC-клиент (его иногда называют приложением или relying party) со следующими параметрами:

| Параметр | Значение |
|---------|-------|
| Тип клиента | Confidential (с client secret) |
| Тип гранта | Authorisation code |
| Redirect URI | `https://bootimus.example.com/api/oidc/callback` — URL, по которому пользователи открывают админ-панель, плюс `/api/oidc/callback` |
| Scopes | `openid`, `profile`, `email` |

Убедитесь, что провайдер передаёт группы пользователя в claim `groups`. Bootimus не пытается угадать собственный публичный URL, поэтому `BOOTIMUS_OIDC_REDIRECT_URL` должен в точности совпадать с redirect URI, зарегистрированным у провайдера.

### Пример Authentik

1. В Authentik откройте **Applications → Providers** и создайте **OAuth2/OpenID Provider**:
   - **Name**: `bootimus`
   - **Authorisation flow**: ваш обычный explicit- или implicit-consent flow
   - **Client type**: Confidential
   - **Redirect URIs/Origins**: `https://bootimus.example.com/api/oidc/callback` (strict)
   - **Scopes**: оставьте стандартные маппинги `openid`, `email` и `profile` — маппинг `profile` в Authentik включает claim `groups`
   - **Subject mode**: любой; по умолчанию Bootimus использует `preferred_username` как имя пользователя
2. Запишите **Client ID** и **Client Secret**
3. Откройте **Applications → Applications** и создайте приложение со slug `bootimus`, использующее этот провайдер. При желании привяжите к нему политику или группу, чтобы ограничить, кто вообще может его запускать.
4. Создайте группы для администраторов Bootimus (например, `bootimus-admins`) и, при необходимости, для остальных допущенных пользователей (например, `bootimus-operators`)
5. Настройте Bootimus:

```bash
export BOOTIMUS_OIDC_ISSUER=https://authentik.example.com/application/o/bootimus/
export BOOTIMUS_OIDC_CLIENT_ID=<client id>
export BOOTIMUS_OIDC_CLIENT_SECRET=<client secret>
export BOOTIMUS_OIDC_REDIRECT_URL=https://bootimus.example.com/api/oidc/callback
export BOOTIMUS_OIDC_ADMIN_GROUPS=bootimus-admins
export BOOTIMUS_OIDC_ALLOWED_GROUPS=bootimus-operators
export BOOTIMUS_OIDC_DISPLAY_NAME=Authentik
```

URL издателя (issuer) показан на странице обзора провайдера в Authentik как **OpenID Configuration Issuer** и всегда заканчивается на `/application/o/<slug приложения>/`.

### Пример Keycloak

Создайте confidential-клиент OpenID Connect с включённым **Standard flow** и указанным выше redirect URI. Keycloak по умолчанию не передаёт группы: добавьте маппер **Group Membership** в dedicated scope клиента с именем token claim `groups` и выключенной опцией **Full group path**.

```bash
export BOOTIMUS_OIDC_ISSUER=https://keycloak.example.com/realms/example
export BOOTIMUS_OIDC_CLIENT_ID=bootimus
export BOOTIMUS_OIDC_CLIENT_SECRET=<client secret>
export BOOTIMUS_OIDC_REDIRECT_URL=https://bootimus.example.com/api/oidc/callback
export BOOTIMUS_OIDC_ADMIN_GROUPS=bootimus-admins
export BOOTIMUS_OIDC_DISPLAY_NAME=Keycloak
```

### Сопоставление групп

| Параметр | Эффект |
|---------|--------|
| `BOOTIMUS_OIDC_ADMIN_GROUPS` | Члены любой из этих групп — администраторы. Если параметр пуст, ни один OIDC-пользователь не становится администратором. |
| `BOOTIMUS_OIDC_ALLOWED_GROUPS` | Если задан, войти могут только члены этих групп (или админ-группы). Если пуст, войти может любой, кого аутентифицировал провайдер. |

Оба параметра принимают список через запятую, поэтому названия групп могут содержать пробелы (`Bootimus Admins,Infrastructure`). Регистр при сравнении не учитывается. Членство в группах перечитывается при каждом входе, поэтому изменения у провайдера применяются при следующем входе пользователя.

> **Примечание:** сейчас для любой страницы админ-панели нужны права администратора. Пользователи, не состоящие в админ-группе, могут войти и отображаются в разделе **Users**, но ничего не видят и не могут менять, пока не появится роль только для чтения. Пока что, если хотите не допускать создания таких учётных записей, задайте в `BOOTIMUS_OIDC_ALLOWED_GROUPS` те же группы, что и в `BOOTIMUS_OIDC_ADMIN_GROUPS`.

### Пример Docker Compose

```yaml
services:
  bootimus:
    image: garybowers/bootimus:latest
    environment:
      BOOTIMUS_OIDC_ISSUER: https://authentik.example.com/application/o/bootimus/
      BOOTIMUS_OIDC_CLIENT_ID: bootimus
      BOOTIMUS_OIDC_CLIENT_SECRET: client-secret
      BOOTIMUS_OIDC_REDIRECT_URL: https://bootimus.example.com/api/oidc/callback
      BOOTIMUS_OIDC_ADMIN_GROUPS: bootimus-admins
      BOOTIMUS_OIDC_DISPLAY_NAME: Authentik
```

### Заметки по безопасности

- Состояние входа, nonce и PKCE verifier хранятся в подписанной короткоживущей (10 минут) cookie `HttpOnly` с областью `/api/oidc/`. Она помечается как `Secure`, если redirect URL использует HTTPS — в продакшене отдавайте админ-панель по HTTPS.
- Используется только поток authorisation code; токены провайдера в браузер не попадают. Браузер получает обычный JWT Bootimus.
- Локальный вход по паролю продолжает работать вместе с OIDC, поэтому локальная учётка `admin` остаётся доступной для восстановления.

## Пользователи из LDAP и OIDC

Пользователи, входящие через LDAP или OIDC, получают локальную запись при первом входе. Благодаря этому их можно увидеть в разделе **Users**, отключить, и их доступ проверяется при каждом запросе так же, как у локальных пользователей.

- **Source**: каждый пользователь помечен как `Local`, `LDAP` или `OIDC`. Пользователь может входить только через свой источник: вход через OIDC или LDAP никогда не перехватит локальную учётку с тем же именем (например, `admin`), а пользователи LDAP и OIDC не могут входить по локальному паролю.
- **Имена пользователей** приводятся к нижнему регистру и могут содержать буквы, цифры и символы `. _ @ + -`. Входы с другими символами отклоняются.
- **Права администратора** определяются группами пользователя и обновляются при каждом входе, поэтому изменение флажка **Administrator** для пользователя LDAP или OIDC действует только до его следующего входа.
- **Отключение** пользователя в Bootimus блокирует его немедленно, даже если провайдер по-прежнему разрешает ему вход.
- **Удаление** пользователя убирает только локальную запись; при следующем входе она создаётся заново. Чтобы отозвать доступ, используйте группы провайдера или отключите пользователя.
- **Паролями** пользователей LDAP и OIDC управляет провайдер, поэтому для них не предлагается **Reset Password**, а `bootimus user set-password` отказывается с ними работать.

Если кто-то входит под именем, которое уже принадлежит пользователю из другого источника, вход отклоняется с сообщением *a user with this name already exists from a different source*. Это касается и локальных учёток, созданных до того, как пользователи LDAP стали создаваться автоматически. Удалите старую локальную учётку, чтобы при следующем входе она была создана заново из LDAP или OIDC.

## Справочник по конфигурации

### Флаги CLI

| Флаг | По умолчанию | Описание |
|------|---------|-------------|
| `--ldap-host` | *(пусто)* | Хост LDAP-сервера (включает LDAP-аутентификацию) |
| `--ldap-port` | `389` | Порт LDAP-сервера |
| `--ldap-tls` | `false` | Использовать LDAPS (TLS при подключении) |
| `--ldap-starttls` | `false` | Использовать StartTLS после подключения |
| `--ldap-skip-verify` | `false` | Пропустить проверку TLS-сертификата |
| `--ldap-bind-dn` | *(пусто)* | DN сервисной учётки для поиска пользователей |
| `--ldap-bind-password` | *(пусто)* | Пароль сервисной учётки |
| `--ldap-base-dn` | *(пусто)* | Base DN для поиска пользователей |
| `--ldap-user-filter` | `(sAMAccountName=%s)` | Фильтр поиска пользователей (`%s` = имя) |
| `--ldap-group-filter` | *(пусто)* | CN группы для админ-доступа |
| `--ldap-group-base-dn` | *(пусто)* | Base DN для поиска групп (по умолчанию — base DN) |
| `--oidc-issuer` | *(пусто)* | URL издателя OIDC (включает вход через OIDC) |
| `--oidc-client-id` | *(пусто)* | Client ID OIDC |
| `--oidc-client-secret` | *(пусто)* | Client secret OIDC |
| `--oidc-redirect-url` | *(пусто)* | Redirect URL, зарегистрированный у провайдера, оканчивается на `/api/oidc/callback` |
| `--oidc-scopes` | `openid,profile,email` | Запрашиваемые scopes через запятую (`openid` включается всегда) |
| `--oidc-username-claim` | `preferred_username` | Claim, используемый как имя пользователя Bootimus (например, `email`) |
| `--oidc-groups-claim` | `groups` | Claim со списком групп пользователя |
| `--oidc-admin-groups` | *(пусто)* | Группы через запятую, члены которых — администраторы |
| `--oidc-allowed-groups` | *(пусто)* | Группы через запятую, которым разрешён вход (пусто — разрешён любому) |
| `--oidc-display-name` | `SSO` | Название на кнопке входа |
| `--oidc-skip-verify` | `false` | Пропустить проверку TLS-сертификата при обращении к провайдеру |

### Переменные окружения

Все флаги задаются через переменные окружения с префиксом `BOOTIMUS_`:

| Переменная | Соответствует |
|----------|---------|
| `BOOTIMUS_LDAP_HOST` | `--ldap-host` |
| `BOOTIMUS_LDAP_PORT` | `--ldap-port` |
| `BOOTIMUS_LDAP_TLS` | `--ldap-tls` |
| `BOOTIMUS_LDAP_STARTTLS` | `--ldap-starttls` |
| `BOOTIMUS_LDAP_SKIP_VERIFY` | `--ldap-skip-verify` |
| `BOOTIMUS_LDAP_BIND_DN` | `--ldap-bind-dn` |
| `BOOTIMUS_LDAP_BIND_PASSWORD` | `--ldap-bind-password` |
| `BOOTIMUS_LDAP_BASE_DN` | `--ldap-base-dn` |
| `BOOTIMUS_LDAP_USER_FILTER` | `--ldap-user-filter` |
| `BOOTIMUS_LDAP_GROUP_FILTER` | `--ldap-group-filter` |
| `BOOTIMUS_LDAP_GROUP_BASE_DN` | `--ldap-group-base-dn` |
| `BOOTIMUS_OIDC_ISSUER` | `--oidc-issuer` |
| `BOOTIMUS_OIDC_CLIENT_ID` | `--oidc-client-id` |
| `BOOTIMUS_OIDC_CLIENT_SECRET` | `--oidc-client-secret` |
| `BOOTIMUS_OIDC_REDIRECT_URL` | `--oidc-redirect-url` |
| `BOOTIMUS_OIDC_SCOPES` | `--oidc-scopes` |
| `BOOTIMUS_OIDC_USERNAME_CLAIM` | `--oidc-username-claim` |
| `BOOTIMUS_OIDC_GROUPS_CLAIM` | `--oidc-groups-claim` |
| `BOOTIMUS_OIDC_ADMIN_GROUPS` | `--oidc-admin-groups` |
| `BOOTIMUS_OIDC_ALLOWED_GROUPS` | `--oidc-allowed-groups` |
| `BOOTIMUS_OIDC_DISPLAY_NAME` | `--oidc-display-name` |
| `BOOTIMUS_OIDC_SKIP_VERIFY` | `--oidc-skip-verify` |

### Файл конфигурации (bootimus.yaml)

```yaml
ldap:
  host: dc.example.com
  port: 389
  tls: false
  starttls: true
  bind_dn: cn=svc-bootimus,ou=Service Accounts,dc=example,dc=com
  bind_password: service-account-password
  base_dn: dc=example,dc=com
  user_filter: (sAMAccountName=%s)
  group_filter: cn=bootimus-admins

oidc:
  issuer: https://authentik.example.com/application/o/bootimus/
  client_id: bootimus
  client_secret: client-secret
  redirect_url: https://bootimus.example.com/api/oidc/callback
  admin_groups: bootimus-admins
  allowed_groups: bootimus-operators
  display_name: Authentik
```

## Диагностика

### Сбой подключения к LDAP

Проверьте связность и настройки TLS:
```bash
# Тест подключения LDAP
ldapsearch -H ldap://dc.example.com -D "cn=svc-bootimus,dc=example,dc=com" -w password -b "dc=example,dc=com" "(sAMAccountName=testuser)"

# Тест LDAPS
ldapsearch -H ldaps://dc.example.com:636 -D "cn=svc-bootimus,dc=example,dc=com" -w password -b "dc=example,dc=com" "(sAMAccountName=testuser)"
```

### Пользователь не найден

Проверьте, что фильтр возвращает результаты:
```bash
ldapsearch -H ldap://dc.example.com -D "bind-dn" -w password \
  -b "dc=example,dc=com" "(sAMAccountName=testuser)" dn
```

Типичные фильтры:
- Active Directory: `(sAMAccountName=%s)`
- OpenLDAP: `(uid=%s)`
- По email: `(mail=%s)`

### LDAP-пользователь не админ

Проверьте членство в группе:
```bash
ldapsearch -H ldap://dc.example.com -D "bind-dn" -w password \
  -b "dc=example,dc=com" "(sAMAccountName=testuser)" memberOf
```

### OIDC: «Unable to reach the identity provider»

Bootimus запрашивает `<issuer>/.well-known/openid-configuration`, когда первый пользователь входит в систему. Посмотрите точный текст ошибки в логе сервера и проверьте, что хост Bootimus может обратиться к издателю:

```bash
curl https://authentik.example.com/application/o/bootimus/.well-known/openid-configuration
```

Значение `issuer` в этом документе должно в точности совпадать с `BOOTIMUS_OIDC_ISSUER`, включая завершающий слэш. Если провайдер использует приватный CA, добавьте его в хранилище доверенных сертификатов хоста; `BOOTIMUS_OIDC_SKIP_VERIFY=true` подходит для тестирования, но не рекомендуется.

### OIDC: провайдер показывает ошибку redirect URI

Redirect URI, зарегистрированный у провайдера, и `BOOTIMUS_OIDC_REDIRECT_URL` должны совпадать в точности — схема, хост, порт и путь.

### OIDC: «Sign-in expired or was started elsewhere»

Вход должен завершиться в течение 10 минут, в том же браузере, где он был начат, и Bootimus не должен перезапускаться за это время. Если ошибка повторяется за обратным прокси, убедитесь, что прокси передаёт cookies и что пользователи открывают Bootimus по тому же хосту, что указан в redirect URL.

### OIDC: пользователь не администратор или ему отказано во входе

В логе сервера записана причина каждого отказа во входе. Проверьте, какие группы провайдер реально передаёт — названия групп должны совпадать с `BOOTIMUS_OIDC_ADMIN_GROUPS` или `BOOTIMUS_OIDC_ALLOWED_GROUPS`, а Keycloak присылает полные пути (`/bootimus-admins`), если не выключить **Full group path**. Если провайдер использует другое имя claim, задайте `BOOTIMUS_OIDC_GROUPS_CLAIM`.

### «A user with this name already exists from a different source»

См. [Пользователи из LDAP и OIDC](#пользователи-из-ldap-и-oidc). Удалите существующую учётную запись в разделе **Users** и войдите снова.

### Токен истёк

JWT-токены действительны 24 часа. После истечения автоматически показывается страница входа. Токены также инвалидируются при перезапуске сервера (секрет подписи генерируется заново).

### Локальный админ заблокирован

Сбросьте пароль администратора:
```bash
./bootimus serve --reset-admin-password
```

Либо, если запускать сервер неудобно (например, из-за конфликтов портов), задайте
пароль напрямую в базе данных:
```bash
./bootimus user set-password admin
```

Это всегда работает, независимо от конфигурации LDAP или OIDC.
