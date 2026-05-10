# S7 Read (`s7-read`)

Reads variables from a SIEMENS S7 PLC over RFC1006 / ISO-on-TCP. Three
modes cover the typical industrial polling patterns:

| Inputs                                            | Outputs |
|---------------------------------------------------|---------|
| 0 (static), 1 (dynamic), 0 or 1 (block)           | 1       |

## Modes

`static` (default) cyclically polls a fixed list of variables at
`pollInterval`. The most common pattern for dashboards and trending.

`dynamic` triggers one read per incoming message. The variable list comes
from the message (`msg.variables` or the `msg.address+dataType` convenience
form); when the message carries no override, the configured sidebar list
acts as a **fallback**, so you can use `dynamic` as "static-on-trigger" or
fully msg-driven.

`block` reads a contiguous byte range from one area in raw form. Pipe the
result into [`s7-parser`](s7-parser.md) to decode multiple fields at once
— typical when a single DB carries a tightly-packed measurement record.
With `triggerOnInput=true` the cyclic poll is **additive** to on-demand
reads (poll continues; messages force an extra read between ticks).

## Configuration

| Field           | Default   | Description |
|-----------------|-----------|-------------|
| `plc`           | —         | ID of the referenced [`s7-plc` config node](s7-plc.md). |
| `mode`          | `static`  | `static` / `dynamic` / `block`. |
| `variables`     | `[]`      | Variables list, see [Variables](#variables). |
| `block`         | —         | Block-mode descriptor, see [Block mode](#block-mode). |
| `outputShape`   | auto      | Variables-mode output: `single` (1 var) / `array` / `object`. Defaults to `single` for 1 var, `object` for 2+. |
| `pollInterval`  | `1000`    | Static / block mode polling period in **milliseconds**. |
| `emitOnChange`  | `false`   | Suppress duplicate emits when the read result is unchanged. |
| `emitOnError`   | `false`   | Emit a separate error message on read failure (in addition to routing through the Catch node). |
| `topicTemplate` | derived   | Optional topic format. Placeholders: `<plc-name>`, `<address>`, `<name>`, `<area>`, `<db>`, `<start>`, `<length>`. |

### Variables

| Field      | Required | Description |
|------------|----------|-------------|
| `name`     | optional | Display name. Defaults to the address. Used as the key in `outputShape=object`. |
| `address`  | yes      | Siemens-style address, see [Address syntax](#address-syntax). |
| `dataType` | yes      | TIA-Portal type, see [Data types](#data-types). |
| `scale`    | optional | Multiplicative scale: `value = raw × scale + offset`. Default 1. |
| `offset`   | optional | Additive offset, applied **after** scaling. Default 0. |

Scaling is skipped automatically for non-numeric types (string, wstring,
raw, date, dt, ldt, dtl, wchar, counter, timer).

### Block mode

| Field            | Default | Description |
|------------------|---------|-------------|
| `area`           | `DB`    | `DB` / `M` / `I` / `Q`. |
| `db`             | —       | DB number. Required when `area=DB`. |
| `start`          | `0`     | Start byte offset within the area. |
| `length`         | —       | Byte count to read. Hard upper bound is the negotiated PDU minus 22 bytes header (≈ 460 B at PDU 480). Larger blocks are auto-split into multiple `AGReadArea` calls and concatenated. |
| `triggerOnInput` | `false` | Adds an input port that fires an extra read on every message (additive on top of the cyclic poll). Override `area` / `db` / `start` / `length` per message via `msg.s7.*`. |

## Address syntax

S7 addresses follow the Siemens engineering tool notation. The full table
lives in [`NODE_S7.md`](https://github.com/loopzedev/loopze-edge/blob/main/specifications/issues/NODE_S7.md#address-syntax);
the practical summary:

| Form                       | Example              | Compatible `dataType` |
|----------------------------|----------------------|------------------------|
| `DB<n>.DBX<byte>.<bit>`    | `DB10.DBX2.3`        | `bool` |
| `DB<n>.DBB<byte>`          | `DB10.DBB4`          | `byte` / `char` / `sint` / `usint` |
| `DB<n>.DBW<byte>`          | `DB10.DBW6`          | `word` / `int` / `uint` / `wchar` / `date` |
| `DB<n>.DBD<byte>`          | `DB10.DBD0`          | `dword` / `dint` / `udint` / `real` / `time` / `tod` |
| `DB<n>.DBL<byte>`          | `DB10.DBL16`         | `lreal` / `lint` / `ulint` / `lword` / `ltime` / `ltod` / `ldt` / `dt` (8 B) |
| `DB<n>.DTL<byte>`          | `DB3.DTL171`         | `dtl` (12 B) |
| `DB<n>.STRING<byte>.<max>` | `DB1.STRING50.20`    | `string` |
| `DB<n>.WSTRING<byte>.<max>`| `DB1.WSTRING50.20`   | `wstring` |
| `M<byte>.<bit>`, `MB`/`MW`/`MD`     | `M0.3`, `MD16` | bit / byte / word / dword |
| `I<byte>.<bit>`, `IB`/`IW`/`ID`     | `I0.0`, `IB1`  | bit / byte / word / dword |
| `Q<byte>.<bit>`, `QB`/`QW`/`QD`     | `Q0.0`, `QB1`  | bit / byte / word / dword |
| `C<n>` / `T<n>`            | `C5`, `T3`           | `int` / `counter` / `timer` |

`DBL` and `DTL` are LOOPZE coinages — Siemens addresses 8- and 12-byte
types symbolically in the engineering tool, so there is no canonical wire
form for them. The names follow the existing `DBB` / `DBW` / `DBD` pattern.

**Optimized DBs** are not reachable via S7 — see the [`s7-plc`](s7-plc.md#optimized-dbs-tia-portal)
note. The parser rejects symbolic addresses with a hint pointing at OPC UA.

## Data types

The codec covers the full SIEMENS TIA-Portal type system. The complete
reference table with per-CPU support is in
[`NODE_S7.md`](https://github.com/loopzedev/loopze-edge/blob/main/specifications/issues/NODE_S7.md#data-types);
the headline:

- **Bitfields**: `bool`, `byte`, `word`, `dword`, `lword`
- **Integers (signed)**: `sint`, `int`, `dint`, `lint`
- **Integers (unsigned)**: `usint`, `uint`, `udint`, `ulint`
- **Floats**: `real`, `lreal`
- **Durations** (decode → integer ms / ns): `timer` (S5TIME), `time`, `ltime`, `tod`, `ltod`
- **Date / DateTime** (decode → ISO date or RFC3339Nano string): `date`, `dt`, `ldt`, `dtl`
- **Characters / strings**: `char`, `wchar`, `string`, `wstring`
- **Other**: `counter` (BCD; read-only), `raw` (byte block in the parser)

Numerical durations decode to plain integers (ms or ns) because downstream
JSON consumers prefer numbers over stringified durations. Date / DT / LDT /
DTL decode to RFC3339Nano UTC strings so timestamps round-trip cleanly
through JSON.

## Outgoing message

### Variables modes (`outputShape=object`, default for ≥ 2 vars)

```json
{
  "topic": "s7/press-line-2",
  "payload": {
    "BoilerTemp":   84.7,
    "PressureBar":   5.2,
    "AlarmActive": false
  },
  "s7": {
    "plc": "Press Line 2",
    "variables": [
      { "name": "BoilerTemp",   "address": "DB10.DBD0", "dataType": "real", "value": 84.7 },
      { "name": "PressureBar",  "address": "DB10.DBD4", "dataType": "real", "value":  5.2 },
      { "name": "AlarmActive",  "address": "M0.0",      "dataType": "bool", "value": false }
    ]
  }
}
```

`outputShape=single` puts the bare value in `msg.payload` (only legal with
exactly 1 variable). `outputShape=array` emits the full per-variable
list as the payload. The `msg.s7` metadata is identical across shapes.

### Block mode

```json
{
  "topic": "s7/press-line-2/db3",
  "payload": [129, 66, 214, 200, 202, 254, 207, 199, ...],
  "s7": {
    "plc":    "Press Line 2",
    "area":   "DB",
    "db":     3,
    "start":  0,
    "length": 256
  }
}
```

`msg.payload` is `[]int` (not `[]byte`) so the Debug panel and any JSON
consumer downstream see the bytes as decimal numbers rather than a
base64-encoded string. [`s7-parser`](s7-parser.md) accepts both shapes.

## Multi-variable reads

When several variables fit one PDU, the manager bundles them into a single
`AGReadMulti` round-trip. For PDU 480 (S7-1500 default) that is roughly 460
bytes of payload — comfortably enough for 20+ scalars. Bundling is automatic;
authoring N variables in the sidebar list is the same wire cost as authoring
the equivalent block read for that case.

For very large reads (> PDU), use **block mode** — the manager auto-splits
the area into multiple `AGReadArea` calls and concatenates the results
before emitting.

## Status

- Green — `connected · <interval>` (static) / `connected · idle` (dynamic).
- Yellow — `connecting...` / `reconnecting...` from the PLC config.
- Red — `disconnected` plus the underlying transport error.

## See also

- [S7 PLC config node](s7-plc.md) — connection details.
- [S7 Write](s7-write.md) — companion sink node.
- [S7 Parser](s7-parser.md) — decode block-mode reads into structured objects.
