# Issue: CSV Parser Node — Convert payload between CSV string and structure

## Status: Proposed

## Context

Second parser from the concept in `PARSER_NODES.md` (Phase 2). CSV is the
lingua franca of industrial reporting: SCADA exports, lab analyzers, MES
batch records, ERP integrations, plain timeseries dumps. Today this
requires a Function node with custom split/quote handling that is brittle
on quoted fields, embedded delimiters, or `\r\n` line endings. The CSV
Parser closes this gap with the same two-click UX as the JSON and XML
parsers.

## Problem description

`msg.payload` from File-In, HTTP-In, or TCP-In often arrives as a **CSV
string** or **`[]byte` (buffer)**. For Switch/Change/Template to access
individual cells, the payload must be parsed to a structured Go value.
When sending in the other direction — e.g. before File-Out or HTTP-Out —
the structure must be serialized back to CSV.

The **CSV Parser node** does exactly that — bidirectional, header-aware,
with two output modes and full quoting/escaping support.

## View / rationale

- **Operator UX**: same two-click experience as JSON/XML — drop in, set
  property, done.
- **Same mental model**: Property, Action, and error behavior are
  identical to the existing parsers. Operators familiar with JSON/XML
  need zero re-learning for the common case.
- **Industrial use case**: CSV is overwhelmingly the file format for
  exports between LIMS, SCADA, MES, and ERP. Without a dedicated node,
  every integration needs Function-node string surgery.
- **Header awareness**: when a header row is present, parsed rows are
  emitted as objects keyed by column name — exactly what downstream
  Switch/Change nodes consume. Without a header, rows fall back to
  positional arrays.

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
| `auto` | **Default.** If value is `string` or `[]byte` → parse to rows. Otherwise → serialize to CSV string. |
| `parse` | Force parsing. Error if value is not a string/buffer. |
| `stringify` | Force serializing. Error if value is already a string. |

### 3. Header

| Field | Description | Default |
|---|---|---|
| `header` | First row defines column names (parse) / emit a header row from object keys (stringify) | `true` |

**Parse, `header=true`**: the first non-empty, non-comment row supplies
the keys. Each subsequent row is a `map[string]any` keyed by those
column names.

**Parse, `header=false`**: each row is a positional `[]any` (or, in
combination with `columns`, a `map[string]any` keyed by the configured
column names — see below).

**Stringify, `header=true`**: a header row is written before the data
rows. Column order comes from `columns` if set, otherwise from the keys
of the first object in the input slice (insertion order preserved).

**Stringify, `header=false`**: no header row is written. Input must
already be a `[][]any` or a slice of objects whose keys all match the
configured `columns` (otherwise → error, since column order is
otherwise undefined).

### 4. Columns (optional, both directions)

| Field | Description | Default |
|---|---|---|
| `columns` | Explicit column list. Comma-separated in the UI. | `[]` |

- **Parse, `header=true`** → `columns` is **ignored** (header wins).
- **Parse, `header=false`** → if `columns` is set, rows become
  `map[string]any` keyed by these names; otherwise rows are `[]any`.
- **Stringify** → `columns` defines the output column order and selects
  which keys to include. Keys present on the input but not in `columns`
  are dropped silently. Columns not present on a given input object are
  written as empty cells.

This field is the explicit answer to "which keys, in which order?" —
without it, stringify of object inputs relies on the first row's key
order, which is fine for round-trips but unsafe for heterogeneous input.

### 5. Delimiter and quoting

| Field | Description | Default |
|---|---|---|
| `delimiter` | Single character separating fields. UI presets: `,`  `;`  `\t`  `\|`  `custom`. | `,` |
| `quoteChar` | Quote character for fields containing delimiter, quote, or newline. | `"` |
| `comment` | Lines starting with this character are skipped on parse. Empty = no comment handling. | `""` |
| `trimSpaces` | Strip leading/trailing whitespace from each parsed field. | `false` |
| `skipEmptyLines` | Skip lines that contain no fields on parse. | `true` |
| `forceQuote` | On stringify, quote every field — not only those that need it. | `false` |
| `newline` | On stringify: line terminator. UI: `\n` / `\r\n`. | `\n` |

Tab as a delimiter is selected via the UI preset (the underlying value
is a literal `\t`). Multi-character delimiters are not supported in
Phase 1 (limitation of `encoding/csv`).

### 6. Output mode (parse only)

| Field | Description | Default |
|---|---|---|
| `output` | `rows` — one message per data row; `array` — single message with an array payload. | `rows` |

**`output=rows`** — one message per data row (excluding header). Each
output message has:

```json
{
  "payload": { "<row object or array>" },
  "isFirst": true,
  "isLast": false,
  "index": 0,
  "total": 12,
  "columns": ["a", "b", "c"]
}
```

- `columns` is the resolved column list (from header or `columns`
  config); absent when `header=false` and `columns` is unset.
- `isFirst`/`isLast`/`index`/`total` enable downstream end-of-batch
  detection (same pattern as `folder-in` and `split`).
- An empty CSV (no data rows) produces **no output messages**, same as
  `incremental=true` on `file-in` when there are no new bytes.
- Existing `msg` properties are preserved on all emitted messages
  (pass-through semantics).

**`output=array`** — single message with the full result as payload:

```json
{
  "payload": [ { "<row 0>" }, { "<row 1>" }, … ],
  "columns": ["a", "b", "c"]
}
```

- `payload` is `[]map[string]any` when keys are available (header or
  `columns`), otherwise `[][]any`.
- An empty CSV produces a message with `payload = []` (empty slice) —
  not nothing — because the upstream message must still propagate.
- This mode is the right fit for downstream nodes that operate on the
  full set (Function, Template, HTTP-Out body).

### 7. Type coercion

| Field | Description | Default |
|---|---|---|
| `cast` | Try to coerce parsed cell values to `bool` / `number` / `null`. | `false` |

When `cast=false` (default), all parsed values are strings — CSV carries
no type information. When `cast=true`:

| Cell content | Becomes |
|---|---|
| `""` (empty) | `nil` |
| `"true"` / `"false"` (case-insensitive) | `bool` |
| Parses as `int64` (no decimal point, no exponent) | `int64` |
| Parses as `float64` | `float64` |
| Anything else | `string` |

Coercion is intentionally conservative: no date/time parsing, no
numeric strings with thousands separators, no scientific notation
edge-casing. Operators who need richer typing apply a Change node after
the parser, where rules are explicit and per-column.

### 8. Encoding

UTF-8 only in Phase 1. A leading **BOM** (`\xEF\xBB\xBF`) on parse is
stripped. On stringify, **no BOM is emitted**. Other encodings (UTF-16,
Windows-1252) are out of scope — see below.

### 9. Status / error handling

- Idle: no status.
- Parse error (malformed quoting, inconsistent column count when strict)
  → status `red` / `"csv parse error"`, catchable error.
- Wrong type for the configured action → status `red` /
  `"csv type error"`, catchable error.
- Stringify with input that is neither `[][]any` nor `[]map[string]any`
  nor `[]any` of map elements → status `red` /
  `"csv stringify: unsupported input type"`, catchable error.
- Stringify with `header=false` and no `columns` configured, and input
  is object-shaped → status `red` / `"csv stringify: columns required"`,
  catchable error.
- Success: status stays unchanged (no green flash — avoids flicker at
  high frequency, same behavior as JSON/XML/Change/Template).

### 10. Inputs / outputs

- **1 input**, **1 output**.
- **Parse, `output=rows`** emits N messages from a single input — same
  fan-out semantics as the Split node. `isFirst`/`isLast` mark the
  boundary.
- **Parse, `output=array`** and **stringify** emit exactly one output
  message per input.
- Property is overwritten **at the same path** — `msg.payload` in,
  `msg.payload` out (the other form).

### 11. Partial-row safety and per-file offset tracking

When the CSV Parser is wired after a `file-in` node in incremental mode,
the incoming `msg.payload` chunk is not guaranteed to end on a row
boundary. A write to the file can be in-flight during the read: the last
bytes of the chunk may be a **partial (incomplete) row** — the line is
still being written and has no trailing `\n` yet.

Parsing an incomplete row with `encoding/csv` would either fail (parse
error) or produce a truncated final record with incorrect field values.
Both outcomes are wrong for a tail-style pipeline.

#### Activation

Partial-row handling is activated automatically whenever `msg.filename`
is set on the incoming message. `file-in` sets this field on every
incremental read. Messages without `msg.filename` (HTTP-In, MQTT-In,
manual test injection) bypass this mechanism and are parsed as-is.

#### Per-file persistent state

The node implements `flow.ContextProvider` and stores, per filename, the
following state in `flowPers` (flow-scoped persistent store):

```go
type csvFileState struct {
    Offset  int64  `json:"offset"`  // byte offset of the last clean row boundary
    Residue string `json:"residue"` // bytes received after the last '\n', not yet complete
}
```

**Storage key**: `_csvstate.<sha256(nodeID+":"+absFilename)[:16]>` — the
same SHA-prefix scheme used by `file-in` for its cursor, keeping keys
short and NATS-KV safe.

#### Processing sequence (per message with `msg.filename`)

1. **Load state** for `msg.filename` from `flowPers`. If absent →
   `{Offset: 0, Residue: ""}` (first time this file is seen).
2. **Combine** `state.Residue + string(msg.payload)` into a single
   byte slice. If `msg.reset == true` (file was truncated/rotated as
   signalled by `file-in`), discard the stored residue and use
   `msg.payload` only, then clear state.
3. **Find the last row boundary**: scan the combined bytes from the end
   for the last `\n`. If `\r\n` endings are present (detected by the
   presence of `\r` immediately before `\n`), the boundary is at the
   `\r`.
4. **If no `\n` found** in the combined bytes: the entire chunk is a
   partial row. Append to residue, update `state.Residue`, persist.
   Emit **no message** (same behaviour as `file-in` incremental with no
   new bytes). Return.
5. **Split** at the last boundary:
   - `complete = combined[:lastNewlineIdx+1]` — parse this.
   - `newResidue = combined[lastNewlineIdx+1:]` — store this.
6. **Parse** `complete` with the configured options (header, delimiter,
   cast, etc.). This slice always ends with `\n` and can be parsed
   safely.
7. **Calculate adjusted position**:
   ```
   adjustedOffset = msg.position - int64(len(newResidue))
   ```
   This is the byte offset in the source file up to and including the
   last complete row that was just parsed.
8. **Persist** `{Offset: adjustedOffset, Residue: newResidue}` for this
   filename in `flowPers`.
9. **Emit** rows or array as usual. Each output message carries:
   - `msg.csvPosition = adjustedOffset` — the last clean byte position.
   - `msg.filename` — passed through unchanged.

#### Header in streaming mode

When `header=true` and the file is read incrementally:
- The header row may arrive in the **first chunk** only.
- The node persists the parsed column list alongside the file state once
  the header has been seen:

  ```go
  type csvFileState struct {
      Offset  int64    `json:"offset"`
      Residue string   `json:"residue"`
      Columns []string `json:"columns,omitempty"` // populated after header is parsed
  }
  ```
- On subsequent chunks the header row is **not** re-emitted. The node
  re-uses `state.Columns` from the persisted state.
- If `state.Columns` is already set, the first row of the new chunk is
  treated as a **data row**, not a header row.
- Reset (`msg.reset=true`) also clears `state.Columns` so the header is
  re-read from the next chunk.

#### `msg.csvPosition` on output

All output messages (row-by-row and array mode) carry this field
whenever `msg.filename` is set:

```json
{
  "csvPosition": 4096,
  "filename": "/var/log/sensors.csv"
}
```

Downstream nodes can log or monitor this to observe parse progress. A
Function node can use it to feed back a `msg.resetCursor` to an upstream
`file-in` node if manual re-alignment is needed.

## Examples

### Example 1 — Parse HTTP CSV body, one message per row

```
[HTTP-In]  →  [CSV: action=auto, header=true, output=rows]  →  [Switch: msg.payload.value > 30]
```

Input:
```csv
sensor,value,unit
T-101,25.4,C
T-102,31.2,C
```

Emits two messages:
- `msg.payload = {"sensor": "T-101", "value": "25.4", "unit": "C"}`,
  `index: 0`, `total: 2`, `isFirst: true`, `isLast: false`,
  `columns: ["sensor", "value", "unit"]`
- `msg.payload = {"sensor": "T-102", "value": "31.2", "unit": "C"}`,
  `index: 1`, `total: 2`, `isFirst: false`, `isLast: true`

### Example 2 — Parse a SCADA export, one batch as array

```
[file-in: report.csv]  →  [CSV: action=parse, header=true, output=array]  →  [Function]
```

`msg.payload` becomes `[]map[string]any` with all rows in one message.

### Example 3 — No header, fall back to positional arrays

```
[TCP-In]  →  [CSV: action=parse, header=false, output=rows]  →  [Debug]
```

Input: `1,2,3\n4,5,6\n`
Emits two messages with `msg.payload = ["1", "2", "3"]` and
`msg.payload = ["4", "5", "6"]`.

### Example 4 — No header but explicit columns

```
[CSV: action=parse, header=false, columns="ts,sensor,value", output=rows]
```

Same input as above produces `msg.payload = {"ts": "1", "sensor": "2",
"value": "3"}`.

### Example 5 — Stringify array of objects

```
[Change: msg.payload = [{name:"A", val:1}, {name:"B", val:2}]]
  →  [CSV: action=stringify, header=true]
  →  [file-out: report.csv]
```

Output:
```csv
name,val
A,1
B,2
```

### Example 6 — Tab-separated, comment-prefixed, with type coercion

```
[CSV: delimiter="\t", comment="#", cast=true, header=true, output=array]
```

Input:
```
# generated 2026-05-10
ts	sensor	ok
1715299200	T-101	true
1715299260	T-102	false
```

Output: `[{"ts": 1715299200, "sensor": "T-101", "ok": true}, …]`

### Example 8 — Incremental tail of a growing sensor log

```
[file-in: mode=read+watch, incremental=true, /var/log/sensors.csv]
  →  [CSV: action=parse, header=true, output=rows, cast=true]
  →  [Switch: msg.payload.value > 30]
```

`file-in` emits a chunk of new bytes on every `write` event. The chunk
may end mid-row. The CSV Parser:
1. Prepends any stored residue for `/var/log/sensors.csv`.
2. Strips the trailing partial row.
3. Parses only the complete rows.
4. Stores the partial bytes + adjusted byte offset.
5. Emits one message per complete row.

If a chunk contains no `\n` at all (e.g., an in-flight partial write of
the first column), no messages are emitted and the bytes are buffered as
residue for the next chunk.

`msg.csvPosition` on each emitted row tells downstream nodes the source
byte offset, useful for auditing or monitoring parse lag.

### Example 7 — Round-trip with quoting

```
[CSV: action=parse]  →  [Change]  →  [CSV: action=stringify, header=true]
```

Input cell `"He said ""hi""\nfriend"` round-trips intact: parsed to
`He said "hi"\nfriend`, then re-quoted on output.

## Technical sketch

### Backend — `internal/nodes/core/parser_csv.go` (new)

```go
type CSVParserNode struct {
    config flow.NodeConfig
    nodes.BaseNode

    property       string
    action         string   // "auto" | "parse" | "stringify"
    header         bool
    columns        []string
    delimiter      rune
    quoteChar      rune
    comment        rune     // 0 = no comment handling
    trimSpaces     bool
    skipEmptyLines bool
    forceQuote     bool
    newline        string   // "\n" | "\r\n"
    output         string   // "rows" | "array"
    cast           bool

    flowPers     flow.ContextStore
    inErrorState bool
}

// SetContext implements flow.ContextProvider.
func (n *CSVParserNode) SetContext(_, _, _, flowPers flow.ContextStore) {
    n.flowPers = flowPers
}
```

- **Inputs:** 1, **Outputs:** 1
- Implements `flow.NodeInstance` and `flow.ContextProvider`.
- Uses Go's standard `encoding/csv` — no external dependency.

#### Per-file state type

```go
type csvFileState struct {
    Offset  int64    `json:"offset"`
    Residue string   `json:"residue"`
    Columns []string `json:"columns,omitempty"`
}

func csvStateKey(nodeID, absFilename string) string {
    h := sha256.Sum256([]byte(nodeID + ":" + absFilename))
    return "_csvstate." + hex.EncodeToString(h[:8])
}
```

#### Action logic (pseudocode)

```go
func (n *CSVParserNode) handle(msg flow.Message) error {
    // If msg.filename is set, route through partial-row-safe streaming path.
    if filename, ok := msg.GetString("filename"); ok && filename != "" {
        return n.handleStreaming(msg, filename)
    }

    value, _ := msg.Get(n.property)

    var (
        result       any
        emitMulti    []map[string]any
        emitMultiArr [][]any
        cols         []string
        err          error
    )
    switch n.action {
    case "parse":
        result, emitMulti, emitMultiArr, cols, err = n.parse(value, nil)
    case "stringify":
        result, err = n.stringify(value)
    case "auto":
        if isStringish(value) {
            result, emitMulti, emitMultiArr, cols, err = n.parse(value, nil)
        } else {
            result, err = n.stringify(value)
        }
    }
    if err != nil { return n.fail(err) }

    if n.action != "stringify" && n.output == "rows" {
        return n.emitRows(msg, emitMulti, emitMultiArr, cols, -1, "")
    }
    msg.Set(n.property, result)
    return n.send(0, msg)
}
```

#### Streaming handler (when `msg.filename` is set)

```go
func (n *CSVParserNode) handleStreaming(msg flow.Message, filename string) error {
    // Load persisted state for this filename.
    var state csvFileState
    _ = n.flowPers.Get(csvStateKey(n.ID(), filename), &state)

    // Build combined input: residue from last call + new bytes.
    incoming := toBytes(msg.Get(n.property))
    if reset, _ := msg.GetBool("reset"); reset {
        // File was truncated or rotated — discard stale residue and columns.
        state = csvFileState{}
    }
    combined := append([]byte(state.Residue), incoming...)

    // Find last complete row boundary (last '\n').
    lastNL := bytes.LastIndexByte(combined, '\n')
    if lastNL < 0 {
        // No complete row yet — buffer everything as residue.
        state.Residue = string(combined)
        _ = n.flowPers.Set(csvStateKey(n.ID(), filename), &state)
        return nil  // no output
    }

    complete   := combined[:lastNL+1]
    newResidue := combined[lastNL+1:]

    // Compute adjusted position in source file.
    msgPos, _ := msg.GetInt64("position")
    adjustedOffset := msgPos - int64(len(newResidue))

    // Parse only the complete rows; pass pre-resolved columns if known.
    var knownCols []string
    if len(state.Columns) > 0 && n.header {
        knownCols = state.Columns  // header already consumed in a prior chunk
    }
    _, rows, arrRows, cols, err := n.parse(complete, knownCols)
    if err != nil { return n.fail(err) }

    // Persist updated state.
    newState := csvFileState{
        Offset:  adjustedOffset,
        Residue: string(newResidue),
    }
    if n.header {
        if len(cols) > 0 {
            newState.Columns = cols
        } else if len(state.Columns) > 0 {
            newState.Columns = state.Columns
        }
    }
    _ = n.flowPers.Set(csvStateKey(n.ID(), filename), &newState)

    // Emit.
    if n.output == "rows" {
        return n.emitRows(msg, rows, arrRows, cols, adjustedOffset, filename)
    }
    // array mode
    var result any
    if rows != nil {
        result = rows
    } else {
        result = arrRows
    }
    out := msg.Clone()
    out.Set(n.property, result)
    out.Set("csvPosition", adjustedOffset)
    if len(cols) > 0 { out.Set("columns", cols) }
    return n.send(0, out)
}
```

#### Parse

```go
// knownCols: pre-resolved column names from a prior streaming chunk (nil = derive from data).
func (n *CSVParserNode) parse(v any, knownCols []string) (full any, rows []map[string]any, arrRows [][]any, cols []string, err error)
```

- Convert `string` / `[]byte` → `*csv.Reader`. Strip leading BOM.
- `r.Comma = n.delimiter`; `r.LazyQuotes = false` (strict by default —
  malformed quoting is an error, not silent corruption).
- `r.Comment = n.comment` if non-zero.
- `r.TrimLeadingSpace = n.trimSpaces` (note: `encoding/csv` only
  trims **leading** space; trailing whitespace is trimmed by the node
  in a post-pass when `trimSpaces=true`, to fully honor the option).
- `r.FieldsPerRecord = -1` (let row-length variance produce zero-fill /
  best-effort behavior; strict equality is enforced separately if
  `header=true`).
- Loop `r.Read()`:
  - Header logic if `header=true`: first record sets `cols`.
  - Each subsequent record → either `map[string]any` (cols present) or
    `[]any` (cols absent) after `cast` coercion if enabled.
  - `skipEmptyLines=true`: drop records with one empty field that comes
    from a blank line.
- Return shape depends on `output`:
  - `array` → `full` is `[]map[string]any` or `[][]any`.
  - `rows` → returns the slice for downstream fan-out via `emitRows`.

#### Stringify

```go
func (n *CSVParserNode) stringify(v any) (string, error)
```

Accept these input shapes (in order):

1. `[]map[string]any` / `[]map[string]string` — natural object form.
2. `[]any` whose elements are all `map[string]any` — the form parsed
   payloads commonly take after a Change/Function node touched them.
3. `[][]any` / `[][]string` — positional rows.

For shapes 1 and 2:
- If `n.columns` is set → use it as the column order.
- Else if `n.header=true` → derive column order from the first object's
  key insertion order.
- Else → error `"csv stringify: columns required"`.

For shape 3:
- `n.columns` (if set) defines the header row when `n.header=true`.
- Otherwise no header row is emitted.

Cells are formatted with `fmt.Sprint(v)` for non-string values; nil →
empty cell. `forceQuote=true` wraps every field in `quoteChar`. Line
terminator is `n.newline`.

`encoding/csv`'s `csv.Writer` uses `\n` by default. `\r\n` is achieved by
either setting `csv.Writer.UseCRLF = true` or post-processing — Phase 1
uses `UseCRLF` directly.

#### Emit rows (fan-out)

```go
// csvPos: adjusted byte offset from streaming handler; -1 = not streaming.
// filename: source filename from streaming handler; "" = not streaming.
func (n *CSVParserNode) emitRows(msg flow.Message, rows []map[string]any, arrRows [][]any, cols []string, csvPos int64, filename string) error {
    total := len(rows)
    if total == 0 { total = len(arrRows) }
    if total == 0 {
        // no rows → no output (same as file-in incremental with no new bytes)
        return nil
    }
    for i := 0; i < total; i++ {
        out := msg.Clone()  // standard fan-out clone helper
        if rows != nil {
            out.Set(n.property, rows[i])
        } else {
            out.Set(n.property, arrRows[i])
        }
        out.Set("isFirst", i == 0)
        out.Set("isLast",  i == total-1)
        out.Set("index",   i)
        out.Set("total",   total)
        if cols != nil    { out.Set("columns", cols) }
        if csvPos >= 0    { out.Set("csvPosition", csvPos) }
        if filename != "" { out.Set("filename", filename) }
        if err := n.send(0, out); err != nil { return err }
    }
    return nil
}
```

`msg.Clone()` is the existing helper used by the Split node — reuse it.

### Node registration — `internal/nodes/core/init.go`

```go
{Type: "csv", Factory: NewCSVParserNode, Info: CSVParserTypeInfo()},
```

```go
func CSVParserTypeInfo() flow.NodeTypeInfo {
    return flow.NodeTypeInfo{
        Type:        "csv",
        Category:    "parser",
        Label:       "CSV",
        Description: "Convert CSV strings to objects/arrays and back",
        Icon:        "csv",
        Defaults: map[string]any{
            "property":       "payload",
            "action":         "auto",
            "header":         true,
            "columns":        []string{},
            "delimiter":      ",",
            "quoteChar":      "\"",
            "comment":        "",
            "trimSpaces":     false,
            "skipEmptyLines": true,
            "forceQuote":     false,
            "newline":        "\n",
            "output":         "rows",
            "cast":           false,
        },
        Inputs:  1,
        Outputs: 1,
    }
}
```

### Frontend — `frontend/src/nodes/core/CSVParserConfig.vue` (new)

Field layout (top to bottom):

- Property (dot-path).
- Action (`auto` / `parse` / `stringify`).
- Header (toggle).
- Columns (comma-separated string input).
- Delimiter (preset dropdown: `,` `;` `\t` `|` `custom` + char input
  when `custom` is selected).
- Quote char (single-char input, default `"`).
- Comment char (single-char input, optional, default empty).
- Trim spaces (toggle).
- Skip empty lines (toggle).
- Force quote (toggle, only visible for stringify/auto).
- Newline (`\n` / `\r\n` dropdown, only visible for stringify/auto).
- Output mode (`rows` / `array`, only visible for parse/auto).
- Cast values (toggle, only visible for parse/auto).

All fields via `useNodeProperty`. Body of the canvas node displays e.g.
`payload · auto` or `payload → csv (rows)`.

### Frontend — Wiring

- `frontend/src/nodes/core/index.ts`: register `csv` in the manifest
  (matches the JSON entry).
- `frontend/src/components/NodeIcon.vue`: icon entry for `csv`.

## Affected files

### Backend
- `internal/nodes/core/parser_csv.go` (new)
- `internal/nodes/core/parser_csv_test.go` (new) — see test list below.
- `internal/nodes/core/init.go` — add registration.

### Frontend
- `frontend/src/nodes/core/CSVParserConfig.vue` (new)
- `frontend/src/nodes/core/index.ts` — register `csv`.
- `frontend/src/components/NodeIcon.vue` — icon entry.

## Tests

| Test | Verifies |
|---|---|
| `TestCSV_Parse_HeaderRows` | Standard CSV with header → `[]map`, columns from header |
| `TestCSV_Parse_HeaderArray` | Same input, `output=array` → single message with slice payload |
| `TestCSV_Parse_NoHeader_PositionalRows` | `header=false`, no `columns` → `[]any` per row |
| `TestCSV_Parse_NoHeader_WithColumns` | `header=false` + `columns` → `map[string]any` keyed by config |
| `TestCSV_Parse_QuotedField` | Field containing delimiter inside quotes preserved |
| `TestCSV_Parse_EscapedQuote` | `""` inside quoted field becomes a literal `"` |
| `TestCSV_Parse_EmbeddedNewline` | Quoted field spanning lines preserved |
| `TestCSV_Parse_BOMStripped` | Leading UTF-8 BOM removed before parse |
| `TestCSV_Parse_CRLF` | `\r\n` line endings handled |
| `TestCSV_Parse_TabDelimiter` | `delimiter="\t"` |
| `TestCSV_Parse_CommentSkipped` | Lines starting with comment char ignored |
| `TestCSV_Parse_EmptyLineSkipped` | `skipEmptyLines=true` drops blank lines |
| `TestCSV_Parse_TrimSpaces` | `trimSpaces=true` strips both leading and trailing whitespace |
| `TestCSV_Parse_BufferInput` | `[]byte` input accepted |
| `TestCSV_Parse_Cast_Bool` | `cast=true` coerces `true`/`false` |
| `TestCSV_Parse_Cast_Int` | Integer-looking cell → `int64` |
| `TestCSV_Parse_Cast_Float` | Decimal cell → `float64` |
| `TestCSV_Parse_Cast_EmptyToNil` | Empty cell → `nil` |
| `TestCSV_Parse_Cast_Off` | `cast=false`: every cell stays string |
| `TestCSV_Parse_OutputRows_FanOut` | `output=rows` emits N messages with isFirst/isLast/index/total |
| `TestCSV_Parse_OutputRows_EmptyCSV` | No data rows → no output messages |
| `TestCSV_Parse_OutputRows_PreservesMsg` | Other `msg.*` properties propagate to all fan-out messages |
| `TestCSV_Parse_OutputRows_ColumnsAttached` | `msg.columns` present when header or columns config set |
| `TestCSV_Parse_OutputArray_EmptyCSV` | Empty CSV → single message with empty slice |
| `TestCSV_Parse_Malformed_Error` | Unbalanced quote → catchable error, status red |
| `TestCSV_Parse_WrongType_Error` | Number input + `action=parse` → catchable error |
| `TestCSV_Stringify_ObjectsWithHeader` | `[]map` + `header=true` → header row + data rows |
| `TestCSV_Stringify_ObjectsExplicitColumns` | `columns` config wins over key order; missing keys → empty cells; extra keys dropped |
| `TestCSV_Stringify_ObjectsNoHeader_NoColumns_Error` | object-shape input + `header=false` + no `columns` → catchable error |
| `TestCSV_Stringify_ArrayOfArrays` | `[][]any` input → positional rows |
| `TestCSV_Stringify_QuoteWhenNeeded` | Field with delimiter/quote/newline gets quoted |
| `TestCSV_Stringify_ForceQuote` | `forceQuote=true` quotes every field |
| `TestCSV_Stringify_CRLFNewline` | `newline="\r\n"` produces `\r\n` line endings |
| `TestCSV_Stringify_CustomDelimiter` | `delimiter=";"` honored on output |
| `TestCSV_Stringify_NilCellEmpty` | `nil` value renders as empty cell |
| `TestCSV_Stringify_StringInput_Error` | `action=stringify` with string input → catchable error |
| `TestCSV_Stringify_UnsupportedShape_Error` | scalar input → `"unsupported input type"` |
| `TestCSV_Auto_StringInput_Parses` | `action=auto` + string → parse branch |
| `TestCSV_Auto_ObjectInput_Stringifies` | `action=auto` + slice of maps → stringify branch |
| `TestCSV_RoundTrip` | `parse → stringify` reproduces original CSV (modulo whitespace) |
| `TestCSV_Stream_CompleteChunk` | `msg.filename` set, chunk ends with `\n` → rows emitted, residue empty |
| `TestCSV_Stream_PartialLastRow` | Chunk ends mid-row (no trailing `\n`) → partial bytes buffered, no extra row emitted |
| `TestCSV_Stream_PartialThenComplete` | Second chunk completes the row started in prior chunk → row emitted with merged content |
| `TestCSV_Stream_NoNewlineInChunk` | Entire chunk is one partial row → no output, residue accumulated |
| `TestCSV_Stream_MultipleChunks` | Five successive chunks, partial row in each → only complete rows emitted per chunk |
| `TestCSV_Stream_CsvPositionAttached` | `msg.csvPosition` on each emitted message equals `msg.position - len(residue)` |
| `TestCSV_Stream_FilenamePassthrough` | `msg.filename` propagated to all fan-out messages |
| `TestCSV_Stream_StatePersistedInFlowPers` | After a chunk, state stored under expected key in flowPers |
| `TestCSV_Stream_StateSurvivesRestart` | Residue and offset loaded from flowPers on cold start |
| `TestCSV_Stream_HeaderOnlyInFirstChunk` | Header parsed from chunk 1; chunk 2 treated as data rows (no re-parse of header) |
| `TestCSV_Stream_HeaderPersistedInState` | Parsed columns saved in csvFileState.Columns; reloaded on restart |
| `TestCSV_Stream_Reset_ClearsResidue` | `msg.reset=true` discards stale residue; bytes from new chunk processed fresh |
| `TestCSV_Stream_Reset_ClearsColumns` | `msg.reset=true` clears persisted columns so next chunk re-reads header |
| `TestCSV_Stream_TruncationAfterPartial` | Residue from prior chunk + reset → residue discarded, not prepended to new chunk |
| `TestCSV_Stream_TwoFilesIndependentState` | Two different `msg.filename` values → separate state keys, no interference |
| `TestCSV_Stream_NoFilename_NormalMode` | Message without `msg.filename` → normal parse, no state stored |
| `TestCSV_Stream_CRLFBoundary` | `\r\n` endings in chunk: last boundary found at `\r`, not mid-sequence |
| `TestCSV_Stream_ArrayOutput` | `output=array` in streaming mode → single message with `csvPosition` set |

## Dependencies

- `flow.Message` with `Get`/`Set` and a fan-out helper (`Clone()` from
  the Split node) — exists.
- Catch node for error forwarding — exists.
- `flow.ContextProvider` / `flow.ContextStore` for per-file streaming
  state persistence — same interface used by `file-in` cursor — exists.
- Standard library: `encoding/csv`, `crypto/sha256`, `encoding/hex`,
  `bytes` — no new module.

## Out of scope for Phase 1

- **Streaming for very large CSVs in a single message** — the node reads
  the full payload into memory when no `msg.filename` is present. Chunked
  streaming via `file-in incremental` is fully supported (see section 11).
  A `csv-stream` node operating on a single multi-GB blob is a follow-up.
- **Encodings other than UTF-8** — Windows-1252 / UTF-16 conversion is
  a separate concern (and ideally lives in a dedicated decode node).
- **flow./global. scopes** — see `PARSER_NODES.md`.
- **Schema / column-type configuration** — `cast` is intentionally
  scalar and uniform. A typed-column spec (e.g. "col 3 = float, col 5 =
  ISO-8601 date") is deferred to a `csv-typed` follow-up node or to a
  Change node downstream.
- **Multi-character delimiters** — `encoding/csv` only supports a
  single rune; multi-character separators would need a custom parser.
- **Configurable escape character** distinct from `quoteChar` — Go's
  `encoding/csv` follows RFC 4180 (`""` escapes a quote inside a quoted
  field) and does not support a separate escape char. Add only if a
  concrete dialect requires it.
- **Detecting `\r\n` vs `\n` automatically on parse** — `encoding/csv`
  already accepts both transparently; no config needed.
- **Strict per-row column-count validation** — `FieldsPerRecord = -1`
  means short rows do not error. A `strict` toggle can be added later
  if a use case appears.

## Open questions

- **Type name**: `csv` (short, consistent with `json`/`xml`) or
  `parser-csv` (explicit). Suggestion: `csv` — matches the established
  pattern.
- **Default `header`**: `true` or `false`? Suggestion: `true` — the
  vast majority of CSVs in industrial integrations carry headers, and
  the failure mode of `header=true` on a headerless CSV (first row
  becomes column names) is loud and easy to diagnose, whereas
  `header=false` on a CSV with a header silently emits an off-by-one
  row of strings.
- **Empty CSV in `output=rows` mode**: emit nothing (current proposal)
  or one message with `total: 0`? Suggestion: emit nothing — consistent
  with `file-in` incremental and avoids forcing downstream nodes to
  filter `total === 0` everywhere. Operators who need a sentinel can
  use `output=array`.
- **`cast` heuristics for `null`**: should literal strings `"null"` and
  `"NULL"` also become `nil`, or only the empty cell? Suggestion:
  empty-only — matches Postgres `COPY` defaults and avoids surprising
  conversions of legitimate strings.
- **Stringify of nested values**: a cell value that is itself a map or
  slice — render as `fmt.Sprint(v)` (current proposal), JSON-encode
  inline, or error? Suggestion: `fmt.Sprint(v)` for Phase 1 — predictable
  for primitives; users with nested data should serialize with the JSON
  parser first.
- **`output=rows` on large CSVs**: emitting 100k messages back-to-back
  may swamp downstream nodes. Should there be a per-message yield (e.g.
  `runtime.Gosched()` every N rows)? Suggestion: defer until measured —
  the existing Split node has the same pattern and no yield, and a
  premature yield is worse than no yield if it changes message ordering
  guarantees.
- **Round-trip stability**: after `parse → stringify`, are quoting
  decisions identical to the input? Not necessarily — `encoding/csv`
  re-quotes on a need-only basis, which may change cosmetic quoting
  even when the content is unchanged. Document this as a known
  property; `forceQuote=true` produces a deterministic output for
  diff-friendly round trips.

---

# Phase 2 — CSV Out Node (`csv-out`)

## Status: Proposed

## Context

Phase 1 ships the bidirectional CSV parser (`csv`). It works in isolation
but exposes architectural friction when chained with `file-out` for the
common "log to CSV file" pattern. This phase adds a dedicated **`csv-out`**
node that bundles CSV serialization with file writing into a single node
that owns the destination-file lifecycle.

The gap analysis identified the following problems with the
`csv (stringify) → file-out (append)` chain:

1. **Header duplication on append**: `csv` does not know whether the
   target file already has a header on disk. The `headerOnce` workaround
   solves this only within a single node lifetime — a redeploy or restart
   re-emits the header, producing duplicates inside the existing file.
2. **Two-step config**: users must configure both nodes consistently
   (newline behavior, encoding, path), and a misconfiguration on either
   side produces broken CSV.
3. **Trailing-newline collision**: `csv` writes `\n` after every row;
   `file-out appendNewline=true` adds another. Easy to misconfigure into
   blank lines between records.
4. **No file-state feedback**: the stringify direction can never adapt
   to what is already on disk (existing header, file size, rotation).

`csv-out` solves all four by owning both serialization and the file
handle: it stats the file before each write, decides whether a header is
needed, and writes the result atomically.

## Problem description

A flow that appends sensor readings to a CSV file every tick needs:

- The header to be written **exactly once**, on the very first write to
  this file.
- Every subsequent write to **only contain data rows**, regardless of
  node lifetime, redeploys, or restarts.
- Line termination to be **exactly one `\n` per row**.
- A clean failure mode when the file disappears or is truncated mid-flow.

The `csv-out` node delivers this as one drop-in node, no second config to
keep in sync.

## View / rationale

- **One node, one job**: write structured data as CSV to a file. No chain
  to maintain.
- **File-state-aware header logic**: the disk decides whether the header
  is needed, not a lifetime flag. Works across redeploys.
- **Single point of truth for line endings**: `csv-out` owns both the
  CSV writer and the file handle, so the trailing-newline question
  collapses to one config field.
- **Same UX as `csv stringify`**: the stringify-relevant fields (header,
  columns, delimiter, quoteChar, forceQuote) keep their names so users
  familiar with `csv` can switch over without re-learning.

## Requirements

### 1. Property (input)

| Field | Description | Default |
|---|---|---|
| `property` | Property on `msg` to serialize as CSV rows. Same shapes as `csv stringify` (object, array of objects, array of arrays). | `payload` |

### 2. Path

| Field | Description | Default |
|---|---|---|
| `path` | Absolute file path. Supports `{{mustache}}` over `msg`. | _(required)_ |
| `rootJail` | Optional containment directory. Resolved path must stay inside. | `""` |
| `createDirs` | Create parent directories if they do not exist. | `false` |

`path` follows the same template semantics as `file-out`: per-message
mustache interpolation lets users route writes by topic, date, tenant,
etc.

### 3. Mode

| Mode | Behavior on each message |
|---|---|
| `append` | **Default.** Open with `O_APPEND`. Header is written **iff the file is empty or missing** at write time. |
| `overwrite` | Open with `O_TRUNC`. Header is **always** written (file starts empty after truncation). |
| `create` | Open with `O_EXCL`. Header is always written. Error if the file already exists. |

The header-writing decision is fully determined by mode + on-disk state.
There is no `headerOnce` flag — the node always does the right thing for
the configured mode.

### 4. Header policy (advanced)

| Field | Description | Default |
|---|---|---|
| `header` | When true, emit a header row according to the mode rules above. | `true` |
| `columns` | Explicit column order. Same semantics as `csv stringify`. | `[]` |

For object inputs, `columns` is the only deterministic way to pin
column order across messages (Go map iteration is unordered).

### 5. CSV format

| Field | Description | Default |
|---|---|---|
| `delimiter` | Field separator. Presets `,` `;` `\t` `|` `custom`. | `,` |
| `quoteChar` | RFC-4180 quote char. Phase 1 of `csv` restricts this to `"`; same restriction applies here. | `"` |
| `forceQuote` | Quote every field, not only those that need it. | `false` |
| `newline` | `\n` or `\r\n`. The line terminator after each row, **including the last**. | `\n` |

There is no `appendNewline` option — every row is line-terminated by
construction. Chaining `csv-out` with anything else is not the intent.

### 6. Encoding

| Field | Description | Default |
|---|---|---|
| `encoding` | `auto` / `utf-8`. Other encodings out of scope (same as `csv` Phase 1). | `auto` |

`auto` resolves to UTF-8 for `.csv`/`.txt`/`.log`/`.tsv` extensions (and
the file gets a BOM-less UTF-8 write). Other extensions error at `Init`.

### 7. Inputs / outputs

- **1 input**, **1 output**.
- The output is **pass-through**: the original message is forwarded
  unchanged after a successful write so downstream nodes can chain
  (typical use: a Debug node confirming the write).
- The output message gains:
  - `msg.filename` — resolved absolute path that was written.
  - `msg.bytesWritten` — bytes appended in this call.
  - `msg.fileSize` — total size of the file on disk after the write.
  - `msg.headerWritten` — `true` if this call emitted the header row.

### 8. Header-decision algorithm

For each incoming message, in order:

1. Resolve `path` (mustache + jail + `msg.filename` override).
2. `os.MkdirAll` parent if `createDirs=true`.
3. **Stat the resolved path**. Three outcomes:
   - **Missing**: needHeader = `header && true`. Open with the
     mode-appropriate flags (`O_CREATE`).
   - **Exists, size == 0**: needHeader = `header && true`. Open per mode.
   - **Exists, size > 0**:
     - `append`: needHeader = `false`.
     - `overwrite`: needHeader = `header && true` (truncation happens
       on `O_TRUNC`).
     - `create`: error `"file already exists"`.
4. Serialize the payload to CSV bytes using the same writer as `csv
   stringify`. Prepend the header row if `needHeader`.
5. Write to disk in one syscall. Close.
6. Re-stat for `fileSize`. Emit pass-through message with metadata.

The stat-then-write window is **not** atomic — a concurrent writer
between steps 3 and 5 can leave the file with a duplicate header (rare,
benign) or with no header (impossible — only `csv-out` writes via this
node). For single-writer scenarios (the dominant use case) the algorithm
is correct.

### 9. Status / error handling

| Outcome | Status | Catchable |
|---|---|---|
| Successful write | `blue` / `wrote N B` | — |
| Header written (this call) | `blue` / `wrote N B · header` | — |
| Path resolves outside jail | `red` / `path escapes jail` | yes |
| Encode error (unsupported payload shape) | `red` / `csv encode error` | yes |
| Mkdir error | `red` / `mkdir error` | yes |
| `create` mode + file exists | `red` / `file exists` | yes |
| Write error (permission, disk full) | `red` / `write error` | yes |

Errors propagate via the catch pipeline (same pattern as `file-out` and
`csv`).

## Examples

### Example 1 — Continuous sensor logging

```
[inject every 1s, payload={ts, sensor, value}]
  →  [csv-out: append, columns="ts,sensor,value", path="/var/log/sensors.csv"]
  →  [debug]
```

Behavior:
- First tick: file is missing → header written, then first row appended.
- Every subsequent tick: file size > 0 → only the data row appended.
- After a redeploy: file still has data on disk → still no header. The
  problem `headerOnce` partially solved is fully solved here.

### Example 2 — Per-day file with mustache path

```
[inject hourly]  →  [csv-out: path="/var/log/{{date}}.csv", createDirs=true]
```

`msg.date = "2026-05-18"` → writes to `/var/log/2026-05-18.csv`.
First hour of a new day: header is written (new file). Subsequent hours
of the same day: only data rows. Day rolls over → new file, header
again. No special logic needed beyond mustache + the algorithm in §8.

### Example 3 — Overwrite full file per message (snapshot)

```
[change: build full snapshot array]  →  [csv-out: overwrite, path="/var/cache/state.csv"]
```

Every message replaces the file. Header is always written (mode is
`overwrite`).

### Example 4 — Strict create (fail on existing)

```
[csv-out: create, path="/var/run/{{batchId}}.csv"]
```

Each batch produces a fresh file. Duplicate batch IDs error out with a
catchable `"file exists"` error.

### Example 5 — Filename override per message

```
[change: msg.filename = "/var/log/{{tenant}}.csv"]
  →  [csv-out: append, createDirs=true]
```

`msg.filename` overrides the configured `path` (same precedence rule as
`file-out`).

## Technical sketch

### Backend — `internal/nodes/core/csv_out.go` (new)

```go
type CSVOutNode struct {
    config flow.NodeConfig
    nodes.BaseNode

    // CSV format (mirrors CSVParserNode where applicable)
    property   string
    header     bool
    columns    []string
    delimiter  rune
    quoteChar  rune
    forceQuote bool
    newline    string

    // File destination
    path       string
    mode       string // "append" | "overwrite" | "create"
    encoding   string
    createDirs bool
    rootJail   string

    inErrorState bool
}
```

- **Inputs:** 1, **Outputs:** 1
- Implements `flow.NodeInstance`.
- No `ContextProvider` needed — header decision comes from disk state,
  not from persistent context.
- Re-uses `stringifyCSV()` / `normalizeStringifyInput()` /
  `writeCSVRow()` from `parser_csv.go`. Shared helpers refactored out
  into `csv_format.go` if needed.

#### Handle-message flow (pseudocode)

```go
func (n *CSVOutNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
    if msg == nil { return nil, nil }

    resolved, err := resolvePath(n.path, msg, n.rootJail)
    if err != nil { return n.fail("path error", err) }

    if n.createDirs {
        if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
            return n.fail("mkdir error", err)
        }
    }

    needHeader, mode, err := n.decideHeader(resolved)
    if err != nil { return n.fail(fileWriteErrorLabel(err), err) }

    payload := msg.Get(n.property)
    body, err := n.serialize(payload, needHeader)
    if err != nil { return n.fail("csv encode error", err) }

    if err := writeFile(resolved, body, mode); err != nil {
        return n.fail(fileWriteErrorLabel(err), err)
    }

    n.clearError()
    info, _ := os.Stat(resolved)
    msg.Set("filename", resolved)
    msg.Set("bytesWritten", len(body))
    msg.Set("fileSize", info.Size())
    msg.Set("headerWritten", needHeader)
    return [][]*flow.Message{{msg}}, nil
}

func (n *CSVOutNode) decideHeader(path string) (needHeader bool, mode string, err error) {
    info, statErr := os.Stat(path)
    switch {
    case os.IsNotExist(statErr):
        return n.header, n.mode, nil
    case statErr != nil:
        return false, "", statErr
    }
    switch n.mode {
    case "append":
        return n.header && info.Size() == 0, "append", nil
    case "overwrite":
        return n.header, "overwrite", nil
    case "create":
        return false, "", fmt.Errorf("%w: %s", errFileExists, path)
    }
    return false, "", fmt.Errorf("unknown mode %q", n.mode)
}

func (n *CSVOutNode) serialize(payload any, withHeader bool) ([]byte, error) {
    // Reuse stringifyCSV but with the header decision controlled by us
    // rather than n.header. Internally, the CSV writer is the same path
    // as parser_csv.go's writeCSVRow + quoting rules.
    return stringifyCSVFor(payload, n.columns, withHeader, n.delimiter,
        n.quoteChar, n.forceQuote, n.newline)
}
```

`writeFile` is the same helper from `file_out.go`; consider moving it
to a shared `filesystem` or `nodes` package if cross-package import is
desirable, or duplicate three lines.

### Frontend — `frontend/src/nodes/core/CSVOutConfig.vue` (new)

Field layout (top to bottom):

- Path (text input, mustache hint)
- Mode (append / overwrite / create dropdown)
- Property (dot-path)
- Header (toggle)
- Columns (comma-separated)
- Delimiter (preset + custom char input)
- Force quote (toggle)
- Newline (`\n` / `\r\n` dropdown)
- Create parent directories (toggle)

### Frontend — Canvas node — `frontend/src/components/nodes/CSVOutNode.vue` (new)

Body line shows e.g. `append → /var/log/sensors.csv` or
`overwrite · {{date}}.csv`.

### Registration

- `internal/nodes/core/init.go` — add
  `{Type: "csv-out", Factory: NewCSVOutNode, Info: CSVOutTypeInfo()}`.
- `frontend/src/nodes/core/index.ts` — add `'csv-out': 'process'` and
  the dynamic import.
- `frontend/src/views/FlowEditor.vue` — add `<template #node-csv-out>`
  slot + import.
- `frontend/src/components/nodes/NodeIcon.vue` — add `csv-out` icon
  (variant of `csv` with a small "→" overlay, or reuse `file-out` icon).

## Affected files

### Backend
- `internal/nodes/core/csv_out.go` (new)
- `internal/nodes/core/csv_out_test.go` (new)
- `internal/nodes/core/init.go` — registration
- `internal/nodes/core/parser_csv.go` — extract shared serializer if
  needed; mark `headerOnce` as deprecated for file-destined flows (doc
  comment only, no behavior change).

### Frontend
- `frontend/src/nodes/core/CSVOutConfig.vue` (new)
- `frontend/src/components/nodes/CSVOutNode.vue` (new)
- `frontend/src/nodes/core/index.ts` — register `csv-out`
- `frontend/src/views/FlowEditor.vue` — slot + import
- `frontend/src/components/nodes/NodeIcon.vue` — icon

## Tests

| Test | Verifies |
|---|---|
| `TestCSVOut_Append_FirstMessage_WritesHeader` | New file → header + data row on disk |
| `TestCSVOut_Append_SecondMessage_NoHeader` | Existing non-empty file → only data row appended |
| `TestCSVOut_Append_AcrossRestart_NoDuplicateHeader` | New node instance against existing file → no duplicate header |
| `TestCSVOut_Append_EmptyFileExists_WritesHeader` | Zero-byte file present → header written |
| `TestCSVOut_Overwrite_AlwaysWritesHeader` | Mode=overwrite → header on every message |
| `TestCSVOut_Create_FailsIfExists` | Mode=create + existing file → catchable error |
| `TestCSVOut_Create_WritesHeaderOnNewFile` | Mode=create + missing file → header written |
| `TestCSVOut_HeaderFalse_NeverWritesHeader` | `header=false` → no header regardless of mode/state |
| `TestCSVOut_ColumnsPin_Order` | `columns="b,a"` → output preserves that order |
| `TestCSVOut_SingleObjectInput` | `map[string]any` payload → one row written |
| `TestCSVOut_ArrayObjectInput` | `[]map[string]any` payload → multiple rows written |
| `TestCSVOut_ArrayArrayInput_NoHeader` | `[][]any` payload + `header=false` → positional rows |
| `TestCSVOut_CRLFNewline` | `newline="\r\n"` → CRLF line endings on disk |
| `TestCSVOut_ForceQuote` | Every field quoted, embedded quotes doubled |
| `TestCSVOut_CustomDelimiter` | `delimiter=";"` honored on disk |
| `TestCSVOut_Path_MustacheTemplate` | `{{date}}` resolves from msg, file written at resolved path |
| `TestCSVOut_Path_MsgFilenameOverride` | `msg.filename` wins over configured path |
| `TestCSVOut_CreateDirs_True` | Non-existent parent dir created |
| `TestCSVOut_CreateDirs_False_NotFound_Error` | Non-existent parent → catchable error |
| `TestCSVOut_JailViolation` | Path outside `rootJail` → catchable error |
| `TestCSVOut_Passthrough_PreservesMsg` | Output message contains original fields plus filename/bytesWritten/fileSize/headerWritten |
| `TestCSVOut_StatusOnSuccess` | Status updates to `blue` with byte count |
| `TestCSVOut_StatusOnError_RecoverOnSuccess` | Red status on error, cleared on next success |
| `TestCSVOut_EncodeError_UnsupportedPayload` | Scalar payload → catchable "csv encode error" |
| `TestCSVOut_HeaderWrittenField` | `msg.headerWritten` is `true` on first message, `false` on subsequent |
| `TestCSVOut_AtomicSingleWrite` | Header + rows hit disk in a single syscall (no torn read possible for typical batches) |

## Dependencies

- `flow.Message` with `Get`/`Set` (exists)
- Catch node for error forwarding (exists)
- `csv` node's stringify helpers (refactor candidate: move
  `stringifyCSV`, `normalizeStringifyInput`, `writeCSVRow`,
  `formatCell` into `internal/nodes/core/csv_format.go`, used by both
  `parser_csv.go` and `csv_out.go`)
- `file-out`'s helpers: `resolvePath`, `writeFile`, `fileWriteErrorLabel`,
  `errFileExists` (refactor candidate: move to a shared
  `internal/nodes/filesystem/exports.go` or duplicate the small bits)
- Standard library: `encoding/csv`, `os`, `path/filepath`, `strings`,
  `fmt`. No new modules.

## Migration & deprecation

After `csv-out` lands:

- The `csv` node's `headerOnce` config field is marked **deprecated for
  file destinations**. The frontend tooltip recommends `csv-out` for any
  `csv → file-out` chain.
- `headerOnce` remains supported for non-file destinations (HTTP body,
  MQTT payload, etc.) where lifetime semantics are acceptable. No
  behavior change.
- A migration note in the release changelog: existing flows using
  `csv stringify + file-out append + headerOnce=true` should switch to
  `csv-out append` to gain redeploy-safety. The old chain continues to
  work for backwards compatibility.

## Out of scope for Phase 2

- **File rotation** (size limits, daily rotation) — same as `file-out`,
  use external tooling or a future `csv-out` extension.
- **Concurrent writers** — single-writer flows only. Multi-writer setups
  need an external lock; `csv-out` does not coordinate.
- **Per-row atomic guarantees** — single `write()` syscall is atomic up
  to `PIPE_BUF` (~4 KB on Linux); for larger batches the kernel may
  split. Document as a known limitation.
- **Non-UTF-8 encodings** — same as Phase 1.
- **Streaming write of huge batches** — the whole serialized buffer
  must fit in memory before the syscall. A streamed writer is a future
  follow-up.
- **Encryption / compression at rest** — orthogonal feature.

## Open questions

- **Header on truncation in `append` mode**: if an external process
  truncates the file between two messages, `csv-out` will see
  `size == 0` on the next stat and re-emit the header. This is correct
  behavior (a truncated file needs a fresh header), but the lack of a
  signal upstream may surprise downstream consumers that already
  parsed the previous content. Suggestion: surface `msg.fileRotated =
  true` when size dropped between consecutive writes from this node.
  Defer to a follow-up if needed.
- **Node type name**: `csv-out` (consistent with `file-out`) vs
  `csv-write` (more explicit) vs `csv-file` (groups by domain).
  Suggestion: `csv-out` — symmetric with `file-out` which it pairs
  with conceptually.
- **Category**: `parser` (consistent with `csv` / `json` / `xml`) or
  `filesystem` (consistent with `file-out`)? Suggestion: `parser` —
  the user's mental model is "I want CSV-shaped output", and the file
  destination is the implementation detail.
- **Stat-then-write race**: the algorithm in §8 is non-atomic between
  stat and write. For a single-writer flow this is fine. Should we
  document an `O_EXCL` first-write strategy that probes empty-file
  status via "try to create exclusively, fall back to append" instead?
  Suggestion: stick with stat-then-write for clarity; the race window
  is single-writer-only, where it cannot fire.
- **Reuse vs duplication of `file-out` helpers**: refactor into a
  shared package, or duplicate the ~30 lines? Suggestion: refactor —
  the helpers (`resolvePath`, `writeFile`, encoding switch) are about
  to have a third consumer (`csv-out`), and the abstraction earns its
  keep.
- **`headerWritten` field semantics on no-header configs**: when
  `header=false`, `msg.headerWritten` is always `false`. Should the
  field be omitted entirely instead? Suggestion: always present so
  downstream Switch nodes can rely on its presence.
