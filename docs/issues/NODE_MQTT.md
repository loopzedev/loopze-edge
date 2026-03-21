# Issue: MQTT Nodes – Subscribe & Publish mit Broker-Konfiguration

## Status: Open

## Problembeschreibung

Flint benötigt seinen ersten **Data Connector** — MQTT. Zwei neue Node-Typen (`mqtt-in` und `mqtt-out`) ermöglichen das Empfangen und Senden von MQTT-Nachrichten. Zentral dabei ist das Konzept einer **Broker-Konfiguration**, die als eigenständige, wiederverwendbare Entität verwaltet wird. Jeder MQTT-Node referenziert genau einen Broker, aber über mehrere Nodes hinweg können unterschiedliche Broker konfiguriert werden.

Dieses Issue führt gleichzeitig das neue Konzept der **Config Nodes** ein — konfigurierbare Entitäten, die nicht auf dem Canvas erscheinen, aber von mehreren Nodes referenziert werden können (z.B. Server-Verbindungen, Authentifizierung). Der MQTT-Broker ist der erste Config Node in Flint.

**Scope**: MQTT v3.1.1. Unterstützung für MQTT v5 wird in einem Folge-Issue ergänzt. Fokus liegt auf funktionierender Broker-Konfiguration und -Instanziierung. Die Subscribe/Publish-Konfiguration ist bewusst minimal gehalten.

## Übersicht

| Node-Typ | Typ-ID | Canvas Inputs | Canvas Outputs | Beschreibung |
|---|---|---|---|---|
| **MQTT Subscribe** | `mqtt-in` | 0 | 1 | Empfängt Nachrichten von einem MQTT-Broker via Subscription |
| **MQTT Publish** | `mqtt-out` | 1 | 0 | Sendet Nachrichten an einen MQTT-Broker |

```
                          MQTT Broker (extern)
                          ┌──────────────┐
Flow A                    │              │
┌─────────────────────┐   │  topic/data  │
│                      │   │      │       │
│  [MQTT In] ←─────────────┘      │       │
│      ↓               │          │       │
│  [Function]          │          │       │
│      ↓               │          │       │
│  [Debug]             │          │       │
│                      │          │       │
└─────────────────────┘          │       │
                                  │       │
Flow B                            │       │
┌─────────────────────┐          │       │
│                      │          │       │
│  [Inject]            │          │       │
│      ↓               │          │       │
│  [MQTT Out] ──────────────────→│       │
│                      │   │              │
└─────────────────────┘   └──────────────┘
```

## Anforderungen

### 1. Config Node: MQTT Broker (`mqtt-broker`)

Der MQTT Broker ist ein **Config Node** — er erscheint nicht auf dem Canvas, sondern wird als eigenständige Konfiguration im Workspace verwaltet und von `mqtt-in`/`mqtt-out` Nodes referenziert.

- **Typ-ID**: `mqtt-broker`
- **Kein Canvas-Element** — rein konfigurativ
- **Konfigurationsfelder**:
  - `name` (string) — Anzeigename im Dropdown, z.B. "Produktion Broker"
  - `host` (string) — Hostname oder IP, z.B. "mqtt.example.com"
  - `port` (number) — Standard: 1883
  - `clientId` (string) — Client-ID, Standard: auto-generiert (`flint-<random>`)
  - `username` (string, optional) — Benutzername
  - `password` (string, optional) — Passwort
  - `keepalive` (number) — Keep-Alive Intervall in Sekunden, Standard: 60
  - `cleanSession` (boolean) — Clean Session Flag, Standard: true
  - `useTLS` (boolean) — TLS aktivieren, Standard: false

- **Zugriff auf den Properties-Dialog**:
  - **Neuer Broker**: Über den "+" Button neben dem Broker-Dropdown in MQTT Nodes
  - **Bestehenden Broker editieren**: Über den "Edit broker config" Link unterhalb des Broker-Dropdowns (nur sichtbar wenn ein Broker ausgewählt ist). Öffnet den gleichen Dialog vorausgefüllt mit den bestehenden Einstellungen
- **Properties-Dialog**:

```
┌──────────────────────────────────────────────┐
│  MQTT Broker                                  │
├──────────────────────────────────────────────┤
│                                               │
│  Name                                         │
│  ┌────────────────────────────────────────┐   │
│  │ Produktion Broker                      │   │
│  └────────────────────────────────────────┘   │
│                                               │
│  Server                                       │
│  ┌──────────────────────────┐ ┌──────────┐   │
│  │ mqtt.example.com         │ │ 1883     │   │
│  └──────────────────────────┘ └──────────┘   │
│                  Host              Port        │
│                                               │
│  Client ID                                    │
│  ┌────────────────────────────────────────┐   │
│  │ flint-abc123                           │   │
│  └────────────────────────────────────────┘   │
│                                               │
│  Credentials                                  │
│  ┌──────────────────────────────────────┐     │
│  │ Username                              │     │
│  └──────────────────────────────────────┘     │
│  ┌──────────────────────────────────────┐     │
│  │ ••••••••                              │     │
│  └──────────────────────────────────────┘     │
│                                               │
│  ┌─────┐                                     │
│  │ TLS │  Keep-Alive: [60]s                   │
│  └─────┘  ☑ Clean Session                     │
│                                               │
│  ┌────────────┐  ┌────────────┐              │
│  │  Speichern  │  │ Abbrechen  │              │
│  └────────────┘  └────────────┘              │
└──────────────────────────────────────────────┘
```

### 2. MQTT Subscribe Node (`mqtt-in`)

- **Canvas**: 0 Inputs, 1 Output (Source Node)
- **Funktion**: Verbindet sich über den konfigurierten Broker mit einem MQTT-Topic und leitet empfangene Nachrichten als Flow-Messages weiter
- **Konfiguration** (minimal für ersten Wurf):
  - `broker` (string) — ID des referenzierten `mqtt-broker` Config Nodes
  - `topic` (string) — MQTT Topic zum Abonnieren, z.B. "sensor/temperature"
  - `qos` (number) — Quality of Service: 0, 1 oder 2. Standard: 0
- **Ausgehende Message**:
  ```json
  {
    "topic": "sensor/temperature",
    "payload": "<empfangene Daten>",
    "qos": 0,
    "retain": false
  }
  ```
- **Status-Anzeige** (via `SetStatus`):
  - Grün: "verbunden" — Broker-Verbindung steht, Subscription aktiv
  - Gelb: "verbinde..." — Verbindungsaufbau läuft
  - Rot: "getrennt" / Fehlermeldung — Verbindung fehlgeschlagen

- **Properties-Panel**:

```
┌──────────────────────────────────────────────┐
│  MQTT Subscribe                               │
├──────────────────────────────────────────────┤
│                                               │
│  Broker                                       │
│  ┌────────────────────────────────┐ ┌───┐    │
│  │ Produktion Broker          ▼  │ │ + │    │
│  └────────────────────────────────┘ └───┘    │
│  Edit broker config                           │
│                                               │
│  Topic                                        │
│  ┌────────────────────────────────────────┐   │
│  │ sensor/temperature                     │   │
│  └────────────────────────────────────────┘   │
│                                               │
│  QoS                                          │
│  ┌────────────────────────────────────────┐   │
│  │ 0 - At most once                   ▼  │   │
│  └────────────────────────────────────────┘   │
│                                               │
└──────────────────────────────────────────────┘
```

### 3. MQTT Publish Node (`mqtt-out`)

- **Canvas**: 1 Input, 0 Outputs (Sink Node)
- **Funktion**: Publiziert eingehende Messages über den konfigurierten Broker auf ein MQTT-Topic
- **Konfiguration** (minimal für ersten Wurf):
  - `broker` (string) — ID des referenzierten `mqtt-broker` Config Nodes
  - `topic` (string, optional) — MQTT Topic zum Publizieren. Wenn leer, wird `msg.topic` verwendet
  - `qos` (number) — Quality of Service: 0, 1 oder 2. Standard: 0
  - `retain` (boolean) — Retain Flag. Standard: false
- **Eingehende Message**:
  - `msg.payload` wird als MQTT-Payload gesendet
  - `msg.topic` wird als Fallback-Topic verwendet wenn keins konfiguriert ist
- **Status-Anzeige**: Analog zu `mqtt-in`

- **Properties-Panel**:

```
┌──────────────────────────────────────────────┐
│  MQTT Publish                                 │
├──────────────────────────────────────────────┤
│                                               │
│  Broker                                       │
│  ┌────────────────────────────────────┐ ┌───┐│
│  │ Produktion Broker              ▼  │ │ + ││
│  └────────────────────────────────────┘ └───┘│
│  Edit broker config                           │
│                                               │
│  Topic                                        │
│  ┌────────────────────────────────────────┐   │
│  │ actuator/command                       │   │
│  └────────────────────────────────────────┘   │
│                                               │
│  QoS                                          │
│  ┌────────────────────────────────────────┐   │
│  │ 0 - At most once                   ▼  │   │
│  └────────────────────────────────────────┘   │
│                                               │
│  ☐ Retain                                     │
│                                               │
└──────────────────────────────────────────────┘
```

### 4. Config Node Konzept (neu in Flint)

Config Nodes sind ein neues architektonisches Konzept, das mit diesem Issue eingeführt wird:

- **Kein Canvas-Element**: Config Nodes haben keine visuelle Darstellung im Flow-Editor
- **Eigenständige Persistenz**: Config Nodes werden in `workspace.json` als eigener Abschnitt gespeichert (nicht innerhalb eines Flows)
- **Referenzierung**: Normale Nodes referenzieren Config Nodes über deren ID
- **Shared Instanz**: Mehrere Nodes können denselben Config Node referenzieren — die Engine erstellt pro Config Node nur **eine** Instanz (z.B. eine MQTT-Verbindung) und teilt sie zwischen allen referenzierenden Nodes
- **Lifecycle**: Config Node Instanzen werden beim Deploy erstellt und beim Re-Deploy/Stop gestoppt

### 5. Broker Connection Sharing

Wenn mehrere MQTT-Nodes denselben Broker referenzieren, wird **eine einzige MQTT-Verbindung** geteilt:

```
[mqtt-in  topic=a] ──┐
[mqtt-in  topic=b] ──┤── Broker "Produktion" ── 1 TCP-Verbindung
[mqtt-out topic=c] ──┘
```

Die Engine muss dafür einen **Broker-Manager** bereitstellen, der:
1. Beim Deploy alle referenzierten Broker-Configs sammelt
2. Pro Broker-ID eine MQTT-Client-Verbindung aufbaut
3. Den MQTT-Nodes Zugriff auf den geteilten Client gibt
4. Beim Stop/Re-Deploy alle Verbindungen sauber trennt

## Datenstruktur

### workspace.json

```json
{
  "flows": [
    {
      "id": "flow-1",
      "type": "tab",
      "label": "Sensoren",
      "nodes": [
        {
          "id": "node-mqtt-in-1",
          "type": "mqtt-in",
          "name": "Temperatur",
          "x": 200,
          "y": 150,
          "z": "flow-1",
          "inputs": 0,
          "outputs": 1,
          "wires": [["node-debug-1"]],
          "config": {
            "broker": "broker-1",
            "topic": "sensor/temperature",
            "qos": 0
          }
        },
        {
          "id": "node-mqtt-out-1",
          "type": "mqtt-out",
          "name": "Steuerung",
          "x": 600,
          "y": 300,
          "z": "flow-1",
          "inputs": 1,
          "outputs": 0,
          "wires": [],
          "config": {
            "broker": "broker-1",
            "topic": "actuator/command",
            "qos": 1,
            "retain": false
          }
        }
      ]
    }
  ],
  "configs": [
    {
      "id": "broker-1",
      "type": "mqtt-broker",
      "name": "Produktion Broker",
      "config": {
        "host": "mqtt.example.com",
        "port": 1883,
        "clientId": "flint-abc123",
        "username": "user",
        "password": "",
        "keepalive": 60,
        "cleanSession": true,
        "useTLS": false
      }
    }
  ]
}
```

**Hinweis**: Das `configs`-Array ist ein neuer Top-Level-Abschnitt in `workspace.json` neben `flows`. Alle Config Nodes (jetzt MQTT-Broker, zukünftig auch HTTP-Auth, Datenbank-Verbindungen etc.) werden hier abgelegt.

## Betroffene Dateien

### Backend – Neue Dateien

- `internal/nodes/mqtt_broker.go` — MQTT Broker Config Node: Verbindungsaufbau, Reconnect-Logik, Subscription-Management. Kapselt den `paho.mqtt.golang` Client
- `internal/nodes/mqtt_in.go` — MQTT Subscribe Node: Registriert Subscription beim geteilten Broker-Client, empfängt Nachrichten und sendet sie via `SendFunc` in den Flow
- `internal/nodes/mqtt_out.go` — MQTT Publish Node: Publiziert eingehende Flow-Messages über den geteilten Broker-Client

### Backend – Anpassungen

- `internal/server/server.go` — Registrierung von `mqtt-in` und `mqtt-out` im `registerNodes()`
- `internal/flow/engine.go` — Config Node Lifecycle:
  - Neuer Abschnitt in `Deploy()`: Config Nodes vor den regulären Nodes instanziieren
  - Config Node Instanzen den referenzierenden Nodes über ein neues Provider-Interface bereitstellen
  - `Stop()` erweitern: Config Node Instanzen sauber herunterfahren
- `internal/flow/registry.go` — Optionale Erweiterung: `ConfigProvider` Interface analog zu `ContextProvider` und `LinkProvider`
  ```go
  type ConfigProvider interface {
      SetConfigNode(configType string, configID string, instance any)
  }
  ```
- `internal/flow/types.go` — Config Node Datentyp für workspace.json Deserialisierung:
  ```go
  type ConfigNode struct {
      ID     string         `json:"id"`
      Type   string         `json:"type"`
      Name   string         `json:"name"`
      Config map[string]any `json:"config"`
  }
  ```
- `internal/storage/` — Workspace Load/Save erweitern um `configs`-Abschnitt

### Frontend – Neue Dateien

- `frontend/src/components/config/MqttNodeConfig.vue` — Gemeinsame Config-Komponente für `mqtt-in` und `mqtt-out` mit Broker-Dropdown + "+" Button, Topic-Eingabe, QoS-Dropdown, Retain-Toggle (nur mqtt-out)
- `frontend/src/components/config/MqttBrokerConfig.vue` — Broker Config Dialog: Formular für Host, Port, Client-ID, Credentials, TLS, Keep-Alive. Öffnet sich als eigenständiges Properties-Panel über den "+" Button

### Frontend – Anpassungen

- `frontend/src/components/PropertyPanel.vue` — Dispatch für `mqtt-in` und `mqtt-out` auf `MqttNodeConfig`. Zusätzlich: Unterstützung für Config Node Dialoge (Broker-Konfiguration als verschachteltes Panel)
- `frontend/src/components/nodes/tokens.ts` — Bereits vorhanden: `mqtt-in` → input (grün), `mqtt-out` → output (orange). Keine Änderung nötig
- `frontend/src/stores/flowStore.ts` — Config Nodes verwalten: CRUD-Operationen für `configs[]` im Workspace, API-Calls für Persistenz
- `frontend/src/types/flow.ts` — TypeScript-Typen für Config Nodes und MQTT-Broker-Config

### Go Dependencies

- `github.com/eclipse/paho.mqtt.golang` — MQTT v3.1.1 Client Library

## Technische Hinweise

### Config Node Lifecycle in der Engine

Config Nodes haben einen eigenen Lifecycle, der **vor** den regulären Nodes ausgeführt wird:

1. **Beim Deploy**: Engine liest `configs[]` aus dem Workspace, instanziiert Config Nodes und baut Verbindungen auf
2. **Injection**: Reguläre Nodes, die `ConfigProvider` implementieren, erhalten Referenzen auf ihre Config Node Instanzen
3. **Beim Stop/Re-Deploy**: Config Node Instanzen werden **nach** den regulären Nodes gestoppt (umgekehrte Reihenfolge)

```
Deploy:   Config Nodes starten → Reguläre Nodes starten
Stop:     Reguläre Nodes stoppen → Config Nodes stoppen
```

### MQTT Broker – Reconnect-Strategie

Der MQTT-Client sollte automatisches Reconnect implementieren:

- `paho.mqtt.golang` bietet `AutoReconnect: true` und `ConnectRetry: true`
- Bei Verbindungsverlust: Status auf Gelb ("reconnecting...")
- Bei erfolgreicher Wiederverbindung: Subscriptions automatisch erneuern, Status auf Grün
- Bei dauerhaftem Fehler: Status auf Rot mit Fehlermeldung

### Broker-Dropdown im Frontend

Das Broker-Dropdown im MQTT Node Config Panel zeigt alle `mqtt-broker` Config Nodes aus `flowStore.configs`:

```typescript
const mqttBrokers = computed(() =>
  flowStore.configs.filter(c => c.type === 'mqtt-broker')
)
```

Der "+" Button neben dem Dropdown öffnet den `MqttBrokerConfig.vue` Dialog. Nach dem Speichern wird der neue Broker automatisch im Dropdown ausgewählt.

### Message-Mapping (mqtt-in)

Empfangene MQTT-Nachrichten werden in das Flint Message-Format übersetzt:

```go
func (n *MqttInNode) onMessage(client mqtt.Client, mqttMsg mqtt.Message) {
    msg := NewMessage()
    msg.Set("topic", mqttMsg.Topic())
    msg.Set("payload", string(mqttMsg.Payload()))
    msg.Set("qos", int(mqttMsg.Qos()))
    msg.Set("retain", mqttMsg.Retained())
    n.send(0, msg)
}
```

### Payload-Handling (mqtt-out)

Der Publish Node liest `msg.payload` und konvertiert es für den MQTT-Publish:

- `string` → direkt als Payload
- `map`/`slice` → JSON-serialisiert
- `number`/`bool` → String-Konvertierung

## Abhängigkeiten

- **Keine Abhängigkeiten** zu bestehenden Issues — dies ist ein eigenständiges Feature
- Führt das **Config Node Konzept** ein, das von zukünftigen Connector-Nodes wiederverwendet wird (HTTP, TCP, Modbus, OPC-UA, Datenbanken etc.)
- Frontend-Tokens für `mqtt-in` und `mqtt-out` sind bereits in `tokens.ts` definiert — sie erscheinen automatisch in der Palette sobald das Backend sie registriert

## Abgrenzung / Nicht im Scope

- **MQTT v5**: Wird in einem Folge-Issue behandelt (Shared Subscriptions, Message Expiry, User Properties etc.)
- **Wildcard-Topics**: `+` und `#` Wildcards in Topics werden für den ersten Wurf nicht explizit validiert, funktionieren aber transparent über den MQTT-Client
- **Last Will / Testament**: Nicht in v1
- **Erweiterte TLS-Konfiguration**: Client-Zertifikate, CA-Bundle etc. — nicht in v1
- **Credential Encryption**: Passwörter werden vorerst im Klartext in der Config gespeichert. Verschlüsselung wird separat über ein Credential-System adressiert
