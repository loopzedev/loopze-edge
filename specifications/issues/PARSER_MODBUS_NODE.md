# Issue: Modbus Parser Node — declaratively parse & encode register layouts

## Status: Open

## Problem description

With `modbus-read` (`dataType: raw`), users today fetch raw register blocks
and then have to dissect them in the **Function node with the buffer API**.
That works (the example in `demo/modbus-server/example-flow.json` shows
it), but per Modbus device leads to two-digit lines of boilerplate code for
what is essentially purely declarative mapping "address -> data type -> field name".

Industrial devices describe their register map in the data sheet as a table:

| Offset | Name | Type | Scale | Unit |
| ------ | ---- | --- | ----- | ---- |
| 0..1   | temperature | float32 | 1 | °C |
| 2..3   | counter     | uint32  | 1 | — |
| 4..5   | pressure    | float32 | 0.1 | bar |
| 6      | setpoint    | int16   | 1 | — |
| 10..14 | tag         | string  | — | — |

This very table should be the **Modbus Parser node** — one configuration
per device, usable in both directions (parse on read, encode on write).

## Perspective / rationale

- **Operator UX before designer power**: a table with add/delete/reorder is
  more accessible than JS code. Data sheet -> layout -> done.
- **Counterpart to the JSON Parser**: same action model (`auto`/`parse`/`encode`),
  same error behavior, same status binding. Sits together with the
  other parser nodes in the palette.
- **DRY**: one layout, both directions. An FC3 read and an FC16 write on
  the same address block share the same schema.
- **No Modbus read/write change required**: the parser works purely on
  `msg.bytes` or `msg.payload` — sits between the `modbus-read` and the
  consumer (parse) or between producer and `modbus-write` (encode).

## Overview

| Node | Type ID | Canvas inputs | Canvas outputs | Description |
|---|---|---|---|---|
| **Modbus Parser** | `modbus-parser` | 1 | 1 | Parses register blocks per layout (parse) or assembles them from a structured payload (encode) |

```
[modbus-read raw]  ──→  [modbus-parser parse]  ──→  [Switch / Function / Debug …]
                                  ↑
                             Layout: 5 fields

[Inject struct]    ──→  [modbus-parser encode] ──→  [modbus-write raw fc=16]
                                  ↑
                          (same layout)
```

## Requirements

### 1. Action model (analogous to parser nodes)

| Action | Behavior |
|---|---|
| `auto` | Heuristic: `msg.bytes`/`msg.payload` is array -> **parse**; is object/map -> **encode** |
| `parse` | Forces parse — error if input is not array/buffer |
| `encode` | Forces encode — error if input is not object/map |

`auto` is the default and covers 90% of cases — the layout is fixed, direction follows from the dataflow.

### 2. Input source

Configurable, which msg field carries the raw material:

- **Parse path**: default `msg.bytes` (matches the new read output form directly). Alternatives:
  - `msg.payload` (when a `[]uint16` word array sits there)
  - own path, freely configurable (`msg.modbus.raw`, …)
- **Encode path**: default `msg.payload` (object). Freely configurable.

### 3. Output

- **Parse**: produces `msg.payload` as a `map[string]any` with the layout field names as keys. Unused msg fields stay unchanged.
- **Encode**: produces `msg.payload` as `[]int` (word array, wire-ready) and additionally `msg.bytes` (byte array). Optionally `msg.address` with the lowest layout offset (for direct forwarding to `modbus-write` without an address override).

### 4. Layout definition

An array of field definitions:

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

Per field:

| Field | Required | Type | Description |
| ----- | -------- | ---- | ----------- |
| `offset` | ✓ | number | 0-based register offset within the input block |
| `name` | ✓ | string | key in the output object |
| `type` | ✓ | enum | `bool`, `int16`, `uint16`, `int32`, `uint32`, `float32`, `int64`, `uint64`, `float64`, `string`, `raw` |
| `length` | for `string`/`raw` | number | number of registers (= 2x ASCII chars for string) |
| `byteOrder` | optional | enum | `bigEndian`/`littleEndian`, default = node value |
| `wordOrder` | optional | enum | `bigEndian`/`littleEndian`, default = node value |
| `scale` | optional | number | Multiplicative factor, default `1` |
| `offset_value` | optional | number | Additive offset, default `0` (collision with `offset` as address — UI label "Offset (value)" / "Address" separates the two) |
| `unit` | optional | string | Pure display (in properties panel & debug hint), not carried in the wire format |
| `bit` | optional, only `bool` with `int*` | number | Read a single bit from a register — e.g. bit 3 of holding 5 |

**Offsets are 0-based on the input block**, not on the absolute
Modbus address. Whoever reads from address 100 still defines their fields with
`offset: 0..n`. That makes the layout portable across different
application addresses.

### 5. Node-level defaults

Configurable per node (apply as defaults for all fields that do not
explicitly override them):

- `byteOrder` — `bigEndian` (default) / `littleEndian`
- `wordOrder` — `bigEndian` (ABCD, default) / `littleEndian` (CDAB)

That way the order has to be set per device exactly once, not per field.

### 6. Validation

On init **and** on parse/encode:

- Layout must have at least 1 field
- Per field, `offset`, `name`, `type` must be set
- Field names must be unique (otherwise init error)
- Fields must not **overlap** (warning on init, no error — deliberate multiple interpretation of the same registers, e.g. uint16 and 16x bool, is allowed)
- For `string`/`raw`: `length` >= 1
- For `bool` with `bit`: `bit` in [0, 15]

On parse: is the input word array sufficient for the highest offset+length? Otherwise error in catch path and no output.

### 7. Action dispatch

Pseudocode for `auto`:

```go
input := msg.Get(node.inputProperty) // default "bytes"
switch v := input.(type) {
case []byte, []int, []uint16:
    parse(input, layout) → object
case map[string]any:
    encode(input, layout) → registers + bytes
case nil:
    // no input → catch error
default:
    // unknown type → catch error
}
```

### 8. Error behavior

- Layout init error (duplicate names, missing type): node does not start, status red.
- Parse/encode error at runtime: routed via `flow.ErrorProvider` to the catch path. No output on port 0.
- `dataType=string` with bytes != ASCII/UTF-8: returns the bytes as a string anyway (with trailing-NUL trim), no error — analogous to the codec behavior in `modbus_read`.

### 9. Properties panel

Layout editor as a table, comparable to the Switch/Change node editor:

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
│  ▸ Per-field override (collapses out): byteOrder, wordOrder, bit  │
│                                                                   │
└──────────────────────────────────────────────────────────────────┘
```

Per-field override (byte/word order, `bit`) is collapsible — rarely needed, should not clutter the default table.

## Data structure

### workspace.json

```json
{
  "id": "node-parser-1",
  "type": "modbus-parser",
  "name": "Energy meter XPM-2000",
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

## Example

### Parse: demo server block 0..21 to object

**Modbus Read** (FC3, address=0, quantity=22, dataType=raw)
delivers `msg.bytes` with 44 bytes.

**Modbus Parser** (action=auto, layout see above) turns it into:

```json
{
  "payload": {
    "temperature": 21.487,
    "counter": 1284,
    "pressure": 1.024,
    "setpoint": 200,
    "tag": "LOOPZE-DEMO",
    "energy": 0.214
  },
  "modbus": { ... }
}
```

### Encode: object back to word array

**Inject** with `msg.payload = { setpoint: 250, mode: 2 }`,
**Modbus Parser** (action=encode, layout from the same device, but **only** the two fields
are set — `mode` at offset 7, `setpoint` at offset 6) delivers:

```json
{
  "payload": [0, 250, 2, ...],   // sparse-encoded, remaining words = 0
  "bytes":   [..., 0xFA, 0, 2, ...],
  "address": 6                    // lowest offset in the layout
}
```

-> directly attachable to **Modbus Write** (FC16, dataType=raw, address from msg.address or fixed).

**Sparse-encoding note**: on encode, the block is sized to the highest offset+length in the layout. Unset fields in the input object -> value 0. Anyone who needs the previous read as the basis ("read-modify-write") reads first, parses, merges in the Function node, and then encodes the complete object.

## Affected files

### Backend – new files

- `internal/nodes/modbus_parser.go` — parser node implementation. Reuse of `modbus_codec.go` (DecodeRegisters, EncodeRegisters, RegistersToBytes, BytesToRegisters)
- `internal/nodes/modbus_parser_test.go` — layout validation, parse round-trip, encode round-trip, bit extraction, sparse-encode behavior

### Backend – changes

- `internal/server/server.go` — registration: `registry.Register("modbus-parser", nodes.NewModbusParserNode, nodes.ModbusParserTypeInfo())`

### Frontend – new files

- `frontend/src/components/config/ModbusParserConfig.vue` — properties panel including layout table
- `frontend/src/components/config/ModbusParserLayoutEditor.vue` — table editor (reorder, add/delete, per-field-override expand). If the same pattern recurs for other layout editors (e.g. later a binary parser for other protocols), it gets extracted into a reusable editor

### Frontend – changes

- `frontend/src/components/PropertyPanel.vue` — dispatch for `modbus-parser`
- `frontend/src/components/nodes/tokens.ts` — `modbus-parser` maps to the `rust` palette (analogous to read/write)
- `frontend/src/views/FlowEditor.vue` — template `#node-modbus-parser`
- `frontend/src/components/nodes/BaseNode.vue` — `typeLabel` map: `modbus-parser` -> "Modbus Parser"

## Technical notes

### Codec reuse

`modbus_codec.go` already covers full encode/decode — the parser only needs a thin wrapper that grabs the right place in the word array per layout field:

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

Encode is symmetric — `EncodeRegisters` fills a word slice at the right position, `RegistersToBytes` builds the byte array from it.

### Bit fields

If `type=bool` AND `bit` is set, the field is interpreted as a single bit from a 16-bit register. Use case: PLC status bits ("bit 0 = motor running, bit 1 = fault, bit 7 = maintenance").

```yaml
- offset: 5, name: "motor_running", type: "bool", bit: 0
- offset: 5, name: "fault",         type: "bool", bit: 1
- offset: 5, name: "maintenance",   type: "bool", bit: 7
```

Encode: the register is assembled sparsely — all bool fields with the same offset are OR-combined in one register. Fields that are **not** in the encode input remain 0.

### Auto-detection heuristic

```go
input := msg.Get(n.inputProperty)
switch v := input.(type) {
case []any:
    // sparse: all elements numbers? → parse, otherwise error
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

`[]any` is the most common case (JSON-decoded), so checked separately.

## Out of scope

- **Generic binary parser** for other protocols (S7, OPC UA, raw TCP frames): for now Modbus-specific. If three protocols need the same table, we extract the layout editor into a `binary-parser` node — until then no.
- **Validation rules per field** (e.g. "temperature must be 0..100, otherwise error"): not in v1. Can be done downstream with Switch node or Function node.
- **Conditional fields** ("if `mode==1`, then at offset 8 `value` is an int16, otherwise float32"): not in v1. Whoever needs that parses two layouts and switches.
- **CSV import of the layout**: not in v1. Manual table is enough for the first 80% of use cases.
- **Coil layouts** (layout over `[]bool` from FC1/FC2): not in v1 — coils are rarely structured as a block, direct access to the `[]bool` array is enough.

## Dependencies

- `modbus_codec.go` — already there, reused
- Catch node integration via existing `flow.ErrorProvider` — no engine change
- No external Go dependencies
