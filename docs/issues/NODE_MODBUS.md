# Issue: Modbus Nodes – Read & Write with Server Configuration

## Status: Open

## Problem Description

After MQTT, the second-most-important industrial connector follows: **Modbus**. Two new node types (`modbus-read` and `modbus-write`) enable reading from and writing to Modbus devices. Analogous to the MQTT pattern, a **Modbus Server config node** (`modbus-server`) introduces the connection as a reusable entity — multiple nodes can reference the same server and share a single connection.

**Guiding principle of this issue**: All functions — function code, address, quantity, data type, unit ID — can be controlled both **statically in the node config** and **dynamically per message**. **Only the server target (host/port or serial parameters) is exclusively static** and is maintained once in the config node.

**Scope**: Modbus TCP and Modbus RTU (serial). ASCII is out of scope. The common function codes (FC1–FC6, FC15, FC16) are supported. Multi-register data types (INT32, FLOAT32, …) are read with configurable byte and word order, since the order is not standardized in practice.

## Overview

| Node type | Type ID | Canvas inputs | Canvas outputs | Description |
|---|---|---|---|---|
| **Modbus Read** | `modbus-read` | 0 or 1 | 1 | Reads coils / discrete inputs / holding registers / input registers |
| **Modbus Write** | `modbus-write` | 1 | 0 or 1 | Writes coils or holding registers |

```
                        Modbus device (e.g. PLC, energy meter)
                        ┌─────────────────────────┐
Flow A                  │  Holding Reg 40001..n   │
┌────────────────────┐  │  Coil       00001..n    │
│  [Inject every 1s] │  │  Discrete   10001..n    │
│        ↓           │  │  Input Reg  30001..n    │
│  [Modbus Read] ←──────┤                          │
│        ↓           │  └─────────────────────────┘
│  [Debug]           │
└────────────────────┘                  ↑
                                         │
Flow B                                   │
┌────────────────────┐                   │
│  [Inject]          │                   │
│        ↓           │                   │
│  [Modbus Write] ──────────────────────┘
└────────────────────┘
```

## Requirements

### 1. Config Node: Modbus Server (`modbus-server`)

The Modbus Server is a **config node** (see NODE_MQTT.md – the concept is reused here) — it does not appear on the canvas and is referenced by `modbus-read` / `modbus-write` nodes.

- **Type ID**: `modbus-server`
- **No canvas element** — purely configurative
- **Configuration fields**:
  - `name` (string) — display name, e.g. "PLC Hall 1"
  - `transport` (string) — `tcp` (default) or `rtu`
  - **TCP fields** (valid when `transport=tcp`):
    - `host` (string) — hostname or IP
    - `port` (number) — default: 502
  - **RTU fields** (valid when `transport=rtu`):
    - `serialPort` (string) — e.g. `/dev/ttyUSB0`, `COM3`
    - `baudRate` (number) — 9600, 19200, 38400, 57600, 115200; default: 9600
    - `dataBits` (number) — 7 or 8; default: 8
    - `parity` (string) — `none`, `even`, `odd`; default: `none`
    - `stopBits` (number) — 1 or 2; default: 1
  - **Common fields**:
    - `timeout` (number) — request timeout in milliseconds; default: 1000
    - `idleTimeout` (number, TCP only) — seconds after which an unused TCP connection is closed; default: 60. `0` = never close
    - `defaultUnitId` (number) — default unit/slave ID when a node does not set its own value; default: 1
    - `reconnectBackoff` (number) — seconds between reconnect attempts after a connection loss; default: 5

- **Access to the properties dialog**:
  - **New server**: via the "+" button next to the server dropdown in Modbus nodes
  - **Edit existing server**: via the "Edit server config" link below the dropdown

- **Properties dialog**:

```
┌──────────────────────────────────────────────┐
│  Modbus Server                                │
├──────────────────────────────────────────────┤
│                                               │
│  Name                                         │
│  ┌────────────────────────────────────────┐   │
│  │ PLC Hall 1                             │   │
│  └────────────────────────────────────────┘   │
│                                               │
│  Transport                                    │
│  ( • ) TCP    ( ) RTU (Serial)                │
│                                               │
│  ── TCP ─────────────────────────────────────│
│  Host                          Port            │
│  ┌──────────────────────────┐ ┌──────────┐   │
│  │ 192.168.1.50             │ │ 502      │   │
│  └──────────────────────────┘ └──────────┘   │
│                                               │
│  ── RTU (greyed out in TCP mode) ────────────│
│  Serial Port                                  │
│  ┌────────────────────────────────────────┐   │
│  │ /dev/ttyUSB0                           │   │
│  └────────────────────────────────────────┘   │
│  Baud   [9600 ▼]   Data Bits [8 ▼]            │
│  Parity [none ▼]   Stop Bits [1 ▼]            │
│                                               │
│  ── Common ──────────────────────────────────│
│  Timeout (ms):       [1000]                   │
│  Idle Timeout (s):   [60] (TCP only)          │
│  Default Unit ID:    [1]                      │
│  Reconnect (s):      [5]                      │
│                                               │
│  ┌────────────┐  ┌────────────┐              │
│  │   Save     │  │  Cancel    │              │
│  └────────────┘  └────────────┘              │
└──────────────────────────────────────────────┘
```

### 2. Modbus Read Node (`modbus-read`)

- **Canvas**:
  - Static mode (cyclic poll): 0 inputs, 1 output
  - Dynamic mode (on demand): 1 input, 1 output — the input triggers the read
- **Function**: Reads the configured address range from a Modbus device and emits the result as a flow message. In dynamic mode all parameters can be overridden via `msg`.

- **Base configuration**:
  - `server` (string) — ID of the referenced `modbus-server` config node
  - `mode` (string) — `static` (default) or `dynamic`
  - `unitId` (number) — Modbus unit/slave ID. If empty, `defaultUnitId` from the server is used
  - `fc` (number) — function code:
    - `1` — Read Coils (1 bit, RW)
    - `2` — Read Discrete Inputs (1 bit, RO)
    - `3` — Read Holding Registers (16 bit, RW) — **default**
    - `4` — Read Input Registers (16 bit, RO)
  - `address` (number) — start address, **0-based** (40001 → 0). A UI toggle allows switching to 1-based input; internally the value is always stored 0-based
  - `quantity` (number) — number of coils or registers to read. Default: 1
  - `dataType` (string) — how the raw block is interpreted:
    - `raw` (default) — `[]uint16` for FC3/FC4, `[]bool` for FC1/FC2
    - `bool` — single `bool` (FC1/FC2 only, `quantity` must be 1)
    - `int16` / `uint16` — a single register as signed/unsigned
    - `int32` / `uint32` / `float32` — two registers (see byte/word order)
    - `int64` / `uint64` / `float64` — four registers
    - `string` — `quantity` registers as ASCII/UTF-8 string (2 chars per register, null terminator is trimmed)
  - `byteOrder` (string) — `bigEndian` (default) or `littleEndian` — byte order **within** a register
  - `wordOrder` (string) — `bigEndian` (default, "ABCD") or `littleEndian` ("CDAB") — order **of multiple** registers for 32/64-bit types. In practice there are devices using all four combinations
  - `scale` (number, optional) — multiplicative scaling factor; useful e.g. when an energy meter delivers watts in hundredths (`/100`)
  - `offset` (number, optional) — additive offset; applied **after** scaling

- **Polling (static mode)**:
  - `pollInterval` (number) — interval in milliseconds between reads. Default: 1000
  - `emitOnChange` (boolean) — if `true`, only emits when the value changes (useful for slowly changing values). Default: `false`
  - `emitOnError` (boolean) — if `true`, emits an error message on a read failure (`msg.error` set, `msg.payload` empty). If `false` (default), only logs internally and sets the status to red — no error output

- **Dynamic mode** — all fields can be overridden via `msg` (missing field → config default):
  - `msg.unitId` (number)
  - `msg.fc` (number 1–4)
  - `msg.address` (number)
  - `msg.quantity` (number)
  - `msg.dataType` (string)
  - `msg.byteOrder` (string)
  - `msg.wordOrder` (string)
  - An input message without `msg.action` triggers a read with the effective parameters. Incoming messages are **not** passed through to the output — the output contains only the read result (or the error when `emitOnError`)

- **Outgoing message** (on successful read):
  ```json
  {
    "payload": 23.7,
    "topic": "modbus/plc-hall-1/40001",
    "bytes": [66, 49, 153, 154],
    "modbus": {
      "fc": 3,
      "address": 0,
      "quantity": 2,
      "dataType": "float32",
      "unitId": 1,
      "raw": [16968, 13107]
    }
  }
  ```
  - `msg.payload` — the decoded value (scalar, array, string, bool — depending on `dataType`)
  - `msg.topic` — default `modbus/<server-name>/<address>`; user-overridable
  - `msg.bytes` — always set for FC3/FC4: `[]int` of wire bytes (uint8, 0..255), suitable for `Buffer.from(msg.bytes)` in the Function node. Redundant with `msg.modbus.raw` (uint16 words) — the user picks based on the use case. Not set for FC1/FC2 (coils already arrive as `[]bool`)
  - `msg.modbus` — metadata block with the effective read parameters and the raw register values (for debugging and round-trip scenarios)

- **Status display**:
  - Green: `connected · <interval>` (static) or `connected · idle` (dynamic)
  - Yellow: `connecting…` / `reconnecting…`
  - Red: error message with Modbus exception code (e.g. `Illegal Data Address (0x02)`)

- **Properties panel**:

```
┌──────────────────────────────────────────────┐
│  Modbus Read                                  │
├──────────────────────────────────────────────┤
│                                               │
│  Server                                       │
│  ┌────────────────────────────────┐ ┌───┐    │
│  │ PLC Hall 1                 ▼  │ │ + │    │
│  └────────────────────────────────┘ └───┘    │
│  Edit server config                           │
│                                               │
│  Mode                                         │
│  ( • ) Static (poll)   ( ) Dynamic (on input) │
│                                               │
│  Unit ID:  [1]   (empty = server default)     │
│                                               │
│  Function Code                                │
│  ┌────────────────────────────────────────┐   │
│  │ FC3 — Read Holding Registers       ▼  │   │
│  └────────────────────────────────────────┘   │
│                                               │
│  Address:    [0]    ☐ 1-based input           │
│  Quantity:   [2]                              │
│                                               │
│  Data Type:  [float32                      ▼] │
│  Byte Order: [Big Endian (default)         ▼] │
│  Word Order: [Big Endian (ABCD, default)   ▼] │
│                                               │
│  Scale:  [1]   Offset: [0]                    │
│                                               │
│  ── Polling (Static only) ──────────────────│
│  Interval: [1000] ms                          │
│  ☐ Emit on change                             │
│  ☐ Emit on error                              │
│                                               │
└──────────────────────────────────────────────┘
```

In dynamic mode the polling fields are hidden and a hint block is shown:

```
ℹ Send any message to trigger a read. Override
   any field via msg.fc / msg.address /
   msg.quantity / msg.dataType / msg.unitId.
```

### 3. Modbus Write Node (`modbus-write`)

- **Canvas**:
  - 1 input
  - 0 outputs (default — sink)
  - **Optional**: 1 output when `emitAck=true` — emits an ACK message after a successful write (for downstream confirm logic)
- **Function**: Writes incoming messages to the Modbus device. All parameters (FC, address, data type, unit ID) are configurable both statically and overridable per `msg`.

- **Base configuration**:
  - `server` (string) — ID of the referenced `modbus-server` config node
  - `unitId` (number, optional) — default unit/slave ID
  - `fc` (number) — function code:
    - `5` — Write Single Coil
    - `6` — Write Single Register
    - `15` — Write Multiple Coils
    - `16` — Write Multiple Holding Registers — **default**
  - `address` (number) — start address, 0-based
  - `dataType` (string) — analogous to `modbus-read`. Determines how `msg.payload` is encoded into registers/coils before writing
  - `byteOrder` (string), `wordOrder` (string) — analogous
  - `scale` (number, optional), `offset` (number, optional) — applied **inversely** to the read path: `register = (payload - offset) / scale`
  - `emitAck` (boolean) — if `true`, the node enables an output and emits a confirmation after a successful write. Default: `false`

- **Incoming message**:
  - `msg.payload` — the value to be written. The type must match the configured / `msg`-passed `dataType`:
    - `bool` for FC5 / `dataType=bool`
    - `number` for `int16/uint16/int32/uint32/int64/uint64/float32/float64`
    - `[]bool` for FC15 (Multiple Coils)
    - `[]number` for FC16 with `dataType=raw` or for multiple values of the same type
    - `string` for `dataType=string`
  - **Override fields** (analogous to read):
    - `msg.unitId` (number)
    - `msg.fc` (number 5/6/15/16)
    - `msg.address` (number)
    - `msg.dataType` (string)
    - `msg.byteOrder`, `msg.wordOrder` (string)

- **Outgoing message** (only when `emitAck=true`):
  ```json
  {
    "payload": true,
    "modbus": {
      "fc": 16,
      "address": 100,
      "quantity": 2,
      "dataType": "float32",
      "unitId": 1,
      "written": [16968, 13107]
    }
  }
  ```
  - `msg.payload` is `true` on success
  - `msg.modbus.written` contains the raw registers actually sent on the bus

- **Error behavior**:
  - On Modbus exception (e.g. `Illegal Function`, `Illegal Data Address`, `Slave Device Failure`): status set to red, error message contains the exception code. If a **Catch node** is present in the flow, the error is routed via the catch path (analogous to other nodes — see `CATCH_NODE.md`)
  - On TCP connection loss: reconnect runs, write is acknowledged with `Server unavailable`

- **Properties panel**:

```
┌──────────────────────────────────────────────┐
│  Modbus Write                                 │
├──────────────────────────────────────────────┤
│                                               │
│  Server                                       │
│  ┌────────────────────────────────┐ ┌───┐    │
│  │ PLC Hall 1                 ▼  │ │ + │    │
│  └────────────────────────────────┘ └───┘    │
│  Edit server config                           │
│                                               │
│  Unit ID:  [1]   (empty = server default)     │
│                                               │
│  Function Code                                │
│  ┌────────────────────────────────────────┐   │
│  │ FC16 — Write Multiple Registers    ▼  │   │
│  └────────────────────────────────────────┘   │
│                                               │
│  Address:    [100]                            │
│                                               │
│  Data Type:  [float32                      ▼] │
│  Byte Order: [Big Endian (default)         ▼] │
│  Word Order: [Big Endian (ABCD, default)   ▼] │
│                                               │
│  Scale:  [1]   Offset: [0]                    │
│                                               │
│  ☐ Emit ACK on success                        │
│                                               │
│  ℹ  msg.fc / msg.address / msg.dataType /     │
│     msg.unitId override the config            │
│     per message. msg.payload carries the val. │
│                                               │
└──────────────────────────────────────────────┘
```

### 4. Server Connection Sharing

When multiple Modbus nodes reference the same server, **one connection** is shared:

```
[modbus-read  fc=3 addr=0]   ──┐
[modbus-read  fc=3 addr=10]  ──┤── Server "PLC Hall 1" ── 1 TCP / 1 Serial
[modbus-write fc=16 addr=100]──┘
```

The engine provides a **server manager** that:
1. Manages one connection per `modbus-server` ID (TCP socket or open serial port)
2. Serializes read and write requests from the nodes (Modbus is half-duplex — concurrent requests on a single connection are not allowed; the manager queues requests)
3. Automatically reconnects on connection loss with the configured backoff
4. Cleanly closes all connections on stop / re-deploy

**Per-server serialization**: In the Modbus protocol only one transaction at a time may run on a connection. Multiple parallel read polls on the same server are processed serially by the manager — either via mutex or request queue. The server manager is **per server config**, not per node — so serialization spans across all nodes that share this server.

**Polling stagger**: When multiple read nodes with the same `pollInterval` poll the same server, their tick times should be offset (round-robin) so the load is distributed evenly and no spikes build up. Optimization — not required for v1.

## Data Structure

### workspace.json

```json
{
  "flows": [
    {
      "id": "flow-1",
      "type": "tab",
      "label": "PLC Hall 1",
      "nodes": [
        {
          "id": "node-modbus-read-1",
          "type": "modbus-read",
          "name": "Boiler Temperature",
          "x": 200,
          "y": 150,
          "z": "flow-1",
          "inputs": 0,
          "outputs": 1,
          "wires": [["node-debug-1"]],
          "config": {
            "server": "server-1",
            "mode": "static",
            "unitId": 1,
            "fc": 3,
            "address": 0,
            "quantity": 2,
            "dataType": "float32",
            "byteOrder": "bigEndian",
            "wordOrder": "bigEndian",
            "scale": 1,
            "offset": 0,
            "pollInterval": 1000,
            "emitOnChange": false,
            "emitOnError": false
          }
        },
        {
          "id": "node-modbus-write-1",
          "type": "modbus-write",
          "name": "Set Setpoint",
          "x": 600,
          "y": 300,
          "z": "flow-1",
          "inputs": 1,
          "outputs": 0,
          "wires": [],
          "config": {
            "server": "server-1",
            "unitId": 1,
            "fc": 16,
            "address": 100,
            "dataType": "float32",
            "byteOrder": "bigEndian",
            "wordOrder": "bigEndian",
            "scale": 1,
            "offset": 0,
            "emitAck": false
          }
        }
      ]
    }
  ],
  "configs": [
    {
      "id": "server-1",
      "type": "modbus-server",
      "name": "PLC Hall 1",
      "config": {
        "transport": "tcp",
        "host": "192.168.1.50",
        "port": 502,
        "timeout": 1000,
        "idleTimeout": 60,
        "defaultUnitId": 1,
        "reconnectBackoff": 5
      }
    }
  ]
}
```

## Affected Files

### Backend – New Files

- `internal/nodes/modbus_server.go` — Modbus Server config node: encapsulates the Modbus client (TCP or RTU), reconnect logic, request serialization
- `internal/nodes/modbus_read.go` — Read node: polling loop (static) or input trigger (dynamic), decoding into the target data type
- `internal/nodes/modbus_write.go` — Write node: encoding `msg.payload` into registers/coils, Modbus request via the shared server
- `internal/nodes/modbus_codec.go` — helper functions: byte/word order handling, scalar encoding/decoding, string ↔ register conversion. Independently testable (table-driven tests)

### Backend – Changes

- `internal/server/server.go` — registration of `modbus-read` and `modbus-write` in `registerNodes()`
- `internal/flow/engine.go` — extension of the config node lifecycle (introduced with MQTT) for `modbus-server`. The config node type is detected automatically; if the generalization there is solid, no Modbus-specific changes are required
- `internal/flow/registry.go` — if `ConfigProvider` from the MQTT issue already exists, it is reused here
- `internal/storage/` — no changes needed (provided the MQTT issue already introduced the `configs` section)

### Frontend – New Files

- `frontend/src/components/config/ModbusNodeConfig.vue` — shared config component for `modbus-read` and `modbus-write`: server dropdown + "+", mode selector, FC, address, dataType, byte/word order, scale/offset; the read- vs. write-specific fields via conditional rendering
- `frontend/src/components/config/ModbusServerConfig.vue` — server config dialog: transport switch (TCP/RTU), associated fields, timeout, default unit ID

### Frontend – Changes

- `frontend/src/components/PropertyPanel.vue` — dispatch for `modbus-read` and `modbus-write` to `ModbusNodeConfig`
- `frontend/src/components/nodes/tokens.ts` — already present: `modbus-read` → input (green), `modbus-write` → output (orange). No change needed
- `frontend/src/stores/flowStore.ts` — if already extended by the MQTT issue with config-node CRUD: nothing. Otherwise generalize there
- `frontend/src/types/flow.ts` — TypeScript types for `ModbusServerConfig`, `ModbusReadConfig`, `ModbusWriteConfig`

### Go Dependencies

- `github.com/goburrow/modbus` — established Go library with support for TCP, RTU, and ASCII. Actively maintained, used by many industrial projects
- `go.bug.st/serial` (transitively via `goburrow/modbus`) — cross-platform serial support for RTU

## Technical Notes

### Function Codes – Overview

| FC | Operation | Address space | Data type | Read/Write |
|----|-----------|-----------|----------|--------------|
| 1  | Read Coils | 00001..0xxxx | bit | RW (read) |
| 2  | Read Discrete Inputs | 10001..1xxxx | bit | RO |
| 3  | Read Holding Registers | 40001..4xxxx | 16-bit | RW (read) |
| 4  | Read Input Registers | 30001..3xxxx | 16-bit | RO |
| 5  | Write Single Coil | 00001..0xxxx | bit | WO |
| 6  | Write Single Register | 40001..4xxxx | 16-bit | WO |
| 15 | Write Multiple Coils | 00001..0xxxx | bit | WO |
| 16 | Write Multiple Registers | 40001..4xxxx | 16-bit | WO |

The **0-based address** equals the vendor-specific notation minus 1 (40001 → 0). Via the UI toggle (`1-based input`) the user can directly enter the 1-based addresses commonly found in datasheets — internally the value is always stored 0-based.

### Byte and Word Order

Modbus generally transmits 16-bit registers in **big endian** (high byte first). For multi-register values (32/64 bit) the order of the registers is **not standardized** — four combinations are encountered in practice:

```
Float32 = 0x12345678 is transmitted depending on the device as:

ABCD (Big BE / Big WO, default):     [0x1234, 0x5678]
CDAB (Big BE / Little WO):           [0x5678, 0x1234]
BADC (Little BE / Big WO):           [0x3412, 0x7856]
DCBA (Little BE / Little WO):        [0x7856, 0x3412]
```

The codec in the read and write path must support all four combinations. With data type `raw` the registers are passed through **unchanged** — byte/word order only matters for the typed variants.

### Polling Loop (Static Mode)

```go
func (n *ModbusReadNode) startPolling(ctx context.Context) {
    ticker := time.NewTicker(time.Duration(n.pollInterval) * time.Millisecond)
    defer ticker.Stop()

    var lastPayload any
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            value, raw, err := n.server.Read(n.unitId, n.fc, n.address, n.quantity)
            if err != nil {
                n.SetStatus(StatusError, err.Error())
                if n.emitOnError {
                    n.send(0, errorMessage(err))
                }
                continue
            }
            decoded := n.codec.Decode(value, raw)
            if n.emitOnChange && reflect.DeepEqual(decoded, lastPayload) {
                continue
            }
            lastPayload = decoded
            n.send(0, n.buildMessage(decoded, raw))
            n.SetStatus(StatusOk, fmt.Sprintf("connected · %dms", n.pollInterval))
        }
    }
}
```

### Dynamic-Mode Read

```go
func (n *ModbusReadNode) OnInput(msg Message) {
    fc       := pickInt(msg.Get("fc"), n.fc)
    address  := pickInt(msg.Get("address"), n.address)
    quantity := pickInt(msg.Get("quantity"), n.quantity)
    unitId   := pickByte(msg.Get("unitId"), n.unitId)
    dataType := pickString(msg.Get("dataType"), n.dataType)

    raw, err := n.server.Read(unitId, fc, address, quantity)
    if err != nil {
        n.SetStatus(StatusError, err.Error())
        if n.emitOnError {
            n.send(0, errorMessage(err))
        }
        return
    }
    decoded := n.codec.DecodeAs(dataType, raw)
    n.send(0, n.buildMessage(decoded, raw))
}
```

Incoming messages are **not** forwarded — the output contains only the read result. Control fields are checked; `msg.payload` is ignored (a read does not need an input payload).

### Request Serialization in the Server Manager

```go
type ModbusServer struct {
    client modbus.Client       // goburrow/modbus
    mu     sync.Mutex          // serializes all requests of this server
    // …
}

func (s *ModbusServer) Read(unitId byte, fc, addr, qty int) ([]byte, error) {
    s.mu.Lock()
    defer s.mu.Unlock()

    s.handler.SlaveId = unitId   // goburrow sets SlaveId per request on the handler
    switch fc {
    case 1:  return s.client.ReadCoils(uint16(addr), uint16(qty))
    case 2:  return s.client.ReadDiscreteInputs(uint16(addr), uint16(qty))
    case 3:  return s.client.ReadHoldingRegisters(uint16(addr), uint16(qty))
    case 4:  return s.client.ReadInputRegisters(uint16(addr), uint16(qty))
    default: return nil, fmt.Errorf("unsupported read FC: %d", fc)
    }
}
```

The mutex ensures that only one Modbus transaction at a time runs on the bus — across multiple nodes that share the same server.

### Reconnect Strategy

- On TCP connection loss `goburrow/modbus` closes the socket. The server manager detects this on the next request via the error return and attempts a reconnect after `reconnectBackoff` seconds
- During the reconnect attempt all pending read/write requests are blocked or fail with `Server unavailable` (configurable — for now: block until timeout)
- Status set to yellow (`reconnecting…`) during the attempt, red when the reconnect fails permanently

### Modbus Exception Codes

The Modbus protocol defines its own exception codes that the slave returns as an error response:

| Code | Meaning | Typical cause |
|------|-----------|------------------|
| 0x01 | Illegal Function | Slave does not support this FC |
| 0x02 | Illegal Data Address | Address outside the valid range |
| 0x03 | Illegal Data Value | Value outside the range (e.g. quantity too large) |
| 0x04 | Slave Device Failure | Slave has an internal error |
| 0x05 | Acknowledge | Long-running operation, retry later |
| 0x06 | Slave Device Busy | Slave currently not addressable |

These exceptions are surfaced in the status display and the catch output with code and meaning — essential for commissioning.

## Dependencies

- **MQTT issue (`NODE_MQTT.md`)** introduces the config node concept in the engine. Modbus builds on this and reuses:
  - `configs[]` section in `workspace.json`
  - `ConfigProvider` interface
  - lifecycle order (start config nodes before regular nodes / stop them after)
- If the MQTT issue is not yet merged, the generic parts from this issue must be introduced together in the Modbus PR — should be structurally identical
- Frontend tokens for `modbus-read` / `modbus-write` are already defined in `tokens.ts`

## Out of Scope

- **Modbus ASCII**: not in v1 (rarely used in production anymore)
- **Modbus mapping file** (e.g. CSV with tags such as `temperature=40001:float32`): may come later as a separate feature; for now each read node is a standalone address block
- **Address discovery / browse**: Modbus has no discovery protocol like OPC UA — the user must take addresses from the device datasheet
- **Multi-slave routing via RTU gateway**: a Modbus TCP gateway can bundle multiple RTU slaves; each slave is addressed via `unitId`. This works transparently with the current design — separate server configs per gateway, unit ID per node
- **Encryption (Modbus Secure)**: out of scope — Modbus is historically unencrypted; in protected OT networks or behind VPN
- **Function codes outside 1–6, 15, 16** (e.g. FC20/21 File Record, FC23 Read/Write Multiple): not in v1 — rarely needed in industry
- **Bit fields in holding registers** (e.g. bit 3 of register 40005): not in v1; can currently be solved with `dataType=uint16` and a downstream Function node — addressed declaratively by the planned **Modbus Parser node** (`PARSER_MODBUS_NODE.md`)
- **Declarative register layouts** (multi-field mapping "address → type → name" per device): covered by the separate **Modbus Parser node**, see [`PARSER_MODBUS_NODE.md`](./PARSER_MODBUS_NODE.md). The parser sits between `modbus-read` (`raw` output) and consumer or between producer and `modbus-write` (`raw` input) — no changes to read/write required
