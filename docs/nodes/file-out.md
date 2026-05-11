# File Out (`file-out`)

Writes `msg.payload` to a file. Three write modes cover the main use
cases: replace, append, and create-only. Path can be a static config
value, a Mustache template resolved from the message, or supplied
per-message via `msg.filename`.

| Inputs | Outputs |
|--------|---------|
| 1      | 0 *(silent on success)* |

## Configuration

| Field           | Default     | Description |
|-----------------|-------------|-------------|
| `path`          | *(empty)*   | File path. May contain `{{mustache}}` placeholders resolved from the message (e.g. `/data/{{topic}}.log`). Leave empty to use `msg.filename`. |
| `mode`          | `overwrite` | `overwrite` (replace entire file) \| `append` (add to end) \| `create` (fail if file already exists). |
| `encoding`      | `utf8`      | `utf8` \| `utf16le` \| `latin1` \| `base64` \| `hex` \| `buffer`. Applies to string payloads. Buffer / `[]byte` payloads are always written as raw bytes. |
| `createDirs`    | `false`     | Automatically create parent directories if they do not exist. |
| `appendNewline` | `false`     | Append a `\n` after the payload. Convenient for line-oriented log files. |
| `rootJail`      | *(empty)*   | Resolved paths must stay inside this directory. Empty = no restriction. |

## Path resolution

1. If `path` is set in config, it is used (after Mustache expansion).
2. If `path` is empty, `msg.filename` is used.
3. The resolved path is normalised and checked against `rootJail` if set.

Mustache placeholders expand message fields: `{{topic}}`, `{{payload.id}}`,
`{{filename}}`, etc. Unknown placeholders expand to an empty string.

## Payload encoding

| `msg.payload` type | Written as |
|--------------------|------------|
| `string`           | String encoded with the configured `encoding`. |
| `object` / `array` | JSON-serialised (`JSON.stringify`), then written as UTF-8. |
| `Buffer` / `[]byte`| Raw bytes, regardless of `encoding` setting. |
| `number` / `bool`  | Converted to string, then encoded. |

## Error handling

| Condition | Status | Routed as |
|-----------|--------|-----------|
| Path not found (parent dir missing, createDirs=off) | `red` | Catchable error |
| Permission denied | `red` | Catchable error |
| Mode=create and file exists | `red` / `file exists` | Catchable error |
| Path outside rootJail | `red` / `path escapes jail` | Catchable error |

The node is silent on success — no output message is emitted.

## Examples

### Append a log line per message

```
[MQTT-In: sensor/events]
  →  [Function: format line]
  →  [File Out: path=/var/log/sensor.log, mode=append, appendNewline=on]
```

Each MQTT message adds one line. No risk of overwriting previous entries.

### Per-topic files with Mustache

```
[MQTT-In: data/#]
  →  [File Out: path=/data/{{topic}}.json, mode=overwrite]
```

`msg.topic = "data/sensor/temp"` → writes `/data/data/sensor/temp.json`.
Parent directories are created automatically when `createDirs=on`.

### Pretty-print JSON to file

```
[Inject]
  →  [Change: set msg.payload = {ok: true, value: 42}]
  →  [JSON Parser: action=stringify, indent=2]
  →  [File Out: path=/data/result.json, mode=overwrite]
```

Object payloads are JSON-serialised by the JSON Parser before writing.

### Exactly-once file creation

```
[HTTP-In: POST /upload]
  →  [File Out: mode=create, createDirs=on]
  →  [HTTP-Out: 201 Created]
```

If the file already exists the node raises a catchable error — a Catch
node can return `409 Conflict` to the client.

## Notes

- **Object payloads**: File Out JSON-serialises objects automatically
  (compact). For pretty output, add a **JSON Parser** (stringify, indent=2)
  node upstream.
- **append + appendNewline**: the combination produces clean line-separated
  log files even if the payload itself doesn't end with a newline.
- **rootJail**: recommended in any flow that accepts user-controlled paths
  (`msg.filename`, Mustache from MQTT topics) to prevent path-traversal
  writes outside a safe directory.
- **No output on success**: the node is intentionally silent. If you need
  to confirm the write upstream, add a Catch node for errors only — success
  is implicit.

## See also

- **File Read** (`file-read`) — read file content on trigger.
- **File Watch** (`file-watch`) — watch a directory for new or changed files.
