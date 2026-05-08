# TCP Request (`tcp-request`)

Synchronous TCP round-trip. On every input message: dial (or reuse
a kept-open connection), write `msg.payload`, read the response per
the configured terminator, optionally close, emit the response on
the output. The boring, ergonomic primitive for short-lived
request-response over TCP.

| Inputs | Outputs |
|--------|---------|
| 1      | 1       |

## When to use this node vs. `tcp-in` + `tcp-out`

For the synchronous "ask a device a question, get an answer"
pattern, the connect+send+receive+close round-trip is one atomic
operation per message. `tcp-request` matches that mental model.

If you need a long-lived bidirectional stream where unsolicited
data can arrive at any time, use [`tcp-in`](tcp-in.md) +
[`tcp-out`](tcp-out.md) instead.

## Configuration

| Field              | Default   | Description                                                                |
|--------------------|-----------|----------------------------------------------------------------------------|
| `host`             | —         | Destination host. Mustache supported (`{{target.host}}`). `msg.host` overrides. |
| `port`             | —         | Destination port. Mustache supported. `msg.port` overrides.                |
| `terminator`       | `time`    | When to stop reading the response. See [Terminators](#terminators) below.  |
| `delimiter`        | `\n`      | Only with `terminator=delimiter`. JS-style escapes supported.              |
| `responseLength`   | —         | Only with `terminator=length`. Number of bytes to read.                    |
| `lengthPrefix`     | —         | Only with `terminator=length-prefix`. Same shape as in [`tcp-in`](tcp-in.md). |
| `appendDelimiter`  | —         | Bytes appended to `msg.payload` before sending (e.g. `\r\n`).              |
| `responseEncoding` | `buffer`  | `buffer` (number array), `string`, or `base64`.                            |
| `responseTimeout`  | `5000ms`  | Overall read deadline. Also the budget for `terminator=time`.              |
| `dialTimeout`      | `10s`     | Connect timeout.                                                           |
| `keepConnection`   | `false`   | Reuse one TCP connection across messages. See below.                       |
| `tls`              | —         | See [TLS configuration](tls.md).                                           |

## Terminators

| Mode            | When the read ends                                                              |
|-----------------|---------------------------------------------------------------------------------|
| `time`          | After `responseTimeout` ms — whatever has arrived is emitted.                   |
| `delimiter`     | When the configured `delimiter` byte sequence is seen (delimiter stripped).     |
| `length`        | After exactly `responseLength` bytes.                                           |
| `length-prefix` | After reading the length header and exactly that many payload bytes.            |
| `close`         | When the peer closes the connection (FIN).                                      |

## Keep-connection mode

With `keepConnection=true` the node opens a TCP connection on the
first message and **reuses** it for every subsequent call.

- Lower latency — no per-message dial / TLS handshake.
- On host/port change (via `msg.host` / `msg.port`) the cached conn
  is dropped and a fresh one is dialed.
- On a stale conn (peer dropped, NAT timeout) the failed round-trip
  triggers a transparent redial-and-retry — the caller sees the
  result of the retry, not the underlying error.
- Output messages report `connectionReused: true` from the second
  call onward, useful for diagnostics.
- Incompatible with `terminator=close` (peer FIN ends the
  connection — would defeat reuse). The configuration is rejected
  at deploy time.

## Outgoing message

```json
{
  "payload": "<response — encoded per responseEncoding>",
  "remoteAddr": "192.0.2.10:9100",
  "bytesSent": 14,
  "bytesReceived": 92,
  "durationMs": 41,
  "terminator": "delimiter",
  "connectionReused": true
}
```

Pre-existing `msg` fields are preserved unless overwritten by these.

## Status

- Pulse on round-trip in flight.
- Green `41ms · 92B` after success.
- Yellow on timeout / network error.
- Red on configuration error (invalid template, etc.).

## Examples

**Poll a barcode scanner.**

```
[Inject 1s] → [Change payload="?READ"] → [tcp-request 192.0.2.7:9100 delim=\r\n appendDelimiter=\r\n] → [Debug]
```

**Banner grab.**

```
[Inject once] → [tcp-request smtp.example.com:25 terminator=close] → [Debug]
```

**TLS-wrapped device with mTLS.**

```yaml
host: secure-device.example.com
port: 4443
terminator: delimiter
delimiter: "\n"
appendDelimiter: "\n"
responseEncoding: string
keepConnection: true
tls:
  enabled: true
  serverName: secure-device.example.com
  caBundle:   "-----BEGIN CERTIFICATE-----\n…"
  clientCert: "-----BEGIN CERTIFICATE-----\n…"
  clientKey:  "-----BEGIN PRIVATE KEY-----\n…"
```

## See also

- [TCP Receive](tcp-in.md) and [TCP Send](tcp-out.md) — long-lived sessions.
- [Framing](framing.md) — same strategies that drive the response terminators.
- [TLS configuration](tls.md).
