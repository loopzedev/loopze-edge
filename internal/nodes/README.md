# Adding a New Node — Go Backend

This document explains how to implement and register a new node type in the backend.

## Node architecture

Every node implements the `flow.NodeInstance` interface defined in `internal/flow/registry.go`:

```go
type NodeInstance interface {
    Init() error
    SetSend(fn flow.SendFunc)
    SetStatus(fn flow.StatusFunc)
    SetDebug(fn flow.DebugFunc)
    Start() error
    HandleMessage(msg *Message) ([][]*Message, error)
    Stop() error
}
```

The engine calls these in order: `Init()` → optional capability injection → `Start()` → `HandleMessage()` (per message) → `Stop()`.

## Optional capabilities

If your node needs extra runtime services, implement one or more of these interfaces. The engine detects them via type-switch and injects after `Init()`:

| Interface | Method | Use case |
|---|---|---|
| `flow.ConfigProvider` | `SetConfigLookup(fn)` | Access a config node (e.g. MQTT broker, S7 PLC) |
| `flow.ErrorProvider` | `SetError(fn)` | Report async background errors (disconnect, timeout) |
| `flow.HTTPMuxProvider` | `SetHTTPMux(mux)` | Register HTTP endpoints (e.g. `http-in`) |
| `flow.SessionRegistryProvider` | `SetSessionRegistry(r)` | Track TCP sessions across nodes |
| `flow.CertStoreProvider` | `SetCertStore(s)` | Resolve TLS certificate references |
| `flow.StatusListenerProvider` | `SetStatusListener(register)` | Observe status events of other nodes |

## Step-by-step: adding a new node type

### 1. Create the implementation file

Create `internal/nodes/<protocol>_<role>.go` (e.g. `bacnet_read.go`).

Minimal skeleton:

```go
package nodes

import (
    "github.com/loopzedev/loopze-edge/internal/flow"
)

type BACnetReadNode struct {
    send   flow.SendFunc
    status flow.StatusFunc
    debug  flow.DebugFunc
    // your config fields parsed from NodeConfig
}

func NewBACnetReadNode(cfg flow.NodeConfig) (flow.NodeInstance, error) {
    n := &BACnetReadNode{}
    // parse cfg.Properties here
    return n, nil
}

func (n *BACnetReadNode) Init() error                                            { return nil }
func (n *BACnetReadNode) SetSend(fn flow.SendFunc)                               { n.send = fn }
func (n *BACnetReadNode) SetStatus(fn flow.StatusFunc)                           { n.status = fn }
func (n *BACnetReadNode) SetDebug(fn flow.DebugFunc)                             { n.debug = fn }
func (n *BACnetReadNode) Start() error                                           { return nil }
func (n *BACnetReadNode) Stop() error                                            { return nil }
func (n *BACnetReadNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
    return nil, nil
}

// BACnetReadTypeInfo returns palette metadata shown in the editor.
func BACnetReadTypeInfo() flow.NodeTypeInfo {
    return flow.NodeTypeInfo{
        Type:        "bacnet-read",
        Label:       "BACnet Read",
        Category:    "input",
        Description: "Reads properties from a BACnet device.",
        Inputs:      0,
        Outputs:     1,
    }
}
```

### 2. Add tests

Create `internal/nodes/<protocol>_<role>_test.go`. See `s7_read_test.go` or `mqtt_in_test.go` for patterns.

```go
func TestBACnetReadNode_ParsesConfig(t *testing.T) { ... }
func TestBACnetReadNode_HandleMessage(t *testing.T) { ... }
```

### 3. Register the node

Open `internal/server/server.go` and add one line in `registerNodes()`:

```go
registry.Register("bacnet-read", nodes.NewBACnetReadNode, nodes.BACnetReadTypeInfo())
```

For config nodes (shared connection managers like `mqtt-broker`, `s7-plc`):

```go
registry.RegisterConfig("bacnet-device", nodes.NewBACnetDevice, nodes.BACnetDeviceConfigTypeInfo())
```

### 4. Add a protocol-specific API handler (optional)

If your node needs auxiliary API endpoints (test-connection, browse, diagnostics), create `internal/api/<protocol>_handlers.go` and mount the routes in `internal/api/routes.go`.

See `s7_handlers.go` and `opcua_handlers.go` for examples.

### 5. Add a demo server (optional)

If a real hardware device is not available for CI testing, add a simulator under `demo/<protocol>-server/`. See `demo/s7-server/` or `demo/modbus-server/` for examples.

## Protocol node patterns

For nodes that communicate with industrial devices, follow the established layering:

```
<protocol>_plc.go       — config node (manages connection + reconnect)
<protocol>_address.go   — address syntax types
<protocol>_codec.go     — wire encoding/decoding (bytes ↔ Go values)
<protocol>_parser.go    — address string → internal descriptor
<protocol>_read.go      — polling / subscription source node
<protocol>_write.go     — sink node
```

See `s7_*.go` or `modbus_*.go` for a complete implementation of this pattern.

## Verify

```bash
go build ./...
go test ./internal/nodes/...
go vet ./...
```
