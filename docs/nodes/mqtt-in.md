# MQTT Subscribe (`mqtt-in`)

Subscribes to one or more MQTT topics on a configured broker and emits one
flow message per received publish. The first node of LOOPZE's MQTT trio,
backed by the v5 client (`paho.golang/autopaho`).

| Inputs                  | Outputs |
|-------------------------|---------|
| 0 (static), 1 (dynamic) | 1       |

## Modes

`static` (default) subscribes to the configured `topic` on deploy and stays
subscribed until stop / re-deploy. The node has no input port.

`dynamic` adds an input port and starts with **no** active subscription. The
flow drives the subscriptions at runtime by sending control messages with
`msg.action = "subscribe"`. Every control message **replaces** the full set of
active subscriptions for this node — there is no incremental add/remove. An
empty payload clears the subscriptions.

```
[Inject {payload: "sensor/+/temp"}] → [mqtt-in dynamic]
[Inject {payload: ["a/b", "c/d"]}]  → [mqtt-in dynamic]   // two topics
[Inject {payload: ""}]              → [mqtt-in dynamic]   // unsubscribe all
```

Control messages are **not** forwarded to the output. The output emits only
real MQTT publishes.

## Configuration

| Field                     | Default  | Description                                                                                          |
|---------------------------|----------|------------------------------------------------------------------------------------------------------|
| `broker`                  | —        | ID of the referenced [`mqtt-broker` config node](#broker-config-node).                               |
| `mode`                    | `static` | `static` (subscribe at deploy) or `dynamic` (subscribe via control msgs).                            |
| `topic`                   | —        | Static-mode topic. Wildcards `+` (one segment) and `#` (rest of path) are supported.                 |
| `qos`                     | `0`      | 0 (at most once), 1 (at least once), 2 (exactly once). In dynamic mode, `msg.qos` overrides per call.|
| `outputFormat`            | `string` | How `msg.payload` is decoded. See [Output formats](#output-formats).                                 |
| `noLocal`                 | `false`  | v5: don't deliver our own publishes back to us. Useful when a flow publishes to a topic it also subs.|
| `retainAsPublished`       | `false`  | v5: forward the original retain flag unchanged on delivery.                                          |
| `retainHandling`          | `0`      | v5: `0` always send retained, `1` only on new sub, `2` never.                                        |
| `subscriptionIdentifier`  | `0`      | v5: numeric ID echoed by the broker on every matching publish (0 = unset).                           |
| `subscribeUserProperties` | —        | v5: SUBSCRIBE-packet user properties (rarely used).                                                  |

In dynamic mode `msg.noLocal`, `msg.retainAsPublished`, `msg.retainHandling`,
`msg.subscriptionIdentifier` override the configured defaults per call.

## Output formats

| Mode     | `msg.payload` shape                                                   |
|----------|------------------------------------------------------------------------|
| `string` | UTF-8 string — non-UTF-8 bytes are preserved byte-identical.           |
| `json`   | Structured value (`map`/`[]any`/scalar). On parse error: falls back to `string` and sets `msg.parseError`. |
| `buffer` | `[]int` — JSON-roundtrip-safe (unlike `[]byte`, which JSON-encodes as base64). [`mqtt-out`](mqtt-out.md) recognises the array and reconstructs the bytes. |

## Outgoing message

```json
{
  "topic": "sensor/temperature",
  "payload": "23.4",
  "qos": 0,
  "retain": false,
  "userProperties": { "source": "sensor-42" },
  "contentType": "application/json",
  "responseTopic": "sensor/temperature/reply",
  "correlationData": "<bytes>",
  "messageExpiry": 60,
  "payloadFormat": 1,
  "subscriptionIdentifier": 42
}
```

The v5 fields are only set when present in the inbound publish. With a v3.1.1
broker they are always missing.

## Status

- Green — `connected · <topic>` (static) or `connected · n topic(s)` (dynamic).
- Yellow — `connecting...` / `reconnecting...`.
- Red — `disconnected` plus the broker error message.

## Broker connection sharing

All MQTT nodes that reference the same `mqtt-broker` config share **one** TCP
connection (= one Client ID). This affects two v5 features:

- `noLocal` filters publishes from the **same Client ID**, so it can drop
  responses you are expecting from a paired publisher in the same LOOPZE
  instance. Default `false` is the safe choice.
- Shared subscriptions (`$share/<group>/<topic>`) work transparently — the
  broker handles fan-out across instances.

## Broker config node

`mqtt-broker` is a **config node** — no canvas element, only configuration.
It is created from the "+" button next to the broker dropdown on any MQTT
node and stored in the workspace alongside the flows. Multiple MQTT nodes
that reference the same broker share one TCP connection (= one Client ID).

The broker configuration holds host, port, credentials, TLS, keep-alive,
clean-start, session-expiry, plus three optional presence messages:

- **onConnect** — published by the LOOPZE client immediately after CONNACK.
- **onDisconnect** — published by the client before a clean DISCONNECT.
- **LastWill** — passed in CONNECT; published by the **broker** when the
  client drops uncleanly (timeout / lost connection). Optional v5 will-delay.

## See also

- [MQTT Publish](mqtt-out.md) — companion sink node.
- [MQTT Request](mqtt-request.md) — synchronous request/response over MQTT v5.
