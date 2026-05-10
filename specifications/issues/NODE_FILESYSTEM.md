# Issue: Filesystem Nodes — File Read/Watch, Folder Read/Watch, File Write

## Status: Proposed

## Problem Description

Flows in industrial and edge contexts regularly need to interact with the local
filesystem: reading configuration files on startup, watching drop folders for
incoming reports from SCADA systems or PLCs, writing log or archive CSV files,
and reacting to file changes without polling. Today this requires a Function
node with `fs` access — no UI, no error handling, fragile code.

Three new node types are introduced, each covering read *and* watch via a
`mode` config:

- `file-in` — **File In** — reads a file on trigger (`mode=read`), watches
  it for changes (`mode=watch`), or watches and emits the new content on
  every change (`mode=read+watch`). Optionally reads only **new bytes since
  the last read** (incremental / tail mode) with a persistent cursor.
- `folder-in` — **Folder In** — lists a folder on trigger (`mode=read`),
  watches it for filesystem events (`mode=watch`), or does both
  (`mode=read+watch`).
- `file-out` — **File Write** — writes or appends `msg.payload` to a file.

## Overview

| Node Type | Type ID | Canvas In | Canvas Out | Description |
|---|---|---|---|---|
| **File In** | `file-in` | 0 or 1 ¹ | 1 | Read file content and/or watch for changes |
| **Folder In** | `folder-in` | 0 or 1 ¹ | 1 | List folder entries and/or watch for changes |
| **File Write** | `file-out` | 1 | 1 ² | Write or append payload to a file |

¹ `mode=read` → 1 input; `mode=watch` and `mode=read+watch` → 0 inputs  
² Optional pass-through output; emits the unchanged `msg` after the write completes

### Typical Flow Shapes

**Read a config file on startup:**

```
[Inject once] → [file-in  mode=read  /etc/app/config.json] → [JSON Parser] → [Change]
```

**React to a SCADA report drop folder:**

```
[folder-in  mode=watch  /data/incoming  *.csv] → [file-in  mode=read] → [CSV Parser]
```

**Watch file for changes and emit new content:**

```
[file-in  mode=read+watch  /var/run/status.json] → [JSON Parser] → [Switch]
```

**Tail a growing log file and emit only new lines:**

```
[file-in  mode=read+watch  incremental=true  /var/log/app.log] → [Split: \n] → [Switch]
```

**Append a log line on each event:**

```
[MQTT-in] → [Change: build log line] → [file-out  append  /var/log/mqtt.log]
```

## Requirements

### 1. File In Node (`file-in`)

#### Mode

| Mode | Canvas | Trigger | Output |
|---|---|---|---|
| `read` | 1 input, 1 output | Incoming message; `msg.filename` overrides path config | File content as `msg.payload` |
| `watch` | 0 inputs, 1 output | fsnotify event | Event metadata only (no file content read) |
| `read+watch` | 0 inputs, 1 output | fsnotify event | Reads file on each event, emits content + event info |

#### Configuration

| Field | Type | Default | Description |
|---|---|---|---|
| `mode` | string | `read` | `read`, `watch`, or `read+watch` |
| `path` | string | — | Absolute path to the file. Supports `{{mustache}}` over `msg` in `read` mode |
| `encoding` | string | `auto` | `auto` (by extension), `utf-8`, `binary` |
| `watchEvents` | string[] | `["write","create"]` | Events that trigger output: `create`, `write`, `remove`, `rename`. Only for `watch`/`read+watch` |
| `debounceMs` | number | `50` | Debounce window for rapid successive events. `0` = no debounce |
| `incremental` | boolean | `false` | Only read bytes appended since the last read. Cursor is stored persistently |
| `fromStart` | boolean | `false` | When `incremental=true`: start from byte 0 on first access. `false` = start from current EOF (skip existing content) |
| `delimiter` | string | `\n` ¹ | Line/record delimiter. `\n`, `\r\n`, `auto` (accept either), or `none` (raw bytes, no line-awareness). Only relevant when `incremental=true` |
| `maxLineBytes` | number | `1048576` | Safety cap for buffered partial lines (1 MiB default). If no delimiter is found within this many buffered bytes, the node raises a catchable error and stops advancing the cursor. Only relevant when `incremental=true` and `delimiter != "none"` |

¹ `delimiter` defaults to `\n` when `incremental=true` and the resolved
encoding is `utf-8`; defaults to `none` when encoding is `binary`. Override
via the config field for explicit control.

**Encoding — `auto` rules:**

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
  "event": "write",
  "position": 4096,
  "bytesRead": 512,
  "lineCount": 12
}
```

- `event` is only present in `watch` and `read+watch` modes.
- In `watch` mode, `payload`, `encoding`, and `size` are **absent** (no file read).
- `position` and `bytesRead` are only present when `incremental=true`.
  `position` is the byte offset after this read (i.e. the next cursor position).
- `lineCount` is only present when `incremental=true` and `delimiter != "none"`
  — counts complete delimited records in `payload`.
- When `incremental=true` and no new bytes (or only a partial line) are
  available, the node emits **no message** and stays silent.
- Existing `msg` properties are preserved in `read` mode (pass-through semantics).

#### Status Display

- `read` mode: no idle status. Pulse blue on each read.
- `watch`/`read+watch`: green `watching · /path/to/file` once the watcher is
  registered. Red on registration failure or missing path.

#### Incremental Read — Persistent Cursor

When `incremental=true`, the node maintains a **byte offset cursor** per file
path and stores it persistently so that it survives redeploys and restarts.

**Cursor storage:** stored in the node's persistent flow context
(`flowPers`) under the key `_filecursor.<sha256(nodeID+":"+absolutePath)[:16]>`.
The SHA-prefix keeps the key short and avoids characters that are illegal in
NATS KV keys (`:`, `/`). The node implements `flow.ContextProvider` and
receives its context stores via `SetContext()`.

**Read sequence on each trigger (read/read+watch mode):**

1. `stat(path)` — get current file size.
2. Load cursor from `flowPers`. If absent → cursor = 0 if `fromStart=true`,
   else cursor = current file size (skip existing content).
3. If cursor > file size → file was **truncated or rotated** (see below).
4. If cursor == file size → no new bytes; emit nothing.
5. Open file, seek to cursor, read to EOF into buffer.
6. **If `delimiter != "none"`**: trim the buffer to the last complete line
   (see "Line-aware reads" below). The trailing partial line stays unread;
   the cursor advances only by `len(trimmedBuffer)`.
7. **If `delimiter == "none"`**: emit the entire buffer; cursor advances
   by full read length.
8. Encode trimmed bytes per `encoding` setting.
9. Emit message with `payload`, `position` (new offset), `bytesRead`,
   optionally `lineCount` (when `delimiter != "none"`).
10. Persist new cursor to `flowPers`.

**Line-aware reads — partial line handling:**

Logs and CSV files are written line by line. A read that races with an
in-progress write may capture a partial trailing line (no terminating
delimiter yet). Advancing the cursor past the partial line would lose data
on the next read.

The line-aware read trims the buffer to the **last byte of the last
complete delimiter**:

| Delimiter | Trim rule |
|---|---|
| `\n` | `i = lastIndexOf(buf, '\n')`. If found: emit `buf[0..i+1]`, advance cursor by `i+1`. If not found: emit nothing, cursor stays. |
| `\r\n` | `i = lastIndexOf(buf, '\r\n')`. If found: emit `buf[0..i+2]`, advance cursor by `i+2`. If not found: emit nothing. |
| `auto` | `i = max(lastIndexOf(buf, '\n'), lastIndexOf(buf, '\r\n')+1)`. The `\n` index is used (CRLF ends in LF too); cursor advances to one past the `\n`. |
| `none` | No trimming. Entire buffer emitted. |

**Example:**

```
File grows from "alice\nbob\n" to "alice\nbob\ncarol\nda"  (write in progress)
Cursor before read: 10  (after "alice\nbob\n")
Buffer read:        "carol\nda"   (8 bytes)
Last \n at index:   5
Emitted payload:    "carol\n"     (6 bytes)
Cursor after:       16
Next read picks up: "da..." plus whatever was appended after
```

**maxLineBytes safeguard:**

If the buffer reaches `maxLineBytes` and contains no delimiter, the file
is either misconfigured (binary file with delimiter set) or producing
abnormally long lines. The node:
- Sets status red `"file-in: line buffer exceeded maxLineBytes"`.
- Raises a catchable error with the same message.
- Does **not** advance the cursor (re-tries on next trigger).

This prevents unbounded memory growth and makes misconfiguration loud.

**Watch + incremental interaction:**

In `watch+incremental` mode, fsnotify events may fire mid-write. The
debounce timer (`debounceMs`) absorbs the most common burst pattern
(write → flush → close), but the line-aware trim is the actual
correctness guarantee — debounce alone cannot detect "write finished".
Recommendation: leave `debounceMs` at the default `50`; line-trim handles
the rest.

**File truncation / rotation:**

| Situation | Detection | Behaviour |
|---|---|---|
| File shrunk (log rotation, truncate) | `cursor > stat.Size` | Reset cursor to 0, read from start, set `msg.reset = true` |
| File replaced (same path, new inode) | Cannot detect inode change cross-platform in v1 | Treated identically to truncation (cursor > size) |

`msg.reset = true` signals downstream nodes that a continuity break occurred.
A downstream Switch can route reset messages separately (e.g., to clear state).

**Cursor reset:**

Three reset paths are supported:

1. **`msg.resetCursor = true`** in `read` mode — resets the stored cursor
   to 0 (or to current EOF if `fromStart=false`) without emitting content.
2. **Reset button in the Properties Panel** — works in all modes; calls a
   dedicated API endpoint that clears the persisted cursor for this node.
   Useful for `watch`/`read+watch` source nodes which have no input port.
3. **Redeploy does NOT reset** — persistence is intentional. To reset,
   use option 1 or 2.

**API endpoint:** `POST /api/v1/file-in/{nodeID}/reset` clears the
persisted cursor for the given node. Returns `204 No Content` on success,
`404` if the node does not exist or is not a `file-in`.

**Cursor inspection:** `msg.position` on every incremental output contains the
new cursor value, allowing downstream nodes or a Debug node to observe drift.

#### Properties Panel

```
┌──────────────────────────────────────────────┐
│  File In                                      │
├──────────────────────────────────────────────┤
│  Mode                                         │
│  ( • ) Read   ( ) Watch   ( ) Read + Watch    │
│                                               │
│  Path                                         │
│  [ /var/run/status.json                    ]  │
│  ℹ {{mustache}} supported in Read mode        │
│                                               │
│  Encoding                                     │
│  [ Auto (by extension)                   ▼ ]  │
│                                               │
│  ☐ Incremental (only new bytes since last read) │
│  ☐   Start from beginning on first access     │
│  Line delimiter [ \n (LF)                ▼ ]  │
│  ℹ Partial trailing lines stay buffered       │
│  Max line size: [ 1048576 ] bytes             │
│  [ Reset cursor ]                             │
│                                               │
│  ▼ Watch options (Watch / Read+Watch only)    │
│  Events:  ☑ Write  ☑ Create  ☐ Remove  ☐ Rename │
│  Debounce: [ 50 ] ms                          │
└──────────────────────────────────────────────┘
```

### 2. Folder In Node (`folder-in`)

#### Mode

| Mode | Canvas | Trigger | Output |
|---|---|---|---|
| `read` | 1 input, 1 output | Incoming message; `msg.path` overrides folder config | One message per entry (or array — see `sendAs`) |
| `watch` | 0 inputs, 1 output | fsnotify event inside the folder | One message per event with entry metadata |
| `read+watch` | 0 inputs, 1 output | fsnotify event | Re-scans folder on each event; emits only entries whose modTime advanced since the last scan (always behaves as if `incremental=true`) |

#### Configuration

| Field | Type | Default | Description |
|---|---|---|---|
| `mode` | string | `read` | `read`, `watch`, or `read+watch` |
| `path` | string | — | Absolute path to the folder |
| `recursive` | boolean | `false` | Include subfolders recursively. **v1 limitation:** only effective in `mode=read`. In `watch` and `read+watch` modes the watcher is non-recursive and only fires on direct children of `path` — fsnotify does not support native recursive watch and manual subdirectory registration is deferred to a follow-up |
| `glob` | string | `*` | Glob filter applied to entry names (e.g. `*.csv`) |
| `watchEvents` | string[] | `["create","write","remove","rename"]` | Events to react to (watch/read+watch only) |
| `sendAs` | string | `individual` | `individual` — one message per entry; `array` — all entries in one `msg.payload` array |
| `includeContent` | boolean | `false` | Read each file's content and include as `entry.content` (`read`/`read+watch` only; ignored for dirs) |
| `contentEncoding` | string | `auto` | Encoding for `includeContent`: `auto`, `utf-8`, `binary` |
| `maxFileSizeBytes` | number | `1048576` | Skip content read for files larger than this (in `includeContent` mode). Entry still emitted, `content` absent, `contentSkipped: true` set |
| `debounceMs` | number | `100` | Debounce for watch events |
| `incremental` | boolean | `false` | Only emit entries whose `modTime` has changed since the last read. The last-seen modTime per file is stored persistently |
| `fromStart` | boolean | `false` | When `incremental=true`: on first access emit all existing entries and record their modTimes. `false` = silently record all current modTimes on first access without emitting anything |

#### Entry Object (per message / per array element)

```json
{
  "name": "report_2026-05-10.csv",
  "path": "/data/incoming/report_2026-05-10.csv",
  "size": 4096,
  "modTime": "2026-05-10T08:14:22Z",
  "isDir": false,
  "mode": "0644",
  "content": "<string or number array — only if includeContent=true>"
}
```

- `size` is `0` for directories.
- `mode` is the Unix permission string (e.g. `0644`, `0755`).

#### Outgoing Message (mode=read, sendAs=individual)

```json
{
  "payload": { "<entry object above>" },
  "topic": "/data/incoming/report_2026-05-10.csv",
  "isFirst": true,
  "isLast": false,
  "index": 0,
  "total": 12
}
```

- `isFirst` / `isLast` / `index` / `total` allow downstream nodes to detect the end of a listing.
- For `sendAs=array`: one message with `msg.payload = [<entry>, ...]`; `isFirst`/`isLast`/`index` absent.

#### Outgoing Message (mode=watch)

```json
{
  "payload": { "<entry object>" },
  "event": "create",
  "topic": "/data/incoming/report.csv"
}
```

- For `remove`/`rename` events where the file no longer exists, `size`, `modTime`, `mode` may be absent; `name` and `path` are always present.

#### Incremental Read — Persistent modTime Map

When `incremental=true`, the node maintains a **per-file modTime map** and
stores it persistently so it survives redeploys and restarts.

**State stored:** `map[absolutePath]modTimeRFC3339` — one entry per file path
seen since the last reset.

**Storage key:** `_dircursor.<sha256(nodeID+":"+folderPath)[:16]>` in
`flowPers` (same NATS KV mechanism as `file-in`). The node implements
`flow.ContextProvider`.

**Scan sequence on each trigger (read / read+watch mode):**

1. Walk the folder (applying `glob`, `recursive` config).
2. Load the stored modTime map from `flowPers`. If absent → map is empty
   (first access).
3. For each entry:
   - **New file** (path not in map) → emit if `fromStart=true`; otherwise
     record modTime silently and skip.
   - **Changed file** (`entry.modTime` > stored modTime) → emit.
   - **Unchanged file** (`entry.modTime` == stored modTime) → skip.
4. For entries that were in the stored map but no longer exist on disk
   (deleted / renamed away): if `watchEvents` includes `"remove"`, emit a
   deletion message with `event="remove"` and `modTime` absent.
5. After all entries processed: write the updated map (current modTimes for
   all existing entries, deleted entries removed) back to `flowPers`.

**watch mode + incremental:**

On each fsnotify event the node stats the affected file and checks its modTime
against the stored map entry. Emits only if new or changed, then updates the
map. This avoids spurious duplicates when the OS fires multiple events for a
single write (e.g., attribute update followed by content update).

**First access behaviour (`fromStart`):**

| `fromStart` | First scan result |
|---|---|
| `false` (default) | All existing entries recorded in map; nothing emitted. Only files added or changed *after* this point are emitted. |
| `true` | All existing entries emitted immediately; map populated with their current modTimes. |

**Cursor reset:**

- In `read` mode: `msg.resetCursor = true` clears the stored map and returns
  without emitting. Next trigger re-initialises per `fromStart` setting.
- In `watch`/`read+watch` mode: use the **Reset button in the Properties
  Panel** (calls `POST /api/v1/folder-in/{nodeID}/reset`).
- Redeploy does **not** clear the map (persistence is intentional).

**Outgoing message additions when `incremental=true`:**

```json
{
  "payload": { "<entry object>" },
  "topic": "/data/incoming/report.csv",
  "changed": true,
  "previousModTime": "2026-05-10T07:00:00Z"
}
```

- `changed: true` is always set on incremental output (distinguishes from
  non-incremental mode in downstream Switch nodes).
- `previousModTime` is set for **changed** files (not for new files where
  no prior modTime exists).
- For deletion events: `changed: true`, `event: "remove"`, `previousModTime`
  contains the last recorded modTime.

#### Status Display

- `read` mode: pulse blue per listing.
- `watch`/`read+watch`: green `watching · /path/to/folder`.
- Red on registration failure, non-existent path, or permission denied.

#### Properties Panel

```
┌──────────────────────────────────────────────┐
│  Folder In                                    │
├──────────────────────────────────────────────┤
│  Mode                                         │
│  ( • ) Read   ( ) Watch   ( ) Read + Watch    │
│                                               │
│  Folder path                                  │
│  [ /data/incoming                          ]  │
│                                               │
│  Filter (glob)   [ *.csv                   ]  │
│  ☐ Recursive                                  │
│                                               │
│  Send as   ( • ) Individual   ( ) Array       │
│                                               │
│  ☐ Include file content                       │
│  Encoding  [ Auto (by extension)         ▼ ]  │
│  Skip files larger than [ 1048576 ] bytes     │
│                                               │
│  ☐ Incremental (only changed files since last read) │
│  ☐   Emit all existing files on first access  │
│  [ Reset cursor ]                             │
│                                               │
│  ▼ Watch options                              │
│  Events: ☑ Create ☑ Write ☑ Remove ☑ Rename  │
│  Debounce: [ 100 ] ms                         │
└──────────────────────────────────────────────┘
```

**Note on `mode=read+watch`:** Always behaves incrementally. Each fsnotify
event triggers a folder re-scan, but only entries whose modTime advanced
since the last scan are emitted. The `incremental` checkbox is ignored
(implicitly always on) and shown disabled in the UI for this mode.

### 3. File Write Node (`file-out`)

- **Canvas**: 1 input, 1 output (optional pass-through)
- **Function**: Writes or appends `msg.payload` to a file. The output port is
  optional — wiring it passes the original `msg` through after the write
  completes. If not wired, the message is discarded.

#### Configuration

| Field | Type | Default | Description |
|---|---|---|---|
| `path` | string | — | Absolute file path. Supports `{{mustache}}` over `msg` |
| `mode` | string | `overwrite` | `overwrite`, `append`, `create` (fail if file exists) |
| `encoding` | string | `auto` | `auto`, `utf-8`, `binary` |
| `createDirs` | boolean | `false` | Create parent directories if they do not exist |
| `appendNewline` | boolean | `false` | Append a `\n` after each write (useful for line-based logs in `append` mode) |

**Encoding — `auto` rules:** string payload → `utf-8`; number array / `[]byte`
payload → raw binary.

#### Message Inputs (override config)

- `msg.filename` — overrides `path` config (same precedence as `file-in`).
- `msg.payload` — the data to write (required).

#### Status Display

- No idle status.
- Pulse blue on each successful write.
- Red on error (permission denied, disk full, `create` mode + file exists, invalid path).

#### Properties Panel

```
┌──────────────────────────────────────────────┐
│  File Write                                   │
├──────────────────────────────────────────────┤
│  Path                                         │
│  [ /var/log/events.log                     ]  │
│  ℹ {{mustache}} supported; msg.filename wins  │
│                                               │
│  Mode                                         │
│  ( ) Overwrite  ( • ) Append  ( ) Create only │
│                                               │
│  Encoding   [ Auto                       ▼ ]  │
│  ☐ Append newline after each write            │
│  ☐ Create parent directories if missing       │
└──────────────────────────────────────────────┘
```

## Security — Path Handling

All three nodes reject paths that are not absolute (must start with `/`). An
optional per-node **root jail** can be configured:

| Field | Type | Default | Description |
|---|---|---|---|
| `rootJail` | string | `""` | If non-empty, all resolved paths must stay inside this directory. Path traversal (symlinks included) outside the jail produces a catchable error |

The jail is enforced after `{{mustache}}` resolution and symlink expansion via
`filepath.EvalSymlinks`. A `..`-based traversal attempt results in a
`"filesystem: path escapes jail"` error. When `rootJail` is empty no
restriction is applied.

## Error Handling & Catch Integration

All three nodes are Catch citizens:

| Error | Node | Status | Message |
|---|---|---|---|
| File/folder not found | `file-in`, `folder-in`, `file-out` | red | `"filesystem: not found"` |
| Permission denied | all | red | `"filesystem: permission denied"` |
| Path traversal / jail violation | all | red | `"filesystem: path escapes jail"` |
| Payload not writable type | `file-out` | red | `"filesystem: unsupported payload type"` |
| `create` mode — file exists | `file-out` | red | `"filesystem: file already exists"` |
| Disk full / I/O error | `file-out` | red | `"filesystem: write error: <os message>"` |
| Watcher registration failed | `file-in`, `folder-in` | red | `"filesystem: watch error: <os message>"` |
| File too large for content read | `folder-in` | — | Entry emitted with `contentSkipped: true`, no error event |

All errors carry `_error.source.{id, type, name, flowId}` per the Catch
contract.

## Data Structure / workspace.json

```json
{
  "id": "node-file-in-1",
  "type": "file-in",
  "name": "Watch status file",
  "x": 200, "y": 150, "z": "flow-1",
  "inputs": 0, "outputs": 1,
  "wires": [["node-json-1"]],
  "config": {
    "mode": "read+watch",
    "path": "/var/run/status.json",
    "encoding": "utf-8",
    "watchEvents": ["write", "create"],
    "debounceMs": 50
  }
}
```

```json
{
  "id": "node-folder-in-1",
  "type": "folder-in",
  "name": "Incoming reports",
  "x": 200, "y": 300, "z": "flow-1",
  "inputs": 0, "outputs": 1,
  "wires": [["node-file-in-2"]],
  "config": {
    "mode": "watch",
    "path": "/data/incoming",
    "glob": "*.csv",
    "recursive": false,
    "sendAs": "individual",
    "watchEvents": ["create"],
    "debounceMs": 100
  }
}
```

```json
{
  "id": "node-file-out-1",
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

### Backend — new package `internal/nodes/filesystem/`

```go
// internal/nodes/filesystem/init.go
func init() {
    nodes.RegisterGroup(nodes.Group{
        Name:        "filesystem",
        Description: "File and folder read, watch, and write",
        Nodes: []nodes.FlowNodeRegistration{
            {Type: "file-in",    Factory: NewFileInNode,    Info: FileInTypeInfo()},
            {Type: "folder-in",  Factory: NewFolderInNode,  Info: FolderInTypeInfo()},
            {Type: "file-out",   Factory: NewFileOutNode,   Info: FileOutTypeInfo()},
        },
    })
}
```

**FileInNode** — `file_in.go`

```go
type FileInNode struct {
    nodes.BaseNode
    config   flow.NodeConfig
    mode        string   // "read" | "watch" | "read+watch"
    path        string
    encoding    string
    watchEvents []string
    debounceMs  int
    rootJail    string
    incremental  bool
    fromStart    bool
    delimiter    string  // "\n" | "\r\n" | "auto" | "none"
    maxLineBytes int64

    watcher  *fsnotify.Watcher  // nil in read-only mode
    flowPers flow.ContextStore  // for cursor persistence
}

// SetContext implements flow.ContextProvider.
func (n *FileInNode) SetContext(_, _, _, flowPers flow.ContextStore) {
    n.flowPers = flowPers
}
```

- `Start()`: in watch/read+watch mode, creates `fsnotify.Watcher`, adds the
  configured path, launches a goroutine reading `watcher.Events` and
  `watcher.Errors`.
- `Stop()`: closes the watcher; drain goroutine exits cleanly.
- `HandleMessage()`: relevant in `read` mode — resolves `{{mustache}}` path
  from `msg`, reads file (incrementally if configured), sends output.
  If `msg.resetCursor == true`, resets cursor and returns without emitting.
- Debounce: a per-path `time.Timer` is reset on each incoming event within the
  window; the message is only sent when the timer fires.
- **Concurrency**: a per-node `sync.Mutex` serialises read paths
  (`HandleMessage` and watcher-triggered reads). If a watcher event arrives
  while a previous read is in progress, the event is coalesced — at most
  one pending event is queued per node. This keeps cursor advancement
  monotonic without complex queueing.

**Incremental read helper** — `cursor.go`

```go
// cursorKey returns the flowPers key for a given nodeID+path pair.
func cursorKey(nodeID, absPath string) string {
    h := sha256.Sum256([]byte(nodeID + ":" + absPath))
    return "_filecursor." + hex.EncodeToString(h[:8])
}

// readIncremental seeks to the stored cursor, reads new bytes, applies
// line-aware trim if delimiter != "none", updates cursor.
// Returns (data, newOffset, lineCount, reset, err).
//   - reset=true when file was truncated (cursor > file size)
//   - lineCount counts complete records when delimiter != "none"; 0 otherwise
//   - data may be empty (no new bytes, or only a partial trailing line)
func readIncremental(ctx context.Context, store flow.ContextStore, nodeID, absPath string, opts IncrementalOpts) ([]byte, int64, int, bool, error)

type IncrementalOpts struct {
    FromStart    bool
    Delimiter    string  // "\n" | "\r\n" | "auto" | "none"
    MaxLineBytes int64
}

// trimToLastLine returns the prefix of buf ending at the last complete delimiter,
// plus the count of complete lines. Returns (nil, 0) if no delimiter is present.
func trimToLastLine(buf []byte, delimiter string) ([]byte, int)
```

**FolderInNode** — `folder_in.go`

```go
type FolderInNode struct {
    nodes.BaseNode
    config   flow.NodeConfig
    mode        string
    path        string
    recursive   bool
    glob        string
    watchEvents []string
    sendAs      string
    includeContent   bool
    contentEncoding  string
    maxFileSizeBytes int64
    debounceMs  int
    rootJail    string
    incremental bool
    fromStart   bool

    watcher  *fsnotify.Watcher
    flowPers flow.ContextStore

    resetPending atomic.Bool  // set true when msg.resetCursor received
}

func (n *FolderInNode) SetContext(_, _, _, flowPers flow.ContextStore) {
    n.flowPers = flowPers
}
```

- `read` trigger: calls `os.ReadDir` (or `filepath.WalkDir` for recursive),
  applies glob filter. When `incremental=true`, loads the modTime map, filters
  to changed/new entries only, emits, then persists the updated map.
- `watch` trigger: `fsnotify.Watcher` in non-recursive mode; for recursive
  watching, registers all subdirectory paths on start and re-registers
  newly-created subdirectories as they appear. When `incremental=true`, stats
  the affected file and checks against the stored map before emitting.
- `includeContent`: per entry, reads file with encoding logic identical to
  `FileInNode`.

**DirCursor helper** — reuses `cursor.go`:

```go
// DirModTimeMap is the persisted state for folder-in incremental mode.
type DirModTimeMap map[string]time.Time  // absolutePath → modTime

func loadDirCursor(store flow.ContextStore, nodeID, folderPath string) (DirModTimeMap, error)
func saveDirCursor(store flow.ContextStore, nodeID, folderPath string, m DirModTimeMap) error
func dirCursorKey(nodeID, folderPath string) string  // same SHA-prefix scheme as cursorKey
```

**FileOutNode** — `file_out.go`

- `Receive()`: resolves path (mustache + `msg.filename` override + jail
  check), opens file with appropriate `os.OpenFile` flags per mode, encodes
  payload, writes, optionally appends `\n`, closes.
- `createDirs`: `os.MkdirAll(filepath.Dir(resolved), 0755)` before open.

**Shared helpers** — `helpers.go`

```go
func resolvePath(tmpl, rootJail string, msg flow.Message) (string, error)
func encodePayload(payload any, encoding string) ([]byte, error)
func resolveEncoding(path, cfgEncoding string) string
func applyJail(resolved, jail string) error
```

### External Dependency

**`github.com/fsnotify/fsnotify`** — cross-platform filesystem watcher.
MIT-licensed, zero transitive dependencies, already the de-facto standard in
the Go ecosystem. Uses `inotify` on Linux (exact fit for edge hardware),
`kqueue` on macOS, `ReadDirectoryChangesW` on Windows.

Alternative: `os.ReadDir` in a polling loop. Rejected — polling burns CPU,
introduces latency, and is fragile on slow storage. `fsnotify` is the correct
tool.

### Frontend — new group `frontend/src/nodes/filesystem/`

```ts
// frontend/src/nodes/filesystem/index.ts
export const manifest: NodeGroupManifest = {
  name: 'filesystem',
  categories: {
    'file-in':   'filesystem',
    'folder-in': 'filesystem',
    'file-out':  'filesystem',
  },
  flowEditors: {
    'file-in':   () => import('./FileInConfig.vue'),
    'folder-in': () => import('./FolderInConfig.vue'),
    'file-out':  () => import('./FileOutConfig.vue'),
  },
}
```

**Canvas node body display:**

| Node | Body text |
|---|---|
| `file-in` (read) | `read · /path/to/file` |
| `file-in` (watch) | `watch · /path/to/file` |
| `file-in` (read+watch) | `read+watch · /path/to/file` |
| `folder-in` | `<mode> · /path/to/folder [*.glob]` |
| `file-out` | `append · /path/to/file` (mode prefix varies) |

## Affected Files

### Backend — New

- `internal/nodes/filesystem/init.go` — group registration
- `internal/server/filesystem_routes.go` — `POST /api/v1/file-in/{id}/reset` and `POST /api/v1/folder-in/{id}/reset` endpoints; resolve node from active flow engine, clear persisted cursor key in `flowPers`
- `internal/nodes/filesystem/file_in.go` — FileInNode
- `internal/nodes/filesystem/file_in_test.go`
- `internal/nodes/filesystem/folder_in.go` — FolderInNode
- `internal/nodes/filesystem/folder_in_test.go`
- `internal/nodes/filesystem/file_out.go` — FileOutNode
- `internal/nodes/filesystem/file_out_test.go`
- `internal/nodes/filesystem/helpers.go` — shared path/encoding utilities
- `internal/nodes/filesystem/helpers_test.go`
- `internal/nodes/filesystem/cursor.go` — incremental read helper (`cursorKey`, `readIncremental`)
- `internal/nodes/filesystem/cursor_test.go`

### Backend — Adjusted

- `internal/server/server.go` — import `_ "github.com/loopzedev/loopze-edge/internal/nodes/filesystem"` (side-effect import triggers `init()`)
- `go.mod` / `go.sum` — add `github.com/fsnotify/fsnotify`

### Frontend — New

- `frontend/src/nodes/filesystem/index.ts` — group manifest
- `frontend/src/nodes/filesystem/FileInConfig.vue` — mode, path, encoding, watch options
- `frontend/src/nodes/filesystem/FolderInConfig.vue` — mode, path, glob, recursive, sendAs, content, watch options
- `frontend/src/nodes/filesystem/FileOutConfig.vue` — path, mode, encoding, newline, createDirs

### Frontend — Adjusted

- `frontend/src/nodes/index.ts` — register `filesystem` manifest
- `frontend/src/components/NodeIcon.vue` — icon entries for `file-in`, `folder-in`, `file-out`

## Tests

| Test | Verifies |
|---|---|
| `TestFileIn_ReadMode_UTF8` | Reads a UTF-8 text file; `msg.payload` is a string |
| `TestFileIn_ReadMode_Binary` | Reads a binary file; `msg.payload` is a number array |
| `TestFileIn_ReadMode_AutoEncoding` | `.json` → utf-8; `.bin` → binary |
| `TestFileIn_ReadMode_MsgFilenameOverride` | `msg.filename` overrides config path |
| `TestFileIn_ReadMode_MustachePath` | `{{payload.name}}` resolved from msg |
| `TestFileIn_ReadMode_NotFound` | Missing file → catchable error |
| `TestFileIn_Incremental_FirstRead_FromEOF` | `fromStart=false`: first read skips existing content, cursor = file size |
| `TestFileIn_Incremental_FirstRead_FromStart` | `fromStart=true`: first read reads full file from byte 0 |
| `TestFileIn_Incremental_SecondRead_NewBytes` | Append bytes to file → second read emits only new bytes; `position` = updated offset |
| `TestFileIn_Incremental_NoNewBytes` | File unchanged → no message emitted |
| `TestFileIn_Incremental_Truncation` | File shrunk below cursor → cursor reset, full re-read, `msg.reset=true` |
| `TestFileIn_Incremental_CursorPersisted` | Cursor stored in flowPers under expected key; survives node restart |
| `TestFileIn_Incremental_ResetMsg` | `msg.resetCursor=true` → cursor reset, no content emitted |
| `TestFileIn_Incremental_Watch_TailGrowingLog` | watch+incremental: each write event emits only the new lines |
| `TestFileIn_Incremental_PartialLine_NotEmitted` | Write without trailing `\n` → no message; cursor stays |
| `TestFileIn_Incremental_PartialLine_CompletedNextRead` | Partial line completed by next write → entire line emitted on next read |
| `TestFileIn_Incremental_LastNewlineWins` | Buffer with mid-buffer `\n` and trailing partial → emits up to last `\n`, leaves partial |
| `TestFileIn_Incremental_DelimiterCRLF` | `\r\n` delimiter: `\r\n` complete lines emitted; lone `\n` or lone `\r` not treated as boundaries |
| `TestFileIn_Incremental_DelimiterAuto` | Mixed `\n` / `\r\n` lines all treated as complete |
| `TestFileIn_Incremental_DelimiterNone` | Raw mode: entire buffer emitted, no line-trim |
| `TestFileIn_Incremental_LineCount` | `msg.lineCount` equals number of complete delimited records |
| `TestFileIn_Incremental_MaxLineBytesExceeded` | Buffer grows past `maxLineBytes` without delimiter → catchable error, cursor stays |
| `TestFileIn_Incremental_DefaultDelimiterByEncoding` | utf-8 + incremental → defaults to `\n`; binary + incremental → defaults to `none` |
| `TestFileIn_WatchMode_EmitsOnWrite` | fsnotify write event → message emitted |
| `TestFileIn_WatchMode_Debounce` | Rapid events → single message after debounce window |
| `TestFileIn_WatchMode_EventFilter` | Only configured events trigger output |
| `TestFileIn_ReadPlusWatch_EmitsContent` | Watch event → file is read; payload is content |
| `TestFileIn_JailViolation` | Path outside rootJail → catchable error |
| `TestFolderIn_Incremental_FirstAccess_FromEOF` | `fromStart=false`: first scan records all modTimes, emits nothing |
| `TestFolderIn_Incremental_FirstAccess_FromStart` | `fromStart=true`: first scan emits all entries |
| `TestFolderIn_Incremental_NewFile` | New file added → emitted on next scan; `changed=true`, no `previousModTime` |
| `TestFolderIn_Incremental_ModifiedFile` | File modTime advanced → emitted; `previousModTime` set |
| `TestFolderIn_Incremental_UnchangedFile` | File modTime unchanged → not emitted |
| `TestFolderIn_Incremental_DeletedFile` | File removed; watchEvents includes remove → deletion message emitted |
| `TestFolderIn_Incremental_DeletedFile_NoRemoveEvent` | File removed; watchEvents excludes remove → not emitted, removed from map |
| `TestFolderIn_Incremental_MapPersisted` | After scan, updated map stored in flowPers under expected key |
| `TestFolderIn_Incremental_MapSurvivesRestart` | Map loaded from flowPers on restart; unchanged files not re-emitted |
| `TestFolderIn_Incremental_ResetMsg` | `msg.resetCursor=true` clears map; next scan behaves as first access |
| `TestFolderIn_Incremental_Watch_OnlyChanged` | fsnotify event + incremental: stat check gates emission |
| `TestFolderIn_Incremental_Recursive` | Nested file change detected when `recursive=true` |
| `TestFolderIn_ReadMode_Individual` | Lists folder; one message per entry with correct metadata |
| `TestFolderIn_ReadMode_Array` | `sendAs=array` → single message with slice payload |
| `TestFolderIn_ReadMode_GlobFilter` | `*.csv` filter — non-matching files excluded |
| `TestFolderIn_ReadMode_Recursive` | Nested dirs included when `recursive=true` |
| `TestFolderIn_ReadMode_IncludeContent` | Entry has `content` field for files below size limit |
| `TestFolderIn_ReadMode_ContentSkipped` | File above `maxFileSizeBytes` → `contentSkipped=true` |
| `TestFolderIn_ReadMode_isFirstIsLast` | `isFirst`, `isLast`, `index`, `total` correct |
| `TestFolderIn_WatchMode_CreateEvent` | New file in folder → message with event=create |
| `TestFolderIn_WatchMode_RemoveEvent` | Deleted file → message with event=remove |
| `TestFolderIn_ReadPlusWatch_ListOnEvent` | fs event triggers full folder listing |
| `TestFileOut_Overwrite` | Payload written; existing content replaced |
| `TestFileOut_Append` | Successive writes accumulate; order preserved |
| `TestFileOut_Create_FailsIfExists` | `create` mode + existing file → catchable error |
| `TestFileOut_AppendNewline` | `appendNewline=true` → `\n` added after each write |
| `TestFileOut_CreateDirs` | Non-existent parent dirs created when `createDirs=true` |
| `TestFileOut_BinaryPayload` | Number array payload → raw bytes on disk |
| `TestFileOut_MsgFilenameOverride` | `msg.filename` overrides config path |
| `TestFileOut_MustachePath` | `{{payload.date}}` resolved from msg |
| `TestFileOut_Passthrough` | Output wired → original msg forwarded after write |
| `TestFileOut_PermissionDenied` | Write to read-only path → catchable error |
| `TestFileOut_JailViolation` | Path traversal → catchable error |
| `TestHelpers_ResolveEncoding` | Auto-encoding resolves by extension |
| `TestHelpers_JailCheck` | Symlink traversal outside jail is caught |

## Dependencies

- **`github.com/fsnotify/fsnotify`** (new) — MIT license, cross-platform
  filesystem watcher. Linux: `inotify`; macOS: `kqueue`; Windows:
  `ReadDirectoryChangesW`.
- `flow.Message` with `Get`/`Set` and `{{mustache}}` template engine
  (exists — reuse from Template node)
- Catch node for error forwarding (exists)

## Out of Scope

- **CSV with embedded newlines in quoted fields** — line-aware incremental
  reads split on raw `\n` / `\r\n` bytes; they are not RFC 4180-aware.
  CSVs that contain newlines inside quoted fields will be split mid-record.
  For correct CSV parsing pair `file-in` (with `delimiter=none` if needed)
  with the dedicated CSV Parser node (`PARSER_CSV_NODE.md`). For typical
  industrial line-per-record CSVs this limitation does not apply.
- **Recursive folder watching** — `recursive` is honoured in `mode=read`
  (full walk) but **not** in `watch`/`read+watch` modes. fsnotify offers no
  native recursive watch on Linux (`inotify`); manual subdirectory
  registration with race-window handling is deferred to a follow-up.
- **Pure metadata events (`touch`, `chmod`)** — `folder-in incremental` will
  emit a "changed" message because modTime advanced. `file-in incremental`
  will see 0 new bytes and emit nothing. This asymmetry is intrinsic to the
  two cursor models and accepted as-is in v1.
- **Streaming reads for large files** — `file-in` reads the entire file into
  memory. A `file-stream-in` node with chunked output is a follow-up.
- **Remote / network filesystems (SMB, NFS, SFTP)** — dedicate separate nodes
  per protocol.
- **Recursive `fsnotify` watch built-in** — `fsnotify` itself is not recursive;
  `folder-in` handles this by walking and registering subdirectories manually
  on start and on `create` events. A future recursive-watch library can replace
  this if the manual approach proves fragile.
- **File rename follow** (watching a path that gets rotated, e.g. log rotation)
  — not addressed in v1; re-deploy clears and re-registers the watcher.
- **Atomic write detection** — many editors write via temp file + rename. The
  `rename` event is surfaced as-is; debouncing helps absorb the typical
  create→rename→write sequence.
- **Permissions management** — `file-out` writes with the process umask; no
  UI for setting explicit file permissions.
- **ZIP / TAR archive traversal** — use a Function node.
- **flow./global. context as path source** — deferred; `msg` and static config
  cover all known use cases.

## Resolved Decisions

- **Cursor storage scope**: `flowPers` (flow-scoped persistent KV). Cursor is
  lost when the flow is deleted; this is intentional.
- **Cursor reset for source-mode nodes**: dedicated reset button in the
  Properties Panel, backed by `POST /api/v1/{type}/{id}/reset`. Works in all
  modes; `msg.resetCursor=true` additionally works in `read` mode.
- **`folder-in` read+watch behaviour**: changed-only — each event triggers a
  scan, but only entries with advanced modTime are emitted. Full-listing
  mode is not offered.
- **Recursive watching**: `recursive=true` is honoured for `mode=read` only.
  `watch` and `read+watch` are non-recursive in v1.
- **`file-out` output port**: always present on canvas; pass-through happens
  if wired, otherwise the message is dropped after write completes.

## Open Questions

- **Incremental cursor key collision**: two `file-in` nodes watching the same
  file on the same flow will have different node IDs, so different cursor keys
  — each tracks independently. This is intentional. Document this behaviour
  in the node help tooltip.
- **modTime granularity on FAT/exFAT**: FAT filesystems have 2-second modTime
  granularity. Files written within the same 2-second window may appear
  unchanged to the incremental scan. This is acceptable for industrial edge
  devices which use ext4/xfs. Document as a known limitation.
- **Inode-change detection for log rotation**: on Linux we could open the file
  by path, stat for inode, compare to stored inode, and detect rotation even
  when the file size grows (new file, same path). This avoids false resets.
  Deferred to v2 — requires storing the inode alongside the cursor and adds
  platform-specific code. For v1, size-based truncation detection is
  sufficient for the common use case.
