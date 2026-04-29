# Plan: Modbus Parser Node

Implementierungsplan zu [`PARSER_MODBUS_NODE.md`](./PARSER_MODBUS_NODE.md). Ein
neuer Node `modbus-parser`, der ein deklaratives Register-Layout
(„Adresse → Typ → Feldname") in beide Richtungen verarbeitet —
**Parse** beim Lesen, **Encode** beim Schreiben.

## Naming bestätigt

`modbus-parser` (Backend-Type), Kategorie `parser`, UI-Label „Modbus Parser".
Konsistent zu `json` (Backend-Type), aber expliziter — der Verb-freie Name
würde mit Read und Write des Modbus-Stacks kollidieren.

## Architektur-Entscheidungen vorab

Drei Designfragen jetzt klären statt beim Implementieren entdecken.

### A) Sparse-Encoding: kein `mergeMode`-Knopf

**Encode = Sparse-Zero, einzige Variante in v1.** Nicht-gesetzte Felder
landen als `0` im Wort-Array.

Begründung:
- „Read-modify-write" sind zwei Modbus-Transaktionen, plus Race-Condition
  zwischendurch. Wer das wirklich braucht, baut explizit
  `[modbus-read] → [function (merge)] → [modbus-parser encode] → [modbus-write]`
  — das ist *expliziter* und damit korrekter als ein Knopf, der „magisch"
  merged.
- Sparse-Zero deckt den häufigsten Use Case (Steuerregister: 0 = "kein
  Befehl") sauber ab. Bit-Felder werden OR-aggregiert, das deckt „ich setze
  nur Bit 0" ab, ohne den Rest des Registers zu bewegen.
- Im Issue klar dokumentieren — kein Code-Knopf in v1.

### B) Bit-Felder im Encode-Pfad: Pre-Pass im Parser, nicht im Codec

`EncodeRegisters` arbeitet wertbasiert (`value any → []uint16`). Mehrere
Bit-Felder fürs gleiche Register sind eine **Layout-Eigenschaft**, kein
Codec-Konzept. Der Codec bleibt frei wie er ist; die Parser-Node aggregiert
vor dem Encode:

```go
bitWords := map[int]uint16{}
for _, f := range n.layout {
    if f.Type != "bool" || f.Bit < 0 { continue }
    v, present := input[f.Name]
    if !present { continue }
    if b, _ := toBool(v); b { bitWords[f.Offset] |= 1 << uint(f.Bit) }
}
// danach pro Offset einmal regs[offset] |= bitWords[offset]
```

### C) Layout-Editor-Komplexität

- **Drag-Reorder** via vorhandenem `PropertyList`/`PropertyListItem`-Pattern
  (Switch/Change-Konvention) — keine Up/Down-Buttons.
- **Zwei-Zeilen-Layout pro Feld**:
  - Zeile 1 (immer sichtbar): `offset` · `name` · `type` · `length` (nur
    bei string/raw) · `scale` · `unit` · ✕
  - Zeile 2 (Per-Row-Expander, ⚙-Toggle): `byteOrder` · `wordOrder` · `bit` ·
    `offsetValue`
- Defaults für Byte/Word-Order leben im Node-Header über der Tabelle.
- Kein Tabellen-Header — Spalten haben Placeholder/Labels (konsistent zu
  Switch/Change, spart vertikalen Platz).

### D) `action=auto`-Heuristik

```go
input := msg.Get(parseFromOrEncodeFrom)
switch v := input.(type) {
case []byte:           parse(BytesToRegisters(v))
case []uint16:         parse(v)
case []int:            parse(toUint16Slice(v))
case []any:            parse(toUint16Slice(v))   // Mix → Codec-Fehler
case map[string]any:   encode(v)
case nil:              errCatch
default:               errCatch
}
```

`toUint16Slice` (Codec) wirft bei Mix saubere Fehler — kein extra Vorscan.

### E) Property-Defaults: zwei separate Felder

`parseFrom` (Default `bytes`) und `encodeFrom` (Default `payload`).
Lese- und Schreib-Konvention sind unterschiedlich, ein gemeinsames
`inputProperty` ist im `auto`-Modus zweideutig. UI zeigt nur das relevante
je nach Action; bei `auto` beide.

---

## Reihenfolge

Backend zuerst (1–4), Frontend danach (5–7). Innerhalb von Backend: Datentyp
→ Codec-Wrapper → Node → Registrierung.

```
[1] Layout-Datentyp + Init-Validierung
   ↓
[2] Parse + Encode (Wrapper um modbus_codec.go)
   ↓
[3] Node mit Action-Auto-Dispatch
   ↓
[4] Backend-Registrierung in server.go      ← danach im Editor sichtbar
   ↓
[5] Frontend: ModbusParserLayoutEditor.vue
   ↓
[6] Frontend: ModbusParserConfig.vue        (Wrapper um Editor + Action)
   ↓
[7] Frontend-Wiring: PropertyPanel, FlowEditor, tokens, BaseNode
```

**Pragmatischer Tipp**: Schritt 4 nach Schritt 1 vorziehen — sobald der Type
registriert ist, kann man im Editor den Node bereits platzieren und die
Frontend-Skeleton-Komponente parallel zum Backend-Codec entwickeln.

---

## Schritt 1 — Backend: Layout-Datentyp + Validierung

**Neue Datei:** `internal/nodes/modbus_parser.go` (nur Datentyp + Init,
Parse/Encode kommt in Schritt 2).

```go
type modbusField struct {
    Offset      int
    Name        string
    Type        string  // "bool", "int16", ..., "string", "raw"
    Length      int     // nur string/raw
    ByteOrder   ByteOrder // leer = Node-Default
    WordOrder   WordOrder
    Scale       float64 // 0 → 1
    OffsetValue float64
    Unit        string  // nur Anzeige
    Bit         int     // -1 = unset
}

type ModbusParserNode struct {
    config flow.NodeConfig
    send   flow.SendFunc
    status flow.StatusFunc
    debug  flow.DebugFunc
    errFn  flow.ErrorFunc

    action     string // "auto" | "parse" | "encode"
    parseFrom  string
    encodeFrom string
    byteOrder  ByteOrder
    wordOrder  WordOrder
    layout     []modbusField

    inErrorState bool
}
```

`Init` validiert:
- mindestens 1 Feld
- pro Feld: `name`, `type` gesetzt; `offset` ≥ 0
- Namen eindeutig
- `string`/`raw`: `length` ≥ 1
- `bool` mit `bit`: `bit` ∈ [0,15]
- Überlappung: `slog.Warn`, kein Fehler. Außer zwei Nicht-Bit-Felder
  beanspruchen exakt dieselbe Position → Fehler.

**Helper-Wiederverwendung:** `readIntProp` / `readFloatProp` aus
`modbus_server.go` und `modbus_read.go`. `intVal` aus `parser_json.go`.
Bit-Default als `int = -1` (kein `*int`-Pointer-Sentinel).

**Smoke-Test-Kriterium:** `go test` mit den Validierungstests grün.

**Pflicht-Tests** (`modbus_parser_test.go`, neu):

| Test | Erwartung |
|---|---|
| `TestParser_LayoutValidate_DuplicateName` | Init-Fehler |
| `TestParser_LayoutValidate_StringNeedsLength` | Init-Fehler |
| `TestParser_LayoutValidate_BitOutOfRange` | Init-Fehler |
| `TestParser_LayoutValidate_OverlapWarn` | kein Fehler, `slog.Warn` |
| `TestParser_LayoutValidate_OK_Mixed` | grünes Layout mit allen Datentypen |

---

## Schritt 2 — Backend: Parse + Encode

**Erweitert:** `internal/nodes/modbus_parser.go`.

### `parse(regs []uint16) (map[string]any, error)`

```go
out := make(map[string]any, len(n.layout))
for _, f := range n.layout {
    regCount := RegistersForType(f.Type, f.Length)
    if f.Offset+regCount > len(regs) {
        return nil, fmt.Errorf("field %q: offset %d + %d regs > input %d",
            f.Name, f.Offset, regCount, len(regs))
    }
    slice := regs[f.Offset : f.Offset+regCount]
    bo, wo := n.effectiveOrders(f)

    v, err := DecodeRegisters(f.Type, bo, wo, slice)
    if err != nil { return nil, fmt.Errorf("field %q: %w", f.Name, err) }

    if f.Type == "bool" && f.Bit >= 0 {
        n, _ := toInt64(v)
        v = (n>>uint(f.Bit))&1 == 1
    }
    if f.Type != "string" && f.Type != "raw" {
        v = ApplyScale(v, scaleOrDefault(f.Scale), f.OffsetValue)
    }
    out[f.Name] = v
}
return out, nil
```

### `encode(input map[string]any) (regs []uint16, err error)`

1. **Block-Größe ermitteln:**
   `maxOffset = max(f.Offset + RegistersForType(f.Type, f.Length))`,
   `regs := make([]uint16, maxOffset)`
2. **Bit-Pre-Pass** (siehe Designentscheidung B): bool-Felder mit `bit` pro
   Offset zu uint16 OR-aggregieren.
3. **Hauptpass** pro Feld:
   - bool+bit → skip (vom Pre-Pass abgedeckt)
   - `f.Name` nicht in input → skip (sparse-zero)
   - sonst: `value = input[f.Name]`; bei Skalierung
     `value, _ = UnapplyScale(value, scale, offsetValue)`; dann
     `EncodeRegisters(...)` → ablegen in `regs[f.Offset:]`
4. Bit-Aggregat-Wörter: `regs[offset] |= bitWord` (OR mit ggf. existierendem
   Wert).

**Pflicht-Tests:**

- `TestParser_Parse_AllTypes` — table-driven, alle 11 Typen
- `TestParser_Parse_SparseLayout` — Offsets 0, 4, 10, …
- `TestParser_Parse_InsufficientInput` — Fehler enthält Feldnamen
- `TestParser_Parse_PerFieldByteOrderOverride`
- `TestParser_Encode_Basic` (Roundtrip mit Parse)
- `TestParser_Encode_Sparse` — 2 von 5 gesetzt, Rest = 0
- `TestParser_Encode_BitFieldsOR` — drei bools auf demselben Offset, alle
  true → alle Bits gesetzt
- `TestParser_Encode_ScaleInverse` — `pressure: 1.024` mit `scale: 0.1`
  (Float-Toleranz `1e-5`)
- `TestParser_RoundTrip_AllTypes` — encode → parse muss Original liefern

---

## Schritt 3 — Backend: Node mit Action-Auto-Dispatch

**Erweitert:** `internal/nodes/modbus_parser.go`.

`HandleMessage` analog `JSONParserNode.HandleMessage`:

- Action `parse` → input *muss* Array/Buffer sein
- Action `encode` → input *muss* Map sein
- Action `auto` → Type-Switch wie in Designentscheidung D
- Output:
  - **Parse**: `msg.payload = result`
  - **Encode**: `msg.payload = []int (Wort-Array)`, `msg.bytes = []int`
    (Byte-Array), `msg.address = minOffset` (falls noch nicht gesetzt)
- Status-Pill rot bei Fehler, leer beim nächsten Erfolg
- Catch-Integration über `n.errFn` und Rückgabewert

`ModbusParserTypeInfo`:

```go
return flow.NodeTypeInfo{
    Type:        "modbus-parser",
    Category:    "parser",
    Label:       "Modbus Parser",
    Description: "Parse register blocks into objects and back",
    Icon:        "memory",
    Defaults: map[string]any{
        "action":     "auto",
        "parseFrom":  "bytes",
        "encodeFrom": "payload",
        "byteOrder":  "bigEndian",
        "wordOrder":  "bigEndian",
        "layout":     []any{},
    },
    Inputs: 1, Outputs: 1,
}
```

**Pflicht-Test:** `TestParser_AutoDispatch` — Map → encode, `[]int` → parse,
`nil` → Catch-Fehler.

---

## Schritt 4 — Backend: Registrierung

**Geändert:** `internal/server/server.go`. Eine Zeile direkt unter `json`,
vor `context-watch` (gruppiert die Parser-Nodes thematisch):

```go
registry.Register("modbus-parser", nodes.NewModbusParserNode, nodes.ModbusParserTypeInfo())
```

**Smoke-Test:** Editor neu laden, Palette zeigt „Modbus Parser" unter
`parser`. Drag&Drop platziert den Node mit Default-Layout `[]`.

---

## Schritt 5 — Frontend: Layout-Editor

**Neu:** `frontend/src/components/config/ModbusParserLayoutEditor.vue`.

Wiederverwendung:
- `PropertyList` (Drag-Reorder, Add-Footer)
- `PropertyListItem` (Drag-Handle, Remove-X, invalid-State-Border)
- `FormSelect`, `FormInput`, `NumberInput`

Pro Item zwei Zeilen, zweite expandierbar via ⚙-Toggle.

Zeile 1 (immer sichtbar):
```
[offset 56px] [name flex-1] [type 92px] [length? 48px] [scale 64px] [unit 56px] [⚙]
```

Zeile 2 (advanced):
```
[byteOrder] [wordOrder] [bit (nur bool)] [offsetValue]
```

`byteOrder`/`wordOrder`-Optionen haben einen `""`-Eintrag mit Label
„Default" um den Override-Charakter sichtbar zu machen.

**Risiken & Maßnahmen:**

| Risiko | Maßnahme |
|---|---|
| Cyclic Re-Render | `useNodeProperty`-Setter strikt; immer `[...modelValue]`-Spread; keine tiefe Mutation |
| Stable Item Keys beim Reorder | `id: string` per `crypto.randomUUID()` beim Add. Beim Lesen aus dem Store fehlende IDs defensiv auffüllen. Backend ignoriert die ID |
| Live-Validierung doppelter Namen | rote Border via `is-invalid` auf der Zeile. Backend lehnt zusätzlich beim Init ab |
| Verwechslung `offset` ↔ `offsetValue` | UI-Label „Address" für `offset`, „Offset (+)" oder „Offset (value)" im Advanced-Drawer |

**Validierung live, nicht beim Save** — der Editor schreibt direkt in den
Store. Live-Border + Backend-Init-Fehler beim Deploy decken die zwei Layer ab.

---

## Schritt 6 — Frontend: Properties-Panel

**Neu:** `frontend/src/components/config/ModbusParserConfig.vue`.

Wrapper um den Layout-Editor mit:
- Action-Selector (auto/parse/encode)
- `parseFrom` (nur sichtbar wenn action ≠ encode)
- `encodeFrom` (nur sichtbar wenn action ≠ parse)
- Default-Byte/Word-Order
- `<ModbusParserLayoutEditor />`

Properties über `useNodeProperty<>()`.

---

## Schritt 7 — Frontend-Wiring

**Geändert:**

1. `frontend/src/components/PropertyPanel.vue` — Import
   `ModbusParserConfig`, Dispatch:
   `<ModbusParserConfig v-else-if="selectedNode?.type === 'modbus-parser'" />`.
2. `frontend/src/components/nodes/tokens.ts` — `TYPE_CATEGORY` Eintrag
   `'modbus-parser': 'rust'` (konsistent zu read/write).
3. `frontend/src/views/FlowEditor.vue` — `<template #node-modbus-parser>`
   zwischen den anderen Modbus-Templates. Body-Slot zeigt
   `${layout.length} field(s) · ${action}`.
4. `frontend/src/components/nodes/BaseNode.vue` — `typeLabel`-Map ergänzen:
   `"modbus-parser": "Modbus Parser"`.
5. `frontend/src/components/help/index.ts` — optional Summary-Funktion.
6. **Palette-Quelle prüfen**: wenn die Node-Liste server-seitig aus dem
   `/api/types`-Endpoint kommt, reicht Schritt 4. Wenn statisch im Frontend,
   dort einen Eintrag ergänzen. Grep nach `'json'` im flowStore zeigt das.

**Smoke-Test:** Demo-Server starten, Layout für Holding 0..21 (Temperature,
Counter, Pressure, Setpoint, Mode, Tag, Energy) anlegen, Read-Parser-Debug
verdrahten, korrekte Werte im Debug-Panel verifizieren. Encode-Pfad mit
Inject `{ setpoint: 250, mode: 2 }` → Parser → Write FC16 → Re-Read mit
gleichem Layout muss exakt das Object zurückgeben.

---

## Antworten auf die Detail-Fragen kompakt

| Frage | Antwort |
|---|---|
| Sparse-Encoding `mergeMode`? | **Nein** in v1. Read-modify-write ist eine Flow-Komposition, kein Knopf. |
| Bit-Felder OR im Codec oder Pre-Pass? | **Pre-Pass im Parser**, Codec bleibt wertbasiert. |
| Drag vs. Up/Down? | **Drag** via `PropertyList`. |
| Per-Field-Override Drawer vs. Inline? | **Per-Row-Expander** (⚙) mit Advanced-Subzeile. |
| Auto-Heuristik streng? | **Mittelstreng**: `[]any` → `toUint16Slice`, Mix wirft Catch-Fehler. |
| Validierung live oder Save? | **Live** (Border-rot), zusätzlich Backend-Init beim Deploy. |
| Naming `modbus-parser`? | **Bestätigt**, Kategorie `parser`. |
| `inputProperty` vs. `parseFrom`/`encodeFrom`? | **Zwei separate Felder** — Lese- und Schreib-Konvention sind unterschiedlich. |

---

## Critical Files

Neue Dateien:
- `internal/nodes/modbus_parser.go`
- `internal/nodes/modbus_parser_test.go`
- `frontend/src/components/config/ModbusParserLayoutEditor.vue`
- `frontend/src/components/config/ModbusParserConfig.vue`

Geänderte Dateien:
- `internal/server/server.go` — eine Zeile
- `frontend/src/components/PropertyPanel.vue` — Import + Dispatch
- `frontend/src/components/nodes/tokens.ts` — eine Zeile
- `frontend/src/views/FlowEditor.vue` — neuer Template-Block
- `frontend/src/components/nodes/BaseNode.vue` — typeLabel-Eintrag
