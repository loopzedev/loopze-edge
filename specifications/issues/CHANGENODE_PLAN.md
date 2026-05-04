# Change Node — implementation plan

## Context

The Change Node is the second core processing node after the Function Node. It enables data manipulation without code — set, change, delete, move properties. Specification: `issues/CHANGENODE.md`.

A lot of infrastructure already exists: icon, token category, FlowEditor slot, ContextProvider interface, message dot-path API.

---

## New files

### 1. `internal/nodes/change.go` (~400 lines)

**Struct:**
```go
type Rule struct {
    Type     string // "set", "change", "delete", "move"
    Property string // target property (without scope)
    PropType string // "msg", "flow", "global"
    To       string // value or target property
    ToType   string // "msg", "flow", "global", "str", "num", "bool", "json", "date", "env"
    From     string // search string (only for "change")
    FromType string // "str", "re", "num", "bool", "env"
    FromRE   bool   // regex flag
}

type ChangeNode struct {
    config   flow.NodeConfig
    send     flow.SendFunc
    status   flow.StatusFunc
    debug    flow.DebugFunc
    ctxMem, ctxPers     flow.ContextStore  // global
    flowMem, flowPers   flow.ContextStore  // flow-scoped
    rules    []Rule
}
```

**Init():** Parses `rules` array from `config.Properties`. Each rule is mapped from `map[string]any`. With `fromRE: true`, the regex is precompiled (`regexp.Compile`).

**HandleMessage():**
```
for each rule:
  1. Resolve target value (resolveValue):
     - "str" → use literal
     - "num" → strconv.ParseFloat
     - "bool" → literal true/false
     - "json" → json.Unmarshal
     - "date" → time.Now().UnixMilli()
     - "env" → os.Getenv()
     - "msg" → msg.Get(path)
     - "flow" → flowMem.Get(key)
     - "global" → ctxMem.Get(key)

  2. Apply operation:
     - "set":
       - pt=msg → msg.Set(property, value)
       - pt=flow → flowMem.Set(property, value)
       - pt=global → ctxMem.Set(property, value)
     - "delete":
       - pt=msg → msg.Delete(property)
       - pt=flow → flowMem.Delete(property)
       - pt=global → ctxMem.Delete(property)
     - "move":
       - Get from source → Set at target → Delete source
     - "change":
       - Get current string value
       - fromRE=true → regexp.ReplaceAllString
       - fromRE=false → strings.ReplaceAll
       - Set modified value back

Return message on port 0.
```

**SetContext():** Implements `flow.ContextProvider` (identical to FunctionNode).

**ChangeTypeInfo():**
```go
Type: "change", Category: "function", Label: "Change"
Inputs: 1, Outputs: 1
Defaults: { rules: [{ t:"set", p:"payload", pt:"msg", to:"", tot:"str" }] }
```

Existing APIs that are reused:
- `msg.Get/Set/Delete` (`internal/flow/types.go:145-218`)
- `ContextStore.Get/Set/Delete` (`internal/flow/context.go:8-20`)
- `ContextProvider` interface (`internal/flow/context.go:31-33`)

### 2. `internal/nodes/change_test.go` (~300 lines)

Tests for each operation:
- `TestChangeNode_SetMsgProperty` — set msg.payload to string
- `TestChangeNode_SetNested` — set msg.data.nested.field
- `TestChangeNode_SetFromMsg` — copy msg.topic → msg.payload
- `TestChangeNode_SetTimestamp` — set to current timestamp
- `TestChangeNode_SetJSON` — set to JSON object
- `TestChangeNode_SetNumber` — set to number
- `TestChangeNode_SetBoolean` — set to boolean
- `TestChangeNode_Delete` — delete property
- `TestChangeNode_Move` — move property
- `TestChangeNode_ChangeString` — search/replace string
- `TestChangeNode_ChangeRegex` — search/replace regex
- `TestChangeNode_MultipleRules` — multiple rules sequentially
- `TestChangeNode_FlowContext` — set/read flow.* context
- `TestChangeNode_GlobalContext` — set/read global.* context

### 3. `frontend/src/components/config/ChangeConfig.vue` (~400 lines)

Most complex config component — rule list with dynamic layout per operation.

**Structure per rule:**
```
┌─────────────────────────────────────────────────┐
│ ≡  [Operation ▼]  [▼ scope] [property    ]  ✕  │
│                                                  │
│    (for "set":)                                  │
│    to the value   [▼ type] [value        ]      │
│                                                  │
│    (for "change":)                               │
│    Search for     [▼ type] [search       ]      │
│    Replace with   [▼ type] [replace      ]      │
│                                                  │
│    (for "move":)                                 │
│    to              [▼ scope] [property   ]      │
│                                                  │
│    (for "delete": no extra fields)               │
└─────────────────────────────────────────────────┘
```

**Component design:**
- `rules` as computed array from `config.rules`
- `updateRule(index, field, value)` — updates a single field
- `addRule()` — adds default rule
- `removeRule(index)` — removes rule
- `moveRule(from, to)` — moves rule (drag & drop or up/down buttons)

**Dropdowns:**
- Operation: `set | change | delete | move`
- Scope: `msg. | flow. | global.`
- Value type (set): `msg. | flow. | global. | string | number | boolean | JSON | buffer | timestamp | environment variable`
- Search type (change): `msg. | flow. | global. | string | regular expression | number | boolean | environment variable`

**Styling:** Consistent with InjectConfig.vue — `terminal-input`, `terminal-border`, `text-terminal-text-dim`, button toggles like in ContextWatchConfig.

### 4. `frontend/src/components/nodes/ChangeNode.vue` (~30 lines)

Simple wrapper around BaseNode:
```vue
<BaseNode node-type="change" :inputs="1" :outputs="1">
  <template #body>
    <span>{{ ruleCount }} rule{{ ruleCount !== 1 ? 's' : '' }}</span>
  </template>
</BaseNode>
```

---

## Existing files — changes

### 5. `internal/server/server.go` (1 line)

```go
// In registerNodes(), after context-watch:
registry.Register("change", nodes.NewChangeNode, nodes.ChangeTypeInfo())
```

### 6. `frontend/src/components/PropertyPanel.vue` (2 lines)

```vue
// Add import:
import ChangeConfig from '@/components/config/ChangeConfig.vue'

// Add condition after ContextWatchConfig:
<ChangeConfig v-else-if="selectedNode?.type === 'change'" />
```

### 7. `frontend/src/views/FlowEditor.vue` (3 lines)

Replace existing slot (`<BaseNode v-bind="nodeProps as any" />`) with:
```vue
import ChangeNode from "@/components/nodes/ChangeNode.vue";

<template #node-change="nodeProps">
    <ChangeNode v-bind="nodeProps as any" />
</template>
```

### 8. `frontend/src/components/nodes/BaseNode.vue` (1 line)

Extend TypeLabel map (if not already present — check):
```typescript
'change': 'Change',
```

---

## Already present (no action needed)

- `tokens.ts`: `change: 'process'` → blue colors ✅
- `NodeIcon.vue`: `change` icon (bidirectional arrows) ✅
- `FlowEditor.vue`: `#node-change` template slot ✅
- `ContextProvider` interface ✅
- `msg.Get/Set/Delete` with dot-path navigation ✅

---

## Implementation order

1. **Backend: `change.go`** — Rule struct, Init, HandleMessage with all 4 operations
2. **Backend: `change_test.go`** — tests for all operations
3. **Backend: `server.go`** — registration
4. **`go build && go test`** — verify backend
5. **Frontend: `ChangeNode.vue`** — node component
6. **Frontend: `ChangeConfig.vue`** — config panel with rule list
7. **Frontend: `PropertyPanel.vue` + `FlowEditor.vue`** — wiring
8. **`npm run type-check`** — verify frontend

---

## Verification

### Backend
```bash
go build ./...
go test -run TestChangeNode -v ./internal/nodes/
```

### Frontend
```bash
cd frontend && npm run type-check
```

### End-to-end
1. Start LOOPZE (`make build-all && ./bin/loopze`)
2. Drag Change Node into flow
3. Open properties → configure rules:
   - Set msg.payload to "Hello"
   - Delete msg.topic
4. Wire Inject → Change → Debug
5. Deploy + trigger Inject
6. Debug panel: message has payload="Hello", no topic
