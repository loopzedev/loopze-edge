# TCP Receive (`tcp-in`)

Listens on a TCP port (server) **or** dials a remote (client) and
emits one message per received [frame](framing.md) on its single
output. Every accepted server-mode connection is registered with
the engine's session registry as a `msg.session` handle so a paired
[`tcp-out` reply](tcp-out.md) can write back on the same socket.

| Inputs | Outputs |
|--------|---------|
| 0      | 1       |

## Modes

| Mode     | Behaviour                                                                                    |
|----------|----------------------------------------------------------------------------------------------|
| `server` | Bind `host:port`, accept many concurrent peers, emit messages from each.                     |
| `client` | Dial the remote `host:port`, read frames, reconnect on drop with exponential backoff.        |

## Configuration

### Common

| Field              | Default     | Description                                                  |
|--------------------|-------------|--------------------------------------------------------------|
| `mode`             | `server`    | `server` or `client`.                                        |
| `host`             | `0.0.0.0`   | Bind address (server) or target host (client).               |
| `port`             | `7000`      | TCP port.                                                    |
| `framing`          | `stream`    | See [framing](framing.md): `stream` / `delimiter` / `length-prefix` / `fixed-length`. |
| `delimiter`        | `\n`        | Only with `framing=delimiter`. JS-style escapes supported.   |
| `lengthPrefix`     | —           | Only with `framing=length-prefix`. `{bytes, endianness, includesHeader}`. |
| `fixedLength`      | —           | Only with `framing=fixed-length`. Frame size in bytes.       |
| `payloadEncoding`  | `buffer`    | `buffer` (number array), `string`, or `base64`.              |
| `maxFrameBytes`    | `1048576`   | Hard cap per frame. Oversize → catchable error + close.      |
| `keepAlive`        | `true`      | Enable TCP keep-alive on the socket.                         |
| `keepAliveInterval`| `30s`       | Keep-alive idle interval.                                    |
| `nodelay`          | `false`     | Disable Nagle's algorithm (`TCP_NODELAY`).                   |
| `emitCloseEvent`   | `false`     | When true, emit a marker message with `payload=null` and `_event="close"` on disconnect. |

### Server-only

| Field             | Default | Description                                                                |
|-------------------|---------|----------------------------------------------------------------------------|
| `maxConnections`  | `0`     | Concurrency cap; `0` = unbounded. Excess connections are closed on accept. |
| `allowedRemotes`  | `[]`    | CIDR allowlist. Empty list = allow all.                                    |

### Client-only

| Field                     | Default | Description                                  |
|---------------------------|---------|----------------------------------------------|
| `reconnect`               | `true`  | Auto-reconnect with exponential backoff.     |
| `reconnectInitialDelay`   | `1000`  | First retry delay in **milliseconds**.       |
| `reconnectMaxDelay`       | `30000` | Backoff cap in **milliseconds**.             |
| `dialTimeout`             | `10`    | Connect timeout in **seconds**.              |
| `tls`                     | —       | See [TLS configuration](tls.md). Server-mode TLS is deferred to a future release. |

## Outgoing message

```json
{
  "_msgid": "...",
  "payload": "<bytes — encoded per payloadEncoding>",
  "session": "<opaque tcp-session handle>",
  "remoteAddr": "192.0.2.10:53412",
  "localAddr":  "0.0.0.0:7000",
  "ip":   "192.0.2.10",
  "port": 53412,
  "frameSize": 137
}
```

Server mode emits a unique `session` per accepted connection (reused
across frames from that peer). Client mode emits the same `session`
for the lifetime of the dialed connection — a new one is registered
after each reconnect.

When `emitCloseEvent=true`, on peer disconnect a marker message is
emitted with `payload=null`, `_event="close"`, and the same `session`,
`remoteAddr`, `ip`, `port`. Off by default to keep the data stream
clean.

## Status

- **Server**: green `listening · :7000 · 3 conn` — live count comes
  from the session registry.
- **Client**: green `connected · host:port`; yellow `retry: …` during
  backoff; red on configuration errors.

## Examples

**Echo server with line framing.**

```
[tcp-in  server :7000  delim=\n] → [Function: transform] → [tcp-out reply]
```

**Pull from a remote sensor with reconnect.**

```
[tcp-in client device:9100  delim=\n] → [Function: parse NMEA] → [Debug]
```

## See also

- [TCP Send](tcp-out.md) — paired writes via `msg.session`.
- [TCP Request](tcp-request.md) — short-lived synchronous round-trips.
- [Framing](framing.md) — explainer for the four framing strategies.
