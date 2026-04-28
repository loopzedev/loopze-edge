# Issue: Modbus Nodes – Read & Write mit Server-Konfiguration

## Status: Open

## Problembeschreibung

Nach MQTT folgt der zweitwichtigste Industrie-Connector: **Modbus**. Zwei neue Node-Typen (`modbus-read` und `modbus-write`) ermöglichen das Lesen und Schreiben von Modbus-Geräten. Analog zum MQTT-Pattern führt ein **Modbus-Server Config Node** (`modbus-server`) die Verbindung als wiederverwendbare Entität ein — mehrere Nodes können denselben Server referenzieren und teilen sich eine Verbindung.

**Leitprinzip dieses Issues**: Alle Funktionen — Function Code, Adresse, Anzahl, Datentyp, Unit-ID — können sowohl **statisch in der Node-Config** als auch **dynamisch per Message** gesteuert werden. **Einzig die Server-Vorgabe (Host/Port bzw. Serial-Parameter) ist ausschließlich statisch** und wird einmalig im Config Node gepflegt.

**Scope**: Modbus TCP und Modbus RTU (Serial). ASCII bleibt außen vor. Unterstützt werden die gängigen Function Codes (FC1–FC6, FC15, FC16). Multi-Register-Datentypen (INT32, FLOAT32, …) werden mit konfigurierbarer Byte- und Word-Order ausgelesen, da die Reihenfolge in der Praxis nicht standardisiert ist.

## Übersicht

| Node-Typ | Typ-ID | Canvas Inputs | Canvas Outputs | Beschreibung |
|---|---|---|---|---|
| **Modbus Read** | `modbus-read` | 0 oder 1 | 1 | Liest Coils / Discrete Inputs / Holding Registers / Input Registers |
| **Modbus Write** | `modbus-write` | 1 | 0 oder 1 | Schreibt Coils oder Holding Registers |

```
                        Modbus-Gerät (z.B. SPS, Energiezähler)
                        ┌─────────────────────────┐
Flow A                  │  Holding Reg 40001..n   │
┌────────────────────┐  │  Coil       00001..n    │
│  [Inject every 1s] │  │  Discrete   10001..n    │
│        ↓           │  │  Input Reg  30001..n    │
│  [Modbus Read] ←──────┤                          │
│        ↓           │  └─────────────────────────┘
│  [Debug]           │
└────────────────────┘                  ↑
                                         │
Flow B                                   │
┌────────────────────┐                   │
│  [Inject]          │                   │
│        ↓           │                   │
│  [Modbus Write] ──────────────────────┘
└────────────────────┘
```

## Anforderungen

### 1. Config Node: Modbus Server (`modbus-server`)

Der Modbus Server ist ein **Config Node** (siehe NODE_MQTT.md – das Konzept wird hier wiederverwendet) — er erscheint nicht auf dem Canvas und wird von `modbus-read` / `modbus-write` Nodes referenziert.

- **Typ-ID**: `modbus-server`
- **Kein Canvas-Element** — rein konfigurativ
- **Konfigurationsfelder**:
  - `name` (string) — Anzeigename, z.B. "SPS Halle 1"
  - `transport` (string) — `tcp` (Default) oder `rtu`
  - **TCP-Felder** (gültig wenn `transport=tcp`):
    - `host` (string) — Hostname oder IP
    - `port` (number) — Standard: 502
  - **RTU-Felder** (gültig wenn `transport=rtu`):
    - `serialPort` (string) — z.B. `/dev/ttyUSB0`, `COM3`
    - `baudRate` (number) — 9600, 19200, 38400, 57600, 115200; Standard: 9600
    - `dataBits` (number) — 7 oder 8; Standard: 8
    - `parity` (string) — `none`, `even`, `odd`; Standard: `none`
    - `stopBits` (number) — 1 oder 2; Standard: 1
  - **Gemeinsame Felder**:
    - `timeout` (number) — Request-Timeout in Millisekunden; Standard: 1000
    - `idleTimeout` (number, nur TCP) — Sekunden, nach denen eine ungenutzte TCP-Verbindung geschlossen wird; Standard: 60. `0` = nie schließen
    - `defaultUnitId` (number) — Default Unit/Slave ID, wenn ein Node keinen eigenen Wert setzt; Standard: 1
    - `reconnectBackoff` (number) — Sekunden zwischen Reconnect-Versuchen nach Verbindungsverlust; Standard: 5

- **Zugriff auf den Properties-Dialog**:
  - **Neuer Server**: Über den "+" Button neben dem Server-Dropdown in Modbus Nodes
  - **Bestehenden Server editieren**: Über den "Edit server config" Link unterhalb des Dropdowns

- **Properties-Dialog**:

```
┌──────────────────────────────────────────────┐
│  Modbus Server                                │
├──────────────────────────────────────────────┤
│                                               │
│  Name                                         │
│  ┌────────────────────────────────────────┐   │
│  │ SPS Halle 1                            │   │
│  └────────────────────────────────────────┘   │
│                                               │
│  Transport                                    │
│  ( • ) TCP    ( ) RTU (Serial)                │
│                                               │
│  ── TCP ─────────────────────────────────────│
│  Host                          Port            │
│  ┌──────────────────────────┐ ┌──────────┐   │
│  │ 192.168.1.50             │ │ 502      │   │
│  └──────────────────────────┘ └──────────┘   │
│                                               │
│  ── RTU (greyed out im TCP-Modus) ───────────│
│  Serial Port                                  │
│  ┌────────────────────────────────────────┐   │
│  │ /dev/ttyUSB0                           │   │
│  └────────────────────────────────────────┘   │
│  Baud   [9600 ▼]   Data Bits [8 ▼]            │
│  Parity [none ▼]   Stop Bits [1 ▼]            │
│                                               │
│  ── Gemeinsam ───────────────────────────────│
│  Timeout (ms):       [1000]                   │
│  Idle Timeout (s):   [60] (TCP only)          │
│  Default Unit ID:    [1]                      │
│  Reconnect (s):      [5]                      │
│                                               │
│  ┌────────────┐  ┌────────────┐              │
│  │  Speichern  │  │ Abbrechen  │              │
│  └────────────┘  └────────────┘              │
└──────────────────────────────────────────────┘
```

### 2. Modbus Read Node (`modbus-read`)

- **Canvas**:
  - Static Mode (cyclic poll): 0 Inputs, 1 Output
  - Dynamic Mode (on demand): 1 Input, 1 Output — der Input löst den Read aus
- **Funktion**: Liest den konfigurierten Adressbereich von einem Modbus-Gerät und gibt das Ergebnis als Flow-Message aus. Im Dynamic-Modus können sämtliche Parameter per `msg` überschrieben werden.

- **Basis-Konfiguration**:
  - `server` (string) — ID des referenzierten `modbus-server` Config Nodes
  - `mode` (string) — `static` (Default) oder `dynamic`
  - `unitId` (number) — Modbus Unit/Slave ID. Wenn leer, wird `defaultUnitId` aus dem Server verwendet
  - `fc` (number) — Function Code:
    - `1` — Read Coils (1 bit, RW)
    - `2` — Read Discrete Inputs (1 bit, RO)
    - `3` — Read Holding Registers (16 bit, RW) — **Default**
    - `4` — Read Input Registers (16 bit, RO)
  - `address` (number) — Start-Adresse, **0-basiert** (40001 → 0). Per UI-Toggle kann auf 1-basierte Eingabe umgeschaltet werden, intern wird stets 0-basiert gespeichert
  - `quantity` (number) — Anzahl Coils bzw. Register, die gelesen werden. Standard: 1
  - `dataType` (string) — wie der Roh-Block interpretiert wird:
    - `raw` (Default) — `[]uint16` für FC3/FC4, `[]bool` für FC1/FC2
    - `bool` — einzelnes `bool` (nur FC1/FC2, `quantity` muss 1 sein)
    - `int16` / `uint16` — ein einzelnes Register als signed/unsigned
    - `int32` / `uint32` / `float32` — zwei Register (siehe Byte/Word-Order)
    - `int64` / `uint64` / `float64` — vier Register
    - `string` — `quantity` Register als ASCII/UTF-8 String (2 Zeichen pro Register, Null-Terminator wird abgeschnitten)
  - `byteOrder` (string) — `bigEndian` (Default) oder `littleEndian` — Byte-Reihenfolge **innerhalb** eines Registers
  - `wordOrder` (string) — `bigEndian` (Default, "ABCD") oder `littleEndian` ("CDAB") — Reihenfolge **mehrerer** Register bei 32/64-bit-Typen. In der Praxis gibt es Geräte aller vier Kombinationen
  - `scale` (number, optional) — multiplikativer Skalierungsfaktor; nützlich z.B. wenn ein Energiezähler Watt als Hundertstel (`/100`) liefert
  - `offset` (number, optional) — additiver Offset; wird **nach** der Skalierung addiert

- **Polling (Static Mode)**:
  - `pollInterval` (number) — Intervall in Millisekunden zwischen Reads. Standard: 1000
  - `emitOnChange` (boolean) — wenn `true`, wird nur bei Änderung des Werts emittiert (sinnvoll bei langsam wechselnden Größen). Standard: `false`
  - `emitOnError` (boolean) — wenn `true`, wird bei Read-Fehler eine Fehler-Message ausgegeben (`msg.error` gesetzt, `msg.payload` leer). Wenn `false` (Default), wird nur intern geloggt und der Status auf Rot gesetzt — kein Fehler-Output

- **Dynamic Mode** — alle Felder können per `msg` überschrieben werden (fehlt das Feld → Config-Default):
  - `msg.unitId` (number)
  - `msg.fc` (number 1–4)
  - `msg.address` (number)
  - `msg.quantity` (number)
  - `msg.dataType` (string)
  - `msg.byteOrder` (string)
  - `msg.wordOrder` (string)
  - Eine Eingangs-Message ohne `msg.action` löst einen Read mit den effektiven Parametern aus. Eingehende Messages werden **nicht** am Output durchgereicht — der Output enthält ausschließlich das Read-Ergebnis (oder den Fehler bei `emitOnError`)

- **Ausgehende Message** (bei erfolgreichem Read):
  ```json
  {
    "payload": 23.7,
    "topic": "modbus/sps-halle-1/40001",
    "modbus": {
      "fc": 3,
      "address": 0,
      "quantity": 2,
      "dataType": "float32",
      "unitId": 1,
      "raw": [16968, 13107]
    }
  }
  ```
  - `msg.payload` — der dekodierte Wert (Skalar, Array, String, Bool — abhängig von `dataType`)
  - `msg.topic` — Default `modbus/<server-name>/<address>`; vom Anwender überschreibbar
  - `msg.modbus` — Metadaten-Block mit den effektiven Read-Parametern und den Roh-Registerwerten (für Debugging und Round-Trip-Szenarien)

- **Status-Anzeige**:
  - Grün: `verbunden · <intervall>` (Static) bzw. `verbunden · idle` (Dynamic)
  - Gelb: `verbinde…` / `reconnecting…`
  - Rot: Fehlermeldung mit Modbus Exception Code (z.B. `Illegal Data Address (0x02)`)

- **Properties-Panel**:

```
┌──────────────────────────────────────────────┐
│  Modbus Read                                  │
├──────────────────────────────────────────────┤
│                                               │
│  Server                                       │
│  ┌────────────────────────────────┐ ┌───┐    │
│  │ SPS Halle 1                ▼  │ │ + │    │
│  └────────────────────────────────┘ └───┘    │
│  Edit server config                           │
│                                               │
│  Mode                                         │
│  ( • ) Static (poll)   ( ) Dynamic (on input) │
│                                               │
│  Unit ID:  [1]   (leer = Server-Default)      │
│                                               │
│  Function Code                                │
│  ┌────────────────────────────────────────┐   │
│  │ FC3 — Read Holding Registers       ▼  │   │
│  └────────────────────────────────────────┘   │
│                                               │
│  Address:    [0]    ☐ 1-based input           │
│  Quantity:   [2]                              │
│                                               │
│  Data Type:  [float32                      ▼] │
│  Byte Order: [Big Endian (default)         ▼] │
│  Word Order: [Big Endian (ABCD, default)   ▼] │
│                                               │
│  Scale:  [1]   Offset: [0]                    │
│                                               │
│  ── Polling (Static only) ──────────────────│
│  Interval: [1000] ms                          │
│  ☐ Emit on change                             │
│  ☐ Emit on error                              │
│                                               │
└──────────────────────────────────────────────┘
```

Im Dynamic-Modus werden Polling-Felder ausgeblendet und ein Hinweis-Block gezeigt:

```
ℹ Send any message to trigger a read. Override
   any field via msg.fc / msg.address /
   msg.quantity / msg.dataType / msg.unitId.
```

### 3. Modbus Write Node (`modbus-write`)

- **Canvas**:
  - 1 Input
  - 0 Outputs (Default — Sink)
  - **Optional**: 1 Output, wenn `emitAck=true` — gibt nach erfolgreichem Write eine ACK-Message aus (für nachgelagerte Confirm-Logik)
- **Funktion**: Schreibt eingehende Messages auf das Modbus-Gerät. Sämtliche Parameter (FC, Adresse, Datentyp, Unit-ID) sind sowohl statisch konfigurierbar als auch per `msg` überschreibbar.

- **Basis-Konfiguration**:
  - `server` (string) — ID des referenzierten `modbus-server` Config Nodes
  - `unitId` (number, optional) — Default Unit/Slave ID
  - `fc` (number) — Function Code:
    - `5` — Write Single Coil
    - `6` — Write Single Register
    - `15` — Write Multiple Coils
    - `16` — Write Multiple Holding Registers — **Default**
  - `address` (number) — Start-Adresse, 0-basiert
  - `dataType` (string) — analog zu `modbus-read`. Bestimmt, wie `msg.payload` vor dem Schreiben in Register/Coils kodiert wird
  - `byteOrder` (string), `wordOrder` (string) — analog
  - `scale` (number, optional), `offset` (number, optional) — wird **invers** zum Read-Pfad angewendet: `register = (payload - offset) / scale`
  - `emitAck` (boolean) — Wenn `true`, schaltet der Node einen Output frei und gibt nach erfolgreichem Write eine Bestätigung aus. Standard: `false`

- **Eingehende Message**:
  - `msg.payload` — der zu schreibende Wert. Der Typ muss zum konfigurierten/`msg`-übergebenen `dataType` passen:
    - `bool` für FC5 / `dataType=bool`
    - `number` für `int16/uint16/int32/uint32/int64/uint64/float32/float64`
    - `[]bool` für FC15 (Multiple Coils)
    - `[]number` für FC16 mit `dataType=raw` oder bei mehreren Werten desselben Typs
    - `string` für `dataType=string`
  - **Override-Felder** (analog zu Read):
    - `msg.unitId` (number)
    - `msg.fc` (number 5/6/15/16)
    - `msg.address` (number)
    - `msg.dataType` (string)
    - `msg.byteOrder`, `msg.wordOrder` (string)

- **Ausgehende Message** (nur bei `emitAck=true`):
  ```json
  {
    "payload": true,
    "modbus": {
      "fc": 16,
      "address": 100,
      "quantity": 2,
      "dataType": "float32",
      "unitId": 1,
      "written": [16968, 13107]
    }
  }
  ```
  - `msg.payload` ist `true` bei Erfolg
  - `msg.modbus.written` enthält die tatsächlich auf den Bus geschickten Roh-Register

- **Fehlerverhalten**:
  - Bei Modbus-Exception (z.B. `Illegal Function`, `Illegal Data Address`, `Slave Device Failure`): Status auf Rot, Fehlermeldung enthält Exception Code. Wenn ein **Catch Node** im Flow vorhanden ist, geht der Fehler über den Catch-Pfad (analog zu anderen Nodes — siehe `CATCH_NODE.md`)
  - Bei TCP-Verbindungsverlust: Reconnect läuft, Write wird mit `Server unavailable` quittiert

- **Properties-Panel**:

```
┌──────────────────────────────────────────────┐
│  Modbus Write                                 │
├──────────────────────────────────────────────┤
│                                               │
│  Server                                       │
│  ┌────────────────────────────────┐ ┌───┐    │
│  │ SPS Halle 1                ▼  │ │ + │    │
│  └────────────────────────────────┘ └───┘    │
│  Edit server config                           │
│                                               │
│  Unit ID:  [1]   (leer = Server-Default)      │
│                                               │
│  Function Code                                │
│  ┌────────────────────────────────────────┐   │
│  │ FC16 — Write Multiple Registers    ▼  │   │
│  └────────────────────────────────────────┘   │
│                                               │
│  Address:    [100]                            │
│                                               │
│  Data Type:  [float32                      ▼] │
│  Byte Order: [Big Endian (default)         ▼] │
│  Word Order: [Big Endian (ABCD, default)   ▼] │
│                                               │
│  Scale:  [1]   Offset: [0]                    │
│                                               │
│  ☐ Emit ACK on success                        │
│                                               │
│  ℹ  msg.fc / msg.address / msg.dataType /     │
│     msg.unitId überschreiben die Config       │
│     pro Message. msg.payload trägt den Wert.  │
│                                               │
└──────────────────────────────────────────────┘
```

### 4. Server Connection Sharing

Wenn mehrere Modbus-Nodes denselben Server referenzieren, wird **eine Verbindung** geteilt:

```
[modbus-read  fc=3 addr=0]   ──┐
[modbus-read  fc=3 addr=10]  ──┤── Server "SPS Halle 1" ── 1 TCP / 1 Serial
[modbus-write fc=16 addr=100]──┘
```

Die Engine stellt einen **Server-Manager** bereit, der:
1. Pro `modbus-server` ID eine Verbindung verwaltet (TCP-Socket bzw. geöffneter Serial-Port)
2. Read- und Write-Anfragen der Nodes serialisiert (Modbus ist halbduplex — gleichzeitige Requests auf einer Verbindung sind nicht erlaubt; der Manager queuet Anfragen)
3. Bei Verbindungsverlust automatisch reconnected mit dem konfigurierten Backoff
4. Beim Stop / Re-Deploy alle Verbindungen sauber schließt

**Serialisierung pro Server**: Im Modbus-Protokoll darf auf einer Verbindung immer nur eine Transaktion gleichzeitig laufen. Mehrere parallele Read-Polls auf demselben Server werden vom Manager seriell abgearbeitet — entweder via Mutex oder Request-Queue. Der Server-Manager ist **pro Server-Config**, nicht pro Node — die Serialisierung greift also über alle Nodes hinweg, die diesen Server teilen.

**Polling-Stagger**: Wenn mehrere Read-Nodes mit gleichem `pollInterval` denselben Server polleln, sollten ihre Tick-Zeitpunkte versetzt werden (Round-Robin), damit die Last gleichmäßig verteilt ist und sich keine Spitzen aufbauen. Optimierung — nicht zwingend für v1.

## Datenstruktur

### workspace.json

```json
{
  "flows": [
    {
      "id": "flow-1",
      "type": "tab",
      "label": "SPS Halle 1",
      "nodes": [
        {
          "id": "node-modbus-read-1",
          "type": "modbus-read",
          "name": "Temperatur Kessel",
          "x": 200,
          "y": 150,
          "z": "flow-1",
          "inputs": 0,
          "outputs": 1,
          "wires": [["node-debug-1"]],
          "config": {
            "server": "server-1",
            "mode": "static",
            "unitId": 1,
            "fc": 3,
            "address": 0,
            "quantity": 2,
            "dataType": "float32",
            "byteOrder": "bigEndian",
            "wordOrder": "bigEndian",
            "scale": 1,
            "offset": 0,
            "pollInterval": 1000,
            "emitOnChange": false,
            "emitOnError": false
          }
        },
        {
          "id": "node-modbus-write-1",
          "type": "modbus-write",
          "name": "Sollwert setzen",
          "x": 600,
          "y": 300,
          "z": "flow-1",
          "inputs": 1,
          "outputs": 0,
          "wires": [],
          "config": {
            "server": "server-1",
            "unitId": 1,
            "fc": 16,
            "address": 100,
            "dataType": "float32",
            "byteOrder": "bigEndian",
            "wordOrder": "bigEndian",
            "scale": 1,
            "offset": 0,
            "emitAck": false
          }
        }
      ]
    }
  ],
  "configs": [
    {
      "id": "server-1",
      "type": "modbus-server",
      "name": "SPS Halle 1",
      "config": {
        "transport": "tcp",
        "host": "192.168.1.50",
        "port": 502,
        "timeout": 1000,
        "idleTimeout": 60,
        "defaultUnitId": 1,
        "reconnectBackoff": 5
      }
    }
  ]
}
```

## Betroffene Dateien

### Backend – Neue Dateien

- `internal/nodes/modbus_server.go` — Modbus Server Config Node: kapselt den Modbus-Client (TCP oder RTU), Reconnect-Logik, Request-Serialisierung
- `internal/nodes/modbus_read.go` — Read Node: Polling-Loop (Static) oder Input-Trigger (Dynamic), Dekodierung in den Ziel-Datentyp
- `internal/nodes/modbus_write.go` — Write Node: Kodierung von `msg.payload` in Register/Coils, Modbus-Request über den geteilten Server
- `internal/nodes/modbus_codec.go` — Hilfsfunktionen: Byte/Word-Order-Handling, Skalar-Encoding/-Decoding, String ↔ Register-Konvertierung. Eigenständig testbar (Table-driven Tests)

### Backend – Anpassungen

- `internal/server/server.go` — Registrierung von `modbus-read` und `modbus-write` in `registerNodes()`
- `internal/flow/engine.go` — Erweiterung des Config Node Lifecycle (eingeführt mit MQTT) um `modbus-server`. Der Config-Node-Typ wird automatisch erkannt; wenn die Generalisierung dort sauber sitzt, sind keine Modbus-spezifischen Anpassungen nötig
- `internal/flow/registry.go` — Wenn `ConfigProvider` aus dem MQTT-Issue bereits existiert, wird er hier wiederverwendet
- `internal/storage/` — Keine Änderung nötig (sofern MQTT-Issue den `configs`-Abschnitt bereits eingeführt hat)

### Frontend – Neue Dateien

- `frontend/src/components/config/ModbusNodeConfig.vue` — Gemeinsame Config-Komponente für `modbus-read` und `modbus-write`: Server-Dropdown + "+", Mode-Selector, FC, Address, DataType, Byte/Word Order, Scale/Offset; per Conditional Rendering die Read- bzw. Write-spezifischen Felder
- `frontend/src/components/config/ModbusServerConfig.vue` — Server Config Dialog: Transport-Switch (TCP/RTU), zugehörige Felder, Timeout, Default Unit ID

### Frontend – Anpassungen

- `frontend/src/components/PropertyPanel.vue` — Dispatch für `modbus-read` und `modbus-write` auf `ModbusNodeConfig`
- `frontend/src/components/nodes/tokens.ts` — Bereits vorhanden: `modbus-read` → input (grün), `modbus-write` → output (orange). Keine Änderung nötig
- `frontend/src/stores/flowStore.ts` — Falls bereits durch MQTT-Issue um Config-Node-CRUD erweitert: nichts. Sonst dort generalisieren
- `frontend/src/types/flow.ts` — TypeScript-Typen für `ModbusServerConfig`, `ModbusReadConfig`, `ModbusWriteConfig`

### Go Dependencies

- `github.com/goburrow/modbus` — etablierte Go-Bibliothek mit Support für TCP, RTU und ASCII. Aktiv gepflegt, von vielen Industrieprojekten verwendet
- `go.bug.st/serial` (transitiv via `goburrow/modbus`) — Cross-Platform Serial-Support für RTU

## Technische Hinweise

### Function Codes – Übersicht

| FC | Operation | Adressraum | Datentyp | Lese/Schreib |
|----|-----------|-----------|----------|--------------|
| 1  | Read Coils | 00001..0xxxx | bit | RW (lesen) |
| 2  | Read Discrete Inputs | 10001..1xxxx | bit | RO |
| 3  | Read Holding Registers | 40001..4xxxx | 16-bit | RW (lesen) |
| 4  | Read Input Registers | 30001..3xxxx | 16-bit | RO |
| 5  | Write Single Coil | 00001..0xxxx | bit | WO |
| 6  | Write Single Register | 40001..4xxxx | 16-bit | WO |
| 15 | Write Multiple Coils | 00001..0xxxx | bit | WO |
| 16 | Write Multiple Registers | 40001..4xxxx | 16-bit | WO |

Die **0-basierte Adresse** entspricht der herstellerspezifischen Notation minus 1 (40001 → 0). Per UI-Toggle (`1-based input`) kann der Anwender die in Datenblättern üblichen 1-basierten Adressen direkt eingeben — intern wird stets 0-basiert gespeichert.

### Byte- und Word-Order

Modbus überträgt 16-bit-Register grundsätzlich in **Big Endian** (high byte first). Bei Multi-Register-Werten (32/64 bit) ist die Reihenfolge der Register **nicht standardisiert** — vier Kombinationen sind in der Praxis anzutreffen:

```
Float32 = 0x12345678 wird je nach Gerät übertragen als:

ABCD (Big BE / Big WO, Default):     [0x1234, 0x5678]
CDAB (Big BE / Little WO):           [0x5678, 0x1234]
BADC (Little BE / Big WO):           [0x3412, 0x7856]
DCBA (Little BE / Little WO):        [0x7856, 0x3412]
```

Der Codec im Read- und Write-Pfad muss alle vier Kombinationen unterstützen. Beim Datentyp `raw` werden die Register **unverändert** durchgereicht — Byte/Word-Order spielt nur bei den getypten Varianten eine Rolle.

### Polling-Loop (Static Mode)

```go
func (n *ModbusReadNode) startPolling(ctx context.Context) {
    ticker := time.NewTicker(time.Duration(n.pollInterval) * time.Millisecond)
    defer ticker.Stop()

    var lastPayload any
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            value, raw, err := n.server.Read(n.unitId, n.fc, n.address, n.quantity)
            if err != nil {
                n.SetStatus(StatusError, err.Error())
                if n.emitOnError {
                    n.send(0, errorMessage(err))
                }
                continue
            }
            decoded := n.codec.Decode(value, raw)
            if n.emitOnChange && reflect.DeepEqual(decoded, lastPayload) {
                continue
            }
            lastPayload = decoded
            n.send(0, n.buildMessage(decoded, raw))
            n.SetStatus(StatusOk, fmt.Sprintf("verbunden · %dms", n.pollInterval))
        }
    }
}
```

### Dynamic-Mode Read

```go
func (n *ModbusReadNode) OnInput(msg Message) {
    fc       := pickInt(msg.Get("fc"), n.fc)
    address  := pickInt(msg.Get("address"), n.address)
    quantity := pickInt(msg.Get("quantity"), n.quantity)
    unitId   := pickByte(msg.Get("unitId"), n.unitId)
    dataType := pickString(msg.Get("dataType"), n.dataType)

    raw, err := n.server.Read(unitId, fc, address, quantity)
    if err != nil {
        n.SetStatus(StatusError, err.Error())
        if n.emitOnError {
            n.send(0, errorMessage(err))
        }
        return
    }
    decoded := n.codec.DecodeAs(dataType, raw)
    n.send(0, n.buildMessage(decoded, raw))
}
```

Eingehende Messages werden **nicht** weitergeleitet — der Output enthält ausschließlich das Read-Ergebnis. Steuer-Felder werden geprüft; `msg.payload` wird ignoriert (Read braucht keinen Eingangs-Payload).

### Request-Serialisierung im Server-Manager

```go
type ModbusServer struct {
    client modbus.Client       // goburrow/modbus
    mu     sync.Mutex          // serialisiert alle Requests dieses Servers
    // …
}

func (s *ModbusServer) Read(unitId byte, fc, addr, qty int) ([]byte, error) {
    s.mu.Lock()
    defer s.mu.Unlock()

    s.handler.SlaveId = unitId   // goburrow setzt SlaveId pro Request am Handler
    switch fc {
    case 1:  return s.client.ReadCoils(uint16(addr), uint16(qty))
    case 2:  return s.client.ReadDiscreteInputs(uint16(addr), uint16(qty))
    case 3:  return s.client.ReadHoldingRegisters(uint16(addr), uint16(qty))
    case 4:  return s.client.ReadInputRegisters(uint16(addr), uint16(qty))
    default: return nil, fmt.Errorf("unsupported read FC: %d", fc)
    }
}
```

Der Mutex sorgt dafür, dass auf dem Bus immer nur eine Modbus-Transaktion läuft — auch über mehrere Nodes hinweg, die denselben Server teilen.

### Reconnect-Strategie

- Bei TCP-Verbindungsverlust schließt `goburrow/modbus` den Socket. Der Server-Manager erkennt das beim nächsten Request über den Fehler-Return und versucht reconnect mit `reconnectBackoff` Sekunden Wartezeit
- Während des Reconnect-Versuchs sind alle wartenden Read/Write-Requests blockiert oder schlagen mit `Server unavailable` fehl (konfigurierbar — vorerst: blockieren bis zum Timeout)
- Status auf Gelb (`reconnecting…`) während des Versuchs, auf Rot wenn der Reconnect dauerhaft fehlschlägt

### Modbus Exception Codes

Das Modbus-Protokoll definiert eigene Exception Codes, die vom Slave als Fehler-Response geliefert werden:

| Code | Bedeutung | Typische Ursache |
|------|-----------|------------------|
| 0x01 | Illegal Function | Slave unterstützt diesen FC nicht |
| 0x02 | Illegal Data Address | Adresse außerhalb des gültigen Bereichs |
| 0x03 | Illegal Data Value | Wert außerhalb des Bereichs (z.B. zu großer Quantity) |
| 0x04 | Slave Device Failure | Slave hat einen internen Fehler |
| 0x05 | Acknowledge | Long-Running Operation, später nochmal anfragen |
| 0x06 | Slave Device Busy | Slave gerade nicht ansprechbar |

Diese Exceptions werden in der Status-Anzeige und im Catch-Output mit Code und Bedeutung sichtbar gemacht — das ist für die Inbetriebnahme essentiell.

## Abhängigkeiten

- **MQTT-Issue (`NODE_MQTT.md`)** führt das Config-Node-Konzept in der Engine ein. Modbus baut darauf auf und wiederverwendet:
  - `configs[]`-Abschnitt in `workspace.json`
  - `ConfigProvider`-Interface
  - Lifecycle-Reihenfolge (Config Nodes vor regulären Nodes starten / nach ihnen stoppen)
- Wenn das MQTT-Issue noch nicht gemerged ist, müssen die generischen Teile aus diesem Issue im Modbus-PR mit eingeführt werden — sollten aber strukturell identisch sein
- Frontend-Tokens für `modbus-read` / `modbus-write` sind bereits in `tokens.ts` definiert

## Abgrenzung / Nicht im Scope

- **Modbus ASCII**: nicht in v1 (kaum noch in produktiver Verwendung)
- **Modbus-Mapping-Datei** (z.B. CSV mit Tags wie `temperatur=40001:float32`): kann später als separates Feature kommen, vorerst bleibt jeder Read-Node ein eigenständiger Adressblock
- **Adress-Discovery / Browse**: Modbus kennt kein Discovery-Protokoll wie OPC-UA — der Anwender muss Adressen aus dem Geräte-Datenblatt entnehmen
- **Multi-Slave-Routing über RTU-Gateway**: ein Modbus-TCP-Gateway kann mehrere RTU-Slaves bündeln; jeder Slave wird über `unitId` adressiert. Das funktioniert mit dem aktuellen Design transparent — separate Server-Configs pro Gateway, Unit-ID pro Node
- **Encryption (Modbus Secure)**: nicht im Scope — Modbus ist historisch unverschlüsselt; in geschützten OT-Netzen oder hinter VPN
- **Function Codes außerhalb 1–6, 15, 16** (z.B. FC20/21 File Record, FC23 Read/Write Multiple): nicht in v1 — werden in der Industrie selten benötigt
- **Bit-Felder in Holding Registers** (z.B. Bit 3 von Register 40005): nicht in v1; lässt sich aktuell mit `dataType=uint16` und einem nachgelagerten Function-Node lösen
