# Properties Panel: Cancel / Revert for Node Changes

## Context

When a node is edited in the properties panel, all changes go **directly into the Pinia store** immediately (`flowStore.updateNodeData()`). There is no intermediate buffer, no transaction, and no undo. The panel closes without a revert option — once changed, values remain until the next deploy or page reload.

The user needs a **Cancel button** to undo changes to a node before they are deployed.

## Requirement

### Cancel Button in the Properties Panel

- Shown in the properties panel when the selected node is **dirty** (`flowStore.isNodeDirty(nodeId)`)
- Clicking "Cancel" resets the node to its **last deployed state**
- After cancel: node is removed from `dirtyNodeIds`
- Button disappears when the node is no longer dirty

### Snapshot Mechanism

On **deploy**, a snapshot of the node data is saved. This snapshot is the reference for cancel/revert.

- New state in the flowStore: `deployedNodeData: Map<string, Record<string, any>>` — stores the last deployed state of each node
- Populated in `deploy()` after a successful deploy
- Populated in `loadFlows()` on initial load
- `revertNode(nodeId)` restores the snapshot

### Dataflow

```
Deploy successful
  -> deployedNodeData.set(nodeId, deepCopy(node.data))  // save snapshot

User edits node
  -> flowStore.updateNodeData()  // as before, changes the store directly
  -> markNodeDirty(nodeId)       // as before

User clicks "Cancel"
  -> flowStore.revertNode(nodeId)
  -> node.data = deepCopy(deployedNodeData.get(nodeId))
  -> dirtyNodeIds.delete(nodeId)
```

## Technical Details

### Affected Files

| File | Change |
|-------|-----------|
| `frontend/src/stores/flowStore.ts` | `deployedNodeData` map, `revertNode()`, snapshot in `deploy()` and `loadFlows()` |
| `frontend/src/components/PropertyPanel.vue` | Cancel button (visible when node is dirty) |

### flowStore Changes

```typescript
// New state
const deployedNodeData = ref<Map<string, Record<string, any>>>(new Map())

// After successful deploy: store snapshot of all nodes
function snapshotDeployedState() {
  const map = new Map<string, Record<string, any>>()
  for (const node of nodes.value) {
    map.set(node.id, structuredClone(node.data))
  }
  deployedNodeData.value = map
}

// Reset node to last deployed state
function revertNode(nodeId: string) {
  const snapshot = deployedNodeData.value.get(nodeId)
  if (!snapshot) return
  const node = nodes.value.find(n => n.id === nodeId)
  if (!node) return
  node.data = structuredClone(snapshot)
  dirtyNodeIds.value.delete(nodeId)
}
```

### PropertyPanel Cancel Button

```html
<button
  v-if="selectedNode && flowStore.isNodeDirty(selectedNode.id)"
  @click="flowStore.revertNode(selectedNode.id)"
>
  Cancel
</button>
```

Placement: in the header area of the properties panel, next to the node name or as a footer action.

## Out of Scope

- No general undo/redo system — only cancel for the currently selected node
- No cancel for flow properties or config nodes (can be added later)
- Cancel always refers to the last deployed state, not to a previous edit step
- Newly added nodes (that have never been deployed) have no snapshot — cancel does NOT remove the node, but only resets to defaults
