# Copy Selected Nodes

## Summary

Allow users to copy and paste selected nodes (including multi-selection) within the flow editor, preserving node configuration and wiring between copied nodes.

## Motivation

Copying nodes is a fundamental workflow in visual flow editors. Users frequently create similar processing pipelines and need to duplicate groups of nodes with their configuration and internal wiring intact. Currently, nodes can only be created from the palette, which is slow for repetitive patterns.

## Requirements

### Core

- **Copy** selected nodes via `Ctrl+C` (keyboard shortcut)
- **Paste** copied nodes via `Ctrl+V` at the current mouse position (or with an offset from the original position if mouse position is unavailable)
- **Cut** selected nodes via `Ctrl+X` (copy + delete originals)
- **Duplicate** selected nodes via `Ctrl+D` as a shortcut for copy+paste in place (with offset)
- Support **single node** and **multi-node** selection (via Shift+Click or box select)

### Copy Behavior

- Copied nodes receive **new unique IDs** (UUIDs)
- All **node configuration** (properties, rules, settings) is deep-cloned
- **Internal wires** (edges between copied nodes) are preserved and reconnected to the new node IDs
- **External wires** (edges from/to nodes outside the selection) are **not** copied
- Pasted nodes are placed with a visual **offset** (e.g. +32px x, +32px y) from the originals to avoid stacking
- Pasted nodes are automatically **selected** after paste
- The flow is marked as **dirty** after paste

### Clipboard

- Use an **internal clipboard** (store/state) rather than the system clipboard, since node data is complex structured data
- Clipboard content persists across paste operations (paste multiple times)
- Clipboard is cleared on page unload or flow switch

### Edge Cases

- Copying a node that references a **config node** (e.g. MQTT broker): the reference is kept, the config node is **not** duplicated
- **Comment nodes** are copyable like any other node
- Copy/paste does **not** duplicate runtime state (status, debug messages)
- Undo/redo integration (if available) should treat paste as a single undoable action

## Out of Scope

- Cross-flow copy/paste (between different flows/tabs)
- Cross-browser/cross-session clipboard (system clipboard integration)
- Copy/paste via context menu (can be added later)

## Technical Notes

- Selection state is managed in `flowStore.ts` via `selectedNodeId` — multi-select support may need to be extended (currently flattened to single node in store)
- Vue Flow provides built-in multi-select via Shift+Click and selection change events
- Node creation logic exists in `flowStore.addNode()` — paste should reuse this path
- New node IDs should use the same UUID generation as `flowStore.addNode()`
