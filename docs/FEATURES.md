# LOOPZE — Features & differentiation from Node-RED

LOOPZE is not a fork but a rebuild informed by the lessons from Node-RED. Same philosophy (visual flow programming), but with deliberate design decisions that solve recurring pain points.

---

## Message models via Function nodes

In Node-RED, messages are untyped `msg` objects — every node can set or omit arbitrary fields. In larger flows this quickly leads to inconsistent payloads that only show up at runtime.

In LOOPZE, Function nodes can **define models**: a JSON Schema describing what the outgoing message looks like. As soon as a model is defined, the Function node guarantees structurally consistent outputs. Downstream nodes can rely on which fields exist and what type they have — no more defensive `if (msg.payload && msg.payload.temperature)`.

---

## Validation node

A dedicated **Validation node** checks incoming messages against a defined model. Messages matching the schema are forwarded, non-conforming messages are dropped (or routed to a separate error output). This enables explicit data contracts between flow sections — especially valuable at system boundaries where external data (MQTT, HTTP, sensors) comes in and should not be trusted blindly.

---

## Native queuing via NATS Streams

Node-RED has no built-in queuing. Anyone wanting to buffer, retry or persistently cache messages needs external systems (Redis, RabbitMQ) or fragile workarounds with context variables.

LOOPZE ships with an embedded NATS server with JetStream. A native **Queue node** can write messages into a NATS Stream and consume them again with configurable delivery guarantees (at-least-once, exactly-once). Retry logic, dead-letter queues and backpressure are part of the base toolkit — no external broker, no plugin, a single binary.

---

## Node profiling & throughput metrics

Node-RED offers no way to see which node takes how long or where a bottleneck sits. You notice that something is slow, but not where.

LOOPZE measures **per-node processing time and throughput**. In the editor a profiling overlay can be toggled on that shows directly on the nodes: average latency, messages per second, queue fill level. Slow nodes are highlighted visually. This makes performance problems visible before they become critical — without external monitoring tools.

---


## Goroutine-per-node parallelism ✅

> Already implemented — every node runs in its own goroutine (`engine.go: nodeLoop`).

Node-RED runs single-threaded on Node.js. A slow Function node blocks the entire event loop — all other nodes wait.

LOOPZE runs **every node in its own goroutine**. CPU-intensive computation in a Function node does not block any other nodes. The Go runtime distributes the work automatically across all CPU cores. Thousands of nodes run truly in parallel, not cooperatively-sequentially.

---

## Single binary, zero dependencies ✅

> Already implemented — Go binary with embedded frontend (`web.Embed`) and embedded NATS server.

Node-RED needs Node.js, npm and a filesystem full of `node_modules`. On a fresh system, installation is a multi-step process with potential version conflicts.

LOOPZE is **a single executable file**. No Node.js, no npm, no external dependencies. Download, run, done. The frontend is embedded in the binary, the NATS server runs embedded. Especially on edge devices and in constrained environments (no internet, no package manager) this is a decisive advantage.

---

## Reactive context store — events instead of polling ✅

> Already implemented — Context Watch node based on the NATS KV watcher (`context_watch.go`).

In Node-RED, the context store (flow/global context) is a passive key-value store. You can read and write values, but there is no way **to be notified when a value changes**. This leads to a fundamental contradiction: Node-RED is an event-based platform, but the central state store is poll-based.

In practice this forces anti-patterns:
- **Polling loops**: Inject nodes that read the context every 500 ms and check whether something changed — CPU load with no benefit
- **Redundant wiring**: Every node that changes a context value must additionally send a message to all interested nodes — duplicated logic, fragile flows
- **Race conditions**: Between two poll cycles a value can have been changed multiple times — intermediate states are lost

LOOPZE solves this with a **reactive context store on top of NATS JetStream KV**. The Context Watch node subscribes to changes on specific keys or key patterns and automatically fires a message whenever a value changes — in real time, without polling. The context becomes a fully-fledged event source:

```
[Sensor] → [Change: set flow.temperature]
                                            → Context Watch (flow.temperature) → [Debug]
[HTTP In] → [Change: set flow.temperature]  ↗
```

No matter which node changes the value — the Context Watch reacts immediately. This eliminates polling completely and keeps flows cleanly event-based.

---

## Native industrial connectors — no community roulette

Node-RED ships no industrial protocols out of the box. OPC-UA, Modbus, S7, MQTT with Sparkplug B — everything has to be installed via community modules. This works initially but leads to serious problems in practice:

- **Orphaned modules**: The maintainer loses interest, the module no longer receives updates. Security holes stay open, compatibility with new Node-RED versions breaks.
- **Quality variance**: For the same protocol there are often 3–5 modules with differing maturity, documentation and error handling. Choosing becomes a gamble.
- **Dependency chains**: Community modules bring their own npm dependencies that can collide with other modules. An `npm install` can break existing flows.
- **No unified config pattern**: Every module invents its own UI for connection settings. Sometimes there is reconnect logic, sometimes not. Sometimes credentials are encrypted, sometimes stored in plain text.

LOOPZE solves this with **native industrial connectors firmly anchored in the product**:

| Protocol | Type | Description |
|---|---|---|
| **MQTT** | Data Connector | v3.1.1 and v5, shared broker connections |
| **OPC-UA** | Industrial | Client for PLC and SCADA integration |
| **Modbus** | Industrial | TCP/RTU, read/write coils and registers |
| **HTTP** | Data Connector | Request/response and webhook endpoints |
| **TCP/UDP** | Data Connector | Raw socket communication |
| **S7** | Industrial | Siemens S7 protocol for S7-300/400/1200/1500 |
| **Databases** | Storage | PostgreSQL, SQLite, InfluxDB |

All connectors are maintained alongside product development — same test coverage, same release cycles, same quality standards. They use the unified **config node plugin system** (shared connections, automatic reconnect, status broadcast), so all connectors behave consistently. No `npm install` that becomes a risk after 18 months.

---

## Data pipelines — the Telegraf approach as a visual flow

Node-RED is built for event-based flows with individual messages. As soon as large data volumes have to be processed — batch imports, CSV files with 100k rows, database dumps, log aggregation — it hits its limits. The single-threaded event loop blocks, the editor UI freezes, MQTT subscriptions miss messages because the runtime is saturated.

Tools like **Telegraf** solve this elegantly with an input → processing → output pipeline. But Telegraf is configuration-driven (TOML files) — no visual representation, no quick experimentation, no conditional logic.

LOOPZE bridges both worlds: **Telegraf-style data pipelines as visual flows**. The key to this is a native **Processing node that executes in Go** — not in an interpreted sandbox like the JavaScript Function node, but directly in the runtime's language. This unlocks full use of all CPU cores, zero-copy data processing and access to the Go ecosystem.

```
                    Data Pipeline Flow
┌─────────────────────────────────────────────────────────┐
│                                                          │
│  [CSV Input]  →  [Go Transform]  →  [InfluxDB Output]   │
│   100k rows       map/filter/         batch write        │
│   streaming       aggregate           5k rows/s          │
│                                                          │
│  [SQL Query]  →  [Go Transform]  →  [MQTT Publish]      │
│   SELECT *        reshape/enrich      fan-out            │
│                                                          │
└─────────────────────────────────────────────────────────┘
```

**Core concept — Go Transform node:**
- Processing in native Go instead of interpreted JavaScript
- Streaming-capable: processes data row by row instead of loading everything into memory
- Batch operations: aggregation, windowing, group-by as built-in primitives
- Parallel processing: a single Go Transform can scale internally across multiple goroutines

**Input nodes** (data sources):
- File input (CSV, JSON Lines, Parquet)
- SQL query (PostgreSQL, SQLite)
- HTTP bulk fetch
- MQTT retained bulk read

**Processing nodes** (Go-native):
- Go Transform — map, filter, reduce with Go syntax
- Aggregate — windowed aggregation (sum, avg, min, max, count)
- Join — merge two streams (inner, left, outer)
- Batch — group messages into configurable batches

**Output nodes** (sinks):
- InfluxDB / TimescaleDB batch write
- File output (CSV, JSON)
- SQL insert/upsert
- MQTT bulk publish

The result: data processing pipelines that take minutes in Node-RED (or crash the runtime) run in seconds in LOOPZE — visually configured, not hidden in TOML files.

---

## Live debugging with message tracing

Node-RED's Debug node shows messages in a separate sidebar — but you cannot see the path a message took through the flow.

LOOPZE enables **message tracing**: a single message can be visually tracked through the flow. The path the message took is highlighted on the canvas, with timestamps and payload snapshots at every node. This makes debugging complex flows with branches, filters and cross-flow links comprehensible.


