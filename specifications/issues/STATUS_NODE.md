# Issue: Status Node — Emit status events of other nodes as messages

## Status: Implemented

## Problem description

Currently, node status (`n.status(fill, text)`) is purely **visual feedback** on the node itself — a colored dot with text. Status changes cannot be processed programmatically in the flow.

This means a whole class of use cases is missing:

- **Watchdog**: If an MQTT-In node turns `red`, an alert should be sent automatically through an MQTT-Out node or a function
- **Status aggregation**: Combine the status of several critical nodes into a central topic / dashboard
- **Bridge to the outside**: Push node status as an MQTT message or webhook into an external monitoring system
- **Reactive flows**: Other nodes (e.g. State Machine, Function) should react to status changes, not just to the payload

In Node-RED this is solved by the **Status node**: It sits in the flow, subscribes to status events of other nodes, and emits each status as a message on its own output.

## View / rationale

The groundwork already exists:

- **Status pipeline** (`NODE_STATUS_PIPELINE.md`): Engine → NATS `status.<flowID>.<nodeID>` → WebSocket → frontend
- **Status cache** (`NODE_STATUS.md`): `engine.NodeStatuses()` holds the last status per node
- **Standard node pattern**: `internal/nodes/*.go` clearly shows how a node implements `Init` / `Start` / `HandleMessage` / `Stop`

A Status node is essentially a **status subscriber node**. Architecture decision: It does not hook into NATS but into a new **engine-internal fan-out** (see Technical sketch) — fits the existing provider interfaces (`LinkProvider`, `ContextProvider`, `ConfigProvider`) and avoids a NATS round-trip per status event.

## Requirements

### 1. Scope configuration

The operator chooses in the properties panel **which status events** the node picks up:

| Value | Label | Behavior |
|---|---|---|
| `flow` | Current flow | Status of all nodes in the same flow as the Status node (default) |
| `selected` | Selected nodes | Status of only an explicit selection of nodes — strictly limited to the own flow |
| `all` | All flows | Status of all nodes system-wide |

- Default: `flow` — the most common expectation and matches Node-RED.
- `selected`: Multi-select with checkbox list. Selection is stored as a set of node IDs in `config.targetNodes` (`string[]`). The list contains **only nodes of the current flow**, without the Status node itself and without other Status nodes (it cannot observe those anyway).

**Status nodes never receive status from other Status nodes** — regardless of scope. This architecturally rules out infinite loops, and a Status node is "invisible" to other Status nodes. Filtering happens centrally in the engine fan-out, not in the individual node — see Technical sketch.

**Double protection for selected mode**: The backend filter additionally checks `FlowID == config.FlowID`, even if the frontend accidentally supplied a foreign node ID or the node later moves to another flow. Selection stays strictly flow-local.

### 2. Output message

Per received status event **one message** is emitted:

```json
{
  "status": {
    "fill": "green",
    "text": "42 msgs",
    "source": {
      "id": "<nodeID>",
      "type": "mqtt-in",
      "name": "Sensor Living Room",
      "flowId": "<flowID>"
    }
  },
  "payload": "42 msgs"
}
```

- `msg.status` contains the full status object including source (analogous to Node-RED `msg.status`)
- `msg.payload` contains the `text` — convenient for direct MQTT-Out / Debug processing without an additional Change node
- `source.name` is resolved from the node name (`config.name`) — fallback: empty string when unknown
- `source.type` enables filtering in the flow (e.g. only evaluate `mqtt-in` status)

### 3. Filter (optional, Phase 2)

- **Only on status change**: Checkbox "Emit only on change" — the node remembers the last status per source node and emits a message only if `fill` or `text` changed. Prevents floods on `count` status with every message.
- **Only on `fill` value**: Multi-select over the five status colors (`red`, `green`, `yellow`, `blue`, `grey`). Default: all.

Phase 1 implements **neither** — first observe whether the need arises.

### 4. No input

The Status node has **no input handle**, only an output. Unlike e.g. Inject, it is not triggered by incoming messages but reacts solely to status events.

### 5. Own status

The Status node itself shows a brief status in the frontend:

- On start: `green` / `"listening"`
- On every output: `grey` / `"<source.name>: <fill>"` for ~2 seconds, then back to `green`/`"listening"` (truncate to 32 characters)
- The blink sequence is protected against race conditions via `atomic.Uint64` — on rapid updates an older reset does not overwrite a newer blink

Since Status nodes are filtered out in the engine fan-out, these own status updates do not re-trigger other Status nodes.

### 6. Hover highlight in multi-select

When hovering over an item in the selected-nodes checkbox list, the corresponding node in the flow editor is marked with a **dashed outline** (1px dashed in accent color). This makes it possible to tell at a glance during configuration which node corresponds to which list entry — no "Which mqtt-in was 'Sensor Living Room' and which was 'Sensor Bedroom'?".

The mechanism already existed in the DebugPanel (`hoveredDebugNodeId` with `outline: dashed` on `BaseNode`). For the second usage it was renamed to a generic name (`hoveredHighlightNodeId` / `setHoveredHighlightNodeId` / `isHighlighted`) so it no longer semantically suggests "Debug hover" only. DebugPanel and StatusConfig consume the same state.

## Technical sketch

### Architecture decision: Engine-internal fan-out

The engine is explicitly designed so that nodes **know nothing** about NATS — external communication runs exclusively via engine callbacks (`SetSend`, `SetStatus`, `SetDebug`) and provider interfaces (`ContextProvider`, `LinkProvider`, `ConfigProvider`). The Status node follows this pattern: It receives status events via a new `StatusListener` mechanism from the engine, instead of listening to NATS itself.

| | Engine fan-out (chosen) | Node subscribes to NATS directly |
|---|---|---|
| Architecture consistency | fits LinkProvider/ContextProvider | breaks "nodes know no NATS" |
| Latency | Go function call (ns) | NATS round-trip (μs), JSON round-trip |
| Data duplication | no — same `StatusMessage` struct | yes — marshal/unmarshal again |
| Lifecycle | engine cleans up listener on stop | node must coordinate unsubscribe itself |

### Backend — `internal/flow/registry.go`

`StatusMessage` extended by `SourceType` and `SourceName` (backward compatible via `omitempty`):

```go
type StatusMessage struct {
    NodeID     string            `json:"nodeId"`
    FlowID     string            `json:"flowId"`
    Status     NodeStatusPayload `json:"status"`
    SourceType string            `json:"sourceType,omitempty"`
    SourceName string            `json:"sourceName,omitempty"`
}
```

`NodeConfig` extended by `FlowID` so nodes know their own flow (for the scope filter):

```go
type NodeConfig struct {
    ID         string         `json:"id"`
    Type       string         `json:"type"`
    Name       string         `json:"name"`
    FlowID     string         `json:"flowId"`
    Properties map[string]any `json:"properties"`
}
```

New provider interface analogous to `LinkProvider`:

```go
type StatusListenerFunc func(msg StatusMessage)

type StatusListenerProvider interface {
    SetStatusListener(register func(StatusListenerFunc) (unregister func()))
}
```

### Backend — `internal/flow/engine.go`

Engine holds a map of registered listeners under `sync.RWMutex`:

```go
type Engine struct {
    // ... existing ...
    statusListenersMu sync.RWMutex
    statusListenerSeq uint64
    statusListeners   map[uint64]StatusListenerFunc
}

func (e *Engine) registerStatusListener(fn StatusListenerFunc) func() {
    e.statusListenersMu.Lock()
    e.statusListenerSeq++
    id := e.statusListenerSeq
    e.statusListeners[id] = fn
    e.statusListenersMu.Unlock()
    return func() {
        e.statusListenersMu.Lock()
        delete(e.statusListeners, id)
        e.statusListenersMu.Unlock()
    }
}

func (e *Engine) fanoutStatus(msg StatusMessage) {
    e.statusListenersMu.RLock()
    defer e.statusListenersMu.RUnlock()
    for _, fn := range e.statusListeners {
        fn(msg)
    }
}
```

`makeStatusFunc` additionally calls `fanoutStatus` — but **Status nodes themselves are excluded from the fan-out**, so no Status node ever receives the status of another Status node:

```go
func (e *Engine) makeStatusFunc(nodeID string, rn *runningNode) StatusFunc {
    return func(fill string, text string) {
        msg := StatusMessage{
            NodeID:     nodeID,
            FlowID:     rn.flowID,
            Status:     NodeStatusPayload{Fill: fill, Text: text},
            SourceType: rn.config.Type,
            SourceName: rn.config.Name,
        }
        e.statusCache.Set(nodeID, msg)
        if rn.config.Type != "status" {
            e.fanoutStatus(msg)
        }
        if e.publishStatus != nil {
            subject := fmt.Sprintf("status.%s.%s", rn.flowID, nodeID)
            e.publishStatus(subject, msg)
        }
    }
}
```

Filtering in the fan-out (instead of in the Status node itself) has two advantages: It is DRY (one place, applies to all future Status nodes / listeners) and saves the closure calls entirely — with many Status nodes, no O(n) effort per status event of another Status node.

In `wireAllNodes`, provider injection analogous to `LinkProvider`:

```go
for _, rn := range e.nodes {
    if slp, ok := rn.instance.(StatusListenerProvider); ok {
        slp.SetStatusListener(e.registerStatusListener)
    }
}
```

`instantiateNode` populates `NodeConfig.FlowID` from the `runningNode.flowID`.

### Backend — `internal/nodes/status.go` (new)

```go
type StatusNode struct {
    config flow.NodeConfig
    send   flow.SendFunc
    status flow.StatusFunc
    debug  flow.DebugFunc

    scope       string              // "flow", "selected" or "all"
    targetNodes map[string]struct{} // populated when scope == "selected"

    register   func(flow.StatusListenerFunc) func()
    unregister func()

    blinkSeq atomic.Uint64
    mu       sync.Mutex
    started  bool
}

func (n *StatusNode) SetStatusListener(register func(flow.StatusListenerFunc) func()) {
    n.mu.Lock()
    defer n.mu.Unlock()
    n.register = register
    // Re-wire on modified-nodes deploy: drop old subscription
    if n.started {
        if n.unregister != nil { n.unregister() }
        n.unregister = register(n.handleStatus)
    }
}

func (n *StatusNode) handleStatus(sm flow.StatusMessage) {
    switch n.scope {
    case "flow":
        if sm.FlowID != n.config.FlowID { return }
    case "selected":
        // Double protection: selection is always flow-local
        if sm.FlowID != n.config.FlowID { return }
        if _, ok := n.targetNodes[sm.NodeID]; !ok { return }
    case "all":
        // no filter
    }
    msg := flow.NewMessage()
    msg.Set("status", map[string]any{
        "fill": sm.Status.Fill,
        "text": sm.Status.Text,
        "source": map[string]any{
            "id": sm.NodeID, "type": sm.SourceType,
            "name": sm.SourceName, "flowId": sm.FlowID,
        },
    })
    msg.SetPayload(sm.Status.Text)
    n.send(0, msg)
    n.blink(sm)
}
```

The Status node has `HandleMessage` as a no-op (returns `nil, nil`) — it is source-only, but the engine's `nodeLoop` stays active for it and blocks on its empty `inputCh` until `stopCh` closes.

### Frontend — `frontend/src/components/nodes/StatusNode.vue` (new)

- Own Vue component analogous to `InjectNode.vue` — only output handle, no input
- Body display shows the scope: `this flow` / `<N> nodes` / `all flows`

### Frontend — `frontend/src/components/config/StatusConfig.vue` (new)

- Scope dropdown (`flow` / `selected` / `all`) via `useNodeProperty`
- On `selected`: scrollable checkbox list of candidates
  - Source: `flowStore.activeNodes` — automatically only the current flow
  - Filter: without the selected Status node itself, without other Status nodes (type filter)
  - Sorted by label, display `<label> (<type>)`
  - Toggle persists `targetNodes: string[]` in the node config
- On hover over an item: `flowStore.setHoveredHighlightNodeId(id)` → flow shows dashed outline
- `onBeforeUnmount` resets the hover state

### Frontend — Hover highlight generalization

`flowStore`:
- `hoveredDebugNodeId` → `hoveredHighlightNodeId`
- `setHoveredDebugNodeId` → `setHoveredHighlightNodeId`

`BaseNode.vue`:
- `isDebugHovered` → `isHighlighted`
- Outline style binding adjusted

`DebugPanel.vue`: setter calls renamed (mouseenter/mouseleave + `onBeforeUnmount`).

Both consumers (DebugPanel, StatusConfig) use the same mechanism — no code duplication.

### Frontend — Node palette

A new entry appears **automatically** under "Common", because the palette is loaded via `api.getNodes()` from the backend registry (`StatusTypeInfo` with `Category: "common"`). No additional frontend mapping needed.

### Frontend — Icon

`NodeIcon.vue` already had the `status` entry (heartbeat-wave path) registered for some time — no additional entry needed.

## Affected files

### Backend
- `internal/flow/registry.go` — `StatusListenerFunc`, `StatusListenerProvider`, `StatusMessage` extended by `SourceType`/`SourceName`, `NodeConfig` extended by `FlowID`
- `internal/flow/engine.go` — `statusListeners` map + mutex + sequence, `registerStatusListener`, `fanoutStatus`, call in `makeStatusFunc` (with `Type != "status"` filter), provider injection in `wireAllNodes`, `FlowID` populated in `instantiateNode`
- `internal/nodes/status.go` (new) — `StatusNode` source-only with scope `flow`/`selected`/`all`, re-wire safe, idle status + 2s blink
- `internal/server/server.go` — registration `registry.Register("status", nodes.NewStatusNode, nodes.StatusTypeInfo())`

### Frontend
- `frontend/src/components/nodes/StatusNode.vue` (new) — 0/1 ports, scope display in body
- `frontend/src/components/config/StatusConfig.vue` (new) — scope dropdown + multi-select checkbox list with hover highlight
- `frontend/src/views/FlowEditor.vue` — import + `<template #node-status>`
- `frontend/src/components/PropertyPanel.vue` — `<StatusConfig>` for `type === 'status'`
- `frontend/src/stores/flowStore.ts` — hover state renamed (generic)
- `frontend/src/components/nodes/BaseNode.vue` — highlight computed renamed, style binding adjusted
- `frontend/src/components/DebugPanel.vue` — setter calls renamed

## Dependencies

- **Status pipeline** must be running (`NODE_STATUS_PIPELINE.md`) — otherwise no status events arrive at the engine fan-out
- **Status cache** (`NODE_STATUS.md`) is **not** mandatory — the Status node only needs live events, no replay. On start it has, by definition, an empty state, which is accepted
- Multi-select accesses `flowStore.activeNodes` — the list of nodes of the active flow is already available there

## Out of scope for Phase 1

- **Filter "only on change"** — first observe whether floods occur
- **Filter by `fill` value** — rarely needed, can be solved by a downstream Switch node
- **Status source = another Status node** — explicitly excluded, infinite loops architecturally prevented
- **Persistent subscription across server restart** — on restart, all running statuses are gone anyway
- **Status replay** when the Status node starts (would emit the last cached status of all source nodes as initial messages) — conceivable, but semantically unclean (replay vs. live events)
- **Selected mode across flows** — multi-select is strictly flow-local. Whoever wants cross-flow takes `scope: all` and filters downstream in the flow via Switch node on `msg.status.source.flowId`

## Open questions

None.
