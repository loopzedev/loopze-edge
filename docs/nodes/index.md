# Nodes

LOOPZE ships a curated set of built-in nodes grouped by purpose.

## Input

| Node    | What it does                              |
|---------|-------------------------------------------|
| Inject  | Manually or periodically send a message.  |
| MQTT In | Subscribe to an MQTT topic.               |
| Modbus  | Read / poll Modbus TCP or RTU registers.  |
| OPC UA  | Read / subscribe to OPC UA nodes.         |

## Output

| Node     | What it does                              |
|----------|-------------------------------------------|
| Debug    | Stream messages to the editor sidebar.    |
| MQTT Out | Publish to an MQTT topic.                 |
| OPC UA   | Write to an OPC UA node.                  |

## Network

| Node                                | What it does                                                     |
|-------------------------------------|------------------------------------------------------------------|
| HTTP In / Response / Request        | Define HTTP endpoints in a flow, or call out to external HTTP services. |
| [TCP Receive](tcp-in.md)            | Listen on a TCP port (server) or dial a remote (client).         |
| [TCP Send](tcp-out.md)              | Reply on a session, broadcast to many, or dial a remote.         |
| [TCP Request](tcp-request.md)       | Synchronous TCP round-trip (dial → send → read → close).         |
| [UDP Receive](udp-in.md)            | Bind a UDP port (with optional multicast).                       |
| [UDP Send](udp-out.md)              | Send a UDP datagram (unicast / broadcast / multicast).           |

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

| Node          | What it does                              |
|---------------|-------------------------------------------|
| Parser JSON   | Encode / decode JSON.                     |
| Parser Modbus | Decode Modbus register frames.            |

## Diagnostics

| Node    | What it does                              |
|---------|-------------------------------------------|
| Status  | Inspect the status of another node.       |
| Catch   | Catch errors emitted by other nodes.      |
