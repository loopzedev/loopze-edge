# Issue: Filesystem Nodes — File Read, File Watch, File Write

## Status: Proposed (revised)

## Problem Description

Flows in industrial and edge contexts regularly need to interact with the
local filesystem: reading configuration files on startup, watching drop
folders for incoming SCADA reports, tailing growing log files for new
entries, writing log or archive CSVs, and reacting to file changes
without polling. Today this requires a Function node with `fs` access —
no UI, no error handling, fragile code.

Three new node types are introduced as the `filesystem` group, each with
**one tightly-scoped responsibility** that composes cleanly with the
others:

- `file-read` — **File Read** — reads a file on trigger. Optional
  **incremental tail mode** reads only bytes appended since the last
  read, with line-aware trim so partial writes never advance the cursor
  past an incomplete record.
- `file-watch` — **File Watch** — watches a single file **or** a folder
  for filesystem changes. Emits messages with **metadata only** —
  `file-watch` never reads file content. To act on the content, chain a
  `file-read` after it.
- `file-out` — **File Write** — writes or appends `msg.payload` to a file.

## Design rationale — why three nodes, not two

The previous design bundled "watch and read on each event" into a single
`file-in mode=read+watch` node. That coupled two concerns: detecting the
change, and consuming the changed data. Splitting them gives users:

- **One unambiguous mental model per node** (read vs watch vs write).
- **Composable pipelines**: `file-watch → file-read → parser` is easier to
  reason about than `file-in mode=read+watch incremental=true delimiter=…`
  bundled into one config.
- **Independent reuse**: a `file-watch` can fan out to multiple
  consumers — e.g. one branch reads the file, another logs the event to
  Slack — without re-watching the path twice.

## Overview

| Node Type | Type ID | Canvas In | Canvas Out | Description |
|---|---|---|---|---|
| **File Read** | `file-read` | 1 | 1 | Reads a file on trigger; optional incremental tail with line-aware trim |
| **File Watch** | `file-watch` | 0 or 1 ¹ | 1 | Watches a file/folder; emits metadata-only events. **Never reads content.** |
| **File Write** | `file-out` | 1 | 1 ² | Writes or appends payload to a file |

¹ `mode=read` (folder listing) → 1 input; `mode=watch`/`read+watch` → 0 inputs (source node)  
² Optional pass-through output; emits the unchanged `msg` after the write completes

## Typical Flow Shapes

**Read a config file on startup:**

```
[Inject once] → [file-read /etc/app/config.json] → [JSON Parser] → [Change]
```

**Tail a growing log (no watcher needed — Inject paces the pulls):**

```
[Inject 5s] → [file-read incremental=true /var/log/app.log] → [Split lines] → [Switch]
```

**React to a SCADA report drop folder — watch + read pattern:**

```
[file-watch /data/incoming watch *.csv] → [file-read msg.path] → [CSV Parser]
```

**Watch a single config file and reload it on every change:**

```
[file-watch /var/run/state.json watch] → [file-read incremental=true] → [JSON Parser]
```

**Append a log line on each event:**

```
[MQTT-in] → [Change: build line] → [file-out append /var/log/mqtt.log]
```

**Drop-folder fan-out (one watch, two consumers):**

```
                                 ┌─→ [file-read msg.path] → [CSV Parser]
[file-watch /drop watch] ───────┤
                                 └─→ [Slack-out: "new file"]
```

## Requirements

### 1. File Read Node (`file-read`)

`file-read` does one thing: read a file's content into `msg.payload`.
It is always triggered by an incoming message — there is no internal
timer or watcher.

#### Configuration

| Field | Type | Default | Description |
|---|---|---|---|
| `path` | string | — | Absolute path to the file. Supports `{{mustache}}` over `msg`. `msg.filename` overrides this |
| `encoding` | string | `auto` | `auto` (by extension), `utf-8`, `binary` |
| `incremental` | boolean | `false` | Only read bytes appended since the last read. Cursor is stored persistently in `flowPers` |
| `fromStart` | boolean | `false` | When `incremental=true`: start from byte 0 on first access. `false` = start from current EOF (skip existing content) |
| `delimiter` | string | `\n` ¹ | Line delimiter for incremental mode: `\n`, `\r\n`, `auto`, or `none` (raw bytes) |
| `maxLineBytes` | number | `1048576` | Safety cap for buffered partial lines. Catchable error when exceeded without a delimiter |
| `rootJail` | string | `""` | If non-empty, all resolved paths must stay inside this directory |

¹ `delimiter` defaults to `\n` for utf-8 / auto encoding; auto-falls-through
to `none` when the resolved encoding is `binary`. Override via the config
field for explicit control.

#### Encoding — `auto` rules

| Extension | Resolved encoding |
|---|---|
| `.txt`, `.json`, `.xml`, `.csv`, `.log`, `.yaml`, `.yml`, `.toml`, `.md` | `utf-8` |
| Everything else | `binary` |

#### Outgoing Message

```json
{
  "payload": "<content — string for utf-8, number array for binary>",
  "filename": "/absolute/path/to/file.txt",
  "encoding": "utf-8",
  "size": 1024,
  "position": 4096,
  "bytesRead": 512,
  "lineCount": 12,
  "reset": true
}
```

- `position`, `bytesRead` are only present when `incremental=true`.
  `position` is the byte offset after this read (next cursor position).
- `lineCount` is only present when `incremental=true` and `delimiter != "none"`
  — counts complete delimited records in `payload`.
- `reset` is set to `true` only when truncation/rotation was detected on
  this read (cursor was past the new file size).
- When `incremental=true` and there are no new bytes (or only a partial
  trailing line), the node emits **no message** and stays silent.
- Existing `msg` properties are preserved (pass-through semantics).

#### Incremental Read — Persistent Cursor

When `incremental=true`, the node maintains a byte-offset cursor per
file path and stores it in `flowPers` (NATS JetStream KV) under the
key `_filecursor.<sha256(nodeID+":"+absolutePath)[:16]>`. The node
implements `flow.ContextProvider` and receives its context store via
`SetContext()`.

**Read sequence on each trigger:**

1. `stat(path)` — get current file size.
2. Load cursor from `flowPers`. If absent → cursor = 0 if `fromStart=true`,
   else cursor = current file size (skip existing content).
3. If cursor > file size → file was truncated/rotated → reset to 0,
   set `msg.reset = true`.
4. If cursor == file size → no new bytes; emit nothing.
5. Open file, seek to cursor, read to EOF.
6. If `delimiter != "none"`: trim to last complete delimiter (see below).
7. Encode trimmed bytes; emit message; persist new cursor.

**Line-aware trim — partial line handling:**

Logs and CSVs are written line-by-line. A read that races with an
in-progress write may capture a partial trailing line. Advancing the
cursor past it would lose data on the next read. The trim returns the
prefix ending at the **last byte of the last complete delimiter**:

| Delimiter | Trim rule |
|---|---|
| `\n` | `i = lastIndexOf(buf, '\n')`. If found: emit `buf[0..i+1]`, advance cursor by `i+1`. |
| `\r\n` | `i = lastIndexOf(buf, '\r\n')`. If found: emit `buf[0..i+2]`, advance by `i+2`. |
| `auto` | LF-based: `i = lastIndexOf(buf, '\n')`. CRLF lines end in LF too, so this catches both. |
| `none` | No trimming. Entire buffer emitted. |

If no delimiter is found, **emit nothing and leave the cursor untouched**.
The next read picks up the partial line plus whatever was appended.

**Example:**

```
File grows from "alice\nbob\n" to "alice\nbob\ncarol\nda"  (write in progress)
Cursor before:    10
Buffer read:      "carol\nda"   (8 bytes)
Last \n at index: 5
Emitted payload:  "carol\n"     (6 bytes)
Cursor after:     16
Next read picks:  "da..." plus whatever was appended after
```

**maxLineBytes safeguard:**

If the buffer reaches `maxLineBytes` without a delimiter the file is
either misconfigured (binary content with delimiter set) or producing
abnormally long lines. The node sets status red `"line buffer
exceeded"`, raises a catchable error, and does **not** advance the cursor.

**Cursor reset:**

A message with `msg.resetCursor = true` clears the persisted cursor
without emitting content. Useful for forcing a re-read after a known
log rotation. A reset button in the Properties Panel + a
`POST /api/v1/file-read/{nodeID}/reset` endpoint will be added in a
follow-up phase.

#### Status Display

- No idle status.
- Pulse blue per read: `read 1.5 kB`, or `cursor reset` after a reset call.
- Red on error: `not found`, `permission denied`, `path error`,
  `line buffer exceeded`.

#### Properties Panel

```
┌──────────────────────────────────────────────┐
│  File Read                                    │
├──────────────────────────────────────────────┤
│  Path                                         │
│  [ /var/log/app.log                        ]  │
│  ℹ {{mustache}} supported; msg.filename wins  │
│                                               │
│  Encoding   [ Auto (by extension)        ▼ ]  │
│                                               │
│  ☐ Incremental (only new bytes since last read) │
│  ☐   Start from beginning on first access     │
│  Line delimiter [ \n (LF)                ▼ ]  │
│  Max line size: [ 1048576 ] bytes             │
│                                               │
│  Root jail (optional)  [ /var/log         ]   │
└──────────────────────────────────────────────┘
```

### 2. File Watch Node (`file-watch`)

`file-watch` does one thing: detect filesystem changes and emit
metadata-only messages. **It never reads file content.** To act on the
content, wire a `file-read` after it (typically using `msg.filename`
or `msg.path` from the watch event).

The path can be either a single file or a folder. In `mode=read` it
returns a directory listing (also metadata only).

#### Mode

| Mode | Canvas | Trigger | Output |
|---|---|---|---|
| `read` | 1 input, 1 output | Incoming message; `msg.path` overrides config | One message per entry (folder) or one message for the file's metadata (single-file path) |
| `watch` | 0 inputs, 1 output | fsnotify event | One message per event with entry metadata |
| `read+watch` | 0 inputs, 1 output | fsnotify event (always incremental) | Re-scan on each event; emits only entries whose modTime advanced since the last scan |

#### Configuration

| Field | Type | Default | Description |
|---|---|---|---|
| `mode` | string | `watch` | `read`, `watch`, or `read+watch` |
| `path` | string | — | Absolute path (file or folder) |
| `recursive` | boolean | `false` | For folder paths, include subfolders. **v1 limitation:** only effective in `mode=read`. Watch is non-recursive |
| `glob` | string | `*` | Glob filter applied to entry names (e.g. `*.csv`). Ignored for single-file paths |
| `watchEvents` | string[] | `["create","write","remove","rename"]` | Events to react to (watch / read+watch only) |
| `sendAs` | string | `individual` | `individual` — one message per entry; `array` — all entries in one `msg.payload` array. Ignored for single-file watch |
| `incremental` | boolean | `false` | (folder watch only) Only emit entries whose modTime advanced. Required for read+watch (forced on) |
| `fromStart` | boolean | `false` | When `incremental=true`: emit all existing entries on first access. `false` = silent first access (record baseline only) |
| `debounceMs` | number | `100` | Debounce window for rapid successive events |
| `rootJail` | string | `""` | If non-empty, all resolved paths must stay inside this directory |

#### Entry Object — `msg.payload`

```json
{
  "name": "report_2026-05-10.csv",
  "path": "/data/incoming/report_2026-05-10.csv",
  "size": 4096,
  "modTime": "2026-05-10T08:14:22Z",
  "isDir": false,
  "mode": "0644"
}
```

Notably **no `content`** field — this node never reads file content.
Downstream `file-read` does that job, picking the path up via
`msg.filename` or `msg.path`.

#### Outgoing Message

| Mode | Output shape |
|---|---|
| `mode=read`, `sendAs=individual` | One message per entry; `msg.payload = entry`, `msg.topic = entry.path`, plus `isFirst`/`isLast`/`index`/`total` bookkeeping |
| `mode=read`, `sendAs=array` | One message; `msg.payload = []entry` |
| `mode=watch` | One message per fsnotify event; `msg.payload = entry`, `msg.event` = `create`/`write`/`remove`/`rename`, `msg.topic = entry.path` |
| `mode=read+watch` | Same as `mode=watch` but driven by re-scan diff (always incremental); includes `msg.changed = true` and optional `msg.previousModTime` |

For `remove`/`rename` events where the file no longer exists, `size`,
`modTime`, `mode` may be absent; `name` and `path` are always present.

#### Incremental — Per-File modTime Map

When `incremental=true` (or `mode=read+watch` which forces it on),
`file-watch` maintains a `map[absolutePath]modTime` snapshot in
`flowPers` under `_dircursor.<sha256(nodeID+":"+folderPath)[:16]>`.

**Scan sequence (read / read+watch):**

1. Walk the folder (applying glob, recursive).
2. Load the stored map. If absent → first access.
3. For each entry:
   - **New file** (path not in map) → emit if `fromStart=true`; otherwise record silently.
   - **Changed file** (`modTime` advanced) → emit with `previousModTime`.
   - **Unchanged** → skip.
4. For map entries no longer on disk (deleted/renamed): if `watchEvents`
   includes `"remove"`, emit a deletion message with `event="remove"`.
5. Persist the updated map.

**`watch` mode + `incremental=true`:** each fsnotify event triggers a
`stat` of the affected file and a comparison against the map — avoids
spurious duplicates from the OS firing multiple events per write.

**`watch` mode + `incremental=false`:** approximates "what just changed"
by re-scanning and emitting entries whose modTime falls in the recent
debounce window. Less precise; users wanting exact change detection
should enable incremental.

#### Status Display

- `read` mode: pulse blue per listing.
- `watch` / `read+watch`: green `watching · /path` once registered.
- Red on registration failure, missing path, permission denied.

#### Properties Panel

```
┌──────────────────────────────────────────────┐
│  File Watch                                   │
├──────────────────────────────────────────────┤
│  Mode                                         │
│  ( ) Read   ( • ) Watch   ( ) Read + Watch    │
│                                               │
│  Path  [ /data/incoming                    ]  │
│                                               │
│  Filter (glob)   [ *.csv                   ]  │
│  ☐ Recursive (Read mode only)                 │
│  Send as   ( • ) Individual   ( ) Array       │
│                                               │
│  ☐ Incremental (only changed files)           │
│  ☐   Emit existing files on first access      │
│                                               │
│  Watch events: ☑ Create ☑ Write ☑ Remove ☑ Rename │
│  Debounce: [ 100 ] ms                         │
│                                               │
│  Root jail (optional)  [ /data           ]    │
└──────────────────────────────────────────────┘
```

### 3. File Write Node (`file-out`)

Unchanged from the previous spec revision.

- **Canvas**: 1 input, 1 output (optional pass-through)
- **Function**: writes or appends `msg.payload` to a file. Output port
  forwards the original message after a successful write; if not wired,
  the message is dropped.

#### Configuration

| Field | Type | Default | Description |
|---|---|---|---|
| `path` | string | — | Absolute file path. Supports `{{mustache}}` over `msg` |
| `mode` | string | `overwrite` | `overwrite`, `append`, `create` (fail if file exists) |
| `encoding` | string | `auto` | `auto`, `utf-8`, `binary` |
| `createDirs` | boolean | `false` | Create parent directories if they do not exist |
| `appendNewline` | boolean | `false` | Append a `\n` after each write |
| `rootJail` | string | `""` | If non-empty, all resolved paths must stay inside this directory |

**Encoding — `auto` rules:** string payload → `utf-8`; number array /
`[]byte` payload → raw binary.

**Status:** pulse blue per write (`wrote 1.5 kB`); red on error
(`not found`, `permission denied`, `file exists`, `disk full`,
`write error`, `path error`).

## Security — Path Handling

All three nodes reject non-absolute paths. The optional `rootJail`
ensures resolved paths stay inside a configured directory; symlinks
are resolved via `filepath.EvalSymlinks` (lenient — works for files
that don't exist yet, e.g. file-out create mode). Path traversal
attempts produce a catchable error.

## Error Handling & Catch Integration

All three nodes are good Catch citizens:

| Error | Node | Status | Message |
|---|---|---|---|
| File/folder not found | all | red | `not found` |
| Permission denied | all | red | `permission denied` |
| Path traversal | all | red | `path escapes jail` |
| Path empty / not absolute | all | red | `path error` |
| Payload not writable type | `file-out` | red | `unsupported payload type` |
| `create` mode + file exists | `file-out` | red | `file exists` |
| Disk full / I/O error | `file-out` | red | `disk full` / `write error` |
| Watcher registration failed | `file-watch` | red | `watch error: <os msg>` |
| Cursor / KV unavailable | `file-read`, `file-watch` | red | `no persistent context` |
| Line buffer exceeded | `file-read` | red | `line buffer exceeded` |

## Data Structure — workspace.json examples

```json
{
  "id": "node-fr-1",
  "type": "file-read",
  "name": "Tail app log",
  "x": 200, "y": 150, "z": "flow-1",
  "inputs": 1, "outputs": 1,
  "wires": [["node-split-1"]],
  "config": {
    "path": "/var/log/app.log",
    "encoding": "utf-8",
    "incremental": true,
    "fromStart": false,
    "delimiter": "\n",
    "maxLineBytes": 1048576
  }
}
```

```json
{
  "id": "node-fw-1",
  "type": "file-watch",
  "name": "Drop folder",
  "x": 200, "y": 300, "z": "flow-1",
  "inputs": 0, "outputs": 1,
  "wires": [["node-fr-2"]],
  "config": {
    "mode": "watch",
    "path": "/data/incoming",
    "glob": "*.csv",
    "incremental": true,
    "watchEvents": ["create"],
    "debounceMs": 100
  }
}
```

```json
{
  "id": "node-fo-1",
  "type": "file-out",
  "name": "Append event log",
  "x": 700, "y": 150, "z": "flow-1",
  "inputs": 1, "outputs": 1,
  "wires": [[]],
  "config": {
    "path": "/var/log/loopze/events.log",
    "mode": "append",
    "encoding": "utf-8",
    "appendNewline": true,
    "createDirs": true
  }
}
```

## Technical Sketch

### Backend — `internal/nodes/filesystem/`

```
init.go           — group registration (file-read, file-watch, file-out)
file_read.go      — FileReadNode (one-shot + incremental tail)
file_watch.go     — FileWatchNode (file or folder, events only)
file_out.go       — FileOutNode (write/append/create)
helpers.go        — resolvePath, encodePayload, decodePayload, applyJail, resolveEncoding
cursor.go         — readIncremental, trimToLastLine, cursorKey, dirCursorKey, modTime map
```

```go
// init.go
func init() {
    nodes.RegisterGroup(nodes.Group{
        Name: "filesystem",
        Description: "File and folder read, watch, and write",
        Nodes: []nodes.FlowNodeRegistration{
            {Type: "file-read",  Factory: NewFileReadNode,  Info: FileReadTypeInfo()},
            {Type: "file-watch", Factory: NewFileWatchNode, Info: FileWatchTypeInfo()},
            {Type: "file-out",   Factory: NewFileOutNode,   Info: FileOutTypeInfo()},
        },
    })
}
```

`FileReadNode` and `FileWatchNode` implement `flow.ContextProvider`
(receive `flowPers` for cursor / modTime map persistence). `FileOutNode`
does not need it.

### External Dependency

`github.com/fsnotify/fsnotify` — only used by `FileWatchNode`. MIT-licensed.

### Frontend — `frontend/src/nodes/filesystem/`

```
index.ts                  — group manifest
FileReadConfig.vue        — read + incremental options
FileWatchConfig.vue       — mode, path, glob, sendAs, incremental, watch options
FileOutConfig.vue         — mode, encoding, createDirs, appendNewline
```

```
frontend/src/components/nodes/
FileReadNode.vue          — body shows "incr · /path" or "read · /path"
FileWatchNode.vue         — body shows "<mode> · /path [glob]"
FileOutNode.vue           — body shows "<mode> · /path"
```

## Affected Files

### Backend — New / Renamed

- `internal/nodes/filesystem/init.go` — group registration (new types)
- `internal/nodes/filesystem/file_read.go` (renamed from `file_in.go`,
  watch logic stripped — pure read)
- `internal/nodes/filesystem/file_read_test.go` (renamed; watch tests removed)
- `internal/nodes/filesystem/file_watch.go` (renamed from `folder_in.go`,
  content-reading logic stripped — pure watch + listing)
- `internal/nodes/filesystem/file_watch_test.go` (renamed; content tests removed)
- `internal/nodes/filesystem/file_out.go` — unchanged
- `internal/nodes/filesystem/file_out_test.go` — unchanged
- `internal/nodes/filesystem/helpers.go` — unchanged
- `internal/nodes/filesystem/cursor.go` — unchanged

### Backend — Adjusted

- `cmd/loopze/groups.go` — blank import unchanged (group name stays `filesystem`)
- `go.mod` / `go.sum` — `fsnotify` already present

### Frontend — Renamed

- `frontend/src/nodes/filesystem/FileReadConfig.vue` (was `FileInConfig.vue`)
- `frontend/src/nodes/filesystem/FileWatchConfig.vue` (was `FolderInConfig.vue`)
- `frontend/src/nodes/filesystem/FileOutConfig.vue` — unchanged
- `frontend/src/nodes/filesystem/index.ts` — type IDs updated
- `frontend/src/components/nodes/FileReadNode.vue` (was `FileInNode.vue`)
- `frontend/src/components/nodes/FileWatchNode.vue` (was `FolderInNode.vue`)
- `frontend/src/components/nodes/FileOutNode.vue` — unchanged

### Frontend — Adjusted

- `frontend/src/views/FlowEditor.vue` — slot templates renamed
- `frontend/src/types/flow.ts` — type union updated
- `frontend/src/components/nodes/NodeIcon.vue` — `file-read`, `file-watch` icons
- `frontend/src/components/nodes/BaseNode.vue` — typeLabel map entries updated

## Tests

The existing test corpus is preserved with appropriate renames and
removals. Tests that exercised `file-in mode=read+watch` with content
emission move to a new integration test that wires `file-watch →
file-read` (or are removed if redundant).

| Area | Tests |
|---|---|
| `file-read` | All `TestFileIn_ReadMode_*`, `TestFileIn_Incremental_*` (renamed to `TestFileRead_*`); watch-related tests **removed** |
| `file-watch` | All `TestFolderIn_*` (renamed to `TestFileWatch_*`); `includeContent` tests **removed** |
| `file-out` | Unchanged |
| `helpers` / `cursor` | Unchanged |

## Dependencies

- `github.com/fsnotify/fsnotify` — only used by `file-watch`
- `flow.Message`, `flow.ContextStore`, `flow.ContextProvider` (all exist)
- Catch node for error forwarding (exists)

## Migration / Breaking Change

The branch is unreleased — old `file-in` / `folder-in` type IDs disappear
without aliasing. Existing dev-environment workspaces that reference them
must be updated by hand. The CHANGELOG entry calls this out under
**Breaking changes** since the previous Unreleased text already advertised
the old names.

## Out of Scope (deferred follow-ups)

- **Reset button in Properties Panel + `POST /api/v1/file-read/{nodeID}/reset`
  and `/api/v1/file-watch/{nodeID}/reset` endpoints.** The cursor reset
  in the current iteration is reachable only via `msg.resetCursor=true`.
- **CSV with embedded newlines in quoted fields** — `file-read`'s
  line-aware mode splits on raw `\n`/`\r\n`. Use the dedicated CSV
  Parser node (`PARSER_CSV_NODE.md`) downstream.
- **Recursive folder watching** — `recursive` is honoured in `mode=read`
  only. Watch is non-recursive (no native `inotify` recursion; manual
  subdirectory registration deferred).
- **Streaming reads for large files** — `file-read` reads the entire
  file into memory. A `file-stream-read` node is a follow-up.
- **Inode-change detection for log rotation** — size-based truncation
  detection is sufficient for v1; inode tracking is a v2 candidate.
- **Pure metadata events (`touch`, `chmod`)** — `file-watch incremental`
  emits a "changed" message because modTime advanced; `file-read
  incremental` would see 0 new bytes and emit nothing. Asymmetry
  intrinsic to the two cursor models; accepted as-is.

## Resolved Decisions

- **Three nodes, single-purpose each** — read, watch, write.
- **`file-watch` never reads file content.** Compose with `file-read`.
- **Cursor / modTime map storage**: `flowPers` (flow-scoped persistent KV).
- **`file-watch read+watch`**: always incremental; `incremental` flag
  forced on internally.
- **Recursive watching**: read-mode only in v1.
- **`file-out` output port**: always present, pass-through if wired.
- **Workspace migration**: hard break; the unreleased branch's previous
  type IDs (`file-in`, `folder-in`) are not aliased.

## Open Questions

- **modTime granularity on FAT/exFAT** — 2-second resolution may mask
  rapid successive writes. Acceptable for industrial edge devices using
  ext4/xfs; documented limitation.
- **Cursor scope (`flowPers` vs `globalPers`)** — current choice is
  `flowPers`. Cursor is lost on flow deletion; intentional.
