# S7 Write (`s7-write`)

Writes variables to a SIEMENS S7 PLC over RFC1006 / ISO-on-TCP. Three
modes mirror [`s7-read`](s7-read.md): a fixed list with values from
config / messages, fully msg-driven, or raw byte block.

| Inputs | Outputs                     |
|--------|-----------------------------|
| 1      | 0 (silent), 1 (ack / passthrough) |

By default the node is a **pure sink** — no output, no ACK. Enabling
`emitAck` or `passthrough` exposes an output port for results.

## Modes

`static` (default) writes the configured list on every input message.
Per-row `valueSource` decides whether the value comes from a baked-in
constant or from a message field (`valuePath`). **This is the right mode
for the most common pattern: fixed addresses, values flowing in via
messages.**

`dynamic` ignores the sidebar list entirely — the variable list comes
**only** from the message via `msg.variables` (full form) or the
convenience triple `msg.address + msg.dataType + msg.payload` (single
write). Use this when the address itself varies per message.

`block` writes raw bytes from `msg.<inputProperty>` (default `payload`)
in one `AGWriteArea` call. Pair with [`s7-parser`](s7-parser.md) action
`encode` upstream to build the buffer from a structured object.

## Configuration

| Field         | Default  | Description |
|---------------|----------|-------------|
| `plc`         | —        | ID of the referenced [`s7-plc` config node](s7-plc.md). |
| `mode`        | `static` | `static` / `dynamic` / `block`. |
| `variables`   | `[]`     | Static-mode list, see [Variables](#variables). Hidden in dynamic mode (the runtime ignores it there). |
| `block`       | —        | Block-mode descriptor, see [Block mode](#block-mode). |
| `emitAck`     | `false`  | Emit a fresh ACK message after every write attempt. Mutually exclusive with `passthrough`. |
| `passthrough` | `false`  | Forward the input message with `msg.s7Write` enriched. Mutually exclusive with `emitAck`. |

### Variables

| Field         | Required        | Description |
|---------------|-----------------|-------------|
| `address`     | yes             | Siemens-style address — see [`s7-read` address syntax](s7-read.md#address-syntax). |
| `dataType`    | yes             | TIA-Portal type. Counter / Timer cannot be written (rejected at deploy). |
| `valueSource` | yes             | `static` (baked-in) or `msg` (read from message). |
| `value`       | when `static`   | Literal value, typed against `dataType`. |
| `valuePath`   | when `msg`      | Dotted path on the incoming message, e.g. `payload`, `payload.setpoint`. |
| `scale`       | optional        | Inverse scaling on write: `raw = (value − offset) / scale`. Default 1. |
| `offset`      | optional        | Inverse offset, applied **before** division. Default 0. |

### Block mode

| Field           | Default   | Description |
|-----------------|-----------|-------------|
| `area`          | `DB`      | `DB` / `M` / `Q`. **`I` (Inputs / PE) is not writable** — rejected at deploy. |
| `db`            | —         | DB number. Required when `area=DB`. |
| `start`         | `0`       | Start byte offset within the area. |
| `length`        | `0`       | Optional fixed length. `0` = use the incoming buffer's length. When set, the buffer length must match exactly. |
| `inputProperty` | `payload` | Message field carrying the byte slice (`[]byte`, `[]int`, or `[]any` of numerics). |

In block mode `msg.s7.area` / `msg.s7.db` / `msg.s7.start` override the
configured destination per message — useful when one writer node serves
multiple targets driven by an upstream router.

## Outgoing message (when `emitAck` or `passthrough` enabled)

```json
{
  "payload": true,
  "s7Write": {
    "plc":   "Press Line 2",
    "results": [
      { "address": "DB10.DBD0",       "dataType": "real",   "ok": true },
      { "address": "M0.0",            "dataType": "bool",   "ok": true },
      { "address": "DB1.STRING50.20", "dataType": "string", "ok": false, "error": "Address out of range" }
    ],
    "allOk": false
  }
}
```

`msg.payload` is `true` if every variable wrote successfully, `false` if
at least one failed. `msg.s7Write.results` always lists every attempted
variable in original order, with per-item `ok` and (on failure) `error`.

`passthrough=true` keeps the input message and only **adds** `msg.s7Write`
— the original `payload` and other fields survive unchanged. Mutually
exclusive with `emitAck` (which emits a fresh message with `payload=allOk`).

## Error semantics

The write surfaces three distinct failure classes:

- **Per-variable encoding errors** (out-of-range, type mismatch, missing
  value at `valuePath`): the offending row goes into `results[i].error`
  and is **excluded** from the wire request — the surviving items still
  get written. The ACK message reflects the partial success via `allOk:
  false`.
- **Per-variable PLC errors** (address out of range, DB doesn't exist):
  the wire round-trip succeeds but individual items report a Snap7 error
  code; mapped to `results[i].error` with the same partial-success
  semantics as above.
- **Whole-transaction failures** (transport timeout, lost connection):
  the function returns an error to the engine, which routes it to the
  Catch node (if any) and **does not emit an ACK**. The flow's status
  pill goes red until the next reconnect.

For "fire and forget" writes (no ACK / passthrough), per-variable
failures are silently logged at WARN level and the engine routes whole-
transaction failures to the Catch path. The status pill always reflects
the underlying PLC connection state.

## Examples

### Static write, value from message

The most common pattern — a setpoint that an upstream Function or
Inject computes:

```
mode: static
variables:
  - address:    DB100.DBW12
    dataType:   int
    valueSource: msg
    valuePath:   payload.setpoint
emitAck: true
```

Every input message writes `msg.payload.setpoint` to `DB100.DBW12` as a
16-bit signed int.

### Dynamic single write (convenience form)

```
mode: dynamic
```

Upstream sends:

```json
{ "address": "DB100.DBW12", "dataType": "int", "payload": 1500 }
```

No sidebar config needed — the address, type, and value all come from
the message.

### Block write of pre-encoded bytes

```
mode:  block
block:
  area:           DB
  db:             1
  start:          12
  length:         0          # use the incoming buffer length
  inputProperty:  payload
```

Pair with `s7-parser` (action `encode`) upstream to build the buffer
from a structured object. The block is sent in one `AGWriteArea` call
(auto-split on PDU).

## Counters and Timers

`counter` and `timer` are intentionally **read-only**. Siemens treats them
as CPU-internal state, the wire encoding is asymmetric / fragile, and
writing to them from a client is rare in industrial practice. The
encoder rejects with a clear message so misuse is obvious at deploy
rather than silent corruption.

## Status

- Green — `ready` (when the PLC is connected).
- Yellow / Red — propagated from the underlying PLC config.

## See also

- [S7 PLC config node](s7-plc.md) — connection details.
- [S7 Read](s7-read.md) — companion source node.
- [S7 Parser](s7-parser.md) — encode structured objects to bytes for block mode.
