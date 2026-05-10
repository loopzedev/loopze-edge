# Issue: XML Parser Node — Convert payload between XML string and structure

## Status: Proposed

## Context

Third parser from the concept in `PARSER_NODES.md` (Phase 3). HTTP APIs
from industrial systems, SOAP services, and legacy PLCs frequently deliver
XML payloads. Today those require a Function node with custom parsing code.
The XML Parser closes this gap with the same two-click UX as the JSON Parser.

## Problem description

`msg.payload` from HTTP-In or TCP-In often arrives as an **XML string** or
**`[]byte` (buffer)**. For Switch/Change/Template to access individual
fields, the payload must be converted to a structured Go value. When
sending in the other direction, the structure must be serialized back to
XML.

The **XML Parser node** does exactly that — bidirectional, with error
handling and without code.

## View / rationale

- **Operator UX**: same two-click experience as the JSON Parser — drop in,
  set property, done.
- **Same mental model**: Property, Action, and error behavior are identical
  to the JSON Parser. Operators already familiar with it need zero
  re-learning.
- **Industrial use case**: SOAP/WS-* APIs, OPC-UA data services, and Siemens
  SINEMA Remote Connect all deliver XML. Without a dedicated node, every
  integration requires a Function node.

## Requirements

### 1. Property

| Field | Description | Default |
|---|---|---|
| `property` | Property on the `msg` object (dot-path) | `payload` |

Phase 1: only `msg.<property>`. Scope selection (`flow`/`global`)
deliberately omitted — see `PARSER_NODES.md`.

### 2. Action

Dropdown determines the conversion direction:

| Value | Behavior |
|---|---|
| `auto` | **Default.** If value is `string` or `[]byte` → parse to map. Otherwise → serialize to XML string. |
| `parse` | Force parsing. Error if value is not a string/buffer. |
| `stringify` | Force serializing. Error if value is already a string. |

### 3. Root element (only for stringify)

| Field | Description | Default |
|---|---|---|
| `root` | Name of the wrapping root element for serialization | `"root"` |

Only relevant when action is `stringify` or `auto` (the auto-stringify
branch). For `parse` the field is hidden in the UI.

XML serialization requires a single root element by spec. The field is
mandatory for stringify — the UI shows a validation error when blank.

### 4. Pretty-print (only for stringify)

| Field | Description | Default |
|---|---|---|
| `indent` | Number of spaces for indentation. `0` = compact. | `0` |

UI: number input (0–8), only visible for action `stringify` or `auto`.

### 5. XML declaration

| Field | Description | Default |
|---|---|---|
| `declaration` | Prepend `<?xml version="1.0" encoding="UTF-8"?>` to output | `false` |

Only relevant for stringify/auto. Hidden for `parse`.

### 6. Map representation convention

Generic XML-to-map conversion follows this convention (same as
`github.com/clbanning/mxj`):

- Element children → map keys using the element name.
- Attributes → prefixed with `"-"`, e.g. `"-id"`, `"-type"`.
- Text content of a mixed element → key `"#text"`.
- Repeated sibling elements with the same name → `[]any` slice.

Example:

```xml
<sensor id="42" unit="C">
  <value>25.4</value>
  <label>Outdoor</label>
</sensor>
```

Parses to:

```json
{
  "sensor": {
    "-id": "42",
    "-unit": "C",
    "value": "25.4",
    "label": "Outdoor"
  }
}
```

> **Note:** All parsed values are strings — XML carries no type
> information. Use a Change or Function node for type coercion if needed.
> A `cast` option may be added in a later phase (see Out of scope).

### 7. Status / error handling

- Idle: no status.
- Parse error → status `red` / `"xml parse error"`, catchable error.
- Wrong type for configured action → status `red` / `"xml type error"`,
  catchable error.
- `root` blank on stringify → status `red` / `"xml root required"`,
  catchable error.
- Success: status stays unchanged (no green flash — avoids flicker at
  high frequency, same behavior as JSON/Change/Template).

### 8. Inputs / outputs

- **1 input**, **1 output**.
- Property overwritten **at the same path** — `msg.payload` in, `msg.payload`
  out.

## Examples

### Example 1 — Parse HTTP XML response

```
[HTTP-In]  →  [XML: action=auto]  →  [Switch: msg.payload.sensor.value > 30]
```

Input: `msg.payload = '<sensor><value>25.4</value></sensor>'` (string)
Output: `msg.payload = {sensor: {value: "25.4"}}` (map)

### Example 2 — Serialize to XML for SOAP request

```
[Change: msg.payload = {name: "Alice", age: 30}]
  →  [XML: action=stringify, root="Person", indent=2]
  →  [HTTP-Out]
```

Output:
```xml
<Person>
  <name>Alice</name>
  <age>30</age>
</Person>
```

### Example 3 — Parse buffer from TCP-In

```
[TCP-In]  →  [XML: action=parse]  →  [Debug]
```

Input: `msg.payload = []byte('<status ok="true"/>')`
Output: `msg.payload = {status: {"-ok": "true"}}`

### Example 4 — Round-trip with declaration

```
[XML: action=parse]  →  [Change]  →  [XML: action=stringify, declaration=true, indent=2]
```

Output begins with: `<?xml version="1.0" encoding="UTF-8"?>`

## Technical sketch

### Backend — `internal/nodes/core/parser_xml.go` (new)

```go
type XMLParserNode struct {
    config flow.NodeConfig
    nodes.BaseNode
    property    string
    action      string
    root        string
    indent      int
    declaration bool

    inErrorState bool
}
```

- **Inputs:** 1, **Outputs:** 1
- Implements `flow.NodeInstance`.
- No `ContextProvider` needed (no flow/global scope in Phase 1).

#### External dependency

Go's `encoding/xml` requires typed structs for marshaling — it cannot
decode into `map[string]any` generically. The recommended approach is the
external library **`github.com/clbanning/mxj/v2`**, which provides
exactly the required generic XML ↔ map conversion and is MIT-licensed.

Alternative: implement a custom token-based parser using `encoding/xml`
tokens. This avoids the dependency but is significantly more code and
harder to test exhaustively. Recommendation: use `mxj/v2`.

#### Action logic (pseudocode)

```go
func (n *XMLParserNode) convert(value any) (any, error) {
    switch n.action {
    case "parse":
        return parseXMLValue(value)
    case "stringify":
        return stringifyXMLValue(value, n.root, n.indent, n.declaration)
    case "auto":
        if isStringish(value) {
            return parseXMLValue(value)
        }
        return stringifyXMLValue(value, n.root, n.indent, n.declaration)
    }
}

func parseXMLValue(v any) (any, error) {
    var data []byte
    switch x := v.(type) {
    case string:
        data = []byte(x)
    case []byte:
        data = x
    default:
        return nil, fmt.Errorf("parse: expected string or []byte, got %T: %w", v, errXMLTypeMismatch)
    }
    m, err := mxj.NewMapXml(data)
    if err != nil {
        return nil, fmt.Errorf("parse: %w", err)
    }
    return m.Old(), nil  // returns map[string]any
}

func stringifyXMLValue(v any, root string, indent int, decl bool) (string, error) {
    if _, ok := v.(string); ok {
        return "", fmt.Errorf("stringify: value already a string: %w", errXMLTypeMismatch)
    }
    if root == "" {
        return "", fmt.Errorf("stringify: root element name required: %w", errXMLRootMissing)
    }
    m, err := mxj.NewMap(asStringMap(v))
    if err != nil {
        return "", fmt.Errorf("stringify: %w", err)
    }
    var b []byte
    if indent > 0 {
        b, err = m.XmlIndent("", strings.Repeat(" ", indent), root)
    } else {
        b, err = m.Xml(root)
    }
    if err != nil {
        return "", fmt.Errorf("stringify: %w", err)
    }
    if decl {
        return `<?xml version="1.0" encoding="UTF-8"?>` + "\n" + string(b), nil
    }
    return string(b), nil
}
```

### Node registration — `internal/server/server.go`

```go
registry.Register("xml", nodes.NewXMLParserNode, nodes.XMLParserTypeInfo())
```

```go
func XMLParserTypeInfo() flow.NodeTypeInfo {
    return flow.NodeTypeInfo{
        Type:        "xml",
        Category:    "parser",
        Label:       "XML",
        Description: "Convert XML strings to objects and back",
        Icon:        "xml",
        Defaults: map[string]any{
            "property":    "payload",
            "action":      "auto",
            "root":        "root",
            "indent":      0,
            "declaration": false,
        },
        Inputs:  1,
        Outputs: 1,
    }
}
```

### Frontend — `frontend/src/components/nodes/XMLParserNode.vue` (new)

- BaseNode with category `parser`.
- Body shows e.g. `payload · auto` or `payload → xml`.

### Frontend — `frontend/src/nodes/core/XMLParserConfig.vue` (new)

- Property input (dot-path).
- Action dropdown (`auto` / `parse` / `stringify`).
- Root element name input — only visible for `stringify` or `auto`, with
  validation error when blank.
- Indent number input (0–8) — only visible for `stringify` or `auto`.
- Declaration toggle — only visible for `stringify` or `auto`.
- All fields via `useNodeProperty`.

### Frontend — Wiring

- `frontend/src/views/FlowEditor.vue`: `<template #node-xml>` + import.
- `frontend/src/components/PropertyPanel.vue`: `<XMLParserConfig>` for
  `type === 'xml'`.
- `frontend/src/components/NodeIcon.vue`: icon entry for `xml`.

## Affected files

### Backend
- `internal/nodes/core/parser_xml.go` (new)
- `internal/nodes/core/parser_xml_test.go` (new) — auto/parse/stringify
  branches, string and `[]byte` inputs, attributes, repeated elements,
  pretty-print, declaration, error cases (invalid XML, wrong type, missing
  root).
- `internal/server/server.go` — registration.
- `go.mod` / `go.sum` — add `github.com/clbanning/mxj/v2`.

### Frontend
- `frontend/src/components/nodes/XMLParserNode.vue` (new)
- `frontend/src/nodes/core/XMLParserConfig.vue` (new)
- `frontend/src/views/FlowEditor.vue` — slot + import.
- `frontend/src/components/PropertyPanel.vue` — config mapping.
- `frontend/src/components/NodeIcon.vue` — icon entry.

## Dependencies

- `flow.Message` with `Get`/`Set` (exists)
- Catch node for error forwarding (exists)
- `github.com/clbanning/mxj/v2` (new) — MIT license, generic XML ↔ map

## Out of scope for Phase 1

- **Namespace handling** — namespaced attributes are included as-is in the
  key (`"ns:attr"`); full namespace resolution is not supported.
- **XSD validation** — separate node later.
- **XPath extraction** — use Change node after parsing.
- **Type casting** of parsed string values — all XML values remain strings;
  a `cast` option (number/boolean detection) may be added later.
- **Streaming for very large payloads** — no use case.
- **flow./global. scopes** — see `PARSER_NODES.md`.
- **CDATA sections** — treated as regular text content by mxj.

## Open questions

- **Dependency vs. custom parser**: use `mxj/v2` (faster, battle-tested)
  or implement token-based parsing with zero new dependencies? Suggestion:
  `mxj/v2` — the map-representation convention it defines is well-known
  and avoids reinventing a non-trivial parser.
- **Type name**: `xml` or `parser-xml`? Suggestion: `xml` — consistent
  with `json`.
- **Attribute prefix**: `"-"` (mxj default) or `"@"` (common in other
  ecosystems like BadgerFish/XPath)? Suggestion: `"-"` if using mxj,
  since switching costs more than the aesthetic choice.
- **Repeated element slices**: should `<a><x/><x/></a>` always yield a
  `[]any` for `x`, or only when there are ≥ 2 siblings? mxj uses the
  latter. Edge case: single-element payloads that become a slice on the
  next message are surprising. Acceptable for Phase 1; document in node
  tooltip.
