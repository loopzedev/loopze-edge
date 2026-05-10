# XML Parser (`xml`)

Converts `msg.payload` (or any message property) between an XML string /
buffer and a structured Go map, and back. Pair with
the **JSON Parser** (`json`) for format conversion, or use standalone to
bridge HTTP APIs, SOAP services, and industrial systems that speak XML.

| Inputs | Outputs |
|--------|---------|
| 1      | 1       |

## Why a parser node?

HTTP responses from legacy APIs, SOAP/WS-* endpoints, and some industrial
gateways arrive as XML strings. Today that requires a Function node with
custom parsing code. The XML Parser does the same job declaratively — two
clicks, no code.

The same node runs in both directions: **parse** (string → map) and
**stringify** (map → string). One node type covers the full round-trip.

## Configuration

| Field         | Default   | Description |
|---------------|-----------|-------------|
| `property`    | `payload` | Dot-path of the message field to convert. |
| `action`      | `auto`    | `auto` \| `parse` \| `stringify` — see [Action](#action). |
| `root`        | `root`    | Root element name used when stringifying. Required when action is `stringify` or `auto` with a non-string input. |
| `indent`      | `0`       | Spaces for pretty-print indentation (`0` = compact). Applies to stringify and the stringify branch of auto. |
| `declaration` | `true`    | Prepend `<?xml version="1.0" encoding="UTF-8"?>` to stringify output. |

## Action

| Value       | Behaviour |
|-------------|-----------|
| `auto`      | **Default.** `string` / `[]byte` input → parse. Anything else → stringify. |
| `parse`     | Force parse. Error if input is not a string or buffer. |
| `stringify` | Force stringify. Error if input is already a string or buffer. |

`auto` covers 90 % of flows — the direction is clear from context
(HTTP-In / TCP-In → parse; before HTTP-Out → stringify). Switch to
`parse` or `stringify` when you want a hard error on the wrong input shape.

## Map representation

The parsed map follows the **mxj convention**:

| XML construct                   | Map key / value                  |
|---------------------------------|----------------------------------|
| Element `<name>…</name>`        | Key `"name"`, value is the content |
| Attribute `id="42"`             | Key `"-id"`, value `"42"` |
| Text content of mixed element   | Key `"#text"` |
| Repeated siblings `<x/><x/>`   | Slice `[]any` under key `"x"` |

All parsed values are **strings** — XML carries no type information. Use a
Change or Function node to cast to numbers or booleans.

## Error handling

| Condition | Status | Routed as |
|-----------|--------|-----------|
| Malformed XML | `red` / `xml parse error` | Catchable error |
| Wrong input type for action | `red` / `xml type error` | Catchable error |
| `root` is blank on stringify | `red` / `xml root required` | Catchable error |

The status pill is cleared automatically on the next successful message —
no manual reset required.

## Examples

### Parse an HTTP XML response

```
[HTTP-In]  →  [XML: action=auto]  →  [Switch: msg.payload.sensor.value > 30]
```

Input: `msg.payload = "<sensor><value>25.4</value></sensor>"`
Output: `msg.payload = { sensor: { value: "25.4" } }`

### Parse element attributes

```xml
<sensor id="42" unit="C"><value>25.4</value></sensor>
```

Parses to:

```json
{
  "sensor": {
    "-id": "42",
    "-unit": "C",
    "value": "25.4"
  }
}
```

### Stringify a map for a SOAP request

```
[Change: msg.payload = {name: "Alice"}]
  →  [XML: action=stringify, root=Person, indent=2]
  →  [HTTP-Out]
```

Output:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<Person>
  <name>Alice</name>
</Person>
```

### Parse a buffer from TCP-In

```
[TCP-In]  →  [XML: action=parse]  →  [Debug]
```

Input: `msg.payload = []byte('<status ok="true"/>')`
Output: `msg.payload = { status: { "-ok": "true" } }`

### Handle repeated elements

```xml
<list><item>a</item><item>b</item></list>
```

Parses to:

```json
{ "list": { "item": ["a", "b"] } }
```

A single `<item>` would produce `"item": "a"` (not a slice). Keep this in
mind when consuming the result downstream — if the number of siblings can
vary, add a normalisation step.

## Notes

- **Encoding**: UTF-8 is assumed for all input. Buffer input is decoded as
  UTF-8. Declarations with other encodings are accepted by the parser but
  the bytes are still treated as UTF-8.
- **Namespaces**: namespace prefixes are included as-is in the key
  (e.g. `ns:attr` → key `"ns:attr"`). Full namespace resolution is not
  supported.
- **CDATA**: treated as regular text content.
- **Special characters**: `&`, `<`, `>`, `"`, `'` in string values are
  automatically escaped to their XML entities on stringify
  (`&` → `&amp;`, etc.) and unescaped on parse.

## See also

- **JSON Parser** (`json`) — same interface for JSON payloads.
