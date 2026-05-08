# Issue: TCP & UDP Nodes – Send / Receive (Server & Client)

## Status: Open

## Problem Description

LOOPZE needs first-class **transport-layer nodes** so flows can speak
raw TCP and UDP — both as a **server** (accept connections / bind a
port) and as a **client** (dial a remote / send datagrams). This is the
foundation for every protocol that does not already have a dedicated
node: industrial protocols on plain TCP, syslog over UDP, custom
device gateways, line-based legacy services, multicast discovery, and
similar long-tail integrations.

Five new node types are introduced:

- `tcp-in` — **TCP Receive** — listens on a port (server) **or** dials
  a remote host (client) and emits a message per received frame /
  chunk.
- `tcp-out` — **TCP Send** — writes the incoming `msg.payload` to a
  TCP peer. Three modes: **server** (broadcast to all currently
  connected clients), **client** (dial-and-send), **reply** (write
  back on the session that produced the inbound message).
- `tcp-request` — **TCP Request** — synchronous client: connect, send,
  read response (by length / delimiter / timeout / connection-close),
  close, emit response on the output.
- `udp-in` — **UDP Receive** — binds a UDP port (optionally joins a
  multicast group) and emits a message per datagram.
- `udp-out` — **UDP Send** — sends `msg.payload` as a datagram to a
  configured (or `msg`-overridden) destination, with optional
  broadcast / multicast support.

Together they let LOOPZE participate in any TCP- or UDP-based
exchange. The Node-RED nodes `tcp in`, `tcp out`, `tcp request`,
`udp in`, and `udp out` are the conceptual template; semantics are
kept compatible in spirit, with a few sharper edges (explicit modes
instead of overloading a single node, opaque session handles instead
of raw connection objects).

## Overview

| Node Type | Type ID | Canvas In | Canvas Out | Description |
|---|---|---|---|---|
| **TCP Receive** | `tcp-in` | 0 | 1 | Listens on a port (server) or dials a remote (client); emits messages for received data |
| **TCP Send** | `tcp-out` | 1 | 0 | Writes `msg.payload` to a TCP peer (server-broadcast / client / reply mode) |
| **TCP Request** | `tcp-request` | 1 | 1 | Connect → send → read response → close, emits the response |
| **UDP Receive** | `udp-in` | 0 | 1 | Binds a UDP port (optionally multicast) and emits one message per datagram |
| **UDP Send** | `udp-out` | 1 | 0 | Sends `msg.payload` as a UDP datagram (unicast / broadcast / multicast) |

### Typical Flow Shapes

**Server side – line-based TCP service (echo with transformation):**

```
[tcp-in  server :7000  delim=\n] → [Function: transform] → [tcp-out reply]
                                                ↓
                                        [Debug / MQTT-out]
```

**Client side – poll a legacy device via TCP:**

```
[Inject 5s] → [Change: build req bytes] → [tcp-request host:9100  delim=\r\n] → [Parser]
```

**UDP – syslog collector:**

```
[udp-in :514] → [Function: parse syslog] → [Switch facility] → [Debug / DB-out]
```

**UDP – mDNS / multicast discovery beacon:**

```
[Inject 30s] → [Change: build query] → [udp-out multicast 224.0.0.251:5353]
```

## Requirements

### 1. TCP Receive Node (`tcp-in`)

- **Canvas**: 0 inputs, 1 output (source node)
- **Function**: Two modes selectable in the config:
  - **server** — binds `host:port` and accepts inbound connections.
    Each connection is a long-lived session; received data is
    framed (see § 6) and emitted as one message per frame.
  - **client** — dials the configured `host:port` and reads from the
    socket. Emits one message per frame. Reconnects on drop with
    backoff (see § 7).

- **Configuration**:
  - `mode` (string) — `server` (default) or `client`
  - `host` (string) — server: bind address (default `0.0.0.0`); client:
    target host. Empty in server mode means all interfaces
  - `port` (number, required) — TCP port
  - `framing` (string) — how to split the byte stream into messages:
    - `stream` (default) — emit one message per `Read` chunk (no
      framing); `msg.payload` is a number array of the raw bytes
    - `delimiter` — split on a configured delimiter; the delimiter is
      stripped from `msg.payload`
    - `length-prefix` — fixed-size length header on every frame
      (configurable size + endianness); the header is stripped
    - `fixed-length` — every frame is exactly N bytes
  - `delimiter` (string) — only when `framing=delimiter`. Accepts
    JS-style escapes (`\n`, `\r\n`, `\0`, `\xFF`); default `\n`
  - `lengthPrefix` (object) — only when `framing=length-prefix`:
    - `bytes` (number) — 1, 2, 4, or 8. Default 4
    - `endianness` (string) — `big` (default) or `little`
    - `includesHeader` (boolean) — does the announced length include
      the header itself? Default `false`
  - `fixedLength` (number) — only when `framing=fixed-length`. Frame size in bytes
  - `payloadEncoding` (string) — how to decode the frame bytes into
    `msg.payload`:
    - `buffer` (default) — number array (raw bytes)
    - `string` — UTF-8 decoded string
    - `base64` — base64-encoded string (useful for binary that needs
      to round-trip through JSON storage)
  - `maxFrameBytes` (number) — hard cap on a single frame, default
    `1048576` (1 MiB). Exceeding it closes the connection and emits a
    catchable error
  - `keepAlive` (boolean) — server: enable TCP keep-alive on accepted
    sockets; client: same on the dialed socket. Default `true`
  - `keepAliveInterval` (number, seconds) — only when `keepAlive=true`. Default 30
  - `nodelay` (boolean) — disable Nagle's algorithm. Default `false`
  - **client-only:**
    - `reconnect` (boolean) — auto-reconnect on disconnect. Default `true`
    - `reconnectInitialDelay` (number, ms) — first retry after this
      delay. Default `1000`
    - `reconnectMaxDelay` (number, ms) — exponential cap. Default `30000`
    - `dialTimeout` (number, seconds) — per-attempt connect timeout. Default `10`
  - **server-only:**
    - `maxConnections` (number) — concurrency cap; new connections
      beyond this are rejected with an immediate close. Default `0`
      (unbounded)
    - `allowedRemotes` (string[], optional) — CIDR allowlist, e.g.
      `["10.0.0.0/8", "192.168.1.42/32"]`. Empty list = allow all

- **Outgoing message** (one per frame):
  ```json
  {
    "_msgid": "...",
    "payload": "<bytes — encoded per payloadEncoding>",
    "session": "<opaque session handle — see § 5>",
    "remoteAddr": "192.0.2.10:53412",
    "localAddr":  "0.0.0.0:7000",
    "ip":   "192.0.2.10",
    "port": 53412,
    "frameSize": 137
  }
  ```
  - In **client** mode `session` is the same handle for the lifetime
    of the dialed connection (reused across frames).
  - In **server** mode `session` is unique per accepted connection —
    `tcp-out reply` uses it to find the right writer.
  - On disconnect a special **end-of-stream marker message** is
    emitted with `payload = null` and `_event = "close"` so flows can
    react to peer disconnects.

- **Status display**:
  - **server** — green `listening · :7000 · 3 conn` (live count of
    open sessions); red on bind error
  - **client** — green `connected · host:port`; yellow `reconnecting
    in 4s` during backoff; red on configuration / DNS error

- **Properties panel**:

```
┌──────────────────────────────────────────────┐
│  TCP Receive                                  │
├──────────────────────────────────────────────┤
│                                               │
│  Mode                                         │
│  ( • ) Server  (listen on port)               │
│  (   ) Client  (connect to remote)            │
│                                               │
│  Host                          Port           │
│  ┌────────────────────┐        ┌────────┐     │
│  │ 0.0.0.0            │        │ 7000   │     │
│  └────────────────────┘        └────────┘     │
│                                               │
│  Framing                                      │
│  ┌────────────────────────────────────────┐   │
│  │ Delimiter                          ▼  │   │
│  └────────────────────────────────────────┘   │
│  Delimiter: [\n              ] (JS escapes)   │
│                                               │
│  Payload as                                   │
│  ┌────────────────────────────────────────┐   │
│  │ Buffer (number array)              ▼  │   │
│  └────────────────────────────────────────┘   │
│                                               │
│  Max frame size: [1048576] bytes              │
│  ☑ TCP keep-alive   Interval: [30] s          │
│  ☐ Disable Nagle (TCP_NODELAY)                │
│                                               │
│  ▼ Server options                             │
│  Max connections:  [0 = unlimited]            │
│  Allowed remotes (CIDR, comma-sep):           │
│  [10.0.0.0/8, 192.168.1.0/24             ]    │
│                                               │
│  ▼ Client options (hidden in server mode)     │
│  ☑ Auto-reconnect                             │
│  Initial delay: [1000] ms  Max: [30000] ms    │
│  Dial timeout: [10] s                         │
└──────────────────────────────────────────────┘
```

### 2. TCP Send Node (`tcp-out`)

- **Canvas**: 1 input, 0 outputs (sink node)
- **Function**: Three modes selectable in the config:
  - **reply** — looks up the session via `msg.session` (set by the
    upstream `tcp-in` server) and writes `msg.payload` on that
    connection. Without a valid `msg.session` ⇒ catchable error
    `"tcp-out: no session handle on msg.session"`
  - **server-broadcast** — writes to **every** currently connected
    client of a referenced `tcp-in` (server mode). Useful for
    fan-out / push notifications. The referenced `tcp-in` is selected
    from a dropdown of server-mode `tcp-in` nodes in the same flow
  - **client** — opens a connection to `host:port`, writes
    `msg.payload`, optionally keeps the connection open across
    messages (`keepConnection=true`) or closes after each send

- **Configuration**:
  - `mode` (string) — `reply` (default) | `server-broadcast` | `client`
  - `host`, `port` — only `client` mode (overridable via `msg.host`/
    `msg.port`)
  - `targetTcpIn` (string) — only `server-broadcast` mode: ID of the
    `tcp-in` server node whose clients should receive the broadcast
  - `keepConnection` (boolean) — client mode: keep the dialed
    connection open and reuse it for subsequent messages. Default
    `true`. When `false`, the node opens, writes, and closes per
    message
  - `appendDelimiter` (string, optional) — bytes to append after
    `msg.payload` (mirrors `tcp-in`'s `delimiter` parsing). Empty =
    none. Useful for line protocols
  - `closeAfterSend` (boolean, optional) — reply / client modes only:
    half-close the connection (FIN) after this write. Default `false`
  - `dialTimeout` (number, seconds) — client mode. Default `10`
  - `writeTimeout` (number, seconds) — applies to all modes; default `10`

- **Incoming message**:
  - `msg.payload` — body to send. Encoded by type:
    - `string` → UTF-8 bytes
    - `[]byte` / number array → raw bytes
    - `nil` / missing → empty write (only sensible with
      `appendDelimiter`)
  - `msg.session` — required in `reply` mode; ignored otherwise
  - `msg.host` / `msg.port` — `client` mode override
  - `msg.closeAfterSend` (boolean) — per-message override

- **Properties panel**:

```
┌──────────────────────────────────────────────┐
│  TCP Send                                     │
├──────────────────────────────────────────────┤
│                                               │
│  Mode                                         │
│  ( • ) Reply           (use msg.session)      │
│  (   ) Server broadcast                       │
│  (   ) Client          (dial host:port)       │
│                                               │
│  ▼ Reply mode                                 │
│  ☐ Close connection after send                │
│                                               │
│  ▼ Server broadcast mode                      │
│  Target TCP-In:                               │
│  ┌────────────────────────────────────────┐   │
│  │ TCP-In node :7000                  ▼  │   │
│  └────────────────────────────────────────┘   │
│                                               │
│  ▼ Client mode                                │
│  Host:                  Port:                 │
│  [device.local       ]  [9100]                │
│  ☑ Keep connection across messages            │
│  ☐ Close after send                           │
│  Dial timeout: [10] s                         │
│                                               │
│  Append after payload (delimiter): [        ] │
│  Write timeout: [10] s                        │
└──────────────────────────────────────────────┘
```

### 3. TCP Request Node (`tcp-request`)

- **Canvas**: 1 input, 1 output
- **Function**: Synchronous request-response. On each input message:
  dial → write `msg.payload` → read response per the configured
  termination rule → close → emit response on the output.

- **Configuration**:
  - `host`, `port` — destination. Mustache substitution over `msg`
    is supported (same engine as the Template / `http-request` nodes).
    `msg.host` / `msg.port` win on conflict
  - `terminator` (string) — when does the response read finish?
    - `time` (default) — read for `responseTimeout` ms then close
    - `delimiter` — read until the configured `delimiter` byte
      sequence is seen (delimiter stripped from output)
    - `length` — read exactly `responseLength` bytes
    - `length-prefix` — same shape as `tcp-in.lengthPrefix`: read
      header → read N bytes → emit
    - `close` — read until the peer closes the connection (FIN). Most
      "fetch a banner / dump" protocols use this
  - `responseTimeout` (number, ms) — overall read deadline; also the
    duration for `terminator=time`. Default `5000`
  - `delimiter`, `responseLength`, `lengthPrefix` — see `tcp-in`
  - `appendDelimiter` (string, optional) — bytes appended to the
    request payload before sending (e.g. `\n`)
  - `responseEncoding` (string) — `buffer` (default) | `string` |
    `base64`
  - `dialTimeout` (number, seconds) — connect timeout. Default `10`
  - `tls` (object, optional) — see § 8

- **Incoming message**: `msg.payload`, `msg.host`, `msg.port`,
  `msg.timeout` (ms, per-request override of `responseTimeout`)

- **Outgoing message**:
  ```json
  {
    "payload": "<response — encoded per responseEncoding>",
    "remoteAddr": "192.0.2.10:9100",
    "bytesSent": 14,
    "bytesReceived": 92,
    "durationMs": 41,
    "terminator": "delimiter"
  }
  ```
  Pre-existing `msg` properties are preserved unless overwritten by
  these fields.

- **Status display**: pulse blue while a request is in flight; green
  `42ms · 92B` after success; yellow on timeout; red on
  config / DNS / TLS error.

### 4. UDP Receive Node (`udp-in`)

- **Canvas**: 0 inputs, 1 output (source node)
- **Function**: Binds a UDP port and emits one message per received
  datagram. Optionally joins one or more multicast groups.

- **Configuration**:
  - `host` (string) — bind address. `0.0.0.0` (default) for all IPv4
    interfaces, `::` for all IPv6, or a specific local address
  - `port` (number, required) — UDP port to bind. `0` lets the OS
    pick a free port (rare; used only if a flow needs an ephemeral
    inbound)
  - `multicastGroups` (string[], optional) — multicast addresses to
    join. Each entry may be `"224.0.0.251"` or
    `"224.0.0.251%eth0"` to pin the join to an interface. v1 supports
    IPv4 multicast; IPv6 multicast is a follow-up
  - `reusePort` (boolean) — `SO_REUSEPORT` so multiple LOOPZE
    instances or sibling nodes can share the bind. Default `false`
  - `payloadEncoding` (string) — `buffer` (default) | `string` | `base64`
  - `maxDatagramBytes` (number) — read buffer size. Default `65507`
    (max UDPv4 datagram). Datagrams larger than this are truncated
    and the truncation is reported as `msg.truncated = true`
  - `allowedRemotes` (string[], optional) — CIDR allowlist; mismatched
    senders are dropped silently with a metrics counter increment

- **Outgoing message** (one per datagram):
  ```json
  {
    "payload": "<bytes — encoded per payloadEncoding>",
    "ip":   "192.0.2.10",
    "port": 49152,
    "remoteAddr": "192.0.2.10:49152",
    "localAddr":  "0.0.0.0:514",
    "size": 137,
    "truncated": false
  }
  ```

- **Status display**: green `listening · :514` (with `· mcast` suffix
  when groups joined); red on bind error.

### 5. UDP Send Node (`udp-out`)

- **Canvas**: 1 input, 0 outputs (sink node)
- **Function**: Sends `msg.payload` as a UDP datagram to a configured
  destination (or to `msg.host` / `msg.port`). Supports unicast,
  broadcast (`SO_BROADCAST`), and multicast (with TTL + outbound
  interface).

- **Configuration**:
  - `host` (string) — destination address. `msg.host` overrides
  - `port` (number) — destination port. `msg.port` overrides
  - `bindHost` (string, optional) — local bind address; usually
    empty (let the OS pick the source). For multicast emit, this
    selects the outbound interface
  - `mode` (string) — `unicast` (default) | `broadcast` | `multicast`
  - `multicastTTL` (number) — only `multicast`. Default `1` (LAN
    only)
  - `multicastLoopback` (boolean) — only `multicast`. Default `true`
  - `reuseSocket` (boolean) — keep one outbound UDP socket open and
    reuse it across messages. Default `true`. Set to `false` for
    "one socket per send" (rarely needed; useful when source-port
    randomization matters per packet)

- **Incoming message**:
  - `msg.payload` — bytes to send (string / `[]byte` / number array).
    Empty payload sends a zero-length datagram (legal in UDP)
  - `msg.host`, `msg.port` — per-message override

- **Status display**: green `sent · 137B → host:port` (last send);
  red on configuration / resolution error.

### 6. Framing — Why and How

TCP is a byte stream, not a message stream. Without explicit framing
the "one message per `Read`" behaviour is essentially random: a
single logical frame may be split across multiple reads, or several
frames may arrive in one read. The framing options on `tcp-in` and
`tcp-request` solve this:

| Mode | Use when |
|---|---|
| `stream` | downstream is byte-oriented (e.g. piping to a Buffer node, or you assemble frames yourself in a Function) |
| `delimiter` | line protocols (HTTP-ish, JSON-lines, telnet, GPS NMEA, AT commands) |
| `length-prefix` | most binary protocols (Modbus-TCP, Postgres wire, custom industrial) |
| `fixed-length` | block protocols (e.g. fixed-record telemetry) |

Framing buffers are **per session**, capped by `maxFrameBytes`. A
client that exceeds the cap is closed with a catchable error so a
runaway peer cannot cause unbounded memory growth.

### 7. Reconnect & Backoff (TCP client / `tcp-in` client mode)

When `mode=client` and the upstream connection drops, the node
reconnects with **exponential backoff and jitter**:

```
delay_n = min(reconnectMaxDelay,
              reconnectInitialDelay * 2^n) ± rand(±20%)
```

Status reflects state transitions: `connected` → `disconnected · retry
in 4s` → `reconnecting` → `connected`. On `Stop()` the backoff
goroutine exits cleanly.

`tcp-out` in `client` mode with `keepConnection=true` follows the same
backoff strategy when the persistent connection drops mid-flow:
inbound messages that arrive while the connection is unhealthy queue
up to `outboundQueueSize` (default 64) messages, beyond which the
node emits a catchable `"tcp-out: outbound queue full"` and drops
the message (no implicit unbounded buffering — that hides bugs).

### 8. TLS Support

`tcp-out client`, `tcp-in client`, and `tcp-request` accept an
optional `tls` block:

```json
{
  "enabled": true,
  "serverName": "device.example.com",
  "caBundle": "<PEM, optional — defaults to system roots>",
  "clientCert": "<PEM, optional>",
  "clientKey":  "<PEM, optional>",
  "insecureSkipVerify": false
}
```

`tcp-in server` does **not** support TLS in v1 — TLS termination on
the server side is the job of a reverse proxy in production
deployments, and adding it here would multiply config surface for a
low-frequency need. Tracked as a follow-up: `tcp-in server: TLS
listener support`.

UDP nodes have no TLS option (DTLS is out of scope; if it materializes,
it ships as a separate `dtls-*` node trio).

### 9. Session Handle (`msg.session`)

`msg.session` on a `tcp-in server` output is an opaque string ID
backed by an engine-wide **session registry**. Same rationale as
`msg.res` for HTTP:

1. **Concurrency**: messages may fan out / clone; multiple branches
   must not race on the same `net.Conn`
2. **Serializability**: Debug node renders `<tcp session>` instead of
   trying to JSON-marshal a socket
3. **Cross-flow safety**: a session that crosses Link nodes into a
   different flow stays valid because the registry is engine-wide

The registry exposes:

```go
type SessionRegistry interface {
    Register(conn net.Conn, ownerID string) (sessionID string, done <-chan struct{})
    Resolve(sessionID string) (SessionSlot, bool)
    CloseByOwner(ownerID string)  // tcp-in calls this on Stop()
}
type SessionSlot struct {
    Conn   net.Conn
    Mu     sync.Mutex            // serializes writes from concurrent reply branches
    Owner  string                 // tcp-in node id that produced the session
    Closed atomic.Bool
}
```

`tcp-out` mode `reply` calls `Resolve(msg.session)`, takes
`slot.Mu`, writes, releases. After the connection closes, the
`done` channel is closed and the slot is removed from the registry;
subsequent `Resolve` returns `(_, false)` and `tcp-out reply` raises
a catchable `"tcp-out: session closed"` error.

Server-broadcast mode iterates over all active sessions whose
`Owner == targetTcpIn` and writes to each, serializing per session
via the same mutex. Per-target write errors are logged and the next
target is tried (one slow client should not block broadcast to the
others — broadcast is best-effort).

### 10. Error Handling & Catch Integration

All five nodes are good Catch citizens (per `NODE_STATUS.md` /
`Catch Node`):

- `tcp-in`: bind failures, accept errors, frame oversize, decoder
  errors, peer disconnect (only as `_event=close`, not as error —
  disconnect is normal)
- `tcp-out`: missing session, write timeout, dial failure (client
  mode), outbound queue full
- `tcp-request`: dial / TLS / read / write timeout, terminator never
  reached (timeout)
- `udp-in`: bind failure, multicast join failure, oversized datagram
  beyond `maxDatagramBytes` (truncation is reported but **not** an
  error)
- `udp-out`: send failure (rare — usually only DNS or kernel buffer
  full), invalid host/port

All errors carry `_error.source.{id, type, name, flowId}` per the
Catch contract.

### 11. Binary Payload Convention

Same as MQTT / HTTP: bytes are number arrays in JSON-serialized
contexts (Debug, persistence) because Go's `encoding/json` renders
`[]byte` as base64. `string` and `base64` encodings are explicit
opt-ins for users who want different representations.

## Data Structure

### workspace.json

```json
{
  "flows": [
    {
      "id": "flow-1",
      "type": "tab",
      "label": "Devices",
      "nodes": [
        {
          "id": "node-tcp-in-1",
          "type": "tcp-in",
          "name": "Line server",
          "x": 200, "y": 150, "z": "flow-1",
          "inputs": 0, "outputs": 1,
          "wires": [["node-fn-1"]],
          "config": {
            "mode": "server",
            "host": "0.0.0.0",
            "port": 7000,
            "framing": "delimiter",
            "delimiter": "\\n",
            "payloadEncoding": "string",
            "maxFrameBytes": 65536,
            "keepAlive": true
          }
        },
        {
          "id": "node-tcp-out-1",
          "type": "tcp-out",
          "name": "Reply",
          "x": 700, "y": 150, "z": "flow-1",
          "inputs": 1, "outputs": 0,
          "wires": [],
          "config": {
            "mode": "reply",
            "appendDelimiter": "\\n",
            "writeTimeout": 10
          }
        },
        {
          "id": "node-tcp-req-1",
          "type": "tcp-request",
          "name": "Poll device",
          "x": 200, "y": 350, "z": "flow-1",
          "inputs": 1, "outputs": 1,
          "wires": [["node-debug-1"]],
          "config": {
            "host": "device.local",
            "port": 9100,
            "terminator": "delimiter",
            "delimiter": "\\r\\n",
            "appendDelimiter": "\\r\\n",
            "responseEncoding": "string",
            "responseTimeout": 5000,
            "dialTimeout": 10
          }
        },
        {
          "id": "node-udp-in-1",
          "type": "udp-in",
          "name": "syslog :514",
          "x": 200, "y": 550, "z": "flow-1",
          "inputs": 0, "outputs": 1,
          "wires": [["node-fn-2"]],
          "config": {
            "host": "0.0.0.0",
            "port": 514,
            "payloadEncoding": "string",
            "maxDatagramBytes": 65507
          }
        },
        {
          "id": "node-udp-out-1",
          "type": "udp-out",
          "name": "mDNS query",
          "x": 200, "y": 750, "z": "flow-1",
          "inputs": 1, "outputs": 0,
          "wires": [],
          "config": {
            "host": "224.0.0.251",
            "port": 5353,
            "mode": "multicast",
            "multicastTTL": 1,
            "multicastLoopback": true
          }
        }
      ]
    }
  ]
}
```

No new config nodes for v1 — destinations live inline. A shared
`tls-credentials` config node is a candidate follow-up once multiple
nodes need to share trust bundles.

## Affected Files

### Backend – New Files

- `internal/nodes/tcp_in.go` — server (listener + accept loop) and
  client (dialer + reconnect) modes; per-session goroutine doing
  framing → `SendFunc`
- `internal/nodes/tcp_out.go` — reply / server-broadcast / client
  modes; persistent-connection worker for client-keep mode;
  outbound queue for backpressure during reconnect
- `internal/nodes/tcp_request.go` — synchronous request-response
  with all four terminator modes
- `internal/nodes/udp_in.go` — `net.ListenUDP`; multicast group
  joining via `golang.org/x/net/ipv4` (already vendored? if not,
  vendor it — see § Dependencies)
- `internal/nodes/udp_out.go` — `net.DialUDP` / `net.ListenUDP` for
  broadcast/multicast; reusable socket cache
- `internal/nodes/framing.go` — shared frame splitter (delimiter /
  length-prefix / fixed-length); reused by `tcp-in` and `tcp-request`
- `internal/nodes/tcp_*_test.go`, `udp_*_test.go` — table-driven
  unit tests using `net.Pipe()` and ephemeral OS-assigned ports
- `internal/flow/session_registry.go` — engine-wide session
  registry shared by `tcp-in` (producer) and `tcp-out` (consumer)

### Backend – Adjustments

- `internal/server/server.go: registerNodes()` — register
  `tcp-in`, `tcp-out`, `tcp-request`, `udp-in`, `udp-out`
- `internal/flow/registry.go` — new provider:
  ```go
  type SessionRegistryProvider interface {
      SetSessionRegistry(reg SessionRegistry)
  }
  ```
  injected into `tcp-in` and `tcp-out` at start so they share the
  same registry instance for the engine
- `internal/flow/engine.go` — on `Stop()`: call
  `sessionRegistry.CloseAll()` so every TCP socket is closed
  before goroutines exit; this prevents leaked file descriptors
  across redeploys
- `internal/config/config.go` — no new top-level config required;
  the nodes are self-contained

### Frontend – New Files

- `frontend/src/components/config/TcpInConfig.vue` — mode toggle
  (server vs client), bind/host+port, framing dropdown with
  context-sensitive sub-fields, payload encoding, server / client
  options sections
- `frontend/src/components/config/TcpOutConfig.vue` — mode radio
  (reply / broadcast / client), conditional fields, target tcp-in
  dropdown for broadcast
- `frontend/src/components/config/TcpRequestConfig.vue` — host/port
  with mustache hint, terminator dropdown with sub-fields, response
  encoding, timeouts, optional TLS section
- `frontend/src/components/config/UdpInConfig.vue` — host/port,
  multicast groups list, payload encoding, max datagram size,
  allowlist
- `frontend/src/components/config/UdpOutConfig.vue` — host/port
  (mustache), mode radio (unicast/broadcast/multicast), multicast
  TTL + loopback, reuse-socket flag
- `frontend/src/components/nodes/TcpInNode.vue`,
  `TcpOutNode.vue`, `TcpRequestNode.vue`, `UdpInNode.vue`,
  `UdpOutNode.vue` — palette-friendly node bodies (mode badge +
  host:port summary)

### Frontend – Adjustments

- `frontend/src/components/config/configEditors.ts` — dispatch the
  five new types to their config components
- `frontend/src/components/nodes/tokens.ts` — palette tokens
  (group: `network` / `connector`, consistent with HTTP & MQTT
  placement)
- `frontend/src/types/flow.ts` — extend the `NodeType` union with
  `tcp-in`, `tcp-out`, `tcp-request`, `udp-in`, `udp-out`
- `frontend/src/components/help/docs.ts` — five help entries with
  the message envelope, framing examples, and one full example
  flow per node

### Go Dependencies

- Standard library `net` covers TCP entirely. **No** new dependency
  for TCP.
- For UDP multicast group joining with interface pinning, prefer
  `golang.org/x/net/ipv4` (small, well-known). If avoiding the
  dependency is preferred, drop interface-pinned multicast in v1
  and accept that the OS picks the interface — document the
  limitation.

## Technical Notes

### Goroutine Model

- `tcp-in server`: 1 listener goroutine + 1 reader goroutine per
  accepted connection. Each reader goroutine owns its framing
  buffer and exits on EOF / error / `Stop()`
- `tcp-in client`: 1 dialer-and-reader goroutine; on disconnect
  the same goroutine handles backoff and re-dial
- `tcp-out client keepConnection`: 1 worker goroutine consuming
  the per-node outbound channel; reconnect handled inline
- `tcp-request`: per-message goroutine driven by `HandleMessage`;
  no long-lived workers
- `udp-in`: 1 read-loop goroutine
- `udp-out`: synchronous from `HandleMessage` (UDP send is
  blocking-but-fast); no worker goroutine

All goroutines exit cleanly on `Stop()` via context cancellation
plus an explicit socket close (which unblocks `Read`).

### Why explicit modes instead of one overloaded node?

Node-RED's `tcp out` has three behaviour modes packed into one node;
they're easy to misconfigure and the help text is famously confusing.
Splitting `tcp-out` into a single node with a clearly visible mode
selector keeps the canvas readable: `tcp-out [reply]`,
`tcp-out [→ device.local:9100]`, `tcp-out [broadcast → tcp-in
:7000]` are all immediately legible.

### Backpressure on outbound queue

`tcp-out client keepConnection` uses a bounded outbound channel
(`outboundQueueSize`, default 64) to handle the case where messages
arrive while the upstream socket is reconnecting. When full, new
messages raise a catchable error rather than blocking the upstream
node — silently blocking would propagate backpressure into nodes
that cannot reason about it (e.g., `udp-in`, where a stuck `tcp-out`
would cause datagram loss). Loud failure beats silent stall.

### Multicast reception lifecycle

`udp-in` joins the multicast groups on Start and leaves them on
Stop. Joins / leaves are best-effort: a failure to join is reported
as a catchable error but does not prevent the node from going green
on the unicast bind (so a misconfigured group address does not
disable the entire listener).

### IPv6

All nodes accept IPv6 addresses where IPv4 is accepted (`[::1]:7000`,
`[fe80::1%eth0]:5353`, etc.). Multicast on IPv6 is **deferred** to a
follow-up issue — v1 supports IPv4 multicast only. UDP unicast and
broadcast on IPv6 work transparently via Go's `net.Dial("udp", ...)`.

### Resource limits

Nothing introduces an unbounded resource:

| Resource | Bound | Rationale |
|---|---|---|
| TCP server connections | `maxConnections` (default unbounded — operator's job) | matches Go HTTP server convention |
| Frame buffer per session | `maxFrameBytes` (1 MiB default) | prevents runaway peer from exhausting memory |
| UDP datagram size | `maxDatagramBytes` (65507) | hard MTU ceiling for UDPv4 |
| Outbound queue (`tcp-out`) | `outboundQueueSize` (64) | bounded; overflow → catchable error |
| Session registry | grows with active connections | bounded transitively by `maxConnections` |

### Why a separate `tcp-request` and not just `tcp-out` + `tcp-in`?

For the synchronous "ask a device a question, get an answer" pattern
the connect+send+receive+close round-trip is one atomic operation in
a single message. Doing it with a long-lived `tcp-in` + `tcp-out`
pair forces the user to maintain connection state across messages,
correlate request and response, and handle reconnect — none of which
fits the "fire one message, get one message back" mental model.
`tcp-request` is the boring, ergonomic primitive for short-lived
request-response over TCP.

## Tests

| Test | Verifies |
|---|---|
| `TestTcpIn_Server_AcceptAndFrameDelimiter` | Multiple delimited frames → multiple messages with correct payload |
| `TestTcpIn_Server_FrameOversize` | Frame > `maxFrameBytes` closes the connection and emits catchable error |
| `TestTcpIn_Server_LengthPrefix` | 4-byte big-endian length-prefix framing produces correct frames |
| `TestTcpIn_Server_FixedLength` | Fixed-length framing produces correct equal-size frames |
| `TestTcpIn_Server_PeerDisconnect` | Peer FIN emits `_event=close` message, removes session from registry |
| `TestTcpIn_Server_AllowedRemotes` | CIDR allowlist blocks unmatched remote |
| `TestTcpIn_Client_Connects` | Client mode dials a test server and reads frames |
| `TestTcpIn_Client_ReconnectBackoff` | Test server closes; client retries with exponential backoff (use a fake clock) |
| `TestTcpIn_Stop_ClosesAllSessions` | Stop() closes every accepted socket and exits goroutines |
| `TestTcpOut_Reply_WritesOnSession` | `msg.session` from `tcp-in` reaches the right peer |
| `TestTcpOut_Reply_NoSession` | Missing `msg.session` → catchable error |
| `TestTcpOut_Reply_SessionClosed` | Session already closed → catchable error |
| `TestTcpOut_ServerBroadcast_FanOut` | All N connected clients receive the payload exactly once |
| `TestTcpOut_ServerBroadcast_SlowPeer` | Slow peer does not block fan-out to other peers |
| `TestTcpOut_Client_KeepConnection` | Single connection reused across N messages |
| `TestTcpOut_Client_Reconnect` | Persistent client reconnects after server restart |
| `TestTcpOut_Client_QueueOverflow` | Backpressure: > queue size → catchable error |
| `TestTcpOut_AppendDelimiter` | Configured delimiter is appended after each payload |
| `TestTcpRequest_Delimiter` | Reads until delimiter, strips it, emits |
| `TestTcpRequest_Length` | Reads exactly N bytes |
| `TestTcpRequest_LengthPrefix` | Reads header → reads body → emits |
| `TestTcpRequest_Close` | Reads until peer FIN, emits all bytes received |
| `TestTcpRequest_Time` | Reads for the configured duration, emits accumulated bytes |
| `TestTcpRequest_Timeout` | Server hangs > timeout → catchable error |
| `TestTcpRequest_DialError` | Unreachable host → catchable error |
| `TestTcpRequest_TLS_HappyPath` | TLS connection to a self-signed server succeeds with `caBundle`/`insecureSkipVerify` |
| `TestTcpRequest_MustacheHostPort` | `{{payload.target}}` resolves into host/port |
| `TestUdpIn_BasicReceive` | Send datagram → message emitted with correct payload + remoteAddr |
| `TestUdpIn_MulticastJoin` | Joining 224.0.0.251 → datagrams to that group are received |
| `TestUdpIn_TruncatedDatagram` | Datagram > `maxDatagramBytes` → message with `truncated=true` |
| `TestUdpIn_AllowedRemotes` | CIDR-mismatched sender → datagram dropped |
| `TestUdpIn_BindError` | Already-bound port → catchable bind error on Start |
| `TestUdpOut_Unicast` | Configured host/port receives the datagram |
| `TestUdpOut_MsgOverrides` | `msg.host` / `msg.port` win over config |
| `TestUdpOut_Broadcast` | `mode=broadcast` sets `SO_BROADCAST` and reaches the broadcast address |
| `TestUdpOut_Multicast` | `mode=multicast` honours TTL and loopback flags |
| `TestUdpOut_ReuseSocket` | Same socket reused across messages when `reuseSocket=true` |
| `TestEndToEnd_TcpInToReplyOut` | tcp-in server + function + tcp-out reply: external client gets transformed echo |
| `TestEndToEnd_TcpRequestRoundTrip` | tcp-request → fake server → response back into flow |
| `TestEndToEnd_UdpEcho` | udp-in + udp-out round-trip |
| `TestEngineRedeploy_NoFdLeak` | Repeated deploy/stop cycles: open file descriptor count stable |
| `TestSessionRegistry_ResolveAfterClose` | Registry returns `(_, false)` after the session has ended |

## Dependencies

- **`Catch Node` issue (`CATCH_NODE.md`)** — strongly recommended so
  async errors (frame oversize, dial failure, write timeout) are
  catchable. Not a hard blocker — errors still log via
  `publishNodeError` — but the user-facing story is much better with
  Catch in place
- **`NODE_STATUS.md`** — the dynamic per-session status counter on
  `tcp-in server` (`listening · :7000 · 3 conn`) reuses the status
  callback contract
- **`Template Node` engine** — reused for `{{mustache}}` host/port
  rendering in `tcp-request` and `udp-out`
- **No** new external Go dependencies for TCP. `golang.org/x/net/ipv4`
  optional for IPv4 multicast group joining with interface pinning
  (see § Go Dependencies)

## Out of Scope / Not in Scope

Deliberately deferred to keep v1 lean:

- **DTLS** (UDP TLS). Separate `dtls-*` nodes if demand materializes
- **TLS listener for `tcp-in server`**. Reverse-proxy in production;
  follow-up issue for a built-in option
- **IPv6 multicast**. v1 ships IPv4 multicast; v6 is a follow-up
- **SCTP / QUIC**. Out of scope; their own node trio when needed
- **Automatic protocol detection / Banner sniffing** on `tcp-in`.
  Users handle protocol identification in a Function node
- **WebSocket / TCP framing protocols** (e.g., STOMP, AMQP, IRC).
  Each has its own node trio if added — these are byte-level
  primitives, not protocol nodes
- **Per-session state attached to the session handle**. v1 carries
  only the connection. Per-session user state belongs in flow
  context keyed by `msg.session`
- **Hot reconfiguration without reconnect**. Changing `host`/`port`
  in `tcp-in client` triggers a full reconnect on deploy. A
  graceful re-bind without dropping sessions is a follow-up
- **Source-address spoofing for UDP**. Requires raw sockets and
  privileges; out of scope
- **PROXY protocol v1/v2** support on `tcp-in server`. Follow-up if
  users behind a HAProxy / cloud LB need original client IPs
- **Rate limiting / DoS protection**. Operator's responsibility via
  reverse proxy, firewall, or explicit Function-node throttle
