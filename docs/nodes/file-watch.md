# File Watch (`file-watch`)

Watches a file or directory for filesystem events and emits **metadata
messages** — it never reads file content. Three modes cover the main
use-cases: list on trigger, event-driven watch, and always-incremental
change detection.

Chain a **File Read** node to consume the content of changed files.

| Inputs | Outputs |
|--------|---------|
| 1 *(read mode only)* | 1 |

## Modes

| Mode         | Trigger | Behaviour |
|--------------|---------|-----------|
| `read`       | Incoming message | Lists directory entries that match the glob. `msg.path` overrides the configured path. |
| `watch`      | Filesystem events | Emits an entry for each matching event (create / write / remove / rename). |
| `read+watch` | Filesystem events | On each event: re-scans the directory and emits only entries whose `modTime` advanced. Always-incremental — the incremental toggle is hidden in this mode. |

## Configuration

| Field          | Default                              | Description |
|----------------|--------------------------------------|-------------|
| `mode`         | `watch`                              | `read` \| `watch` \| `read+watch` — see [Modes](#modes). |
| `path`         | *(empty)*                            | Absolute path to the directory (or file). Required for `watch` / `read+watch`. Optional for `read` (falls back to `msg.path`). |
| `glob`         | `*`                                  | Filename filter. Examples: `*.csv`, `data-*.json`, `**/*.log`. |
| `recursive`    | `false`                              | Descend into subdirectories. Read mode only — watch is non-recursive in v1. |
| `sendAs`       | `individual`                         | `individual` (one message per entry) \| `array` (one message with all entries). |
| `incremental`  | `false`                              | Read mode: only emit entries whose `modTime` advanced since the last scan. State is persisted. |
| `fromStart`    | `false`                              | Emit all existing entries on the first access. Off = emit only changes after deploy. |
| `watchEvents`  | `[create, write, remove, rename]`   | Which FS events to listen for. |
| `debounceMs`   | `100`                                | Coalesce rapid events into one emission per file (ms). |
| `rootJail`     | *(empty)*                            | Resolved paths must stay inside this directory. Empty = no restriction. |

## Output message fields

Each emitted message carries one entry in `msg.payload`:

| Field                  | Value |
|------------------------|-------|
| `msg.payload.name`     | Filename (without directory). |
| `msg.payload.path`     | Absolute resolved path. |
| `msg.payload.size`     | File size in bytes. |
| `msg.payload.modTime`  | Last modified timestamp (RFC3339). |
| `msg.payload.isDir`    | `true` for directories. |
| `msg.payload.event`    | *(watch / read+watch only)* `"create"` \| `"write"` \| `"remove"` \| `"rename"`. |

When `sendAs=array`, `msg.payload` is a `[]Entry` slice containing all
matched entries.

## Incremental state

When `incremental=true` (read mode) or `mode=read+watch`, the node
persists a `modTime` map per directory. On each scan it emits only entries
whose `modTime` has advanced. The map is stored in the flow's persistent
context and survives restarts.

`fromStart=false` (default): only entries that change _after_ the first
deploy are emitted. `fromStart=true`: all existing entries are emitted on
the first scan.

## Error handling

| Condition | Status | Routed as |
|-----------|--------|-----------|
| Path not found | `red` / `path not found` | Catchable error |
| Permission denied | `red` / `permission denied` | Catchable error |
| Path outside rootJail | `red` / `path escapes jail` | Catchable error |

## Examples

### Watch for new CSV files, then read them

```
[File Watch: mode=watch, path=/data/incoming, glob=*.csv, events=[create]]
  →  [Change: set msg.filename = msg.payload.path]
  →  [File Read: encoding=utf8, delimiter=\n]
  →  [Function: parseCSV(msg.payload)]
```

File Watch emits the file path; a Change node maps it to `msg.filename`;
File Read loads the content.

### Incremental directory scan on a timer

```
[Inject: every 30s]
  →  [File Watch: mode=read, path=/data/reports, incremental=on, sendAs=individual]
  →  [Function: process each new or changed report]
```

On each trigger only files added or modified since the last run are
forwarded. The state is persisted across redeployments.

### Always-on folder monitoring (read+watch)

```
[File Watch: mode=read+watch, path=/data/feeds, glob=*.json, sendAs=individual]
  →  [Change: set msg.filename = msg.payload.path]
  →  [File Read]
  →  [JSON Parser]
```

Every filesystem event triggers a re-scan; only entries with an updated
`modTime` are emitted. Combines low-latency event detection with accurate
change tracking.

### List a directory on HTTP request

```
[HTTP-In: GET /files]
  →  [Change: set msg.path = "/data"]
  →  [File Watch: mode=read, sendAs=array]
  →  [HTTP-Out]
```

The directory listing is returned as a JSON array in the HTTP response.

## Notes

- **Content**: File Watch emits **metadata only**. `msg.payload.path`
  points to the file — use a File Read node to load the content.
- **Debounce**: some editors (VS Code, vim) write files in multiple chunks.
  The 100 ms debounce collapses the burst into one event per file. Increase
  it for slower storage (NAS, network mounts).
- **v1 watch limitation**: the OS watcher is non-recursive. For recursive
  change detection use `mode=read` with `recursive=true` and `incremental=on`,
  or `mode=read+watch`.
- **rootJail**: set a rootJail in untrusted flows to prevent a compromised
  upstream from redirecting the watch to sensitive directories.

## See also

- **File Read** (`file-read`) — read file content on trigger; supports
  incremental byte-cursor tailing.
- **File Out** (`file-out`) — write message payloads to files.
