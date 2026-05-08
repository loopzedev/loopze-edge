# MQTT Publish (`mqtt-out`)

Publishes incoming flow messages to an MQTT topic on a configured broker.
Sink node — 1 input, 0 outputs.

| Inputs | Outputs |
|--------|---------|
| 1      | 0       |

## Publish target

`target = topic` (default) — publish to the configured `topic`, falling back
to `msg.topic`.

`target = responseTopic` — publish to **`msg.responseTopic`** (the v5 Response
Topic delivered by an inbound `mqtt-in`, typically forwarded from a remote
[`mqtt-request`](mqtt-request.md)). The configured `topic` and `msg.topic` are
ignored. `msg.correlationData` is forwarded automatically as the v5 Correlation
Data property — that's what closes the request/response round-trip.

| Mode            | Effective topic                          | Correlation forwarding |
|-----------------|------------------------------------------|------------------------|
| `topic`         | `config.topic` → fallback `msg.topic`    | only if `msg.correlationData` is set |
| `responseTopic` | `msg.responseTopic` only                 | always (when present) |

If `target=responseTopic` and `msg.responseTopic` is empty/missing, the node
sets a catchable error and discards the message.

## Configuration

| Field                      | Default | Description                                                                                |
|----------------------------|---------|--------------------------------------------------------------------------------------------|
| `broker`                   | —       | ID of the referenced [`mqtt-broker`](mqtt-in.md#broker-config-node) config node.           |
| `target`                   | `topic` | `topic` or `responseTopic` (see above).                                                    |
| `topic`                    | —       | Static publish topic (Topic mode only). Empty → fallback to `msg.topic`.                   |
| `qos`                      | `0`     | Publish QoS. `msg.qos` does **not** override (mqtt-out always uses the configured QoS).    |
| `retain`                   | `false` | Retain flag — broker keeps the last value for late subscribers.                            |
| `defaultUserProperties`    | —       | v5: default User Properties; merged with `msg.userProperties` (msg keys win on conflict).  |
| `defaultContentType`       | —       | v5: e.g. `application/json`. Overridden by `msg.contentType`.                              |
| `defaultResponseTopic`     | —       | v5: default Response Topic. Overridden by `msg.responseTopic`.                             |
| `defaultMessageExpiry`     | `0`     | v5: seconds. `0` = unset. Overridden by `msg.messageExpiry`.                               |
| `defaultPayloadFormat`     | `0`     | v5: `0` bytes / `1` UTF-8 text. Overridden by `msg.payloadFormat`.                         |

## Payload encoding

| `msg.payload` type     | Sent as                                    |
|------------------------|--------------------------------------------|
| `string`               | UTF-8 bytes of the string.                 |
| `[]byte`               | Raw bytes.                                 |
| `[]int` / `[]any` of numbers | Each element clamped to a byte — round-trip-safe with [`mqtt-in` buffer mode](mqtt-in.md#output-formats). |
| `map` / `slice`        | `json.Marshal`.                            |
| `nil`                  | Empty body.                                |

## v5 message overrides

| Field                  | Effect                                                                              |
|------------------------|-------------------------------------------------------------------------------------|
| `msg.userProperties`   | Merged with `defaultUserProperties` — msg keys win, non-overlapping keys survive.   |
| `msg.contentType`      | Overrides `defaultContentType`.                                                     |
| `msg.responseTopic`    | Overrides `defaultResponseTopic` (Topic mode); is THE topic in responseTopic mode.  |
| `msg.correlationData`  | Set as v5 Correlation Data. Accepts `[]byte`, base64 string, or `[]int` / `[]any`.  |
| `msg.messageExpiry`    | Overrides `defaultMessageExpiry`. Seconds.                                          |
| `msg.payloadFormat`    | Overrides `defaultPayloadFormat`. `0` bytes / `1` UTF-8 text.                       |

`msg.correlationData` is also accepted as a base64-encoded string — Go's
default `[]byte → JSON` encoding is base64, so messages that travelled
through a function node or a JSON round-trip carry the field as a string.
The node decodes back to the original bytes; if decoding fails, the string's
literal bytes are sent.

## Status

- Green / yellow / red — inherited from the broker connection.
- Red on publish error (broker disconnected, payload conversion failure).

## Examples

**Plain telemetry publish.**

```
[Inject 5s] → [Function: build payload] → [mqtt-out target=topic topic=sensor/heartbeat qos=1 retain]
```

**Reply to a request from a paired `mqtt-request`.**

```
[mqtt-in topic=rpc/devices/+/getState] → [Function: handle] → [mqtt-out target=responseTopic qos=1]
```

The function node only needs to set `msg.payload`. `mqtt-out target=responseTopic`
reads `msg.responseTopic` and `msg.correlationData` from the inbound
publish and pairs the response back to the original requester.

## See also

- [MQTT Subscribe](mqtt-in.md) — companion source node.
- [MQTT Request](mqtt-request.md) — full request/response pattern in one node.
