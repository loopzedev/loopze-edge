# Nodes

LOOPZE ships a curated set of built-in nodes grouped by purpose.

## Input

| Node                   | What it does                              |
|------------------------|-------------------------------------------|
| Inject                 | Manually or periodically send a message.  |
| Modbus                 | Read / poll Modbus TCP or RTU registers.  |
| OPC UA                 | Read / subscribe to OPC UA nodes.         |
| [S7 Read](s7-read.md)  | Read variables or raw blocks from a SIEMENS S7 PLC. |

## Output

| Node                    | What it does                              |
|-------------------------|-------------------------------------------|
| Debug                   | Stream messages to the editor sidebar.    |
| OPC UA                  | Write to an OPC UA node.                  |
| [S7 Write](s7-write.md) | Write variables or raw blocks to a SIEMENS S7 PLC. |

## Network

| Node                                  | What it does                                                     |
|---------------------------------------|------------------------------------------------------------------|
| HTTP In / Response / Request          | Define HTTP endpoints in a flow, or call out to external HTTP services. |
| [MQTT Subscribe](mqtt-in.md)          | Subscribe to MQTT topics (static or dynamic, MQTT v5).           |
| [MQTT Publish](mqtt-out.md)           | Publish to an MQTT topic, including v5 response-topic replies.   |
| [MQTT Request](mqtt-request.md)       | Synchronous MQTT v5 request/response in one node.                |
| [TCP Receive](tcp-in.md)              | Listen on a TCP port (server) or dial a remote (client).         |
| [TCP Send](tcp-out.md)                | Reply on a session, broadcast to many, or dial a remote.         |
| [TCP Request](tcp-request.md)         | Synchronous TCP round-trip (dial → send → read → close).         |
| [UDP Receive](udp-in.md)              | Bind a UDP port (with optional multicast).                       |
| [UDP Send](udp-out.md)                | Send a UDP datagram (unicast / broadcast / multicast).           |

Cross-cutting reference:

- [Framing](framing.md) — strategies that turn TCP byte streams into messages.
- [TLS configuration](tls.md) — shared TLS block for TCP-based outbound nodes.

## Logic & flow control

| Node      | What it does                                                    |
|-----------|-----------------------------------------------------------------|
| Function  | Run JavaScript or expr-lang on the message.                     |
| Change    | Set, copy, move or delete fields on the message.                |
| Switch    | Route messages to outputs based on rules.                       |
| Template  | Render Mustache / Go templates.                                 |
| Split     | Split arrays / strings into multiple messages.                  |
| Join      | Recombine split messages.                                       |
| Delay     | Delay or rate-limit the message stream.                         |
| Link In / Link Out | Cross-flow wiring without dragged wires.               |

## Parsing

| Node                          | What it does                              |
|-------------------------------|-------------------------------------------|
| Parser JSON                   | Encode / decode JSON.                     |
| [Parser XML](xml-parser.md)   | Encode / decode XML.                      |
| Parser Modbus                 | Decode Modbus register frames.            |
| [Parser S7](s7-parser.md)     | Decode raw S7 byte blocks into structured objects (and back). |

## Industrial — SIEMENS S7

| Node                       | What it does                              |
|----------------------------|-------------------------------------------|
| [S7 PLC](s7-plc.md)        | Connection-config node for an S7 PLC (RFC1006 / ISO-on-TCP). |
| [S7 Read](s7-read.md)      | Static / dynamic / block-mode variable reads. |
| [S7 Write](s7-write.md)    | Static / dynamic / block-mode variable writes. |
| [S7 Parser](s7-parser.md)  | Decode block-mode bytes into structured objects (and back). |

## Diagnostics

| Node    | What it does                              |
|---------|-------------------------------------------|
| Status  | Inspect the status of another node.       |
| Catch   | Catch errors emitted by other nodes.      |
