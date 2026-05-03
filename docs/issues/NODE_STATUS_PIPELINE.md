# Issue: Node status end-to-end pipeline

## Status: Open

## Problem description

The infrastructure for node status is already in place in frontend and backend, but the **end-to-end chain is broken**. Nodes call `n.status(fill, text)`, but the status never reaches the browser.

The existing debug pipeline shows the correct pattern:

```
Node → n.debug() → Engine.PublishDebugFunc → NATS "debug.>" → Server Subscriber → hub.Broadcast() → WebSocket → Browser
```

For status, this pattern is completely missing — `makeStatusFunc` only logs via `slog.Debug`.

## Inventory

### What already works

| Component | Status | Details |
|---|---|---|
| `StatusFunc` definition | OK | `internal/flow/registry.go` — `func(fill string, text string)` |
| `SetStatus()` lifecycle | OK | Engine calls `SetStatus(makeStatusFunc(nodeID))` after `Init()` |
| Nodes call `n.status()` | OK | Debug node (grey, statusText), Function node (`node.status(fill, text)` in JS) |
| WebSocket Hub broadcast | OK | `hub.Broadcast(eventType, data)` in `internal/ws/hub.go` |
| `StatusEvent` TypeScript types | OK | `frontend/src/types/events.ts` — `StatusEvent`, `NodeStatus` |
| WebSocket `onStatus` dispatcher | OK | `frontend/src/composables/useWebSocket.ts` — dispatches status events correctly |
| BaseNode status rendering | OK | `frontend/src/components/nodes/BaseNode.vue` — color dot + text |
| Status colors | OK | `frontend/src/components/nodes/tokens.ts` — red, green, yellow, blue, grey |

### Reference: debug pipeline (works)

```
Engine.makeDebugFunc(nodeID)                    → PublishDebugFunc(subject, msg)
    ↓
server.go: engine.SetPublishDebug(func(...) {
    conn.Publish("debug.<flowID>.<nodeID>", data)   → NATS
})
    ↓
server.go: conn.Subscribe("debug.>", func(m) {
    hub.Broadcast(ws.EventDebug, dbg)               → WebSocket Hub
})
    ↓
Browser: ws.onDebug → debugStore.addMessage()
```

### The 4 missing connections

#### Gap 1: Engine — `PublishStatusFunc` callback missing

**Analogous to:** `SetPublishDebug` / `PublishDebugFunc` in `internal/flow/engine.go`

The engine needs a `PublishStatusFunc` callback (analogous to `PublishDebugFunc`) that the server sets at start.

```go
type PublishStatusFunc func(subject string, msg StatusMessage)
```

New `StatusMessage` struct in `internal/flow/registry.go`:

```go
type StatusMessage struct {
    NodeID string `json:"nodeId"`
    FlowID string `json:"flowId"`
    Fill   string `json:"fill"`
    Text   string `json:"text"`
}
```

#### Gap 2: Engine — `makeStatusFunc` must publish via NATS

**File:** `internal/flow/engine.go:344-350`

```go
// CURRENT:
func (e *Engine) makeStatusFunc(nodeID string) StatusFunc {
    return func(fill string, text string) {
        slog.Debug("node status",
            "node_id", nodeID, "fill", fill, "text", text)
        // TODO: broadcast status via WebSocket to frontend
    }
}
```

**Fix:** Analogous to `makeDebugFunc`, call the `PublishStatusFunc` callback:

```go
func (e *Engine) makeStatusFunc(nodeID string) StatusFunc {
    return func(fill string, text string) {
        if e.publishStatus != nil {
            e.publishStatus("status."+e.flowID+"."+nodeID, StatusMessage{
                NodeID: nodeID,
                FlowID: e.flowID,
                Fill:   fill,
                Text:   text,
            })
        }
    }
}
```

#### Gap 3: Server — NATS publish + subscribe for status

**File:** `internal/server/server.go`

Analogous to the debug pattern, two additions:

1. **Set publish callback** (analogous to `SetPublishDebug`):
```go
s.engine.SetPublishStatus(func(subject string, msg flow.StatusMessage) {
    data, _ := json.Marshal(msg)
    conn.Publish(subject, data)
})
```

2. **NATS subscriber** (analogous to `debug.>`):
```go
conn.Subscribe("status.>", func(m *nats.Msg) {
    var status flow.StatusMessage
    json.Unmarshal(m.Data, &status)
    s.hub.Broadcast(ws.EventStatus, status)
})
```

#### Gap 4: Frontend — listener + FlowStore action

**File:** `frontend/src/App.vue` — `ws.onStatus()` listener missing:

```typescript
ws.onStatus((event) => {
    flowStore.updateNodeStatus(event.nodeId, event.status)
})
```

**File:** `frontend/src/stores/flowStore.ts` — action to update:

```typescript
function updateNodeStatus(nodeId: string, status: NodeStatus) {
    const node = nodes.value.find(n => n.id === nodeId)
    if (node) {
        node.data = { ...node.data, status }
    }
}
```

## Affected files

### Backend
- `internal/flow/registry.go` — `StatusMessage` struct + `PublishStatusFunc` type
- `internal/flow/engine.go` — `SetPublishStatus`, `makeStatusFunc` publishes via NATS
- `internal/server/server.go` — NATS publish callback + `status.>` subscriber
- `internal/ws/hub.go` — `EventStatus` constant already exists

### Frontend
- `frontend/src/App.vue` — add `ws.onStatus()` listener
- `frontend/src/stores/flowStore.ts` — add `updateNodeStatus()` action

## Dependencies

- The Debug node (`internal/nodes/debug.go`) already uses `n.status("grey", statusText)` for the configurable status output — only becomes visible once this pipeline is in place
- The Function node (`internal/nodes/function.go`) offers `node.status(fill, text)` in the JS runtime — also blocked

## Notes

- `NodeStatus` in the frontend expects a `shape` field (`ring` | `dot`); the backend currently only delivers `fill` + `text`. Use default `dot` if not specified.
- Status updates can be high-frequency (e.g. on every message in the Debug node with `count` mode) — consider throttling/debouncing on backend or frontend side if needed.
