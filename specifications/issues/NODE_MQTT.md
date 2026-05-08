# Issue: MQTT Nodes – Subscribe, Publish & Request/Response with Broker Configuration

## Status: Open

## Problem Description

LOOPZE needs its first **Data Connector** — MQTT. Three new node types (`mqtt-in`, `mqtt-out`, `mqtt-request`) enable receiving, sending, and request/response patterns over MQTT. Central to this is the concept of a **broker configuration**, managed as a standalone, reusable entity. Each MQTT node references exactly one broker, but different brokers can be configured across multiple nodes.

This issue simultaneously introduces the new concept of **Config Nodes** — configurable entities that do not appear on the canvas but can be referenced by multiple nodes (e.g., server connections, authentication). The MQTT broker is the first config node in LOOPZE.

**Scope**: **MQTT v5 is mandatory.** The broker client must speak v5. v3.1.1 remains available as an alternatively selectable protocol version — the user picks it deliberately per broker config, there is no automatic fallback. Focus is on a working broker configuration and instantiation. The subscribe/publish configuration is intentionally minimal — v5-specific features (User Properties, Message Expiry, Shared Subscriptions, Response Topic, Correlation Data) are supported in a lean first tier. The request/response pattern relies on the v5 `Response Topic` + `Correlation Data` properties; with a v3.1.1 broker the request node degrades (see § 4).

## Overview

| Node Type | Type ID | Canvas Inputs | Canvas Outputs | Description |
|---|---|---|---|---|
| **MQTT Subscribe** | `mqtt-in` | 0 | 1 | Receives messages from an MQTT broker via subscription |
| **MQTT Publish** | `mqtt-out` | 1 | 0 | Sends messages to an MQTT broker (configurable target: fixed topic or `msg.responseTopic`) |
| **MQTT Request** | `mqtt-request` | 1 | 1 | Sends a request publish with a temporary response topic and correlation data, emits the matching response (or timeout) on the output |

```
                          MQTT Broker (external)
                          ┌──────────────┐
Flow A                    │              │
┌─────────────────────┐   │  topic/data  │
│                      │   │      │       │
│  [MQTT In] ←─────────────┘      │       │
│      ↓               │          │       │
│  [Function]          │          │       │
│      ↓               │          │       │
│  [Debug]             │          │       │
│                      │          │       │
└─────────────────────┘          │       │
                                  │       │
Flow B                            │       │
┌─────────────────────┐          │       │
│                      │          │       │
│  [Inject]            │          │       │
│      ↓               │          │       │
│  [MQTT Out] ──────────────────→│       │
│                      │   │              │
└─────────────────────┘   │              │
                          │              │
Flow C — request side     │              │
┌──────────────────────┐  │              │
│                       │  │              │
│  [Inject] → [MQTT Request]──→ rpc/foo  │
│                ↑    ↓ │  │              │
│                │    └─────── loopze/    │
│                │      │  │   response/  │
│                │      │  │   <uuid>     │
│             [Debug]   │  │              │
└──────────────────────┘  │              │
                          │              │
Flow D — responder side   │              │
┌──────────────────────┐  │              │
│                       │  │              │
│  [MQTT In rpc/foo] ──────┘              │
│         ↓             │                 │
│  [Function: handle]   │                 │
│         ↓             │                 │
│  [MQTT Out target=    │                 │
│   responseTopic] ─────────→ loopze/    │
│                       │   response/    │
└──────────────────────┘   <uuid>         │
                          └──────────────┘
```

## Requirements

### 1. Config Node: MQTT Broker (`mqtt-broker`)

The MQTT broker is a **config node** — it does not appear on the canvas but is managed as a standalone configuration in the workspace and referenced by `mqtt-in`/`mqtt-out` nodes.

- **Type ID**: `mqtt-broker`
- **No canvas element** — purely configurational
- **Configuration fields**:
  - `name` (string) — display name in the dropdown, e.g., "Production Broker"
  - `host` (string) — hostname or IP, e.g., "mqtt.example.com"
  - `port` (number) — default: 1883
  - `clientId` (string) — client ID, default: auto-generated (`loopze-<random>`)
  - `protocolVersion` (string) — `5` (default) or `3.1.1`. Must be deliberately chosen by the user — no automatic fallback
  - `username` (string, optional) — username
  - `password` (string, optional) — password
  - `keepalive` (number) — keep-alive interval in seconds, default: 60
  - `cleanStart` (boolean) — Clean Start (v5) or Clean Session (v3.1.1) flag, default: true
  - `sessionExpiry` (number, v5) — Session Expiry Interval in seconds, default: 0 (session ends on disconnect). Ignored in v3.1.1 fallback
  - `useTLS` (boolean) — enable TLS, default: false
  - **onConnect Message** — client-side convention: sent by the LOOPZE client immediately after a successful CONNACK as a regular PUBLISH (typical "online" status). Optional, all fields empty = no message:
    - `onConnectTopic` (string)
    - `onConnectPayload` (string)
    - `onConnectQoS` (number) — 0, 1, or 2. Default: 0
    - `onConnectRetain` (boolean) — default: false
  - **onDisconnect Message** — client-side convention: sent by the LOOPZE client before a **regular** disconnect (Stop / Re-Deploy) as a regular PUBLISH, before the DISCONNECT packet goes out. Optional:
    - `onDisconnectTopic` (string)
    - `onDisconnectPayload` (string)
    - `onDisconnectQoS` (number) — default: 0
    - `onDisconnectRetain` (boolean) — default: false
  - **LastWill** — MQTT protocol feature: passed in the CONNECT packet to the broker and published by the **broker** when the client **uncleanly** drops (keep-alive timeout, connection loss without a clean DISCONNECT). Optional:
    - `lastWillTopic` (string)
    - `lastWillPayload` (string)
    - `lastWillQoS` (number) — 0, 1, or 2. Default: 0
    - `lastWillRetain` (boolean) — default: false
    - `lastWillDelayInterval` (number, v5) — delay in seconds before the broker publishes the LastWill. Default: 0. Ignored in v3.1.1 mode

  onConnect, onDisconnect, and LastWill are three separate messages with clearly separated triggers: **onConnect** after a successful connection, **onDisconnect** on a clean disconnect (sent by the client), **LastWill** on an unclean disconnect (sent by the broker).

- **Access to the properties dialog**:
  - **New broker**: Via the "+" button next to the broker dropdown in MQTT nodes
  - **Edit existing broker**: Via the "Edit broker config" link below the broker dropdown (only visible when a broker is selected). Opens the same dialog prefilled with the existing settings
- **Properties dialog**:

```
┌──────────────────────────────────────────────┐
│  MQTT Broker                                  │
├──────────────────────────────────────────────┤
│                                               │
│  Name                                         │
│  ┌────────────────────────────────────────┐   │
│  │ Production Broker                      │   │
│  └────────────────────────────────────────┘   │
│                                               │
│  Server                                       │
│  ┌──────────────────────────┐ ┌──────────┐   │
│  │ mqtt.example.com         │ │ 1883     │   │
│  └──────────────────────────┘ └──────────┘   │
│                  Host              Port        │
│                                               │
│  Client ID                                    │
│  ┌────────────────────────────────────────┐   │
│  │ loopze-abc123                           │   │
│  └────────────────────────────────────────┘   │
│                                               │
│  Credentials                                  │
│  ┌──────────────────────────────────────┐     │
│  │ Username                              │     │
│  └──────────────────────────────────────┘     │
│  ┌──────────────────────────────────────┐     │
│  │ ••••••••                              │     │
│  └──────────────────────────────────────┘     │
│                                               │
│  Protocol Version                             │
│  ┌────────────────────────────────────────┐   │
│  │ MQTT v5 (Default)                  ▼  │   │
│  └────────────────────────────────────────┘   │
│                                               │
│  ┌─────┐                                     │
│  │ TLS │  Keep-Alive: [60]s                   │
│  └─────┘  ☑ Clean Start                       │
│                                               │
│  Session Expiry: [0]s         (v5 only)       │
│                                               │
│  ▼ onConnect (after connect, optional)        │
│  Topic:   [status/loopze-abc123          ]     │
│  Payload: [online                       ]     │
│  QoS: [0 ▼]   ☐ Retain                        │
│                                               │
│  ▼ onDisconnect (clean disconnect)            │
│  Topic:   [status/loopze-abc123          ]     │
│  Payload: [offline                      ]     │
│  QoS: [0 ▼]   ☐ Retain                        │
│                                               │
│  ▼ LastWill (unclean disconnect, MQTT-spec)   │
│  Topic:   [status/loopze-abc123          ]     │
│  Payload: [offline                      ]     │
│  QoS: [0 ▼]   ☐ Retain   Delay: [0]s (v5)     │
│                                               │
│  ┌────────────┐  ┌────────────┐              │
│  │    Save     │  │   Cancel   │              │
│  └────────────┘  └────────────┘              │
└──────────────────────────────────────────────┘
```

### 2. MQTT Subscribe Node (`mqtt-in`)

- **Canvas**:
  - Static Mode: 0 inputs, 1 output (source node)
  - Dynamic Mode: 1 input, 1 output — the input only serves control (subscribe / clear), not data forwarding
- **Function**: Connects via the configured broker and forwards received MQTT messages as flow messages. The subscription can be either fixed in the config or controlled at runtime via input message.
- **Base configuration**:
  - `broker` (string) — ID of the referenced `mqtt-broker` config node
  - `mode` (string) — `static` (default) or `dynamic`
  - `topic` (string) — MQTT topic to subscribe to, e.g., `sensor/temperature` (only relevant in static mode)
  - `qos` (number) — Quality of Service: 0, 1, or 2. Default: 0
  - `outputFormat` (string) — format in which `msg.payload` is delivered to the output:
    - `string` (default) — the MQTT payload is passed through as a Go string (raw bytes interpreted as UTF-8; non-UTF-8 bytes are preserved byte-identical but may not be printable as strings)
    - `json` — the payload is parsed as JSON; the result is a structured value (map/array/number/bool/null). If parsing fails, falls back to `string` and a `msg.parseError` with the error message is set (the message still goes out)
    - `buffer` — the payload is passed through as a number array (`[]int`), e.g., `[222, 173, 190, 239]` for the bytes `0xDE 0xAD 0xBE 0xEF`. Useful for binary data (images, Protobuf, MessagePack, etc.). Note: deliberately not `[]byte`, because Go's `encoding/json` serializes `[]byte` as a base64 string, which is unreadable in the debug viewer and loses type info on JSON round-trips. The mqtt-out node recognizes the number array on publish and reconstructs the original bytes.

- **MQTT v5 Subscription Options** (all optional, apply per subscription; ignored in v3.1.1 mode):
  - `noLocal` (boolean, default `false`) — prevents the broker from delivering the client's own publishes on the same topic back to it. Useful against echo loops when a LOOPZE flow publishes to a topic it also subscribes to
  - `retainAsPublished` (boolean, default `false`) — when `true`, the retain flag of the original publish is forwarded unchanged. When `false` (default), the broker sets the flag to `0` on delivery — consumers can then no longer distinguish whether the message was retained
  - `retainHandling` (number, default `0`) — controls when retained messages are sent on subscribe:
    - `0`: send all retained messages on every subscribe (default behavior)
    - `1`: only send retained messages if the subscription is new (no re-send on re-subscribe after reconnect with session)
    - `2`: never send retained messages on subscribe
  - `subscriptionIdentifier` (number, optional) — numeric ID returned by the broker on every publish that matches this subscription. Useful in dynamic mode with multiple parallel subscriptions to identify the source of a message

- **MQTT v5 SUBSCRIBE Properties** (all optional, once per SUBSCRIBE packet):
  - `subscribeUserProperties` (object) — string-to-string map, sent as User Properties on the SUBSCRIBE packet. Rarely used; some brokers evaluate them for authorization hooks

- **Outgoing message** (for each received MQTT message):
  ```json
  {
    "topic": "sensor/temperature",
    "payload": "<received data>",
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
  The v5 fields are only set when present in the incoming MQTT message. In v3.1.1 mode they are always missing.
  - `userProperties` (object) — string-to-string map with the User Properties from the PUBLISH packet
  - `contentType` (string) — e.g., `application/json`, `text/plain`
  - `responseTopic` (string) — topic to which a reply should be published (request/response pattern)
  - `correlationData` (bytes) — opaque bytes for correlating request and response
  - `messageExpiry` (number, seconds) — remaining lifetime of the message; on receipt after expiry the broker would have discarded it anyway
  - `payloadFormat` (number, 0 or 1) — `0` = unspecified/bytes, `1` = UTF-8 text. Hint to consumers for decoding
  - `subscriptionIdentifier` (number) — the ID set on subscribe. With Shared Subscriptions or multiple overlapping subscriptions the broker may return multiple — we then deliver a `[]number` array

- **Shared Subscriptions (v5)**: Topic patterns `$share/<group>/<topic>` are supported transparently — the MQTT library forwards them as a regular subscription to the broker, which handles load distribution. In v3.1.1 fallback such a topic would be subscribed literally, so usage is bound to v5.

#### Static Mode (Default)

- On deploy / start the node subscribes to the pattern configured in `topic`.
- Subscription stays active for the entire lifetime of the node.
- Input port is not present.

#### Dynamic Mode

- On deploy / start the node has **no** active subscriptions — it waits for control messages.
- Control via `msg.action`:
  - `msg.action = "subscribe"` → the topics specified in `msg.payload` are subscribed:
    - `msg.payload` as **string** → a single topic
    - `msg.payload` as **string array** → multiple topics
  - **On every `subscribe` all existing subscriptions of the node are unsubscribed first**, then the new topics are subscribed. So there is always only the latest state.
- Incoming control messages are **not** passed through on the output — the output delivers exclusively received MQTT messages.
- Topics that are already active in the same `subscribe` call may continue without interruption (implementation: diff `old → new`, only un-/subscribe differences — optimization, not required for v1).
- An empty array or empty string acts as "delete all subscriptions".
- **QoS override per message:** `msg.qos` (number, valid values 0/1/2) overrides the QoS configured in the config for this `subscribe` call. If `msg.qos` is missing or out of range 0–2, the configured default is used. The QoS applies uniformly to all topics subscribed with the control message.

- **Status display** (via `SetStatus`):
  - Green: "connected" — broker connection is up
    - Static: format `connected · <topic>`
    - Dynamic: format `connected · <n> topic(s)` (or "connected · idle" when none active)
  - Yellow: "connecting..." — connection establishment in progress
  - Red: "disconnected" / error message — connection failed

- **Properties panel**:

```
┌──────────────────────────────────────────────┐
│  MQTT Subscribe                               │
├──────────────────────────────────────────────┤
│                                               │
│  Broker                                       │
│  ┌────────────────────────────────┐ ┌───┐    │
│  │ Production Broker          ▼  │ │ + │    │
│  └────────────────────────────────┘ └───┘    │
│  Edit broker config                           │
│                                               │
│  Mode                                         │
│  ( • ) Static    ( ) Dynamic (msg.action)     │
│                                               │
│  Topic                          (Static only) │
│  ┌────────────────────────────────────────┐   │
│  │ sensor/temperature                     │   │
│  └────────────────────────────────────────┘   │
│                                               │
│  QoS                                          │
│  ┌────────────────────────────────────────┐   │
│  │ 0 - At most once                   ▼  │   │
│  └────────────────────────────────────────┘   │
│                                               │
│  ▼ MQTT v5 Subscription Options (optional)    │
│  ☐ No Local                                   │
│  ☐ Retain As Published                        │
│  Retain Handling: [0 — send always         ▼]│
│  Subscription ID: [          ] (leave empty)  │
│                                               │
│  ▼ MQTT v5 SUBSCRIBE Properties (rare)        │
│  User Properties:                             │
│  ┌──────────────┐ ┌──────────────┐ ┌───┐     │
│  │ key          │ │ value        │ │ × │     │
│  └──────────────┘ └──────────────┘ └───┘     │
│  [+ Add property]                             │
│                                               │
└──────────────────────────────────────────────┘
```

In dynamic mode the topic field is hidden and a hint block is shown below the mode selector:

```
ℹ Send msg.action = "subscribe" with msg.payload as
   topic string or array of topics. Existing
   subscriptions are replaced on each call.
```

In dynamic mode the v5 subscription options can additionally be overridden per `msg`:
- `msg.noLocal` (boolean), `msg.retainAsPublished` (boolean), `msg.retainHandling` (number 0/1/2), `msg.subscriptionIdentifier` (number)
- If the field is missing in the control message, the config value applies.

### 3. MQTT Publish Node (`mqtt-out`)

- **Canvas**: 1 input, 0 outputs (sink node)
- **Function**: Publishes incoming messages via the configured broker. The target topic is either a configured/`msg.topic` value (default) or the `msg.responseTopic` carried by an inbound request — see `target` below.
- **Base configuration**:
  - `broker` (string) — ID of the referenced `mqtt-broker` config node
  - `target` (string) — `topic` (default) or `responseTopic`:
    - `topic` — publishes to the configured `topic`, falling back to `msg.topic` (current default behavior)
    - `responseTopic` — publishes to `msg.responseTopic` (the v5 Response Topic of an inbound request, typically delivered via `mqtt-in` § 2 or paired with `mqtt-request` § 4 on the requester side). The configured `topic` and `msg.topic` are **ignored** in this mode. If `msg.responseTopic` is missing or empty the node sets the catchable error `"mqtt-out: target=responseTopic but msg.responseTopic is missing"` and discards the message. When the inbound message also carries `msg.correlationData`, it is automatically forwarded as the v5 `Correlation Data` property on the publish — this is what pairs the response with the original request on the requester side. The mode requires v5 on the broker; with v3.1.1 the inbound `mqtt-in` will have no `responseTopic` at all and the node will always error
  - `topic` (string, optional) — MQTT topic to publish to. If empty, `msg.topic` is used. Ignored when `target = responseTopic`
  - `qos` (number) — Quality of Service: 0, 1, or 2. Default: 0
  - `retain` (boolean) — retain flag. Default: false

- **MQTT v5 Default Properties** (all optional, apply as defaults for every publish; can be overridden per `msg` — see below):
  - `defaultUserProperties` (object) — string-to-string map; merged with `msg.userProperties` (msg keys win on conflict)
  - `defaultContentType` (string) — e.g., `application/json`
  - `defaultResponseTopic` (string) — topic for replies (request/response pattern)
  - `defaultMessageExpiry` (number, seconds) — default expiry for every sent message
  - `defaultPayloadFormat` (number, 0 or 1) — `0` = bytes (default), `1` = UTF-8 text. If set to `1` and the payload is not a valid UTF-8 string, it is sent anyway (the broker may reject)

  **Deliberately not in the static config**:
  - `correlationData` is by definition per-message (request/response correlation) — only via `msg.correlationData`
  - `topicAlias` is managed transparently by the client (optimization in the broker manager) — not user-configurable
  - `subscriptionIdentifier` is only relevant in the PUBLISH **from the broker** to the subscriber, never in the outbound publish

- **Incoming message**:
  - `msg.payload` is sent as the MQTT payload
  - `msg.topic` is used as fallback topic when none is configured
  - **MQTT v5 (optional)** — the following fields override the default properties from the config (ignored in v3.1.1 mode):
    - `msg.userProperties` (object) — merged with `defaultUserProperties`; on the same key msg wins
    - `msg.contentType` (string) — overrides `defaultContentType`
    - `msg.responseTopic` (string) — overrides `defaultResponseTopic`
    - `msg.correlationData` (string/bytes) — no config default, message-only
    - `msg.messageExpiry` (number, seconds) — overrides `defaultMessageExpiry`
    - `msg.payloadFormat` (number, 0 or 1) — overrides `defaultPayloadFormat`
- **Status display**: Analogous to `mqtt-in`

- **Properties panel**:

```
┌──────────────────────────────────────────────┐
│  MQTT Publish                                 │
├──────────────────────────────────────────────┤
│                                               │
│  Broker                                       │
│  ┌────────────────────────────────────┐ ┌───┐│
│  │ Production Broker              ▼  │ │ + ││
│  └────────────────────────────────────┘ └───┘│
│  Edit broker config                           │
│                                               │
│  Publish target                               │
│  ( • ) Topic                                  │
│  (   ) Response to responseTopic (v5)         │
│                                               │
│  Topic                          (Topic mode)  │
│  ┌────────────────────────────────────────┐   │
│  │ actuator/command                       │   │
│  └────────────────────────────────────────┘   │
│  ℹ Falls back to msg.topic if empty.          │
│                                               │
│  ℹ When "Response to responseTopic" is set,   │
│    the topic field is ignored. The node       │
│    publishes to msg.responseTopic and         │
│    forwards msg.correlationData as the v5     │
│    Correlation Data property. Pair with an    │
│    mqtt-in carrying responseTopic / correlat- │
│    ionData from a remote mqtt-request.        │
│                                               │
│  QoS                                          │
│  ┌────────────────────────────────────────┐   │
│  │ 0 - At most once                   ▼  │   │
│  └────────────────────────────────────────┘   │
│                                               │
│  ☐ Retain                                     │
│                                               │
│  ▼ MQTT v5 Default Properties (optional)      │
│  Content Type    [application/json         ]  │
│  Response Topic  [                         ]  │
│  Message Expiry  [    ] sec                   │
│  Payload Format  [0 — bytes              ▼]   │
│                                               │
│  User Properties:                             │
│  ┌──────────────┐ ┌──────────────┐ ┌───┐     │
│  │ source       │ │ loopze-flow-1 │ │ × │     │
│  └──────────────┘ └──────────────┘ └───┘     │
│  [+ Add property]                             │
│                                               │
│  ℹ  msg.userProperties / msg.contentType /    │
│     msg.responseTopic / msg.messageExpiry /   │
│     msg.payloadFormat override the            │
│     defaults per message.                     │
│     msg.correlationData is message-only.      │
│                                               │
└──────────────────────────────────────────────┘
```

### 4. MQTT Request Node (`mqtt-request`)

- **Canvas**: 1 input, 1 output
- **Function**: Implements the **MQTT v5 request/response pattern** end-to-end. On every incoming message the node:
  1. generates a unique `responseTopic` and a fresh `correlationData`,
  2. starts a one-shot subscription on the `responseTopic`,
  3. publishes the request to the configured target topic with v5 `Response Topic` and `Correlation Data` properties set,
  4. waits for a matching response or for the timeout,
  5. unsubscribes and emits the response on the output (or raises a catchable error on timeout).

  The Subscribe + Publish pair could be wired manually, but the bookkeeping (random topic, correlation, parallel inflight requests, timeout, cleanup on stop / disconnect) is non-trivial and would be re-built by every flow that talks to MQTT request handlers — the dedicated node owns it once.

- **Base configuration**:
  - `broker` (string) — ID of the referenced `mqtt-broker` config node. **Should target an MQTT v5 broker** — the node relies on the v5 `Response Topic` and `Correlation Data` properties; with a v3.1.1 broker the node degrades to a warning at deploy time (see *v3.1.1 fallback* below)
  - `topic` (string, optional) — target topic for the request publish. If empty, `msg.topic` is used; if both are missing the node sets the catchable error `"mqtt-request: no target topic"` and discards the message
  - `qos` (number) — QoS for **both** the request publish and the response subscription. Default: 0
  - `retain` (boolean) — retain flag on the request publish. Default: false (rare to retain a request — usually wrong)
  - `responseTopicPrefix` (string, optional) — prefix used to build the random response topic. Default: `loopze/response`. The full topic is `<prefix>/<random-uuid>`. A trailing slash is normalized
  - `timeout` (number, seconds) — how long to wait for the response before giving up. Default: 30. `0` disables the timeout (the request stays inflight until response, broker disconnect, or node stop — risky)
  - `timeoutMode` (string) — what happens on timeout:
    - `error` (default) — emits the catchable error `"mqtt-request: timeout"`; **no** message goes out the regular output
    - `passthrough` — emits a message on the regular output with `msg.timedOut = true` and the original `msg.payload` preserved — lets a downstream Switch decide
  - `responseFormat` (string) — same options as `mqtt-in` (`string` / `json` / `buffer`), default `string`. Applies to `msg.payload` of the response message

- **MQTT v5 Default Properties on the request publish** (optional, all merged with `msg.*` overrides — same semantics as `mqtt-out`):
  - `defaultUserProperties` (object)
  - `defaultContentType` (string)
  - `defaultMessageExpiry` (number, seconds)
  - `defaultPayloadFormat` (number, 0 or 1)

  **Deliberately not configurable**: `responseTopic` and `correlationData` are always generated by the node. Any `msg.responseTopic` / `msg.correlationData` on the input is **ignored** — the request/response correlation is owned by the node.

- **Incoming message** (the request):
  - `msg.payload` — request payload (encoded the same way as `mqtt-out`)
  - `msg.topic` (optional) — overrides the configured target topic
  - `msg.qos` (optional, 0/1/2) — overrides the configured QoS for this single request and its response subscription
  - v5 message overrides analogous to `mqtt-out`: `msg.userProperties`, `msg.contentType`, `msg.messageExpiry`, `msg.payloadFormat`. **`msg.responseTopic` and `msg.correlationData` are ignored** (see above)

- **Outgoing message** (on response):
  The original message is forwarded with the response merged in:
  - `msg.payload` — response payload, decoded per `responseFormat`
  - `msg.topic` — the random response topic (visible in Debug); the original request topic is preserved as `msg.requestTopic`
  - `msg.qos`, `msg.retain` — from the response publish
  - `msg.correlationData` — the bytes the responder echoed back (matches the bytes the node generated)
  - v5 fields if present on the response: `msg.userProperties`, `msg.contentType`, `msg.messageExpiry`, `msg.payloadFormat`

  On `timeoutMode = passthrough` the original payload is preserved unchanged and `msg.timedOut = true` is added; no response fields are set.

- **Behavior**:
  1. On every incoming message the node creates a fresh correlation context: `responseTopic = <prefix>/<uuid4>` and `correlationData = <16 random bytes>`
  2. It subscribes to `responseTopic` via the shared broker manager (`Subscribe(nodeID, topic, qos, handler)`) — `noLocal = false`, `retainHandling = 2`. **`noLocal` must be `false`**: requester and responder may share the same broker connection (e.g., when both `mqtt-request` and the paired `mqtt-out target=responseTopic` live in the same LOOPZE instance), and MQTT v5 §3.8.3.1 says the broker filters publishes from a connection with the same Client ID when `noLocal=true`. Echo-loop is not a concern because the response topic is a random UUID
  3. After SUBACK it publishes the request to the target topic with `Properties.ResponseTopic = responseTopic`, `Properties.CorrelationData = correlationData`, and any additional v5 properties merged from config + msg
  4. A timer starts (`timeout`)
  5. On the first incoming publish on `responseTopic` whose `Properties.CorrelationData` equals the recorded value, the node:
     - cancels the timer,
     - unsubscribes from `responseTopic`,
     - emits the response message on output `0`
  6. If the timer fires first, the node unsubscribes and either raises a catch error or emits a `timedOut` message according to `timeoutMode`
  7. Publishes that arrive on `responseTopic` but with a *non-matching* `correlationData` are silently dropped (defensive — should not happen because the topic is unique per request)

  **Multiple inflight requests** are supported — every incoming message has its own `responseTopic` + correlation, tracked in an internal map keyed by `correlationData`. On stop / re-deploy / broker disconnect all pending contexts are unsubscribed, timers cleared, and (for an unclean broker disconnect) every pending context is failed according to `timeoutMode`.

- **v3.1.1 fallback**: the v5 `Response Topic` / `Correlation Data` properties don't exist on the wire in v3.1.1. The node still subscribes to the random topic and publishes the user payload **as-is**, but it can only correlate by topic uniqueness — the responder must know the response topic by some out-of-band convention. In practice the node is intended for v5; on v3.1.1 the node logs a warning at deploy (`"mqtt-request: full request/response pattern requires MQTT v5; broker speaks 3.1.1"`) and remains functional with the documented limitation. Pairing with the `mqtt-out` *Response to responseTopic* mode requires v5 on both ends because that mode reads `msg.responseTopic` / `msg.correlationData` from a v5 inbound publish.

- **Status display** (via `SetStatus`):
  - Green: `idle` when no request is inflight, or `n inflight` when one or more requests are pending
  - Yellow: broker reconnecting (inherited from broker manager)
  - Red: broker disconnected; on broker disconnect all inflight contexts are failed (catch error or `timedOut` message depending on `timeoutMode`)

- **Properties panel**:

```
┌──────────────────────────────────────────────┐
│  MQTT Request                                 │
├──────────────────────────────────────────────┤
│                                               │
│  Broker                                       │
│  ┌────────────────────────────────────┐ ┌───┐│
│  │ Production Broker              ▼  │ │ + ││
│  └────────────────────────────────────┘ └───┘│
│  Edit broker config                           │
│                                               │
│  Request topic                                │
│  ┌────────────────────────────────────────┐   │
│  │ rpc/devices/42/getState                │   │
│  └────────────────────────────────────────┘   │
│  ℹ Falls back to msg.topic if empty.          │
│                                               │
│  QoS    Retain  Timeout (s)   On timeout      │
│  [0 ▼]   ☐      [    30    ]  ( • ) Error     │
│                               (   ) Passthrough│
│                                               │
│  Response topic prefix                        │
│  ┌────────────────────────────────────────┐   │
│  │ loopze/response                        │   │
│  └────────────────────────────────────────┘   │
│  ℹ Full topic: <prefix>/<random-uuid>         │
│                                               │
│  Response format                              │
│  ┌────────────────────────────────────────┐   │
│  │ String                            ▼   │   │
│  └────────────────────────────────────────┘   │
│                                               │
│  ▼ MQTT v5 Default Properties (optional)      │
│  Content Type    [application/json         ]  │
│  Message Expiry  [    ] sec                   │
│  Payload Format  [0 — bytes              ▼]   │
│                                               │
│  User Properties:                             │
│  ┌──────────────┐ ┌──────────────┐ ┌───┐     │
│  │ key          │ │ value        │ │ × │     │
│  └──────────────┘ └──────────────┘ └───┘     │
│  [+ Add property]                             │
│                                               │
│  ℹ correlationData is generated by the node;  │
│    msg.correlationData / msg.responseTopic    │
│    are ignored.                               │
│                                               │
└──────────────────────────────────────────────┘
```

### 5. Config Node Concept (new in LOOPZE)

Config nodes are a new architectural concept introduced with this issue:

- **No canvas element**: Config nodes have no visual representation in the flow editor
- **Standalone persistence**: Config nodes are stored in `workspace.json` as their own section (not within a flow)
- **Referencing**: Regular nodes reference config nodes via their ID
- **Shared instance**: Multiple nodes can reference the same config node — the engine creates only **one** instance per config node (e.g., one MQTT connection) and shares it among all referencing nodes
- **Lifecycle**: Config node instances are created on deploy and stopped on re-deploy/stop

### 6. Broker Connection Sharing

When multiple MQTT nodes reference the same broker, **a single MQTT connection** is shared:

```
[mqtt-in  topic=a] ──┐
[mqtt-in  topic=b] ──┤── Broker "Production" ── 1 TCP connection
[mqtt-out topic=c] ──┘
```

The engine must provide a **broker manager** that:
1. Collects all referenced broker configs on deploy
2. Establishes one MQTT client connection per broker ID
3. Gives the MQTT nodes access to the shared client
4. Cleanly disconnects all connections on stop/re-deploy

## Data Structure

### workspace.json

```json
{
  "flows": [
    {
      "id": "flow-1",
      "type": "tab",
      "label": "Sensors",
      "nodes": [
        {
          "id": "node-mqtt-in-1",
          "type": "mqtt-in",
          "name": "Temperature",
          "x": 200,
          "y": 150,
          "z": "flow-1",
          "inputs": 0,
          "outputs": 1,
          "wires": [["node-debug-1"]],
          "config": {
            "broker": "broker-1",
            "mode": "static",
            "topic": "sensor/temperature",
            "qos": 0
          }
        },
        {
          "id": "node-mqtt-in-2",
          "type": "mqtt-in",
          "name": "Dynamic Subscription",
          "x": 200,
          "y": 250,
          "z": "flow-1",
          "inputs": 1,
          "outputs": 1,
          "wires": [["node-debug-1"]],
          "config": {
            "broker": "broker-1",
            "mode": "dynamic",
            "qos": 0
          }
        },
        {
          "id": "node-mqtt-out-1",
          "type": "mqtt-out",
          "name": "Control",
          "x": 600,
          "y": 300,
          "z": "flow-1",
          "inputs": 1,
          "outputs": 0,
          "wires": [],
          "config": {
            "broker": "broker-1",
            "target": "topic",
            "topic": "actuator/command",
            "qos": 1,
            "retain": false
          }
        },
        {
          "id": "node-mqtt-out-response-1",
          "type": "mqtt-out",
          "name": "RPC Reply",
          "x": 800,
          "y": 400,
          "z": "flow-1",
          "inputs": 1,
          "outputs": 0,
          "wires": [],
          "config": {
            "broker": "broker-1",
            "target": "responseTopic",
            "qos": 1,
            "retain": false
          }
        },
        {
          "id": "node-mqtt-request-1",
          "type": "mqtt-request",
          "name": "Get Device State",
          "x": 400,
          "y": 500,
          "z": "flow-1",
          "inputs": 1,
          "outputs": 1,
          "wires": [["node-debug-1"]],
          "config": {
            "broker": "broker-1",
            "topic": "rpc/devices/42/getState",
            "qos": 1,
            "retain": false,
            "responseTopicPrefix": "loopze/response",
            "timeout": 30,
            "timeoutMode": "error",
            "responseFormat": "json"
          }
        }
      ]
    }
  ],
  "configs": [
    {
      "id": "broker-1",
      "type": "mqtt-broker",
      "name": "Production Broker",
      "config": {
        "host": "mqtt.example.com",
        "port": 1883,
        "clientId": "loopze-abc123",
        "protocolVersion": "5",
        "username": "user",
        "password": "",
        "keepalive": 60,
        "cleanStart": true,
        "sessionExpiry": 0,
        "useTLS": false,
        "onConnectTopic": "status/loopze-abc123",
        "onConnectPayload": "online",
        "onConnectQoS": 0,
        "onConnectRetain": true,
        "onDisconnectTopic": "status/loopze-abc123",
        "onDisconnectPayload": "offline",
        "onDisconnectQoS": 0,
        "onDisconnectRetain": true,
        "lastWillTopic": "status/loopze-abc123",
        "lastWillPayload": "offline",
        "lastWillQoS": 0,
        "lastWillRetain": true,
        "lastWillDelayInterval": 0
      }
    }
  ]
}
```

**Note**: The `configs` array is a new top-level section in `workspace.json` alongside `flows`. All config nodes (now MQTT broker, in the future also HTTP auth, database connections, etc.) are stored here.

## Affected Files

### Backend – New Files

- `internal/nodes/mqtt_broker.go` — MQTT Broker config node: connection setup, reconnect logic, subscription management. Encapsulates the `paho.mqtt.golang` client
- `internal/nodes/mqtt_in.go` — MQTT Subscribe node: registers subscription with the shared broker client, receives messages, and sends them via `SendFunc` into the flow
- `internal/nodes/mqtt_out.go` — MQTT Publish node: publishes incoming flow messages via the shared broker client. Honors `target = topic | responseTopic`
- `internal/nodes/mqtt_request.go` — MQTT Request node: per-message random response topic + correlation data, one-shot subscribe, publish, timeout/cleanup. Owns an inflight map keyed by correlationData

### Backend – Adjustments

- `internal/server/server.go` — registration of `mqtt-in`, `mqtt-out`, and `mqtt-request` in `registerNodes()`
- `internal/flow/engine.go` — config node lifecycle:
  - New section in `Deploy()`: instantiate config nodes before the regular nodes
  - Provide config node instances to the referencing nodes via a new provider interface
  - Extend `Stop()`: cleanly shut down config node instances
- `internal/flow/registry.go` — optional extension: `ConfigProvider` interface analogous to `ContextProvider` and `LinkProvider`
  ```go
  type ConfigProvider interface {
      SetConfigNode(configType string, configID string, instance any)
  }
  ```
- `internal/flow/types.go` — config node data type for workspace.json deserialization:
  ```go
  type ConfigNode struct {
      ID     string         `json:"id"`
      Type   string         `json:"type"`
      Name   string         `json:"name"`
      Config map[string]any `json:"config"`
  }
  ```
- `internal/storage/` — extend workspace load/save with `configs` section

### Frontend – New Files

- `frontend/src/components/config/MqttNodeConfig.vue` — shared config component for `mqtt-in` and `mqtt-out` with broker dropdown + "+" button, topic input, QoS dropdown, retain toggle (mqtt-out only). Renders the `target = topic | responseTopic` selector for `mqtt-out` and disables/hides the topic field in responseTopic mode
- `frontend/src/components/config/MqttRequestConfig.vue` — config component for `mqtt-request`: broker dropdown, request topic, QoS, retain, timeout + timeoutMode, response topic prefix, response format, v5 default properties block. Reuses the broker dropdown subcomponent from `MqttNodeConfig.vue`
- `frontend/src/components/config/MqttBrokerConfig.vue` — Broker config dialog: form for host, port, client ID, credentials, TLS, keep-alive. Opens as a standalone properties panel via the "+" button

### Frontend – Adjustments

- `frontend/src/components/PropertyPanel.vue` — dispatch for `mqtt-in` and `mqtt-out` to `MqttNodeConfig`, and `mqtt-request` to `MqttRequestConfig`. Additionally: support for config node dialogs (broker configuration as a nested panel)
- `frontend/src/components/nodes/tokens.ts` — already present: `mqtt-in` → input (green), `mqtt-out` → output (orange). Add `mqtt-request` (function/orange — request node, paired symbology with `http-request`)
- `frontend/src/stores/flowStore.ts` — manage config nodes: CRUD operations for `configs[]` in the workspace, API calls for persistence
- `frontend/src/types/flow.ts` — TypeScript types for config nodes and MQTT broker config

### Go Dependencies

- `github.com/eclipse/paho.golang/paho` — MQTT v5 client library (Eclipse Paho v5)
- `github.com/eclipse/paho.golang/autopaho` — connection manager with auto-reconnect logic around the v5 client
- **Note**: The older `github.com/eclipse/paho.mqtt.golang` library only speaks v3.1.1 and is therefore not sufficient. The v5 fallback to v3.1.1 is handled via the broker's protocol negotiation — the `paho.golang` library can cover this sufficiently for our purposes; if needed, with `protocolVersion=3.1.1` an explicit run against the old library or a separate v3 path may be required.

## Technical Notes

### Config Node Lifecycle in the Engine

Config nodes have their own lifecycle that runs **before** the regular nodes:

1. **On deploy**: Engine reads `configs[]` from the workspace, instantiates config nodes, and establishes connections
2. **Injection**: Regular nodes that implement `ConfigProvider` receive references to their config node instances
3. **On stop/re-deploy**: Config node instances are stopped **after** the regular nodes (reverse order)

```
Deploy:   Start config nodes → Start regular nodes
Stop:     Stop regular nodes → Stop config nodes
```

### MQTT Broker – Reconnect Strategy

The MQTT client implements automatic reconnect via `paho.golang/autopaho`:

- `autopaho.NewConnection` handles connection setup and reconnect with configurable backoff
- For v5 sessions with `sessionExpiry > 0` the broker reactivates existing subscriptions; with `cleanStart=true` LOOPZE must restore all subscriptions itself after reconnect
- On connection loss: status to yellow ("reconnecting...")
- On successful reconnect: refresh subscriptions automatically (unless already active via session), status to green
- On permanent error: status to red with error message
- If the broker rejects the chosen protocol version, CONNECT fails with status red and reason code in the error message — the user must explicitly switch the broker config to v3.1.1

### onConnect / onDisconnect / LastWill – Lifecycle

- **onConnect**: after every successful CONNACK (including after reconnect) the broker manager fires the onConnect publish as the first action, before any other node may register subscriptions or publish. This way consumers see "online" first consistently, then the actual data stream
- **onDisconnect**: on regular stop / re-deploy the broker manager first publishes the onDisconnect message, waits for the ACK (with QoS > 0) or the flush (with QoS 0), and only then sends the DISCONNECT packet
- **LastWill**: passed in the CONNECT packet to the broker and published exclusively by the broker itself when the connection drops uncleanly. On a regular DISCONNECT the broker discards the LastWill (as specified) — that is why the separate onDisconnect message is needed
- onConnect and onDisconnect are **client-side convention** (regular PUBLISH packets), LastWill is the **MQTT protocol feature**
- If onConnect and onDisconnect publish to the same topic with `retain=true`, LastWill with `retain=true` is recommended so that the retained status remains consistent across every disconnect variant

### Broker Dropdown in the Frontend

The broker dropdown in the MQTT node config panel shows all `mqtt-broker` config nodes from `flowStore.configs`:

```typescript
const mqttBrokers = computed(() =>
  flowStore.configs.filter(c => c.type === 'mqtt-broker')
)
```

The "+" button next to the dropdown opens the `MqttBrokerConfig.vue` dialog. After saving, the new broker is automatically selected in the dropdown.

### Message Mapping (mqtt-in)

Received MQTT messages are translated into the LOOPZE message format. v5 properties, when delivered by the broker, are carried over into the outgoing message:

```go
func (n *MqttInNode) onPublish(p *paho.Publish) {
    msg := NewMessage()
    msg.Set("topic", p.Topic)
    msg.Set("payload", string(p.Payload))
    msg.Set("qos", int(p.QoS))
    msg.Set("retain", p.Retain)

    if props := p.Properties; props != nil { // MQTT v5 Properties
        if len(props.User) > 0 {
            up := map[string]string{}
            for _, kv := range props.User {
                up[kv.Key] = kv.Value
            }
            msg.Set("userProperties", up)
        }
        if props.ContentType != "" {
            msg.Set("contentType", props.ContentType)
        }
        if props.ResponseTopic != "" {
            msg.Set("responseTopic", props.ResponseTopic)
        }
        if len(props.CorrelationData) > 0 {
            msg.Set("correlationData", props.CorrelationData)
        }
        if props.MessageExpiry != nil {
            msg.Set("messageExpiry", *props.MessageExpiry)
        }
    }

    n.send(0, msg)
}
```

### Dynamic Subscription (mqtt-in, Dynamic Mode)

In dynamic mode the node implements `OnInput`, keeps the list of currently active
topics internally, and reacts to control messages:

```go
type MqttInNode struct {
    // … broker, qos, …
    activeTopics []string // only populated in dynamic mode
    mu           sync.Mutex
}

func (n *MqttInNode) OnInput(msg Message) {
    if action, _ := msg.GetString("action"); action != "subscribe" {
        return // only "subscribe" is accepted; everything else is discarded
    }

    next := toTopicSlice(msg.Get("payload")) // string → [s], []string → s, empty → []
    qos  := extractQoS(msg.Get("qos"), n.qos) // msg.qos overrides config-qos (0/1/2), else fallback

    n.mu.Lock()
    defer n.mu.Unlock()

    // Unsubscribe all previous topics of this node
    for _, t := range n.activeTopics {
        n.broker.Unsubscribe(n.id, t)
    }
    // Subscribe new topics
    for _, t := range next {
        n.broker.Subscribe(n.id, t, qos, n.onMessage)
    }
    n.activeTopics = next
    n.updateStatus()
}
```

Important:
- Control messages are **not** forwarded (`return` instead of `n.send(0, msg)`).
- On stop / redeploy all `activeTopics` must be cleanly unsubscribed.
- The MQTT broker (config node) must offer `Subscribe` and `Unsubscribe` per subscriber ID so that on redeploy or re-subscribe targeted cleanup is possible.

### Payload Handling (mqtt-out)

The publish node reads `msg.payload` and converts it for the MQTT publish:

- `string` → directly as payload
- `map`/`slice` → JSON-serialized
- `number`/`bool` → string conversion

### Request / Response Pattern (mqtt-request ↔ mqtt-out target=responseTopic)

The request/response pair leans on two MQTT v5 properties on the wire:

- **Response Topic** — set by the requester on the request publish; the responder reads it and publishes the reply to that topic
- **Correlation Data** — opaque bytes set by the requester; the responder copies them onto the reply, allowing the requester to match reply ↔ request even when topics are reused

End-to-end lifecycle:

```
  ┌──────────────┐                              ┌─────────────────┐
  │ mqtt-request │                              │ Responder side  │
  │  (requester) │                              │  (mqtt-in →     │
  │              │                              │   mqtt-out      │
  │              │                              │   target=       │
  │              │                              │   responseTopic)│
  └──────────────┘                              └─────────────────┘

  1. msg in
  2. generate responseTopic = loopze/response/<uuid>
     generate correlationData = <16 random bytes>
  3. SUBSCRIBE responseTopic ─────────►  (broker)
     ◄──────────── SUBACK
  4. PUBLISH target_topic
       Properties.ResponseTopic   = responseTopic
       Properties.CorrelationData = correlationData
       Payload                    = msg.payload
                                       ──────────►   mqtt-in receives
                                                     msg.responseTopic
                                                     msg.correlationData

                                                     [user flow]

                                                     mqtt-out target=
                                                       responseTopic
                                                     PUBLISH
                                                       msg.responseTopic
                                                       Properties.
                                                         CorrelationData
                                                         = msg.correlation
                                                           Data
                                       ◄──────────
  5. publish on responseTopic with matching correlationData →
     - cancel timeout
     - UNSUBSCRIBE responseTopic
     - emit response on output 0
```

The `mqtt-request` node implementation sketch:

```go
type inflight struct {
    correlation []byte
    responseTopic string
    timer       *time.Timer
    inMsg       Message // original input msg for passthrough on timeout
}

type MqttRequestNode struct {
    // … broker, topic, qos, retain, prefix, timeout, timeoutMode, format, defaults
    pending map[string]*inflight // key = hex(correlation)
    mu      sync.Mutex
}

func (n *MqttRequestNode) OnInput(msg Message) {
    topic, ok := pickTopic(n.topic, msg) // configured else msg.topic
    if !ok {
        n.catchError(msg, "mqtt-request: no target topic")
        return
    }

    rt := n.prefix + "/" + uuid.NewString()
    cd := randBytes(16)
    key := hex.EncodeToString(cd)

    ctx := &inflight{correlation: cd, responseTopic: rt, inMsg: msg}
    n.mu.Lock()
    n.pending[key] = ctx
    n.mu.Unlock()

    // 1) one-shot subscription on rt (noLocal=true, retainHandling=2)
    if err := n.broker.Subscribe(n.id, rt, n.qosFor(msg), n.onResponse); err != nil {
        n.cleanup(key)
        n.catchError(msg, "mqtt-request: subscribe failed: "+err.Error())
        return
    }

    // 2) publish with v5 ResponseTopic + CorrelationData
    props := mergeProps(n.defaults, msg)
    props.ResponseTopic = rt
    props.CorrelationData = cd
    if err := n.broker.Publish(topic, msg.Get("payload"), n.qosFor(msg), n.retain, props); err != nil {
        n.broker.Unsubscribe(n.id, rt)
        n.cleanup(key)
        n.catchError(msg, "mqtt-request: publish failed: "+err.Error())
        return
    }

    // 3) timer
    if n.timeout > 0 {
        ctx.timer = time.AfterFunc(n.timeout, func() { n.onTimeout(key) })
    }
}

func (n *MqttRequestNode) onResponse(p *paho.Publish) {
    if p.Properties == nil || len(p.Properties.CorrelationData) == 0 {
        return // no correlation → cannot match, ignore (defensive)
    }
    key := hex.EncodeToString(p.Properties.CorrelationData)

    n.mu.Lock()
    ctx, ok := n.pending[key]
    if !ok { n.mu.Unlock(); return } // unknown / late response — drop
    delete(n.pending, key)
    n.mu.Unlock()

    if ctx.timer != nil { ctx.timer.Stop() }
    n.broker.Unsubscribe(n.id, ctx.responseTopic)

    out := ctx.inMsg.Clone()
    out.Set("requestTopic", out.Get("topic"))
    out.Set("topic", p.Topic)
    out.Set("payload", decode(p.Payload, n.responseFormat))
    out.Set("qos", int(p.QoS))
    out.Set("retain", p.Retain)
    out.Set("correlationData", p.Properties.CorrelationData)
    mergeV5IntoMsg(out, p.Properties)
    n.send(0, out)
}

func (n *MqttRequestNode) onTimeout(key string) {
    n.mu.Lock()
    ctx, ok := n.pending[key]
    if !ok { n.mu.Unlock(); return }
    delete(n.pending, key)
    n.mu.Unlock()

    n.broker.Unsubscribe(n.id, ctx.responseTopic)

    if n.timeoutMode == "passthrough" {
        out := ctx.inMsg.Clone()
        out.Set("timedOut", true)
        n.send(0, out)
        return
    }
    n.catchError(ctx.inMsg, "mqtt-request: timeout")
}
```

Important:

- On `Stop()` the node iterates `pending`, stops every timer, unsubscribes every response topic, and depending on `timeoutMode` either emits `timedOut` passthrough messages or raises catch errors so flow callers don't hang silently.
- On broker disconnect the broker manager invokes a per-subscriber callback that the request node uses to fail all inflight contexts the same way.
- `Subscribe` for the response topic must use the v5 options `noLocal=false`, `retainHandling=2`. The default in mqtt-in (`noLocal=false`) is the right choice here too — `true` would cause the broker to drop the response when requester and responder share a connection (single LOOPZE instance, both nodes referencing the same broker config). `retainHandling=2` keeps the temporary topic from receiving stale retained junk.

### mqtt-out — `target = responseTopic`

The publish node has two target modes that route to different topics:

- `target = "topic"` (default) — `effectiveTopic = config.topic || msg.topic` (current behavior)
- `target = "responseTopic"` — `effectiveTopic = msg.responseTopic` only; `config.topic` and `msg.topic` are ignored. If `msg.responseTopic` is empty/missing → catchable error, no publish

When `target = "responseTopic"`, the node also forwards `msg.correlationData` (if present) as the v5 `Correlation Data` property on the publish — which is what makes the round-trip with `mqtt-request` work. All other v5 property merge rules from § 3 apply unchanged.

```go
func (n *MqttOutNode) effectiveTopic(msg Message) (string, error) {
    if n.target == "responseTopic" {
        rt, _ := msg.GetString("responseTopic")
        if rt == "" {
            return "", errors.New("mqtt-out: target=responseTopic but msg.responseTopic is missing")
        }
        return rt, nil
    }
    if n.topic != "" { return n.topic, nil }
    if t, _ := msg.GetString("topic"); t != "" { return t, nil }
    return "", errors.New("mqtt-out: no topic")
}
```

## Dependencies

- **No dependencies** on existing issues — this is a standalone feature
- Introduces the **config node concept** that will be reused by future connector nodes (HTTP, TCP, Modbus, OPC UA, databases, etc.)
- Frontend tokens for `mqtt-in` and `mqtt-out` are already defined in `tokens.ts` — they appear in the palette automatically once the backend registers them. A new token for `mqtt-request` must be added (function-class, paired symbology with `http-request`)

## Out of Scope / Not in Scope

- **MQTT v5 feature scope**:
  - **In**: User Properties (CONNECT, PUBLISH, SUBSCRIBE — outbound + inbound), Message Expiry Interval, Content Type, Response Topic, Correlation Data, Payload Format Indicator, Shared Subscriptions, Subscription Identifiers (set + receive), Subscription Options (No Local, Retain As Published, Retain Handling), LastWill with Will Delay Interval, Session Expiry Interval, Clean Start
  - **Deliberately not in v1**:
    - **Topic Aliases**: managed transparently by the client manager but not user-configurable (e.g., `Topic Alias Maximum` in CONNECT). Default 0 = off
    - **Reason code routing** to catch outputs: ACK reason codes (PUBACK, SUBACK, UNSUBACK, DISCONNECT) are logged but not exposed as a separate flow output
    - **Enhanced Authentication** (auth properties, AUTH packet, SASL-style flows): not in v1
    - **Flow Control**: `Receive Maximum`, `Maximum Packet Size`, `Server Keep Alive`, `Server Reference` — client defaults are used, no UI configuration
    - **Request/Response Information** in CONNECT (`Request Response Information`, `Request Problem Information`): default true for Problem Information, otherwise not configurable
    - **CONNECT User Properties**: not in the UI currently; can be set via library API if anyone needs them (extension later)
- **Wildcard topics**: `+` and `#` wildcards in topics are not explicitly validated for the first cut, but they work transparently via the MQTT client
- **Request/response on v3.1.1**: the `mqtt-request` node and `mqtt-out target = responseTopic` mode rely on the v5 `Response Topic` and `Correlation Data` properties. With a v3.1.1 broker, request/response is **degraded** — the request node logs a warning at deploy and works only by topic-uniqueness convention; the `mqtt-out responseTopic` mode will always error because v3.1.1 inbound publishes carry no `responseTopic`. A custom v3.1.1 envelope (e.g., embedding `responseTopic` and `correlationData` in the payload) is **deliberately not** implemented in v1
- **Extended TLS configuration**: client certificates, CA bundle, etc. — not in v1
- **Credential encryption**: passwords are stored in plaintext in the config for now. Encryption is addressed separately via a credential system
