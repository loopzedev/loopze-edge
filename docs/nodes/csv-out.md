# CSV Out (`csv-out`)

Serialises `msg.payload` (or any message property) as CSV and writes it
to a file in one step. **File-state aware** — the header decision is
made by stat'ing the target file, so a redeploy or restart never produces
duplicate header rows in append mode.

| Inputs | Outputs |
|--------|---------|
| 1      | 1       |

## Why a dedicated node?

The chain `[CSV: stringify] → [File Out: append]` works for one-shot
runs but breaks on redeploy: the CSV node has no way to know whether the
target file already carries a header, and a `headerOnce` lifetime flag
resets at every Start(). `csv-out` solves this by owning both the CSV
writer and the file handle: it stats the file before each write, decides
whether the header is needed, and writes the result in a single syscall.

For non-file destinations (HTTP body, MQTT payload) keep using
[`csv`](csv-parser.md) in stringify mode — its `headerOnce` config fits
those lifetime-scoped patterns.

## Configuration

| Field        | Default     | Description |
|--------------|-------------|-------------|
| `path`       | *(required)*| Absolute path to the file. Supports `{{mustache}}` templates over `msg`. |
| `mode`       | `append`    | `append` \| `overwrite` \| `create`. See [Mode](#mode). |
| `createDirs` | `false`     | Run `MkdirAll` for parent directories before writing. |
| `rootJail`   | *(empty)*   | Resolved paths must stay inside this directory. Empty = no restriction. |
| `property`   | `payload`   | Dot-path of the message field to serialise. |
| `header`     | `true`      | Whether to ever emit a header row. See [Header decision](#header-decision). |
| `columns`    | *(empty)*   | Comma-separated column list. Pins column order. Required for `[][]any` input when `header=true`. |
| `delimiter`  | `,`         | Field separator. UI presets: `,` `;` `\t` `\|` `custom`. |
| `quoteChar`  | `"`         | Quote character. Phase 1 supports only `"`. |
| `forceQuote` | `false`     | Quote every field, even when not required. |
| `newline`    | `\n`        | Line terminator: `\n` (LF) or `\r\n` (CRLF). Applied after every row. |
| `encoding`   | `auto`      | `auto` or `utf-8`. `auto` resolves to UTF-8 for `.csv` / `.txt` / `.log` extensions. |

## Mode

| Value       | Open flags                          | Behaviour |
|-------------|-------------------------------------|-----------|
| `append`    | `O_CREATE \| O_WRONLY \| O_APPEND`  | **Default.** Append to existing content. Header written **only** when the file is missing or empty. |
| `overwrite` | `O_CREATE \| O_WRONLY \| O_TRUNC`   | Replace existing content. Header always written (file starts empty after truncation). |
| `create`    | `O_CREATE \| O_WRONLY \| O_EXCL`    | Create a new file. Error if the file already exists. Header always written on success. |

## Header decision

For every incoming message, in order:

1. Resolve `path` (mustache + `msg.filename` override + jail check).
2. `MkdirAll(parent)` if `createDirs=true`.
3. `os.Stat(resolved)`:
   - **Missing** → `needHeader = header && true`. File will be created.
   - **Exists, size > 0**:
     - `append` → `needHeader = false`. File already has content.
     - `overwrite` → `needHeader = header && true`. Truncation happens on open.
     - `create` → error `"file already exists"`.
   - **Exists, size = 0** → `needHeader = header && true`. Empty file
     gets a fresh header.
4. Serialise `msg.<property>` to CSV (with or without header).
5. Single-syscall write to disk.
6. Re-stat for `fileSize` and emit the pass-through message.

The stat-then-write window is **not** atomic. For single-writer flows
(the dominant use case) this is correct.

## Output message fields

The original message is forwarded on the output port with these fields
added / overwritten:

| Field               | Value |
|---------------------|-------|
| `msg.filename`      | Resolved absolute path that was written. |
| `msg.bytesWritten`  | Bytes written in this call. |
| `msg.fileSize`      | Total size of the file on disk after the write. |
| `msg.headerWritten` | `true` if a header row was prepended in this call, `false` otherwise. |

## Accepted input shapes

| Shape                       | Behaviour |
|-----------------------------|-----------|
| `map[string]any`            | One row. Typical "one record per message" pattern. |
| `[]map[string]any`          | Multiple rows. Column order from `columns`; otherwise alphabetical. |
| `[][]any` / `[][]string`    | Positional rows. With `header=true`, `columns` is required to provide the header values. |
| `[]any` of the above        | Post-JSON shape (e.g. after `json` parse). |

Scalar payloads (`string`, `number`, `bool`) error with `csv encode
error`.

## Error handling

| Condition | Status | Routed as |
|-----------|--------|-----------|
| Path resolves outside `rootJail` | `red` / `path escapes jail` | Catchable error |
| Unsupported payload shape | `red` / `csv encode error` | Catchable error |
| Parent dir missing + `createDirs=false` | `red` / `mkdir error` | Catchable error |
| `create` mode + file exists | `red` / `file exists` | Catchable error |
| Write error (permission, disk full) | `red` / `write error` | Catchable error |

The status pill clears automatically on the next successful message.

## Examples

### Continuous sensor logging

```
[Inject every 1s, payload={ts, sensor, value}]
  →  [CSV Out: mode=append, columns="ts,sensor,value", path="/var/log/sensors.csv"]
  →  [Debug]
```

- **First tick**: file is missing → header written, then first row appended.
- **Every subsequent tick**: file size > 0 → only the data row appended.
- **After a redeploy** (or any restart): file still has content on disk →
  still no header. Zero `headerOnce` workaround needed.

### Per-day file with mustache path

```
[Inject hourly]
  →  [Change: msg.date = ... ]
  →  [CSV Out: path="/var/log/{{date}}.csv", createDirs=true]
```

`msg.date = "2026-05-19"` → writes to `/var/log/2026-05-19.csv`. First
hour of a new day: header written (new file). Subsequent hours: only
rows. Day rolls over → new file, header again.

### Overwrite full file per message (snapshot)

```
[Change: build full snapshot array]
  →  [CSV Out: mode=overwrite, columns="id,name,state", path="/var/cache/state.csv"]
```

Every message replaces the file. Header is always written.

### Strict create (fail on existing)

```
[CSV Out: mode=create, path="/var/run/{{batchId}}.csv"]
```

Each batch produces a fresh file. Duplicate batch IDs error out with a
catchable `"file exists"` error — wire a Catch node to route conflict
handling.

### Filename override per message

```
[Change: msg.filename = "/var/log/{{tenant}}.csv"]
  →  [CSV Out: mode=append, createDirs=true]
```

`msg.filename` always wins over the configured `path` (same precedence
as `file-out`). The configured `path` becomes the fallback template.

### Migrate from `csv stringify + file-out`

Before:

```
[CSV: action=stringify, headerOnce=true]  →  [File Out: mode=append, path=/var/log/sensors.csv]
```

After:

```
[CSV Out: mode=append, columns=..., path=/var/log/sensors.csv]
```

- One node instead of two.
- File-state aware: no duplicate header after redeploy / restart.
- Pass-through message gains `bytesWritten`, `fileSize`, `headerWritten`
  for downstream auditing.

## Notes

- **Single-writer assumption**: the stat-then-write algorithm assumes
  this node is the only writer to the target file. Concurrent writers
  from other processes can race; use external locking if needed.
- **Atomic append window**: a single `f.Write(data)` syscall is atomic
  up to `PIPE_BUF` (≈4 KB on Linux). For typical CSV rows (<80 bytes)
  this is safe; for large batches the kernel may split.
- **Trailing newline**: every row, including the last, ends with the
  configured `newline`. There is no separate `appendNewline` toggle —
  termination is handled internally.
- **`msg.filename` override**: per-message paths always win over the
  configured `path`. Use this for fan-out to many files from a single
  Inject / Function.
- **`rootJail`** uses lenient symlink resolution so writes to
  non-existing files inside the jail still pass the safety check.

## See also

- **[CSV Parser](csv-parser.md)** (`csv`) — read side. Use stringify mode
  only for non-file destinations.
- **[File Out](file-out.md)** — generic byte-level file writer. Use when
  you have a non-CSV payload.
- **[File Read](file-read.md)** + **[CSV Parser](csv-parser.md)** — read
  pipeline counterpart to `csv-out`.
