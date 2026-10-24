# Guide d'authentification

Bootimus utilise l'authentification JWT (JSON Web Token) pour le panneau d'administration. Tu peux également connecter un serveur LDAP ou Active Directory, ou un fournisseur d'identité OpenID Connect (OIDC) comme Authentik ou Keycloak, pour l'authentification unique (SSO).

## Table des matières

- [Authentification locale](#authentification-locale)
- [Flux de connexion](#flux-de-connexion)
- [Authentification de l'API](#authentification-de-lapi)
- [LDAP / Active Directory](#ldap--active-directory)
- [OIDC / authentification unique](#oidc--authentification-unique)
- [Utilisateurs LDAP et OIDC](#utilisateurs-ldap-et-oidc)
- [Référence de configuration](#référence-de-configuration)
- [Dépannage](#dépannage)

## Authentification locale

Par défaut, Bootimus utilise des comptes utilisateurs locaux stockés en base (SQLite ou PostgreSQL).

### Compte admin par défaut

Au premier démarrage, un mot de passe aléatoire est généré et affiché dans les logs du serveur :

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

### Réinitialiser le mot de passe admin

Réinitialiser vers un nouveau mot de passe aléatoire (démarre une instance complète du serveur) :
```bash
./bootimus serve --reset-admin-password
# ou avec Docker
docker exec bootimus /bootimus serve --reset-admin-password
```

Ou définir un mot de passe précis directement dans la base de données, sans
ouvrir de port (idéal pour la récupération d'urgence) :
```bash
# Saisie interactive (entrée masquée, garde le mot de passe hors de l'historique du shell)
./bootimus user set-password admin

# Ou le fournir de façon non interactive pour les scripts
./bootimus user set-password admin --password 'nouveau-mot-de-passe'
```

### Gestion des utilisateurs

Des utilisateurs supplémentaires peuvent être créés depuis l'onglet **Users** du panneau d'administration. Chaque utilisateur a :
- **Username** : nom de login unique
- **Password** : stocké en hash bcrypt
- **Admin** : si l'utilisateur a les privilèges admin
- **Enabled** : peut être désactivé sans suppression
- **Source** : `Local`, `LDAP` ou `OIDC` — d'où vient le compte (voir [Utilisateurs LDAP et OIDC](#utilisateurs-ldap-et-oidc))

Les utilisateurs peuvent aussi être gérés depuis la CLI sans démarrer le serveur
(utile pour la récupération). Ces commandes agissent directement sur la base de
données configurée (SQLite ou PostgreSQL) :
```bash
./bootimus user list                       # lister tous les utilisateurs et leur source
./bootimus user enable <username>          # activer un compte
./bootimus user disable <username>         # désactiver un compte
./bootimus user set-admin <username>       # accorder les droits admin
./bootimus user unset-admin <username>     # retirer les droits admin
./bootimus user set-password <username>    # définir un mot de passe (saisie interactive, ou --password)
```

## Flux de connexion

1. Va sur `http://your-server:8081`
2. La page de login s'affiche avec les champs nom d'utilisateur et mot de passe
3. Si LDAP est configuré, un menu déroulant d'authentification apparaît pour sélectionner le backend
4. Si OIDC est configuré, un bouton **Sign in with …** apparaît sous le formulaire
5. À la connexion réussie, un token JWT est émis (valable 24 heures)
6. Le token est stocké dans le navigateur et envoyé avec toutes les requêtes API
7. À la déconnexion ou à l'expiration du token, la page de login est réaffichée

## Authentification de l'API

Tous les endpoints de l'API (sauf `/api/login`, `/api/auth-info` et les endpoints de connexion `/api/oidc/`) requièrent un token JWT valide.

### Obtenir un token

```bash
# Login and get token
TOKEN=$(curl -s -X POST http://localhost:8081/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"your-password"}' | jq -r '.data.token')

echo $TOKEN
```

### Utiliser le token

```bash
# Include in all API requests
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/clients

# Example: list images
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/images
```

### Détails du token

- **Algorithme** : HMAC-SHA256
- **Expiration** : 24 heures à partir de l'émission
- **Secret** : généré aléatoirement à chaque démarrage du serveur (tous les tokens sont invalidés au redémarrage)
- **Claims** : nom d'utilisateur, statut admin, heure d'émission, expiration

### Vérifier les backends d'authentification disponibles

```bash
# No authentication required
curl http://localhost:8081/api/auth-info
```

Réponse :
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

Bootimus supporte l'authentification LDAP comme backend additionnel. Une fois configuré, les utilisateurs peuvent choisir entre l'authentification locale et LDAP sur la page de login. Les comptes locaux fonctionnent toujours en fallback.

### Fonctionnement

1. L'utilisateur sélectionne « LDAP » sur la page de login et entre ses identifiants
2. Bootimus se connecte au serveur LDAP avec le compte de service (bind DN)
3. Recherche l'utilisateur avec le filtre configuré
4. Tente un bind en tant qu'utilisateur trouvé avec le mot de passe fourni
5. En cas de succès, vérifie l'appartenance au groupe pour l'accès admin
6. Crée ou met à jour un enregistrement utilisateur local pour lui (voir [Utilisateurs LDAP et OIDC](#utilisateurs-ldap-et-oidc))
7. Émet un token JWT (comme pour l'auth locale)

### Exemple Active Directory

```bash
# Environment variables
export BOOTIMUS_LDAP_HOST=dc.example.com
export BOOTIMUS_LDAP_BASE_DN="dc=example,dc=com"
export BOOTIMUS_LDAP_BIND_DN="cn=svc-bootimus,ou=Service Accounts,dc=example,dc=com"
export BOOTIMUS_LDAP_BIND_PASSWORD="service-account-password"
export BOOTIMUS_LDAP_USER_FILTER="(sAMAccountName=%s)"
export BOOTIMUS_LDAP_GROUP_FILTER="cn=bootimus-admins"
```

### Exemple OpenLDAP

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

# Or use StartTLS on port 389
export BOOTIMUS_LDAP_HOST=ldap.example.com
export BOOTIMUS_LDAP_STARTTLS=true

# Skip certificate verification (not recommended for production)
export BOOTIMUS_LDAP_SKIP_VERIFY=true
```

### Exemple Docker Compose

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

### Appartenance au groupe admin

Si `BOOTIMUS_LDAP_GROUP_FILTER` est défini, seuls les utilisateurs membres du groupe correspondant obtiennent l'accès admin. L'appartenance au groupe est vérifiée via :

1. L'attribut `memberOf` sur l'objet utilisateur
2. Une requête de recherche de groupe si `memberOf` n'est pas disponible

Si `BOOTIMUS_LDAP_GROUP_FILTER` n'est **pas défini**, tous les utilisateurs LDAP obtiennent l'accès admin.

Les droits admin sont réévalués à chaque connexion : retirer quelqu'un du groupe le rétrograde à sa prochaine connexion.

### Login via l'API avec LDAP

```bash
# Specify auth_method: "ldap"
TOKEN=$(curl -s -X POST http://localhost:8081/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"jdoe","password":"ldap-password","auth_method":"ldap"}' | jq -r '.data.token')
```

## OIDC / authentification unique

Bootimus peut connecter les utilisateurs via n'importe quel fournisseur OpenID Connect — Authentik, Keycloak, Zitadel, Authelia, Entra ID, Google Workspace et d'autres. Les utilisateurs cliquent sur **Sign in with …** sur la page de login, s'authentifient auprès du fournisseur, puis sont renvoyés vers Bootimus déjà connectés. Les comptes sont créés automatiquement à la première connexion, et les droits admin suivent les groupes de l'utilisateur chez le fournisseur.

### Fonctionnement

1. L'utilisateur clique sur **Sign in with …** sur la page de login
2. Bootimus le redirige vers le fournisseur (flux authorization code avec PKCE)
3. L'utilisateur s'authentifie chez le fournisseur, qui le redirige vers `/api/oidc/callback`
4. Bootimus échange le code contre un ID token, vérifie sa signature, son audience, son expiration et son nonce, puis lit les claims du nom d'utilisateur et des groupes
5. Si des groupes autorisés sont configurés, l'utilisateur doit appartenir à l'un d'eux (ou à un groupe admin)
6. Un enregistrement utilisateur local est créé ou mis à jour (voir [Utilisateurs LDAP et OIDC](#utilisateurs-ldap-et-oidc)), avec les droits admin s'il appartient à un groupe admin
7. Le navigateur est renvoyé vers le panneau d'administration avec un token JWT (comme pour l'auth locale)

Si l'ID token ne contient pas le claim du nom d'utilisateur ou des groupes, Bootimus interroge aussi l'endpoint userinfo du fournisseur.

### Configuration du fournisseur

Crée un client OIDC (parfois appelé application ou relying party) chez ton fournisseur avec :

| Paramètre | Valeur |
|---------|-------|
| Type de client | Confidentiel (avec un client secret) |
| Grant type | Authorization code |
| Redirect URI | `https://bootimus.example.com/api/oidc/callback` — l'URL par laquelle les utilisateurs accèdent au panneau d'administration, plus `/api/oidc/callback` |
| Scopes | `openid`, `profile`, `email` |

Assure-toi que le fournisseur inclut les groupes de l'utilisateur dans un claim `groups`. Bootimus ne devine pas sa propre URL publique : `BOOTIMUS_OIDC_REDIRECT_URL` doit donc correspondre exactement à la redirect URI enregistrée chez le fournisseur.

### Exemple Authentik

1. Dans Authentik, va dans **Applications → Providers** et crée un **OAuth2/OpenID Provider** :
   - **Name** : `bootimus`
   - **Authorisation flow** : ton flux de consentement explicite ou implicite habituel
   - **Client type** : Confidential
   - **Redirect URIs/Origins** : `https://bootimus.example.com/api/oidc/callback` (strict)
   - **Scopes** : garde les mappings par défaut `openid`, `email` et `profile` — le mapping `profile` d'Authentik inclut le claim `groups`
   - **Subject mode** : au choix ; Bootimus utilise `preferred_username` comme nom d'utilisateur par défaut
2. Note le **Client ID** et le **Client Secret**
3. Va dans **Applications → Applications** et crée une application avec le slug `bootimus` utilisant ce provider. Tu peux éventuellement lui associer une policy ou un groupe pour contrôler qui peut la lancer.
4. Crée des groupes pour les administrateurs de Bootimus (ex. `bootimus-admins`) et, éventuellement, pour les autres utilisateurs autorisés (ex. `bootimus-operators`)
5. Configure Bootimus :

```bash
export BOOTIMUS_OIDC_ISSUER=https://authentik.example.com/application/o/bootimus/
export BOOTIMUS_OIDC_CLIENT_ID=<client id>
export BOOTIMUS_OIDC_CLIENT_SECRET=<client secret>
export BOOTIMUS_OIDC_REDIRECT_URL=https://bootimus.example.com/api/oidc/callback
export BOOTIMUS_OIDC_ADMIN_GROUPS=bootimus-admins
export BOOTIMUS_OIDC_ALLOWED_GROUPS=bootimus-operators
export BOOTIMUS_OIDC_DISPLAY_NAME=Authentik
```

L'URL de l'issuer est affichée sur la page de présentation du provider dans Authentik sous **OpenID Configuration Issuer**, et se termine toujours par `/application/o/<slug de l'application>/`.

### Exemple Keycloak

Crée un client OpenID Connect confidentiel avec **Standard flow** activé et la redirect URI ci-dessus. Keycloak n'envoie pas les groupes par défaut : ajoute un mapper **Group Membership** au scope dédié du client avec le nom de claim de token `groups` et **Full group path** désactivé.

```bash
export BOOTIMUS_OIDC_ISSUER=https://keycloak.example.com/realms/example
export BOOTIMUS_OIDC_CLIENT_ID=bootimus
export BOOTIMUS_OIDC_CLIENT_SECRET=<client secret>
export BOOTIMUS_OIDC_REDIRECT_URL=https://bootimus.example.com/api/oidc/callback
export BOOTIMUS_OIDC_ADMIN_GROUPS=bootimus-admins
export BOOTIMUS_OIDC_DISPLAY_NAME=Keycloak
```

### Mapping des groupes

| Paramètre | Effet |
|---------|--------|
| `BOOTIMUS_OIDC_ADMIN_GROUPS` | Les membres de l'un de ces groupes sont administrateurs. Si vide, aucun utilisateur OIDC n'est administrateur. |
| `BOOTIMUS_OIDC_ALLOWED_GROUPS` | Si défini, seuls les membres de ces groupes (ou d'un groupe admin) peuvent se connecter. Si vide, tout utilisateur authentifié par le fournisseur peut se connecter. |

Les deux acceptent une liste séparée par des virgules, donc les noms de groupes peuvent contenir des espaces (`Bootimus Admins,Infrastructure`). La comparaison ignore la casse. L'appartenance aux groupes est relue à chaque connexion : les changements chez le fournisseur s'appliquent à la prochaine connexion de l'utilisateur.

> **Note :** Actuellement, chaque page du panneau d'administration requiert les droits administrateur. Les utilisateurs qui ne sont pas dans un groupe admin peuvent se connecter et apparaissent dans **Users**, mais ne peuvent rien voir ni modifier tant qu'un rôle en lecture seule n'a pas été ajouté. Pour l'instant, définis `BOOTIMUS_OIDC_ALLOWED_GROUPS` avec les mêmes groupes que `BOOTIMUS_OIDC_ADMIN_GROUPS` si tu veux empêcher la création de tels comptes.

### Exemple Docker Compose

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

### Notes de sécurité

- Le state de connexion, le nonce et le verifier PKCE sont conservés dans un cookie `HttpOnly` signé et de courte durée (10 minutes), limité à `/api/oidc/`. Il est marqué `Secure` quand l'URL de redirection utilise HTTPS — sers le panneau d'administration en HTTPS en production.
- Seul le flux authorization code est utilisé ; les tokens du fournisseur n'atteignent jamais le navigateur. Le navigateur reçoit le JWT Bootimus habituel.
- La connexion locale par mot de passe continue de fonctionner à côté d'OIDC, donc le compte local `admin` reste disponible pour la récupération.

## Utilisateurs LDAP et OIDC

Les utilisateurs qui se connectent via LDAP ou OIDC obtiennent un enregistrement utilisateur local dès leur première connexion. Tu peux ainsi les voir dans **Users**, les désactiver, et faire vérifier leur accès à chaque requête comme pour les utilisateurs locaux.

- **Source** : chaque utilisateur est marqué `Local`, `LDAP` ou `OIDC`. Un utilisateur ne peut se connecter que via sa propre source : une connexion OIDC ou LDAP ne reprendra jamais un compte local du même nom (comme `admin`), et les utilisateurs LDAP et OIDC ne peuvent pas utiliser la connexion locale par mot de passe.
- **Les noms d'utilisateur** sont mis en minuscules et peuvent contenir des lettres, des chiffres et `. _ @ + -`. Les connexions avec d'autres caractères sont refusées.
- **Les droits admin** suivent les groupes de l'utilisateur et sont rafraîchis à chaque connexion : modifier la case **Administrator** d'un utilisateur LDAP ou OIDC ne dure donc que jusqu'à sa prochaine connexion.
- **Désactiver** un utilisateur dans Bootimus le bloque immédiatement, même si le fournisseur l'autorise encore à se connecter.
- **Supprimer** un utilisateur ne retire que l'enregistrement local ; il est recréé à sa prochaine connexion. Utilise les groupes du fournisseur, ou désactive l'utilisateur, pour retirer l'accès.
- **Les mots de passe** des utilisateurs LDAP et OIDC sont gérés par le fournisseur : **Reset Password** n'est donc pas proposé pour eux et `bootimus user set-password` les refuse.

Si quelqu'un se connecte avec un nom qui appartient déjà à un utilisateur d'une autre source, la connexion est refusée avec *a user with this name already exists from a different source*. Cela inclut les comptes locaux créés avant que les utilisateurs LDAP ne soient provisionnés automatiquement. Supprime l'ancien compte local pour qu'il soit recréé depuis LDAP ou OIDC à la prochaine connexion.

## Référence de configuration

### Flags CLI

| Flag | Défaut | Description |
|------|---------|-------------|
| `--ldap-host` | *(vide)* | Hostname du serveur LDAP (active l'auth LDAP) |
| `--ldap-port` | `389` | Port du serveur LDAP |
| `--ldap-tls` | `false` | Utiliser LDAPS (TLS à la connexion) |
| `--ldap-starttls` | `false` | Utiliser StartTLS après la connexion |
| `--ldap-skip-verify` | `false` | Sauter la vérification du certificat TLS |
| `--ldap-bind-dn` | *(vide)* | DN du compte de service pour la recherche utilisateur |
| `--ldap-bind-password` | *(vide)* | Mot de passe du compte de service |
| `--ldap-base-dn` | *(vide)* | Base DN pour la recherche utilisateur |
| `--ldap-user-filter` | `(sAMAccountName=%s)` | Filtre de recherche utilisateur (`%s` = username) |
| `--ldap-group-filter` | *(vide)* | CN du groupe pour l'accès admin |
| `--ldap-group-base-dn` | *(vide)* | Base DN pour la recherche de groupe (par défaut le base DN) |
| `--oidc-issuer` | *(vide)* | URL de l'issuer OIDC (active la connexion OIDC) |
| `--oidc-client-id` | *(vide)* | Client ID OIDC |
| `--oidc-client-secret` | *(vide)* | Client secret OIDC |
| `--oidc-redirect-url` | *(vide)* | URL de redirection enregistrée chez le fournisseur, se terminant par `/api/oidc/callback` |
| `--oidc-scopes` | `openid,profile,email` | Scopes à demander, séparés par des virgules (`openid` est toujours inclus) |
| `--oidc-username-claim` | `preferred_username` | Claim utilisé comme nom d'utilisateur Bootimus (ex. `email`) |
| `--oidc-groups-claim` | `groups` | Claim listant les groupes de l'utilisateur |
| `--oidc-admin-groups` | *(vide)* | Groupes, séparés par des virgules, dont les membres sont administrateurs |
| `--oidc-allowed-groups` | *(vide)* | Groupes, séparés par des virgules, autorisés à se connecter (vide autorise tout le monde) |
| `--oidc-display-name` | `SSO` | Nom affiché sur le bouton de connexion |
| `--oidc-skip-verify` | `false` | Sauter la vérification du certificat TLS lors des échanges avec le fournisseur |

### Variables d'environnement

Tous les flags peuvent être définis via des variables d'environnement avec le préfixe `BOOTIMUS_` :

| Variable | Correspond à |
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

### Fichier de config (bootimus.yaml)

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

## Dépannage

### Connexion LDAP échouée

Vérifie la connectivité et les paramètres TLS :
```bash
# Test LDAP connection
ldapsearch -H ldap://dc.example.com -D "cn=svc-bootimus,dc=example,dc=com" -w password -b "dc=example,dc=com" "(sAMAccountName=testuser)"

# Test LDAPS
ldapsearch -H ldaps://dc.example.com:636 -D "cn=svc-bootimus,dc=example,dc=com" -w password -b "dc=example,dc=com" "(sAMAccountName=testuser)"
```

### Utilisateur introuvable

Vérifie que le filtre utilisateur renvoie des résultats :
```bash
ldapsearch -H ldap://dc.example.com -D "bind-dn" -w password \
  -b "dc=example,dc=com" "(sAMAccountName=testuser)" dn
```

Filtres courants :
- Active Directory : `(sAMAccountName=%s)`
- OpenLDAP : `(uid=%s)`
- Basé sur l'email : `(mail=%s)`

### L'utilisateur LDAP n'est pas admin

Vérifie l'appartenance au groupe :
```bash
ldapsearch -H ldap://dc.example.com -D "bind-dn" -w password \
  -b "dc=example,dc=com" "(sAMAccountName=testuser)" memberOf
```

### OIDC : « Unable to reach the identity provider »

Bootimus récupère `<issuer>/.well-known/openid-configuration` quand le premier utilisateur se connecte. Consulte les logs du serveur pour l'erreur exacte, et vérifie que l'hôte Bootimus peut joindre l'issuer :

```bash
curl https://authentik.example.com/application/o/bootimus/.well-known/openid-configuration
```

La valeur `issuer` de ce document doit correspondre exactement à `BOOTIMUS_OIDC_ISSUER`, y compris un éventuel slash final. Si le fournisseur utilise une CA privée, ajoute-la au magasin de confiance de l'hôte ; `BOOTIMUS_OIDC_SKIP_VERIFY=true` fonctionne pour tester mais n'est pas recommandé.

### OIDC : le fournisseur affiche une erreur de redirect URI

La redirect URI enregistrée chez le fournisseur et `BOOTIMUS_OIDC_REDIRECT_URL` doivent correspondre exactement — schéma, hôte, port et chemin.

### OIDC : « Sign-in expired or was started elsewhere »

La connexion doit se terminer dans les 10 minutes, dans le même navigateur que celui qui l'a démarrée, et Bootimus ne doit pas avoir redémarré entre-temps. Si ça se répète derrière un reverse proxy, vérifie que le proxy laisse passer les cookies et que les utilisateurs accèdent à Bootimus sur le même hôte que l'URL de redirection.

### OIDC : l'utilisateur n'est pas administrateur ou est refusé

Les logs du serveur indiquent pourquoi chaque connexion a été refusée. Vérifie quels groupes le fournisseur envoie réellement — les noms de groupes doivent correspondre à `BOOTIMUS_OIDC_ADMIN_GROUPS` ou `BOOTIMUS_OIDC_ALLOWED_GROUPS`, et Keycloak envoie des chemins complets (`/bootimus-admins`) sauf si **Full group path** est désactivé. Si ton fournisseur utilise un autre nom de claim, définis `BOOTIMUS_OIDC_GROUPS_CLAIM`.

### « A user with this name already exists from a different source »

Voir [Utilisateurs LDAP et OIDC](#utilisateurs-ldap-et-oidc). Supprime le compte existant dans **Users** et reconnecte-toi.

### Token expiré

Les tokens JWT sont valables 24 heures. Après expiration, la page de login s'affiche automatiquement. Les tokens sont aussi invalidés au redémarrage du serveur (le secret de signature est regénéré).

### Admin local verrouillé

Réinitialise le mot de passe admin :
```bash
./bootimus serve --reset-admin-password
```

Ou, si démarrer le serveur est peu pratique (ex. conflits de ports), définir le
mot de passe directement dans la base de données :
```bash
./bootimus user set-password admin
```

Ça marche toujours, quelle que soit la config LDAP ou OIDC.
