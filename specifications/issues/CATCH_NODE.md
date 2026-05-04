# Catch Node

## Status: Open

## Description

The Catch node intercepts **runtime errors from other nodes** and makes them processable as a regular message. Today, errors in `internal/flow/engine.go` (in `nodeLoop`, when calling `instance.HandleMessage`) are merely logged and forwarded via `publishNodeError` as a `DebugMessage` to the frontend — after that the message is gone, and the flow has no lever to do anything. The Catch node closes exactly this gap.

`catch` is already declared in the frontend as a `NodeType` in `frontend/src/types/flow.ts`, but has neither backend, config UI, help doc, nor engine integration.

## Design Template: Status Node

The Catch node is **structurally identical** to the already implemented Status node — it observes errors instead of status updates. Wherever possible, the Status-node pattern is reused:

- 0 inputs, 1 output, event-driven
- Listener pattern (`ErrorListenerProvider` analogous to `StatusListenerProvider`)
- Identical scope model and property schema
- Identical UI building blocks (scope select, multi-select list with highlight)
- Engine-side loop protection (Catch's own errors are filtered, like status events from Status nodes)

## Motivation

Today, every error is a silent death of the message:

- Function node throws → message disappears, no logging in the flow possible
- MQTT publish fails → no notification possible
- HTTP request times out → no fallback route

With Catch, standard patterns like dead-letter queue, notification, or logging into a Debug node are trivial to build.

## Behavior

- **0 inputs**, **1 output** — Catch is event-driven (analogous to Status node, which also has no input)
- `HandleMessage` is a no-op (return nil) — sending happens in the listener callback
- The outgoing message contains the **original message** plus `msg._error` (see below)

### Scope (three options, analogous to Status node)

| Scope | Description |
|---|---|
| **flow** (default) | Catches errors from all nodes in the same flow |
| **selected** | Catches errors only from explicitly selected nodes (always within the same flow) |
| **all** | Catches errors from all flows (power user) |

Multiple Catch nodes are allowed — each catches independently. If an error matches multiple Catch nodes, all are triggered (each gets its own COW clone of the message).

### Loop Protection

Filtered on the engine side: errors from Catch nodes themselves are **not** propagated in the listener fan-out — exactly analogous to the Status node, where status events from Status nodes are excluded so there are no self-trigger loops.

Consequence: **Retry patterns via Catch are deliberately not possible in the MVP.** Anyone needing retry does it in the source node itself (its own feature per node type). Catch is a logging/notification tool.

### `msg._error` Format

Flat and minimal — structure deliberately parallel to `msg.status` of the Status node:

```json
{
  "_error": {
    "message": "TypeError: Cannot read property 'foo' of undefined",
    "source": {
      "id": "n_abc123",
      "type": "function",
      "name": "Parse Payload",
      "flowId": "f_xyz"
    }
  }
}
```

Additionally, `_id` (the original message ID) is preserved — `flow.Message.COWClone()` already does this correctly.

### Async Errors

Many nodes (MQTT publish, HTTP request) send asynchronously in the background. If `HandleMessage` has already returned and the error occurs only afterwards, the `nodeLoop` path doesn't see it. So that Catch still triggers, nodes are injected with a `flow.ErrorFunc` (just like they get `flow.StatusFunc` via `SetStatus` today):

```go
func (n *MyNode) SetError(fn flow.ErrorFunc) { n.errorFn = fn }
// ...
n.errorFn(err, msg)  // triggers the same Catch pipeline as a synchronous return error
```

The engine fans out to the same `errorListeners` as for synchronous errors. The concrete migration of individual nodes (HTTP, MQTT) happens **in their own issues** — the Catch node itself is unaffected.

## Configuration (Backend)

```json
{
  "scope": "flow",
  "targetNodes": []
}
```

| Field | Description |
|---|---|
| `scope` | `flow` (default), `selected`, or `all` |
| `targetNodes` | When `scope=selected`: list of node IDs |

## UI

Clone of `StatusConfig.vue`, only labels and help text adjusted. The `useNodeProperty` pattern, the scope dropdown, the checkbox list with hover highlight (`setHoveredHighlightNodeId`), and the inline help text are taken over 1:1.

Node display:
- Category: **`common`** (like Status — no dedicated accent color needed)
- Body: `scope=flow` → "this flow", `scope=selected` → "N nodes", `scope=all` → "all flows"
- Left anchor (input) is not rendered — `BaseNode` with `:inputs="0"`

## Implementation

### Engine Extension (`internal/flow/engine.go`)

Full mirror of the status-listener mechanism (`engine.go:102-104, 567-575, 914-939`):

- New field `errorListeners map[uint64]ErrorListenerFunc` + `errorListenersMu sync.RWMutex` + sequence counter
- `registerErrorListener(fn) (unregister func())` analogous to `registerStatusListener`
- `fanoutError(msg ErrorMessage)` analogous to `fanoutStatus` — with filter: if the source node is a Catch, return early (loop protection)
- In `wireAllNodes`, wire up the error-listener providers (second loop directly after the status-listener loop)
- `makeErrorFunc(nodeID, rn)` builds the per-node `ErrorFunc` closure (analogous to `makeStatusFunc`), which internally calls both `publishNodeError` AND `fanoutError`
- In the `nodeLoop` error path: instead of calling `publishNodeError` directly, the engine calls `errorFunc(err, msg)` — same pipeline for sync and async

New types in `internal/flow/registry.go` (analogous to Status):

```go
type ErrorMessage struct {
    NodeID     string
    FlowID     string
    SourceType string
    SourceName string
    Error      string
    Msg        *Message
}

type ErrorFunc func(err error, msg *Message)
type ErrorListenerFunc func(msg ErrorMessage)
type ErrorListenerProvider interface {
    SetErrorListener(register func(ErrorListenerFunc) (unregister func()))
}
```

### Catch Node (`internal/nodes/catch.go`)

Clone of `internal/nodes/status.go` with minimal changes:
- `handleStatus(sm StatusMessage)` → `handleError(em ErrorMessage)`
- Builds `_error` map instead of `status` map on the output message
- `SourceType` filter: the Status node filters its own status events; the Catch node doesn't have to do this itself because the engine already does it in `fanoutError`
- Idle status: `"listening for errors"` instead of `"listening"`

### Registration

```go
registry.Register("catch", nodes.NewCatchNode, nodes.CatchTypeInfo())
```

### Frontend

- **`frontend/src/components/config/CatchConfig.vue`** — Clone of `StatusConfig.vue`, filter `n.type !== 'catch'` instead of `'status'`, help text adjusted
- **`frontend/src/components/nodes/CatchNode.vue`** — Clone of `StatusNode.vue`, `node-type="catch"`
- **`frontend/src/components/help/docs.ts`** — Help entry `catch` with the note "not a retry tool"
- Vue Flow node-type map: `catch` → `CatchNode.vue` (at the spot where `status` is also registered)
- Config editor dispatcher: `catch` → `CatchConfig.vue`

## Tests

Clones of the Status-node tests in `internal/nodes/catch_test.go`, plus engine integration tests in `internal/flow/engine_catch_test.go`:

| Test | Verifies |
|---|---|
| `TestCatchScopeFlow_TriggeredOnError` | Function node throws → Catch (`scope=flow`) emits `_error.source.id == function_node_id` |
| `TestCatchScopeFlow_IgnoresOtherFlows` | Error in flow A does not trigger a Catch in flow B |
| `TestCatchScopeSelected_OnlyMatching` | `targetNodes=[A]` ignores errors from node B |
| `TestCatchScopeSelected_MultipleNodes` | `targetNodes=[A,B]` catches from both |
| `TestCatchScopeAll_AcrossFlows` | `scope=all` catches errors from all flows |
| `TestMultipleCatch_BothTriggered` | Two matching Catch nodes → both trigger (separate COW clones) |
| `TestLoopGuard_CatchErrorsFiltered` | An error in the Catch output branch does not trigger a Catch (engine filter) |
| `TestErrorPayload_PreservesOriginalMsg` | `payload` and `_id` identical to the original message |
| `TestErrorPayload_ContainsErrorFields` | `_error.message`, `_error.source.{id,type,name,flowId}` correct |
| `TestSelectedNodes_UnknownIDIgnored` | Unknown ID in `targetNodes` → no crash |
| `TestStopReleasesListener` | After `Stop()` no more Catch triggers |
| `TestAsyncError_TriggersCatch` | Stub node with `SetError` calls `errorFn(err, msg)` from a goroutine → Catch is triggered |

## Deliberately Out of Scope

- **Retry pattern via Catch** — loop protection prevents this on purpose
- **Filtering by error type/pattern** — the downstream Switch node handles that
- **Migrating all existing nodes to `SetError`** — the API is provided; concrete usage per node type in their own issues (HTTP first, then MQTT-Out)

## Dependencies

- `flow.Message.COWClone()` — exists
- `StatusListenerProvider` pattern as a template — exists, will be mirrored
- `flow.NewMessage()` + `Set/SetPayload` — exists
- `BaseNode` with `:inputs="0"` — exists (Status, Inject)
- `useNodeProperty`, `setHoveredHighlightNodeId` — exists
