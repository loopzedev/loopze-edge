# Issue: Restore node status after page load

## Status: Open

## Problem description

After reloading the page (F5 / browser refresh), all node status indicators are empty even though the nodes are still running in the backend and actively reporting status. The status only reappears once a node calls `n.status(fill, text)` again — e.g. on the next incoming message.

**Cause**: The node status is currently transmitted only as a **live stream** via WebSocket. There is no persistence and no mechanism to retrieve the last known status when the connection is established.

## Current pipeline

```
Node calls n.status("green", "42 msgs")
    ↓
Engine.makeStatusFunc → NATS Publish "status.<flowID>.<nodeID>"
    ↓
Server: conn.Subscribe("status.>") → hub.Broadcast(EventStatus, msg)
    ↓
WebSocket → Frontend: flowStore.updateNodeStatus(nodeId, status)
    ↓
BaseNode renders color dot + text
```

**Problem**: When the page loads, the WebSocket is not yet connected when the flows are loaded. Status messages sent before the connection was established are lost.

## Solution: in-memory map in the engine

A simple Go map in the engine stores the **most recent status** per node. On page load, the frontend queries the current status via a new API endpoint.

### Why a Go map instead of NATS KV?

- LOOPZE runs as a **single process** — no distributed system that would need KV
- The status is **volatile** — it is lost on server restart anyway (the engine starts fresh, nodes have no status)
- The engine already has the `nodes` map — the status cache lives in the same scope
- **Zero overhead**: no network roundtrip, no serialization, direct map access

### Race conditions

The status map is written by **multiple goroutines** concurrently (each node runs in its own goroutine via `nodeLoop`). At the same time, the API handler reads the map on HTTP requests.

**Solution**: `sync.RWMutex` protects the map.

- **Write access** (`Lock`): `makeStatusFunc` — called from node goroutines
- **Read access** (`RLock`): API handler — parallel reads are allowed

```go
type statusCache struct {
    mu      sync.RWMutex
    entries map[string]StatusMessage // nodeID → last status
}

func (c *statusCache) Set(nodeID string, msg StatusMessage) {
    c.mu.Lock()
    c.entries[nodeID] = msg
    c.mu.Unlock()
}

func (c *statusCache) GetAll() map[string]StatusMessage {
    c.mu.RLock()
    defer c.mu.RUnlock()
    result := make(map[string]StatusMessage, len(c.entries))
    for k, v := range c.entries {
        result[k] = v
    }
    return result
}

func (c *statusCache) Clear() {
    c.mu.Lock()
    c.entries = make(map[string]StatusMessage)
    c.mu.Unlock()
}
```

## Affected files

### Backend — changes

#### `internal/flow/engine.go`

New field in the Engine struct:

```go
type Engine struct {
    // ... existing fields ...
    statusCache statusCache
}
```

Initialization in `NewEngine`:

```go
statusCache: statusCache{entries: make(map[string]StatusMessage)},
```

In `makeStatusFunc`, additionally cache the status:

```go
func (e *Engine) makeStatusFunc(nodeID string, rn *runningNode) StatusFunc {
    return func(fill string, text string) {
        msg := StatusMessage{
            NodeID: nodeID,
            FlowID: rn.flowID,
            Status: NodeStatusPayload{Fill: fill, Text: text},
        }
        e.statusCache.Set(nodeID, msg)
        if e.publishStatus != nil {
            subject := fmt.Sprintf("status.%s.%s", rn.flowID, nodeID)
            e.publishStatus(subject, msg)
        }
    }
}
```

In `stopNodes`, clear the cache:

```go
e.statusCache.Clear()
```

New public method for the API handler:

```go
func (e *Engine) NodeStatuses() map[string]StatusMessage {
    return e.statusCache.GetAll()
}
```

#### `internal/api/handlers.go`

New endpoint:

```
GET /api/v1/status/nodes → { "statuses": { "<nodeId>": { "nodeId": "...", "flowId": "...", "status": { "fill": "green", "text": "42" } } } }
```

The handler calls `engine.NodeStatuses()` and serializes the result.

#### `internal/api/routes.go`

Register new route:

```go
r.Get("/api/v1/status/nodes", handler.GetNodeStatuses)
```

### Frontend — changes

#### `frontend/src/composables/useApi.ts`

New method:

```typescript
async function getNodeStatuses(): Promise<Record<string, { fill: string; text: string }>> {
    const res = await fetch('/api/v1/status/nodes')
    const data = await res.json()
    return data.statuses ?? {}
}
```

#### `frontend/src/views/FlowEditor.vue`

In `onMounted`, after `loadFlows`, fetch the status and apply it to the nodes:

```typescript
onMounted(async () => {
    const response = await api.getFlows()
    flowStore.loadFlows(response.flows, response.rev)

    // NEW: restore last node status
    const statuses = await api.getNodeStatuses()
    for (const [nodeId, status] of Object.entries(statuses)) {
        flowStore.updateNodeStatus(nodeId, status)
    }
})
```

## Flow after implementation

```
Page is loaded
    ↓
GET /api/v1/flows → load flows + nodes
    ↓
GET /api/v1/status/nodes → fetch last status of all nodes from engine cache
    ↓
flowStore.updateNodeStatus() for each node with status
    ↓
BaseNode immediately shows the last known status
    ↓
WebSocket connects → live updates take over from now on
```

## Lifecycle notes

- **Deploy**: `stopNodes()` calls `statusCache.Clear()` — all nodes restart and initially have no status
- **Server restart**: cache is gone (in-memory) — intentional, since the engine has no running nodes either
- **High-frequency updates**: `statusCache.Set()` always overwrites the last value — no memory growth, regardless of how often a node updates its status

## Dependencies

- The existing status pipeline (NATS Publish → WebSocket → Frontend) must work — implemented since `NODE_STATUS_PIPELINE.md`
- No frontend component changes needed — `updateNodeStatus()` and the BaseNode rendering already exist
