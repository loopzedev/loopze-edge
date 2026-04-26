# Issue: MQTT Nodes – Subscribe & Publish mit Broker-Konfiguration

## Status: Open

## Problembeschreibung

Flint benötigt seinen ersten **Data Connector** — MQTT. Zwei neue Node-Typen (`mqtt-in` und `mqtt-out`) ermöglichen das Empfangen und Senden von MQTT-Nachrichten. Zentral dabei ist das Konzept einer **Broker-Konfiguration**, die als eigenständige, wiederverwendbare Entität verwaltet wird. Jeder MQTT-Node referenziert genau einen Broker, aber über mehrere Nodes hinweg können unterschiedliche Broker konfiguriert werden.

Dieses Issue führt gleichzeitig das neue Konzept der **Config Nodes** ein — konfigurierbare Entitäten, die nicht auf dem Canvas erscheinen, aber von mehreren Nodes referenziert werden können (z.B. Server-Verbindungen, Authentifizierung). Der MQTT-Broker ist der erste Config Node in Flint.

**Scope**: **MQTT v5 ist Pflicht.** Der Broker-Client muss v5 sprechen. v3.1.1 bleibt als alternativ wählbare Protokoll-Version verfügbar — die Auswahl trifft der Anwender pro Broker-Config bewusst, es gibt keinen automatischen Fallback. Fokus liegt auf funktionierender Broker-Konfiguration und -Instanziierung. Die Subscribe/Publish-Konfiguration ist bewusst minimal gehalten — v5-spezifische Features (User Properties, Message Expiry, Shared Subscriptions) werden in einer schlanken ersten Stufe unterstützt.

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
  - `protocolVersion` (string) — `5` (Default) oder `3.1.1`. Muss vom Anwender bewusst gewählt werden — kein automatischer Fallback
  - `username` (string, optional) — Benutzername
  - `password` (string, optional) — Passwort
  - `keepalive` (number) — Keep-Alive Intervall in Sekunden, Standard: 60
  - `cleanStart` (boolean) — Clean Start (v5) bzw. Clean Session (v3.1.1) Flag, Standard: true
  - `sessionExpiry` (number, v5) — Session Expiry Interval in Sekunden, Standard: 0 (Session endet beim Disconnect). Wird im v3.1.1-Fallback ignoriert
  - `useTLS` (boolean) — TLS aktivieren, Standard: false
  - **onConnect Message** — Client-seitige Konvention: wird vom Flint-Client unmittelbar nach erfolgreichem CONNACK als regulärer PUBLISH gesendet (typischer "online"-Status). Optional, alle Felder leer = keine Nachricht:
    - `onConnectTopic` (string)
    - `onConnectPayload` (string)
    - `onConnectQoS` (number) — 0, 1 oder 2. Standard: 0
    - `onConnectRetain` (boolean) — Standard: false
  - **onDisconnect Message** — Client-seitige Konvention: wird vom Flint-Client vor einem **regulären** Disconnect (Stop / Re-Deploy) als regulärer PUBLISH gesendet, bevor das DISCONNECT-Paket geht. Optional:
    - `onDisconnectTopic` (string)
    - `onDisconnectPayload` (string)
    - `onDisconnectQoS` (number) — Standard: 0
    - `onDisconnectRetain` (boolean) — Standard: false
  - **LastWill** — MQTT-Protokoll-Feature: wird im CONNECT-Paket an den Broker übergeben und vom **Broker** publiziert, wenn der Client **unsauber** abreißt (Keep-Alive-Timeout, Verbindungsverlust ohne sauberes DISCONNECT). Optional:
    - `lastWillTopic` (string)
    - `lastWillPayload` (string)
    - `lastWillQoS` (number) — 0, 1 oder 2. Standard: 0
    - `lastWillRetain` (boolean) — Standard: false
    - `lastWillDelayInterval` (number, v5) — Verzögerung in Sekunden, bevor der Broker den LastWill publiziert. Standard: 0. Im v3.1.1-Modus ignoriert

  onConnect, onDisconnect und LastWill sind drei separate Messages mit klar getrennten Triggern: **onConnect** nach erfolgreicher Verbindung, **onDisconnect** beim sauberen Disconnect (vom Client gesendet), **LastWill** beim unsauberen Disconnect (vom Broker gesendet).

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
│  Protocol Version                             │
│  ┌────────────────────────────────────────┐   │
│  │ MQTT v5 (Default)                  ▼  │   │
│  └────────────────────────────────────────┘   │
│                                               │
│  ┌─────┐                                     │
│  │ TLS │  Keep-Alive: [60]s                   │
│  └─────┘  ☑ Clean Start                       │
│                                               │
│  Session Expiry: [0]s         (v5 only)       │
│                                               │
│  ▼ onConnect (after connect, optional)        │
│  Topic:   [status/flint-abc123          ]     │
│  Payload: [online                       ]     │
│  QoS: [0 ▼]   ☐ Retain                        │
│                                               │
│  ▼ onDisconnect (clean disconnect)            │
│  Topic:   [status/flint-abc123          ]     │
│  Payload: [offline                      ]     │
│  QoS: [0 ▼]   ☐ Retain                        │
│                                               │
│  ▼ LastWill (unclean disconnect, MQTT-spec)   │
│  Topic:   [status/flint-abc123          ]     │
│  Payload: [offline                      ]     │
│  QoS: [0 ▼]   ☐ Retain   Delay: [0]s (v5)     │
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
- **Basis-Konfiguration**:
  - `broker` (string) — ID des referenzierten `mqtt-broker` Config Nodes
  - `mode` (string) — `static` (Default) oder `dynamic`
  - `topic` (string) — MQTT Topic zum Abonnieren, z.B. `sensor/temperature` (nur im Static-Modus relevant)
  - `qos` (number) — Quality of Service: 0, 1 oder 2. Standard: 0

- **MQTT v5 Subscription Options** (alle optional, gelten pro Subscription; im v3.1.1-Modus ignoriert):
  - `noLocal` (boolean, default `false`) — verhindert, dass der Broker dem Client seine eigenen Publishes auf demselben Topic zustellt. Nützlich gegen Echo-Schleifen, wenn ein Flint-Flow auf ein Topic published, das er auch subscribed
  - `retainAsPublished` (boolean, default `false`) — wenn `true`, wird das Retain-Flag der Original-Publish unverändert weitergereicht. Wenn `false` (Default), setzt der Broker das Flag bei Auslieferung auf `0` — Konsumenten können also nicht mehr unterscheiden, ob die Nachricht retained war
  - `retainHandling` (number, default `0`) — Steuert, wann retained Messages beim Subscribe gesendet werden:
    - `0`: bei jedem Subscribe alle retained Messages senden (Default-Verhalten)
    - `1`: retained Messages nur senden, wenn die Subscription neu ist (kein Re-Send beim Re-Subscribe nach Reconnect mit Session)
    - `2`: niemals retained Messages beim Subscribe senden
  - `subscriptionIdentifier` (number, optional) — numerische ID, die der Broker bei jedem Publish, der diese Subscription matcht, zurückliefert. Nützlich bei Dynamic-Mode mit mehreren parallelen Subscriptions, um die Quelle einer Nachricht zu identifizieren

- **MQTT v5 SUBSCRIBE Properties** (alle optional, einmal pro SUBSCRIBE-Paket):
  - `subscribeUserProperties` (object) — String-zu-String-Map, wird als User Properties am SUBSCRIBE-Paket mitgesendet. Selten genutzt; manche Broker werten sie für Authorization-Hooks aus

- **Ausgehende Message** (für jede empfangene MQTT-Nachricht):
  ```json
  {
    "topic": "sensor/temperature",
    "payload": "<empfangene Daten>",
    "qos": 0,
    "retain": false,
    "userProperties": { "source": "sensor-42" },
    "contentType": "application/json",
    "responseTopic": "sensor/temperature/reply",
    "correlationData": "<bytes>",
    "messageExpiry": 60,
    "payloadFormat": 1,
    "subscriptionIdentifier": 42
  }
  ```
  Die v5-Felder werden nur gesetzt, wenn sie in der eingehenden MQTT-Nachricht vorhanden sind. Im v3.1.1-Modus fehlen sie immer.
  - `userProperties` (object) — String-zu-String-Map mit den User Properties aus dem PUBLISH-Paket
  - `contentType` (string) — z.B. `application/json`, `text/plain`
  - `responseTopic` (string) — Topic, auf das eine Antwort publiziert werden soll (Request/Response-Pattern)
  - `correlationData` (bytes) — opake Bytes zur Korrelation von Request und Response
  - `messageExpiry` (number, Sekunden) — verbleibende Lebensdauer der Nachricht; bei Empfang nach Ablauf hätte der Broker sie ohnehin verworfen
  - `payloadFormat` (number, 0 oder 1) — `0` = unspezifiziert/Bytes, `1` = UTF-8 Text. Hint für Konsumenten zur Decodierung
  - `subscriptionIdentifier` (number) — die ID, die beim Subscribe gesetzt wurde. Bei Shared Subscriptions oder mehreren überlappenden Subscriptions kann der Broker mehrere zurückgeben — wir liefern dann ein `[]number`-Array

- **Shared Subscriptions (v5)**: Topic-Pattern `$share/<group>/<topic>` werden transparent unterstützt — die MQTT-Bibliothek leitet diese als gewöhnliche Subscription an den Broker weiter, der die Lastverteilung übernimmt. Im v3.1.1-Fallback würde ein solches Topic literal subscribed, daher ist die Verwendung an v5 gebunden.

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
│  ▼ MQTT v5 Subscription Options (optional)    │
│  ☐ No Local                                   │
│  ☐ Retain As Published                        │
│  Retain Handling: [0 — send always         ▼]│
│  Subscription ID: [          ] (leave empty)  │
│                                               │
│  ▼ MQTT v5 SUBSCRIBE Properties (rare)        │
│  User Properties:                             │
│  ┌──────────────┐ ┌──────────────┐ ┌───┐     │
│  │ key          │ │ value        │ │ × │     │
│  └──────────────┘ └──────────────┘ └───┘     │
│  [+ Add property]                             │
│                                               │
└──────────────────────────────────────────────┘
```

Im Dynamic-Modus wird das Topic-Feld ausgeblendet und unterhalb des Mode-Selectors ein Hinweis-Block gezeigt:

```
ℹ Send msg.action = "subscribe" with msg.payload as
   topic string or array of topics. Existing
   subscriptions are replaced on each call.
```

Im Dynamic-Modus können die v5-Subscription-Options zusätzlich per `msg` überschrieben werden:
- `msg.noLocal` (boolean), `msg.retainAsPublished` (boolean), `msg.retainHandling` (number 0/1/2), `msg.subscriptionIdentifier` (number)
- Fehlt das Feld in der Steuer-Message, gilt der Config-Wert.

### 3. MQTT Publish Node (`mqtt-out`)

- **Canvas**: 1 Input, 0 Outputs (Sink Node)
- **Funktion**: Publiziert eingehende Messages über den konfigurierten Broker auf ein MQTT-Topic
- **Basis-Konfiguration**:
  - `broker` (string) — ID des referenzierten `mqtt-broker` Config Nodes
  - `topic` (string, optional) — MQTT Topic zum Publizieren. Wenn leer, wird `msg.topic` verwendet
  - `qos` (number) — Quality of Service: 0, 1 oder 2. Standard: 0
  - `retain` (boolean) — Retain Flag. Standard: false

- **MQTT v5 Default Properties** (alle optional, gelten als Defaults für jeden Publish; können per `msg` überschrieben werden — siehe unten):
  - `defaultUserProperties` (object) — String-zu-String-Map; wird mit `msg.userProperties` zusammengeführt (msg-Keys gewinnen bei Konflikt)
  - `defaultContentType` (string) — z.B. `application/json`
  - `defaultResponseTopic` (string) — Topic für Antworten (Request/Response-Pattern)
  - `defaultMessageExpiry` (number, Sekunden) — Default-Ablauf für jede gesendete Nachricht
  - `defaultPayloadFormat` (number, 0 oder 1) — `0` = Bytes (Default), `1` = UTF-8 Text. Wenn `1` gesetzt ist und der Payload kein gültiger UTF-8-String ist, wird er trotzdem gesendet (der Broker kann ggf. ablehnen)

  **Bewusst nicht in der Static-Config**:
  - `correlationData` ist per Definition pro-Message (Request/Response-Korrelation) — nur via `msg.correlationData`
  - `topicAlias` wird vom Client transparent verwaltet (Optimierung im Broker-Manager) — nicht user-konfigurierbar
  - `subscriptionIdentifier` ist nur im PUBLISH **vom Broker** an den Subscriber relevant, nie im Outbound-Publish

- **Eingehende Message**:
  - `msg.payload` wird als MQTT-Payload gesendet
  - `msg.topic` wird als Fallback-Topic verwendet wenn keins konfiguriert ist
  - **MQTT v5 (optional)** — folgende Felder überschreiben die Default-Properties aus der Config (im v3.1.1-Modus ignoriert):
    - `msg.userProperties` (object) — wird mit `defaultUserProperties` zusammengeführt; bei gleichem Key gewinnt msg
    - `msg.contentType` (string) — überschreibt `defaultContentType`
    - `msg.responseTopic` (string) — überschreibt `defaultResponseTopic`
    - `msg.correlationData` (string/bytes) — kein Config-Default, nur per Message
    - `msg.messageExpiry` (number, Sekunden) — überschreibt `defaultMessageExpiry`
    - `msg.payloadFormat` (number, 0 oder 1) — überschreibt `defaultPayloadFormat`
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
│  ▼ MQTT v5 Default Properties (optional)      │
│  Content Type    [application/json         ]  │
│  Response Topic  [                         ]  │
│  Message Expiry  [    ] sec                   │
│  Payload Format  [0 — bytes              ▼]   │
│                                               │
│  User Properties:                             │
│  ┌──────────────┐ ┌──────────────┐ ┌───┐     │
│  │ source       │ │ flint-flow-1 │ │ × │     │
│  └──────────────┘ └──────────────┘ └───┘     │
│  [+ Add property]                             │
│                                               │
│  ℹ  msg.userProperties / msg.contentType /    │
│     msg.responseTopic / msg.messageExpiry /   │
│     msg.payloadFormat überschreiben die       │
│     Defaults pro Message.                     │
│     msg.correlationData ist nur per Message.  │
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
        "protocolVersion": "5",
        "username": "user",
        "password": "",
        "keepalive": 60,
        "cleanStart": true,
        "sessionExpiry": 0,
        "useTLS": false,
        "onConnectTopic": "status/flint-abc123",
        "onConnectPayload": "online",
        "onConnectQoS": 0,
        "onConnectRetain": true,
        "onDisconnectTopic": "status/flint-abc123",
        "onDisconnectPayload": "offline",
        "onDisconnectQoS": 0,
        "onDisconnectRetain": true,
        "lastWillTopic": "status/flint-abc123",
        "lastWillPayload": "offline",
        "lastWillQoS": 0,
        "lastWillRetain": true,
        "lastWillDelayInterval": 0
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

- `github.com/eclipse/paho.golang/paho` — MQTT v5 Client Library (Eclipse Paho v5)
- `github.com/eclipse/paho.golang/autopaho` — Connection-Manager mit Auto-Reconnect-Logik um den v5-Client herum
- **Hinweis**: Die ältere `github.com/eclipse/paho.mqtt.golang` Library spricht nur v3.1.1 und ist daher nicht ausreichend. Der v5-Fallback auf v3.1.1 wird über die Protocol-Negotiation des Brokers abgewickelt — die `paho.golang` Library kann das für unsere Zwecke ausreichend abbilden, ggf. muss bei `protocolVersion=3.1.1` explizit gegen die alte Library oder einen separaten v3-Pfad gefahren werden.

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

Der MQTT-Client implementiert automatisches Reconnect über `paho.golang/autopaho`:

- `autopaho.NewConnection` übernimmt Verbindungsaufbau und Reconnect mit konfigurierbarem Backoff
- Bei v5-Sessions mit `sessionExpiry > 0` reaktiviert der Broker bestehende Subscriptions; bei `cleanStart=true` muss Flint nach Reconnect alle Subscriptions selbst wiederherstellen
- Bei Verbindungsverlust: Status auf Gelb ("reconnecting...")
- Bei erfolgreicher Wiederverbindung: Subscriptions automatisch erneuern (sofern nicht durch Session bereits aktiv), Status auf Grün
- Bei dauerhaftem Fehler: Status auf Rot mit Fehlermeldung
- Lehnt der Broker die gewählte Protokoll-Version ab, schlägt der CONNECT mit Status Rot und Reason-Code in der Fehlermeldung fehl — der Anwender muss in der Broker-Config explizit auf v3.1.1 umstellen

### onConnect / onDisconnect / LastWill – Lifecycle

- **onConnect**: nach jedem erfolgreichen CONNACK (auch nach Reconnect) feuert der Broker-Manager als erste Aktion den onConnect-Publish, bevor irgendein anderer Node Subscriptions registrieren oder publizieren darf. Dadurch sehen Konsumenten konsistent erst „online", dann den eigentlichen Datenstrom
- **onDisconnect**: beim regulären Stop / Re-Deploy publiziert der Broker-Manager zuerst die onDisconnect-Message, wartet auf das ACK (bei QoS > 0) bzw. den Flush (bei QoS 0) und schickt erst dann das DISCONNECT-Paket
- **LastWill**: wird im CONNECT-Paket an den Broker übergeben und ausschließlich vom Broker selbst publiziert, wenn die Verbindung unsauber abreißt. Bei einem regulären DISCONNECT verwirft der Broker den LastWill (so spezifiziert) — daher braucht es die separate onDisconnect-Message
- onConnect und onDisconnect sind **Client-seitige Konvention** (reguläre PUBLISH-Pakete), LastWill ist das **MQTT-Protokoll-Feature**
- Wenn onConnect und onDisconnect auf dasselbe Topic mit `retain=true` publizieren, ist LastWill mit `retain=true` empfehlenswert, damit der retained-Status bei jeder Disconnect-Variante konsistent bleibt

### Broker-Dropdown im Frontend

Das Broker-Dropdown im MQTT Node Config Panel zeigt alle `mqtt-broker` Config Nodes aus `flowStore.configs`:

```typescript
const mqttBrokers = computed(() =>
  flowStore.configs.filter(c => c.type === 'mqtt-broker')
)
```

Der "+" Button neben dem Dropdown öffnet den `MqttBrokerConfig.vue` Dialog. Nach dem Speichern wird der neue Broker automatisch im Dropdown ausgewählt.

### Message-Mapping (mqtt-in)

Empfangene MQTT-Nachrichten werden in das Flint Message-Format übersetzt. v5-Properties werden, falls vom Broker mitgeliefert, in die ausgehende Message übernommen:

```go
func (n *MqttInNode) onPublish(p *paho.Publish) {
    msg := NewMessage()
    msg.Set("topic", p.Topic)
    msg.Set("payload", string(p.Payload))
    msg.Set("qos", int(p.QoS))
    msg.Set("retain", p.Retain)

    if props := p.Properties; props != nil { // MQTT v5 Properties
        if len(props.User) > 0 {
            up := map[string]string{}
            for _, kv := range props.User {
                up[kv.Key] = kv.Value
            }
            msg.Set("userProperties", up)
        }
        if props.ContentType != "" {
            msg.Set("contentType", props.ContentType)
        }
        if props.ResponseTopic != "" {
            msg.Set("responseTopic", props.ResponseTopic)
        }
        if len(props.CorrelationData) > 0 {
            msg.Set("correlationData", props.CorrelationData)
        }
        if props.MessageExpiry != nil {
            msg.Set("messageExpiry", *props.MessageExpiry)
        }
    }

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

- **MQTT v5 Feature-Umfang**:
  - **Drin**: User Properties (CONNECT, PUBLISH, SUBSCRIBE — outbound + inbound), Message Expiry Interval, Content Type, Response Topic, Correlation Data, Payload Format Indicator, Shared Subscriptions, Subscription Identifiers (set + receive), Subscription Options (No Local, Retain As Published, Retain Handling), LastWill mit Will Delay Interval, Session Expiry Interval, Clean Start
  - **Bewusst nicht in v1**:
    - **Topic Aliases**: werden vom Client-Manager transparent verwaltet, sind aber nicht user-konfigurierbar (z.B. `Topic Alias Maximum` im CONNECT). Default 0 = aus
    - **Reason-Code-Routing** auf Catch-Outputs: ACK-Reason-Codes (PUBACK, SUBACK, UNSUBACK, DISCONNECT) werden geloggt, aber nicht als separater Flow-Output bereitgestellt
    - **Enhanced Authentication** (Auth-Properties, AUTH-Paket, SASL-Style-Flows): nicht in v1
    - **Flow Control**: `Receive Maximum`, `Maximum Packet Size`, `Server Keep Alive`, `Server Reference` — Default-Werte des Clients werden verwendet, keine UI-Konfiguration
    - **Request/Response-Information** im CONNECT (`Request Response Information`, `Request Problem Information`): default true für Problem Information, ansonsten nicht konfigurierbar
    - **CONNECT User Properties**: aktuell nicht im UI; können über Library-API gesetzt werden, falls jemand sie brauchen sollte (Erweiterung später)
- **Wildcard-Topics**: `+` und `#` Wildcards in Topics werden für den ersten Wurf nicht explizit validiert, funktionieren aber transparent über den MQTT-Client
- **Erweiterte TLS-Konfiguration**: Client-Zertifikate, CA-Bundle etc. — nicht in v1
- **Credential Encryption**: Passwörter werden vorerst im Klartext in der Config gespeichert. Verschlüsselung wird separat über ein Credential-System adressiert
