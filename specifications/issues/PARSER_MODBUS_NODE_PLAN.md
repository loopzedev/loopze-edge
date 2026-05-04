# Plan: Modbus Parser Node

Implementation plan for [`PARSER_MODBUS_NODE.md`](./PARSER_MODBUS_NODE.md). A
new node `modbus-parser` that processes a declarative register layout
("address -> type -> field name") in both directions —
**parse** on read, **encode** on write.

## Naming confirmed

`modbus-parser` (backend type), category `parser`, UI label "Modbus Parser".
Consistent with `json` (backend type), but more explicit — the verb-free name
would collide with read and write of the Modbus stack.

## Architectural decisions up front

Three design questions to settle now instead of discovering during
implementation.

### A) Sparse encoding: no `mergeMode` knob

**Encode = sparse-zero, the only variant in v1.** Unset fields land
as `0` in the word array.

Rationale:
- "Read-modify-write" is two Modbus transactions, plus a race condition in
  between. Anyone who really needs that builds explicitly
  `[modbus-read] -> [function (merge)] -> [modbus-parser encode] -> [modbus-write]`
  — that is *more explicit* and therefore more correct than a knob that
  "magically" merges.
- Sparse-zero cleanly covers the most common use case (control register: 0 = "no
  command"). Bit fields are OR-aggregated, which covers "I only set
  bit 0" without moving the rest of the register.
- Document clearly in the issue — no code knob in v1.

### B) Bit fields in the encode path: pre-pass in the parser, not in the codec

`EncodeRegisters` works value-based (`value any -> []uint16`). Multiple
bit fields for the same register are a **layout property**, not a
codec concept. The codec stays free as it is; the parser node aggregates
before the encode:

```go
bitWords := map[int]uint16{}
for _, f := range n.layout {
    if f.Type != "bool" || f.Bit < 0 { continue }
    v, present := input[f.Name]
    if !present { continue }
    if b, _ := toBool(v); b { bitWords[f.Offset] |= 1 << uint(f.Bit) }
}
// then per offset once regs[offset] |= bitWords[offset]
```

### C) Layout editor complexity

- **Drag-reorder** via the existing `PropertyList`/`PropertyListItem` pattern
  (Switch/Change convention) — no up/down buttons.
- **Two-row layout per field**:
  - Row 1 (always visible): `offset` · `name` · `type` · `length` (only
    for string/raw) · `scale` · `unit` · ✕
  - Row 2 (per-row expander, ⚙ toggle): `byteOrder` · `wordOrder` · `bit` ·
    `offsetValue`
- Defaults for byte/word order live in the node header above the table.
- No table header — columns have placeholders/labels (consistent with
  Switch/Change, saves vertical space).

### D) `action=auto` heuristic

```go
input := msg.Get(parseFromOrEncodeFrom)
switch v := input.(type) {
case []byte:           parse(BytesToRegisters(v))
case []uint16:         parse(v)
case []int:            parse(toUint16Slice(v))
case []any:            parse(toUint16Slice(v))   // mix → codec error
case map[string]any:   encode(v)
case nil:              errCatch
default:               errCatch
}
```

`toUint16Slice` (codec) raises clean errors on mix — no extra pre-scan.

### E) Property defaults: two separate fields

`parseFrom` (default `bytes`) and `encodeFrom` (default `payload`).
Read and write conventions are different; a shared
`inputProperty` is ambiguous in `auto` mode. UI shows only the relevant
one depending on action; for `auto` both.

---

## Order

Backend first (1–4), frontend after (5–7). Within backend: data type
-> codec wrapper -> node -> registration.

```
[1] Layout data type + init validation
   ↓
[2] Parse + encode (wrapper around modbus_codec.go)
   ↓
[3] Node with action-auto dispatch
   ↓
[4] Backend registration in server.go      ← visible in editor afterwards
   ↓
[5] Frontend: ModbusParserLayoutEditor.vue
   ↓
[6] Frontend: ModbusParserConfig.vue        (wrapper around editor + action)
   ↓
[7] Frontend wiring: PropertyPanel, FlowEditor, tokens, BaseNode
```

**Pragmatic tip**: pull step 4 forward right after step 1 — once the type
is registered, the node can already be placed in the editor and the
frontend skeleton component developed in parallel with the backend codec.

---

## Step 1 — Backend: layout data type + validation

**New file:** `internal/nodes/modbus_parser.go` (only data type + init,
parse/encode comes in step 2).

```go
type modbusField struct {
    Offset      int
    Name        string
    Type        string  // "bool", "int16", ..., "string", "raw"
    Length      int     // only string/raw
    ByteOrder   ByteOrder // empty = node default
    WordOrder   WordOrder
    Scale       float64 // 0 → 1
    OffsetValue float64
    Unit        string  // display only
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

`Init` validates:
- at least 1 field
- per field: `name`, `type` set; `offset` >= 0
- names unique
- `string`/`raw`: `length` >= 1
- `bool` with `bit`: `bit` in [0,15]
- overlap: `slog.Warn`, no error. Except when two non-bit fields
  occupy exactly the same position -> error.

**Helper reuse:** `readIntProp` / `readFloatProp` from
`modbus_server.go` and `modbus_read.go`. `intVal` from `parser_json.go`.
Bit default as `int = -1` (no `*int` pointer sentinel).

**Smoke test criterion:** `go test` with the validation tests green.

**Required tests** (`modbus_parser_test.go`, new):

| Test | Expectation |
|---|---|
| `TestParser_LayoutValidate_DuplicateName` | init error |
| `TestParser_LayoutValidate_StringNeedsLength` | init error |
| `TestParser_LayoutValidate_BitOutOfRange` | init error |
| `TestParser_LayoutValidate_OverlapWarn` | no error, `slog.Warn` |
| `TestParser_LayoutValidate_OK_Mixed` | green layout with all data types |

---

## Step 2 — Backend: parse + encode

**Extends:** `internal/nodes/modbus_parser.go`.

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

1. **Determine block size:**
   `maxOffset = max(f.Offset + RegistersForType(f.Type, f.Length))`,
   `regs := make([]uint16, maxOffset)`
2. **Bit pre-pass** (see design decision B): bool fields with `bit` per
   offset OR-aggregated to a uint16.
3. **Main pass** per field:
   - bool+bit -> skip (covered by the pre-pass)
   - `f.Name` not in input -> skip (sparse-zero)
   - otherwise: `value = input[f.Name]`; on scaling
     `value, _ = UnapplyScale(value, scale, offsetValue)`; then
     `EncodeRegisters(...)` -> place in `regs[f.Offset:]`
4. Bit aggregate words: `regs[offset] |= bitWord` (OR with any existing
   value).

**Required tests:**

- `TestParser_Parse_AllTypes` — table-driven, all 11 types
- `TestParser_Parse_SparseLayout` — offsets 0, 4, 10, …
- `TestParser_Parse_InsufficientInput` — error contains field name
- `TestParser_Parse_PerFieldByteOrderOverride`
- `TestParser_Encode_Basic` (round-trip with parse)
- `TestParser_Encode_Sparse` — 2 of 5 set, rest = 0
- `TestParser_Encode_BitFieldsOR` — three bools on the same offset, all
  true -> all bits set
- `TestParser_Encode_ScaleInverse` — `pressure: 1.024` with `scale: 0.1`
  (float tolerance `1e-5`)
- `TestParser_RoundTrip_AllTypes` — encode -> parse must yield the original

---

## Step 3 — Backend: node with action-auto dispatch

**Extends:** `internal/nodes/modbus_parser.go`.

`HandleMessage` analogous to `JSONParserNode.HandleMessage`:

- Action `parse` -> input *must* be array/buffer
- Action `encode` -> input *must* be map
- Action `auto` -> type switch as in design decision D
- Output:
  - **Parse**: `msg.payload = result`
  - **Encode**: `msg.payload = []int (word array)`, `msg.bytes = []int`
    (byte array), `msg.address = minOffset` (if not yet set)
- Status pill red on error, empty on next success
- Catch integration via `n.errFn` and return value

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

**Required test:** `TestParser_AutoDispatch` — map -> encode, `[]int` -> parse,
`nil` -> catch error.

---

## Step 4 — Backend: registration

**Changed:** `internal/server/server.go`. One line directly under `json`,
before `context-watch` (groups the parser nodes thematically):

```go
registry.Register("modbus-parser", nodes.NewModbusParserNode, nodes.ModbusParserTypeInfo())
```

**Smoke test:** reload editor, palette shows "Modbus Parser" under
`parser`. Drag&drop places the node with default layout `[]`.

---

## Step 5 — Frontend: layout editor

**New:** `frontend/src/components/config/ModbusParserLayoutEditor.vue`.

Reuse:
- `PropertyList` (drag-reorder, add footer)
- `PropertyListItem` (drag handle, remove ✕, invalid-state border)
- `FormSelect`, `FormInput`, `NumberInput`

Per item two rows, second expandable via ⚙ toggle.

Row 1 (always visible):
```
[offset 56px] [name flex-1] [type 92px] [length? 48px] [scale 64px] [unit 56px] [⚙]
```

Row 2 (advanced):
```
[byteOrder] [wordOrder] [bit (only bool)] [offsetValue]
```

`byteOrder`/`wordOrder` options have a `""` entry with the label
"Default" to make the override character visible.

**Risks & mitigations:**

| Risk | Mitigation |
|---|---|
| Cyclic re-render | `useNodeProperty` setter strict; always `[...modelValue]` spread; no deep mutation |
| Stable item keys on reorder | `id: string` via `crypto.randomUUID()` on add. Defensively fill in missing IDs when reading from the store. Backend ignores the ID |
| Live validation of duplicate names | red border via `is-invalid` on the row. Backend additionally rejects on init |
| Confusion `offset` vs `offsetValue` | UI label "Address" for `offset`, "Offset (+)" or "Offset (value)" in the advanced drawer |

**Validation live, not on save** — the editor writes directly into the
store. Live border + backend init error on deploy cover the two layers.

---

## Step 6 — Frontend: properties panel

**New:** `frontend/src/components/config/ModbusParserConfig.vue`.

Wrapper around the layout editor with:
- Action selector (auto/parse/encode)
- `parseFrom` (only visible when action != encode)
- `encodeFrom` (only visible when action != parse)
- Default byte/word order
- `<ModbusParserLayoutEditor />`

Properties via `useNodeProperty<>()`.

---

## Step 7 — Frontend wiring

**Changed:**

1. `frontend/src/components/PropertyPanel.vue` — import
   `ModbusParserConfig`, dispatch:
   `<ModbusParserConfig v-else-if="selectedNode?.type === 'modbus-parser'" />`.
2. `frontend/src/components/nodes/tokens.ts` — `TYPE_CATEGORY` entry
   `'modbus-parser': 'rust'` (consistent with read/write).
3. `frontend/src/views/FlowEditor.vue` — `<template #node-modbus-parser>`
   between the other Modbus templates. Body slot shows
   `${layout.length} field(s) · ${action}`.
4. `frontend/src/components/nodes/BaseNode.vue` — extend `typeLabel` map:
   `"modbus-parser": "Modbus Parser"`.
5. `frontend/src/components/help/index.ts` — optional summary function.
6. **Check palette source**: if the node list comes server-side from the
   `/api/types` endpoint, step 4 is enough. If static in the frontend,
   add an entry there. Grep for `'json'` in the flowStore shows it.

**Smoke test:** start demo server, set up layout for holding 0..21 (temperature,
counter, pressure, setpoint, mode, tag, energy), wire read-parser-debug,
verify correct values in the debug panel. Encode path with
inject `{ setpoint: 250, mode: 2 }` -> parser -> write FC16 -> re-read with
the same layout must return exactly the object.

---

## Compact answers to the detailed questions

| Question | Answer |
|---|---|
| Sparse encoding `mergeMode`? | **No** in v1. Read-modify-write is a flow composition, not a knob. |
| Bit fields OR in codec or pre-pass? | **Pre-pass in the parser**, codec stays value-based. |
| Drag vs. up/down? | **Drag** via `PropertyList`. |
| Per-field override drawer vs. inline? | **Per-row expander** (⚙) with advanced sub-row. |
| Auto heuristic strict? | **Medium-strict**: `[]any` -> `toUint16Slice`, mix raises catch error. |
| Validation live or save? | **Live** (red border), additionally backend init on deploy. |
| Naming `modbus-parser`? | **Confirmed**, category `parser`. |
| `inputProperty` vs. `parseFrom`/`encodeFrom`? | **Two separate fields** — read and write conventions are different. |

---

## Critical Files

New files:
- `internal/nodes/modbus_parser.go`
- `internal/nodes/modbus_parser_test.go`
- `frontend/src/components/config/ModbusParserLayoutEditor.vue`
- `frontend/src/components/config/ModbusParserConfig.vue`

Changed files:
- `internal/server/server.go` — one line
- `frontend/src/components/PropertyPanel.vue` — import + dispatch
- `frontend/src/components/nodes/tokens.ts` — one line
- `frontend/src/views/FlowEditor.vue` — new template block
- `frontend/src/components/nodes/BaseNode.vue` — typeLabel entry
