# Issue: S7 Parser Node — declaratively parse & encode SIEMENS S7 byte layouts

## Status: Open

## Problem description

With `s7-read` in `block` mode (see [`NODE_S7.md`](./NODE_S7.md)), users today
fetch raw byte ranges from a DB / Merker / I / Q area and then have to dissect
them in the **Function node with the buffer API**. That works, but per PLC
project leads to two-digit lines of boilerplate code for what is essentially
purely declarative mapping "offset → S7 type → field name".

Industrial Siemens projects describe their DB layout in TIA Portal exactly as a
table:

| Offset | Name        | Type   | Scale | Unit |
| ------ | ----------- | ------ | ----- | ---- |
| 0      | temperature | REAL   | 1     | °C   |
| 4      | pressure    | REAL   | 0.1   | bar  |
| 8      | counter     | DINT   | 1     | —    |
| 12.0   | alarm       | BOOL   | —     | —    |
| 12.1   | running     | BOOL   | —     | —    |
| 14     | mode        | INT    | 1     | —    |
| 50     | tag         | STRING(20) | — | —    |

This very table should be the **S7 Parser node** — one configuration per device
or DB layout, usable in both directions (parse on read, encode on write).

## Perspective / rationale

- **Operator UX before designer power**: a table with add/delete/reorder is more
  accessible than JS code. TIA Portal screenshot → layout → done.
- **Counterpart to the JSON Parser and Modbus Parser**: same action model
  (`auto`/`parse`/`encode`), same error behavior, same status binding. Sits
  together with the other parser nodes in the palette.
- **DRY**: one layout, both directions. An `s7-read block` and an
  `s7-write block` against the same DB share the same schema definition.
- **No s7-read/s7-write change required**: the parser works purely on
  `msg.payload` (`[]byte`) — sits between `s7-read block` and the consumer
  (parse) or between producer and `s7-write block` (encode).
- **Counterpart to `PARSER_MODBUS_NODE.md`**: deliberate design parity. Anyone
  who can use the Modbus Parser can use the S7 Parser within minutes; the only
  things that change are the data type set (S7 native types) and the byte/word
  ordering (S7 is fixed big-endian on the wire, no four-combination order
  matrix).

## Overview

| Node          | Type ID      | Canvas inputs | Canvas outputs | Description |
|---|---|---|---|---|
| **S7 Parser** | `s7-parser`  | 1             | 1              | Parses S7 byte blocks per layout (parse) or assembles them from a structured payload (encode) |

```
[s7-read  block]   ──→  [s7-parser parse]   ──→  [Switch / Function / Debug …]
                                  ↑
                             Layout: 7 fields

[Inject struct]    ──→  [s7-parser encode] ──→  [s7-write block]
                                  ↑
                          (same layout)
```

## Requirements

### 1. Action model (analogous to other parser nodes)

| Action   | Behavior |
|---|---|
| `auto`   | Heuristic: input is `[]byte`/`[]int` → **parse**; input is object/map → **encode** |
| `parse`  | Forces parse — error if input is not a byte buffer |
| `encode` | Forces encode — error if input is not an object/map |

`auto` is the default and covers 90% of cases — the layout is fixed, direction
follows from the dataflow.

### 2. Input source

Configurable, which `msg` field carries the raw material:

- **Parse path**: default `msg.payload` (matches `s7-read` block-mode output
  directly). Alternatives: `msg.bytes`, or a freely configurable path
  (`msg.s7.raw`, …).
- **Encode path**: default `msg.payload` (object). Freely configurable.

### 3. Output

- **Parse**: produces `msg.payload` as a `map[string]any` with the layout field
  names as keys. Unused `msg` fields stay unchanged. Optionally the original
  raw bytes are preserved as `msg.bytes` (configurable, default `false` —
  keeps the message lean).
- **Encode**: produces `msg.payload` as `[]byte` (wire-ready). Additionally
  `msg.bytes` mirrors `msg.payload` so downstream nodes can pick whichever
  field convention they expect. Optionally `msg.s7.start` is set to the lowest
  layout offset so a downstream `s7-write block` with `inputProperty=payload`
  picks up the start address without an explicit override.

### 4. Layout definition

An array of field definitions. Each entry mirrors a TIA Portal DB row:

```json
{
  "layout": [
    { "offset": 0,     "name": "temperature", "type": "real",   "scale": 1,    "unit": "°C" },
    { "offset": 4,     "name": "pressure",    "type": "real",   "scale": 0.1,  "unit": "bar" },
    { "offset": 8,     "name": "counter",     "type": "dint" },
    { "offset": "12.0","name": "alarm",       "type": "bool" },
    { "offset": "12.1","name": "running",     "type": "bool" },
    { "offset": 14,    "name": "mode",        "type": "int" },
    { "offset": 16,    "name": "energy",      "type": "real",   "scale": 0.001,"unit": "kWh" },
    { "offset": 50,    "name": "tag",         "type": "string", "length": 20 }
  ]
}
```

Per field:

| Field       | Required           | Type   | Description |
| ----------- | ------------------ | ------ | ----------- |
| `offset`    | ✓                  | number / string | 0-based byte offset within the input block. For `bool`: dotted notation `"<byte>.<bit>"` (e.g. `"12.3"`) — Siemens convention. The string form is parsed into `byte=12, bit=3`; the int form (`12`) implies `bit=0` |
| `name`      | ✓                  | string | Key in the output object |
| `type`      | ✓                  | enum   | `bool`, `byte`, `word`, `dword`, `int`, `dint`, `real`, `string`, `char`, `raw`. See [S7 type table](#s7-type-table) below |
| `length`    | for `string` / `raw` / `char[]` | number | For `string`: maximum-length characters (the on-wire span is `length + 2` bytes — see [S7 STRING encoding](#s7-string-encoding)). For `raw`: byte count to read out as `[]byte`. For `char` arrays: char count |
| `scale`     | optional           | number | Multiplicative factor; applied on parse, inverted on encode. Default `1` |
| `valueOffset` | optional         | number | Additive offset; applied **after** scaling on parse, inverted on encode. Default `0`. UI label "Offset (value)" — separated from the field's positional `offset` to avoid collision |
| `unit`      | optional           | string | Pure display (in properties panel & debug hint), not carried in the wire format |
| `signed`    | optional, only for `byte`/`word`/`dword` | boolean | If `true`, parse as `int8`/`int16`/`int32` even though the type name suggests unsigned. Convenience for users who need a signed integer at a byte boundary without picking the explicit `int`/`dint` types |

**Offsets are 0-based on the input block**, not on the absolute PLC address.
A user reading `DB1` bytes 100..200 still defines their fields with
`offset: 0..n` — that makes the layout portable across different DBs and
servers.

#### S7 type table

| Type   | Wire representation                                        | Bytes consumed              | JSON output |
|--------|------------------------------------------------------------|-----------------------------|-------------|
| `bool` | 1 bit at `(offset.byte, offset.bit)`                       | 0 (sub-byte; doesn't advance the cursor) | `true` / `false` |
| `byte` | 1 byte, unsigned (or signed when `signed=true`)            | 1                           | `0..255` (or `-128..127`) |
| `word` | 2 bytes big-endian, unsigned (or signed when `signed=true`)| 2                           | `0..65535` |
| `dword`| 4 bytes big-endian, unsigned (or signed when `signed=true`)| 4                           | `0..4294967295` |
| `int`  | 2 bytes big-endian, signed (two's complement)              | 2                           | `-32768..32767` |
| `dint` | 4 bytes big-endian, signed (two's complement)              | 4                           | `-2147483648..2147483647` |
| `real` | 4 bytes IEEE 754 single-precision big-endian               | 4                           | float |
| `string` | S7 STRING: `[maxLen][actLen][char × maxLen]`             | `length + 2`                | string (trimmed to `actLen`) |
| `char` | 1 byte ASCII (use `length` for char arrays, then JSON string) | `1` or `length`           | string (1 char or `length` chars) |
| `raw`  | `length` bytes opaque                                      | `length`                    | `[]byte` array of ints `0..255` |

**Endianness**: S7 is fixed **big-endian** on the wire — there is no per-field
byte order configuration (unlike Modbus, which defines four combinations). Any
swap is an application-level oddity that belongs in a Function node.

### 5. Node-level defaults

Configurable per node (apply as defaults for all fields that do not explicitly
override them):

- `blockLength` — the expected size of the incoming block (parse) or the size of
  the output block (encode). On parse, declaring this lets the parser refuse
  short / over-long inputs early instead of silently slicing. On encode, the
  output `[]byte` is allocated to exactly this size; fields beyond the highest
  declared offset are zero-padded. If left unset, the parser derives it from
  `max(field.offset + field.size)` of the layout
- `preserveBytes` (parse only, default `false`) — when `true`, the original raw
  bytes are forwarded as `msg.bytes` alongside the parsed object. Useful for
  audit / replay / round-trip-test flows
- `setStartAddress` (encode only, default `true`) — when `true`, sets
  `msg.s7.start` to the lowest field offset, so a downstream `s7-write block`
  with input property `payload` picks up the correct start address automatically

### 6. Validation

On init **and** on parse/encode:

- Layout must have at least 1 field
- Per field, `offset`, `name`, `type` must be set
- Field names must be unique (otherwise init error)
- Fields **may** overlap (warning on init, no error — deliberate multiple
  interpretation of the same bytes, e.g. one `dword` and 32 individual `bool`
  bits at the same offset, is allowed)
- For `string` / `raw` / `char` arrays: `length >= 1`, and `length <= 254` for
  `string` (S7 STRING max length)
- For `bool`: `bit ∈ [0, 7]` (rejects Modbus-style `bit ∈ [0, 15]` syntax with
  a clear hint pointing to the dotted-byte notation)
- The highest `offset + size` must fit into `blockLength` (when set)

On parse: is the input byte buffer at least `max(offset + size)` bytes long?
Otherwise error in catch path and no output. Trailing bytes past the highest
declared offset are silently ignored (deliberate — DBs commonly contain padding
or fields the user has not yet mapped).

### 7. Action dispatch

Pseudocode for `auto`:

```go
input := msg.Get(node.inputProperty) // default "payload"
switch v := input.(type) {
case []byte:
    return n.parse(v)
case []int, []any: // JSON-decoded byte arrays
    return n.parse(toByteSlice(v))
case map[string]any:
    return n.encode(v)
case nil:
    return errCatch("input %q missing", node.inputProperty)
default:
    return errCatch("unknown input type %T", v)
}
```

`[]any` is the most common case (JSON-decoded), so checked separately and
coerced into `[]byte` before parsing.

### 8. Error behavior

- Layout init error (duplicate names, missing type, invalid offset syntax):
  node does not start, status red.
- Parse/encode error at runtime: routed via `flow.ErrorProvider` to the catch
  path. No output on port 0.
- `type=string` with bytes outside ASCII/UTF-8: returns the bytes as a string
  anyway (with trailing-NUL trim past `actLen`), no error — analogous to the
  codec behavior in `s7_codec.go`. A warning is logged once per node lifecycle.
- `type=bool` with bit outside `[0, 7]`: hard init error, refuses to start.

### 9. Properties panel

Layout editor as a table, comparable to the Modbus Parser editor:

```
┌──────────────────────────────────────────────────────────────────┐
│  S7 Parser                                                        │
├──────────────────────────────────────────────────────────────────┤
│                                                                   │
│  Action                                                           │
│  ( • ) Auto    ( ) Parse only    ( ) Encode only                  │
│                                                                   │
│  Input Property                                                   │
│  ┌──────────────────────┐                                        │
│  │ msg.payload          │ ← default; switchable to msg.bytes     │
│  └──────────────────────┘                                        │
│                                                                   │
│  Block Length           [ auto ]   ← derive from layout, or set  │
│  ☐ Preserve raw bytes (parse → msg.bytes)                         │
│  ☑ Set msg.s7.start on encode                                     │
│                                                                   │
│  Layout                                                           │
│  ┌────┬────────┬────────────┬─────────┬──────┬───────┬────────┬─┐│
│  │ ↕  │ Offset │ Name       │ Type    │ Len  │ Scale │ Unit   │×││
│  ├────┼────────┼────────────┼─────────┼──────┼───────┼────────┼─┤│
│  │ ⋮  │   0    │ temperature│ real    │  —   │  1    │ °C     │×││
│  │ ⋮  │   4    │ pressure   │ real    │  —   │  0.1  │ bar    │×││
│  │ ⋮  │   8    │ counter    │ dint    │  —   │  1    │        │×││
│  │ ⋮  │  12.0  │ alarm      │ bool    │  —   │  —    │        │×││
│  │ ⋮  │  12.1  │ running    │ bool    │  —   │  —    │        │×││
│  │ ⋮  │  14    │ mode       │ int     │  —   │  1    │        │×││
│  │ ⋮  │  50    │ tag        │ string  │  20  │  —    │        │×││
│  └────┴────────┴────────────┴─────────┴──────┴───────┴────────┴─┘│
│  [+ Add field]                                                   │
│                                                                   │
│  ▸ Per-field override (collapses out): scale/valueOffset, signed  │
│                                                                   │
└──────────────────────────────────────────────────────────────────┘
```

The offset cell accepts both `12` (= `12.0`, byte address) and `12.3` (BOOL bit
3 of byte 12). The UI shows a small inline hint for BOOL rows that the dotted
form is required.

Per-field override (`scale`, `valueOffset`, `signed`) is collapsible — rarely
needed for non-numeric types, should not clutter the default table.

## Data structure

### workspace.json

```json
{
  "id": "node-s7-parser-1",
  "type": "s7-parser",
  "name": "Press Line 2 — DB1 layout",
  "x": 600, "y": 200, "z": "flow-1",
  "inputs": 1, "outputs": 1,
  "wires": [["..."]],
  "config": {
    "action": "auto",
    "inputProperty": "payload",
    "blockLength": 200,
    "preserveBytes": false,
    "setStartAddress": true,
    "layout": [
      { "offset": 0,     "name": "temperature", "type": "real" },
      { "offset": 4,     "name": "pressure",    "type": "real",   "scale": 0.1 },
      { "offset": 8,     "name": "counter",     "type": "dint" },
      { "offset": "12.0","name": "alarm",       "type": "bool" },
      { "offset": "12.1","name": "running",     "type": "bool" },
      { "offset": 14,    "name": "mode",        "type": "int" },
      { "offset": 16,    "name": "energy",      "type": "real",   "scale": 0.001 },
      { "offset": 50,    "name": "tag",         "type": "string", "length": 20 }
    ]
  }
}
```

## Example

### Parse: demo PLC `DB1` block (200 B) → object

**S7 Read** in block mode (area=DB, db=1, start=0, length=200) delivers
`msg.payload = []byte` of 200 bytes.

**S7 Parser** (action=auto, layout from the workspace.json above) turns it
into:

```json
{
  "payload": {
    "temperature": 21.487,
    "pressure": 0.102,
    "counter": 1284,
    "alarm": false,
    "running": true,
    "mode": 1,
    "energy": 0.000208,
    "tag": "LOOPZE-S7-DEMO"
  },
  "s7": {
    "plc": "Press Line 2",
    "area": "DB", "db": 1, "start": 0, "length": 200
  }
}
```

The original `msg.s7` metadata from the read flows through unchanged — the
parser only mutates `msg.payload` (and optionally `msg.bytes` if
`preserveBytes=true`).

### Encode: object back to byte block

**Inject** with `msg.payload = { setpoint: 250, mode: 2, temperature: 22.5 }`,
**S7 Parser** (action=encode, layout where `setpoint` sits at offset 12,
`mode` at 14, `temperature` at 0) delivers:

```json
{
  "payload": [/* 200 bytes, all zero except positions 0..3, 12..13, 14..15 */],
  "bytes":   [/* same as payload */],
  "s7": { "start": 0 }
}
```

→ directly attachable to **S7 Write** block mode (area=DB, db=1, start=`msg.s7.start` or fixed 0).

**Sparse-encoding note**: on encode, the block is sized to `blockLength` (or
to `max(offset+size)` if not set). Unset fields in the input object → byte 0.
Anyone who needs the previous read as the basis ("read-modify-write") reads
first, parses, merges in the Function node, and then encodes the complete
object. This is a deliberately conservative default — silently merging the
prior value of *unmentioned* fields would be surprising and a foot-gun in
multi-writer scenarios.

## Affected files

### Backend – new files

- `internal/nodes/s7_parser.go` — parser node implementation. Reuse of
  `s7_codec.go` (encode/decode primitives for BOOL, BYTE, WORD, DWORD, INT,
  DINT, REAL, STRING, CHAR) introduced by `NODE_S7.md`
- `internal/nodes/s7_parser_test.go` — layout validation, parse round-trip,
  encode round-trip, BOOL bit extraction at all 8 positions, S7 STRING
  round-trip, sparse-encode behavior, oversized-input handling, blockLength
  derivation

### Backend – changes

- `internal/server/server.go` — registration:
  `registry.Register("s7-parser", nodes.NewS7ParserNode, nodes.S7ParserTypeInfo())`

### Frontend – new files

- `frontend/src/components/config/S7ParserConfig.vue` — properties panel
  including the layout table
- `frontend/src/components/config/S7ParserLayoutEditor.vue` — table editor
  (reorder, add/delete, per-field override expand). If the same pattern recurs
  for further binary protocols, extract into a shared editor — until then keep
  it S7-specific so the type dropdown can use Siemens vocabulary

### Frontend – changes

- `frontend/src/components/PropertyPanel.vue` — dispatch for `s7-parser`
- `frontend/src/components/nodes/tokens.ts` — `s7-parser` maps to the parser
  palette (analogous to `modbus-parser`, `json-parser`)
- `frontend/src/views/FlowEditor.vue` — template `#node-s7-parser`
- `frontend/src/components/nodes/BaseNode.vue` — `typeLabel` map:
  `s7-parser` → "S7 Parser"
- `frontend/src/types/flow.ts` — TypeScript types for `S7ParserConfig`,
  `S7ParserField`

## Technical notes

### Codec reuse

`s7_codec.go` (introduced by `NODE_S7.md`) already covers full encode/decode
for all S7 native types. The parser only needs a thin wrapper that grabs the
right slice from the byte buffer per layout field:

```go
func (n *S7ParserNode) parse(buf []byte) (map[string]any, error) {
    out := make(map[string]any, len(n.layout))
    for _, f := range n.layout {
        size := f.WireSize() // 1, 2, 4, length, length+2, …
        if f.Type != "bool" && f.Byte+size > len(buf) {
            return nil, fmt.Errorf("field %q: offset %d + %d > input %d",
                f.Name, f.Byte, size, len(buf))
        }

        var v any
        var err error
        switch f.Type {
        case "bool":
            if f.Byte >= len(buf) {
                return nil, fmt.Errorf("field %q: byte %d > input %d",
                    f.Name, f.Byte, len(buf))
            }
            v = (buf[f.Byte]>>f.Bit)&1 == 1
        case "byte", "word", "dword", "int", "dint", "real":
            v, err = DecodeS7Scalar(f.Type, f.Signed, buf[f.Byte:f.Byte+size])
        case "string":
            v, err = DecodeS7String(buf[f.Byte:f.Byte+size]) // size = length+2
        case "char":
            if f.Length <= 1 {
                v = string(buf[f.Byte])
            } else {
                v = string(buf[f.Byte : f.Byte+f.Length])
            }
        case "raw":
            v = append([]byte(nil), buf[f.Byte:f.Byte+f.Length]...)
        }
        if err != nil { return nil, fmt.Errorf("field %q: %w", f.Name, err) }

        v = ApplyScale(v, f.Scale, f.ValueOffset)
        out[f.Name] = v
    }
    return out, nil
}
```

Encode is symmetric — `EncodeS7Scalar` writes into a pre-allocated `[]byte` at
the right offset; multiple BOOL fields at the same byte are OR-combined into a
single byte.

### BOOL bit packing on encode

Multiple BOOL fields at the same byte offset (typical for Siemens status words
— e.g. `byte 12: alarm, running, fault, maintenance, …`) are merged on encode:

```yaml
- offset: "12.0", name: alarm,       type: bool
- offset: "12.1", name: running,     type: bool
- offset: "12.7", name: maintenance, type: bool
```

On encode with `{ alarm: true, running: true, maintenance: false }` → byte 12
becomes `0b00000011 = 0x03`. Fields **not** present in the encode input are
treated as `false` (= bit cleared) — same sparse semantics as the rest of the
parser.

### S7 STRING encoding

S7 STRING has a two-byte header followed by `maxLen` bytes of character space:

```
+--------+---------+----------+----------+ ... +
| maxLen | actLen  | char[0]  | char[1]  |     |
+--------+---------+----------+----------+ ... +
                   <-- actLen bytes used -->
                   <----- maxLen total ----->
```

- On parse: read `maxLen` from byte 0, `actLen` from byte 1, take chars
  `[2..2+actLen]`. Return as Go string. If `actLen > maxLen` (corrupt PLC
  state), clamp to `maxLen` and log a warning once
- On encode: set `maxLen` from the layout (`length` field), set `actLen` from
  the actual provided string length, write chars, zero-pad up to `maxLen`. If
  the input string is longer than `length`, truncate and log a warning once
- Total wire footprint = `length + 2` bytes — the layout offset cell shows
  this in a tooltip ("STRING(20) — 22 bytes total")

This is the most common encoding mistake in S7 client libraries — dedicated
codec functions handle it transparently; the user only deals with `string`
values.

### Auto-detection heuristic

```go
input := msg.Get(n.inputProperty)
switch v := input.(type) {
case []byte:
    return n.parse(v)
case []any:
    // JSON-decoded byte array: all elements are float64 in [0, 255]
    bytes, ok := toBytesFromAny(v)
    if !ok {
        return errCatch("input %q is array but not all bytes", n.inputProperty)
    }
    return n.parse(bytes)
case []int:
    return n.parse(toBytesFromInts(v))
case map[string]any:
    return n.encode(v)
case nil:
    return errCatch("input %q missing", n.inputProperty)
default:
    return errCatch("unknown input type %T", v)
}
```

`[]any` is the most common case in flows that have crossed a JSON-decode
boundary, so checked separately.

### Why not auto-detect S7 STRING from the byte stream?

In principle `actLen <= maxLen <= 254` could be a heuristic for a STRING field.
We deliberately don't do this — silently re-typing a field based on byte
content is a foot-gun. Layouts are static; the user declares STRING explicitly.

## Out of scope

- **CSV / TIA Portal export import**: not in v1. Would be high-value (one-click
  layout import from the engineering tool), but parsing the TIA XML / SCL is a
  separate feature with its own surface. Manual table is enough for the first
  80% of use cases — a 7-field DB takes about 30 seconds to type
- **Validation rules per field** (e.g. "temperature must be 0..100, otherwise
  error"): not in v1. Can be done downstream with Switch node or Function node
- **Conditional fields** ("if `mode==1`, then at offset 8 `value` is an INT,
  otherwise REAL"): not in v1. Whoever needs that defines two layouts and
  switches between them
- **S5TIME / DATE_AND_TIME / TOD / DATE / TIME types**: defer — these legacy
  S7-300/400 types are rarely used in modern projects (TIA Portal favors
  `LDT`, `LTIME`, etc.). Add as a v1.x addendum once we have customer demand
- **`LREAL` (8-byte double-precision float)**: 1500-only; defer. Same shape as
  `REAL` with double the byte count, but the gos7 codec doesn't expose it
  natively — would need a wrapper. Add when a customer asks
- **Optimized DB symbol resolution** (read field names from the PLC instead of
  declaring them locally): not in scope — symbols only exist for Optimized DBs,
  which the S7 protocol can't read in the first place. Use OPC UA for that
  workflow
- **`ANY` pointer / `STRUCT` / nested arrays of structs**: not in v1. STRUCT
  layouts can be flattened into the field list manually; nested structs are
  rare in the kind of DBs that get exposed for external read

## Dependencies

- **`NODE_S7.md`** — must be implemented (or implemented in parallel). Provides
  the `block` mode in `s7-read` and `s7-write` that this parser plugs between.
  Specifically, the codec primitives in `internal/nodes/s7_codec.go` (ENCODE /
  DECODE for all S7 native types) are introduced there and reused here.
- **Catch node** integration via existing `flow.ErrorProvider` — no engine change
- **`PARSER_MODBUS_NODE.md`** — design parity reference. The frontend table
  editor pattern, action model, and out-of-scope list mirror that issue
  closely; an implementer should read both side by side
- No external Go dependencies
