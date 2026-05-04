# Issue: Template Node — build texts from templates

## Status: Proposed

## Problem description

Currently there is no easy way to produce multi-line texts with variables from
`msg`, `flow.` or `global.`. Anyone who wants to assemble e.g. an MQTT payload, a
notification, or a JSON object from multiple sources today has to fall back on
the Function Node — code for a task that is more elegant when declarative.

In Node-RED, the **Template Node** does this: a text field with
Mustache placeholders, the result lands as a string in `msg.payload`
(or another property).

## View / rationale

- **Operator UX**: a textarea with `{{payload}}` placeholders is much more
  accessible to non-programmers than JS code in the Function Node.
- **Low complexity**: Mustache as a template language is small, well-established,
  has Go libs (`github.com/cbroglie/mustache` etc.).
- **Pattern already there**: scope selection (`msg`/`flow`/`global`) including
  storage switch (`memory`/`persistent`) exists in the Change Node — Template
  Node uses the same mental model.
- **Common use cases**:
  - Notification texts: `"Sensor {{topic}} reports {{payload}}°C"`
  - JSON bodies for HTTP/MQTT: `{"id": "{{flow.deviceId}}", "v": {{payload}}}`
  - Log lines, status messages, dashboard texts

## Requirements

### 1. Template source

A **textarea** in the properties panel with Mustache syntax.

| Placeholder | Meaning |
|---|---|
| `{{payload}}` | `msg.payload` |
| `{{topic}}` | `msg.topic` |
| `{{<dot.path>}}` | arbitrary `msg.<dot.path>` |
| `{{flow.<key>}}` | value from flow context |
| `{{global.<key>}}` | value from global context |
| `{{{value}}}` | unescaped (relevant with output format `html`) |

Mustache sections (`{{#list}}…{{/list}}`, `{{^missing}}…{{/missing}}`) are
supported — standard Mustache behavior, no special semantics.

### 2. Output target

| Field | Description | Default |
|---|---|---|
| `field` | Property name on the output (dot-path) | `payload` |
| `fieldType` | Scope: `msg`, `flow`, `global` | `msg` |

For `flow`/`global`, a storage dropdown (`memory` / `persistent`) appears
**behind the property field** — analogous to Change Node.

### 3. Output format

A dropdown determines post-processing of the rendered string:

| Value | Behavior |
|---|---|
| `plain` | String is output as-is (default) |
| `json` | Result is parsed with `json.Unmarshal` — on error: catchable error |
| `yaml` | Result is parsed with YAML parser — on error: catchable error |

Phase 1 implements only `plain` and `json`. `yaml` is nice-to-have, can be
added once needed.

### 4. Syntax mode

Dropdown:

| Value | Description |
|---|---|
| `mustache` | Template is rendered (default) |
| `plain` | Template is passed through 1:1 — useful for static texts with `{{`/`}}` |

### 5. Error handling

- Invalid Mustache syntax → node goes to `red` / `"template error"`,
  message is fed to the Catch node as a catchable error.
- Unknown placeholder → empty (Mustache standard), no error.
- JSON/YAML parse errors with corresponding output format → catchable error.

### 6. Status

- Idle: no status (like Change Node).
- On render error: `red` / `"template error"`.

## Technical sketch

### Backend — `internal/nodes/template.go` (new)

```go
type TemplateNode struct {
    config flow.NodeConfig
    send   flow.SendFunc
    status flow.StatusFunc

    template     string                 // raw template
    field        string                 // dot-path
    fieldType    string                 // "msg" | "flow" | "global"
    fieldStorage string                 // "memory" | "persistent"
    format       string                 // "plain" | "json" | "yaml"
    syntax       string                 // "mustache" | "plain"

    contextProvider flow.ContextProvider
    parsed          *mustache.Template   // pre-parsed in Init
}
```

- **Inputs:** 1, **Outputs:** 1
- Implements `ContextProvider` consumption analogous to Function/Change Node
- Pre-parse of the template in `Init` (fail-fast on syntax error at deploy)
- For `syntax == "plain"`: no parsing, template string output directly
- In `HandleMessage`:
  - Build view map: `{payload, topic, ...msg-fields, flow: lookup(...), global: lookup(...)}`
  - Render → string
  - Format post-process (`json.Unmarshal` if `format == "json"`)
  - Write result to `field` in the respective scope
  - `send(0, msg)`

### Mustache lib

`github.com/cbroglie/mustache` — small, dependency-free Go implementation,
supports sections and lambdas-light. Integration via `go.mod`.

### Node registration — `internal/server/server.go`

```go
registry.Register("template", nodes.NewTemplateNode, nodes.TemplateTypeInfo())
```

```go
func TemplateTypeInfo() flow.NodeTypeInfo {
    return flow.NodeTypeInfo{
        Type:        "template",
        Category:    "function",
        Label:       "Template",
        Description: "Build a string from a Mustache template using msg/flow/global values",
        Icon:        "mdi-text-box-outline",
        Defaults: map[string]any{
            "template":      "This is the payload: {{payload}}!",
            "field":         "payload",
            "fieldType":     "msg",
            "fieldStorage":  "memory",
            "format":        "plain",
            "syntax":        "mustache",
        },
        Inputs:  1,
        Outputs: 1,
    }
}
```

### Frontend — `frontend/src/components/nodes/TemplateNode.vue` (new)

- BaseNode with category `function`
- Body shows e.g. the first ~24 characters of the template: `"Sensor {{topic}} ..."`

### Frontend — `frontend/src/components/config/TemplateConfig.vue` (new)

- Multi-line textarea (monospace) for the template
- Property field (dot-path) + scope dropdown (`msg`/`flow`/`global`)
- Storage dropdown for `flow`/`global` (DRY: same component as Change/Function)
- Output format dropdown (`plain`/`json`)
- Syntax dropdown (`mustache`/`plain`)
- All fields via `useNodeProperty`

### Frontend — wiring

- `frontend/src/views/FlowEditor.vue`: `<template #node-template>` + import
- `frontend/src/components/PropertyPanel.vue`: `<TemplateConfig>` for `type === 'template'`
- `frontend/src/components/NodeIcon.vue`: icon entry for `template`

## Affected files

### Backend
- `go.mod` — dependency `github.com/cbroglie/mustache`
- `internal/nodes/template.go` (new)
- `internal/nodes/template_test.go` (new) — pre-parse error, render with
  msg/flow/global, output formats, syntax mode `plain`
- `internal/server/server.go` — registration

### Frontend
- `frontend/src/components/nodes/TemplateNode.vue` (new)
- `frontend/src/components/config/TemplateConfig.vue` (new)
- `frontend/src/views/FlowEditor.vue` — slot + import
- `frontend/src/components/PropertyPanel.vue` — config mapping
- `frontend/src/components/NodeIcon.vue` — icon entry

## Dependencies

- `flow.ContextProvider` (exists)
- `flow.ContextStore` with storage selection (exists, used by Change Node)
- Catch node for error forwarding (exists)

## Out of scope for phase 1

- **YAML output** — added when needed
- **Custom delimiters** (`{{=<% %>=}}`) — Mustache supports this, but rarely
  desired in the UI; if a need arises, can be added as an optional field
- **Partials/includes** across multiple nodes — no use case in sight
- **Live preview in the properties panel** — nice, but not MVP

## Open questions

- Should the output value with `format: json` and a parse error instead leave
  the raw string in place (instead of catchable error)? — Suggestion: no, a clear
  error is more consistent with the other nodes.
