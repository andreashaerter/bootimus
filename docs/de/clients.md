#  Leitfaden zur Client-Verwaltung

Kompletter Leitfaden zur Verwaltung von Netzwerk-Boot-Clients mit MAC-basierter Zugriffskontrolle.

##  Inhaltsverzeichnis

- [Überblick](#überblick)
- [Clients hinzufügen](#clients-hinzufügen)
- [Client-Berechtigungen](#client-berechtigungen)
- [Öffentliche vs. private Images](#öffentliche-vs-private-images)
- [Client-Statistiken](#client-statistiken)
- [Bulk-Operationen](#bulk-operationen)
- [Fehlersuche](#fehlersuche)

## Überblick

Bootimus nutzt MAC-Adress-basierte Zugriffskontrolle, um zu steuern, welche Clients booten dürfen und auf welche ISOs sie zugreifen können. Das gibt dir granulare Kontrolle über deine Netzwerk-Boot-Umgebung.

### Kernkonzepte

- **Client**: Ein Netzwerk-Boot-Gerät, identifiziert per MAC-Adresse
- **Statischer Client**: Manuell angelegt oder aus einem entdeckten Client hochgestuft — ein permanenter Eintrag
- **Entdeckter Client**: Automatisch angelegt, wenn ein unbekanntes Gerät per PXE bootet (wie ein DHCP-Lease)
- **Enabled**: Client darf booten (taucht im Boot-Menü auf)
- **Disabled**: Client darf nicht booten (Zugriff aufs Boot-Menü geblockt)
- **Zugewiesene Images**: Wenn ein Client zugewiesene Images hat, sieht er **nur diese** (nicht die volle Public-Liste)
- **Öffentliche Images zeigen**: Wenn zusätzlich zu zugewiesenen Images aktiviert, sieht der Client sowohl zugewiesene als auch öffentliche Images
- **Next-Boot-Aktion**: Einmaliger Boot-Image-Override, der sich nach Nutzung automatisch löscht

### Client-Auto-Discovery

Wenn ein unbekanntes Gerät per PXE bootet, legt Bootimus automatisch einen **entdeckten** Client-Eintrag an, mit:
- MAC-Adresse aus dem PXE-Request
- Hardware-Inventar (CPU, Speicher, Hersteller, Seriennummer, NIC)
- Enabled, mit standardmäßig sichtbaren öffentlichen Images

Entdeckte Clients tauchen in der Clients-Tabelle mit einem "Discovered"-Badge auf. Du kannst sie mit dem Button **"Make Static"** zu statischen Clients hochstufen, was sie als permanente Einträge registriert. Wenn ein zuvor gelöschter Client erneut per PXE bootet, wird er automatisch wiederhergestellt.

### Datenbank-Modi

**SQLite-Modus**:
- Clients in SQLite-Datenbank gespeichert
- Image-Zuweisungen im JSON-Feld `allowed_images` gespeichert
- Ideal für Single-Server-Deployments

**PostgreSQL-Modus**:
- Clients in PostgreSQL-Datenbank gespeichert
- Image-Zuweisungen über Many-to-Many-Beziehungstabelle
- Bessere Performance für große Deployments

## Clients hinzufügen

### Per Web-Oberfläche

1. Zum Admin-Panel navigieren: `http://your-server:8081`
2. Tab **"Clients"** öffnen
3. Auf Button **"Add Client"** klicken
4. Details ausfüllen:
   - **MAC-Adresse**: `00:11:22:33:44:55` (Pflicht)
   - **Name**: Anzeigename (z.B. "Lab Server 1")
   - **Beschreibung**: Zusätzliche Details (optional)
   - **Enabled**: Anhaken, um Boot zuzulassen
5. Auf **"Create Client"** klicken

### Per API

```bash
curl -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8081/api/clients \
  -H "Content-Type: application/json" \
  -d '{
    "mac_address": "00:11:22:33:44:55",
    "name": "Lab Server 1",
    "description": "Dell PowerEdge R720",
    "enabled": true
  }'
```

### MAC-Adress-Format

Bootimus akzeptiert MAC-Adressen in diesen Formaten:
- `00:11:22:33:44:55` (mit Doppelpunkten, bevorzugt)
- `00-11-22-33-44-55` (mit Bindestrichen, wird auto-konvertiert)
- `001122334455` (ohne Trennzeichen, wird auto-konvertiert)

Alle Formate werden auf lowercase mit Doppelpunkten normalisiert.

## Client-Berechtigungen

### Images einem Client zuweisen

**Per Web-Oberfläche**:
1. **"Edit"** in der Client-Zeile klicken
2. Images aus dem Multi-Select-Dropdown wählen
3. **"Update Client"** klicken

**Per API**:
```bash
curl -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8081/api/assign-images \
  -H "Content-Type: application/json" \
  -d '{
    "mac_address": "00:11:22:33:44:55",
    "image_filenames": [
      "ubuntu-24.04-live-server-amd64.iso",
      "debian-13.2.0-amd64-netinst.iso",
      "archlinux-2025.12.01-x86_64.iso"
    ]
  }'
```

### Client-Berechtigungen ansehen

**Per Web-Oberfläche**:
- Die zugewiesenen Images eines Clients werden im Edit-Modal angezeigt

**Per API**:
```bash
# Client-Details inkl. zugewiesener Images abrufen
curl -H "Authorization: Bearer $TOKEN" "http://localhost:8081/api/clients?mac=00:11:22:33:44:55" | jq
```

**Antwort**:
```json
{
  "success": true,
  "data": {
    "id": 1,
    "mac_address": "00:11:22:33:44:55",
    "name": "Lab Server 1",
    "description": "Dell PowerEdge R720",
    "enabled": true,
    "boot_count": 15,
    "last_boot": "2025-01-02T10:30:00Z",
    "allowed_images": [
      "ubuntu-24.04-live-server-amd64.iso",
      "debian-13.2.0-amd64-netinst.iso"
    ]
  }
}
```

## Öffentliche vs. private Images

### Öffentliche Images

Öffentliche Images stehen **allen Clients** zur Verfügung, sogar nicht registrierten.

**Anwendungsfälle**:
-  Rescue-/Recovery-ISOs
-  Netzwerk-Diagnose-Tools
-  Gängige Deployment-Images
-  Offene Lab-Umgebungen

**Image öffentlich machen**:
```bash
curl -H "Authorization: Bearer $TOKEN" -X PUT "http://localhost:8081/api/images?filename=ubuntu.iso" \
  -H "Content-Type: application/json" \
  -d '{"public": true}'
```

### Private Images

Private Images sind **nur für zugewiesene Clients** verfügbar.

**Anwendungsfälle**:
-  Sensitive oder lizenzierte Images
-  Client-spezifische Deployments
-  Eingeschränkte Umgebungen
-  Beta-/Test-Images

**Image privat machen**:
```bash
curl -H "Authorization: Bearer $TOKEN" -X PUT "http://localhost:8081/api/images?filename=windows.iso" \
  -H "Content-Type: application/json" \
  -d '{"public": false}'
```

### Zugriffskontroll-Matrix

| Client-Status | Was er sieht |
|--------------|---------------|
| **Enabled + zugewiesen** | Nur seine zugewiesenen Images |
| **Enabled + keine Zuweisungen** | Alle öffentlichen Images |
| **Disabled** | Alle öffentlichen Images |
| **Nicht registriert** | Alle öffentlichen Images |

## Client-Statistiken

Bootimus trackt Boot-Statistiken pro Client:

- **Boot Count**: Gesamtzahl der Boot-Versuche
- **Last Boot**: Zeitstempel des letzten Boots
- **Success Rate**: Prozentsatz erfolgreicher Boots

### Statistiken ansehen

**Per Web-Oberfläche**:
- Statistiken werden in der Clients-Tabelle angezeigt

**Per API**:
```bash
# Alle Clients mit Statistiken
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/clients | jq '.data[] | {name, boot_count, last_boot}'

# Top-Clients nach Boot-Count
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/clients | \
  jq '.data | sort_by(.boot_count) | reverse | .[0:10] | .[] | {name, boot_count}'
```

### Boot-Logs

Detaillierte Boot-Logs pro Client ansehen:

```bash
# Boot-Logs nach MAC-Adresse filtern
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/logs | \
  jq '.data[] | select(.mac_address=="00:11:22:33:44:55")'
```

## Bulk-Operationen

### Clients in Bulk hinzufügen

```bash
#!/bin/bash
# bulk-add-clients.sh

ADMIN_PASSWORD="${ADMIN_PASSWORD:-your-password}"

# Format: MAC:NAME:DESCRIPTION
CLIENTS=(
  "00:11:22:33:44:01:Server-01:Production Web Server"
  "00:11:22:33:44:02:Server-02:Production Database Server"
  "00:11:22:33:44:03:Server-03:Production Cache Server"
  "00:11:22:33:44:10:Workstation-01:Developer Laptop"
  "00:11:22:33:44:11:Workstation-02:QA Testing Machine"
)

for entry in "${CLIENTS[@]}"; do
  IFS=':' read -r mac name description <<< "$entry"

  curl -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8081/api/clients \
    -H "Content-Type: application/json" \
    -d "{
      \"mac_address\":\"$mac\",
      \"name\":\"$name\",
      \"description\":\"$description\",
      \"enabled\":true
    }"

  echo "Added $name ($mac)"
  sleep 0.5
done
```

### Images in Bulk zuweisen

```bash
#!/bin/bash
# bulk-assign-images.sh

ADMIN_PASSWORD="${ADMIN_PASSWORD:-your-password}"

# Ubuntu und Debian an alle Server zuweisen
SERVER_MACS=(
  "00:11:22:33:44:01"
  "00:11:22:33:44:02"
  "00:11:22:33:44:03"
)

IMAGES='["ubuntu-24.04-live-server-amd64.iso","debian-13.2.0-amd64-netinst.iso"]'

for mac in "${SERVER_MACS[@]}"; do
  curl -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8081/api/assign-images \
    -H "Content-Type: application/json" \
    -d "{\"mac_address\":\"$mac\",\"image_filenames\":$IMAGES}"

  echo "Assigned images to $mac"
done
```

### Bulk Enable/Disable

```bash
#!/bin/bash
# bulk-enable.sh

ADMIN_PASSWORD="${ADMIN_PASSWORD:-your-password}"

# Alle Clients holen und aktivieren
macs=$(curl -H "Authorization: Bearer $TOKEN" -s http://localhost:8081/api/clients | \
  jq -r '.data[].mac_address')

for mac in $macs; do
  curl -H "Authorization: Bearer $TOKEN" -X PUT "http://localhost:8081/api/clients?mac=$mac" \
    -H "Content-Type: application/json" \
    -d '{"enabled":true}'
  echo "Enabled $mac"
done
```

### Client-Liste exportieren

```bash
#!/bin/bash
# export-clients.sh

ADMIN_PASSWORD="${ADMIN_PASSWORD:-your-password}"

echo "MAC Address,Name,Description,Enabled,Boot Count,Last Boot"

curl -H "Authorization: Bearer $TOKEN" -s http://localhost:8081/api/clients | \
  jq -r '.data[] | [.mac_address, .name, .description, .enabled, .boot_count, .last_boot] | @csv'
```

### Clients aus CSV importieren

```bash
#!/bin/bash
# import-clients.sh

ADMIN_PASSWORD="${ADMIN_PASSWORD:-your-password}"
CSV_FILE="clients.csv"

# Header-Zeile überspringen und CSV verarbeiten
tail -n +2 "$CSV_FILE" | while IFS=',' read -r mac name description enabled; do
  # Anführungszeichen aus CSV-Werten entfernen
  mac=$(echo $mac | tr -d '"')
  name=$(echo $name | tr -d '"')
  description=$(echo $description | tr -d '"')
  enabled=$(echo $enabled | tr -d '"')

  curl -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8081/api/clients \
    -H "Content-Type: application/json" \
    -d "{
      \"mac_address\":\"$mac\",
      \"name\":\"$name\",
      \"description\":\"$description\",
      \"enabled\":$enabled
    }"

  echo "Imported $name ($mac)"
done
```

## Next-Boot-Aktion

Setze für einen Client ein einmaliges Boot-Image. Beim nächsten PXE-Boot wird das gewählte Image als Default-Menüeintrag mit Timeout vorausgewählt. Die Aktion löscht sich nach Nutzung automatisch — folgende Boots laufen wieder normal.

### Per Web-Oberfläche

1. **"Next Boot"** in einer Client-Zeile klicken
2. Image aus dem Dropdown wählen
3. **"Set Next Boot"** klicken, um nur das Image zu setzen, oder **"Set & Wake"**, um zusätzlich ein Wake-on-LAN-Paket zu schicken

### Per API

```bash
# Next-Boot-Image setzen
curl -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8081/api/clients/next-boot \
  -H "Content-Type: application/json" \
  -d '{"mac_address":"00:11:22:33:44:55","image_filename":"ubuntu-24.04.iso"}'

# Next-Boot-Aktion löschen
curl -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8081/api/clients/next-boot \
  -H "Content-Type: application/json" \
  -d '{"mac_address":"00:11:22:33:44:55","image_filename":""}'
```

### Verhalten

- Das Boot-Menü wird wie gewohnt angezeigt, aber das Next-Boot-Image ist als Default vorausgewählt
- Wenn das globale Menü-Timeout deaktiviert ist (auf 0 gesetzt), wird ersatzweise ein 10-Sekunden-Timeout angewandt
- Wenn der Client nicht bootet, bevor die Aktion verbraucht ist, löscht sich der Next-Boot beim ersten PXE-Request
- Leere Gruppen werden aus dem Menü ausgeblendet, wenn ein Client zugewiesene Images hat

## Wake-on-LAN

Sende ein WOL-Magic-Paket, um einen Client aus der Ferne aufzuwecken. Kombiniere das mit **Next Boot**, um eine Maschine zu wecken und sie in ein bestimmtes Image booten zu lassen.

### Per API

```bash
# Client aufwecken
curl -H "Authorization: Bearer $TOKEN" -X POST "http://localhost:8081/api/clients/wake?mac=00:11:22:33:44:55"

# Aufwecken mit eigener Broadcast-Adresse
curl -H "Authorization: Bearer $TOKEN" -X POST "http://localhost:8081/api/clients/wake?mac=00:11:22:33:44:55" \
  -H "Content-Type: application/json" \
  -d '{"broadcast_addr":"192.168.1.255"}'
```

## Hardware-Inventar

Bootimus sammelt während des Boots Hardware-Infos der PXE-Clients, darunter:
- CPU, Speicher, Plattform und Architektur
- Hersteller, Produktname und Seriennummer
- UUID und NIC-Chip-Info
- IP-Adresse

Das Inventar ist im Modal **Edit & Assign Images** für jeden Client einsehbar, oder über die API:

```bash
# Aktuelles Inventar
curl -H "Authorization: Bearer $TOKEN" "http://localhost:8081/api/clients/inventory?mac=00:11:22:33:44:55"

# Inventar-Historie
curl -H "Authorization: Bearer $TOKEN" "http://localhost:8081/api/clients/inventory/history?mac=00:11:22:33:44:55&limit=10"
```

## Fehlersuche

### Client sieht kein Boot-Menü

**Symptome**: Client bootet, zeigt aber leeres Menü oder "No boot images available"

**Mögliche Ursachen**:
1. Client ist disabled
2. Keine öffentlichen Images verfügbar
3. Keine Images dem Client zugewiesen
4. Alle Images sind disabled

**Lösung**:
```bash
# Client-Status prüfen
curl -H "Authorization: Bearer $TOKEN" "http://localhost:8081/api/clients?mac=00:11:22:33:44:55" | jq

# Client aktivieren
curl -H "Authorization: Bearer $TOKEN" -X PUT "http://localhost:8081/api/clients?mac=00:11:22:33:44:55" \
  -H "Content-Type: application/json" \
  -d '{"enabled":true}'

# Verfügbare Images prüfen
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/images | jq '.data[] | {filename, enabled, public}'

# Images öffentlich machen
curl -H "Authorization: Bearer $TOKEN" -X PUT "http://localhost:8081/api/images?filename=ubuntu.iso" \
  -H "Content-Type: application/json" \
  -d '{"public":true,"enabled":true}'
```

### MAC-Adresse nicht erkannt

**Symptome**: Boot-Logs zeigen "unknown" MAC-Adresse

**Mögliche Ursachen**:
1. iPXE kann die MAC nicht vom Netzwerk-Interface ermitteln
2. Client nutzt mehrere Netzwerk-Interfaces

**Lösung**:
```bash
# Boot-Logs auf tatsächliche IP-Adresse prüfen
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/logs | jq '.data[] | {mac_address, ip_address}'

# Client per IP registrieren, falls MAC unbekannt
# (Hinweis: Weniger zuverlässig, IP kann sich ändern)
```

### Zugewiesene Images werden nicht angezeigt

**Symptome**: Client sieht nur öffentliche, nicht die zugewiesenen Images

**Mögliche Ursachen**:
1. Client nicht aktiviert
2. Images nicht aktiviert
3. Falsches MAC-Adress-Format
4. Datenbank-Sync-Problem

**Lösung**:
```bash
# Sicherstellen, dass der Client existiert und aktiviert ist
curl -H "Authorization: Bearer $TOKEN" "http://localhost:8081/api/clients?mac=00:11:22:33:44:55" | jq

# Image-Zuweisungen prüfen
curl -H "Authorization: Bearer $TOKEN" "http://localhost:8081/api/clients?mac=00:11:22:33:44:55" | \
  jq '.data.allowed_images'

# Images neu zuweisen
curl -H "Authorization: Bearer $TOKEN" -X POST http://localhost:8081/api/assign-images \
  -H "Content-Type: application/json" \
  -d '{
    "mac_address":"00:11:22:33:44:55",
    "image_filenames":["ubuntu.iso","debian.iso"]
  }'

# Datenbank direkt prüfen (SQLite)
sqlite3 data/bootimus.db "SELECT * FROM clients WHERE mac_address='00:11:22:33:44:55';"
```

### Duplicate-Client-Fehler

**Symptome**: "Client already exists" oder UNIQUE-Constraint-Fehler

**Ursache**: MAC-Adresse bereits registriert

**Lösung**:
```bash
# Bestehenden Client finden
curl -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/clients | \
  jq '.data[] | select(.mac_address=="00:11:22:33:44:55")'

# Stattdessen bestehenden Client aktualisieren
curl -H "Authorization: Bearer $TOKEN" -X PUT "http://localhost:8081/api/clients?mac=00:11:22:33:44:55" \
  -H "Content-Type: application/json" \
  -d '{"name":"Updated Name","enabled":true}'

# Oder löschen und neu anlegen
curl -H "Authorization: Bearer $TOKEN" -X DELETE "http://localhost:8081/api/clients?mac=00:11:22:33:44:55"
```

## Nächste Schritte

-  [Image-Verwaltung](images.md) konfigurieren, um ISOs hinzuzufügen
-  Die [Admin-Konsole](admin.md) zur Verwaltung nutzen
-  [DHCP-Konfiguration](dhcp.md) für Netzwerk-Boot einrichten
-  [Boot-Logs](admin.md#boot-logs) zum Monitoring ansehen
