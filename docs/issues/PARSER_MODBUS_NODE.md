# Issue: Modbus Parser Node — Register-Layout deklarativ parsen & encodieren

## Status: Open

## Problembeschreibung

Mit `modbus-read` (`dataType: raw`) holen Anwender heute rohe Register-Blöcke
und müssen sie dann im **Function Node mit der Buffer-API** zerlegen. Das
funktioniert (das Beispiel in `demo/modbus-server/example-flow.json` zeigt
es), führt aber pro Modbus-Gerät zu zweistelligen Codezeilen Boilerplate für
ein eigentlich rein deklaratives Mapping „Adresse → Datentyp → Feldname".

Industrie-Geräte beschreiben ihren Register-Map im Datenblatt als Tabelle:

| Offset | Name | Typ | Skalierung | Einheit |
| ------ | ---- | --- | ---------- | ------- |
| 0..1   | temperature | float32 | 1 | °C |
| 2..3   | counter     | uint32  | 1 | — |
| 4..5   | pressure    | float32 | 0.1 | bar |
| 6      | setpoint    | int16   | 1 | — |
| 10..14 | tag         | string  | — | — |

Genau diese Tabelle soll der **Modbus Parser Node** sein — eine Konfiguration
pro Gerät, in beide Richtungen verwendbar (parse beim Lesen, encode beim
Schreiben).

## Sichtweise / Begründung

- **Bediener-UX vor Designer-Power**: Eine Tabelle mit Add/Delete/Reorder ist
  zugänglicher als JS-Code. Datenblatt → Layout → fertig.
- **Pendant zum JSON Parser**: gleiches Aktions-Modell (`auto`/`parse`/`encode`),
  gleiches Fehlerverhalten, gleiche Status-Anbindung. Sitzt zusammen mit den
  anderen Parser-Nodes in der Palette.
- **DRY**: ein Layout, beide Richtungen. Ein FC3-Read und ein FC16-Write auf
  denselben Adressblock teilen sich dasselbe Schema.
- **Keine Modbus-Read/Write-Änderung nötig**: der Parser arbeitet rein auf
  `msg.bytes` bzw. `msg.payload` — sitzt zwischen dem `modbus-read` und dem
  Verbraucher (parse) bzw. zwischen Erzeuger und `modbus-write` (encode).

## Übersicht

| Node | Typ-ID | Canvas Inputs | Canvas Outputs | Beschreibung |
|---|---|---|---|---|
| **Modbus Parser** | `modbus-parser` | 1 | 1 | Parst Register-Blöcke nach Layout (parse) bzw. baut sie aus strukturiertem Payload zusammen (encode) |

```
[modbus-read raw]  ──→  [modbus-parser parse]  ──→  [Switch / Function / Debug …]
                                  ↑
                             Layout: 5 Felder

[Inject struct]    ──→  [modbus-parser encode] ──→  [modbus-write raw fc=16]
                                  ↑
                          (gleiches Layout)
```

## Anforderungen

### 1. Action-Modell (analog Parser-Nodes)

| Action | Verhalten |
|---|---|
| `auto` | Heuristik: `msg.bytes`/`msg.payload` ist Array → **parse**; ist Object/Map → **encode** |
| `parse` | Erzwingt parse — Fehler wenn Input nicht Array/Buffer |
| `encode` | Erzwingt encode — Fehler wenn Input nicht Object/Map |

`auto` ist Default und deckt 90 % der Fälle ab — Layout liegt fest, Richtung ergibt sich aus dem Datenfluss.

### 2. Eingabequelle

Konfigurierbar, welches msg-Feld das Roh-Material trägt:

- **Parse-Pfad**: Default `msg.bytes` (passt direkt zur neuen Read-Output-Form). Alternativen:
  - `msg.payload` (wenn dort ein `[]uint16`-Wort-Array sitzt)
  - eigener Pfad, frei konfigurierbar (`msg.modbus.raw`, …)
- **Encode-Pfad**: Default `msg.payload` (Object). Frei konfigurierbar.

### 3. Ausgabe

- **Parse**: erzeugt `msg.payload` als `map[string]any` mit den Layout-Feldnamen als Schlüsseln. Ungenutzte msg-Felder bleiben unverändert.
- **Encode**: erzeugt `msg.payload` als `[]int` (Wort-Array, wire-ready) und zusätzlich `msg.bytes` (Byte-Array). Optional `msg.address` mit dem niedrigsten Layout-Offset (für direkte Weiterleitung an `modbus-write` ohne address-Override).

### 4. Layout-Definition

Ein Array von Feld-Definitionen:

```json
{
  "layout": [
    { "offset": 0,  "name": "temperature", "type": "float32", "scale": 1, "unit": "°C" },
    { "offset": 2,  "name": "counter",     "type": "uint32" },
    { "offset": 4,  "name": "pressure",    "type": "float32", "scale": 0.1, "unit": "bar" },
    { "offset": 6,  "name": "setpoint",    "type": "int16" },
    { "offset": 7,  "name": "mode",        "type": "uint16" },
    { "offset": 10, "name": "tag",         "type": "string", "length": 5 },
    { "offset": 20, "name": "energy",      "type": "float32", "scale": 0.001, "unit": "kWh" }
  ]
}
```

Pro Feld:

| Feld | Pflicht | Typ | Beschreibung |
| ---- | ------- | --- | ------------ |
| `offset` | ✓ | number | 0-basierter Register-Offset im Eingangsblock |
| `name` | ✓ | string | Schlüssel im Output-Object |
| `type` | ✓ | enum | `bool`, `int16`, `uint16`, `int32`, `uint32`, `float32`, `int64`, `uint64`, `float64`, `string`, `raw` |
| `length` | bei `string`/`raw` | number | Anzahl Register (= 2× ASCII-Zeichen bei String) |
| `byteOrder` | optional | enum | `bigEndian`/`littleEndian`, Default = Node-Wert |
| `wordOrder` | optional | enum | `bigEndian`/`littleEndian`, Default = Node-Wert |
| `scale` | optional | number | Multiplikativer Faktor, Default `1` |
| `offset_value` | optional | number | Additiver Offset, Default `0` (Konflikt mit `offset` als Adresse — UI-Label „Offset (value)" / „Adresse" trennt die beiden) |
| `unit` | optional | string | Reine Anzeige (im Properties-Panel & Debug-Hint), wird nicht im Wire-Format mitgegeben |
| `bit` | optional, nur `bool` mit `int*` | number | Einzelnes Bit aus einem Register lesen — z.B. Bit 3 von Holding 5 |

**Offsets sind 0-basiert auf den Eingangsblock**, nicht auf die absolute
Modbus-Adresse. Wer ab Adresse 100 liest, definiert seine Felder weiterhin mit
`offset: 0..n`. Das macht das Layout portabel zwischen verschiedenen
Anwendungs-Adressen.

### 5. Node-Level Defaults

Pro Node konfigurierbar (gelten als Default für alle Felder, die es nicht
explizit überschreiben):

- `byteOrder` — `bigEndian` (Default) / `littleEndian`
- `wordOrder` — `bigEndian` (ABCD, Default) / `littleEndian` (CDAB)

Damit muss man die Order pro Gerät genau einmal setzen, nicht pro Feld.

### 6. Validierung

Beim Init **und** beim Parse/Encode:

- Layout muss mindestens 1 Feld haben
- Pro Feld müssen `offset`, `name`, `type` gesetzt sein
- Feldnamen müssen eindeutig sein (sonst Init-Fehler)
- Felder dürfen sich nicht **überlappen** (Warning beim Init, kein Fehler — bewusste Mehrfach-Interpretation derselben Register, z.B. uint16 und 16× bool, ist erlaubt)
- Bei `string`/`raw`: `length` ≥ 1
- Bei `bool` mit `bit`: `bit` ∈ [0, 15]

Beim Parse: Reicht das Eingangs-Wort-Array für den höchsten Offset+Länge? Sonst Fehler in Catch-Pfad und kein Output.

### 7. Action-Dispatch

Pseudocode für `auto`:

```go
input := msg.Get(node.inputProperty) // default "bytes"
switch v := input.(type) {
case []byte, []int, []uint16:
    parse(input, layout) → object
case map[string]any:
    encode(input, layout) → registers + bytes
case nil:
    // kein Input → Catch-Fehler
default:
    // unbekannter Typ → Catch-Fehler
}
```

### 8. Fehlerverhalten

- Layout-Init-Fehler (doppelte Namen, fehlendes type): Node startet nicht, Status rot.
- Parse/Encode-Fehler zur Laufzeit: über `flow.ErrorProvider` an Catch-Pfad. Kein Output am 0-Port.
- `dataType=string` mit Bytes ≠ ASCII/UTF-8: liefert die Bytes trotzdem als String (mit Trailing-NUL-Trimm), kein Fehler — analog zum Codec-Verhalten in `modbus_read`.

### 9. Properties-Panel

Layout-Editor als Tabelle, vergleichbar mit dem Switch-/Change-Node-Editor:

```
┌──────────────────────────────────────────────────────────────────┐
│  Modbus Parser                                                    │
├──────────────────────────────────────────────────────────────────┤
│                                                                   │
│  Action                                                           │
│  ( • ) Auto    ( ) Parse only    ( ) Encode only                  │
│                                                                   │
│  Input Property                                                   │
│  ┌──────────────────────┐                                        │
│  │ msg.bytes            │ ← default; switchable to msg.payload   │
│  └──────────────────────┘                                        │
│                                                                   │
│  Default Byte Order:  [Big Endian ▼]                              │
│  Default Word Order:  [Big Endian (ABCD) ▼]                       │
│                                                                   │
│  Layout                                                           │
│  ┌────┬────────────┬─────────┬──────┬───────┬────────┬──────┬───┐│
│  │ ↕  │ Offset │ Name      │ Type    │ Len  │ Scale │ Unit   │ ✕ ││
│  ├────┼────────┼───────────┼─────────┼──────┼───────┼────────┼───┤│
│  │ ⋮  │   0    │ temperature│ float32│  —   │  1    │ °C     │ × ││
│  │ ⋮  │   2    │ counter    │ uint32 │  —   │  1    │        │ × ││
│  │ ⋮  │   4    │ pressure   │ float32│  —   │  0.1  │ bar    │ × ││
│  │ ⋮  │   6    │ setpoint   │ int16  │  —   │  1    │        │ × ││
│  │ ⋮  │  10    │ tag        │ string │  5   │  —    │        │ × ││
│  └────┴────────┴───────────┴─────────┴──────┴───────┴────────┴───┘│
│  [+ Add field]                                                   │
│                                                                   │
│  ▸ Per-Field Override (klappt aus): byteOrder, wordOrder, bit     │
│                                                                   │
└──────────────────────────────────────────────────────────────────┘
```

Per-Field-Override (Byte/Word-Order, `bit`) ist ausklappbar — selten gebraucht, soll die Default-Tabelle nicht überfrachten.

## Datenstruktur

### workspace.json

```json
{
  "id": "node-parser-1",
  "type": "modbus-parser",
  "name": "Energiezähler XPM-2000",
  "x": 600, "y": 200, "z": "flow-1",
  "inputs": 1, "outputs": 1,
  "wires": [["..."]],
  "config": {
    "action": "auto",
    "inputProperty": "bytes",
    "byteOrder": "bigEndian",
    "wordOrder": "bigEndian",
    "layout": [
      { "offset": 0,  "name": "temperature", "type": "float32" },
      { "offset": 2,  "name": "counter",     "type": "uint32"  },
      { "offset": 4,  "name": "pressure",    "type": "float32", "scale": 0.1 },
      { "offset": 6,  "name": "setpoint",    "type": "int16"   },
      { "offset": 10, "name": "tag",         "type": "string", "length": 5 }
    ]
  }
}
```

## Beispiel

### Parse: Demo-Server Block 0..21 nach Object

**Modbus Read** (FC3, address=0, quantity=22, dataType=raw)
liefert `msg.bytes` mit 44 Bytes.

**Modbus Parser** (action=auto, Layout siehe oben) macht daraus:

```json
{
  "payload": {
    "temperature": 21.487,
    "counter": 1284,
    "pressure": 1.024,
    "setpoint": 200,
    "tag": "FLINT-DEMO",
    "energy": 0.214
  },
  "modbus": { ... }
}
```

### Encode: Object zurück nach Wort-Array

**Inject** mit `msg.payload = { setpoint: 250, mode: 2 }`,
**Modbus Parser** (action=encode, Layout vom selben Gerät, aber **nur** die zwei Felder
werden gesetzt — `mode` an Offset 7, `setpoint` an Offset 6) liefert:

```json
{
  "payload": [0, 250, 2, ...],   // sparse-encoded, restliche Wörter = 0
  "bytes":   [..., 0xFA, 0, 2, ...],
  "address": 6                    // niedrigster Offset im Layout
}
```

→ direkt an **Modbus Write** (FC16, dataType=raw, address aus msg.address oder fest) anhängbar.

**Sparse-Encoding-Hinweis**: Beim Encode wird der Block so groß wie der höchste Offset+Länge im Layout. Nicht-gesetzte Felder im Input-Object → Wert 0. Wer den vorherigen Read als Basis braucht („read-modify-write"), liest erst, parst, mergt im Function-Node und encodet dann das vollständige Object.

## Betroffene Dateien

### Backend – Neue Dateien

- `internal/nodes/modbus_parser.go` — Parser-Node-Implementation. Wiederverwendung von `modbus_codec.go` (DecodeRegisters, EncodeRegisters, RegistersToBytes, BytesToRegisters)
- `internal/nodes/modbus_parser_test.go` — Layout-Validierung, Parse-Roundtrip, Encode-Roundtrip, Bit-Extraktion, Sparse-Encode-Verhalten

### Backend – Anpassungen

- `internal/server/server.go` — Registrierung: `registry.Register("modbus-parser", nodes.NewModbusParserNode, nodes.ModbusParserTypeInfo())`

### Frontend – Neue Dateien

- `frontend/src/components/config/ModbusParserConfig.vue` — Properties-Panel inkl. Layout-Tabelle
- `frontend/src/components/config/ModbusParserLayoutEditor.vue` — Tabellen-Editor (Reorder, Add/Delete, Per-Field-Override-Expand). Falls dasselbe Pattern für andere Layout-Editor wiederkehrt (z.B. später ein Binary-Parser für andere Protokolle), wird er in einen wiederverwendbaren Editor extrahiert

### Frontend – Anpassungen

- `frontend/src/components/PropertyPanel.vue` — Dispatch für `modbus-parser`
- `frontend/src/components/nodes/tokens.ts` — `modbus-parser` mappt auf die `rust`-Palette (analog zu read/write)
- `frontend/src/views/FlowEditor.vue` — Template `#node-modbus-parser`
- `frontend/src/components/nodes/BaseNode.vue` — `typeLabel`-Map: `modbus-parser` → "Modbus Parser"

## Technische Hinweise

### Wiederverwendung des Codecs

`modbus_codec.go` deckt das vollständige Encode/Decode bereits ab — der Parser braucht nur einen dünnen Wrapper, der pro Layout-Feld an die richtige Stelle im Wort-Array greift:

```go
func (n *ModbusParserNode) parse(regs []uint16) (map[string]any, error) {
    out := make(map[string]any, len(n.layout))
    for _, f := range n.layout {
        regCount := RegistersForType(f.dataType, f.length)
        if f.offset+regCount > len(regs) {
            return nil, fmt.Errorf("field %q: offset %d + %d regs > input %d",
                f.name, f.offset, regCount, len(regs))
        }
        slice := regs[f.offset : f.offset+regCount]

        bo := f.byteOrder
        if bo == "" { bo = n.byteOrder }
        wo := f.wordOrder
        if wo == "" { wo = n.wordOrder }

        v, err := DecodeRegisters(f.dataType, bo, wo, slice)
        if err != nil { return nil, fmt.Errorf("field %q: %w", f.name, err) }

        v = ApplyScale(v, f.scale, f.offsetValue)
        if f.bit >= 0 { v = extractBit(v, f.bit) }
        out[f.name] = v
    }
    return out, nil
}
```

Encode ist symmetrisch — `EncodeRegisters` füllt einen Wort-Slice an der richtigen Position, `RegistersToBytes` baut den Byte-Array daraus.

### Bit-Felder

Wenn `type=bool` UND `bit` gesetzt ist, wird das Feld als einzelnes Bit aus einem 16-bit-Register interpretiert. Anwendungsfall: SPS-Statusbits („Bit 0 = Motor läuft, Bit 1 = Störung, Bit 7 = Wartung").

```yaml
- offset: 5, name: "motor_running", type: "bool", bit: 0
- offset: 5, name: "fault",         type: "bool", bit: 1
- offset: 5, name: "maintenance",   type: "bool", bit: 7
```

Encode: das Register wird sparse zusammengebaut — alle bool-Felder mit gleichem Offset werden in einem Register OR-verknüpft. Felder, die **nicht** im Encode-Input sind, bleiben 0.

### Auto-Detection-Heuristik

```go
input := msg.Get(n.inputProperty)
switch v := input.(type) {
case []any:
    // sparse: alle Elemente Number? → parse, sonst Fehler
case []uint16, []int:
    return n.parse(toUint16Slice(v))
case []byte:
    return n.parse(BytesToRegisters(v))
case map[string]any:
    return n.encode(v)
case nil:
    return errCatch("input %q missing", n.inputProperty)
default:
    return errCatch("unknown input type %T", v)
}
```

`[]any` ist der häufigste Fall (JSON-decoded), deshalb extra geprüft.

## Abgrenzung / Nicht im Scope

- **Generischer Binary-Parser** für andere Protokolle (S7, OPC-UA, raw TCP-Frames): vorerst Modbus-spezifisch. Wenn drei Protokolle dieselbe Tabelle brauchen, extrahieren wir den Layout-Editor zu einem `binary-parser`-Node — bis dahin nicht.
- **Validierungs-Regeln pro Feld** (z.B. „temperature muss 0..100 sein, sonst Fehler"): nicht in v1. Lässt sich nachgelagert mit Switch-Node oder Function-Node erledigen.
- **Bedingte Felder** („wenn `mode==1`, dann an Offset 8 ist `value` ein int16, sonst float32"): nicht in v1. Wer das braucht, parst zwei Layouts und switcht.
- **CSV-Import des Layouts**: nicht in v1. Manuelle Tabelle reicht für die ersten 80 % der Use Cases.
- **Coil-Layouts** (Layout über `[]bool` aus FC1/FC2): nicht in v1 — Coils sind selten als Block strukturiert, dafür reicht der Direkt-Zugriff auf das `[]bool`-Array.

## Abhängigkeiten

- `modbus_codec.go` — bereits da, wird wiederverwendet
- Catch-Node-Integration über bestehendes `flow.ErrorProvider` — keine Engine-Änderung
- Keine externen Go-Dependencies
