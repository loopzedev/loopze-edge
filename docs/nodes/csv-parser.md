# CSV Parser (`csv`)

Converts `msg.payload` (or any message property) between a CSV string /
buffer and structured rows, and back. Pair with **File Read** for tail-style
log parsing or use standalone for HTTP / MQTT CSV payloads.

| Inputs | Outputs |
|--------|---------|
| 1      | 1       |

## Why a parser node?

CSV from a SCADA export, lab analyzer, MES batch record, or HTTP body
needs quoting, escaping, embedded newlines, and CRLF handling done right.
The `csv` node ships those out of the box (RFC 4180 via Go's
`encoding/csv`), with bidirectional `parse` ↔ `stringify` and the same
Property / Action interface as the JSON and XML parsers.

For **writing** CSV to a file, prefer the [`csv-out`](csv-out.md) node —
it's file-state aware and survives redeploys cleanly. The stringify side
of `csv` is best for non-file destinations (HTTP body, MQTT payload).

## Configuration

| Field            | Default   | Description |
|------------------|-----------|-------------|
| `property`       | `payload` | Dot-path of the message field to convert. |
| `action`         | `auto`    | `auto` \| `parse` \| `stringify` — see [Action](#action). |
| `header`         | `true`    | Treat the first row as the column header on parse. On stringify, emit a header row before the data rows. |
| `columns`        | *(empty)* | Comma-separated column list. On parse with `header=true`, **overrides** the column names from the file (header row is still dropped). On parse with `header=false`, supplies the keys for the row objects. On stringify, pins column order. |
| `delimiter`      | `,`       | Field separator. UI presets: `,` `;` `\t` `\|` `custom`. |
| `quoteChar`      | `"`       | Quote character. Phase 1 supports only `"` (RFC 4180 default). |
| `comment`        | *(empty)* | Lines starting with this character are skipped on parse. Leave blank to disable. |
| `trimSpaces`     | `false`   | Strip leading + trailing whitespace from each parsed cell. |
| `skipEmptyLines` | `true`    | Drop blank records on parse. |
| `forceQuote`     | `false`   | Quote every field on stringify, even when not required. |
| `newline`        | `\n`      | Line terminator on stringify: `\n` (LF) or `\r\n` (CRLF). |
| `output`         | `rows`    | `rows` (one message per data row) or `array` (single message with the full slice). Parse only. |
| `cast`           | `false`   | Type-coerce cells on parse — see [Cast](#cast). |
| `headerOnce`     | `false`   | Stringify only: suppress the header on every call after the first within a node lifetime. Use for HTTP / MQTT bodies where the consumer keeps state. **Not** recommended for file destinations — use `csv-out` instead. |

## Action

| Value       | Behaviour |
|-------------|-----------|
| `auto`      | **Default.** `string` / `[]byte` input → parse. Anything else → stringify. |
| `parse`     | Force parse. Error if the input is not a string or buffer. |
| `stringify` | Force stringify. Error if the input is already a string or buffer. |

## Output shapes

### Parse → object rows (default)

With `header=true` **or** explicit `columns`, each row becomes a
`map[string]any` keyed by the column names:

```csv
sensor,value,unit
T-101,25.4,C
T-102,31.2,C
```

→ `{ sensor: "T-101", value: "25.4", unit: "C" }` (one per row)

### Parse → positional arrays

With `header=false` and no `columns`, each row becomes a positional
`[]any`:

```csv
1,2,3
4,5,6
```

→ `["1", "2", "3"]`, `["4", "5", "6"]`

### `output=rows` (fan-out, default)

One outgoing message per data row. Each message carries
`msg.isFirst` / `msg.isLast` / `msg.index` / `msg.total` for
end-of-batch detection, plus `msg.columns` listing the resolved column
names. An empty CSV emits nothing. Pattern matches the Split node.

### `output=array` (single message)

The entire CSV in one message — `msg.payload` is `[]map[string]any` (or
`[][]any` for positional). An empty CSV emits a message with
`msg.payload = []`.

## Cast

When `cast=true`, parsed cells are coerced conservatively:

| Cell content | Becomes |
|--------------|---------|
| `""` (empty) | `nil` |
| `true` / `false` (case-insensitive) | `bool` |
| Integer-looking (`42`, `-1`) | `int64` |
| Decimal (`3.14`) | `float64` |
| Anything else | `string` (unchanged) |

No date parsing, no thousands-separators, no scientific notation. Apply a
Change or Function node downstream for richer coercion.

## Stringify input shapes

`stringify` accepts (in order of preference):

| Input shape                | Notes |
|----------------------------|-------|
| `map[string]any`           | One row. Useful for "one record per message" patterns. |
| `[]map[string]any`         | Multiple rows. Column order from `columns` config; otherwise alphabetical (Go maps are unordered). |
| `[][]any` / `[][]string`   | Positional rows. Header is only written when `columns` is configured. |
| `[]any` of the above       | Post-JSON shape (e.g. after `json` parse). |

## Streaming (tail mode)

When the upstream node is `file-read` with `incremental=true`, the parser
automatically engages a partial-row-safe streaming path:

- Per-file state is persisted in flow context as
  `{Offset, Residue, Columns}`.
- A chunk that ends mid-row (because the writer was still in progress) is
  buffered as `Residue` and concatenated with the next chunk before
  parsing — no truncated record ever reaches downstream.
- The header from the first chunk is persisted in `state.Columns`;
  subsequent chunks do not re-consume a header row.
- File-read's `msg.reset=true` (truncation / rotation) clears the state
  for the affected file.
- Output messages gain `msg.csvPosition` (clean byte offset after the
  last emitted row) and `msg.filename`.

The streaming path activates **only** when both `msg.filename` AND
`msg.position` are set. Full-file reads, file-watch events, HTTP uploads
with a filename header, and manual injection all take the one-shot path
so headers are consumed normally and no stale state is loaded.

## Error handling

| Condition | Status | Routed as |
|-----------|--------|-----------|
| Malformed CSV (unbalanced quote, etc.) | `red` / `csv parse error` | Catchable error |
| Wrong input type for action | `red` / `csv type error` | Catchable error |
| Stringify with unsupported payload shape | `red` / `csv stringify: unsupported input type` | Catchable error |
| Stringify object rows with `header=false` and no `columns` | `red` / `csv stringify: columns required` | Catchable error |

The status pill clears automatically on the next successful message.

## Examples

### Parse an HTTP CSV response, one message per row

```
[HTTP-In]  →  [CSV: action=auto, header=true, output=rows]  →  [Switch: msg.payload.value > 30]
```

Input:
```csv
sensor,value
T-101,25.4
T-102,31.2
```

Emits two messages with `msg.payload = {sensor: "T-101", value: "25.4"}`
and `msg.payload = {sensor: "T-102", value: "31.2"}`. `msg.isFirst` /
`msg.isLast` / `msg.index` / `msg.total` mark the boundary; `msg.columns
= ["sensor", "value"]`.

### Parse a header row but rename the keys

```
[CSV: action=parse, header=true, columns="ts,val"]
```

Input:
```csv
time,value
2026-05-19,0.20
```

The first row (`time,value`) is dropped because `header=true`. Output
rows are keyed by the configured `columns`: `{ts: "2026-05-19", val:
"0.20"}`.

### Tail a growing log file

```
[File Read: path=/var/log/sensors.csv, incremental=true, delimiter=\n]
  →  [CSV: action=parse, header=true, cast=true]
  →  [Switch: msg.payload.value > 30]
```

Each tick emits only newly-appended rows. The streaming path buffers any
partial trailing row that arrived mid-write; the header is consumed once
from the first chunk and never re-emitted.

### Parse a buffer from TCP-In

```
[TCP-In]  →  [CSV: action=parse, header=false, columns="ts,sensor,value"]  →  [Debug]
```

Input bytes: `1715299200,T-101,25.4\n` → `{ts: "1715299200", sensor:
"T-101", value: "25.4"}`.

### Stringify for an HTTP response body

```
[Function: msg.payload = [{name:"A", val:1}, {name:"B", val:2}]]
  →  [CSV: action=stringify, header=true, columns="name,val"]
  →  [HTTP Response]
```

Output:
```csv
name,val
A,1
B,2
```

### Stringify a single object per message

```
[MQTT-In]  →  [JSON: parse]  →  [CSV: action=stringify, columns="ts,sensor,value", headerOnce=true]
```

First message includes the `ts,sensor,value` header; every subsequent
message in the same node lifetime emits just the data row. Useful for
appending to a streaming HTTP response or a websocket frame.

## Notes

- **Encoding**: UTF-8 only. A leading BOM (`EF BB BF`) is stripped on
  parse; BOM is not emitted on stringify.
- **Quote character**: Phase 1 supports only `"`. Other quote characters
  are accepted in config but silently normalised to `"` at `Init` with a
  warning.
- **Empty CSV in `output=rows` mode**: emits zero messages, matching the
  Split node convention. Use `output=array` if you need a sentinel
  message for empty input.
- **Header + columns interaction**: `header=true` always drops the first
  row of the file. Setting `columns` alongside replaces the parsed header
  names with the configured ones (override mode).
- **Stringify map key order**: Go map iteration is unordered. For
  `[]map[string]any` input, set `columns` explicitly to pin order —
  otherwise alphabetical order is used.

## See also

- **[CSV Out](csv-out.md)** (`csv-out`) — serialise to CSV and write to a
  file in one node, file-state aware. Prefer this over `csv stringify +
  file-out` for log-file generation.
- **JSON Parser** (`json`) — same Property / Action interface for JSON.
- **[XML Parser](xml-parser.md)** (`xml`) — same interface for XML.
- **[File Read](file-read.md)** — chain in front of `csv` for tail-style
  log parsing.
