# Core concepts

A short tour of the moving parts in a LOOPZE runtime.

## Flows, nodes, wires

A **flow** is a directed graph of **nodes** connected by **wires**. Each
node has zero or more input ports and one or more output ports. Messages
travel along wires from output to input.

Flows live in tabs in the editor. A LOOPZE instance can run many flows in
parallel; each flow is independent unless wired together via Link nodes
or NATS.

## Messages

A message is a JSON-compatible object that flows along wires. By
convention it carries:

- `payload` — the primary data of interest.
- `topic` — an optional routing key.
- additional fields any node may attach.

!!! info "Models"
    Function nodes can declare a JSON-Schema **model** for their output.
    Downstream nodes can rely on the shape of the message instead of
    defensively checking every field.

## The runtime

Every node runs in its own goroutine. CPU-heavy work in a Function node
does not block any other node — the Go runtime spreads execution across
all CPU cores.

## Deploy

The editor is a *design surface*; the runtime is what actually executes
flows. **Deploy** snapshots the editor state and tells the runtime to
swap in the new graph. LOOPZE supports:

- **Full deploy** — restart everything.
- **Modified flows deploy** — only restart flows that changed.
- **Partial deploy** — restart only the nodes that changed.

## Persistence

Three things live on disk under `--data-dir`:

| File / dir              | What it stores                              |
|-------------------------|---------------------------------------------|
| `flows.json`            | The flow graph (nodes + wires + tabs)       |
| `credentials.json`      | Encrypted secrets (AES-256-GCM)             |
| `users.json`            | Local user accounts (Argon2id-hashed)       |

The encryption key for credentials is stored in a separate file
(`*.key`) — never commit it.

## NATS

An embedded NATS server runs inside every LOOPZE instance. It powers:

- Context storage (`flow.set` / `flow.get` etc.).
- Debug streams to the editor.
- Future: fleet communication between LOOPZE instances.
