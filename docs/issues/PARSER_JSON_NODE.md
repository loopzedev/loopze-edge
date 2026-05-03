# Issue: JSON Parser Node — Convert payload between JSON string and structure

## Status: Proposed

## Context

First concrete parser from the concept in `PARSER_NODES.md`. Anyone who
wires up MQTT topics, HTTP APIs, or NATS subjects with JSON payloads
today needs a Function node with `JSON.parse` / `JSON.stringify` —
a clear case for a declarative solution.

## Problem description

`msg.payload` arrives from MQTT-In or HTTP-In typically as a
**string** or **`[]byte` (buffer)**. For Switch/Change/Template to
access individual fields, the payload must be parsed to a structured
value (`map`, `[]any`, scalar). When sending in the other direction,
it must be serialized back to a string.

The **JSON Parser node** does exactly that — bidirectional, with
error handling and without code.

## View / rationale

- **Operator UX**: two clicks (drop-in, check property) instead of
  Function node + JS code.
- **Most common task in any flow**: every MQTT connection with a
  structured payload needs it.
- **Consistent with the Template node** — it already has `format: json`
  as output post-processing; the JSON parser is the counterpart for the
  input side.

## Requirements

### 1. Property

| Field | Description | Default |
|---|---|---|
| `property` | Property on the `msg` object (dot-path) | `payload` |

Phase 1: only `msg.<property>`. Scope selection (`flow`/`global`) deliberately
omitted — see `PARSER_NODES.md`.

### 2. Action

Dropdown determines the conversion direction:

| Value | Behavior |
|---|---|
| `auto` | **Default.** If value is `string` or `[]byte` → parse to structure. Otherwise → stringify to string. |
| `parse` | Force parsing. Error if value is not a string/buffer. |
| `stringify` | Force serializing. Error if value is already a string. |

`auto` is deliberately the default: in practice the direction is clear
from the preceding node (MQTT-In → parse, before MQTT-Out → stringify).
Whoever needs strictness switches to `parse` / `stringify`.

### 3. Pretty-print (only for stringify)

| Field | Description | Default |
|---|---|---|
| `indent` | Number of spaces for `json.MarshalIndent`. `0` = compact (no indent). | `0` |

UI: number input (0–8), only visible for action `stringify` or `auto`.

### 4. Status / error handling

- Idle: no status.
- Parse error (`json.Unmarshal` fails) →
  status `red` / `"json parse error"`, catchable error.
- Value has wrong type for the chosen action →
  status `red` / `"json type error"`, catchable error.
- Success: status stays unchanged (no "green" for every
  message — avoids status flicker at high frequency, analogous to
  Change/Template).

### 5. Inputs / outputs

- **1 input**, **1 output**.
- Property is overwritten **at the same property path** — i.e.
  `msg.payload` in, `msg.payload` out (in the respective other form).

## Examples

### Example 1 — Parse MQTT JSON

```
[MQTT-In: sensor/temp]  →  [JSON: action=auto]  →  [Switch: msg.payload.value > 30]
```

Input: `msg.payload = '{"value": 25.4, "unit": "C"}'` (string)
Output: `msg.payload = {value: 25.4, unit: "C"}` (object)

### Example 2 — Serialize HTTP body

```
[Function: msg.payload = {ok:true}]  →  [JSON: action=stringify, indent=2]  →  [HTTP-Out]
```

Input: `msg.payload = {ok: true}` (object)
Output: `msg.payload = "{\n  \"ok\": true\n}"` (string, pretty)

### Example 3 — Parse buffer from Modbus

```
[Modbus-In: 0/0..63]  →  [JSON: action=parse]  →  [Debug]
```

Input: `msg.payload = []byte('{"reg0": 1234}')`
Output: `msg.payload = {reg0: 1234}`

## Technical sketch

### Backend — `internal/nodes/parser_json.go` (new)

```go
type JSONParserNode struct {
    config flow.NodeConfig
    send   flow.SendFunc
    status flow.StatusFunc

    property string  // dot-path, default "payload"
    action   string  // "auto" | "parse" | "stringify"
    indent   int     // 0..8
}
```

- **Inputs:** 1, **Outputs:** 1
- Implements `flow.NodeInstance`.
- No `ContextProvider` needed (no flow/global scope in Phase 1).
- In `HandleMessage`:
  1. Read value via `msg.Get(n.property)`.
  2. Action logic (see below).
  3. Write result back via `msg.Set(n.property, …)`.
  4. `send(0, msg)`.

### Action logic (pseudocode)

```go
func (n *JSONParserNode) convert(value any) (any, error) {
    switch n.action {
    case "parse":
        return parseAny(value)
    case "stringify":
        return stringify(value, n.indent)
    case "auto":
        if isStringish(value) {
            return parseAny(value)
        }
        return stringify(value, n.indent)
    }
}

func parseAny(v any) (any, error) {
    var data []byte
    switch x := v.(type) {
    case string:
        data = []byte(x)
    case []byte:
        data = x
    default:
        return nil, fmt.Errorf("json parse: expected string or []byte, got %T", v)
    }
    var parsed any
    if err := json.Unmarshal(data, &parsed); err != nil {
        return nil, err
    }
    return parsed, nil
}

func stringify(v any, indent int) (string, error) {
    if _, ok := v.(string); ok {
        return "", fmt.Errorf("json stringify: value already a string")
    }
    if indent > 0 {
        b, err := json.MarshalIndent(v, "", strings.Repeat(" ", indent))
        return string(b), err
    }
    b, err := json.Marshal(v)
    return string(b), err
}
```

### Node registration — `internal/server/server.go`

```go
registry.Register("json", nodes.NewJSONParserNode, nodes.JSONParserTypeInfo())
```

```go
func JSONParserTypeInfo() flow.NodeTypeInfo {
    return flow.NodeTypeInfo{
        Type:        "json",
        Category:    "parser",
        Label:       "JSON",
        Description: "Convert JSON strings to objects and back",
        Icon:        "json",
        Defaults: map[string]any{
            "property": "payload",
            "action":   "auto",
            "indent":   0,
        },
        Inputs:  1,
        Outputs: 1,
    }
}
```

> **Open — category:** Introduce a new palette category `parser`,
> or run alongside `function`? Suggestion: new category,
> so XML/CSV later sit visually together. Align with
> `PROPERTIES_PANEL.md` / palette convention.

### Frontend — `frontend/src/components/nodes/JSONParserNode.vue` (new)

- BaseNode with category `parser` (or `function`, see above).
- Body shows e.g. `payload · auto` or `payload → string`.

### Frontend — `frontend/src/components/config/JSONParserConfig.vue` (new)

- Property input (dot-path).
- Action dropdown (`auto` / `parse` / `stringify`).
- Indent number input (0–8), only visible for `stringify` or `auto`.
- All fields via `useNodeProperty`.

### Frontend — Wiring

- `frontend/src/views/FlowEditor.vue`: `<template #node-json>` + import.
- `frontend/src/components/PropertyPanel.vue`: `<JSONParserConfig>` for `type === 'json'`.
- `frontend/src/components/NodeIcon.vue`: icon entry for `json`.

## Affected files

### Backend
- `internal/nodes/parser_json.go` (new)
- `internal/nodes/parser_json_test.go` (new) — auto/parse/stringify
  branches, string and `[]byte` inputs, pretty-print, error cases
  (invalid JSON, wrong type).
- `internal/server/server.go` — registration.

### Frontend
- `frontend/src/components/nodes/JSONParserNode.vue` (new)
- `frontend/src/components/config/JSONParserConfig.vue` (new)
- `frontend/src/views/FlowEditor.vue` — slot + import.
- `frontend/src/components/PropertyPanel.vue` — config mapping.
- `frontend/src/components/NodeIcon.vue` — icon entry.

## Dependencies

- `flow.Message` with `Get`/`Set` (exists)
- Catch node for error forwarding (exists)
- Standard library: `encoding/json` (no new module)

## Out of scope for Phase 1

- **flow./global. scopes** — see `PARSER_NODES.md`.
- **JSON schema validation** — separate node later.
- **JSONPath / sub-path extraction** — Change node covers that after
  parsing.
- **Streaming for very large payloads** — no use case.

## Open questions

- **Palette category**: new group `parser` or sort into `function`?
  (see above)
- **Type name**: `json` (short, clear) or `parser-json` (explicit
  namespace)? Suggestion `json` — analogous to Node-RED, shorter.
- **Indent range**: hard cap at 8 or leave more open?
  Suggestion: 0–8 is practically enough.
