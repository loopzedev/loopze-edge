# Issue: OPC UA Nodes – Read / Subscribe / Write mit Server-Konfiguration

## Status: Done (v1)

**Implementiert (Phase 0–7):**
- `opcua-server` Config Node mit AutoReconnect, State-Watcher, Test-Connection-Endpoint
- `opcua-read` Node: static / triggered / dynamic Modes, Output-Shapes single/array/object, Per-Item-StatusCodes
- `opcua-write` Node: static / dynamic Modes, Type-Coercion mit Range-Checks, Type-Cache, Status-Pulses, ExtensionObject-Encoding über TypeResolver
- `opcua-subscribe` Node: MonitoredItems mit Sampling/Queue/Deadband, Recovery nach Reconnect, Output-Shapes per-item/batch
- ExtensionObject-Handling: Marker-Type-Registrierung gegen gopcua, schema-driven Encode/Decode für Struct/Enum/Optional/Union/Nested/Array
- Type-Resolver lädt `DataTypeDefinition` mit Recursion-Schutz, geteilter Cache pro Server, Reset bei Reconnect
- Address-Space-Browser: Modal mit Lazy-Tree, NodeClass-Filter, Multi-/Single-Select, Detail-Pane mit DataType-Info, integriert in alle drei Operations-Nodes
- Tests gegen den extern gepflegten Deno-OPCUA-Test-Server via `LOOPZE_OPCUA_TEST_ENDPOINT`

**Bewusst auf später verschoben (eigene Issues):**
- Cert-Pinning (TOFU): Feld in der Server-Config vorhanden, Fingerprint-Persistenz + Vergleich nicht implementiert
- Browse-Fallback für Server ohne `DataTypeDefinition` (1.03-Server-Kompat)
- Subscription-Sharing zwischen Nodes mit identischem PublishingInterval
- `/api/v1/opcua/read-attributes` und `/api/v1/opcua/resolve-path` Endpoints (Browse-Endpoint deckt v1-UI-Bedarf ab)
- Browse-Pagination via ContinuationPoint im Frontend (Backend liefert ihn durch)

## Problembeschreibung

OPC UA ist **der** Standard für Maschinenkommunikation in der Industrie (Werkzeugmaschinen, Roboter, SPSen, MES, SCADA, Edge-Gateways). LOOPZE braucht erstklassige OPC-UA-Unterstützung, damit es als ernstzunehmende Edge-Automation-Plattform im industriellen Umfeld eingesetzt werden kann.

Drei neue Node-Typen — `opcua-read`, `opcua-subscribe`, `opcua-write` — decken die drei zentralen OPC-UA-Operationen ab. Sie referenzieren einen gemeinsamen **OPC UA Server Config Node** (`opcua-server`), der die Verbindungs- und Sicherheitsparameter bündelt. Das Config-Node-Konzept wurde mit dem MQTT-Issue ([NODE_MQTT.md](NODE_MQTT.md)) eingeführt und wird hier wiederverwendet.

**Leitprinzip**: Bis auf die Server-Wahl (statisch, pro Node fest) muss **jede** Operation auch dynamisch zur Laufzeit per Eingangs-Message gesteuert werden können — NodeIDs, Werte, Subscription-Listen. Ein OPC-UA-Node, der nur statisch konfiguriert werden kann, ist in der Praxis nicht ausreichend, weil reale Anlagen oft hunderte von Variablen haben, deren Auswahl erst zur Laufzeit feststeht (über UI, Rezept, Auftragsdaten, etc.).

**Scope**: OPC UA Binary (`opc.tcp://`) mit Standard-Security-Modes. Read, Subscribe (MonitoredItems), Write. **ExtensionObjects (Strukturen) werden erstklassig unterstützt** — beim Read werden sie automatisch in JSON-Objekte konvertiert, beim Write wird ein JSON-Objekt anhand der server-seitigen DataTypeDefinition wieder in ein ExtensionObject encodiert. Ohne diese Funktion ist OPC UA gegen reale SPSen (Siemens UDTs, Beckhoff-Strukturen, B&R-Datentypen, AAS-Submodels, EUInformation/Range/AnalogItem-Configs) nicht praktisch nutzbar. **Nicht** in v1: Method Calls, HistoryRead, Browse-as-Node, Events/Alarms, Auto-Discovery, X.509-Client-Cert-Generierung im UI. Diese kommen als separate Issues nach.

## Übersicht

| Node-Typ | Typ-ID | Canvas Inputs | Canvas Outputs | Beschreibung |
|---|---|---|---|---|
| **OPC UA Server** | `opcua-server` | — | — | Config Node: Endpoint, Security, Authentifizierung |
| **OPC UA Read** | `opcua-read` | 0–1 | 1 | Einmaliges Lesen von einem oder mehreren Nodes |
| **OPC UA Subscribe** | `opcua-subscribe` | 0–1 | 1 | MonitoredItems mit Push-Updates bei Wertänderung |
| **OPC UA Write** | `opcua-write` | 1 | 0–1 | Schreiben von Werten in einen oder mehrere Nodes |

```
                          OPC UA Server (extern, z.B. SPS/Maschine)
                          ┌──────────────────────────┐
Flow                      │  ns=2;s=Temp.Sensor.1    │
┌─────────────────────┐   │  ns=2;s=Motor.Speed      │
│  [Inject 1s]         │   │  ns=2;s=Setpoint.Target  │
│      ↓               │   │                          │
│  [OPCUA Read] ───────┼───┤ Read                     │
│      ↓               │   │                          │
│  [Function]          │   │ Subscribe                │
│      ↓               ├───┤ (MonitoredItems)         │
│  [OPCUA Write] ──────┼───┤ Write                    │
│                      │   │                          │
└─────────────────────┘   └──────────────────────────┘
```

## Anforderungen

### 1. Config Node: OPC UA Server (`opcua-server`)

Der OPC UA Server ist ein **Config Node** — analog zum MQTT-Broker. Er erscheint nicht auf dem Canvas und wird von `opcua-read`, `opcua-subscribe` und `opcua-write` Nodes referenziert. Mehrere Operations-Nodes, die denselben Server referenzieren, teilen sich **eine** OPC-UA-Session.

- **Typ-ID**: `opcua-server`
- **Konfigurationsfelder**:
  - `name` (string) — Anzeigename, z.B. "SPS Linie 3"
  - `endpointUrl` (string) — z.B. `opc.tcp://192.168.1.50:4840`
  - `securityPolicy` (string) — `None` (Default) | `Basic128Rsa15` | `Basic256` | `Basic256Sha256` | `Aes128_Sha256_RsaOaep` | `Aes256_Sha256_RsaPss`
  - `securityMode` (string) — `None` (Default) | `Sign` | `SignAndEncrypt`. Bei `securityPolicy=None` wird `securityMode=None` erzwungen
  - `authMode` (string) — `anonymous` (Default) | `username` | `certificate`
  - `username` (string, optional) — bei `authMode=username`
  - `password` (string, optional) — bei `authMode=username`
  - `clientCertFile` (string, optional) — Pfad zur Client-Zertifikatsdatei (PEM/DER), bei Bedarf für `securityMode != None` oder `authMode=certificate`
  - `clientKeyFile` (string, optional) — Pfad zum privaten Schlüssel
  - `applicationUri` (string) — Default: `urn:loopze:client`. Muss zum Subject-AltName im Client-Cert passen, sonst lehnen viele Server die Verbindung ab
  - `applicationName` (string) — Default: `LOOPZE OPC UA Client`
  - `sessionTimeout` (number, ms) — Default: `60000`
  - `requestTimeout` (number, ms) — Default: `5000`. Timeout für einzelne Service-Calls (Read/Write/CreateSubscription)
  - `serverCertTrust` (string) — `prompt` (Default in v1: log-only, akzeptiert) | `pinned` (akzeptiert nur das Cert mit gespeichertem Fingerprint) | `system` (akzeptiert alles im OS-Truststore). v1 implementiert `prompt`/`pinned` — TOFU-Pattern: erstes Cert wird geloggt, kann manuell gepinnt werden
  - `keepaliveInterval` (number, ms) — Default: `10000`. Sendet leere Reads, um die Session am Leben zu halten

- **Verbindung & Reconnect**:
  - Beim Deploy wird die Session aufgebaut. Verbindungsverlust → automatischer Reconnect mit exponentiellem Backoff (1s, 2s, 4s, ..., max 30s)
  - Bei v5-äquivalenter Session-Recovery: Subscriptions und MonitoredItems werden vom Client-State automatisch wiederhergestellt (Library: `gopcua/opcua` unterstützt das via `subscription.Recreate()`)
  - Status während Reconnect: gelb mit "reconnecting…"

- **Properties-Dialog**:

```
┌──────────────────────────────────────────────┐
│  OPC UA Server                                │
├──────────────────────────────────────────────┤
│                                               │
│  Name                                         │
│  [ SPS Linie 3                            ]   │
│                                               │
│  Endpoint URL                                 │
│  [ opc.tcp://192.168.1.50:4840            ]   │
│                                               │
│  ▼ Security                                   │
│  Policy:  [ None                        ▼ ]   │
│  Mode:    [ None                        ▼ ]   │
│                                               │
│  Authentication                               │
│  ( • ) Anonymous                              │
│  (   ) Username / Password                    │
│  (   ) Certificate                            │
│                                               │
│  Username   [                               ] │
│  Password   [ •••••                         ] │
│                                               │
│  ▼ Client Identity (advanced)                 │
│  Application URI  [ urn:loopze:client       ]  │
│  Application Name [ LOOPZE OPC UA Client    ]  │
│  Client Cert File [                        ]  │
│  Client Key  File [                        ]  │
│                                               │
│  ▼ Timing (advanced)                          │
│  Session Timeout    [ 60000 ] ms              │
│  Request Timeout    [  5000 ] ms              │
│  Keepalive Interval [ 10000 ] ms              │
│                                               │
│  Server Cert Trust: [ Pin on first use   ▼ ]  │
│                                               │
│  [ Test Connection ]                          │
│                                               │
│  [ Speichern ]   [ Abbrechen ]                │
└──────────────────────────────────────────────┘
```

Der **Test Connection** Button macht einen einmaligen CONNECT/CloseSession und meldet Erfolg/Fehlercode zurück (StatusCode aus dem Server). Hilft beim Konfigurieren ohne Deploy.

### 2. OPC UA Read Node (`opcua-read`)

- **Canvas**:
  - Static Mode: 0 Inputs, 1 Output — Lesen kann zeitgesteuert (per `interval`), einmalig beim Start oder gar nicht passieren (dann nur als Sub-Node eines anderen Auslösers verwendbar — siehe Mixed Mode unten)
  - Triggered Mode: 1 Input, 1 Output — jede Eingangs-Message löst einen Read aus
  - Dynamic Mode: 1 Input, 1 Output — die zu lesenden NodeIDs kommen aus der Eingangs-Message

- **Funktion**: Liest Werte einer Liste von OPC-UA-Nodes via `Read`-Service. Output ist eine Message mit dem oder den gelesenen Werten plus Metadaten (StatusCode, Timestamps).

- **Basis-Konfiguration**:
  - `server` (string) — ID des referenzierten `opcua-server` Config Nodes
  - `mode` (string) — `static` | `triggered` (Default) | `dynamic`
  - `nodeIds` (string[]) — Liste der zu lesenden NodeIDs, z.B. `["ns=2;s=Temp", "ns=2;i=42"]`. Im `dynamic`-Modus optional als Default wenn `msg` nichts liefert
  - `attribute` (string) — Default: `Value`. Zu lesendes Attribut. v1: nur `Value` und `Description` werden im UI angeboten — andere Attributes (DisplayName, BrowseName, DataType, AccessLevel, ...) per `msg.attribute` möglich
  - `outputShape` (string) — `single` | `array` | `object` (siehe unten)
  - `interval` (number, ms, nur `static`) — Lese-Intervall. `0` = nur einmal beim Start
  - `startupRead` (boolean, nur `static`) — Default `true`. Read sofort beim Deploy, nicht erst nach `interval`

- **NodeID-Syntax** (Standard-OPC-UA-Form):
  - `ns=<index>;i=<integer>` — numerischer Identifier
  - `ns=<index>;s=<string>` — String-Identifier
  - `ns=<index>;g=<guid>` — GUID-Identifier
  - `ns=<index>;b=<base64>` — Opaque-Identifier (Bytes)
  - Beispiele: `ns=2;s=Demo.Static.Scalar.Int32`, `ns=0;i=2258` (CurrentTime)

- **Output-Shape**:

| Shape | `msg.payload` Form | Wann sinnvoll |
|---|---|---|
| `single` | Skalar (nur bei genau 1 NodeID) | Klassischer Read auf 1 Wert: `msg.payload = 23.5` |
| `array` (Default bei >1) | `[{nodeId, value, statusCode, sourceTimestamp, serverTimestamp}, …]` | Mehrere Werte, Reihenfolge der Konfiguration zählt |
| `object` | `{ "<nodeId>": <value>, … }` oder `{ "<nodeId>": {value, statusCode, …}, … }` | Map-Zugriff im Folge-Node, NodeID als Key |

  Bei `single`/`array`/`object` mit reduziertem Output (`includeMetadata=false`, Default `true`) wird nur der Value direkt gemappt. Mit Metadaten kommen StatusCode, SourceTimestamp, ServerTimestamp dazu.

- **Eingehende Message** (Triggered- und Dynamic-Modus):
  - `msg.action = "read"` — explizite Aktion. Im Triggered-Modus optional (jeder Input triggert), im Dynamic-Modus nötig
  - `msg.nodeIds` (string oder string[]) — überschreibt die Config-NodeIDs für diesen Read
  - `msg.attribute` (string) — überschreibt das zu lesende Attribut
  - Im Dynamic-Modus ohne `msg.nodeIds` und ohne Default-NodeIDs in der Config wird der Read mit Status-Warnung übersprungen (kein Output)

- **Ausgehende Message** (Beispiel mit `outputShape=array`, `includeMetadata=true`):
  ```json
  {
    "payload": [
      {
        "nodeId": "ns=2;s=Temp",
        "value": 23.5,
        "dataType": "Double",
        "statusCode": "Good",
        "statusCodeRaw": 0,
        "sourceTimestamp": "2026-04-28T10:15:23.123Z",
        "serverTimestamp": "2026-04-28T10:15:23.124Z"
      },
      {
        "nodeId": "ns=2;s=MotorState",
        "value": {
          "Speed": 1450.5,
          "Torque": 12.3,
          "FaultCode": 0,
          "Running": true,
          "Mode": "Auto"
        },
        "dataType": "ExtensionObject",
        "structureType": "ns=2;i=3001",
        "structureName": "MotorStatusType",
        "statusCode": "Good",
        "statusCodeRaw": 0,
        "sourceTimestamp": "2026-04-28T10:15:23.123Z",
        "serverTimestamp": "2026-04-28T10:15:23.124Z"
      }
    ]
  }
  ```

  **ExtensionObjects** (server-seitige Strukturen / UDTs) werden automatisch in JSON-Objekte umgewandelt. Die Feldnamen entsprechen denen der DataTypeDefinition (siehe Abschnitt „ExtensionObject-Handling"). Verschachtelte Strukturen werden rekursiv konvertiert, Arrays bleiben Arrays. Die Original-NodeID des Strukturtyps und der Klartextname kommen als `structureType` / `structureName` mit, damit der Anwender weiß, womit er es zu tun hat (und damit ein nachgeschalteter Write-Node die Type-Info nicht verlieren muss, falls die Struktur 1:1 zurückgeschrieben werden soll).

  Bei einzelnen Reads mit Bad-Status: `statusCode` als String (z.B. `"BadNodeIdUnknown"`), `value = null`. Der Read als Ganzes wirft nur dann einen Catch-baren Fehler, wenn der Service-Call selbst fehlschlägt (Verbindung weg, ungültige Session) — einzelne Bad-StatusCodes pro NodeID sind reguläre Output-Daten und werden weitergereicht.

- **Status-Anzeige** (via `SetStatus`):
  - Grün: `verbunden · <n> Node(s)` (oder `verbunden · idle` im Dynamic ohne aktive Reads)
  - Gelb: `verbinde…` / `reconnecting…`
  - Rot: `getrennt` oder konkrete Fehlermeldung (Service Result)

- **Properties-Panel**:

```
┌──────────────────────────────────────────────┐
│  OPC UA Read                                  │
├──────────────────────────────────────────────┤
│                                               │
│  Server                                       │
│  [ SPS Linie 3                       ▼ ] [+]  │
│  Edit server config                           │
│                                               │
│  Mode                                         │
│  ( ) Static (interval)                        │
│  (•) Triggered (msg in)                       │
│  ( ) Dynamic (msg.nodeIds)                    │
│                                               │
│  Node IDs                                     │
│  ┌─────────────────────────────────────┐ ┌─┐  │
│  │ Temperature  · ns=2;s=Temp          │ │×│  │
│  └─────────────────────────────────────┘ └─┘  │
│  ┌─────────────────────────────────────┐ ┌─┐  │
│  │ ns=2;i=42                           │ │×│  │
│  └─────────────────────────────────────┘ └─┘  │
│  [+ Add NodeID]   [ 🔎 Browse server… ]       │
│                                               │
│  Attribute            [ Value           ▼ ]   │
│                                               │
│  Output Shape         [ Array (default) ▼ ]   │
│  ☑ Include metadata (statusCode, timestamps)  │
│                                               │
│  ▼ Static-Mode Options (only if static)       │
│  Interval         [ 1000 ] ms                 │
│  ☑ Read at startup                            │
│                                               │
└──────────────────────────────────────────────┘
```

### 3. OPC UA Subscribe Node (`opcua-subscribe`)

Der wichtigste Read-Pfad in der Praxis: **Push statt Poll**. Der Node legt eine OPC-UA-Subscription an, registriert MonitoredItems und gibt jede vom Server gepushte Wertänderung als Output-Message weiter.

- **Canvas**:
  - Static Mode: 0 Inputs, 1 Output
  - Dynamic Mode: 1 Input, 1 Output — Steuer-Messages am Input, Daten am Output (Steuer-Messages werden **nicht** durchgereicht)

- **Basis-Konfiguration**:
  - `server` (string) — ID des `opcua-server` Config Nodes
  - `mode` (string) — `static` (Default) | `dynamic`
  - `monitoredItems` (object[]) — pro Item:
    - `nodeId` (string)
    - `attribute` (string, Default `Value`)
    - `samplingInterval` (number, ms, Default `1000`) — wie oft der Server intern abtastet. `-1` = Server-Default, `0` = "so schnell wie möglich"
    - `queueSize` (number, Default `1`) — wie viele Werte der Server puffert, falls Updates schneller anfallen als die Publish-Round
    - `discardOldest` (boolean, Default `true`) — bei vollem Queue: ältesten oder neuesten verwerfen
    - `deadband` (object, optional) — Wertänderungs-Filter:
      - `type`: `none` | `absolute` | `percent`
      - `value`: Schwellwert (nur bei `absolute`/`percent`)
  - `publishingInterval` (number, ms, Default `500`) — wie oft die Subscription Daten an den Client pusht
  - `lifetimeCount` (number, Default `60`) — Anzahl Publish-Intervalle ohne Aktivität, bevor der Server die Subscription als tot markiert
  - `keepAliveCount` (number, Default `10`) — Anzahl Publish-Intervalle ohne Daten, bevor ein leeres Keep-Alive gesendet wird
  - `priority` (number, Default `0`)
  - `outputShape` (string) — `per-item` (Default) | `batch`:
    - `per-item`: jede Wertänderung wird als eigene Message ausgegeben (`msg.nodeId`, `msg.payload = value`, `msg.statusCode`, `msg.sourceTimestamp`)
    - `batch`: alle innerhalb einer PublishResponse zusammenkommenden Updates werden als ein Array in `msg.payload` ausgegeben

- **Static Mode**:
  - Beim Deploy werden Subscription und alle MonitoredItems angelegt
  - Bei Wertänderung pusht der Server → Output-Message
  - Beim Re-Deploy / Stop wird die Subscription `DeleteSubscriptions` und Items implizit aufgeräumt

- **Dynamic Mode**:
  - Beim Deploy hat der Node **keine** MonitoredItems, nur eine leere Subscription wartet
  - Steuer-Messages am Input:
    - `msg.action = "subscribe"` mit `msg.payload` als:
      - String → ein NodeID (Standard-Optionen aus Config)
      - String-Array → mehrere NodeIDs (Standard-Optionen)
      - Object oder Object-Array → vollständige `monitoredItems`-Spezifikation analog zur Static-Config
    - **Bei jedem `subscribe` werden zuerst alle bestehenden MonitoredItems entfernt**, dann die neuen angelegt. Diff-Optimierung (nur unverändert gebliebene Items behalten) ist möglich, aber nicht zwingend für v1
    - `msg.action = "unsubscribe"` mit `msg.payload` als String/String-Array → gezielt einzelne Items entfernen, ohne den Rest anzufassen
    - `msg.action = "clear"` → alle Items entfernen
  - Leeres Array oder leerer String bei `subscribe` wirkt wie `clear`
  - Per-Message-Override der Subscription-weiten Defaults:
    - `msg.publishingInterval`, `msg.samplingInterval`, `msg.queueSize`, `msg.deadband` — gelten für die mit dieser Steuer-Message angelegten Items

- **Status-Anzeige**:
  - Grün: `aktiv · <n> Items` (Static) bzw. `aktiv · <n> Items` / `aktiv · idle` (Dynamic)
  - Gelb: `verbinde…`
  - Rot: Fehlercode der Subscription (z.B. `BadTooManyMonitoredItems`)

- **Properties-Panel** (Auszug, der MonitoredItems-Editor ist die zentrale Komponente):

```
┌──────────────────────────────────────────────┐
│  OPC UA Subscribe                             │
├──────────────────────────────────────────────┤
│                                               │
│  Server   [ SPS Linie 3              ▼ ] [+]  │
│  Mode     ( • ) Static  ( ) Dynamic           │
│                                               │
│  Monitored Items                              │
│  ┌─────────────────────────────────────────┐  │
│  │ ≡ NodeID  [ ns=2;s=Temp           ]  ✕  │  │
│  │   Sampling [1000]ms  Queue [1] ☑oldest  │  │
│  │   Deadband [none ▼]                     │  │
│  ├─────────────────────────────────────────┤  │
│  │ ≡ NodeID  [ ns=2;s=Pressure       ]  ✕  │  │
│  │   Sampling [500]ms  Queue [10] ☑oldest  │  │
│  │   Deadband [absolute ▼]  Value [0.5]    │  │
│  └─────────────────────────────────────────┘  │
│  [+ Add MonitoredItem]                        │
│                                               │
│  ▼ Subscription Defaults                      │
│  Publishing Interval [ 500 ] ms               │
│  Lifetime Count      [  60 ]                  │
│  KeepAlive Count     [  10 ]                  │
│  Priority            [   0 ]                  │
│                                               │
│  Output Shape  [ Per-item (one msg per change) ▼ ] │
│                                               │
└──────────────────────────────────────────────┘
```

Im Dynamic-Modus wird der MonitoredItems-Editor durch einen Hinweis-Block ersetzt:

```
ℹ Send msg.action = "subscribe" with msg.payload as
   NodeID string, array of NodeIDs, or array of
   {nodeId, samplingInterval, queueSize, deadband}.
   Existing items are replaced on each subscribe.
   Use action="unsubscribe" or "clear" to remove items.
```

### 4. OPC UA Write Node (`opcua-write`)

- **Canvas**:
  - 1 Input, 1 Output (optional, Default 1) — der Output trägt das Write-Ergebnis (StatusCode pro NodeID), kann auf 0 gesetzt werden für reine Sink-Verwendung

- **Basis-Konfiguration**:
  - `server` (string) — ID des `opcua-server` Config Nodes
  - `mode` (string) — `static` (Default) | `dynamic`
  - `writes` (object[], static) — pro Write:
    - `nodeId` (string)
    - `attribute` (string, Default `Value`)
    - `dataType` (string) — `Boolean` | `SByte` | `Byte` | `Int16` | `UInt16` | `Int32` | `UInt32` | `Int64` | `UInt64` | `Float` | `Double` | `String` | `DateTime` | `ByteString` | `ExtensionObject` | `Variant`. Wird benötigt, weil OPC UA strikt typisiert ist und der Server bei falschem Typ den Write ablehnt
    - `structureType` (string, optional, nur bei `dataType=ExtensionObject`) — NodeID des Strukturtyps, z.B. `ns=2;i=3001`. Wird benötigt, damit der Node weiß, gegen welche DataTypeDefinition er das eingehende JSON-Objekt encoden muss. Bei leerer Angabe + aktivem Type-Cache wird die Information aus einem vorherigen Read übernommen
    - `valueSource` (string) — `static` | `msg`:
      - `static`: `value` (string oder JSON, in den Ziel-Datentyp konvertiert)
      - `msg`: Wert wird aus einem Message-Pfad gelesen (Default: `payload`, alternativ `payload.<key>` etc.)
    - `valuePath` (string, nur bei `valueSource=msg`, Default `payload`)
  - `passthrough` (boolean, Default `false`) — Eingangs-Message wird (mit angereichertem `msg.writeResult`) am Output durchgereicht. Wenn `false`, geht eine neue Message mit dem Result raus, falls der Output > 0 ist

- **Eingehende Message**:
  - **Static Mode**: jede Eingangs-Message triggert die in der Config definierten Writes. Werte aus `msg` werden gemäß `valuePath` extrahiert
  - **Dynamic Mode**: `msg.writes` enthält die Write-Spezifikation als Array von Objekten:
    ```json
    {
      "writes": [
        { "nodeId": "ns=2;s=Setpoint", "value": 42.5, "dataType": "Double" },
        { "nodeId": "ns=2;s=Mode",     "value": 3,    "dataType": "Int32"  },
        {
          "nodeId": "ns=2;s=MotorCmd",
          "dataType": "ExtensionObject",
          "structureType": "ns=2;i=3010",
          "value": {
            "TargetSpeed": 1500.0,
            "AccelRamp":   200,
            "Direction":   "CW",
            "EnableLimits": true
          }
        }
      ]
    }
    ```
  - Convenience-Form für einen einzelnen Write:
    ```json
    { "nodeId": "ns=2;s=Setpoint", "payload": 42.5, "dataType": "Double" }
    ```
    `dataType` ist optional, wenn der Server-seitige Typ über vorheriges Read bekannt ist und gecached wird (siehe „Type-Caching" unten).

- **Type-Coercion**: Eingehende Werte werden in den Ziel-OPC-UA-Typ konvertiert. Beispiele:
  - JSON-Number → `Int32` / `Float` / `Double` (mit Range-Check, sonst `BadOutOfRange`)
  - JSON-String `"true"`/`"false"`/`"1"`/`"0"` → `Boolean`
  - JSON-String → `DateTime` (RFC3339)
  - JSON-Array von Numbers → `ByteString` (für Binärdaten, analog zu mqtt-out)
  - JSON-Object → `ExtensionObject` (siehe „ExtensionObject-Handling" unten). Felder werden gegen die DataTypeDefinition gemappt; fehlende Felder werden mit Typ-Defaults aufgefüllt (0 / `false` / `""` / `null`); überzählige Felder werden ignoriert (mit Warnung im Log)

  Bei Konversionsfehlern: Write wird **nicht** abgesetzt, der Fehler erscheint im `writeResult` als `BadTypeMismatch` mit Klartext-Begründung.

- **Type-Caching** (Optimierung): Beim ersten Write auf eine NodeID liest der Node das `DataType`-Attribut des Servers und cached es. Danach kann `dataType` in `msg.writes` weggelassen werden. Cache wird beim Reconnect verworfen. Im UI deaktivierbar via `disableTypeCache` (Default `false`).

- **Ausgehende Message** (am Output, falls vorhanden):
  ```json
  {
    "writeResult": [
      { "nodeId": "ns=2;s=Setpoint", "statusCode": "Good", "statusCodeRaw": 0 },
      { "nodeId": "ns=2;s=Mode",     "statusCode": "BadTypeMismatch", "statusCodeRaw": 2147614720 }
    ],
    "allGood": false
  }
  ```

  `allGood` ist `true`, wenn alle Writes Status `Good` zurückgegeben haben — praktisch für nachgeschaltete Switches.

- **Status-Anzeige**:
  - Grün: `bereit · <n> Writes` (Static) / `bereit · idle` (Dynamic)
  - Nach Write: kurz Pulse (gelb → grün) mit Status-Text `<m>/<n> ok`, bei Fehler: rot mit Fehlermeldung
  - Rot dauerhaft bei Verbindungsverlust

- **Properties-Panel** (Static Mode):

```
┌──────────────────────────────────────────────┐
│  OPC UA Write                                 │
├──────────────────────────────────────────────┤
│                                               │
│  Server   [ SPS Linie 3              ▼ ] [+]  │
│  Mode     ( • ) Static  ( ) Dynamic           │
│                                               │
│  Writes                                       │
│  ┌─────────────────────────────────────────┐  │
│  │ ≡ NodeID  [ ns=2;s=Setpoint        ] ✕ │  │
│  │   DataType [ Double            ▼ ]      │  │
│  │   Source   [ msg.payload          ]     │  │
│  ├─────────────────────────────────────────┤  │
│  │ ≡ NodeID  [ ns=2;s=Mode            ] ✕ │  │
│  │   DataType [ Int32             ▼ ]      │  │
│  │   Source   [ static               ]     │  │
│  │   Value    [ 3                    ]     │  │
│  └─────────────────────────────────────────┘  │
│  [+ Add Write]                                │
│                                               │
│  ☑ Pass message through with writeResult      │
│  ☑ Cache DataType per NodeID                  │
│                                               │
└──────────────────────────────────────────────┘
```

## Gemeinsame Konzepte

### Dynamische Steuerung — Leitprinzip

Jeder Operations-Node bietet Static **und** Dynamic Modes. Die Steuer-Messages folgen einer einheitlichen Konvention:

| Node | `msg.action` | `msg.payload` / weitere Felder |
|---|---|---|
| `opcua-read` | `"read"` (optional im triggered) | `msg.nodeIds` (string oder string[]) |
| `opcua-subscribe` | `"subscribe"` / `"unsubscribe"` / `"clear"` | `msg.payload` als String, String-Array oder Object-Array |
| `opcua-write` | implizit (jede Message ist ein Write-Trigger) | `msg.writes` (Array) oder `msg.nodeId` + `msg.payload` (Single) |

Diese Einheitlichkeit macht es einfach, OPC-UA-Operationen aus Function- oder Switch-Nodes heraus zu steuern, z.B. NodeID-Listen aus einer Datenbank zu lesen oder Schreibwerte aus einem Rezept-Loader zu beziehen.

### Server-Connection-Sharing

Mehrere Operations-Nodes mit demselben Server-Referenzen teilen sich **eine** OPC-UA-Session. Der `OpcuaServer` Config Node verwaltet:

1. Eine OPC-UA-Session (CreateSession / ActivateSession)
2. Eine optionale Subscription-Pool-Strategie:
   - **Variante A (v1)**: Jeder `opcua-subscribe` Node legt seine eigene Subscription an. Vorteil: voneinander unabhängige PublishingIntervals, isoliertes Re-/Deploy. Nachteil: mehr Subscriptions = mehr Server-Load
   - **Variante B (later)**: Subscription-Sharing zwischen Nodes mit gleichem PublishingInterval. Optimierung, später
3. Read/Write-Service-Calls werden auf die Session multiplexed (gopcua/opcua-Client ist thread-safe)

```
[opcua-read    nodes=A]   ──┐
[opcua-subscribe items=B] ──┤── Server "SPS Linie 3" ── 1 OPC-UA Session
[opcua-write   nodes=C]   ──┘
```

### NodeID-Validation & Browser

NodeIDs werden frontend-seitig **syntaktisch** validiert (Regex). Existenz auf dem Server wird **nicht** vorab geprüft — das passiert beim ersten Read/Write/Subscribe und schlägt sich im StatusCode nieder. Das macht das System robust gegen NodeIDs, die erst zur Laufzeit existieren (z.B. dynamische Items in Aggregating Servern).

Zusätzlich zur manuellen Eingabe bietet jeder Operations-Node (Read, Subscribe, Write) einen **Address-Space-Browser** — siehe Abschnitt „Address-Space-Browser" weiter unten. NodeIDs lassen sich dort interaktiv durch den Server-Adressraum navigieren und auswählen.

### Status-Codes

OPC UA hat ein eigenes 32-Bit-StatusCode-Schema (`Good = 0`, `BadXxx`, `UncertainXxx`). Wir geben StatusCodes immer in zwei Formen aus:

- `statusCode` (string) — sprechende Form: `"Good"`, `"BadNodeIdUnknown"`, `"UncertainSubNormal"`
- `statusCodeRaw` (number) — raw 32-bit-Wert für Programmverarbeitung

So kann ein nachgeschalteter Switch-Node sowohl `msg.statusCode == "Good"` als auch `msg.statusCodeRaw & 0xC0000000` matchen.

### Catch-Integration

Service-weite Fehler (Verbindung weg, Session ungültig, Timeout) werfen einen Catch-baren Fehler über die bestehende `Error`-Mechanik. Per-Item-Bad-StatusCodes sind **keine** Catch-Fehler — sie sind reguläre Output-Daten, die der nachgeschaltete Flow auswerten kann.

### Credentials & Sicherheit

Username, Passwort und Pfade zu Cert/Key werden vorerst **im Klartext** in der Config gespeichert — analog zu MQTT v1. Verschlüsselung kommt mit dem späteren Credential-System.

Der OPC-UA-Standard verlangt für `securityMode != None` ein Client-Zertifikat. v1: Anwender muss Cert/Key extern erzeugen und Pfad eintragen. Spätere Iteration: Auto-Generierung self-signed im Data-Dir.

## Datenstruktur

### workspace.json

```json
{
  "flows": [
    {
      "id": "flow-1",
      "type": "tab",
      "label": "Maschinenanbindung",
      "nodes": [
        {
          "id": "node-opcua-read-1",
          "type": "opcua-read",
          "name": "Temperatur lesen",
          "x": 200, "y": 150, "z": "flow-1",
          "inputs": 0, "outputs": 1,
          "wires": [["node-debug-1"]],
          "config": {
            "server": "server-1",
            "mode": "static",
            "nodeIds": ["ns=2;s=Temp", "ns=2;i=42"],
            "attribute": "Value",
            "outputShape": "array",
            "includeMetadata": true,
            "interval": 1000,
            "startupRead": true
          }
        },
        {
          "id": "node-opcua-sub-1",
          "type": "opcua-subscribe",
          "name": "Maschinen-Status",
          "x": 200, "y": 250, "z": "flow-1",
          "inputs": 0, "outputs": 1,
          "wires": [["node-debug-1"]],
          "config": {
            "server": "server-1",
            "mode": "static",
            "monitoredItems": [
              {
                "nodeId": "ns=2;s=Pressure",
                "samplingInterval": 500,
                "queueSize": 10,
                "discardOldest": true,
                "deadband": { "type": "absolute", "value": 0.5 }
              }
            ],
            "publishingInterval": 500,
            "lifetimeCount": 60,
            "keepAliveCount": 10,
            "outputShape": "per-item"
          }
        },
        {
          "id": "node-opcua-write-1",
          "type": "opcua-write",
          "name": "Setpoint schreiben",
          "x": 600, "y": 300, "z": "flow-1",
          "inputs": 1, "outputs": 1,
          "wires": [["node-debug-1"]],
          "config": {
            "server": "server-1",
            "mode": "static",
            "writes": [
              {
                "nodeId": "ns=2;s=Setpoint",
                "dataType": "Double",
                "valueSource": "msg",
                "valuePath": "payload"
              }
            ],
            "passthrough": true,
            "disableTypeCache": false
          }
        }
      ]
    }
  ],
  "configs": [
    {
      "id": "server-1",
      "type": "opcua-server",
      "name": "SPS Linie 3",
      "config": {
        "endpointUrl": "opc.tcp://192.168.1.50:4840",
        "securityPolicy": "None",
        "securityMode": "None",
        "authMode": "anonymous",
        "applicationUri": "urn:loopze:client",
        "applicationName": "LOOPZE OPC UA Client",
        "sessionTimeout": 60000,
        "requestTimeout": 5000,
        "keepaliveInterval": 10000,
        "serverCertTrust": "pinned"
      }
    }
  ]
}
```

## Betroffene Dateien

### Backend – Neue Dateien

- `internal/nodes/opcua_server.go` — Config Node: Session-Lifecycle, Reconnect, Subscription-Pool, Type-Cache. Kapselt den `gopcua/opcua` Client
- `internal/nodes/opcua_server_test.go` — End-to-End-Tests gegen einen externen OPC-UA-Test-Server (Node.js / `node-opcua`, gepflegt unter `demo/opcua-server/`). Tests skippen, wenn `LOOPZE_OPCUA_TEST_ENDPOINT` nicht gesetzt ist
- `internal/nodes/opcua_read.go` — Read Node mit Static/Triggered/Dynamic Mode
- `internal/nodes/opcua_read_test.go`
- `internal/nodes/opcua_subscribe.go` — Subscribe Node mit MonitoredItem-Verwaltung
- `internal/nodes/opcua_subscribe_test.go`
- `internal/nodes/opcua_write.go` — Write Node mit Type-Coercion und -Caching
- `internal/nodes/opcua_write_test.go`
- `internal/nodes/opcua_types.go` — gemeinsame Helfer: NodeID-Parsing, DataType-Mapping, StatusCode-Names, Variant-Building
- `internal/nodes/opcua_extobj.go` — ExtensionObject ↔ JSON-Konvertierung (Encode + Decode anhand Schema-Definition)
- `internal/nodes/opcua_extobj_test.go`
- `internal/nodes/opcua_typeresolver.go` — Type-Resolver: lädt `DataTypeDefinition`-Attribute (oder Browse-Fallback) und cached die Schema-Repräsentation pro Server. Wird von Read/Subscribe/Write geteilt
- `internal/nodes/opcua_typeresolver_test.go`
- `internal/server/opcua_handlers.go` — REST-Handler für die Browse-/Read-Attributes-/Resolve-Path- und Test-Connection-Endpoints. Greift auf einen optional vom Engine geliehenen Session-Pool oder eine temporäre Session zurück
- `internal/server/opcua_handlers_test.go`

### Backend – Anpassungen

- `internal/server/server.go` — `registerNodes()` erweitern um `opcua-server`, `opcua-read`, `opcua-subscribe`, `opcua-write`
- `internal/flow/engine.go` — keine Änderung nötig, falls das Config-Node-Lifecycle aus dem MQTT-Issue bereits sauber generisch ist. Sonst kleinere Anpassungen am Provider-Pattern

### Frontend – Neue Dateien

- `frontend/src/components/config/OpcuaServerConfig.vue` — Server-Dialog mit allen Verbindungs-, Security- und Auth-Feldern. „Test Connection"-Button mit Backend-Endpoint `POST /api/v1/opcua/test-connection`
- `frontend/src/components/config/OpcuaReadConfig.vue` — Read-Properties: Server-Dropdown, Mode-Switch, NodeID-Liste, Output-Shape, Static-Options
- `frontend/src/components/config/OpcuaSubscribeConfig.vue` — Subscribe-Properties: MonitoredItem-Editor (Drag-&-Drop, ähnlich wie ChangeConfig), Subscription-Defaults
- `frontend/src/components/config/OpcuaWriteConfig.vue` — Write-Properties: Writes-Editor mit DataType- und Source-Auswahl pro Zeile
- `frontend/src/components/config/shared/NodeIdInput.vue` — wiederverwendbare NodeID-Eingabe mit Syntax-Validation (`ns=N;[isgb]=…`) und „Browse"-Button neben dem Eingabefeld
- `frontend/src/components/config/shared/DataTypeSelect.vue` — Dropdown der OPC-UA-Datentypen
- `frontend/src/components/config/shared/OpcuaBrowser.vue` — Address-Space-Browser-Modal: Tree mit Lazy-Loading, Filter (NodeClass, Text-Suche), Detail-Panel mit Attribute-Ansicht, Multi-/Single-Select je nach Aufrufer
- `frontend/src/composables/useOpcuaBrowser.ts` — Tree-State: Lazy-Loading-Cache pro NodeID, Expand-State, Selection-State, Pagination via ContinuationPoint

### Frontend – Anpassungen

- `frontend/src/components/PropertyPanel.vue` — Dispatch für die vier neuen Node-Typen
- `frontend/src/components/nodes/tokens.ts` — Tokens für `opcua-read` (input, blau), `opcua-subscribe` (input, blau-grün), `opcua-write` (output, blau-orange)
- `frontend/src/types/flow.ts` — TypeScript-Typen für die vier neuen Configs

### Backend – Neue API-Endpoints

- `POST /api/v1/opcua/test-connection` — Body: vollständige Server-Config; Response: `{ ok: true, serverInfo?: {…} }` oder `{ ok: false, error: "<reason>" }`. Server-info enthält ApplicationName, Server-Cert-Fingerprint (für Pinning), Endpoints
- `POST /api/v1/opcua/browse` — Browse einer NodeID, liefert Children mit Metadaten. Nutzt aktive Session des Server-Configs, fällt auf temporäre Session zurück
- `POST /api/v1/opcua/read-attributes` — alle Attribute einer NodeID für das Detail-Panel des Browsers
- `POST /api/v1/opcua/resolve-path` — BrowsePath-Lookup: Liste von BrowseNames → NodeID. Hilfreich für Workflows, in denen NodeIDs aus einem semantischen Pfad reproduzierbar sein sollen

### Go Dependencies

- `github.com/gopcua/opcua` — OPC UA Client Library für Go. Aktuell die einzige produktionsreife Pure-Go-Implementierung. Unterstützt Read, Write, Subscribe, Browse, alle gängigen Security-Policies und v1.04

**Test-Server**: ein dedizierter Deno-basierter OPC-UA-Test-Server wird parallel gepflegt (separates Projekt). Tests in diesem Issue laufen gegen diesen Server via `LOOPZE_OPCUA_TEST_ENDPOINT`-Env-Variable.

## Technische Hinweise

### Library-Wahl: gopcua/opcua

`github.com/gopcua/opcua` ist Pure Go, hat keine CGO-Abhängigkeiten und wird aktiv gepflegt (letzter Release ~6 Monate, regelmäßige Commits). Es hat keine Konkurrenz im Pure-Go-Ökosystem; die einzige Alternative wäre ein Wrapper um die C-basierte open62541-Library, was aber unsere Single-Binary-Strategie bricht. Daher klare Wahl.

### Read-Service-Mapping

```go
func (n *OpcuaReadNode) doRead(ctx context.Context, nodeIds []string, attr ua.AttributeID) ([]ReadResult, error) {
    req := &ua.ReadRequest{
        TimestampsToReturn: ua.TimestampsToReturnBoth,
        NodesToRead:        []*ua.ReadValueID{},
    }
    for _, nid := range nodeIds {
        parsed, err := ua.ParseNodeID(nid)
        if err != nil {
            // Sammeln als Per-Item-Fehler, NICHT als Service-Fehler
            continue
        }
        req.NodesToRead = append(req.NodesToRead, &ua.ReadValueID{
            NodeID:      parsed,
            AttributeID: attr,
        })
    }
    resp, err := n.session.Read(ctx, req)
    // ...
}
```

### Subscription-Recovery nach Reconnect

`gopcua/opcua` bietet `subscription.Recreate()`. Beim Reconnect ruft der Server-Manager dies für alle aktiven Subscriptions auf — die Library legt Subscription und MonitoredItems server-seitig neu an, ohne dass die Operations-Nodes etwas davon mitbekommen. Wichtig: bei `cleanStart`-Verhalten wird die alte Subscription explizit verworfen, sonst können wir Geister-Subscriptions auf dem Server hinterlassen.

### Type-Coercion-Tabelle (Write)

| Eingehender JSON-Typ | Ziel `dataType` | Konversion |
|---|---|---|
| `number` | `Int*`, `UInt*`, `Float`, `Double` | Range-Check; bei Overflow `BadOutOfRange` |
| `boolean` | `Boolean` | direkt |
| `string` `"true"`/`"false"` | `Boolean` | direkt |
| `string` ISO8601 | `DateTime` | `time.Parse(time.RFC3339, …)` |
| `string` | `String` | direkt |
| `string` Hex/Base64 | `ByteString` | konfigurierbar (separater Type-Hint `ByteStringHex` / `ByteStringBase64`) |
| `[]number` | `ByteString` | jedes Element als Byte (analog mqtt-out) |
| `object` | `ExtensionObject` | rekursive Encoding gegen DataTypeDefinition (siehe nächster Abschnitt) |
| `*` | `Variant` | OPC-UA-Server entscheidet anhand des aktuellen Werttyps |

### ExtensionObject-Handling (zentrales Feature)

ExtensionObjects sind das OPC-UA-Vehikel für komplexe Strukturen — UDTs in SPSen, AAS-Submodels, Gerätekonfigurationen, EUInformation, Range, AnalogItem-Properties. In der Praxis ist **kein** industrieller OPC-UA-Server ohne sie nutzbar. LOOPZE muss daher Strukturen transparent zwischen OPC UA und JSON umwandeln können.

**Read-Pfad: ExtensionObject → JSON**

1. Der Server liefert ein `*ua.ExtensionObject` mit:
   - `TypeID` — NodeID des Encoding-Knotens (i.d.R. `<DataTypeNodeId>+Encoding+DefaultBinary`)
   - `Encoding` — `Binary` (Standard) oder `XML`
   - `Value` — Bytes
2. Der `OpcuaServer` Config Node hat einen **Type-Resolver**, der zur jeweiligen `TypeID` die DataTypeDefinition vom Server zieht (`Read` auf `DataTypeDefinition`-Attribut der zugehörigen DataType-Node) und daraus eine in-Memory-Schema-Repräsentation aufbaut. Resultate werden gecached, bis Reconnect.
3. Anhand des Schemas wird der Bytestrom dekodiert und Feld-für-Feld in eine `map[string]any` übertragen:
   - Skalare Felder → JSON-Primitive (Number, Bool, String)
   - Verschachtelte Strukturen → rekursive `map[string]any`
   - Arrays → `[]any`
   - Enums → String mit dem Symbolnamen (Mapping über `EnumDefinition`); zusätzlich `<feld>__raw` mit dem numerischen Wert, falls der Konsument den Enum-Wert programmatisch braucht
   - Optional Fields (Switches in OptionSets) → fehlen einfach im JSON-Object, wenn nicht gesetzt
   - Union-Types → Ein-Feld-Objekt: `{ "<aktiverFeldName>": <wert> }`
4. Die Output-Message bekommt zusätzlich `structureType` (NodeID des Strukturtyps) und `structureName` (BrowseName) als Metadaten.

**Write-Pfad: JSON → ExtensionObject**

1. Anwender liefert ein JSON-Objekt als Wert + `structureType` (NodeID) als Hint
2. Der Type-Resolver lädt die DataTypeDefinition des Strukturtyps (oder nutzt Cache)
3. Felder im JSON werden gegen die Definition gemappt:
   - **Reihenfolge im OPC-UA-Encoding wird durch die Definition vorgegeben**, nicht durch die JSON-Reihenfolge — Anwender muss sich darum nicht kümmern
   - **Fehlende Felder** werden mit Typ-Defaults gefüllt (0 / `false` / `""` / leere Sub-Struktur). Es wird einmal pro Write-Pfad-Lifecycle eine Warning geloggt
   - **Überzählige Felder** im JSON werden ignoriert + geloggt
   - **Type-Mismatches** im Sub-Feld (z.B. String wo Number erwartet) → `BadTypeMismatch` mit Pfad-Angabe (`field "Speed" expected Double, got string`)
   - **Enums**: Anwender darf String (Symbolname) **oder** Number (Raw-Value) liefern. Beides wird akzeptiert
4. Das encodierte Byte-Array wird als ExtensionObject mit korrekter `TypeID` an den Write-Service übergeben

**Discovery der DataTypeDefinition**

Der OPC UA Standard ab 1.04 verlangt vom Server, die Struktur via `DataTypeDefinition`-Attribut maschinenlesbar bereitzustellen (`StructureDefinition` oder `EnumDefinition`). Die `gopcua/opcua` Library implementiert das Read aus diesem Attribut; LOOPZE nutzt es im Type-Resolver. Server, die kein DataTypeDefinition liefern (alte 1.03-Server, ggf. einfache OSS-Implementierungen), fallen auf einen Browse-basierten Discovery-Pfad zurück:

- Browse `HasComponent`-Children der DataType-Node
- Lese `BrowseName` und `DataType` jedes Sub-Felds
- Baue Definition rekursiv auf

Der Browse-Fallback ist eine Notlösung für Felder; Anwender bekommt einen Hinweis im Status-Text wenn er aktiv ist (`fallback type discovery`), weil bestimmte Edge-Cases (Optional-Fields, Unions, Enum-Symbolnamen) damit nicht abdeckbar sind.

**Edge-Cases & Grenzen**

- **`AbstractDataType`** als Feld-Typ (z.B. `BaseDataType`): kommt als rohes `Variant` durch und ist beim Write nur per `Variant`-Override schreibbar (Anwender muss den konkreten Typ kennen)
- **Custom XML-Encodings**: in v1 nur Binary; XML-Encoding wird mit StatusCode `BadEncodingError` abgelehnt. XML ist in industriellen Servern selten, kann später ergänzt werden
- **Rekursive Strukturen** (`StructA` enthält `StructA`): unterstützt, der Type-Resolver verhindert Endlosschleifen via Visited-Set
- **Sehr große Strukturen** (>1 MB): funktionieren, aber Default-Limits `MaxMessageSize` ggf. in Server-Config hochsetzen
- **Strukturen mit binärem Custom-Encoding** ohne DataTypeDefinition (proprietär, ältere Server): nicht decodierbar — kommen als `value: { "raw": "<base64-bytes>" }` zusammen mit einer Warnung durch, sodass der Anwender sie zumindest weiterleiten kann

**Type-Cache-Lifecycle**

Der Type-Cache ist Teil des `OpcuaServer` Config Nodes (also pro Server-Verbindung):

- Eintrag wird beim ersten Read/Write einer NodeID dieses Strukturtyps gefüllt
- Eintrag wird **vollständig** verworfen bei Reconnect (Server könnte Type-Versionen geändert haben)
- Eintrag wird auch verworfen, wenn ein Write mit `BadTypeMismatch` und Hinweis auf Schema-Änderung fehlschlägt — der nächste Write zieht eine frische Definition
- Pro `OpcuaServer` ein zentraler Cache → wird zwischen Read/Subscribe/Write-Nodes geteilt

### Address-Space-Browser (zentrales UX-Feature)

NodeIDs in der Praxis sind oft kryptisch (`ns=4;s=|var|CODESYS Control Win V3.Application.GVL.bMotor1Run`) und niemand tippt sie freiwillig aus dem Kopf ab. Der Address-Space-Browser ist daher kein Nice-to-have, sondern der Standard-Weg, im UI Variablen für `opcua-read`, `opcua-subscribe` und `opcua-write` auszuwählen.

**Aufruf**

In jedem Operations-Properties-Panel gibt es neben dem NodeID-Listen-Editor einen Button **„Browse server…"**. Voraussetzung: ein Server ist im Server-Dropdown ausgewählt. Klick öffnet ein Modal mit Tree-Ansicht des Server-Adressraums.

**UI-Aufbau**

```
┌──────────────────────────────────────────────────────────┐
│  Browse · SPS Linie 3                                [✕]  │
├──────────────────────────────────────────────────────────┤
│  [ 🔍 Search browseName…           ]  Filter: [Vars ▼]   │
├────────────────────────────┬─────────────────────────────┤
│  ▾ Objects                 │  Selected: ns=2;s=Temp       │
│    ▾ Server                │                              │
│    ▾ Devices               │  BrowseName    Temperature   │
│      ▾ Linie3 (Folder)     │  DisplayName   Temperatur     │
│        ▸ Motor1            │  NodeClass     Variable       │
│        ▾ Sensors           │  DataType      Double         │
│          ☑ Temperature     │  AccessLevel   ReadWrite      │
│          ☑ Pressure        │  Description   Vorlauf-Temp   │
│          ☐ Humidity        │                              │
│          ☐ MotorState      │  Path                        │
│            (Struct)        │  /Objects/Devices/Linie3/    │
│                            │  Sensors/Temperature         │
│                            │                              │
│  Loaded: 47 nodes          │                              │
├────────────────────────────┴─────────────────────────────┤
│  Selection: 2 variables                                   │
│  [ Add 2 to list ]   [ Add & close ]   [ Cancel ]         │
└──────────────────────────────────────────────────────────┘
```

**Verhalten**

- **Lazy-Loading**: Children werden erst beim Aufklappen eines Knotens vom Server geholt (`Browse`-Service, ein Backend-Roundtrip pro Aufklappen). Dadurch bleibt der Browser auch gegen Server mit Millionen Nodes performant
- **Default-Root**: `Objects`-Folder (NodeID `ns=0;i=85`) — dort liegen 99 % aller anwender-relevanten Variablen. Über ein Schalter „Show full address space" lässt sich auf den `Root`-Folder umstellen, um auch `Types`/`Views` zu sehen
- **Filter**:
  - **NodeClass**: `All` | `Variables` (Default für Operations-Nodes) | `Variables + Folders` | `Methods` | `Objects`. Variablen-Auswahl filtert auf NodeClass=Variable, lässt aber `Object`/`Folder` als Container im Tree sichtbar damit man durchnavigieren kann
  - **Suche**: tippt der Anwender, wird im aktuell geladenen Subtree client-seitig gefiltert. Ein Server-weiter Suchindex existiert in OPC UA nicht — das ist eine Komfort-Suche über das, was schon geladen ist, nicht eine Volltextsuche. Hinweistext im Suchfeld macht das transparent
- **Multi-Select** (Read und Subscribe): Checkboxes vor jedem Variable-Knoten, beliebig viele auf einmal auswählbar; werden als Batch in die NodeID-Liste übernommen
- **Single-Select** (Write): Radio-Auswahl, eine Variable
- **Detail-Panel** rechts: zeigt alle relevanten Attributes der aktuell fokussierten Node:
  - `BrowseName`, `DisplayName`, `NodeClass`
  - Bei Variablen: `DataType` (mit Klartextname, z.B. `Double` oder `MotorStatusType`), `ValueRank` (Skalar/Array/Matrix), `AccessLevel` (R/W/RW/Hist), `Description`
  - Bei Strukturen: kleiner Schema-Preview (Felder + Typen) aus der DataTypeDefinition
  - Voller Pfad ab `Objects` (über `BrowseName`-Kette gerendert)
- **Sortierung**: Children alphabetisch nach `DisplayName`, Folders/Objects vor Variables (sonst gehen Variablen in tiefen Strukturen unter)
- **Read-bereits-vorhandener-NodeIDs**: bereits in der Liste enthaltene NodeIDs werden im Tree visuell markiert (Häkchen, dimm-fade), damit man Doppel-Auswahlen vermeidet

**Auswahl-Übergabe**

Beim „Add to list" wird pro ausgewählter Variable ein Eintrag in der NodeID-Liste des Operations-Nodes angelegt. Mitgegebene Metadaten:

- `nodeId` — das Pflicht-Feld
- `displayName` — als initialer **Anzeigename** in der Liste, kann frei umbenannt werden (rein UI, nicht persistent ans Backend gesendet außer als optionales Label)
- `dataType` — nur beim **Write-Node** wird der DataType automatisch in das `dataType`-Feld der Write-Spezifikation eingetragen, damit der Anwender sich diese Information nicht selbst aus dem Server-Manual zusammensuchen muss. Beim Subscribe wird `dataType` als reine UI-Info gehalten, weil die Subscription den Typ nicht braucht
- `structureType` — bei Variablen mit Struktur-DataType wird auch die Struktur-NodeID übernommen, damit ExtensionObject-Writes ohne weitere Klicks funktionieren

**Backend-API**

Drei REST-Endpoints, alle mit dem Server-Config als Body (oder mit Server-ID + Lookup auf eine bereits aktive Session):

```
POST /api/v1/opcua/browse
  Body: { serverConfig | serverId, nodeId, nodeClassFilter? }
  Response: {
    parent: { nodeId, browseName, displayName, nodeClass, hasChildren },
    children: [
      {
        nodeId: "ns=2;s=Temp",
        browseName: "Temperature",
        displayName: "Temperatur",
        nodeClass: "Variable",
        dataType: { nodeId: "i=11", name: "Double", isStructure: false },
        valueRank: -1,
        accessLevel: "ReadWrite",
        hasChildren: false,
        description: "Vorlauf-Temp"
      },
      …
    ]
  }

POST /api/v1/opcua/read-attributes
  Body: { serverConfig | serverId, nodeId }
  Response: { all attributes für das Detail-Panel }

POST /api/v1/opcua/resolve-path
  Body: { serverConfig | serverId, browsePath: ["Objects","Devices","…"] }
  Response: { nodeId, displayName, nodeClass, dataType, … }
```

Server-seitig nutzt der Endpoint die **bestehende Session** des Server-Config-Nodes, falls dieser bereits deployed ist (gemeinsamer Pool im Engine-Lifecycle). Falls nicht (Browser wird vor dem ersten Deploy benutzt), öffnet das Backend eine **temporäre Session** ausschließlich für den Browse-Vorgang und schließt sie nach Inaktivität (60 s Timeout). So ist Browsing immer verfügbar — auch im frisch importierten Workspace, der noch nie deployed wurde.

**Sicherheit**

Die Browse-Endpoints sind authenticated wie alle anderen API-Endpoints (Sessions/Auth aus dem Auth-V1-System). Wer keinen Zugriff auf den Editor hat, kann auch nicht browsen.

**Frontend-Komponente**

- `frontend/src/components/config/shared/OpcuaBrowser.vue` — Modal mit Tree, Filter, Detail-Panel, Auswahl-Logik. Wird von `OpcuaReadConfig.vue`, `OpcuaSubscribeConfig.vue`, `OpcuaWriteConfig.vue` instanziiert. Multi-/Single-Select wird via Prop konfiguriert
- `frontend/src/composables/useOpcuaBrowser.ts` — Composable für den Tree-State (Lazy-Loading-Cache, Expand/Collapse-State, Selection)

**Edge-Cases**

- **Verbindung weg während Browse**: Modal zeigt Banner „connection lost — retry", Tree bleibt mit dem zuletzt geladenen Stand sichtbar (kein Datenverlust)
- **Sehr große Folder** (>1000 Children): Backend liefert paginiert via `ContinuationPoint`; Frontend lädt automatisch nach beim Scroll-Ende, mit „Loaded N of ?" Indikator
- **Reference-Loops** (selten, aber möglich in custom Adressräumen): Frontend hält ein Visited-Set pro Pfad und bricht Re-Expand ab — keine Endlos-Trees
- **Variablen mit `null` DataType** (kaputter Server): kommen mit Hinweis-Icon im Tree, sind auswählbar, der Write-Node bekommt aber `dataType=Variant` als Default mit einem TODO-Marker

### Status-Codes als Strings

Die Library liefert `ua.StatusCode` als uint32. Ein Mapping `statusCodeName(code) string` deckt die ~200 Standard-Codes ab; unbekannte Codes werden als `Bad_0x<hex>` formatiert.

### "Test Connection" Endpoint

Der Endpoint öffnet eine Session mit den übergebenen Parametern, ruft `Read(ServerStatus)` und schließt sofort wieder. Liefert:

- bei Erfolg: ApplicationName, Build-Info, Server-Cert-Fingerprint (SHA-256), verfügbare Endpoints
- bei Misserfolg: konkrete Fehlermeldung (Library-Error oder OPC-UA-StatusCode)

Dieser Endpoint ist **read-only** und ohne Persistenz — er berührt die `configs[]` in `workspace.json` nicht.

## Tests

### Backend

| Test | Prüft |
|---|---|
| `TestOpcuaServerConnect` | Session-Aufbau gegen In-Process-Test-Server |
| `TestOpcuaServerReconnect` | Reconnect-Verhalten bei TCP-Drop |
| `TestOpcuaReadStatic` | Static-Mode mit Interval, mehrere NodeIDs |
| `TestOpcuaReadDynamic` | NodeIDs aus `msg.nodeIds`, Override Config |
| `TestOpcuaReadOutputShapes` | `single`, `array`, `object` Shapes |
| `TestOpcuaReadBadStatus` | Eine NodeID liefert `BadNodeIdUnknown`, andere `Good` — beide kommen im Output an |
| `TestOpcuaSubscribeStatic` | MonitoredItems werden angelegt, Wertänderungen kommen am Output an |
| `TestOpcuaSubscribeDynamic` | `subscribe`/`unsubscribe`/`clear` Aktionen |
| `TestOpcuaSubscribeDeadband` | Absolute und Percent Deadband filtern Updates korrekt |
| `TestOpcuaSubscribeRecovery` | Nach Reconnect kommen Items wieder an |
| `TestOpcuaWriteStatic` | Werte aus `msg.payload` werden geschrieben, Result-Output stimmt |
| `TestOpcuaWriteDynamic` | `msg.writes` Array verarbeiten |
| `TestOpcuaWriteTypeCoercion` | Number → Int32, String → Boolean, etc. |
| `TestOpcuaWriteTypeCache` | Erster Write triggert Type-Lookup, zweiter nutzt Cache |
| `TestOpcuaSharedSession` | Mehrere Nodes mit gleichem Server teilen sich eine Session |
| `TestOpcuaExtensionObjectRead` | Lesen einer Struktur → JSON-Object mit korrekten Feldern, `structureType` und `structureName` im Output |
| `TestOpcuaExtensionObjectNestedRead` | Verschachtelte Strukturen werden rekursiv gemappt |
| `TestOpcuaExtensionObjectArrayRead` | Array von Strukturen → Array von JSON-Objects |
| `TestOpcuaExtensionObjectEnumRead` | Enum-Feld kommt als Symbolname raus, `__raw` enthält numerischen Wert |
| `TestOpcuaExtensionObjectWrite` | JSON-Object → ExtensionObject, Server akzeptiert (StatusCode Good) |
| `TestOpcuaExtensionObjectWriteMissingFields` | Fehlende Felder werden mit Defaults gefüllt, Warning geloggt |
| `TestOpcuaExtensionObjectWriteExtraFields` | Überzählige Felder werden ignoriert |
| `TestOpcuaExtensionObjectWriteTypeMismatch` | String wo Number erwartet → `BadTypeMismatch` mit Feld-Pfad |
| `TestOpcuaExtensionObjectRoundTrip` | Read → Write derselben Struktur ohne Modifikation: Server-State unverändert |
| `TestOpcuaTypeResolverCache` | DataTypeDefinition wird einmal geladen, mehrfach genutzt |
| `TestOpcuaTypeResolverInvalidationOnReconnect` | Cache wird bei Reconnect verworfen |
| `TestOpcuaBrowseFallbackDiscovery` | Server ohne `DataTypeDefinition`-Attribut: Browse-basiertes Discovery liefert nutzbare Definition |
| `TestOpcuaBrowseHandler` | `/api/v1/opcua/browse` liefert Children mit korrekten Metadaten |
| `TestOpcuaBrowseHandlerPagination` | ContinuationPoint-Pagination bei großen Foldern |
| `TestOpcuaBrowseHandlerTempSession` | Browse vor Deploy nutzt temporäre Session, schließt nach Inaktivität |
| `TestOpcuaBrowseHandlerSharedSession` | Browse während Deploy nutzt aktive Engine-Session |
| `TestOpcuaReadAttributesHandler` | `/api/v1/opcua/read-attributes` liefert vollständigen Attribute-Satz |
| `TestOpcuaResolvePathHandler` | BrowsePath wird korrekt zur NodeID aufgelöst |

### Frontend

- Render-Tests für die vier Config-Komponenten
- Validation-Test für `NodeIdInput` (akzeptiert `ns=2;s=Foo`, lehnt `Foo` ab)

## Abhängigkeiten

- **Config-Node-Konzept**: bereits eingeführt mit [NODE_MQTT.md](NODE_MQTT.md). Die Engine-Lifecycle-Erweiterungen werden hier wiederverwendet
- **`ValueTypeInput.vue`**: kann für Static-Werte im Write-Node mit verwendet werden, falls sinnvoll
- **Catch-Node**: für Service-weite Fehler (Verbindungsabbruch) — ist bereits implementiert

## Abgrenzung / Nicht im Scope

**Bewusst nicht in v1**:

- **Browse-as-Node**: Ein `opcua-browse` Node, der zur Laufzeit Children eines NodeIDs auflistet. Sinnvoll für Discovery-Workflows, aber eigenes Issue
- **Method Calls**: `opcua-call` Node für `Call`-Service. Braucht eigenes Argument-Mapping und Output-Argument-Handling, eigenes Issue
- **HistoryRead**: Lesen historischer Werte über `HistoryRead`-Service. Eigenes Issue
- **Events / Alarms & Conditions**: Subscribing auf Event-Notifier-Nodes. Eigenes Issue, weil das Event-Type-Filtering eine eigene UI braucht
- **OPC UA Server Mode**: LOOPZE als OPC-UA-**Server**, nicht Client. Vollständig anderes Konzept, zukünftiges Issue
- **Auto-Discovery (LDS)**: Discovery-Endpoint absuchen und alle Server auflisten. Praxisnutzen begrenzt, später
- **Cert-Auto-Generation im UI**: Self-signed Cert/Key beim ersten Save erzeugen. Wäre nett, aber zunächst Pfad-Eingabe
- **Multi-Endpoint-Failover**: Eine Server-Config mit mehreren Endpoints und automatischem Failover. Bauernregeln-Industrieanforderung, aber später
- **Subscription-Sharing zwischen Nodes**: Optimierung über gemeinsame Subscription bei gleichem PublishingInterval — kann später ohne Breaking Change nachgezogen werden
- **XML-Encoding für ExtensionObjects**: nur Binary-Encoding wird unterstützt; XML-encodierte ExtensionObjects sind selten und kommen erst später
- **OPC UA Method Output Arguments mit ExtensionObject**: irrelevant für v1, weil Method Calls insgesamt nicht im Scope sind
- **Schreiben von `AbstractDataType`-Feldern** ohne expliziten Variant-Override: Anwender muss den konkreten Typ angeben
