# Change Node

## Description

The Change node manipulates message properties, flow context, and global context without having to write code. It is the main tool for simple data manipulations and replaces a Function node in many cases.

## Operations

Each rule consists of an **operation**, a **target**, and (depending on the operation) a **value**.

### Operations (left dropdown)

| Operation | Description |
|---|---|
| **Set** | Sets a property to a value. Creates it if it does not exist. |
| **Change** | Searches and replaces text within a string property (regex or string match). |
| **Delete** | Removes a property completely from the object. |
| **Move** | Moves a property to another location (source is deleted). |

### Target Scope (dropdown in the property field)

| Scope | Description |
|---|---|
| **msg.** | Message property (e.g. `msg.payload`, `msg.topic`, `msg.myField`) |
| **flow.** | Flow context (key-value store, shared within the flow) |
| **global.** | Global context (key-value store, shared across all flows) |

### Value Types (dropdown in the value field)

For the **Set** operator, the value can come from various sources:

| Type | Icon | Description |
|---|---|---|
| **msg.** | — | Read value from another message property |
| **flow.** | — | Read value from the flow context |
| **global.** | — | Read value from the global context |
| **string** | `a/z` | Static string value |
| **number** | `0/9` | Static numeric value |
| **boolean** | `(o)` | `true` or `false` |
| **JSON** | `{}` | JSON object or array (parsed) |
| **buffer** | `01/10` | Buffer/byte array |
| **timestamp** | `clock` | Current Unix timestamp in milliseconds |
| **environment variable** | `$` | Read value from an environment variable |

## Storage Selection for Flow/Global Context

As soon as **flow.** or **global.** is chosen as scope or value type, an additional dropdown appears **after the text field** to select the storage type:

| Option | Description |
|---|---|
| **memory** | Volatile in-memory store (fast, data lost on restart) — **default** |
| **persistent** | File-backed persistent store (survives restarts) |

This applies to **all places** where flow/global appears as scope or value type:
- **Target scope** (property field): when `flow.` or `global.` -> storage dropdown appears
- **Value type** (value field for "Set"): when `flow.` or `global.` as source -> storage dropdown appears
- **Search type** (for "Change"): when `flow.` or `global.` -> storage dropdown appears
- **Replace type** (for "Change"): when `flow.` or `global.` -> storage dropdown appears
- **Target** (for "Move"): when `flow.` or `global.` -> storage dropdown appears

### UI Layout

```
Set v | v flow. [key         ] [memory v]
       to value | v global. [source_key  ] [persistent v]
```

### Backend Config Fields

New fields per rule for storage selection:

| Field | Description |
|---|---|
| `ps` | Property storage: `memory` or `persistent` (only when `pt` = flow/global) |
| `tos` | Value storage: `memory` or `persistent` (only when `tot` = flow/global) |
| `froms` | Search storage: `memory` or `persistent` (only when `fromt` = flow/global) |

## Rules UI

- Rules are presented as a **sortable list** (drag handle `=` on the left)
- Each rule has a **delete button** (`x`) on the right
- Below an **"+ add"** button for new rules
- Rules are executed **sequentially** from top to bottom
- Changes made by one rule are visible to subsequent rules

## Examples

### Set msg.payload to a String
```
Set | msg.payload | to the value | string: "Hello World"
```

### Copy msg.topic to msg.payload
```
Set | msg.payload | to the value | msg.topic
```

### Delete a Property
```
Delete | msg.temp
```

### Move Property
```
Move | msg.payload | to | msg.data.original
```

### Search & Replace (Change)

The Change operation has its **own layout** with two value fields:

```
Change | v msg. [property]
         Search for:    | v [type] [value]
         Replace with:  | v [type] [value]
```

**"Search for" types** (restricted):

| Type | Description |
|---|---|
| **msg.** | Search string from message property |
| **flow.** | Search string from flow context |
| **global.** | Search string from global context |
| **string** | Static search string |
| **regular expression** | Regex pattern (e.g. `foo\d+`) |
| **number** | Numeric value |
| **boolean** | true/false |
| **environment variable** | Search string from ENV |

**"Replace with" types**: identical, but without "regular expression".

Example:
```
Change | msg.payload | Search for: string "foo" | Replace with: string "bar"
```

### Set Timestamp
```
Set | msg.timestamp | to the value | timestamp
```

### Write Flow Context
```
Set | flow.lastValue | to the value | msg.payload
```

## Configuration (Backend)

```json
{
  "rules": [
    {
      "t": "set",
      "p": "payload",
      "pt": "msg",
      "to": "Hello World",
      "tot": "str"
    },
    {
      "t": "change",
      "p": "payload",
      "pt": "msg",
      "from": "foo",
      "fromt": "str",
      "to": "bar",
      "tot": "str",
      "fromRE": false
    },
    {
      "t": "delete",
      "p": "temp",
      "pt": "msg"
    },
    {
      "t": "move",
      "p": "payload",
      "pt": "msg",
      "to": "data.original",
      "tot": "msg"
    }
  ]
}
```

### Rule Fields

| Field | Description |
|---|---|
| `t` | Operation: `set`, `change`, `delete`, `move` |
| `p` | Property name (without scope prefix) |
| `pt` | Property scope: `msg`, `flow`, `global` |
| `ps` | Property storage: `memory` or `persistent` (only when `pt` = flow/global) |
| `to` | Target value or target property |
| `tot` | Value type: `msg`, `flow`, `global`, `str`, `num`, `bool`, `json`, `buf`, `date`, `env` |
| `tos` | Value storage: `memory` or `persistent` (only when `tot` = flow/global) |
| `from` | Search string (only for `change`) |
| `fromt` | Search type: `str`, `re` (regex) |
| `froms` | Search storage: `memory` or `persistent` (only when `fromt` = flow/global) |
| `fromRE` | Regex flag (only for `change`) |

## Implementation

### Backend (`internal/nodes/change.go`)

- **Inputs:** 1, **Outputs:** 1
- Parses `rules` array from `config.Properties`
- Executes rules sequentially on the message
- For `msg.*`: uses `msg.Get()` / `msg.Set()` / `msg.Delete()`
- For `flow.*` / `global.*`: uses `ContextStore.Get()` / `ContextStore.Set()`
- Implements `ContextProvider` (like FunctionNode) for flow/global context access
- For `change` (search/replace): `strings.Replace()` or `regexp.ReplaceAllString()`
- For `move`: get -> set at target -> delete at source
- For `date` (timestamp): `time.Now().UnixMilli()`
- For `env`: `os.Getenv()`

### Frontend

#### `ChangeConfig.vue`
- Sortable rule list (drag & drop ordering)
- Per rule: operation dropdown, scope dropdown, property input, value-type dropdown, value input
- "+" button at the bottom to add new rules
- "x" button on the right to delete a rule

#### `ChangeNode.vue`
- Uses BaseNode with category `process` (blue colors)
- Body shows compact summary of the rules (e.g. "3 rules")

### Node Registration

```go
registry.Register("change", nodes.NewChangeNode, nodes.ChangeTypeInfo())
```

```go
func ChangeTypeInfo() flow.NodeTypeInfo {
    return flow.NodeTypeInfo{
        Type:        "change",
        Category:    "function",
        Label:       "Change",
        Description: "Set, change, delete or move message properties",
        Icon:        "mdi-pencil",
        Defaults: map[string]any{
            "rules": []any{
                map[string]any{
                    "t": "set", "p": "payload", "pt": "msg",
                    "to": "", "tot": "str",
                },
            },
        },
        Inputs:  1,
        Outputs: 1,
    }
}
```

## Dependencies

- `flow.ContextStore` interface (already exists)
- `flow.ContextProvider` interface (already exists)
- `msg.Get()` / `msg.Set()` / `msg.Delete()` (already exist)
- Dot-path navigation in `msg.Get/Set` (already exists)
