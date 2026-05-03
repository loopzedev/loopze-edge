# Issue: Inject Node — rules system like Change Node

## Status: Open

## Problem description

The Inject Node currently has hardcoded `payload` + `topic` fields. The user can configure only one payload and one topic. Node-RED's Inject Node, in contrast, allows a **dynamic list of properties** that are set on the message — exactly like the "Set" rules of the Change Node.

**Goal:** The Inject Node gets a rules system analogous to the Change Node. Each rule defines a property to set on the outgoing message. Rules can be reordered via drag & drop, added, and removed.

## Current (will be replaced)

```json
{
  "once": false,
  "interval": 0,
  "payloadType": "date",
  "payload": "rfc3339",
  "topic": "my/topic",
  "topicType": "str"
}
```

Hardcoded: exactly 1 payload + 1 topic, not extensible.

## New: rules-based property list

### Configuration

```json
{
  "once": false,
  "interval": 0,
  "props": [
    { "p": "payload", "vt": "date", "v": "rfc3339" },
    { "p": "topic",   "vt": "str",  "v": "sensors/temperature" },
    { "p": "qos",     "vt": "num",  "v": "1" }
  ]
}
```

Each rule in `props` defines:

| Field | Description |
|---|---|
| `p` | Property name on `msg` (e.g. `payload`, `topic`, `qos`, `retain`, arbitrary) |
| `vt` | Value type: `str`, `num`, `bool`, `json`, `date`, `env`, `flow`, `global` |
| `v` | Value (depends on type) |
| `vs` | Storage: `memory` or `persistent` (only for `flow`/`global`) |

**No `msg` type** — the Inject Node has no incoming message.

### Default rules

A new Inject Node starts with two default rules:

```json
[
  { "p": "payload", "vt": "date", "v": "rfc3339" },
  { "p": "topic",   "vt": "str",  "v": "" }
]
```

### Supported value types

| Type | `vt` | `v` content | Description |
|---|---|---|---|
| **String** | `str` | `"Hello world"` | Static string |
| **Number** | `num` | `"42.5"` | Numeric value (float64) |
| **Boolean** | `bool` | `"true"` | true/false |
| **JSON** | `json` | `'{"key": "val"}'` | JSON object or array |
| **Timestamp** | `date` | `"rfc3339"` or `"epoch"` | Current timestamp |
| **Env variable** | `env` | `"MY_VAR"` | Read environment variable |
| **Flow context** | `flow` | `"myKey"` | Read value from flow context |
| **Global context** | `global` | `"myKey"` | Read value from global context |

## Implementation

### Backend (`inject.go`)

The `emit()` method iterates over all rules and sets the properties on the message:

```go
func (n *InjectNode) emit() {
    msg := flow.NewMessage()
    for _, rule := range n.props {
        val, err := ResolveValue(rule.vt, rule.v, rule.vs, nil, n.valueContext())
        if err != nil {
            slog.Warn("inject: resolve error", "prop", rule.p, "error", err)
            continue
        }
        if val != nil {
            msg.Set(rule.p, val)
        }
    }
    n.send(0, msg)
}
```

**Config parsing:** `Init()` reads `props` as `[]map[string]any` (analogous to `rules` in the Change Node).

**ContextProvider:** already implemented — InjectNode implements `SetContext()`.

### Shared value-type component (frontend)

**Already exists:** `ValueTypeInput.vue` — reused.

### Frontend (`InjectConfig.vue`)

The properties panel gets the same rule list as the Change Node:

```
┌─────────────────────────────────────────────────┐
│ ☐ Inject once at startup                        │
│                                                  │
│ Repeat Interval                                  │
│ [None] [100ms] [500ms] [1s] [5s] [10s] ...     │
│ [________] ms                                    │
│                                                  │
│ Properties                                       │
│ ┌───────────────────────────────────────────┐   │
│ │ ≡  msg.[payload ] [timestamp▾] [rfc3339▾] │   │
│ │ ≡  msg.[topic   ] [string   ▾] [________] │   │
│ └───────────────────────────────────────────┘   │
│ [+ add property]                                 │
└─────────────────────────────────────────────────┘
```

Each row contains:
- **Drag handle** (`≡`) for ordering via drag & drop
- **Property name** (FormInput, e.g. `payload`, `topic`, `qos`)
- **ValueTypeInput** (type dropdown + value input + optional storage)
- **Delete button** (`✕`)

The drag & drop logic is reused from `ChangeConfig.vue` (same `onDragStart`/`onDragOver`/`onDrop` pattern).

### Shared value resolution (backend)

**Already exists:** `valuetype.go` with `ResolveValue()` — reused.

### Existing functionality (unchanged)

- Trigger modes: Manual, Once, Interval, Once+Interval
- TRIG button on canvas
- API endpoint `POST /api/v1/inject/{id}`
- Node display on canvas (InjectNode.vue)
- Interval presets in the property panel

## Affected files

| File | Change |
|---|---|
| `internal/nodes/inject.go` | Rules parsing instead of hardcoded payload/topic, `emit()` iterates rules |
| `internal/nodes/inject_test.go` | Tests for multi-rule, ordering, various value types |
| `frontend/src/components/config/InjectConfig.vue` | Rule list with drag & drop, uses `ValueTypeInput` |

## Tests

| Test | Verifies |
|---|---|
| `TestInjectSingleRule` | One rule: payload=str |
| `TestInjectMultipleRules` | Multiple rules: payload + topic + custom property |
| `TestInjectRuleOrder` | Order of rules is preserved |
| `TestInjectDefaultRules` | Without props config → default payload=timestamp |
| `TestInjectPayloadDateEpoch` | date type with epoch |
| `TestInjectPayloadJSON` | json type |
| `TestInjectPayloadEnv` | env type |
| `TestInjectEmptyRules` | Empty props list → message without properties |
