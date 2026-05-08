# TCP Send (`tcp-out`)

Writes `msg.payload` to a TCP peer. The mode selector picks where
the destination comes from: a `msg.session` handle attached upstream
by [`tcp-in` server](tcp-in.md), every active session of a sibling
`tcp-in` (broadcast), or a fresh dial to a remote.

| Inputs | Outputs |
|--------|---------|
| 1      | 0       |

## Modes

| Mode               | Destination source                                                                  |
|--------------------|-------------------------------------------------------------------------------------|
| `reply`            | `msg.session` (set by an upstream `tcp-in` server).                                 |
| `server-broadcast` | All currently-connected sessions whose owner matches the configured `targetTcpIn`.  |
| `client`           | Dialed to `host:port` (or `msg.host` / `msg.port`).                                 |

## Configuration

### Common

| Field             | Default | Description                                                                  |
|-------------------|---------|------------------------------------------------------------------------------|
| `mode`            | `reply` | One of `reply`, `server-broadcast`, `client`.                                |
| `appendDelimiter` | —       | Bytes appended after `msg.payload` (JS-style escapes). Useful for `\n` framing. |
| `closeAfterSend`  | `false` | Half-close the connection after writing.                                     |
| `writeTimeout`    | `10s`   | Per-write deadline.                                                          |

### `server-broadcast` mode

| Field         | Description                                       |
|---------------|---------------------------------------------------|
| `targetTcpIn` | Node ID of the `tcp-in` server whose sessions receive the broadcast. |

Broadcast iterates sessions serially with a per-session
`writeTimeout`. A slow peer cannot block delivery to others — its
write fails after the deadline and the loop continues. Per-target
errors are logged but do not stop the broadcast.

### `client` mode

| Field               | Default | Description                                              |
|---------------------|---------|----------------------------------------------------------|
| `host`              | —       | Destination host. `msg.host` overrides.                  |
| `port`              | —       | Destination port. `msg.port` overrides.                  |
| `keepConnection`    | `true`  | Keep one connection across messages (recommended).       |
| `dialTimeout`       | `10s`   | Connect timeout.                                         |
| `outboundQueueSize` | `64`    | Backpressure buffer when the persistent conn is reconnecting. Overflow → catchable error. |
| `tls`               | —       | See [TLS configuration](tls.md).                         |

With `keepConnection=true` a single worker holds the connection
open and drains messages from a bounded outbound channel. On
disconnect it reconnects with exponential backoff. Messages
enqueued during the down phase wait in the channel; **overflow
raises a catchable error** rather than silently blocking — loud
failure beats silent stall.

## Incoming message

| Field             | Required | Description                                                  |
|-------------------|----------|--------------------------------------------------------------|
| `payload`         | yes*     | Body to send. `string` → UTF-8 bytes; `[]byte` / `[]int` → raw bytes; `nil` → empty write (only useful with `appendDelimiter`). |
| `session`         | reply    | The opaque handle from an upstream `tcp-in` server.          |
| `host`, `port`    | client   | Override the configured destination.                         |
| `closeAfterSend`  | optional | Per-message override.                                        |

(* `payload` may be empty when paired with `appendDelimiter`.)

## Errors

All write failures are catchable — pair the node with a Catch node to
react:

| Cause                            | Error text                                  |
|----------------------------------|---------------------------------------------|
| Missing `msg.session` (reply)    | `tcp-out … : no session handle on msg.session` |
| Session already closed (reply)   | `tcp-out … : flow: tcp session closed`      |
| Dial failure (client)            | `tcp-out … : dial host:port: …`             |
| Write timeout / broken pipe      | `tcp-out … : write: …`                      |
| Persistent client overflow       | `tcp-out … : outbound queue full`           |

## Examples

**Echo-on-receive with line framing.**

```
[tcp-in  server :7000 delim=\n] → [Function: uppercase] → [tcp-out reply append=\n]
```

**Push notifications to every connected client.**

```
[Inject 10s] → [Change set payload="heartbeat"] → [tcp-out broadcast → tcp-in :7000]
```

**Persistent client to a downstream service.**

```
[mqtt-in topic=ingest] → [Change …] → [tcp-out client device:9100 keepConnection=true]
```

## See also

- [TCP Receive](tcp-in.md) — produces the `msg.session` handle.
- [TCP Request](tcp-request.md) — when you also need to read a reply.
- [TLS configuration](tls.md) — for `client` mode.
