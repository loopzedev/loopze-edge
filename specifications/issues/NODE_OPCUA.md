# Issue: OPC UA Nodes – Read / Subscribe / Write with Server Configuration

## Status: Done (v1)

**Implemented (Phase 0–7):**
- `opcua-server` config node with auto-reconnect, state watcher, test-connection endpoint
- `opcua-read` node: static / triggered / dynamic modes, output shapes single/array/object, per-item status codes
- `opcua-write` node: static / dynamic modes, type coercion with range checks, type cache, status pulses, ExtensionObject encoding via TypeResolver
- `opcua-subscribe` node: MonitoredItems with sampling/queue/deadband, recovery after reconnect, output shapes per-item/batch
- ExtensionObject handling: marker type registration against gopcua, schema-driven encode/decode for struct/enum/optional/union/nested/array
- Type resolver loads `DataTypeDefinition` with recursion guard, shared cache per server, reset on reconnect
- Address space browser: modal with lazy tree, NodeClass filter, multi/single select, detail pane with DataType info, integrated into all three operations nodes
- Tests against the externally maintained Deno OPC UA test server via `LOOPZE_OPCUA_TEST_ENDPOINT`

**Deliberately deferred (separate issues):**
- Cert pinning (TOFU): field present in the server config, fingerprint persistence + comparison not implemented
- Browse fallback for servers without `DataTypeDefinition` (1.03 server compatibility)
- Subscription sharing between nodes with identical PublishingInterval
- `/api/v1/opcua/read-attributes` and `/api/v1/opcua/resolve-path` endpoints (browse endpoint covers v1 UI needs)
- Browse pagination via ContinuationPoint in the frontend (backend already passes it through)

## Problem Description

OPC UA is **the** standard for machine communication in industry (machine tools, robots, PLCs, MES, SCADA, edge gateways). LOOPZE needs first-class OPC UA support to be deployable as a serious edge automation platform in industrial environments.

Three new node types — `opcua-read`, `opcua-subscribe`, `opcua-write` — cover the three central OPC UA operations. They reference a shared **OPC UA Server Config Node** (`opcua-server`) that bundles connection and security parameters. The config node concept was introduced with the MQTT issue ([NODE_MQTT.md](NODE_MQTT.md)) and is reused here.

**Guiding principle**: Apart from the server choice (static, fixed per node), **every** operation must also be controllable dynamically at runtime via input message — NodeIDs, values, subscription lists. An OPC UA node that can only be configured statically is not sufficient in practice, because real-world plants often have hundreds of variables whose selection is only known at runtime (via UI, recipe, job data, etc.).

**Scope**: OPC UA Binary (`opc.tcp://`) with standard security modes. Read, Subscribe (MonitoredItems), Write. **ExtensionObjects (structures) are first-class supported** — on read they are automatically converted into JSON objects, on write a JSON object is encoded back into an ExtensionObject based on the server-side DataTypeDefinition. Without this feature, OPC UA against real-world PLCs (Siemens UDTs, Beckhoff structures, B&R data types, AAS submodels, EUInformation/Range/AnalogItem configs) is not practically usable. **Not** in v1: Method Calls, HistoryRead, Browse-as-Node, Events/Alarms, Auto-Discovery, X.509 client cert generation in the UI. These come as separate issues afterward.

## Overview

| Node Type | Type ID | Canvas Inputs | Canvas Outputs | Description |
|---|---|---|---|---|
| **OPC UA Server** | `opcua-server` | — | — | Config node: endpoint, security, authentication |
| **OPC UA Read** | `opcua-read` | 0–1 | 1 | One-shot read of one or more nodes |
| **OPC UA Subscribe** | `opcua-subscribe` | 0–1 | 1 | MonitoredItems with push updates on value change |
| **OPC UA Write** | `opcua-write` | 1 | 0–1 | Write values to one or more nodes |

```
                          OPC UA Server (external, e.g. PLC/machine)
                          +--------------------------+
Flow                      |  ns=2;s=Temp.Sensor.1    |
+---------------------+   |  ns=2;s=Motor.Speed      |
|  [Inject 1s]         |   |  ns=2;s=Setpoint.Target  |
|      v               |   |                          |
|  [OPCUA Read] -------+---+ Read                     |
|      v               |   |                          |
|  [Function]          |   | Subscribe                |
|      v               +---+ (MonitoredItems)         |
|  [OPCUA Write] ------+---+ Write                    |
|                      |   |                          |
+---------------------+   +--------------------------+
```

## Requirements

### 1. Config Node: OPC UA Server (`opcua-server`)

The OPC UA Server is a **config node** — analogous to the MQTT broker. It does not appear on the canvas and is referenced by `opcua-read`, `opcua-subscribe`, and `opcua-write` nodes. Multiple operations nodes that reference the same server share **one** OPC UA session.

- **Type ID**: `opcua-server`
- **Configuration fields**:
  - `name` (string) — display name, e.g. "PLC Line 3"
  - `endpointUrl` (string) — e.g. `opc.tcp://192.168.1.50:4840`
  - `securityPolicy` (string) — `None` (default) | `Basic128Rsa15` | `Basic256` | `Basic256Sha256` | `Aes128_Sha256_RsaOaep` | `Aes256_Sha256_RsaPss`
  - `securityMode` (string) — `None` (default) | `Sign` | `SignAndEncrypt`. With `securityPolicy=None`, `securityMode=None` is enforced
  - `authMode` (string) — `anonymous` (default) | `username` | `certificate`
  - `username` (string, optional) — when `authMode=username`
  - `password` (string, optional) — when `authMode=username`
  - `clientCertFile` (string, optional) — path to the client certificate file (PEM/DER), required for `securityMode != None` or `authMode=certificate`
  - `clientKeyFile` (string, optional) — path to the private key
  - `applicationUri` (string) — default: `urn:loopze:client`. Must match the Subject AltName in the client cert, otherwise many servers refuse the connection
  - `applicationName` (string) — default: `LOOPZE OPC UA Client`
  - `sessionTimeout` (number, ms) — default: `60000`
  - `requestTimeout` (number, ms) — default: `5000`. Timeout for individual service calls (Read/Write/CreateSubscription)
  - `serverCertTrust` (string) — `prompt` (default in v1: log-only, accepted) | `pinned` (only accepts the cert with the stored fingerprint) | `system` (accepts everything in the OS truststore). v1 implements `prompt`/`pinned` — TOFU pattern: first cert is logged and can be pinned manually
  - `keepaliveInterval` (number, ms) — default: `10000`. Sends empty reads to keep the session alive

- **Connection & reconnect**:
  - On deploy, the session is established. Connection loss -> automatic reconnect with exponential backoff (1s, 2s, 4s, ..., max 30s)
  - With v5-equivalent session recovery: subscriptions and MonitoredItems are automatically restored from the client state (library: `gopcua/opcua` supports this via `subscription.Recreate()`)
  - Status during reconnect: yellow with "reconnecting…"

- **Properties dialog**:

```
+----------------------------------------------+
|  OPC UA Server                                |
+----------------------------------------------+
|                                               |
|  Name                                         |
|  [ PLC Line 3                             ]   |
|                                               |
|  Endpoint URL                                 |
|  [ opc.tcp://192.168.1.50:4840            ]   |
|                                               |
|  v Security                                   |
|  Policy:  [ None                        v ]   |
|  Mode:    [ None                        v ]   |
|                                               |
|  Authentication                               |
|  ( . ) Anonymous                              |
|  (   ) Username / Password                    |
|  (   ) Certificate                            |
|                                               |
|  Username   [                               ] |
|  Password   [ *****                         ] |
|                                               |
|  v Client Identity (advanced)                 |
|  Application URI  [ urn:loopze:client       ]  |
|  Application Name [ LOOPZE OPC UA Client    ]  |
|  Client Cert File [                        ]  |
|  Client Key  File [                        ]  |
|                                               |
|  v Timing (advanced)                          |
|  Session Timeout    [ 60000 ] ms              |
|  Request Timeout    [  5000 ] ms              |
|  Keepalive Interval [ 10000 ] ms              |
|                                               |
|  Server Cert Trust: [ Pin on first use   v ]  |
|                                               |
|  [ Test Connection ]                          |
|                                               |
|  [ Save ]   [ Cancel ]                        |
+----------------------------------------------+
```

The **Test Connection** button performs a one-shot CONNECT/CloseSession and reports success/error code (StatusCode from the server). Helps with configuring without deploy.

### 2. OPC UA Read Node (`opcua-read`)

- **Canvas**:
  - Static mode: 0 inputs, 1 output — reading can happen on a timer (via `interval`), once on startup, or not at all (then only usable as a sub-node of another trigger — see mixed mode below)
  - Triggered mode: 1 input, 1 output — every input message triggers a read
  - Dynamic mode: 1 input, 1 output — the NodeIDs to read come from the input message

- **Function**: Reads values of a list of OPC UA nodes via the `Read` service. Output is a message with the read value(s) plus metadata (StatusCode, timestamps).

- **Base configuration**:
  - `server` (string) — ID of the referenced `opcua-server` config node
  - `mode` (string) — `static` | `triggered` (default) | `dynamic`
  - `nodeIds` (string[]) — list of NodeIDs to read, e.g. `["ns=2;s=Temp", "ns=2;i=42"]`. In `dynamic` mode optional as default if `msg` provides nothing
  - `attribute` (string) — default: `Value`. Attribute to read. v1: only `Value` and `Description` are offered in the UI — other attributes (DisplayName, BrowseName, DataType, AccessLevel, ...) are possible via `msg.attribute`
  - `outputShape` (string) — `single` | `array` | `object` (see below)
  - `interval` (number, ms, only `static`) — read interval. `0` = once on startup only
  - `startupRead` (boolean, only `static`) — default `true`. Read immediately on deploy, not after `interval`

- **NodeID syntax** (standard OPC UA form):
  - `ns=<index>;i=<integer>` — numeric identifier
  - `ns=<index>;s=<string>` — string identifier
  - `ns=<index>;g=<guid>` — GUID identifier
  - `ns=<index>;b=<base64>` — opaque identifier (bytes)
  - Examples: `ns=2;s=Demo.Static.Scalar.Int32`, `ns=0;i=2258` (CurrentTime)

- **Output shape**:

| Shape | `msg.payload` form | When useful |
|---|---|---|
| `single` | scalar (only with exactly 1 NodeID) | Classic read on 1 value: `msg.payload = 23.5` |
| `array` (default for >1) | `[{nodeId, value, statusCode, sourceTimestamp, serverTimestamp}, …]` | Multiple values, configuration order matters |
| `object` | `{ "<nodeId>": <value>, … }` or `{ "<nodeId>": {value, statusCode, …}, … }` | Map access in the next node, NodeID as key |

  With `single`/`array`/`object` and reduced output (`includeMetadata=false`, default `true`), only the value is mapped directly. With metadata, StatusCode, SourceTimestamp, ServerTimestamp are added.

- **Incoming message** (Triggered and Dynamic mode):
  - `msg.action = "read"` — explicit action. Optional in triggered mode (every input triggers), required in dynamic mode
  - `msg.nodeIds` (string or string[]) — overrides the config NodeIDs for this read
  - `msg.attribute` (string) — overrides the attribute to read
  - In dynamic mode without `msg.nodeIds` and without default NodeIDs in the config, the read is skipped with a status warning (no output)

- **Outgoing message** (example with `outputShape=array`, `includeMetadata=true`):
  ```json
  {
    "payload": [
      {
        "nodeId": "ns=2;s=Temp",
        "value": 23.5,
        "dataType": "Double",
        "statusCode": "Good",
        "statusCodeRaw": 0,
        "sourceTimestamp": "2026-04-28T10:15:23.123Z",
        "serverTimestamp": "2026-04-28T10:15:23.124Z"
      },
      {
        "nodeId": "ns=2;s=MotorState",
        "value": {
          "Speed": 1450.5,
          "Torque": 12.3,
          "FaultCode": 0,
          "Running": true,
          "Mode": "Auto"
        },
        "dataType": "ExtensionObject",
        "structureType": "ns=2;i=3001",
        "structureName": "MotorStatusType",
        "statusCode": "Good",
        "statusCodeRaw": 0,
        "sourceTimestamp": "2026-04-28T10:15:23.123Z",
        "serverTimestamp": "2026-04-28T10:15:23.124Z"
      }
    ]
  }
  ```

  **ExtensionObjects** (server-side structures / UDTs) are automatically converted to JSON objects. The field names correspond to those of the DataTypeDefinition (see the "ExtensionObject Handling" section). Nested structures are converted recursively, arrays remain arrays. The original NodeID of the structure type and the plain-text name come along as `structureType` / `structureName` so the user knows what they are dealing with (and so a downstream write node does not have to lose the type info if the structure should be written back 1:1).

  For individual reads with bad status: `statusCode` as string (e.g. `"BadNodeIdUnknown"`), `value = null`. The read as a whole only throws a catchable error if the service call itself fails (connection lost, invalid session) — individual bad StatusCodes per NodeID are regular output data and are passed through.

- **Status display** (via `SetStatus`):
  - Green: `connected · <n> node(s)` (or `connected · idle` in dynamic without active reads)
  - Yellow: `connecting…` / `reconnecting…`
  - Red: `disconnected` or specific error message (service result)

- **Properties panel**:

```
+----------------------------------------------+
|  OPC UA Read                                  |
+----------------------------------------------+
|                                               |
|  Server                                       |
|  [ PLC Line 3                        v ] [+]  |
|  Edit server config                           |
|                                               |
|  Mode                                         |
|  ( ) Static (interval)                        |
|  (.) Triggered (msg in)                       |
|  ( ) Dynamic (msg.nodeIds)                    |
|                                               |
|  Node IDs                                     |
|  +-------------------------------------+ +-+  |
|  | Temperature  · ns=2;s=Temp          | |x|  |
|  +-------------------------------------+ +-+  |
|  +-------------------------------------+ +-+  |
|  | ns=2;i=42                           | |x|  |
|  +-------------------------------------+ +-+  |
|  [+ Add NodeID]   [ Browse server… ]          |
|                                               |
|  Attribute            [ Value           v ]   |
|                                               |
|  Output Shape         [ Array (default) v ]   |
|  [x] Include metadata (statusCode, timestamps)|
|                                               |
|  v Static-Mode Options (only if static)       |
|  Interval         [ 1000 ] ms                 |
|  [x] Read at startup                          |
|                                               |
+----------------------------------------------+
```

### 3. OPC UA Subscribe Node (`opcua-subscribe`)

The most important read path in practice: **push instead of poll**. The node creates an OPC UA subscription, registers MonitoredItems, and forwards every value change pushed by the server as an output message.

- **Canvas**:
  - Static mode: 0 inputs, 1 output
  - Dynamic mode: 1 input, 1 output — control messages on the input, data on the output (control messages are **not** passed through)

- **Base configuration**:
  - `server` (string) — ID of the `opcua-server` config node
  - `mode` (string) — `static` (default) | `dynamic`
  - `monitoredItems` (object[]) — per item:
    - `nodeId` (string)
    - `attribute` (string, default `Value`)
    - `samplingInterval` (number, ms, default `1000`) — how often the server samples internally. `-1` = server default, `0` = "as fast as possible"
    - `queueSize` (number, default `1`) — how many values the server buffers if updates arrive faster than the publish round
    - `discardOldest` (boolean, default `true`) — when queue full: drop oldest or newest
    - `deadband` (object, optional) — value-change filter:
      - `type`: `none` | `absolute` | `percent`
      - `value`: threshold (only for `absolute`/`percent`)
  - `publishingInterval` (number, ms, default `500`) — how often the subscription pushes data to the client
  - `lifetimeCount` (number, default `60`) — number of publish intervals without activity before the server marks the subscription as dead
  - `keepAliveCount` (number, default `10`) — number of publish intervals without data before an empty keep-alive is sent
  - `priority` (number, default `0`)
  - `outputShape` (string) — `per-item` (default) | `batch`:
    - `per-item`: each value change is emitted as its own message (`msg.nodeId`, `msg.payload = value`, `msg.statusCode`, `msg.sourceTimestamp`)
    - `batch`: all updates arriving within one PublishResponse are emitted as an array in `msg.payload`

- **Static mode**:
  - On deploy, subscription and all MonitoredItems are created
  - On value change the server pushes -> output message
  - On re-deploy / stop, the subscription is `DeleteSubscriptions`-ed and items are implicitly cleaned up

- **Dynamic mode**:
  - On deploy the node has **no** MonitoredItems, only an empty subscription waiting
  - Control messages on the input:
    - `msg.action = "subscribe"` with `msg.payload` as:
      - String -> one NodeID (default options from config)
      - String array -> multiple NodeIDs (default options)
      - Object or object array -> full `monitoredItems` specification analogous to the static config
    - **On every `subscribe`, all existing MonitoredItems are first removed**, then the new ones are created. Diff optimization (keep only unchanged items) is possible but not required for v1
    - `msg.action = "unsubscribe"` with `msg.payload` as string/string-array -> remove specific items without touching the rest
    - `msg.action = "clear"` -> remove all items
  - Empty array or empty string on `subscribe` acts like `clear`
  - Per-message override of subscription-wide defaults:
    - `msg.publishingInterval`, `msg.samplingInterval`, `msg.queueSize`, `msg.deadband` — apply to the items created with this control message

- **Status display**:
  - Green: `active · <n> items` (static) or `active · <n> items` / `active · idle` (dynamic)
  - Yellow: `connecting…`
  - Red: subscription error code (e.g. `BadTooManyMonitoredItems`)

- **Properties panel** (excerpt; the MonitoredItems editor is the central component):

```
+----------------------------------------------+
|  OPC UA Subscribe                             |
+----------------------------------------------+
|                                               |
|  Server   [ PLC Line 3               v ] [+]  |
|  Mode     ( . ) Static  ( ) Dynamic           |
|                                               |
|  Monitored Items                              |
|  +-----------------------------------------+  |
|  | = NodeID  [ ns=2;s=Temp           ]  x  |  |
|  |   Sampling [1000]ms  Queue [1] [x]oldest|  |
|  |   Deadband [none v]                     |  |
|  +-----------------------------------------+  |
|  | = NodeID  [ ns=2;s=Pressure       ]  x  |  |
|  |   Sampling [500]ms  Queue [10] [x]oldest|  |
|  |   Deadband [absolute v]  Value [0.5]    |  |
|  +-----------------------------------------+  |
|  [+ Add MonitoredItem]                        |
|                                               |
|  v Subscription Defaults                      |
|  Publishing Interval [ 500 ] ms               |
|  Lifetime Count      [  60 ]                  |
|  KeepAlive Count     [  10 ]                  |
|  Priority            [   0 ]                  |
|                                               |
|  Output Shape  [ Per-item (one msg per change) v ] |
|                                               |
+----------------------------------------------+
```

In dynamic mode, the MonitoredItems editor is replaced by a hint block:

```
i  Send msg.action = "subscribe" with msg.payload as
   NodeID string, array of NodeIDs, or array of
   {nodeId, samplingInterval, queueSize, deadband}.
   Existing items are replaced on each subscribe.
   Use action="unsubscribe" or "clear" to remove items.
```

### 4. OPC UA Write Node (`opcua-write`)

- **Canvas**:
  - 1 input, 1 output (optional, default 1) — the output carries the write result (StatusCode per NodeID), can be set to 0 for pure sink usage

- **Base configuration**:
  - `server` (string) — ID of the `opcua-server` config node
  - `mode` (string) — `static` (default) | `dynamic`
  - `writes` (object[], static) — per write:
    - `nodeId` (string)
    - `attribute` (string, default `Value`)
    - `dataType` (string) — `Boolean` | `SByte` | `Byte` | `Int16` | `UInt16` | `Int32` | `UInt32` | `Int64` | `UInt64` | `Float` | `Double` | `String` | `DateTime` | `ByteString` | `ExtensionObject` | `Variant`. Required because OPC UA is strictly typed and the server rejects writes with the wrong type
    - `structureType` (string, optional, only for `dataType=ExtensionObject`) — NodeID of the structure type, e.g. `ns=2;i=3001`. Needed so the node knows which DataTypeDefinition to encode the incoming JSON object against. If left empty + active type cache, the information is taken from a previous read
    - `valueSource` (string) — `static` | `msg`:
      - `static`: `value` (string or JSON, converted to the target data type)
      - `msg`: value is read from a message path (default: `payload`, alternatively `payload.<key>` etc.)
    - `valuePath` (string, only for `valueSource=msg`, default `payload`)
  - `passthrough` (boolean, default `false`) — input message is passed through (with enriched `msg.writeResult`) on the output. If `false`, a new message with the result goes out, provided the output count is > 0

- **Incoming message**:
  - **Static mode**: every input message triggers the writes defined in the config. Values from `msg` are extracted according to `valuePath`
  - **Dynamic mode**: `msg.writes` contains the write specification as an array of objects:
    ```json
    {
      "writes": [
        { "nodeId": "ns=2;s=Setpoint", "value": 42.5, "dataType": "Double" },
        { "nodeId": "ns=2;s=Mode",     "value": 3,    "dataType": "Int32"  },
        {
          "nodeId": "ns=2;s=MotorCmd",
          "dataType": "ExtensionObject",
          "structureType": "ns=2;i=3010",
          "value": {
            "TargetSpeed": 1500.0,
            "AccelRamp":   200,
            "Direction":   "CW",
            "EnableLimits": true
          }
        }
      ]
    }
    ```
  - Convenience form for a single write:
    ```json
    { "nodeId": "ns=2;s=Setpoint", "payload": 42.5, "dataType": "Double" }
    ```
    `dataType` is optional if the server-side type is known from a previous read and is cached (see "Type caching" below).

- **Type coercion**: Incoming values are converted into the target OPC UA type. Examples:
  - JSON number -> `Int32` / `Float` / `Double` (with range check, otherwise `BadOutOfRange`)
  - JSON string `"true"`/`"false"`/`"1"`/`"0"` -> `Boolean`
  - JSON string -> `DateTime` (RFC3339)
  - JSON array of numbers -> `ByteString` (for binary data, analogous to mqtt-out)
  - JSON object -> `ExtensionObject` (see "ExtensionObject Handling" below). Fields are mapped against the DataTypeDefinition; missing fields are filled with type defaults (0 / `false` / `""` / `null`); extra fields are ignored (with a warning in the log)

  On conversion errors: write is **not** issued, the error appears in the `writeResult` as `BadTypeMismatch` with a plain-text explanation.

- **Type caching** (optimization): On the first write to a NodeID, the node reads the server's `DataType` attribute and caches it. After that, `dataType` can be omitted in `msg.writes`. The cache is discarded on reconnect. Disableable in the UI via `disableTypeCache` (default `false`).

- **Outgoing message** (on the output, if present):
  ```json
  {
    "writeResult": [
      { "nodeId": "ns=2;s=Setpoint", "statusCode": "Good", "statusCodeRaw": 0 },
      { "nodeId": "ns=2;s=Mode",     "statusCode": "BadTypeMismatch", "statusCodeRaw": 2147614720 }
    ],
    "allGood": false
  }
  ```

  `allGood` is `true` if all writes returned status `Good` — practical for downstream switches.

- **Status display**:
  - Green: `ready · <n> writes` (static) / `ready · idle` (dynamic)
  - After write: brief pulse (yellow -> green) with status text `<m>/<n> ok`, on error: red with error message
  - Permanently red on connection loss

- **Properties panel** (Static mode):

```
+----------------------------------------------+
|  OPC UA Write                                 |
+----------------------------------------------+
|                                               |
|  Server   [ PLC Line 3               v ] [+]  |
|  Mode     ( . ) Static  ( ) Dynamic           |
|                                               |
|  Writes                                       |
|  +-----------------------------------------+  |
|  | = NodeID  [ ns=2;s=Setpoint        ] x  |  |
|  |   DataType [ Double            v ]      |  |
|  |   Source   [ msg.payload          ]     |  |
|  +-----------------------------------------+  |
|  | = NodeID  [ ns=2;s=Mode            ] x  |  |
|  |   DataType [ Int32             v ]      |  |
|  |   Source   [ static               ]     |  |
|  |   Value    [ 3                    ]     |  |
|  +-----------------------------------------+  |
|  [+ Add Write]                                |
|                                               |
|  [x] Pass message through with writeResult    |
|  [x] Cache DataType per NodeID                |
|                                               |
+----------------------------------------------+
```

## Shared Concepts

### Dynamic Control — Guiding Principle

Every operations node offers static **and** dynamic modes. The control messages follow a uniform convention:

| Node | `msg.action` | `msg.payload` / further fields |
|---|---|---|
| `opcua-read` | `"read"` (optional in triggered) | `msg.nodeIds` (string or string[]) |
| `opcua-subscribe` | `"subscribe"` / `"unsubscribe"` / `"clear"` | `msg.payload` as string, string array, or object array |
| `opcua-write` | implicit (every message is a write trigger) | `msg.writes` (array) or `msg.nodeId` + `msg.payload` (single) |

This uniformity makes it easy to control OPC UA operations from Function or Switch nodes, e.g. to read NodeID lists from a database or to source write values from a recipe loader.

### Server Connection Sharing

Multiple operations nodes with the same server reference share **one** OPC UA session. The `OpcuaServer` config node manages:

1. One OPC UA session (CreateSession / ActivateSession)
2. An optional subscription pool strategy:
   - **Variant A (v1)**: Each `opcua-subscribe` node creates its own subscription. Advantage: independent PublishingIntervals, isolated re-/deploy. Disadvantage: more subscriptions = more server load
   - **Variant B (later)**: Subscription sharing between nodes with the same PublishingInterval. Optimization, later
3. Read/write service calls are multiplexed onto the session (gopcua/opcua client is thread-safe)

```
[opcua-read    nodes=A]   --+
[opcua-subscribe items=B] --+-- Server "PLC Line 3" -- 1 OPC UA session
[opcua-write   nodes=C]   --+
```

### NodeID Validation & Browser

NodeIDs are validated **syntactically** on the frontend (regex). Existence on the server is **not** checked up front — this happens on the first read/write/subscribe and is reflected in the StatusCode. This makes the system robust against NodeIDs that only exist at runtime (e.g. dynamic items in aggregating servers).

In addition to manual entry, every operations node (Read, Subscribe, Write) offers an **address space browser** — see the "Address Space Browser" section below. NodeIDs can be navigated and selected interactively through the server address space.

### Status Codes

OPC UA has its own 32-bit StatusCode scheme (`Good = 0`, `BadXxx`, `UncertainXxx`). We always emit StatusCodes in two forms:

- `statusCode` (string) — speaking form: `"Good"`, `"BadNodeIdUnknown"`, `"UncertainSubNormal"`
- `statusCodeRaw` (number) — raw 32-bit value for programmatic processing

This way a downstream Switch node can match both `msg.statusCode == "Good"` and `msg.statusCodeRaw & 0xC0000000`.

### Catch Integration

Service-wide errors (connection lost, invalid session, timeout) throw a catchable error via the existing `Error` mechanism. Per-item bad StatusCodes are **not** catch errors — they are regular output data the downstream flow can evaluate.

### Credentials & Security

Username, password, and paths to cert/key are stored **in cleartext** in the config for now — analogous to MQTT v1. Encryption comes with the later credential system.

The OPC UA standard requires a client certificate for `securityMode != None`. v1: user must generate cert/key externally and enter the path. Later iteration: auto-generate self-signed in the data dir.

## Data Structure

### workspace.json

```json
{
  "flows": [
    {
      "id": "flow-1",
      "type": "tab",
      "label": "Machine Connection",
      "nodes": [
        {
          "id": "node-opcua-read-1",
          "type": "opcua-read",
          "name": "Read temperature",
          "x": 200, "y": 150, "z": "flow-1",
          "inputs": 0, "outputs": 1,
          "wires": [["node-debug-1"]],
          "config": {
            "server": "server-1",
            "mode": "static",
            "nodeIds": ["ns=2;s=Temp", "ns=2;i=42"],
            "attribute": "Value",
            "outputShape": "array",
            "includeMetadata": true,
            "interval": 1000,
            "startupRead": true
          }
        },
        {
          "id": "node-opcua-sub-1",
          "type": "opcua-subscribe",
          "name": "Machine status",
          "x": 200, "y": 250, "z": "flow-1",
          "inputs": 0, "outputs": 1,
          "wires": [["node-debug-1"]],
          "config": {
            "server": "server-1",
            "mode": "static",
            "monitoredItems": [
              {
                "nodeId": "ns=2;s=Pressure",
                "samplingInterval": 500,
                "queueSize": 10,
                "discardOldest": true,
                "deadband": { "type": "absolute", "value": 0.5 }
              }
            ],
            "publishingInterval": 500,
            "lifetimeCount": 60,
            "keepAliveCount": 10,
            "outputShape": "per-item"
          }
        },
        {
          "id": "node-opcua-write-1",
          "type": "opcua-write",
          "name": "Write setpoint",
          "x": 600, "y": 300, "z": "flow-1",
          "inputs": 1, "outputs": 1,
          "wires": [["node-debug-1"]],
          "config": {
            "server": "server-1",
            "mode": "static",
            "writes": [
              {
                "nodeId": "ns=2;s=Setpoint",
                "dataType": "Double",
                "valueSource": "msg",
                "valuePath": "payload"
              }
            ],
            "passthrough": true,
            "disableTypeCache": false
          }
        }
      ]
    }
  ],
  "configs": [
    {
      "id": "server-1",
      "type": "opcua-server",
      "name": "PLC Line 3",
      "config": {
        "endpointUrl": "opc.tcp://192.168.1.50:4840",
        "securityPolicy": "None",
        "securityMode": "None",
        "authMode": "anonymous",
        "applicationUri": "urn:loopze:client",
        "applicationName": "LOOPZE OPC UA Client",
        "sessionTimeout": 60000,
        "requestTimeout": 5000,
        "keepaliveInterval": 10000,
        "serverCertTrust": "pinned"
      }
    }
  ]
}
```

## Affected Files

### Backend – New Files

- `internal/nodes/opcua_server.go` — Config node: session lifecycle, reconnect, subscription pool, type cache. Wraps the `gopcua/opcua` client
- `internal/nodes/opcua_server_test.go` — End-to-end tests against an external OPC UA test server (Node.js / `node-opcua`, maintained under `demo/opcua-server/`). Tests skip when `LOOPZE_OPCUA_TEST_ENDPOINT` is not set
- `internal/nodes/opcua_read.go` — Read node with static/triggered/dynamic mode
- `internal/nodes/opcua_read_test.go`
- `internal/nodes/opcua_subscribe.go` — Subscribe node with MonitoredItem management
- `internal/nodes/opcua_subscribe_test.go`
- `internal/nodes/opcua_write.go` — Write node with type coercion and caching
- `internal/nodes/opcua_write_test.go`
- `internal/nodes/opcua_types.go` — Shared helpers: NodeID parsing, DataType mapping, StatusCode names, variant building
- `internal/nodes/opcua_extobj.go` — ExtensionObject <-> JSON conversion (encode + decode based on schema definition)
- `internal/nodes/opcua_extobj_test.go`
- `internal/nodes/opcua_typeresolver.go` — Type resolver: loads `DataTypeDefinition` attributes (or browse fallback) and caches the schema representation per server. Shared by Read/Subscribe/Write
- `internal/nodes/opcua_typeresolver_test.go`
- `internal/server/opcua_handlers.go` — REST handlers for the browse / read-attributes / resolve-path and test-connection endpoints. Falls back to a session pool optionally lent by the engine or a temporary session
- `internal/server/opcua_handlers_test.go`

### Backend – Adjustments

- `internal/server/server.go` — Extend `registerNodes()` with `opcua-server`, `opcua-read`, `opcua-subscribe`, `opcua-write`
- `internal/flow/engine.go` — No change needed, provided the config node lifecycle from the MQTT issue is already cleanly generic. Otherwise minor adjustments to the provider pattern

### Frontend – New Files

- `frontend/src/components/config/OpcuaServerConfig.vue` — Server dialog with all connection, security, and auth fields. "Test Connection" button with backend endpoint `POST /api/v1/opcua/test-connection`
- `frontend/src/components/config/OpcuaReadConfig.vue` — Read properties: server dropdown, mode switch, NodeID list, output shape, static options
- `frontend/src/components/config/OpcuaSubscribeConfig.vue` — Subscribe properties: MonitoredItem editor (drag & drop, similar to ChangeConfig), subscription defaults
- `frontend/src/components/config/OpcuaWriteConfig.vue` — Write properties: writes editor with DataType and source selection per row
- `frontend/src/components/config/shared/NodeIdInput.vue` — Reusable NodeID input with syntax validation (`ns=N;[isgb]=…`) and a "Browse" button next to the input field
- `frontend/src/components/config/shared/DataTypeSelect.vue` — Dropdown of OPC UA data types
- `frontend/src/components/config/shared/OpcuaBrowser.vue` — Address space browser modal: tree with lazy loading, filters (NodeClass, text search), detail panel with attribute view, multi/single select depending on caller
- `frontend/src/composables/useOpcuaBrowser.ts` — Tree state: lazy-loading cache per NodeID, expand state, selection state, pagination via ContinuationPoint

### Frontend – Adjustments

- `frontend/src/components/PropertyPanel.vue` — Dispatch for the four new node types
- `frontend/src/components/nodes/tokens.ts` — Tokens for `opcua-read` (input, blue), `opcua-subscribe` (input, blue-green), `opcua-write` (output, blue-orange)
- `frontend/src/types/flow.ts` — TypeScript types for the four new configs

### Backend – New API Endpoints

- `POST /api/v1/opcua/test-connection` — Body: full server config; response: `{ ok: true, serverInfo?: {…} }` or `{ ok: false, error: "<reason>" }`. Server info contains ApplicationName, server cert fingerprint (for pinning), endpoints
- `POST /api/v1/opcua/browse` — Browse a NodeID, returns children with metadata. Uses the active session of the server config, falls back to a temporary session
- `POST /api/v1/opcua/read-attributes` — All attributes of a NodeID for the browser detail panel
- `POST /api/v1/opcua/resolve-path` — BrowsePath lookup: list of BrowseNames -> NodeID. Useful for workflows in which NodeIDs should be reproducible from a semantic path

### Go Dependencies

- `github.com/gopcua/opcua` — OPC UA client library for Go. Currently the only production-ready pure-Go implementation. Supports Read, Write, Subscribe, Browse, all common security policies, and v1.04

**Test server**: a dedicated Deno-based OPC UA test server is maintained in parallel (separate project). Tests in this issue run against this server via the `LOOPZE_OPCUA_TEST_ENDPOINT` env variable.

## Technical Notes

### Library Choice: gopcua/opcua

`github.com/gopcua/opcua` is pure Go, has no CGO dependencies, and is actively maintained (latest release ~6 months ago, regular commits). It has no competition in the pure-Go ecosystem; the only alternative would be a wrapper around the C-based open62541 library, which would break our single-binary strategy. Therefore a clear choice.

### Read Service Mapping

```go
func (n *OpcuaReadNode) doRead(ctx context.Context, nodeIds []string, attr ua.AttributeID) ([]ReadResult, error) {
    req := &ua.ReadRequest{
        TimestampsToReturn: ua.TimestampsToReturnBoth,
        NodesToRead:        []*ua.ReadValueID{},
    }
    for _, nid := range nodeIds {
        parsed, err := ua.ParseNodeID(nid)
        if err != nil {
            // Collect as per-item error, NOT as service error
            continue
        }
        req.NodesToRead = append(req.NodesToRead, &ua.ReadValueID{
            NodeID:      parsed,
            AttributeID: attr,
        })
    }
    resp, err := n.session.Read(ctx, req)
    // ...
}
```

### Subscription Recovery After Reconnect

`gopcua/opcua` provides `subscription.Recreate()`. On reconnect, the server manager calls this for all active subscriptions — the library recreates subscription and MonitoredItems server-side without the operations nodes noticing. Important: with `cleanStart` behavior, the old subscription is explicitly discarded; otherwise we can leave ghost subscriptions on the server.

### Type Coercion Table (Write)

| Incoming JSON type | Target `dataType` | Conversion |
|---|---|---|
| `number` | `Int*`, `UInt*`, `Float`, `Double` | Range check; on overflow `BadOutOfRange` |
| `boolean` | `Boolean` | direct |
| `string` `"true"`/`"false"` | `Boolean` | direct |
| `string` ISO8601 | `DateTime` | `time.Parse(time.RFC3339, …)` |
| `string` | `String` | direct |
| `string` Hex/Base64 | `ByteString` | configurable (separate type hint `ByteStringHex` / `ByteStringBase64`) |
| `[]number` | `ByteString` | each element as a byte (analogous to mqtt-out) |
| `object` | `ExtensionObject` | recursive encoding against DataTypeDefinition (see next section) |
| `*` | `Variant` | OPC UA server decides based on the current value type |

### ExtensionObject Handling (Central Feature)

ExtensionObjects are the OPC UA vehicle for complex structures — UDTs in PLCs, AAS submodels, device configurations, EUInformation, Range, AnalogItem properties. In practice, **no** industrial OPC UA server is usable without them. LOOPZE must therefore be able to convert structures transparently between OPC UA and JSON.

**Read path: ExtensionObject -> JSON**

1. The server delivers a `*ua.ExtensionObject` with:
   - `TypeID` — NodeID of the encoding node (typically `<DataTypeNodeId>+Encoding+DefaultBinary`)
   - `Encoding` — `Binary` (default) or `XML`
   - `Value` — bytes
2. The `OpcuaServer` config node has a **type resolver** that, for the given `TypeID`, fetches the DataTypeDefinition from the server (`Read` on the `DataTypeDefinition` attribute of the corresponding DataType node) and builds an in-memory schema representation from it. Results are cached until reconnect.
3. Based on the schema, the byte stream is decoded and transferred field by field into a `map[string]any`:
   - Scalar fields -> JSON primitives (number, bool, string)
   - Nested structures -> recursive `map[string]any`
   - Arrays -> `[]any`
   - Enums -> string with the symbol name (mapping via `EnumDefinition`); additionally `<field>__raw` with the numeric value, in case the consumer needs the enum value programmatically
   - Optional fields (switches in OptionSets) -> simply absent from the JSON object when not set
   - Union types -> single-field object: `{ "<activeFieldName>": <value> }`
4. The output message also gets `structureType` (NodeID of the structure type) and `structureName` (BrowseName) as metadata.

**Write path: JSON -> ExtensionObject**

1. User provides a JSON object as value + `structureType` (NodeID) as a hint
2. The type resolver loads the DataTypeDefinition of the structure type (or uses cache)
3. Fields in the JSON are mapped against the definition:
   - **The order in the OPC UA encoding is dictated by the definition**, not by the JSON order — the user does not have to worry about it
   - **Missing fields** are filled with type defaults (0 / `false` / `""` / empty sub-structure). A warning is logged once per write-path lifecycle
   - **Extra fields** in the JSON are ignored + logged
   - **Type mismatches** in a sub-field (e.g. string where number is expected) -> `BadTypeMismatch` with path info (`field "Speed" expected Double, got string`)
   - **Enums**: the user may provide a string (symbol name) **or** a number (raw value). Both are accepted
4. The encoded byte array is passed to the write service as an ExtensionObject with the correct `TypeID`

**Discovery of the DataTypeDefinition**

The OPC UA standard from 1.04 onward requires the server to provide the structure machine-readably via the `DataTypeDefinition` attribute (`StructureDefinition` or `EnumDefinition`). The `gopcua/opcua` library implements the read from this attribute; LOOPZE uses it in the type resolver. Servers that do not provide DataTypeDefinition (old 1.03 servers, possibly simple OSS implementations) fall back to a browse-based discovery path:

- Browse `HasComponent` children of the DataType node
- Read `BrowseName` and `DataType` of each sub-field
- Build the definition recursively

The browse fallback is a workaround for fields; the user is shown a hint in the status text when it is active (`fallback type discovery`), because certain edge cases (optional fields, unions, enum symbol names) cannot be covered with it.

**Edge Cases & Limits**

- **`AbstractDataType`** as a field type (e.g. `BaseDataType`): comes through as raw `Variant` and on write is only writable via `Variant` override (user must know the concrete type)
- **Custom XML encodings**: in v1 only Binary; XML encoding is rejected with StatusCode `BadEncodingError`. XML is rare in industrial servers, can be added later
- **Recursive structures** (`StructA` contains `StructA`): supported, the type resolver prevents infinite loops via a visited set
- **Very large structures** (>1 MB): work, but default limits `MaxMessageSize` may need to be raised in the server config
- **Structures with binary custom encoding** without DataTypeDefinition (proprietary, older servers): not decodable — come through as `value: { "raw": "<base64-bytes>" }` together with a warning, so the user can at least forward them

**Type Cache Lifecycle**

The type cache is part of the `OpcuaServer` config node (i.e. per server connection):

- Entry is populated on the first read/write of a NodeID of this structure type
- Entry is **completely** discarded on reconnect (the server may have changed type versions)
- Entry is also discarded if a write fails with `BadTypeMismatch` and a hint of schema change — the next write fetches a fresh definition
- Per `OpcuaServer` one central cache -> shared across Read/Subscribe/Write nodes

### Address Space Browser (Central UX Feature)

NodeIDs in practice are often cryptic (`ns=4;s=|var|CODESYS Control Win V3.Application.GVL.bMotor1Run`) and nobody types them out from memory. The address space browser is therefore not a nice-to-have but the standard way to select variables for `opcua-read`, `opcua-subscribe`, and `opcua-write` in the UI.

**Invocation**

In every operations properties panel there is a button **"Browse server…"** next to the NodeID list editor. Prerequisite: a server is selected in the server dropdown. Click opens a modal with a tree view of the server address space.

**UI Layout**

```
+----------------------------------------------------------+
|  Browse · PLC Line 3                                 [x]  |
+----------------------------------------------------------+
|  [ Search browseName…              ]  Filter: [Vars v]   |
+----------------------------+-----------------------------+
|  v Objects                 |  Selected: ns=2;s=Temp       |
|    v Server                |                              |
|    v Devices               |  BrowseName    Temperature   |
|      v Line3 (Folder)      |  DisplayName   Temperature   |
|        > Motor1            |  NodeClass     Variable       |
|        v Sensors           |  DataType      Double         |
|          [x] Temperature   |  AccessLevel   ReadWrite      |
|          [x] Pressure      |  Description   Inlet temp     |
|          [ ] Humidity      |                              |
|          [ ] MotorState    |  Path                        |
|            (Struct)        |  /Objects/Devices/Line3/     |
|                            |  Sensors/Temperature         |
|                            |                              |
|  Loaded: 47 nodes          |                              |
+----------------------------+-----------------------------+
|  Selection: 2 variables                                   |
|  [ Add 2 to list ]   [ Add & close ]   [ Cancel ]         |
+----------------------------------------------------------+
```

**Behavior**

- **Lazy loading**: Children are fetched from the server only when a node is expanded (`Browse` service, one backend roundtrip per expand). This keeps the browser performant even against servers with millions of nodes
- **Default root**: `Objects` folder (NodeID `ns=0;i=85`) — this is where 99% of all user-relevant variables live. A "Show full address space" switch lets you switch to the `Root` folder to also see `Types`/`Views`
- **Filters**:
  - **NodeClass**: `All` | `Variables` (default for operations nodes) | `Variables + Folders` | `Methods` | `Objects`. The variable selection filters on NodeClass=Variable but leaves `Object`/`Folder` visible as containers in the tree so you can navigate through
  - **Search**: as the user types, filtering happens client-side over the currently loaded subtree. A server-wide search index does not exist in OPC UA — this is a comfort search over what is already loaded, not a full-text search. A hint in the search field makes that transparent
- **Multi-select** (Read and Subscribe): checkboxes in front of each variable node, any number selectable at once; transferred as a batch into the NodeID list
- **Single-select** (Write): radio selection, one variable
- **Detail panel** on the right: shows all relevant attributes of the currently focused node:
  - `BrowseName`, `DisplayName`, `NodeClass`
  - For variables: `DataType` (with plain-text name, e.g. `Double` or `MotorStatusType`), `ValueRank` (Scalar/Array/Matrix), `AccessLevel` (R/W/RW/Hist), `Description`
  - For structures: small schema preview (fields + types) from the DataTypeDefinition
  - Full path from `Objects` (rendered via the `BrowseName` chain)
- **Sorting**: Children alphabetically by `DisplayName`, folders/objects before variables (otherwise variables get lost in deep structures)
- **Read of already-present NodeIDs**: NodeIDs already in the list are visually marked in the tree (checkmark, dim/fade), so duplicate selections are avoided

**Selection Handover**

On "Add to list", an entry in the NodeID list of the operations node is created per selected variable. Carried metadata:

- `nodeId` — the mandatory field
- `displayName` — as the initial **display name** in the list, can be freely renamed (purely UI, not persisted to the backend except as an optional label)
- `dataType` — only on the **Write node** is the DataType automatically filled into the `dataType` field of the write specification, so the user does not have to look this information up in the server manual themselves. On Subscribe, `dataType` is kept as pure UI info, because the subscription does not need the type
- `structureType` — for variables with structure DataType, the structure NodeID is also taken over so ExtensionObject writes work without further clicks

**Backend API**

Three REST endpoints, all with the server config in the body (or with server ID + lookup against an already-active session):

```
POST /api/v1/opcua/browse
  Body: { serverConfig | serverId, nodeId, nodeClassFilter? }
  Response: {
    parent: { nodeId, browseName, displayName, nodeClass, hasChildren },
    children: [
      {
        nodeId: "ns=2;s=Temp",
        browseName: "Temperature",
        displayName: "Temperature",
        nodeClass: "Variable",
        dataType: { nodeId: "i=11", name: "Double", isStructure: false },
        valueRank: -1,
        accessLevel: "ReadWrite",
        hasChildren: false,
        description: "Inlet temp"
      },
      …
    ]
  }

POST /api/v1/opcua/read-attributes
  Body: { serverConfig | serverId, nodeId }
  Response: { all attributes for the detail panel }

POST /api/v1/opcua/resolve-path
  Body: { serverConfig | serverId, browsePath: ["Objects","Devices","…"] }
  Response: { nodeId, displayName, nodeClass, dataType, … }
```

Server-side, the endpoint uses the **existing session** of the server config node, if it is already deployed (shared pool in the engine lifecycle). If not (browser is used before the first deploy), the backend opens a **temporary session** exclusively for the browse operation and closes it after inactivity (60s timeout). This way browsing is always available — also in a freshly imported workspace that has never been deployed.

**Security**

The browse endpoints are authenticated like all other API endpoints (sessions/auth from the Auth V1 system). Anyone without access to the editor cannot browse either.

**Frontend Component**

- `frontend/src/components/config/shared/OpcuaBrowser.vue` — Modal with tree, filters, detail panel, selection logic. Instantiated by `OpcuaReadConfig.vue`, `OpcuaSubscribeConfig.vue`, `OpcuaWriteConfig.vue`. Multi/single select is configured via prop
- `frontend/src/composables/useOpcuaBrowser.ts` — Composable for the tree state (lazy-loading cache, expand/collapse state, selection)

**Edge Cases**

- **Connection lost during browse**: modal shows banner "connection lost — retry", tree remains visible at the last loaded state (no data loss)
- **Very large folders** (>1000 children): backend delivers paginated via `ContinuationPoint`; frontend automatically loads more on scroll-end, with "Loaded N of ?" indicator
- **Reference loops** (rare but possible in custom address spaces): frontend keeps a visited set per path and aborts re-expand — no infinite trees
- **Variables with `null` DataType** (broken server): come with a hint icon in the tree, are selectable, but the write node receives `dataType=Variant` as default with a TODO marker

### Status Codes as Strings

The library delivers `ua.StatusCode` as uint32. A mapping `statusCodeName(code) string` covers the ~200 standard codes; unknown codes are formatted as `Bad_0x<hex>`.

### "Test Connection" Endpoint

The endpoint opens a session with the supplied parameters, calls `Read(ServerStatus)`, and immediately closes again. Returns:

- on success: ApplicationName, build info, server cert fingerprint (SHA-256), available endpoints
- on failure: specific error message (library error or OPC UA StatusCode)

This endpoint is **read-only** and without persistence — it does not touch `configs[]` in `workspace.json`.

## Tests

### Backend

| Test | Verifies |
|---|---|
| `TestOpcuaServerConnect` | Session setup against in-process test server |
| `TestOpcuaServerReconnect` | Reconnect behavior on TCP drop |
| `TestOpcuaReadStatic` | Static mode with interval, multiple NodeIDs |
| `TestOpcuaReadDynamic` | NodeIDs from `msg.nodeIds`, override config |
| `TestOpcuaReadOutputShapes` | `single`, `array`, `object` shapes |
| `TestOpcuaReadBadStatus` | One NodeID returns `BadNodeIdUnknown`, others `Good` — both arrive in the output |
| `TestOpcuaSubscribeStatic` | MonitoredItems are created, value changes arrive on the output |
| `TestOpcuaSubscribeDynamic` | `subscribe`/`unsubscribe`/`clear` actions |
| `TestOpcuaSubscribeDeadband` | Absolute and percent deadband filter updates correctly |
| `TestOpcuaSubscribeRecovery` | After reconnect, items arrive again |
| `TestOpcuaWriteStatic` | Values from `msg.payload` are written, result output is correct |
| `TestOpcuaWriteDynamic` | Process `msg.writes` array |
| `TestOpcuaWriteTypeCoercion` | Number -> Int32, string -> Boolean, etc. |
| `TestOpcuaWriteTypeCache` | First write triggers type lookup, second uses cache |
| `TestOpcuaSharedSession` | Multiple nodes with the same server share one session |
| `TestOpcuaExtensionObjectRead` | Read of a structure -> JSON object with correct fields, `structureType` and `structureName` in the output |
| `TestOpcuaExtensionObjectNestedRead` | Nested structures are recursively mapped |
| `TestOpcuaExtensionObjectArrayRead` | Array of structures -> array of JSON objects |
| `TestOpcuaExtensionObjectEnumRead` | Enum field comes out as symbol name, `__raw` contains numeric value |
| `TestOpcuaExtensionObjectWrite` | JSON object -> ExtensionObject, server accepts (StatusCode Good) |
| `TestOpcuaExtensionObjectWriteMissingFields` | Missing fields filled with defaults, warning logged |
| `TestOpcuaExtensionObjectWriteExtraFields` | Extra fields are ignored |
| `TestOpcuaExtensionObjectWriteTypeMismatch` | String where number is expected -> `BadTypeMismatch` with field path |
| `TestOpcuaExtensionObjectRoundTrip` | Read -> Write of the same structure without modification: server state unchanged |
| `TestOpcuaTypeResolverCache` | DataTypeDefinition is loaded once, used multiple times |
| `TestOpcuaTypeResolverInvalidationOnReconnect` | Cache is discarded on reconnect |
| `TestOpcuaBrowseFallbackDiscovery` | Server without `DataTypeDefinition` attribute: browse-based discovery delivers a usable definition |
| `TestOpcuaBrowseHandler` | `/api/v1/opcua/browse` returns children with correct metadata |
| `TestOpcuaBrowseHandlerPagination` | ContinuationPoint pagination on large folders |
| `TestOpcuaBrowseHandlerTempSession` | Browse before deploy uses a temporary session, closes after inactivity |
| `TestOpcuaBrowseHandlerSharedSession` | Browse during deploy uses the active engine session |
| `TestOpcuaReadAttributesHandler` | `/api/v1/opcua/read-attributes` returns the full attribute set |
| `TestOpcuaResolvePathHandler` | BrowsePath is correctly resolved to the NodeID |

### Frontend

- Render tests for the four config components
- Validation test for `NodeIdInput` (accepts `ns=2;s=Foo`, rejects `Foo`)

## Dependencies

- **Config node concept**: already introduced with [NODE_MQTT.md](NODE_MQTT.md). The engine lifecycle extensions are reused here
- **`ValueTypeInput.vue`**: can be reused for static values in the Write node, if useful
- **Catch node**: for service-wide errors (connection drop) — already implemented

## Out of Scope / Not in Scope

**Deliberately not in v1**:

- **Browse-as-Node**: An `opcua-browse` node that lists children of a NodeID at runtime. Useful for discovery workflows, but a separate issue
- **Method Calls**: `opcua-call` node for the `Call` service. Needs its own argument mapping and output argument handling, separate issue
- **HistoryRead**: Reading historical values via the `HistoryRead` service. Separate issue
- **Events / Alarms & Conditions**: Subscribing to event notifier nodes. Separate issue, because event-type filtering needs its own UI
- **OPC UA server mode**: LOOPZE as an OPC UA **server**, not client. Completely different concept, future issue
- **Auto-discovery (LDS)**: Scan the discovery endpoint and list all servers. Practical benefit limited, later
- **Cert auto-generation in the UI**: Generate self-signed cert/key on first save. Would be nice, but path entry first
- **Multi-endpoint failover**: One server config with multiple endpoints and automatic failover. Common industrial requirement, but later
- **Subscription sharing between nodes**: Optimization via shared subscription with the same PublishingInterval — can be added later without breaking change
- **XML encoding for ExtensionObjects**: only binary encoding is supported; XML-encoded ExtensionObjects are rare and come later
- **OPC UA method output arguments with ExtensionObject**: irrelevant for v1, because method calls are not in scope at all
- **Writing `AbstractDataType` fields** without explicit Variant override: user must specify the concrete type
