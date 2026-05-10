# Adding a node

This is the end-to-end walkthrough for shipping a new node into LOOPZE. The
example throughout uses a fictional **BACnet** group, but the steps apply
equally to any new protocol or logic node.

A node lives in two trees that mirror each other:

```
internal/nodes/bacnet/         ← backend Go code, tests, init()
frontend/src/nodes/bacnet/     ← Vue editors, manifest
```

A single string — the group `name` — ties them together. Get that right
and the rest is mechanical.

## 0. Decide what you're building

Before writing code, sketch the node:

- **Type IDs** — short kebab-case strings (e.g. `bacnet-read`, `bacnet-write`).
- **Inputs / outputs** — number of canvas ports.
- **Config node?** — does this protocol need a shared connection manager
  (like `mqtt-broker` or `s7-plc`)? If yes, you'll add a config-node type
  too.
- **Group name** — short, lowercase, matches the directory (e.g. `bacnet`).

For protocol nodes, also draft a spec under `specifications/issues/NODE_BACNET.md`
following the existing examples. Specs must be in English.

## 1. Backend skeleton

### Create the package

```bash
mkdir -p internal/nodes/bacnet
```

Each flow-node file looks like this. The `BaseNode` embed handles the
engine-injected `Send` / `Status` / `Debug` callbacks for you:

```go
// internal/nodes/bacnet/read.go
package bacnet

import (
    "github.com/loopzedev/loopze-edge/internal/flow"
    "github.com/loopzedev/loopze-edge/internal/nodes"
)

type ReadNode struct {
    nodes.BaseNode
    config flow.NodeConfig

    // your parsed config / runtime state
    deviceID string
    objectID int
}

func NewReadNode(cfg flow.NodeConfig) (flow.NodeInstance, error) {
    n := &ReadNode{config: cfg}
    return n, nil
}

func (n *ReadNode) Init() error {
    p := n.config.Properties
    n.deviceID = nodes.StringVal(p, "deviceId", "")
    n.objectID = nodes.IntVal(p, "objectId", 0)
    return nil
}

func (n *ReadNode) Start() error                     { return nil }
func (n *ReadNode) Stop() error                      { return nil }
func (n *ReadNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
    // do work, then call n.Send(0, outMsg) to emit downstream
    return nil, nil
}

func ReadTypeInfo() flow.NodeTypeInfo {
    return flow.NodeTypeInfo{
        Type:        "bacnet-read",
        Label:       "BACnet Read",
        Category:    "industrial",
        Description: "Reads a property from a BACnet device.",
        Icon:        "memory",
        Inputs:      0,
        Outputs:     1,
        Defaults: map[string]any{
            "deviceId": "",
            "objectId": 0,
        },
    }
}
```

### Optional capabilities

If your node needs runtime services beyond the basics, add the matching
optional setter — the engine detects it via type-switch and injects:

| Need | Interface | Method |
|---|---|---|
| Read a config node | `flow.ConfigProvider` | `SetConfigLookup(fn)` |
| Report async errors | `flow.ErrorProvider` | `SetError(fn)` |
| Register HTTP routes | `flow.HTTPMuxProvider` | `SetHTTPMux(mux)` |
| Track TCP sessions | `flow.SessionRegistryProvider` | `SetSessionRegistry(r)` |
| Resolve TLS certs | `flow.CertStoreProvider` | `SetCertStore(s)` |

For config-node lookups, use the typed helper:

```go
device, err := nodes.ResolveConfigInstance[Device](
    n.configLookup, n.deviceID, n.Status,
    nodes.ResolveConfigParams{
        NodeKind:   "bacnet-read",
        NodeID:     n.config.ID,
        ConfigKind: "device",
        TypeLabel:  "a BACnet device",
    },
)
```

### Self-registration

Every group has an `init.go` that announces itself to the registry:

```go
// internal/nodes/bacnet/init.go
package bacnet

import "github.com/loopzedev/loopze-edge/internal/nodes"

func init() {
    nodes.RegisterGroup(nodes.Group{
        Name:        "bacnet",
        Description: "BACnet read / write nodes (BACnet/IP)",
        Nodes: []nodes.FlowNodeRegistration{
            {Type: "bacnet-read",  Factory: NewReadNode,  Info: ReadTypeInfo()},
            {Type: "bacnet-write", Factory: NewWriteNode, Info: WriteTypeInfo()},
        },
        ConfigNodes: []nodes.ConfigNodeRegistration{
            {Type: "bacnet-device", Factory: NewDevice, Info: DeviceConfigTypeInfo()},
        },
    })
}
```

The `Name` field is **load-bearing**: it must match the frontend manifest
(see step 2). Use the same string for the directory name.

### Wire into the binary

Add one blank-import line to `cmd/loopze/groups.go`:

```go
import (
    _ "github.com/loopzedev/loopze-edge/internal/nodes/bacnet"
    // …existing groups
)
```

That's the only place outside the new package that the backend needs.

### Tests

Drop a `*_test.go` next to each implementation file. The `nodestest`
package provides reusable fixtures:

```go
import "github.com/loopzedev/loopze-edge/internal/nodes/nodestest"

func TestReadNode_HandleMessage(t *testing.T) {
    c := &nodestest.Collector{}
    n := &ReadNode{}
    n.Send = c.Send

    n.HandleMessage(flow.NewMessage())

    if c.Count() != 1 {
        t.Fatalf("expected 1 emitted message, got %d", c.Count())
    }
}
```

For TLS-related tests, use `nodestest.GenerateTLSPair` and
`nodestest.NewCertStore`.

### Optional API handlers

Some protocols need REST endpoints — e.g. `POST /api/v1/bacnet/test-connection`.
Drop them in `internal/api/bacnet_handlers.go` and mount in
`internal/api/routes.go`. Look at `s7_handlers.go` for the pattern.

### Optional demo server

If your protocol benefits from a local simulator for integration tests,
add `demo/bacnet-server/` with a minimal stub. See `demo/s7-server/` and
`demo/modbus-server/` for examples. Wire it via `make demo-bacnet`.

## 2. Frontend manifest

### Create the package

```bash
mkdir -p frontend/src/nodes/bacnet
```

Add one Vue editor per node type. Editors receive the selected node via
the flow store and write back via `flowStore.updateNodeData`:

```vue
<!-- frontend/src/nodes/bacnet/BACnetReadConfig.vue -->
<script setup lang="ts">
import { computed } from 'vue'
import { useFlowStore } from '@/stores/flowStore'
import FormLabel from '@/components/ui/FormLabel.vue'
import FormInput from '@/components/ui/FormInput.vue'

const flowStore = useFlowStore()
const node = computed(() => flowStore.selectedNode)
const config = computed(() => (node.value?.data?.config ?? {}) as Record<string, unknown>)

function update(patch: Record<string, unknown>) {
  if (!node.value) return
  flowStore.updateNodeData(node.value.id, { config: { ...config.value, ...patch } })
}
</script>

<template>
  <div class="flex flex-col gap-3">
    <div class="flex flex-col gap-1">
      <FormLabel>Device ID</FormLabel>
      <FormInput
        :model-value="(config.deviceId as string) ?? ''"
        placeholder="urn:device:1"
        @update:model-value="update({ deviceId: $event })"
      />
    </div>
  </div>
</template>
```

### Group-specific enums

If your group has its own option lists (data types, function codes,
address regexes), put them in `frontend/src/nodes/bacnet/enums.ts`. Keep
the file in sync with the canonical Go definitions — leave a comment
pointing at the corresponding `.go` file.

Cross-group lists (`VALUE_TYPES`, `STORAGE_TYPES`, `INTERVAL_PRESETS`)
live in `frontend/src/components/config/enums.ts` and must **not** be
extended with protocol-specific entries.

### Manifest

```ts
// frontend/src/nodes/bacnet/index.ts
import type { Component } from 'vue'
import type { NodeGroupManifest } from '../types'

export const manifest: NodeGroupManifest = {
  name: 'bacnet',          // must match backend Group.Name
  category: 'industrial',  // palette colour key from nodes/tokens.ts
  flowEditors: {
    'bacnet-read':  () => import('./BACnetReadConfig.vue') as Promise<Component>,
    'bacnet-write': () => import('./BACnetWriteConfig.vue') as Promise<Component>,
  },
  configEditors: {
    'bacnet-device': () => import('./BACnetDeviceConfig.vue') as Promise<Component>,
  },
}
```

Use `category` when every node in the group shares one palette colour, or
`categories: { 'bacnet-read': 'industrial', 'bacnet-write': 'output' }`
for per-type overrides. For full-height code-style editors, list the
type strings in `fullHeightEditors`.

### Wire into the aggregator

One line in `frontend/src/nodes/index.ts`:

```ts
import { manifest as bacnetManifest } from './bacnet'

export const GROUPS: NodeGroupManifest[] = [
  // …existing groups
  bacnetManifest,
]
```

### Optional palette colour

If your group introduces a new colour family, add an entry to the `TOKENS`
map in `frontend/src/components/nodes/tokens.ts`:

```ts
bacnet: {
  accent:     '#a855f7',
  accentDim:  '#a855f712',
  accentBdr:  '#a855f72a',
  accentGlow: '#a855f714',
  bg:         '#0a060c',
  bgHdr:      '#180a1c',
  bgIcon:     '#20102a',
  border:     '#3a1a4a',
  textSub:    '#9a6abf',
},
```

Then reference `bacnet` in your manifest's `category` field.

## 3. Verify

```bash
# Backend
make build
go test ./internal/nodes/bacnet/...
go vet ./...

# Frontend
cd frontend
pnpm type-check
pnpm build
```

Start the app (`make dev`) and check:

- The new nodes appear in the palette under your group's section.
- Dragging one onto the canvas opens your config editor.
- The palette colour matches the category you set.
- For protocol nodes, the test-connection button (if implemented) works
  against your demo server.

## 4. Open the PR

- One logical change per PR — split unrelated drive-by edits.
- Add a short note to `CHANGELOG.md` under the unreleased section.
- For protocol nodes, link the spec from `specifications/issues/`.
- See [`CONTRIBUTING.md`](https://github.com/loopzedev/loopze-edge/blob/main/CONTRIBUTING.md)
  for the licensing terms (AGPL-3.0 inbound + dual-license grant to the
  maintainer).

## Reference: where things live

| What | Where |
|---|---|
| Node interface | `internal/flow/registry.go` (`flow.NodeInstance`) |
| Group registry | `internal/nodes/registry.go` |
| Embeddable callbacks | `internal/nodes/base.go` (`nodes.BaseNode`) |
| Conversion helpers | `internal/nodes/conv.go` |
| Property readers | `internal/nodes/props.go` |
| Config-node lookup | `internal/nodes/config_lookup.go` |
| TLS block parser | `internal/nodes/tls_config.go` |
| Test fixtures | `internal/nodes/nodestest/` |
| Bundled-groups list | `cmd/loopze/groups.go` |
| Frontend manifest type | `frontend/src/nodes/types.ts` |
| Frontend aggregator | `frontend/src/nodes/index.ts` |
| Palette colour tokens | `frontend/src/components/nodes/tokens.ts` |
| Shared frontend enums | `frontend/src/components/config/enums.ts` |
