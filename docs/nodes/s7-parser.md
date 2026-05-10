# S7 Parser (`s7-parser`)

Decodes raw S7 byte blocks into structured objects (parse direction) or
builds raw byte blocks from objects (encode direction). The layout is
configured declaratively as a list of fields and is shared by both
directions — pair the parser with [`s7-read` block mode](s7-read.md#block-mode)
upstream (parse) or [`s7-write` block mode](s7-write.md#block-mode)
downstream (encode).

| Inputs | Outputs |
|--------|---------|
| 1      | 1       |

## Why a parser node?

A typical TIA-Portal DB packs a measurement record into a tightly-laid-out
byte range:

```
DB100 (record layout)
  byte  0   REAL    setpoint
  byte  4   REAL    actual
  byte  8   INT     mode
  byte 10.0 BOOL    running
  byte 10.1 BOOL    alarm
  byte 11   BYTE    quality
```

You can read the six fields as six `s7-read` variables — bundled into one
`AGReadMulti` round-trip when they fit a PDU. Or you can read the entire
12-byte range in `block` mode and decode it client-side with this node.
The block path is the right call when:

- The record is **larger than one PDU** (auto-splits into multiple
  `AGReadArea` calls — much cheaper than 50+ individual variable reads).
- You want **symmetric read + write** of the same record (the layout is
  reusable for both directions).
- The DB is **not yours** — third-party PLCs often expose contiguous
  records that you'd rather decode in one place than scatter across N
  variables.

## Configuration

| Field          | Default   | Description |
|----------------|-----------|-------------|
| `action`       | `auto`    | `auto` (sniff input shape) / `parse` (force decode) / `encode` (force build). Auto: object input → encode; everything else → parse. |
| `parseFrom`    | `payload` | Message field carrying the byte block on parse. |
| `encodeFrom`   | `payload` | Message field carrying the structured object on encode. |
| `blockLength`  | `0`       | Fixed output buffer size in bytes (`0` = derive from layout extent). Larger than the layout pads with zeros; smaller than required is rejected at deploy. |
| `preserveBytes`| `false`   | Parse only — also forward the raw bytes as `msg.bytes` (`[]int`). |
| `layout`       | `[]`      | Ordered list of field descriptors, see [Layout](#layout). |

## Layout

Each entry describes one field in the byte block:

| Field         | Required | Description |
|---------------|----------|-------------|
| `name`        | yes      | Output key (parse) or input key (encode). Duplicates are rejected at deploy. |
| `offset`      | yes      | Buffer offset (0-based, **relative to the block, not the PLC address**). Integer for byte-aligned types; dotted `byte.bit` for `bool` (e.g. `"12.3"`). |
| `type`        | yes      | TIA type — see [`s7-read` data types](s7-read.md#data-types). Plus `raw` (byte slice with `length`). |
| `length`      | when type=`string` / `wstring` / `raw` | Max length in characters (string/wstring) or byte count (raw). |
| `signed`      | optional | For `byte` / `word` / `dword`: interpret as signed. Inherently-signed types (`int`, `dint`, `lint`) reject this flag. |
| `scale`       | optional | Multiplicative scale: `value = raw × scale + offset`. Default 1. Skipped for non-numeric types. |
| `valueOffset` | optional | Additive offset, applied **after** scaling. Default 0. |
| `unit`        | optional | Display-only label shown in the layout editor. Not propagated to the message. |

Offsets are **buffer-relative**: a layout starting at offset 0 is portable
across DBs. The downstream `s7-write` `block.start` positions the buffer at
the right PLC byte. (The parser deliberately does **not** auto-set
`msg.s7.start` — that would silently break the invariant that encode and
parse use the same offset semantics.)

## Examples

### Decode a sensor block from `s7-read`

Layout for the record above:

```yaml
- { name: setpoint, offset: 0,    type: real }
- { name: actual,   offset: 4,    type: real }
- { name: mode,     offset: 8,    type: int  }
- { name: running,  offset: 10.0, type: bool }
- { name: alarm,    offset: 10.1, type: bool }
- { name: quality,  offset: 11,   type: byte }
```

Pipe `s7-read` (block mode, DB100, length 12) into the parser. Output:

```json
{
  "payload": {
    "setpoint": 70.0,
    "actual":   68.5,
    "mode":     1,
    "running":  true,
    "alarm":    false,
    "quality":  255
  }
}
```

### Build a write buffer for `s7-write` block mode

Same layout, `action=encode`. Upstream Function emits:

```json
{ "payload": { "setpoint": 75.0, "mode": 2, "running": true } }
```

The parser emits `msg.payload = []int` (the wire bytes), with missing
keys (`actual`, `alarm`, `quality`) left at zero. Pipe into `s7-write`
(block mode, DB100, start 0). Sparse-zero semantics let you author one
layout that handles partial updates without resetting unmentioned fields
— provided your downstream tolerates the zero-fill.

### Pack a BOOL status word

Multiple BOOLs sharing one host byte are OR-aggregated on encode, so a
status word is straightforward:

```yaml
- { name: alarm,   offset: "0.0", type: bool }
- { name: warn,    offset: "0.1", type: bool }
- { name: running, offset: "0.7", type: bool }
```

Input `{ alarm: true, running: true }` produces `0x81` (bits 0 and 7
set). Missing keys default to false, matching sparse-zero behaviour for
non-bool fields.

## Auto mode

`action=auto` dispatches based on input shape:

- `msg.payload` is a map / object → **encode**
- `msg.payload` is a byte array (or array of numbers) → **parse**

Auto is convenient when one parser node serves both directions of a
single layout. Use `parse` or `encode` to force one path when the input
shape is ambiguous (e.g. a JSON-decoded byte array that happens to look
like an object key-value pair to an observer).

## Output shape

### Parse direction

```json
{
  "payload": { "setpoint": 70.0, "actual": 68.5, "mode": 1, "running": true, ... },
  "bytes":   [66, 140, 0, 0, 66, 137, 0, 0, 0, 1, 0x80, 255]
}
```

`msg.bytes` is only added when `preserveBytes=true`.

### Encode direction

```json
{
  "payload": [66, 140, 0, 0, 66, 137, 0, 0, 0, 1, 0x80, 255],
  "bytes":   [66, 140, 0, 0, 66, 137, 0, 0, 0, 1, 0x80, 255]
}
```

`msg.payload` is `[]int` (decimal numbers in JSON) so the Debug panel
renders them legibly. `msg.bytes` mirrors the same value for symmetry
with parse direction.

## Notes

- **Big-endian** throughout — the canonical S7 wire order regardless of
  CPU family.
- **No timezone metadata** on date/time wire types. `dt` / `ldt` / `dtl`
  decode to RFC3339Nano UTC strings; the wall-clock vs. UTC distinction
  is documented per type in the [data types reference](s7-read.md#data-types).
- **Overlapping fields** produce a `slog.Warn` at deploy (BOOLs sharing
  one host byte are excluded — that's the canonical Siemens status-word
  pattern).

## See also

- [S7 Read](s7-read.md) — produce the byte block in `block` mode.
- [S7 Write](s7-write.md) — consume the encoded byte block in `block` mode.
- [S7 PLC config node](s7-plc.md) — connection details.
