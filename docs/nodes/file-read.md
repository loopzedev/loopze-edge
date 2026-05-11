# File Read (`file-read`)

Reads a file's content on every incoming message. Two modes: **full read**
(entire file each time) and **incremental** (byte-cursor tail — only new
content since the last read). Path comes from the config or from
`msg.filename` per message.

| Inputs | Outputs |
|--------|---------|
| 1      | 1       |

## Why a File Read node?

Reading files in a flow today requires a Function node with `fs.readFile`
or similar. File Read does the same job declaratively — configure a path,
pick an encoding, and optionally enable incremental tailing for log files.

The incremental cursor persists across redeployments and restarts so
log-tail workflows survive flow updates.

## Configuration

| Field           | Default       | Description |
|-----------------|---------------|-------------|
| `path`          | *(empty)*     | Absolute path to the file. Leave empty to use `msg.filename` from each incoming message. |
| `encoding`      | `utf8`        | `utf8` \| `utf16le` \| `latin1` \| `base64` \| `hex` \| `buffer`. `buffer` emits a raw `Buffer` object. |
| `incremental`   | `false`       | Only read bytes added since the last run. Cursor persisted via flow context. |
| `fromStart`     | `false`       | Incremental only — emit all existing content on the first access (cursor starts at 0). Off = cursor pins to current EOF at start. |
| `delimiter`     | *(empty)*     | Split output into an array of lines at this byte sequence (e.g. `\n`). Empty = no split. |
| `maxLineBytes`  | `1048576`     | Safety limit per line when delimiter is set. Lines longer than this are truncated. |
| `rootJail`      | *(empty)*     | Resolved paths must stay inside this directory. Empty = no restriction. |

## Output message fields

| Field             | Value |
|-------------------|-------|
| `msg.payload`     | File content — string, `Buffer`, or `string[]` (when delimiter is set). |
| `msg.filename`    | Resolved absolute path used for the read. |
| `msg.encoding`    | Encoding applied to the content. |
| `msg.eof`         | *(incremental only)* `true` when cursor reaches the end of the file. |

## Incremental cursor

When `incremental` is on the node stores a byte offset in the flow's
persistent context. On each trigger it reads only the bytes added since
the last run:

- `fromStart=false` (default): cursor is pinned to the current file size
  at the first `Start()` call — only content written _after_ deploy is
  emitted.
- `fromStart=true`: cursor starts at 0 — all existing content is emitted
  on the first trigger.

The cursor is **line-aware**: it never advances past a partial trailing
line so content written mid-line by a writer is not emitted until the
newline arrives.

## Error handling

| Condition | Status | Routed as |
|-----------|--------|-----------|
| File not found | `red` / `file not found` | Catchable error |
| Permission denied | `red` / `permission denied` | Catchable error |
| Path outside rootJail | `red` / `path escapes jail` | Catchable error |
| Line exceeds maxLineBytes | payload truncated, `msg.truncated=true` | Forwarded |

## Examples

### Tail a growing log file

```
[Inject: every 5s]  →  [File Read: path=/var/log/app.log, incremental=on, delimiter=\n]  →  [Debug]
```

Each trigger emits only new lines appended since the last read. Cursor
survives redeployments.

### Watch → Read pipeline (recommended)

```
[File Watch: watch /data/incoming *.csv]
  →  [File Read: path=(empty), encoding=utf8]
  →  [Function: parse CSV lines]
```

File Watch emits `msg.payload.path`; File Read receives it as `msg.filename`
after a Change node sets `msg.filename = msg.payload.path`.

### Read a binary file on demand

```
[HTTP-In]  →  [File Read: path=/firmware/v1.bin, encoding=buffer]  →  [HTTP-Out]
```

`msg.payload` = `Buffer`. Pipe into `TCP-Out` or `HTTP-Out` directly.

### JSON-Lines log tailing

```
[Inject: every 1s]
  →  [File Read: incremental=on, delimiter=\n]
  →  [Split: by array element]
  →  [JSON Parser: auto]
  →  [Switch: msg.payload.level == "error"]
```

Each NDJSON line is parsed independently. The Split node fans each line
into its own message.

## Notes

- **msg.filename override**: when `path` is configured and `msg.filename`
  is also set, the config path wins. Leave `path` empty if you want
  per-message path control.
- **Encoding and Buffer**: `buffer` encoding bypasses all string decoding
  and emits a raw `Buffer`. Use it for binary files (images, firmware,
  compressed data).
- **Delimiter edge cases**: the last element of the output array may be
  an empty string if the file ends with a newline. Filter it downstream
  with a Switch node if needed.

## See also

- **File Watch** (`file-watch`) — event-driven filesystem monitoring;
  chain into File Read for watch-then-read pipelines.
- **File Out** (`file-out`) — write message payloads to files.
