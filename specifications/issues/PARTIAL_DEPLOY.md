# Issue: Partial Deploy – Three deployment modes

## Status: Open

## Problem description

LOOPZE currently uses a **full-restart strategy**: On every deploy **all** running nodes are stopped and the entire workspace is re-instantiated (`engine.go:204-205`). For larger workspaces this is problematic:

- **Downtime**: All flows are briefly interrupted — even those that haven't changed
- **Message loss**: Messages in node buffers (`inputCh`) are lost on stop
- **State loss**: In-memory node state (e.g. counters in Function nodes) is reset on every deploy
- **Config-node restart**: Shared resources (MQTT connections etc.) are unnecessarily disconnected and rebuilt

Node-RED solves this with three deploy modes that the user can select via a dropdown next to the deploy button.

## The three deploy modes

### 1. Modified Nodes (default)
Only nodes/flows that have actually changed are redeployed.

**Logic:**
- Compute diff between current and new workspace
- Only stop and restart affected nodes
- Unchanged nodes keep running uninterrupted
- Config nodes restart only if their config changed

### 2. Modified Flows
All flows that contain at least one change are completely redeployed.

**Logic:**
- Flow-level diff: Has a flow changed? (nodes, wires, properties)
- Stop and restart affected flows completely
- Unchanged flows keep running

### 3. Full (Restart All)
The entire workspace is stopped and redeployed. All nodes are stopped, re-instantiated and started. Matches the current behavior.

### 4. Restart
Engine is fully shut down and restarted (`Engine.Stop()` → `Engine.Start()` → `Engine.Deploy()`). Resets **everything** — including engine state, context stores (memory), NATS connections and config-node resources. Useful when the system has gotten into an inconsistent state or a clean restart is desired.

## Technical implementation

### Phase 1: Diff engine (backend)

New package/file `internal/flow/diff.go`:

```go
type DeployMode string

const (
    DeployModifiedNodes DeployMode = "nodes"    // only changed nodes
    DeployModifiedFlows DeployMode = "flows"    // entire flows with changes
    DeployFull          DeployMode = "full"     // everything new (current behavior)
    DeployRestart       DeployMode = "restart"  // engine Stop → Start → Deploy
)

type WorkspaceDiff struct {
    AddedFlows    []string   // new flow IDs
    RemovedFlows  []string   // deleted flow IDs
    ModifiedFlows []string   // flows with changes

    AddedNodes    []string   // new node IDs
    RemovedNodes  []string   // deleted node IDs
    ModifiedNodes []string   // nodes with changed config/wires

    AddedConfigs    []string // new config-node IDs
    RemovedConfigs  []string // deleted config nodes
    ModifiedConfigs []string // config nodes with changes
}

func DiffWorkspaces(old, new Workspace) WorkspaceDiff { ... }
```

**Diff criteria for nodes:**
- Config map changed (deep-equal)
- Wires changed (connections re-plugged)
- Disabled flag changed
- Position (X/Y) is **not** a diff criterion (purely visual)

**Diff criteria for flows:**
- Nodes added/removed
- At least one node changed (see above)
- Flow disabled flag changed
- Flow env vars changed

### Phase 2: Selective node lifecycle (backend)

Refactor `Engine.Deploy()` to `Engine.Deploy(flows, configs, mode)`:

#### Mode: `DeployRestart`
```
1. Engine.Stop() — stop all nodes, clear state, configInstances nil
2. Engine.Start() — initialize engine fresh
3. Engine.Deploy(flows, configs, DeployFull) — rebuild everything
```
Hardest reset: engine lifecycle is fully traversed. All in-memory state (node context, status cache) is lost.

#### Mode: `DeployFull`
Existing behavior — `stopNodes()` → rebuild everything. Engine stays running.

#### Mode: `DeployModifiedFlows`
```
1. Compute diff
2. Only stop nodes in affected flows (stopNodesInFlows)
3. Check config nodes: restart changed config nodes
4. Re-instantiate + wire affected flows
5. Start new node goroutines
6. Unchanged flows stay running
```

#### Mode: `DeployModifiedNodes`
```
1. Compute diff
2. Stop removed nodes + close channels
3. Stop modified nodes (but don't remove from maps)
4. Check config nodes: restart changed ones
5. Instantiate + Init added + modified nodes
6. Rebuild wires for affected nodes
7. Start affected nodes + launch goroutines
8. Downstream nodes from changed wires: update SendFunc
```

**Challenges with selective stop:**
- `stopCh` is currently shared by **all** nodes — must become per-node/flow
- `wg.Wait()` waits for all goroutines — must become selective
- Wires from unchanged nodes can point to changed nodes → SendFunc must be updated
- Link registry must be partially updated

#### Proposed changes to `runningNode`:

```go
type runningNode struct {
    instance NodeInstance
    config   NodeConfig
    flowID   string
    inputCh  chan *Message
    stopCh   chan struct{}  // NEW: per-node stop channel (instead of global)
    done     chan struct{}  // NEW: signals that goroutine has ended
}
```

#### Proposed new engine methods:

```go
// stopNode stops a single node and its goroutine
func (e *Engine) stopNode(nodeID string) error

// stopFlow stops all nodes of a flow
func (e *Engine) stopFlow(flowID string) error

// rewireNode updates a node's SendFunc with new wires
func (e *Engine) rewireNode(nodeID string, wires [][]string)

// startNode instantiates, initializes and starts a single node
func (e *Engine) startNode(nodeID string, node Node, flowID string) error
```

### Phase 3: API extension

Extend `deployRequest`:

```go
type deployRequest struct {
    Flows   []flow.Flow       `json:"flows"`
    Configs []flow.ConfigNode `json:"configs,omitempty"`
    Rev     string            `json:"rev,omitempty"`
    Mode    string            `json:"deployMode,omitempty"` // "nodes", "flows", "full", "restart"
}
```

Default when `Mode` is empty: `"nodes"` (Modified Nodes).

The handler forwards the mode to `Engine.Deploy()`. On `"restart"` the handler calls `Engine.Stop()` → `Engine.Start()` before the deploy:

```go
// Engine signature
func (e *Engine) Deploy(flows []Flow, configs []ConfigNode, mode DeployMode) error

// Handler logic for restart
if req.Mode == "restart" {
    d.Engine.Stop()
    d.Engine.Start()
}
d.Engine.Deploy(req.Flows, req.Configs, flow.DeployFull)
```

### Phase 4: Frontend

#### Deploy button with dropdown
The deploy button gets a dropdown arrow (split button) like in Node-RED:

```
┌──────────┬───┐
│  Deploy  │ ▾ │
└──────────┴───┘
              │
              ├─ ● Modified Nodes  (default)
              ├─ ○ Modified Flows
              ├─ ○ Full Deploy
              └─ ○ Restart
```

- Click on "Deploy" → deploys with the currently selected mode
- Click on ▾ → dropdown opens, mode can be changed
- Selected mode is stored in `localStorage`

#### flowStore changes

```typescript
// New state
const deployMode = ref<'nodes' | 'flows' | 'full' | 'restart'>('nodes')

// Extend deploy payload
const payload: DeployPayload = {
    flows: flows.value,
    configs: configs.value.length > 0 ? configs.value : undefined,
    rev: revision.value ?? undefined,
    deployMode: deployMode.value,
}
```

The existing `dirtyNodeIds` and `dirtyFlowIds` sets are already tracked and can be used for visual hints (e.g. mark changed nodes/flows).

### Phase 5: Deploy feedback

Extend WebSocket event with the deploy mode and affected flows/nodes:

```json
{
    "action": "deployed",
    "revision": "a1b2c3d4",
    "mode": "nodes",
    "affected": {
        "added": ["node-id-1"],
        "modified": ["node-id-2", "node-id-3"],
        "removed": ["node-id-4"]
    }
}
```

## Implementation order

1. **Diff engine** (`diff.go` + tests) — foundation for everything
2. **Per-node stop channel** — convert `stopCh` from global to per-node
3. **`DeployModifiedFlows`** — simpler than node-level, good intermediate step
4. **`DeployModifiedNodes`** — the actual core
5. **API extension** — `deployMode` parameter
6. **Frontend dropdown** — split button on the deploy button
7. **Deploy feedback** — extended WebSocket events

## Affected files

### Backend
- `internal/flow/diff.go` — **NEW**: workspace diff logic
- `internal/flow/diff_test.go` — **NEW**: tests for diff
- `internal/flow/engine.go` — refactor Deploy/Stop for selective lifecycle
- `internal/flow/types.go` — `DeployMode` type
- `internal/api/handlers.go` — read `deployMode` from request + forward

### Frontend
- `frontend/src/stores/flowStore.ts` — `deployMode` state + payload
- `frontend/src/types/flow.ts` — extend `DeployPayload` type
- `frontend/src/components/HeaderBar.vue` — split button with dropdown

## Edge cases

- **First deploy** (no old state): always full deploy
- **Flow added/deleted**: new flow is started, deleted one is stopped, rest stays
- **Config node changed**: all nodes that reference this config must also be restarted (cascading restart)
- **Wire to deleted node**: SendFunc must handle missing targets gracefully (already does via `e.nodes[targetID]` lookup)
- **Link nodes**: change to Link-In/Out affects cross-flow communication → partially update link registry
- **Disabled flag toggle**: disable node = stop node; enable node = start node
