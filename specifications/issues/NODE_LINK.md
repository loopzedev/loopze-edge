# Issue: Link Nodes — Cross-Flow Messaging (Input, Output, Request)

## Status: Open

## Problem Description

With the introduction of multiple flows per workspace, the ability to send **messages across flow boundaries** is missing. Currently nodes can only be wired together within a single flow. For modular flow architectures, three new node types are needed that act as virtual bridges between flows.

## Overview of the Three Node Types

| Node type | Type ID | Canvas inputs | Canvas outputs | Description |
|---|---|---|---|---|
| **Link Input** | `link-in` | 0 | 1 | Receives messages from Link Output nodes in other flows |
| **Link Output** | `link-out` | 1 | 0 | Sends messages to Link Input nodes in other flows |
| **Link Request** | `link-call` | 1 | 1 | Sends a request to a Link Input node and waits for the response |

```
Flow A                              Flow B
+---------------------+             +---------------------+
|                      |             |                      |
|  [Inject] -> [Link Output] ------> [Link Input] -> [Debug] |
|                      |             |                      |
+---------------------+             +---------------------+

Flow C (Request/Response)           Flow D (Service)
+--------------------------+        +--------------------------+
|                           |        |                           |
|  [Inject] -> [Link Request] -----> [Link Input]               |
|              ^ (Response)  |        |      v                   |
|              |             |        |  [Function]              |
|              +-------------+------ [Link Output] <- +          |
|          [Debug] <-+       |        |                           |
+--------------------------+        +--------------------------+
```

## Requirements

### 1. Link Output Node (`link-out`)

- **Canvas**: 1 input, 0 outputs
- **Function**: receives messages and forwards them to configured Link Input nodes
- **Properties** (double-click): tabular view of all available **Link Input** nodes in the entire workspace
  - Columns: checkbox, flow name, node name
  - **Multi-selection** possible — one Link Output can send to multiple Link Inputs
  - Only Link Input nodes are shown (no other node types)
- **Configuration**:
  ```json
  {
    "links": ["link-in-node-id-1", "link-in-node-id-2"]
  }
  ```

### 2. Link Input Node (`link-in`)

- **Canvas**: 0 inputs, 1 output
- **Function**: receives messages from Link Output nodes and forwards them to the connected output port
- **Properties** (double-click): tabular view of all available **Link Output** nodes in the entire workspace
  - Columns: checkbox, flow name, node name
  - **Multi-selection** possible — one Link Input can receive from multiple Link Outputs
  - Only Link Output nodes are shown (no other node types)
- **Configuration**:
  ```json
  {
    "links": ["link-out-node-id-1", "link-out-node-id-2"]
  }
  ```
- **Note**: the link configuration is **bidirectionally mirrored** — when a Link Output selects Link Input `A`, the Link Output automatically appears as selected in the properties table of `A` (and vice versa). The mapping is maintained from both sides.

### 3. Link Request Node (`link-call`)

- **Canvas**: 1 input, 1 output
- **Function**: sends a message to a Link Input node, waits for the response from the corresponding Link Output node, and emits the response on its own output port
- **Properties** (double-click): tabular view of all available **Link Input** nodes
  - Columns: radio button, flow name, node name
  - **Only one selection** possible (radio instead of checkbox)
  - Only Link Input nodes are shown
- **Configuration**:
  ```json
  {
    "linkTarget": "link-in-node-id"
  }
  ```

### 4. Request/Response Mechanism

The Link Request node implements a **synchronous request/response pattern** across flow boundaries:

1. **Request**: the Link Request node sends the message to the configured Link Input node. The **requestor ID** (node ID of the Link Request node) is embedded in the message
2. **Processing**: the target flow processes the message normally through its nodes
3. **Response**: when the processed message reaches a Link Output node, that node checks whether a requestor ID is present:
   - **Yes**: the message is sent directly back to the calling Link Request node (not to the regular link targets)
   - **No**: normal forwarding to the configured Link Input nodes
4. **Output**: the Link Request node receives the response and emits it on its output port

**Message structure for requestor tracking**:
```go
// An internal field is carried along in the message object:
msg.Set("_linkSource", requestNodeID)  // set by Link Request
msg.Get("_linkSource")                 // read by Link Output
```

The `_linkSource` field is removed by the Link Request node after delivery so it does not leak into subsequent nodes.

### 5. Properties — Tabular Link View

The properties table is structured identically for all three node types (only the selection type and displayed nodes differ):

```
+----------------------------------------------+
|  Link Output                                  |
+----------------------------------------------+
|  Name                                         |
|  +----------------------------------------+   |
|  | my-link-out                            |   |
|  +----------------------------------------+   |
|                                               |
|  Available Link Inputs                        |
|  +----+--------------+--------------------+   |
|  | v  | Flow         | Node               |   |
|  +----+--------------+--------------------+   |
|  | [x]| Flow 1       | api-input          |   |
|  | [ ]| Flow 2       | data-receiver      |   |
|  | [x]| Flow 3       | event-handler      |   |
|  +----+--------------+--------------------+   |
+----------------------------------------------+
```

For the Link Request node, a **radio button** is used instead of checkboxes (single selection only).

**Determining available link nodes**:
- The table searches **all flows** in `flowStore.flows` for nodes of the matching type
- For Link Output -> shows all `link-in` nodes
- For Link Input -> shows all `link-out` nodes
- For Link Request -> shows all `link-in` nodes
- The own flow is included (links within the same flow are allowed)

## Data Structure

### workspace.json

```json
[
  {
    "id": "flow-1",
    "type": "tab",
    "label": "Flow 1",
    "nodes": [
      {
        "id": "node-link-out-1",
        "type": "link-out",
        "name": "to-processor",
        "x": 400,
        "y": 200,
        "z": "flow-1",
        "inputs": 1,
        "outputs": 0,
        "wires": [],
        "config": {
          "links": ["node-link-in-1", "node-link-in-2"]
        }
      }
    ]
  },
  {
    "id": "flow-2",
    "type": "tab",
    "label": "Flow 2",
    "nodes": [
      {
        "id": "node-link-in-1",
        "type": "link-in",
        "name": "from-sender",
        "x": 100,
        "y": 200,
        "z": "flow-2",
        "inputs": 0,
        "outputs": 1,
        "wires": [["node-debug-1"]],
        "config": {
          "links": ["node-link-out-1"]
        }
      }
    ]
  }
]
```

## Affected Files

### Backend — New Files

- `internal/nodes/link_in.go` — Link Input node: registers with the engine, receives messages and sends them to the canvas output
- `internal/nodes/link_out.go` — Link Output node: forwards messages to configured Link Input nodes, checks `_linkSource` for request/response
- `internal/nodes/link_call.go` — Link Request node: sends with `_linkSource`, receives response and emits it on the output

### Backend — Changes

- `internal/server/server.go` — registration of the three new node types in `registerNodes()`
- `internal/flow/engine.go` — cross-flow routing: the engine must allow Link Output nodes to send messages directly to nodes in other flows. To do this, the engine needs a **link registry** built at deploy time:
  ```go
  type linkRegistry struct {
      inputs  map[string]*runningNode   // nodeID -> running link-in node
      outputs map[string]*runningNode   // nodeID -> running link-out node
  }
  ```
  Link nodes get access to this registry via a new callback (`SetLinkSend`) to send messages directly to other nodes — independent of the normal wire wiring.

### Frontend — New Files

- `frontend/src/components/config/LinkConfig.vue` — common config component for all three Link node types with tabular view. Encapsulates the logic for determining available link nodes from all flows and distinguishes via prop between checkbox (multi) and radio (single) selection.

### Frontend — Changes

- `frontend/src/components/PropertyPanel.vue` — dispatch for `link-in`, `link-out`, and `link-call` to the new `LinkConfig` component
- `frontend/src/components/nodes/tokens.ts` — already present: `link-in` -> input category (green), `link-out` -> output category (orange). Add: `link-call` -> process category
- `frontend/src/types/flow.ts` — add `link-call` to the `NodeType` union (link-in and link-out are already defined)

## Technical Notes

### Cross-Flow Routing in the Engine

The normal wire wiring (`makeSendFunc`) only works within a single flow. For Link nodes, a parallel routing path is needed:

1. **At deploy time**: engine iterates over all nodes, identifies Link nodes, and builds the `linkRegistry`
2. **Link Output -> Link Input**: the Link Output node reads `config.links[]`, fetches the corresponding `runningNode` references from the registry, and sends the message directly into their `inputCh`
3. **Link Request -> Link Input -> Link Output -> Link Request**: the Request node sets `_linkSource`, the Output node reads it and routes the response back

### Bidirectional Link Mirroring (Frontend)

When the operator selects Link Input `A` in a Link Output node, Link Input `A` must also list the Link Output in its `config.links[]`. This is synchronized in the frontend on save:

```typescript
function toggleLink(targetNodeId: string, selected: boolean) {
  // Update own config
  updateOwnLinks(targetNodeId, selected)
  // Mirror: update target node's config
  updateTargetLinks(ownNodeId, selected)
}
```

Both nodes are marked dirty.

### Engine — Link Nodes Need Access to Other Nodes

The existing `NodeInstance` interface offers no mechanism for cross-node communication. Options:

**Option A: New callback `SetLinkSend`**
```go
type LinkSendFunc func(targetNodeID string, msg *Message)

type LinkProvider interface {
    SetLinkSend(fn LinkSendFunc)
}
```
The engine checks at deploy time whether a node implements `LinkProvider` and sets the callback. Analogous to `ContextProvider`.

**Option B: Engine-level routing**
The engine takes over routing entirely: after `HandleMessage` it checks whether the node is a Link node and routes accordingly. The Link nodes themselves are then "dumb" and simply return the message.

**Recommendation: Option A** — consistent with the existing provider pattern (`ContextProvider`), keeps the routing logic in the nodes.

## Dependencies

- **Flow management (FLOWS.md)**: must be implemented so that multiple flows exist and Link nodes can be sensibly used
- The node types `link-in` and `link-out` are already defined in the frontend as types and in the styling tokens — they appear automatically in the palette as soon as the backend registers them
