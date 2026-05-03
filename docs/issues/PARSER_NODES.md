# Issue: Parser Nodes — payload conversion between formats

## Status: Proposed

## Problem description

LOOPZE flows exchange data with the outside world: MQTT topics deliver
sensor values as JSON strings, HTTP APIs respond with XML, industrial tools
export CSV, Modbus delivers raw bytes. So that downstream nodes
(Switch, Change, Function, Template …) can work meaningfully, the
payload must be converted into a **structured format** — and packed back
into wire-form string before sending.

Today this ends up in the Function Node with `JSON.parse(...)` /
`JSON.stringify(...)`. Works, but:

- Operator UX suffers: code for an 80% standard task.
- XML/CSV are not available out-of-the-box in Goja.
- Error handling has to be wired up manually each time.
- No consistent behavior between flows / nodes.

Solution: three dedicated **Parser Nodes**, declaratively configurable,
with a unified API and status/catch hookup.

## View / rationale

- **Operator UX before designer power**: A "String → Object" dropdown is
  more accessible than JS code. The most common use cases (parsing MQTT
  JSON payloads) are covered with two clicks.
- **One shared mental model** for all three formats: same fields
  (Property, Action), same error paths, same status behavior.
- **Counterpart to the Template Node**: Template builds strings *from*
  data — Parser nodes pull data *out of* strings. Both sit in the same
  "Function" area of the palette.

## Planned nodes

| Node | Input (wire form) | Output (parsed) | Phase |
|---|---|---|---|
| **JSON** | `string` / `[]byte` (Buffer) | `map` / `[]any` / scalar | **Phase 1 — see `PARSER_JSON_NODE.md`** |
| **CSV** | `string` / `[]byte` | `[]map[string]any` (with header) or `[][]any` | Phase 2 |
| **XML** | `string` / `[]byte` | `map[string]any` (element tree) | Phase 3 |

All parsers can work in both directions (string/buffer ⇄
structured), controlled via an Action dropdown:

| Action | Behavior |
|---|---|
| `auto` | Heuristic: string/buffer → parse; everything else → stringify |
| `parse` | Forces parsing — error on non-string/buffer |
| `stringify` | Forces serialization — error on string/buffer |

`auto` is the default and covers 90% of cases.

## Common requirements

### 1. Property selection

| Field | Description | Default |
|---|---|---|
| `property` | Property on the `msg` object (dot-path) | `payload` |

In phase 1 only **`msg.<property>`** is supported — no flow/global
scope. Rationale: parsers typically run directly after an
input node (MQTT-In, HTTP-In) and write back to the same property.
Scope selection can be added later, once a concrete use case requires it.

### 2. Buffer compatibility

Input may be a **buffer** (`[]byte` from the Go backend, in the future
also the `Buffer` object from `BUFFER_API.md`). The parser converts
internally via `string(b)` or `[]byte(s)` — UTF-8 is assumed.

### 3. Status / error handling

- Idle: no status text (analogous to Change/Template).
- On parse/stringify error: status `red` / `"<format> parse error"`,
  message is routed as a **catchable error** to Catch nodes
  (same pattern as Template node on `format: json` errors).
- If action is `parse` and the property is not a string/buffer →
  catchable error.
- If action is `stringify` and the property is already a string
  → catchable error.

### 4. Inputs / outputs

- **1 input**, **1 output** — parsers don't route, they only convert.

## Out of scope for phase 1

- **Schema validation** (JSON Schema, XSD) — separate nodes, later.
- **Streaming parsers** for very large payloads — no use case so far.
- **Custom encodings** other than UTF-8.
- **flow./global. scopes** — see above.

## Order / roadmap

1. **JSON Parser** (phase 1) — see `PARSER_JSON_NODE.md`. Highest
   priority because every MQTT/HTTP flow needs it.
2. **CSV Parser** (phase 2) — second most common use case (industrial
   exports, reports). Own issue once JSON is in place.
3. **XML Parser** (phase 3) — rarer, but indispensable for
   SOAP/legacy APIs. Own issue once CSV is in place.

Each parser gets its own issue with a detailed specification.
This document remains as the shared concept paper.

## Dependencies

- `flow.Message` with `Get`/`Set` (exists)
- Catch node for error forwarding (exists)
- `BUFFER_API.md` — if available, the parser can accept the `Buffer`
  type directly; without it we work on raw `[]byte` slices
