# Issue: SIEMENS S7 Nodes – Read & Write with PLC Configuration

## Status: Open

## Problem Description

After Modbus, the next industrial connector follows: **SIEMENS S7**. The S7 family (S7-300, S7-400, S7-1200, S7-1500, LOGO!, S7-200 Smart) is the dominant PLC line in European automation and large parts of Asia. Many plants use S7 directly via the proprietary RFC1006 / ISO-on-TCP protocol — without the OPC UA detour — because S7 is faster, configuration-free on the PLC side (no UA server license, no namespace mapping), and exposes the original symbol/address world of the engineering tool 1:1.

Two new node types (`s7-read` and `s7-write`) enable reading from and writing to S7 PLCs. Analogous to the Modbus pattern, an **S7 PLC config node** (`s7-plc`) introduces the connection as a reusable entity — multiple nodes can reference the same PLC and share a single connection.

**Guiding principle of this issue**: All operation-relevant fields — area, address, data type, length — can be controlled both **statically in the node config** and **dynamically per message**. **Only the PLC target (host/rack/slot) is exclusively static** and is maintained once in the config node.

**Read shape**: two complementary operational patterns are supported, each with its own UX, both backed by the same connection manager:

1. **Variables mode** (`mode=static|dynamic`) — the user lists named variables (`DB10.DBD0`, `M0.0`, …) with data types. The node reads them via S7 multi-read (`AGReadMulti`) and emits decoded values. Right choice for sparse layouts (a couple of values across different areas) and for casual development reads.
2. **Block mode** (`mode=block`) — the user picks one contiguous byte range (e.g. *DB1, bytes 0..200*). The node fetches that block in a **single** `AGReadArea` call per poll and emits the raw `[]byte`. Decoding happens downstream in an [`s7-parser`](./PARSER_S7_NODE.md) node. Right choice for tightly packed DBs typical of a Siemens project — it reduces round-trips dramatically (one block read of 200 bytes vs. ~50 individual reads with 12-byte item headers each), and the parse layout is shared between read and write paths via the parser node.

The **PLC manager** transparently auto-coalesces nearby individual variables in *variables* mode into a single block read where it can — see [Out of Scope](#out-of-scope) for the planned Phase 3 implementation.

**Scope**: S7 over RFC1006 (ISO-on-TCP, port 102) for S7-300 / S7-400 / S7-1200 / S7-1500. LOGO! (TSAP-based) is supported via an explicit `connection=logo` toggle with manual TSAP fields. S7 Optimized DBs (`Optimized block access` checkbox in TIA Portal, only on 1200/1500) are **not** addressable via the S7 protocol — those DBs require OPC UA. The user must un-tick "Optimized" on DBs they want to access from LOOPZE; this is documented prominently. Supported areas: **DB**, **M (Merker / Flags)**, **I (Inputs / PE)**, **Q (Outputs / PA)**, **C (Counters)**, **T (Timers)**. Data types: see [Data Types](#data-types) for the full table — common ones are BOOL, BYTE, WORD/INT, DWORD/DINT, REAL, LREAL, LINT, ULINT, STRING, WSTRING.

## Overview

| Node type | Type ID | Canvas inputs | Canvas outputs | Description |
|---|---|---|---|---|
| **S7 Read** | `s7-read` | 0 or 1 | 1 | Reads variables from one or more areas of the PLC, **or** a contiguous byte block (raw `[]byte`) for downstream parsing |
| **S7 Write** | `s7-write` | 1 | 0 or 1 | Writes values to one or more variables in the PLC, **or** writes a raw `[]byte` block in one call |

```
                          SIEMENS PLC (e.g. S7-1500)
                          ┌─────────────────────────┐
Flow A                    │  DB10.DBD0 (REAL)       │
┌────────────────────┐    │  DB10.DBW4 (INT)        │
│  [Inject every 1s] │    │  M0.0      (BOOL)       │
│        ↓           │    │  IB0       (BYTE)       │
│  [S7 Read] ←───────────┤  QB0       (BYTE)       │
│        ↓           │    │                          │
│  [Debug]           │    └─────────────────────────┘
└────────────────────┘                  ↑
                                         │
Flow B                                   │
┌────────────────────┐                   │
│  [Inject]          │                   │
│        ↓           │                   │
│  [S7 Write] ──────────────────────────┘
└────────────────────┘
```

## Requirements

### 1. Config Node: S7 PLC (`s7-plc`)

The S7 PLC is a **config node** (see `NODE_MQTT.md` / `NODE_MODBUS.md` — concept reused here). It does not appear on the canvas and is referenced by `s7-read` / `s7-write` nodes.

- **Type ID**: `s7-plc`
- **No canvas element** — purely configurative
- **Configuration fields**:
  - `name` (string) — display name, e.g. "Press Line 2"
  - `host` (string) — hostname or IP of the CP / PN interface
  - `port` (number) — default: 102
  - `connection` (string) — `s7-1200-1500` (default) | `s7-300-400` | `logo` | `custom`
  - **Auto-derived rack/slot** depending on `connection`:
    - `s7-1200-1500`: rack=0, slot=1
    - `s7-300-400`: rack=0, slot=2
    - `logo`: rack and slot ignored, TSAPs used instead
    - `custom`: free entry of `rack` / `slot`
  - `rack` (number, only `custom` / overridable for `s7-300-400`) — default depends on connection mode
  - `slot` (number, only `custom` / overridable) — default depends on connection mode
  - **LOGO! / custom TSAPs** (only when `connection=logo` or `connection=custom`):
    - `localTsap` (string, hex, e.g. `0x0100`) — Source TSAP
    - `remoteTsap` (string, hex, e.g. `0x0200`) — Destination TSAP
  - `pduSize` (number) — requested max PDU size in bytes; default: 480 (S7-1500), 240 (S7-300/400). Negotiated downward by the PLC, the actual value is logged
  - `timeout` (number) — request timeout in milliseconds; default: 2000
  - `idleTimeout` (number) — seconds after which an unused TCP connection is closed; default: 60. `0` = never close
  - `reconnectBackoff` (number) — seconds between reconnect attempts after a connection loss; default: 5

- **Access to the properties dialog**:
  - **New PLC**: via the "+" button next to the PLC dropdown in S7 nodes
  - **Edit existing PLC**: via the "Edit PLC config" link below the dropdown

- **Properties dialog**:

```
┌──────────────────────────────────────────────┐
│  S7 PLC                                       │
├──────────────────────────────────────────────┤
│                                               │
│  Name                                         │
│  ┌────────────────────────────────────────┐   │
│  │ Press Line 2                           │   │
│  └────────────────────────────────────────┘   │
│                                               │
│  Host                          Port            │
│  ┌──────────────────────────┐ ┌──────────┐   │
│  │ 192.168.1.20             │ │ 102      │   │
│  └──────────────────────────┘ └──────────┘   │
│                                               │
│  Connection Type                              │
│  ┌────────────────────────────────────────┐   │
│  │ S7-1200 / S7-1500                  ▼  │   │
│  └────────────────────────────────────────┘   │
│                                               │
│  ── Auto-derived ────────────────────────────│
│  Rack: [0]   Slot: [1]                        │
│                                               │
│  ── LOGO! / Custom TSAPs (greyed out) ───────│
│  Local TSAP   [ 0x0100               ]        │
│  Remote TSAP  [ 0x0200               ]        │
│                                               │
│  ── Advanced ────────────────────────────────│
│  PDU Size (req.):    [480]                    │
│  Timeout (ms):       [2000]                   │
│  Idle Timeout (s):   [60]                     │
│  Reconnect (s):      [5]                      │
│                                               │
│  ┌──────────────────┐                         │
│  │ Test Connection  │                         │
│  └──────────────────┘                         │
│                                               │
│  ┌────────────┐  ┌────────────┐              │
│  │   Save     │  │  Cancel    │              │
│  └────────────┘  └────────────┘              │
└──────────────────────────────────────────────┘
```

The **Test Connection** button performs a one-shot connect (`ISO-TSAP CR/CC` + `S7 COMM SETUP`), reports the negotiated PDU size and the CPU type returned by `READSZL` (e.g. "CPU 1516-3 PN/DP"), and closes the session. Helps with diagnosing rack/slot/TSAP issues without deploy.

### 2. S7 Read Node (`s7-read`)

- **Canvas**:
  - Static mode (cyclic poll of the variables list): 0 inputs, 1 output
  - Dynamic mode (variables list, on demand): 1 input, 1 output — the input triggers the read
  - Block mode (cyclic poll of one contiguous byte block): 0 inputs (default) or 1 input when `triggerOnInput=true`, 1 output. Returns raw `[]byte`; downstream decoding happens in [`s7-parser`](./PARSER_S7_NODE.md)
- **Function**: Reads one or more configured variables (or one contiguous byte block in `block` mode) from the PLC and emits the result(s) as a flow message. In dynamic mode all parameters can be overridden via `msg`.

- **Base configuration**:
  - `plc` (string) — ID of the referenced `s7-plc` config node
  - `mode` (string) — `static` (default) | `dynamic` | `block`
  - `variables` (object[], used when `mode=static|dynamic`) — list of variables to read in one round-trip:
    - `name` (string) — user-defined display name (e.g. "Boiler Temperature"), used as key in the output object and as `topic` in per-item mode
    - `address` (string) — Siemens-style address, see "Address Syntax" below (e.g. `DB10.DBD0`, `M0.0`, `IB4`, `DB1.STRING50.20`)
    - `dataType` (string) — see the full [Data Types](#data-types) table for the complete set. Common values: `bool` | `byte` | `sint` | `usint` | `word` | `int` | `uint` | `dword` | `dint` | `udint` | `real` | `lreal` | `lint` | `ulint` | `lword` | `string` | `wstring` | `wchar` | `time` / `ltime` / `tod` / `ltod` / `date` / `dt` / `ldt` / `dtl` (numerical/structured time types) | `raw`. For BOOL the address must contain a bit (e.g. `M0.3`); for STRING the encoded length comes from the address (`DB1.STRING50.20` = STRING starting at byte 50, max length 20). 8-byte types use the `DBL` form (`DB10.DBL16`); the 12-byte `dtl` uses its own `DTL` form (`DB10.DTL171`)
    - `scale` (number, optional) — multiplicative scaling factor; default 1
    - `offset` (number, optional) — additive offset; applied **after** scaling
  - `block` (object, used when `mode=block`) — single contiguous byte block to read:
    - `area` (string) — `DB` (default) | `M` | `I` | `Q`
    - `db` (number) — DB number, required when `area=DB`
    - `start` (number) — start byte offset within the area (0-based)
    - `length` (number) — byte count to read. Hard upper bound: negotiated PDU size minus 22 bytes header (≈ 460 bytes on S7-1500 with default PDU 480, ≈ 220 bytes on S7-300 with PDU 240). The PLC manager auto-splits oversized blocks into multiple `AGReadArea` calls and concatenates the results
    - `triggerOnInput` (boolean, default `false`) — adds an input port that triggers an extra read on every message (additive on top of the cyclic poll). Useful for "read on demand without waiting for the next tick" patterns
  - `outputShape` (string, used when `mode=static|dynamic`) — `single` | `array` | `object` (default for >1):
    - `single`: scalar in `msg.payload` (only if `variables.length === 1`)
    - `array`: `[{name, address, dataType, value}, …]` in `msg.payload`
    - `object`: `{ "<name>": <value>, … }` in `msg.payload` — fastest for downstream Function/Switch nodes
  - `topicTemplate` (string, optional) — default `s7/<plc-name>/<address>` (variables mode) or `s7/<plc-name>/<area><db>` (block mode). Variables `<plc-name>`, `<address>`, `<name>`, `<area>`, `<db>`, `<start>`, `<length>` are substituted
- **Polling (static mode)**:
  - `pollInterval` (number) — interval in milliseconds between reads. Default: 1000
  - `emitOnChange` (boolean) — if `true`, only emits when **any** value changes. Default: `false`
  - `emitOnError` (boolean) — if `true`, emits an error message on a read failure (`msg.error` set, `msg.payload` empty). If `false` (default), only logs internally and sets the status to red — no error output
- **Dynamic mode** — overrides via `msg` (missing field → config default):
  - `msg.variables` (object[]) — full override of the variable list
  - `msg.address` + `msg.dataType` (strings) — convenience form for a single read; equivalent to `msg.variables=[{address, dataType, name: "value"}]`
  - An input message without `msg.action` triggers a read with the effective parameters. Incoming messages are **not** passed through to the output — the output contains only the read result

- **Block mode + `triggerOnInput=true`** — overrides via `msg`:
  - `msg.s7.area` / `msg.s7.db` / `msg.s7.start` / `msg.s7.length` — override the configured block per message. Useful for variable-length payload fetches (e.g. read the DB header byte 0 first, parse the length, then read the rest in a follow-up message)
  - Incoming messages are **not** forwarded — the output contains only the raw block bytes plus metadata

- **Outgoing message** (example, `outputShape=object`):
  ```json
  {
    "payload": {
      "BoilerTemp": 84.7,
      "PressureBar": 5.2,
      "AlarmActive": false
    },
    "topic": "s7/press-line-2",
    "s7": {
      "plc": "Press Line 2",
      "variables": [
        { "name": "BoilerTemp",   "address": "DB10.DBD0", "dataType": "real", "value": 84.7 },
        { "name": "PressureBar",  "address": "DB10.DBD4", "dataType": "real", "value": 5.2 },
        { "name": "AlarmActive",  "address": "M0.0",      "dataType": "bool", "value": false }
      ]
    }
  }
  ```

  - `msg.payload` — depending on `outputShape` (scalar / array / object)
  - `msg.s7` — metadata block with the effective read parameters and per-variable values; useful for debugging and round-trip workflows

- **Outgoing message** (block mode):
  ```json
  {
    "payload": [/* raw byte array, e.g. 200 bytes from DB1 */],
    "topic": "s7/press-line-2/db1",
    "s7": {
      "plc": "Press Line 2",
      "area": "DB",
      "db": 1,
      "start": 0,
      "length": 200
    }
  }
  ```

  In block mode `msg.payload` is a raw `[]byte` ready to be piped into an [`s7-parser`](./PARSER_S7_NODE.md) node. No per-variable decoding happens here — the parser owns the layout, which keeps read and write sharing one schema.

- **Status display**:
  - Green: `connected · <interval>` (static / block) or `connected · idle` (dynamic)
  - Yellow: `connecting…` / `reconnecting…`
  - Red: error message with S7 error code, e.g. `Item not available (0x05)` — typical when an Optimized DB is addressed

- **Properties panel**:

```
┌──────────────────────────────────────────────┐
│  S7 Read                                      │
├──────────────────────────────────────────────┤
│                                               │
│  PLC                                          │
│  ┌────────────────────────────────┐ ┌───┐    │
│  │ Press Line 2               ▼  │ │ + │    │
│  └────────────────────────────────┘ └───┘    │
│  Edit PLC config                              │
│                                               │
│  Mode                                         │
│  ( • ) Static (variables, poll)               │
│  ( ) Dynamic (variables, on input)            │
│  ( ) Block (raw bytes, poll)                  │
│                                               │
│  Variables                                    │
│  ┌─────────────────────────────────────────┐  │
│  │ ≡ Name    [BoilerTemp                ] x│  │
│  │   Address [DB10.DBD0      ] Type [real▼]│  │
│  │   Scale [1]   Offset [0]                │  │
│  ├─────────────────────────────────────────┤  │
│  │ ≡ Name    [AlarmActive               ] x│  │
│  │   Address [M0.0            ] Type [bool▼]│ │
│  └─────────────────────────────────────────┘  │
│  [+ Add Variable]                             │
│                                               │
│  Output Shape  [Object (default for >1)   ▼]  │
│                                               │
│  ── Polling (Static only) ──────────────────│
│  Interval: [1000] ms                          │
│  ☐ Emit on change                             │
│  ☐ Emit on error                              │
│                                               │
└──────────────────────────────────────────────┘
```

In dynamic mode the variables editor is replaced by a hint block:

```
ℹ Send any message to trigger a read. Override the
   variable list via msg.variables=[{address,dataType,name}]
   or use the convenience form msg.address + msg.dataType
   for a single read.
```

In block mode the variables editor is replaced by a single block configuration:

```
┌──────────────────────────────────────────────┐
│  Block                                        │
│  Area  [ DB ▼ ]   DB Number [ 1 ]             │
│  Start [ 0  ]     Length    [ 200 ] bytes     │
│  ☐ Trigger on input (additive to poll)        │
│                                               │
│  ℹ  Output is raw msg.payload = []byte.       │
│     Pipe into an `s7-parser` node to decode.  │
│     Block size is capped by the negotiated    │
│     PDU; oversized blocks are auto-split.     │
└──────────────────────────────────────────────┘
```

### 3. S7 Write Node (`s7-write`)

- **Canvas**:
  - 1 input
  - 0 outputs (default — sink)
  - **Optional**: 1 output when `emitAck=true` — emits an ACK message after a successful write
- **Function**: Writes incoming values to one or more PLC variables, or — in `block` mode — writes a raw `[]byte` block in a single `AGWriteArea` call. Static or dynamic per `msg`.

- **Base configuration**:
  - `plc` (string) — ID of the referenced `s7-plc` config node
  - `mode` (string) — `static` (default) | `dynamic` | `block`
  - `block` (object, used when `mode=block`) — single contiguous byte block to write:
    - `area` (string) — `DB` (default) | `M` | `Q` (writing to `I` is not allowed by the protocol on most CPUs and is rejected by the parser)
    - `db` (number) — DB number, required when `area=DB`
    - `start` (number) — start byte offset within the area
    - `length` (number, optional) — expected byte count. If set, incoming `msg.payload` arrays of a different length are rejected with `BadTypeMismatch`. If omitted, the length of the incoming `msg.payload` is taken as-is
    - `inputProperty` (string, default `payload`) — message field carrying the raw byte array. Switchable to e.g. `bytes` for direct chaining behind an [`s7-parser`](./PARSER_S7_NODE.md) `encode` action
  - `variables` (object[], used when `mode=static|dynamic`) — per write:
    - `address` (string) — Siemens-style address
    - `dataType` (string) — analogous to `s7-read`
    - `valueSource` (string) — `static` | `msg`:
      - `static`: `value` is taken from the config (typed against `dataType`)
      - `msg`: value is read from a message path (default `payload`, alternatively `payload.<key>`, etc.)
    - `valuePath` (string, only `valueSource=msg`, default `payload`)
    - `scale` (number, optional), `offset` (number, optional) — applied **inversely** to the read path: `raw = (value - offset) / scale`
  - `emitAck` (boolean) — if `true`, the node enables an output and emits a confirmation after a successful write. Default: `false`
  - `passthrough` (boolean) — if `true`, the input message is forwarded with `msg.s7Write` enriched. Mutually exclusive with `emitAck` style new-message output. Default: `false`

- **Incoming message**:
  - **Static mode**: every input message triggers the writes defined in the config; values come from `valuePath` per row (or from the baked-in `value` for `valueSource=static` rows). This is the right mode for "fixed addresses, values from messages" — by far the most common pattern
  - **Dynamic mode**: the variable list comes **only** from the message — the configured `variables` array is **ignored**. The UI hides the sidebar list in this mode to make that explicit. Either provide the full form `msg.variables`:
    ```json
    {
      "variables": [
        { "address": "DB10.DBD0", "dataType": "real", "value": 100.5 },
        { "address": "M0.0",      "dataType": "bool", "value": true   },
        { "address": "DB1.STRING50.20", "dataType": "string", "value": "JOB-4711" }
      ]
    }
    ```
    or the convenience form for a single write:
    ```json
    { "address": "DB10.DBD0", "dataType": "real", "payload": 100.5 }
    ```
    Note: the read-side `effectiveVariables` falls back to the configured list when a dynamic message carries no overrides; the write side does **not** (writing also needs values, and the static mode already covers fixed-address-with-msg-values configurations).
  - **Block mode**: `msg.payload` (or whichever field `inputProperty` points to) is a `[]byte` and is written verbatim to the configured `area`/`db`/`start`. Override per message via `msg.s7.area` / `msg.s7.db` / `msg.s7.start`. The block is sent in **one** `AGWriteArea` call (or auto-split into multiple calls when it exceeds the negotiated PDU size). This is the natural counterpart to `s7-parser` `encode` action — no per-field protocol overhead

- **Outgoing message** (only when `emitAck=true` or `passthrough=true`):
  ```json
  {
    "payload": true,
    "s7Write": {
      "plc": "Press Line 2",
      "results": [
        { "address": "DB10.DBD0", "dataType": "real", "ok": true },
        { "address": "M0.0",      "dataType": "bool", "ok": true }
      ],
      "allOk": true
    }
  }
  ```
  - `msg.payload` is `true` if `allOk` else `false`
  - `msg.s7Write.results` lists per-variable success/error info; `error` carries the S7 error name when `ok=false`

- **Error behavior**:
  - On S7 error response (e.g. `Item not available`, `Address out of range`, `DB does not exist`): status set to red, error message contains the code. If a **Catch node** is present in the flow, the error is routed via the catch path
  - On TCP connection loss: reconnect runs, write fails with `PLC unavailable`
  - On type mismatch (e.g. trying to write a `string` value into a `real` address): write is **not** sent, error returned as `BadTypeMismatch` with field path

- **Properties panel**:

```
┌──────────────────────────────────────────────┐
│  S7 Write                                     │
├──────────────────────────────────────────────┤
│                                               │
│  PLC                                          │
│  ┌────────────────────────────────┐ ┌───┐    │
│  │ Press Line 2               ▼  │ │ + │    │
│  └────────────────────────────────┘ └───┘    │
│                                               │
│  Mode  ( • ) Static  ( ) Dynamic  ( ) Block   │
│                                               │
│  Variables                                    │
│  ┌─────────────────────────────────────────┐  │
│  │ ≡ Address [DB10.DBD0   ] Type [real ▼] x│  │
│  │   Source  [msg.payload                ] │  │
│  │   Scale [1]   Offset [0]                │  │
│  ├─────────────────────────────────────────┤  │
│  │ ≡ Address [M0.0        ] Type [bool ▼] x│  │
│  │   Source  [static                     ] │  │
│  │   Value   [true                       ] │  │
│  └─────────────────────────────────────────┘  │
│  [+ Add Variable]                             │
│                                               │
│  ☐ Emit ACK on success                        │
│  ☐ Pass message through with s7Write          │
│                                               │
│  ℹ  msg.variables overrides the config in     │
│     dynamic mode. Convenience: msg.address +  │
│     msg.dataType + msg.payload for a single   │
│     write.                                    │
│                                               │
└──────────────────────────────────────────────┘
```

In block mode the variables editor is replaced by the same block configuration as `s7-read` (Area / DB / Start / Length / Input property), and the hint text changes to:

```
ℹ  Input is a raw msg.payload = []byte (or whatever
   `inputProperty` points to — set to `bytes` to chain
   directly behind an `s7-parser` encode). The block is
   sent in one AGWriteArea call (or auto-split when it
   exceeds the negotiated PDU). msg.s7.area/db/start
   override the config per message.
```

### 4. Connection Sharing & Multi-Variable Reads

When multiple S7 nodes reference the same PLC, **one** TCP/RFC1006 connection is shared:

```
[s7-read  vars=A,B,C]   ──┐
[s7-read  vars=D,E]    ──┤── PLC "Press Line 2" ── 1 TCP / 1 S7 session
[s7-write vars=F,G]    ──┘
```

The engine provides a **PLC manager** that:
1. Manages one connection per `s7-plc` ID
2. Serializes read and write requests (S7 is half-duplex on a single connection — concurrent requests are queued)
3. **Bundles** multiple variables of one node into a single S7 multi-read request (`ReadMultiVars`) up to the negotiated PDU size; oversized requests are split automatically
4. Automatically reconnects on connection loss with the configured backoff
5. Cleanly closes all connections on stop / re-deploy

**Per-PLC serialization**: In the S7 protocol only one job may run at a time on a connection. Multiple parallel polls on the same PLC are processed serially by the manager — via mutex or request queue. The PLC manager is **per PLC config**, not per node — so serialization spans across all nodes that share this PLC.

**PDU bundling**: The `gos7` library exposes `ReadMultiVars` which packs N items into one telegram. The manager fills the buffer up to `(pduSize - header overhead)` bytes per request and chains the rest. This is the fundamental performance trick on S7: 50 single reads at 10 ms each is 500 ms; one bundled multi-read is ~15 ms.

**Block-mode bundling**: In `mode=block` the node bypasses `ReadMultiVars` entirely and uses `AGReadArea` (single contiguous fetch). This is even cheaper than a multi-read — no per-item descriptors, the PLC just streams the byte range. For tightly-packed DBs (the typical Siemens project layout), block mode + downstream `s7-parser` is the fastest path and the recommended pattern for production polling. Each item header in a multi-read costs ~12 bytes; a 200-byte block read carries ~22 bytes of overhead total versus ~600 bytes for the equivalent 50-variable multi-read.

**Auto-coalescing** (Phase 3, see [Out of Scope](#out-of-scope)): a planned later optimization detects clusters of individual variables that fall within the same DB and within a configurable byte gap, and silently rewrites those reads as one block fetch + client-side slicing — giving block-mode performance to flows that were authored in variables mode.

**Polling stagger**: When multiple read nodes with the same `pollInterval` poll the same PLC, their tick times are offset (round-robin) so the load is distributed evenly. Optimization — not required for v1.

## Address Syntax

S7 addresses follow the Siemens engineering tool notation. The string in the `address` field is parsed by a dedicated address parser. Forms supported:

| Form | Example | Meaning | Required `dataType` |
|---|---|---|---|
| `DB<n>.DBX<byte>.<bit>` | `DB10.DBX2.3` | DB10, byte 2, bit 3 | `bool` |
| `DB<n>.DBB<byte>` | `DB10.DBB4` | DB10, byte 4 | `byte` / `char` / `sint` / `usint` |
| `DB<n>.DBW<byte>` | `DB10.DBW6` | DB10, word at byte 6 | `word` / `int` / `uint` / `wchar` / `date` |
| `DB<n>.DBD<byte>` | `DB10.DBD0` | DB10, dword at byte 0 | `dword` / `dint` / `udint` / `real` / `time` / `tod` |
| `DB<n>.DBL<byte>` | `DB10.DBL16` | DB10, 8-byte long at byte 16 (S7-1500 64-bit types; not native TIA syntax — see notes) | `lreal` / `lint` / `ulint` / `lword` / `ltime` / `ltod` / `ldt` / `dt` |
| `DB<n>.DTL<byte>` | `DB3.DTL171` | DB3, 12-byte structured DateTime at byte 171 (DTL is the only fixed-12-byte type; not native TIA syntax — see notes) | `dtl` |
| `DB<n>.STRING<byte>.<maxlen>` | `DB1.STRING50.20` | DB1, S7 STRING starting at byte 50, max length 20 (= 22 wire bytes incl. 2-byte header) | `string` |
| `DB<n>.WSTRING<byte>.<maxlen>` | `DB1.WSTRING50.20` | DB1, S7 WSTRING starting at byte 50, max length 20 chars (= 44 wire bytes: 4-byte header + 20×2-byte UCS-2 chars) | `wstring` |
| `M<byte>.<bit>` | `M0.3` | Merker bit | `bool` |
| `MB<byte>` / `MW<byte>` / `MD<byte>` | `MB10`, `MW12`, `MD16` | Merker byte / word / dword | `byte` / `word` / `dword` / `int` / `dint` / `real` |
| `I<byte>.<bit>` / `IB`/`IW`/`ID` | `I0.0`, `IB1`, `IW2`, `ID4` | Inputs (PE) | bit / byte / word / dword |
| `Q<byte>.<bit>` / `QB`/`QW`/`QD` | `Q0.0`, `QB1`, `QW2`, `QD4` | Outputs (PA) | bit / byte / word / dword |
| `C<n>` | `C5` | Counter | `int` (BCD-decoded by the library) |
| `T<n>` | `T3` | Timer | `int` (S5 time-decoded by the library) |

**Validation**: The frontend validates syntax via regex on input — invalid addresses are flagged before save. Existence and access-rights checks happen on the first read/write at runtime; a non-existing DB returns `Item not available` from the PLC (catchable via Catch node).

**Optimized DBs**: Address `DB<n>.<symbol>` (symbolic addressing) is **not supported** — Optimized DBs return only via OPC UA. The parser explicitly rejects symbolic syntax with a hint `S7 protocol requires non-optimized DBs; use OPC UA for symbolic access`.

## Data Types

The full SIEMENS TIA-Portal type system. The `LOOPZE code` column is the
lowercase identifier accepted by the `dataType` field (in `s7-read` /
`s7-write` variables and in the `s7-parser` layout). Status legend:

- ✅ **shipped** — codec + address parser + parser layout
- 🟡 **codec only** — usable in the parser layout; no native address form yet (use `DBB`/`DBW`/`DBD` with the `signed` flag, or block-mode + parser)
- ⏳ **planned** — backlog item, not implemented yet

### Bitfields (Binärzahlen)

| TIA type | Width | LOOPZE code | Range / format | Status | S7-300/400 | S7-1200 | S7-1500 |
|---|---|---|---|---|---|---|---|
| `BOOL` | 1 bit | `bool` | `false` / `true` | ✅ | ✓ | ✓ | ✓ |
| `BYTE` | 8 bit | `byte` | `0..255` (or `-128..127` with `signed:true`) | ✅ | ✓ | ✓ | ✓ |
| `WORD` | 16 bit | `word` | `0..65535` (BE; `signed:true` flips to `int16`) | ✅ | ✓ | ✓ | ✓ |
| `DWORD` | 32 bit | `dword` | `0..4_294_967_295` (BE; `signed:true` → `int32`) | ✅ | ✓ | ✓ | ✓ |
| `LWORD` | 64 bit | `lword` | `0..2^64-1` (64-bit bitfield) | ✅ | — | — | ✓ |

### Integers (Ganzzahlen)

| TIA type | Width | LOOPZE code | Range | Status | S7-300/400 | S7-1200 | S7-1500 |
|---|---|---|---|---|---|---|---|
| `SINT` | 8 bit | `sint` | `-128..127` | ✅ | — | ✓ | ✓ |
| `USINT` | 8 bit | `usint` | `0..255` | ✅ | — | ✓ | ✓ |
| `INT` | 16 bit | `int` | `-32_768..32_767` | ✅ | ✓ | ✓ | ✓ |
| `UINT` | 16 bit | `uint` | `0..65_535` | ✅ | — | ✓ | ✓ |
| `DINT` | 32 bit | `dint` | `-2_147_483_648..2_147_483_647` | ✅ | ✓ | ✓ | ✓ |
| `UDINT` | 32 bit | `udint` | `0..4_294_967_295` | ✅ | — | ✓ | ✓ |
| `LINT` | 64 bit | `lint` | `-2^63..2^63-1` (~±9.2 quintillion) | ✅ | — | — | ✓ |
| `ULINT` | 64 bit | `ulint` | `0..2^64-1` (~1.84 × 10^19) | ✅ | — | — | ✓ |

JSON-decoded numbers arrive as `float64`, which can represent integers
exactly up to 2^53 (~9 quadrillion). For LINT / ULINT values beyond that, a
caller passing native Go `int64` / `uint64` to `EncodeS7Scalar` keeps the
full precision; JSON callers are limited by the float64 mantissa.

### Floats (Gleitpunktzahlen)

| TIA type | Width | LOOPZE code | Precision | Status | S7-300/400 | S7-1200 | S7-1500 |
|---|---|---|---|---|---|---|---|
| `REAL` | 32 bit | `real` | IEEE 754 single, ~6-7 decimal digits | ✅ | ✓ | ✓ | ✓ |
| `LREAL` | 64 bit | `lreal` | IEEE 754 double, ~15 decimal digits | ✅ | — | ✓ | ✓ |

### Time durations (Zeiten)

| TIA type | Width | LOOPZE code | Format | Status | S7-300/400 | S7-1200 | S7-1500 |
|---|---|---|---|---|---|---|---|
| `S5TIME` | 16 bit | `timer` | BCD with timebase, `S5T#10s` | ✅ (read only; decode → ms `int`) | ✓ | — | ✓ |
| `TIME` | 32 bit | `time` | Signed ms, `T#-24d20h31m23s648ms..+24d…` | ✅ (decode → ms `int`) | ✓ | ✓ | ✓ |
| `LTIME` | 64 bit | `ltime` | Signed ns, `LT#±106751d…` | ✅ (decode → ns `int64`) | — | ✓ | ✓ |

### Characters & strings (Zeichen)

| TIA type | Width | LOOPZE code | Range | Status | S7-300/400 | S7-1200 | S7-1500 |
|---|---|---|---|---|---|---|---|
| `CHAR` | 8 bit | `char` | ASCII | ✅ | ✓ | ✓ | ✓ |
| `WCHAR` | 16 bit | `wchar` | Unicode BMP | ✅ (decode → 1-char `string`; encode rejects multi-char) | — | ✓ | ✓ |
| `STRING` | n+2 byte | `string` | 0..254 ASCII chars; wire = `[maxLen][actLen][char × maxLen]` | ✅ | ✓ | ✓ | ✓ |
| `WSTRING` | 4+2n byte | `wstring` | 0..16382 UCS-2 chars; wire = `[maxLen u16][actLen u16][char × maxLen × u16]` | ✅ | — | ✓ | ✓ |

WSTRING handles the BMP (code points up to U+FFFF). Surrogate pairs (above
U+FFFF) get truncated to their low 16 bits — same semantics as gos7's
`SetWStringAt`. Industrial text payloads (machine names, recipe IDs) are
typically Latin / Cyrillic / CJK BMP, all of which round-trip cleanly.

### Date & time (Datum und Uhrzeit)

| TIA type | Width | LOOPZE code | Format / range | Status | S7-300/400 | S7-1200 | S7-1500 |
|---|---|---|---|---|---|---|---|
| `DATE` | 16 bit | `date` | Days since 1990-01-01, `D#1990-01-01..D#2168-12-31` | ✅ (decode → ISO `"YYYY-MM-DD"` string; encode accepts string or days as `int`) | ✓ | ✓ | ✓ |
| `TOD` (`TIME_OF_DAY`) | 32 bit | `tod` | `00:00:00.000..23:59:59.999` (ms since midnight) | ✅ (decode → ms `int`) | ✓ | ✓ | ✓ |
| `LTOD` (`LTIME_OF_DAY`) | 64 bit | `ltod` | ns since midnight, full nanosecond precision | ✅ (decode → ns `uint64`) | — | ✓ | ✓ |
| `DT` (`DATE_AND_TIME`) | 64 bit | `dt` | BCD year/month/day/hour/min/sec/ms+weekday | ✅ (decode → RFC3339Nano `string`; encode accepts RFC3339 string) | ✓ | — | — |
| `LDT` (`L_DATE_AND_TIME`) | 64 bit | `ldt` | ns since 1970-01-01 (epoch), `LDT#1970-01-01..2262-04-11` | ✅ (decode → RFC3339Nano `string`; encode accepts RFC3339) | — | ✓ | ✓ |
| `DTL` | 96 bit | `dtl` | Structured: year (u16) / month / day / weekday / hour / min / sec / ns (u32) | ✅ (decode → RFC3339Nano `string`; encode accepts RFC3339) | — | ✓ | ✓ |

**Output conventions**:
- Numerical durations (`TIME`, `LTIME`, `TOD`, `LTOD`) decode to integers
  (ms or ns as documented). Easier for downstream consumers than parsing
  RFC3339 duration strings, and JSON-clean.
- `DATE` decodes to a plain ISO date string (`"2026-05-10"`).
- `DT` / `LDT` / `DTL` decode to **RFC3339Nano UTC strings** so timestamps
  round-trip cleanly through JSON and most parsers. The wall-clock vs.
  UTC distinction is documented per type — there is no timezone metadata
  on the wire.

### Special

| Type | LOOPZE code | Meaning | Status |
|---|---|---|---|
| Counter | `counter` | S7 BCD counter (2 bytes BCD, 0..999) — decode only | ✅ (read) |
| Timer | `timer` | Same as S5TIME (kept as alias for the C/T areas) | ✅ (read) |
| Raw | `raw` | Opaque byte slice, `length` bytes — passes through unchanged | ✅ |

Counter/timer encoding is intentionally not in v1 — writing to a CPU's
internal counters / timers from a client is rare in industrial practice and
the wire format is asymmetric (gos7's `ToCounter` is buggy). Add when a real
customer use case appears.

### Implementation notes

- **Aliases as a quick win**: `sint`, `usint`, `uint`, `udint` are wire-equivalent to existing types (`byte`, `byte`, `word`, `dword`). Adding them as recognised `dataType` strings is a one-line dispatch in `S7TypeWordLen` / the codec — straightforward follow-up. Tracked but not blocking v1.
- **64-bit bitfield (`LWORD`)**: identical wire layout to `ULINT`. Adding it as a recognised type name is trivial; the only difference from `ULINT` is the JSON output type (`uint64` either way, but the user-facing label differs).
- **Date/Time types**: gos7's `Helper` already provides `GetDateTimeAt`, `GetDateAt`, `GetTODAt`, `GetLTODAt`, `GetLDTAt`, `GetDTLAt` (and the inverse setters). Wiring these into the codec is a half-day exercise; the open question is the **JSON output shape** — `time.Time` marshals as RFC3339, but downstream parsers may prefer epoch ms. Pick one convention before implementing.
- **Per-CPU availability**: 64-bit types (`LWORD`/`LINT`/`ULINT`/`LREAL`/`LTIME`/`LTOD`/`LDT`) are S7-1500-only. The protocol on S7-300/400 doesn't support them; attempting a multi-read on those CPUs returns "Item not available" at runtime. We don't pre-validate against the negotiated CPU type — the runtime error is informative enough.

## Data Structure

### workspace.json

```json
{
  "flows": [
    {
      "id": "flow-1",
      "type": "tab",
      "label": "Press Line 2",
      "nodes": [
        {
          "id": "node-s7-read-1",
          "type": "s7-read",
          "name": "Boiler & Pressure",
          "x": 200,
          "y": 150,
          "z": "flow-1",
          "inputs": 0,
          "outputs": 1,
          "wires": [["node-debug-1"]],
          "config": {
            "plc": "plc-1",
            "mode": "static",
            "variables": [
              { "name": "BoilerTemp",   "address": "DB10.DBD0", "dataType": "real" },
              { "name": "PressureBar",  "address": "DB10.DBD4", "dataType": "real", "scale": 0.1 },
              { "name": "AlarmActive",  "address": "M0.0",      "dataType": "bool" }
            ],
            "outputShape": "object",
            "pollInterval": 1000,
            "emitOnChange": false,
            "emitOnError": false
          }
        },
        {
          "id": "node-s7-read-block-1",
          "type": "s7-read",
          "name": "DB1 block (200 B)",
          "x": 200,
          "y": 250,
          "z": "flow-1",
          "inputs": 0,
          "outputs": 1,
          "wires": [["node-s7-parser-1"]],
          "config": {
            "plc": "plc-1",
            "mode": "block",
            "block": {
              "area": "DB",
              "db": 1,
              "start": 0,
              "length": 200,
              "triggerOnInput": false
            },
            "pollInterval": 500,
            "emitOnChange": true,
            "emitOnError": false
          }
        },
        {
          "id": "node-s7-write-1",
          "type": "s7-write",
          "name": "Set Setpoint",
          "x": 600,
          "y": 300,
          "z": "flow-1",
          "inputs": 1,
          "outputs": 0,
          "wires": [],
          "config": {
            "plc": "plc-1",
            "mode": "static",
            "variables": [
              {
                "address": "DB10.DBD8",
                "dataType": "real",
                "valueSource": "msg",
                "valuePath": "payload"
              }
            ],
            "emitAck": false,
            "passthrough": false
          }
        },
        {
          "id": "node-s7-write-block-1",
          "type": "s7-write",
          "name": "Push DB10 block",
          "x": 600,
          "y": 400,
          "z": "flow-1",
          "inputs": 1,
          "outputs": 0,
          "wires": [],
          "config": {
            "plc": "plc-1",
            "mode": "block",
            "block": {
              "area": "DB",
              "db": 10,
              "start": 0,
              "length": 32,
              "inputProperty": "bytes"
            },
            "emitAck": true
          }
        }
      ]
    }
  ],
  "configs": [
    {
      "id": "plc-1",
      "type": "s7-plc",
      "name": "Press Line 2",
      "config": {
        "host": "192.168.1.20",
        "port": 102,
        "connection": "s7-1200-1500",
        "rack": 0,
        "slot": 1,
        "pduSize": 480,
        "timeout": 2000,
        "idleTimeout": 60,
        "reconnectBackoff": 5
      }
    }
  ]
}
```

## Affected Files

### Backend – New Files

- `internal/nodes/s7_plc.go` — S7 PLC config node: encapsulates the `gos7` client, ISO-on-TCP connect, COMM SETUP, reconnect logic, request serialization, multi-read bundling
- `internal/nodes/s7_read.go` — Read node: polling loop (static) or input trigger (dynamic), variable bundling into a single telegram, decoding into the target data types, output shape mapping
- `internal/nodes/s7_write.go` — Write node: encoding values into the wire format, S7 write request via the shared PLC, optional ACK
- `internal/nodes/s7_address.go` — Address parser: maps `DB10.DBD0`, `M0.3`, `IB4`, `DB1.STRING50.20` etc. to the `gos7` `S7DataItem` (Area, WordLen, DBNumber, Start, Amount, Bit). Independently testable (table-driven tests)
- `internal/nodes/s7_codec.go` — helper functions: type encoding/decoding (BOOL/BYTE/WORD/DWORD/INT/DINT/REAL/STRING), scale/offset handling, S7 STRING ↔ Go string conversion (length-byte aware), BCD decode for counters, S5 time decode for timers
- `internal/nodes/s7_address_test.go`, `internal/nodes/s7_codec_test.go`, `internal/nodes/s7_read_test.go`, `internal/nodes/s7_write_test.go`, `internal/nodes/s7_plc_test.go`
- `internal/server/s7_handlers.go` — REST handler for `/api/v1/s7/test-connection`
- `internal/server/s7_handlers_test.go`

### Backend – Changes

- `internal/server/server.go` — registration of `s7-read` and `s7-write` in `registerNodes()`
- `internal/flow/engine.go` — extension of the config node lifecycle (introduced with MQTT, generalized via Modbus) for `s7-plc`. Should require no S7-specific changes if the existing generalization is solid
- `internal/flow/registry.go` — `ConfigProvider` from the MQTT issue is reused
- `internal/storage/` — no changes needed (provided MQTT/Modbus already introduced the `configs` section)

### Frontend – New Files

- `frontend/src/components/config/S7NodeConfig.vue` — shared config component for `s7-read` and `s7-write`: PLC dropdown + "+", mode selector, variables editor (drag-and-drop reorder, add/remove), output shape (read), emit-ack/passthrough (write)
- `frontend/src/components/config/S7PlcConfig.vue` — PLC config dialog: connection-type switch with auto rack/slot, custom/LOGO TSAP fields, advanced timing block, "Test Connection" button
- `frontend/src/components/config/shared/S7AddressInput.vue` — Reusable address input with syntax validation (regex per form) and inline hint when the user types a symbolic address (suggests OPC UA)
- `frontend/src/components/config/shared/S7DataTypeSelect.vue` — Dropdown of S7 data types

### Frontend – Changes

- `frontend/src/components/PropertyPanel.vue` — dispatch for `s7-read` and `s7-write` to `S7NodeConfig`
- `frontend/src/components/nodes/tokens.ts` — add `s7-read` (input, blue), `s7-write` (output, blue-orange), in line with the Siemens corporate color
- `frontend/src/types/flow.ts` — TypeScript types for `S7PlcConfig`, `S7ReadConfig`, `S7WriteConfig`, `S7Variable`

### Backend – New API Endpoint

- `POST /api/v1/s7/test-connection` — Body: full PLC config; response: `{ ok: true, info: { cpuType, negotiatedPduSize, orderCode } }` or `{ ok: false, error: "<reason>" }`. The endpoint opens a session, calls `GetCpuInfo` (SZL ID 0x0011 / 0x001C), and immediately closes again. Read-only, no persistence

### Go Dependencies

- `github.com/robinson/gos7` — established Go library for the S7 protocol. Pure Go, no CGO. Supports S7-300 / 400 / 1200 / 1500 / LOGO!, multi-read/write, all standard data types. Currently the only production-ready pure-Go S7 implementation; an alternative would be a CGO wrapper around Snap7 (libsnap7), which would break the single-binary strategy

## Technical Notes

### Library Choice: gos7

`github.com/robinson/gos7` is pure Go (no CGO), implements RFC1006 ISO-on-TCP and the S7 COMM layer, and has been actively maintained for years (used by the Open Industrial UI projects). Functionally complete for the LOOPZE scope: connect, COMM SETUP, ReadArea / WriteArea, ReadMultiVars / WriteMultiVars, GetCpuInfo, GetCpStatus. The alternative — wrapping Snap7 via CGO — is rejected for the same reasons OPC UA picked `gopcua/opcua` over an open62541 wrapper: cross-compile, single binary, no system dependencies.

### S7 Areas — Mapping to `gos7`

| S7 area | Address prefix | `gos7` constant |
|---|---|---|
| Data Block | `DB<n>.…` | `S7AreaDB` |
| Merker / Flags | `M…`, `MB`, `MW`, `MD` | `S7AreaMK` |
| Inputs (PE) | `I…`, `IB`, `IW`, `ID` | `S7AreaPE` |
| Outputs (PA) | `Q…`, `QB`, `QW`, `QD` | `S7AreaPA` |
| Counters | `C<n>` | `S7AreaCT` |
| Timers | `T<n>` | `S7AreaTM` |

`WordLen` (Bit / Byte / Word / DWord / Real / S7String / Counter / Timer) is derived from `dataType`. `DBNumber` is set only for area `DB`. `Start` is the byte offset (or bit address: byte * 8 + bit for `WordLen=Bit`). `Amount` is normally `1`, except for `string` (= `maxLen`) and `raw` (= explicit length).

### Multi-Variable Read

The bundler in the PLC manager packs N items per request into a single S7 telegram (`ReadMultiVars`). The PDU size dictates the upper bound:

```go
func (m *S7Manager) Read(ctx context.Context, items []S7DataItem) error {
    m.mu.Lock()
    defer m.mu.Unlock()

    // Pack as many items as fit in (PDU - 22) bytes header overhead per chunk.
    // Each item header costs 12 bytes, the value follows.
    chunks := splitForPDU(items, m.pduSize)
    for _, chunk := range chunks {
        if err := m.client.AGReadMulti(chunk, len(chunk)); err != nil {
            return err
        }
    }
    return nil
}
```

Per-item errors (`item.Error`) are surfaced individually — a non-existing DB in one item does not abort the whole transaction. The read node maps these into the per-variable result list with status text.

### Polling Loop (Static Mode)

```go
func (n *S7ReadNode) startPolling(ctx context.Context) {
    ticker := time.NewTicker(time.Duration(n.pollInterval) * time.Millisecond)
    defer ticker.Stop()

    var lastPayload any
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            results, err := n.plc.Read(ctx, n.dataItems)
            if err != nil {
                n.SetStatus(StatusError, err.Error())
                if n.emitOnError {
                    n.send(0, errorMessage(err))
                }
                continue
            }
            decoded := n.codec.DecodeAll(results, n.variables)
            if n.emitOnChange && reflect.DeepEqual(decoded, lastPayload) {
                continue
            }
            lastPayload = decoded
            n.send(0, n.buildMessage(decoded, results))
            n.SetStatus(StatusOk, fmt.Sprintf("connected · %dms", n.pollInterval))
        }
    }
}
```

### Reconnect Strategy

- On TCP / RFC1006 error the `gos7` client returns from the next call. The PLC manager closes the socket, marks the connection as dead, and starts a reconnect loop with `reconnectBackoff` seconds between attempts
- During reconnect all pending read/write requests block until the timeout (configurable later: fail-fast mode for latency-critical use cases — for v1: block)
- The S7 COMM SETUP is repeated on every reconnect — the negotiated PDU size may change after a CPU swap, so the manager re-queries it
- Status is yellow during the attempt, red when the reconnect fails permanently

### S7 Error Codes

The S7 protocol returns numeric error codes that the `gos7` library exposes via `CliErrorText`. The most common ones in operation:

| Code | Meaning | Typical cause |
|---|---|---|
| `0x00000000` | OK | — |
| `0x00700000` (`errCliItemNotAvailable`) | Item not available | Address points to a non-existing / Optimized DB |
| `0x00800000` | Address out of range | Address beyond DB length |
| `0x00C00000` | DB does not exist | Wrong DB number |
| `0x00D00000` | Function not available | Operation not supported on this CPU |
| `0xFFFFFFFE` | Connection refused (CR) | Wrong rack/slot or LOGO without TSAPs |

These are surfaced in the status display, the catch output, and (on multi-read) per-variable in `s7.variables[].error`.

### Type Coercion Table (Write)

| Incoming JSON type | Target `dataType` | Conversion |
|---|---|---|
| `boolean` | `bool` | direct |
| `string` `"true"`/`"false"`/`"1"`/`"0"` | `bool` | direct |
| `number` | `byte` / `word` / `dword` | range-checked unsigned integer; on overflow `BadOutOfRange` |
| `number` | `int` / `dint` | range-checked signed; INT is 16-bit, DINT 32-bit |
| `number` | `real` | IEEE 754 single-precision; loss of precision on values outside `±3.4e38` results in `BadOutOfRange` |
| `string` | `string` | encoded into the S7 STRING format (header byte `maxLen`, header byte `actualLen`, then bytes); truncation at `maxLen` with a warning logged once per node lifecycle |
| `[]number` | `raw` | each element as a byte (analogous to mqtt-out / modbus-write raw) |

### S7 STRING Encoding

S7 STRING is a structured type with two leading length bytes:

```
+--------+---------+----------+ ... +
| maxLen | actLen  | char[0]  |     |
+--------+---------+----------+ ... +
                   <- actLen bytes ->
                   <-     maxLen     ->
```

- `maxLen` is fixed by the address (`DB1.STRING50.20` ⇒ `maxLen=20`)
- `actLen` is set by the writer to the number of payload chars
- Reads return the substring `[0:actLen]` as a Go string
- Writes truncate the payload to `maxLen` if longer; trailing bytes within `maxLen` are zero-padded

This S7-specific string format is the most common encoding mistake in client libraries. The codec handles it transparently — the user only deals with `string` values.

### LOGO! / S7-200 Smart

LOGO! and S7-200 Smart use TSAP-based addressing instead of rack/slot. The connection type `logo` switches the dialog to TSAP mode and uses the `gos7` `SetConnectionParams` instead of `ConnectTo(host, rack, slot)`. Common defaults that should appear as a hint in the dialog:

- **LOGO! 0BA7**: localTSAP `0x0100`, remoteTSAP `0x0200`
- **LOGO! 0BA8**: localTSAP `0x0200`, remoteTSAP `0x0200`
- **S7-200 Smart**: rack=0, slot=1 (not LOGO mode — works with the standard connection)

The user picks the device family from a hint dropdown that pre-fills the TSAP fields; manual override remains available.

### "Test Connection" Endpoint

The endpoint opens a session with the supplied parameters, calls `GetCpuInfo` and `GetOrderCode`, and immediately closes again. Returns:

- on success: CPU type (e.g. `CPU 1516-3 PN/DP`), order code (e.g. `6ES7 516-3AN02-0AB0`), negotiated PDU size
- on failure: specific error message (library error or S7 error code)

This endpoint is **read-only** and without persistence — it does not touch `configs[]` in `workspace.json`.

## Tests

### Backend

| Test | Verifies |
|---|---|
| `TestS7AddressParseDB` | `DB10.DBX2.3`, `DB10.DBB4`, `DB10.DBW6`, `DB10.DBD0`, `DB1.STRING50.20` parse to the correct `S7DataItem` |
| `TestS7AddressParseFlags` | `M0.3`, `MB0`, `MW0`, `MD0` parse to area MK with correct `WordLen` |
| `TestS7AddressParseInputsOutputs` | `I0.0`, `IB1`, `Q0.0`, `QD4` parse to PE/PA |
| `TestS7AddressParseCountersTimers` | `C5`, `T3` parse to CT / TM |
| `TestS7AddressParseRejectsSymbolic` | `DB1.MyVariable` is rejected with the OPC UA hint |
| `TestS7CodecBool` | BOOL encode/decode against the correct bit position |
| `TestS7CodecInt` | INT/DINT signed range, two's complement |
| `TestS7CodecReal` | REAL = IEEE 754 single-precision, sample bit patterns |
| `TestS7CodecString` | STRING encode/decode with maxLen / actLen header |
| `TestS7CodecScaleOffset` | scale and offset are applied / inverted correctly |
| `TestS7PlcConnectS71500` | Connect / disconnect against `gos7server` test PLC for S7-1500 (rack=0, slot=1) |
| `TestS7PlcConnectS7300` | Connect / disconnect against test PLC for S7-300 (rack=0, slot=2) |
| `TestS7PlcReconnect` | TCP drop is detected, reconnect succeeds, state restored |
| `TestS7PlcMultiRead` | 50 variables in one request — bundling fits into PDU |
| `TestS7PlcMultiReadSplit` | 200 variables — split into multiple chunks, all results correct |
| `TestS7PlcSerialization` | Two parallel goroutines reading on the same PLC: requests are serialized, no interleaving |
| `TestS7ReadStatic` | Static mode with interval, multiple variables, output shape `object` |
| `TestS7ReadDynamic` | `msg.variables` overrides config; convenience form `msg.address` works |
| `TestS7ReadEmitOnChange` | Only emits when at least one value changes |
| `TestS7ReadOptimizedDB` | Returns `Item not available` per item; status red, error in output when `emitOnError=true` |
| `TestS7WriteStatic` | Static writes from `msg.payload` arrive at the PLC, ACK output is correct |
| `TestS7WriteDynamic` | Process `msg.variables` array |
| `TestS7WriteTypeMismatch` | string → real → `BadTypeMismatch` with field path |
| `TestS7WriteString` | S7 STRING encoding round-trip — read back equals written value |
| `TestS7WritePassthrough` | Input message is forwarded with `msg.s7Write` enriched |
| `TestS7TestConnectionHandler` | `/api/v1/s7/test-connection` returns CPU type + order code on a healthy PLC, error string on bad rack/slot |
| `TestS7SharedConnection` | Multiple nodes with the same PLC reference share one connection |
| `TestS7ReadBlockStatic` | Block mode with cyclic poll: emits `msg.payload = []byte` of the configured length, metadata in `msg.s7` |
| `TestS7ReadBlockOversized` | Block size > negotiated PDU is auto-split into multiple `AGReadArea` calls and concatenated; consumer sees one contiguous block |
| `TestS7ReadBlockTriggerOnInput` | With `triggerOnInput=true`, every input message produces an extra block read on top of the cyclic poll |
| `TestS7ReadBlockDynamicOverride` | `msg.s7.area` / `db` / `start` / `length` overrides the configured block per message |
| `TestS7WriteBlockStatic` | Block mode write: `msg.payload = []byte` is sent in one `AGWriteArea` call, contents read back match |
| `TestS7WriteBlockLengthMismatch` | When `block.length` is set and `msg.payload` length differs, the write is rejected with `BadTypeMismatch` |
| `TestS7WriteBlockInputProperty` | Custom `inputProperty=bytes` works (chains directly behind an `s7-parser` encode action) |

**Test PLC**: a dedicated Snap7-based demo PLC is maintained under [`demo/s7-server/`](../../demo/s7-server/) (Python + `python-snap7`, libsnap7 bundled in the wheel — no native dependency setup required). It pre-fills DB1 / DB10, the Merker area, and inputs/outputs with well-known values and animates a handful of "live" measurements so polling tests see motion. By default it listens on the un-privileged port `:1102` (the LOOPZE PLC config simply uses `port=1102` in tests). Tests in this issue connect to it via `LOOPZE_S7_TEST_HOST` / `LOOPZE_S7_TEST_PORT` env vars and **skip** when those are unset (consistent with the OPC UA test pattern). The demo speaks the standard Snap7 surface (Connect / COMM-Setup / ReadArea / WriteArea / ReadMultiVars / WriteMultiVars); advanced services like `GetCpuInfo` / `GetOrderCode` return placeholder strings from the embedded Snap7 server — the LOOPZE Test-Connection endpoint must tolerate placeholder content here. For real-CPU parity tests (Optimized DB rejection, firmware-specific quirks) PLCSIM Advanced or actual hardware is required and lives outside CI.

### Frontend

- Render tests for `S7NodeConfig.vue`, `S7PlcConfig.vue`
- Validation test for `S7AddressInput.vue` (accepts each supported form, rejects symbolic / nonsense)
- Connection-type switch correctly auto-fills rack/slot
- "+" button opens `S7PlcConfig.vue` and after save selects the new PLC in the dropdown

## Dependencies

- **MQTT issue (`NODE_MQTT.md`)** introduces the config node concept. S7 reuses:
  - `configs[]` section in `workspace.json`
  - `ConfigProvider` interface
  - lifecycle order (start config nodes before regular nodes / stop them after)
- **Modbus issue (`NODE_MODBUS.md`)** has hardened the pattern: server manager, request serialization, multi-variable read bundling. S7 follows the same blueprint, only the protocol changes
- **`PARSER_S7_NODE.md`** — companion issue introducing the declarative byte-block parser/encoder. The S7 read/write `block` mode in this issue is the producer/consumer counterpart. The parser issue can be implemented in parallel; v1 of NODE_S7 is functionally complete without it (block mode still emits raw bytes that can be decoded in a Function node), but the typical production flow assumes both are present
- **Catch node** for service-wide errors (connection drop) — already implemented
- **Central TLS storage (`CENTRAL_TLS_STORAGE.md`)**: not relevant for v1 — S7 over RFC1006 is unencrypted per protocol; S7 communication runs in protected OT networks or behind VPN. If "Secure S7" via TLS comes later (S7-1500 firmware ≥ V3 supports it), the cert reference plugs in as in the other connection nodes

## Out of Scope

- **Optimized DBs** (symbolic addressing on S7-1200/1500): not addressable via the S7 protocol — the user must un-tick `Optimized block access` on DBs that LOOPZE should access. Symbolic addresses are explicitly rejected by the parser with a hint towards OPC UA
- **Secure S7 / TLS encryption**: S7-1500 firmware ≥ V3 supports it. Out of scope for v1; the protocol is historically unencrypted, deployments belong in protected OT networks
- **PLC programming / online block download**: out of scope, this is engineering tool territory (TIA Portal / STEP 7), not what an edge runtime should do
- **Diagnostic buffer / SZL block reading** beyond `GetCpuInfo` for the test-connection: useful for advanced diagnostics, but a separate issue
- **Symbolic table import** (export from TIA Portal as XML / `.s7p` and use as autocomplete in the address input): convenient, but a separate issue. Workaround: copy the address from the PLC tag table by hand
- **Multi-CPU H-systems** (S7-400H redundancy): one config can only address one CPU; H-failover via two configs in different flows possible, transparent failover is a separate issue
- **PROFINET / PROFIBUS network discovery**: out of scope — the user enters the IP address; for a discovery feature `dcp` would be needed, separate
- **PUT/GET enable check on 1200/1500**: from S7-1500 onward "permit access via PUT/GET" must be activated on the CPU. LOOPZE cannot set this — operator's task. The error `Connection refused (CR)` in the test-connection points to this; the dialog shows a hint
- **Block-level operations** (read DB list, upload DB definition): not in v1 — the read/write node only operates on already-known addresses
- **Auto-coalescing of variables-mode reads into block fetches** (the "Phase 3" optimization): planned but not in v1. Detects clusters of individual variables in the same DB within a small byte gap and silently rewrites those reads as one `AGReadArea` + client-side slicing. Transparent — flows authored in variables mode would simply get faster on next deploy, no UI change needed. Deferred until block mode + the `s7-parser` node have shipped and seen real-world use, so the coalescer can be tuned against actual customer DB layouts. A config flag (`autoCoalesceBlockReads: bool`, default true once shipped) lets operators opt out for diagnostic comparisons
