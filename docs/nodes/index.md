# Nodes

!!! note "TODO"
    Per-node reference pages still need to be authored. Source material
    for many of them lives in `docs/issues/NODE_*.md` (internal specs).

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
