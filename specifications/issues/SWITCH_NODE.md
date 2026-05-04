# Switch Node

## Description

The Switch node forwards incoming messages to one or more outputs based on configurable conditions. It is the central tool for **routing/branching** in a flow — analogous to an `if/elseif/else` or `switch` statement, but without code.

Counterpart to the Change node: while Change **manipulates** data, Switch only **routes** — the message is passed through unchanged, only the output is selected.

## Behavior

- **1 input**, **N outputs** (N = number of rules)
- Each rule corresponds to **one output port** (order = port order from top to bottom)
- For each incoming message, the rules are evaluated sequentially
- The message is sent **unchanged** to all matching outputs (no clone needed as long as the receiver does not mutate it — the implementation may need to clone defensively, see Implementation)

### Evaluation Modes

| Mode | Description |
|---|---|
| **stop after first match** (default) | As soon as a rule matches, further rules are skipped. Classic `if/elseif` behavior. |
| **check all rules** | All rules are evaluated; every matching rule sends to its output. A message can thus appear at multiple outputs. |

## Comparison Value / Property

As with the Change node, the **value to be checked** is selected via scope + property:

| Scope | Example |
|---|---|
| **msg.** | `msg.payload`, `msg.topic`, `msg.foo.bar` |
| **flow.** | Value from the flow context |
| **global.** | Value from the global context |

For `flow.`/`global.`, the **storage dropdown** (`memory` / `persistent`) appears additionally — identical to the Change node.

## Operators (per rule)

### Value Comparisons

| Operator | Symbol | Description |
|---|---|---|
| **==** | `==` | Equality (loose, with type conversion) |
| **!=** | `!=` | Inequality |
| **<** | `<` | Less than |
| **<=** | `<=` | Less than or equal |
| **>** | `>` | Greater than |
| **>=** | `>=` | Greater than or equal |
| **is between** | `[a..b]` | Value lies in the interval [a, b] (two value fields) |
| **contains** | `⊃` | String/array contains value |
| **matches regex** | `.*` | Regex match on string property |

### Type Checks (no value needed)

| Operator | Description |
|---|---|
| **is true** | Value is `true` |
| **is false** | Value is `false` |
| **is null** | Value is `null` or completely missing |
| **is not null** | Value exists and is not null |
| **is empty** | String/array/object is empty |
| **is not empty** | Counterpart to `is empty` |
| **is of type** | Comparison against type dropdown: `string`, `number`, `boolean`, `array`, `object`, `buffer`, `null`, `undefined` |

### Default

| Operator | Description |
|---|---|
| **otherwise** | Catch-all. Matches exactly when **no** previous rule matched in `stop after first match` mode. Should be placed as the last rule. |

## Value Types (right of the operator)

Identical to the Change node — comparison values can be static or come from other sources:

| Type | Description |
|---|---|
| **msg.** | Value from another message property |
| **flow.** | Value from flow context (with storage dropdown) |
| **global.** | Value from global context (with storage dropdown) |
| **string** | Static string |
| **number** | Static number |
| **boolean** | `true` / `false` |
| **JSON** | Parsed JSON object/array |
| **environment variable** | Value from ENV |
| **previous value** | Value from the last evaluation of this property (only with operators where it makes sense — e.g. `!=` for "has changed") |

## UI Layout

```
Property:    v msg. [payload                        ]

Rules:
  =  v ==           v string  [active        ]   x      -> Output 1
  =  v >            v number  [10            ]   x      -> Output 2
  =  v matches re   v string  [^err_         ]   x      -> Output 3
  =  v otherwise                                  x      -> Output 4
       [ + Add rule ]

Mode:  (o) stop after first match   ( ) check all rules
```

- Sortable list (drag handle `=`) — order determines **port order**
- Per rule: operator dropdown, value-type dropdown, value input (two inputs for `is between`/`index between`), delete button
- When a rule is added/removed, an output port is added/removed — existing wires stay attached to their rule (wire map by rule ID, not port index)

> See also `NODE_AND_MULTIOUTPUT.md` — the multi-output scaling problems described there must be cleanly solved for the Switch node, otherwise the UI becomes unusable.

## Configuration (Backend)

```json
{
  "property": "payload",
  "propertyType": "msg",
  "propertyStorage": "memory",
  "checkall": false,
  "rules": [
    { "id": "r1", "t": "eq",      "v": "active",  "vt": "str" },
    { "id": "r2", "t": "gt",      "v": "10",       "vt": "num" },
    { "id": "r3", "t": "regex",   "v": "^err_",   "vt": "str", "case": false },
    { "id": "r4", "t": "btwn",    "v": "0",  "vt": "num", "v2": "100", "v2t": "num" },
    { "id": "r5", "t": "else" }
  ]
}
```

### Top-Level Fields

| Field | Description |
|---|---|
| `property` | Name of the property to check (without scope prefix) |
| `propertyType` | Scope: `msg`, `flow`, `global` |
| `propertyStorage` | `memory` / `persistent` (only for flow/global) |
| `checkall` | `false` = stop after first match (default), `true` = check all rules |

### Rule Fields

| Field | Description |
|---|---|
| `id` | Stable ID (for wire mapping across order changes) |
| `t` | Operator (see operator table, abbreviations below) |
| `v` | Comparison value |
| `vt` | Value type: `msg`, `flow`, `global`, `str`, `num`, `bool`, `json`, `env`, `prev` |
| `vs` | Storage for `v` (only when `vt` = flow/global) |
| `v2` | Second value (only for `btwn`, `idxbtwn`) |
| `v2t` | Type for `v2` |
| `v2s` | Storage for `v2` |
| `case` | For `regex`/`cont`: case-sensitivity flag |

### Operator Abbreviations (`t`)

| Abbrev | Operator |
|---|---|
| `eq` / `neq` | `==` / `!=` |
| `lt` / `lte` / `gt` / `gte` | Comparisons |
| `btwn` | `is between` |
| `cont` | `contains` |
| `regex` | `matches regex` |
| `true` / `false` | `is true` / `is false` |
| `null` / `nnull` | `is null` / `is not null` |
| `empty` / `nempty` | `is empty` / `is not empty` |
| `istype` | `is of type` (type in `v`) |
| `else` | `otherwise` (catch-all) |

## Examples

### Routing by Status String
```
Property: msg.payload.status
  == "ok"      -> Output 1
  == "warn"    -> Output 2
  == "error"   -> Output 3
  otherwise    -> Output 4
```

### Threshold Splitting
```
Property: msg.payload
  <  10        -> Output 1 (low)
  is between 10..50  -> Output 2 (mid)
  >  50        -> Output 3 (high)
```

### Topic Filter via Regex
```
Property: msg.topic
  matches regex "^sensor/temp/"  -> Output 1
  matches regex "^sensor/hum/"   -> Output 2
  otherwise                       -> Output 3 (unknown)
```

### Existence Filter
```
Property: msg.payload.userId
  is not null  -> Output 1 (process)
  otherwise    -> Output 2 (discard / log)
```

## Implementation

### Backend (`internal/nodes/switch.go`)

- **Inputs:** 1, **Outputs:** dynamic = `len(rules)`
- Implements `flow.NodeInstance` and `flow.ContextProvider` (for flow/global lookups)
- `Init()` parses `rules`, compiles regex once if applicable
- `OnMessage(msg)`:
  1. Get property value via scope (msg/flow/global)
  2. Iterate over rules — for each matching rule call `n.send(msg, outputIdx)`
  3. In `stop after first match` mode, abort after the first match
  4. `else` matches only when nothing else matched (also in `checkall` mode)
- Defensive copy of the message **only** when `checkall=true` and multiple outputs match — otherwise pointer pass

### Type Conversion

- Comparisons use the same conversion logic as the Change node (`valuetype.go`)
- `==`/`!=` with loose type conversion (e.g. `"10" == 10` is true)
- Strict comparisons (`===`) deliberately omitted — can be added later if desired

### Frontend

#### `SwitchConfig.vue`
- Reuse the building blocks from `ChangeConfig.vue`:
  - `MsgFieldEditor` for property selection with scope + storage
  - `ValueTypeInput` for the comparison values
  - `FormSelect` for operator dropdown
- Sortable rule list (drag & drop) — order maps to port order
- Mode toggle (radio: stop after first / check all)

#### `SwitchNode.vue`
- BaseNode with category `function` (or new category `routing` if it should be visually distinct)
- Body shows property + rule count, e.g. `msg.payload * 4 rules`
- Dynamic height depending on output count (see `NODE_AND_MULTIOUTPUT.md`)

### Node Registration

```go
registry.Register("switch", nodes.NewSwitchNode, nodes.SwitchTypeInfo())
```

```go
func SwitchTypeInfo() flow.NodeTypeInfo {
    return flow.NodeTypeInfo{
        Type:        "switch",
        Category:    "function",
        Label:       "Switch",
        Description: "Route messages based on property values",
        Icon:        "mdi-call-split",
        Defaults: map[string]any{
            "property":     "payload",
            "propertyType": "msg",
            "checkall":     false,
            "rules": []any{
                map[string]any{"id": "r1", "t": "eq", "v": "", "vt": "str"},
                map[string]any{"id": "r2", "t": "else"},
            },
        },
        Inputs:  1,
        Outputs: 2, // derived at runtime from len(rules)
    }
}
```

> **Open:** How is a node type with a **dynamic** output count registered? Currently `Outputs` is a static int. This must be solved either via a function `OutputsFunc(props) int` or by computation on editor save.

## Dependencies

- `flow.ContextStore` / `flow.ContextProvider` — present (used by Change node)
- `msg.Get()` with dot path — present
- `valuetype.go` — reuse type conversion
- Multi-output handling in the engine — present (Function node has it), see `NODE_AND_MULTIOUTPUT.md` for open UI issues
- Frontend: `MsgFieldEditor`, `ValueTypeInput`, `FormSelect` from `components/config/`

## Open Questions

1. **Dynamic output count** in `NodeTypeInfo` — agree on a pattern, possibly a separate issue.
2. **`previous value` type** — needs per-node state between messages. MVP or skip?
3. **`is of type`** — which types are actually supported (Buffer? Date?).
4. **Strict comparisons** (`===`/`!==`) — wanted or deliberately omitted?

> Sequence-related operators (`head`/`tail`/`index between`) were deliberately omitted — see `SPLIT_JOIN_NODE.md`. They presuppose a `msg.parts` concept that LOOPZE does not have today.
