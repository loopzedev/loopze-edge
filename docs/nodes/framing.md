# Framing

TCP is a **byte stream**, not a message stream. Without explicit
framing the boundary between two messages is essentially random: a
single logical frame may be split across multiple kernel reads, or
several frames may arrive in one read. The **TCP Receive**
(`tcp-in`) and **TCP Request** (`tcp-request`) nodes solve this by
applying one of four framing strategies to the raw byte stream.

The same framing options also exist as terminator strategies on
`tcp-request`'s response side.

## Modes

| Mode            | Use when                                                                                  |
|-----------------|-------------------------------------------------------------------------------------------|
| `stream`        | Downstream is byte-oriented (e.g. assembling frames yourself in a Function node).         |
| `delimiter`     | Line protocols — JSON-lines, telnet, GPS NMEA, AT commands.                               |
| `length-prefix` | Most binary protocols — Modbus-TCP, Postgres wire, custom industrial.                     |
| `fixed-length`  | Block protocols where every record is exactly `N` bytes.                                  |

### `stream`

Pass-through. Every kernel read produces one message. The split
between messages is whatever the kernel decided to deliver — useful
only when the downstream flow does its own framing.

### `delimiter`

Split on a configurable byte sequence. The delimiter is **stripped**
from the emitted frames.

The delimiter accepts JS-style escape syntax:

| Escape    | Bytes        | Note                          |
|-----------|--------------|-------------------------------|
| `\n`      | `0x0A`       | LF                            |
| `\r\n`    | `0x0D 0x0A`  | CRLF                          |
| `\0`      | `0x00`       | NUL                           |
| `\xFF`    | `0xFF`       | Arbitrary byte                |
| `\t`      | `0x09`       | Tab                           |
| `\\`      | `0x5C`       | Literal backslash             |

Partial frames straddling read boundaries are buffered. A frame that
exceeds `maxFrameBytes` causes a catchable error and tears down the
connection, so a runaway peer can't exhaust memory.

### `length-prefix`

Read a fixed-size header that announces the payload size, then read
exactly that many payload bytes. Header is stripped.

| Field             | Meaning                                                |
|-------------------|--------------------------------------------------------|
| `bytes`           | Header size: 1, 2, 4, or 8.                            |
| `endianness`      | `big` (default, network) or `little`.                  |
| `includesHeader`  | Does the announced length include the header itself?   |

Example — 4-byte big-endian header announcing payload size only
(`includesHeader=false`):

```
00 00 00 05 'h' 'e' 'l' 'l' 'o'
└── header ──┘ └─ payload (5 bytes) ─┘
```

→ one frame, payload = `hello`.

### `fixed-length`

Every frame is exactly `fixedLength` bytes. No framing metadata on
the wire; the agreement is purely out-of-band.

## Common pitfalls

- **Delimiter inside the payload.** The framer does not understand
  escaping. A protocol that allows newlines inside a record needs
  `length-prefix` instead of `delimiter`.
- **Length prefix too small.** A 1-byte header caps frames at 255
  bytes. Match the header size to the largest frame the protocol can
  legitimately produce.
- **Mismatched endianness.** Most modern protocols are big-endian
  ("network byte order"), but legacy industrial protocols are often
  little-endian.
- **`maxFrameBytes`.** Default 1 MiB. Raise it for protocols that
  legitimately exceed that — but only after confirming the peer is
  trusted, since the cap is a memory-exhaustion guardrail.
