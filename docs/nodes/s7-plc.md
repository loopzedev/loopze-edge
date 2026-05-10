# S7 PLC config node (`s7-plc`)

Connection-config node for a SIEMENS S7 PLC over RFC1006 / ISO-on-TCP
(port 102). Holds host, port, rack/slot, manages the [`gos7`](https://github.com/robinson/gos7)
client lifecycle, exposes the negotiated PDU size and the reconnect state to
every read/write node that references it, and powers the **Test Connection**
button in the configuration panel.

`s7-plc` is a **config node** — no canvas element. It is created from the "+"
button next to the PLC dropdown on any S7 read/write node and stored in the
workspace alongside the flows. Multiple S7 nodes that reference the same PLC
share **one** TCP connection, mirroring how real S7 PLCs serialise access
per connection.

## Configuration

| Field             | Default | Description |
|-------------------|---------|-------------|
| `name`            | —       | Display name. Used in topics (`s7/<name>/...`) and as the PLC label in dropdowns. |
| `host`            | —       | PLC IP address or hostname. |
| `port`            | `102`   | TCP port. The bundled [demo PLC](#demo-plc) uses `1102` to avoid `sudo`. |
| `connectionType`  | `s7-1200-1500` | CPU family preset — drives the rack/slot defaults. |
| `rack` / `slot`   | derived | Manual override (only honoured when `connectionType=custom`). |
| `timeout`         | `5000`  | Connect / read / write timeout in **milliseconds**. |
| `reconnectDelay`  | `2000`  | Backoff between reconnect attempts after the link drops, in **milliseconds**. |

### Connection types

| Preset           | Rack | Slot | When to use |
|------------------|------|------|-------------|
| `s7-1200-1500`   | 0    | 1    | S7-1200 and S7-1500 CPUs (default). |
| `s7-300-400`     | 0    | 2    | S7-300, S7-400, ET 200S CPU. |
| `logo`           | 1    | 0    | LOGO! 0BA7/0BA8 and S7-200 Smart. |
| `custom`         | —    | —    | Anything else — set `rack` / `slot` manually. |

## Optimized DBs (TIA Portal)

S7-1200 and S7-1500 CPUs default to **Optimized block access** for new DBs.
Optimized DBs are **not** reachable via the S7 wire protocol — only via OPC
UA with symbolic addressing. To use the S7 nodes, un-tick *Optimized block
access* on each DB you want to address with classical wire forms
(`DB10.DBD0`, `M0.0`, …).

The address parser explicitly rejects symbolic forms (`DB1.MotorSpeed`)
with a hint pointing to OPC UA, so the misconfiguration surfaces at deploy
rather than as a confusing runtime error.

## Test Connection

The PLC config panel exposes a **Test Connection** button that performs a
round-trip Connect → `GetCpuInfo` → `GetOrderCode` → Disconnect against the
configured host. The result includes the PLC's reported firmware version and
order code so you can confirm the right CPU is reachable before deploying
the flows.

The endpoint is intentionally tolerant of placeholder strings returned by
software simulators (the bundled [demo PLC](#demo-plc) returns Snap7
defaults like `"Snap7-Server"` instead of a real article number).

## Status propagation

The PLC's connection state is propagated to **every** referencing node's
status pill:

- Green — `connected · <pdu> B PDU`
- Yellow — `connecting...` / `reconnecting...`
- Red — `disconnected: <reason>`

A flow with five `s7-read` nodes against the same PLC sees all five turn
red simultaneously when the link drops, and all five recover together on
the next successful reconnect.

## Demo PLC

The repo bundles a `python-snap7`-based demo PLC at
[`demo/s7-server/`](https://github.com/loopzedev/loopze-edge/tree/main/demo/s7-server)
with pre-filled DB1 / DB2 / DB3 / DB10 plus animated values for cyclic
polling tests. Run it via `make demo-s7`; stop with `make demo-s7-stop`.

The demo PLC carries two compatibility patches over upstream `python-snap7`
1.4 — see its [README](https://github.com/loopzedev/loopze-edge/tree/main/demo/s7-server#compatibility-patches-mainpy)
for the details. Both are scoped to the demo and do not affect the production
client.

## See also

- [S7 Read](s7-read.md) — cyclic / on-demand / block-mode reads.
- [S7 Write](s7-write.md) — variable, msg-driven, or block-mode writes.
- [S7 Parser](s7-parser.md) — decode block-mode bytes into structured objects (and back).
