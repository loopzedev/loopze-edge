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

- **Canvas**:
  - Static Mode: 0 Inputs, 1 Output (Source Node)
  - Dynamic Mode: 1 Input, 1 Output — der Input dient nur der Steuerung (subscribe / clear), nicht der Datenweitergabe
- **Funktion**: Verbindet sich über den konfigurierten Broker und leitet empfangene MQTT-Nachrichten als Flow-Messages weiter. Die Subscription kann entweder fest in der Config hinterlegt oder zur Laufzeit per Eingangs-Message gesteuert werden.
- **Konfiguration**:
  - `broker` (string) — ID des referenzierten `mqtt-broker` Config Nodes
  - `mode` (string) — `static` (Default) oder `dynamic`
  - `topic` (string) — MQTT Topic zum Abonnieren, z.B. `sensor/temperature` (nur im Static-Modus relevant)
  - `qos` (number) — Quality of Service: 0, 1 oder 2. Standard: 0
- **Ausgehende Message** (für jede empfangene MQTT-Nachricht):
  ```json
  {
    "topic": "sensor/temperature",
    "payload": "<empfangene Daten>",
    "qos": 0,
    "retain": false
  }
  ```

#### Static Mode (Default)

- Beim Deploy / Start subscribed der Node das in `topic` konfigurierte Pattern.
- Subscription bleibt für die gesamte Lebensdauer des Nodes bestehen.
- Eingangs-Port ist nicht vorhanden.

#### Dynamic Mode

- Beim Deploy / Start hat der Node **keine** aktiven Subscriptions — er wartet auf Steuer-Messages.
- Steuerung über `msg.action`:
  - `msg.action = "subscribe"` → die in `msg.payload` angegebenen Topics werden subscribed:
    - `msg.payload` als **string** → ein einzelnes Topic
    - `msg.payload` als **string-array** → mehrere Topics
  - **Bei jedem `subscribe` werden zuerst alle bestehenden Subscriptions des Nodes unsubscribed**, danach werden die neuen Topics subscribed. Es gibt also stets nur den jüngsten Stand.
- Eingehende Steuer-Messages werden **nicht** am Output durchgereicht — der Output liefert ausschließlich empfangene MQTT-Nachrichten.
- Topics, die im selben `subscribe`-Aufruf bereits aktiv sind, dürfen ohne Aussetzer weitergehen (Implementation: Diff `alt → neu`, nur Differenzen un-/subscriben — Optimierung, nicht zwingend für v1).
- Ein leeres Array bzw. ein leerer String wirkt als „alle Subscriptions löschen“.
- **QoS-Override per Message:** `msg.qos` (number, gültige Werte 0/1/2) überschreibt für diesen `subscribe`-Aufruf den in der Config hinterlegten QoS. Fehlt `msg.qos` oder liegt der Wert außerhalb 0–2, wird der konfigurierte Default verwendet. Der QoS gilt einheitlich für alle Topics, die mit der Steuer-Message subscribed werden.

- **Status-Anzeige** (via `SetStatus`):
  - Grün: "verbunden" — Broker-Verbindung steht
    - Static: Format `verbunden · <topic>`
    - Dynamic: Format `verbunden · <n> Topic(s)` (oder "verbunden · idle" wenn keine aktiv)
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
│  Mode                                         │
│  ( • ) Static    ( ) Dynamic (msg.action)     │
│                                               │
│  Topic                          (Static only) │
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

Im Dynamic-Modus wird das Topic-Feld ausgeblendet und unterhalb des Mode-Selectors ein Hinweis-Block gezeigt:

```
ℹ Send msg.action = "subscribe" with msg.payload as
   topic string or array of topics. Existing
   subscriptions are replaced on each call.
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
            "mode": "static",
            "topic": "sensor/temperature",
            "qos": 0
          }
        },
        {
          "id": "node-mqtt-in-2",
          "type": "mqtt-in",
          "name": "Dynamische Subscription",
          "x": 200,
          "y": 250,
          "z": "flow-1",
          "inputs": 1,
          "outputs": 1,
          "wires": [["node-debug-1"]],
          "config": {
            "broker": "broker-1",
            "mode": "dynamic",
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

### Dynamic Subscription (mqtt-in, Dynamic Mode)

Im Dynamic-Modus implementiert der Node `OnInput`, hält die Liste der aktuell aktiven
Topics intern und reagiert auf Steuer-Messages:

```go
type MqttInNode struct {
    // … broker, qos, …
    activeTopics []string // nur im Dynamic-Modus belegt
    mu           sync.Mutex
}

func (n *MqttInNode) OnInput(msg Message) {
    if action, _ := msg.GetString("action"); action != "subscribe" {
        return // nur "subscribe" wird akzeptiert; alles andere wird verworfen
    }

    next := toTopicSlice(msg.Get("payload")) // string → [s], []string → s, leer → []
    qos  := extractQoS(msg.Get("qos"), n.qos) // msg.qos überschreibt config-qos (0/1/2), sonst Fallback

    n.mu.Lock()
    defer n.mu.Unlock()

    // Alle bisherigen Topics dieses Nodes unsubscriben
    for _, t := range n.activeTopics {
        n.broker.Unsubscribe(n.id, t)
    }
    // Neue Topics subscriben
    for _, t := range next {
        n.broker.Subscribe(n.id, t, qos, n.onMessage)
    }
    n.activeTopics = next
    n.updateStatus()
}
```

Wichtig:
- Steuer-Messages werden **nicht** weitergeleitet (`return` statt `n.send(0, msg)`).
- Beim Stop / Redeploy müssen alle `activeTopics` sauber unsubscribed werden.
- Der MQTT Broker (Config Node) muss `Subscribe` und `Unsubscribe` per Subscriber-ID anbieten, damit beim Redeploy oder Re-Subscribe gezielt aufgeräumt werden kann.

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
