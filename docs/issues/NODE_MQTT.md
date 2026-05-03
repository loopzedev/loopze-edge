# Issue: MQTT Nodes – Subscribe & Publish with Broker Configuration

## Status: Open

## Problem Description

LOOPZE needs its first **Data Connector** — MQTT. Two new node types (`mqtt-in` and `mqtt-out`) enable receiving and sending MQTT messages. Central to this is the concept of a **broker configuration**, managed as a standalone, reusable entity. Each MQTT node references exactly one broker, but different brokers can be configured across multiple nodes.

This issue simultaneously introduces the new concept of **Config Nodes** — configurable entities that do not appear on the canvas but can be referenced by multiple nodes (e.g., server connections, authentication). The MQTT broker is the first config node in LOOPZE.

**Scope**: **MQTT v5 is mandatory.** The broker client must speak v5. v3.1.1 remains available as an alternatively selectable protocol version — the user picks it deliberately per broker config, there is no automatic fallback. Focus is on a working broker configuration and instantiation. The subscribe/publish configuration is intentionally minimal — v5-specific features (User Properties, Message Expiry, Shared Subscriptions) are supported in a lean first tier.

## Overview

| Node Type | Type ID | Canvas Inputs | Canvas Outputs | Description |
|---|---|---|---|---|
| **MQTT Subscribe** | `mqtt-in` | 0 | 1 | Receives messages from an MQTT broker via subscription |
| **MQTT Publish** | `mqtt-out` | 1 | 0 | Sends messages to an MQTT broker |

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
└─────────────────────┘   └──────────────┘
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
- **Function**: Publishes incoming messages via the configured broker to an MQTT topic
- **Base configuration**:
  - `broker` (string) — ID of the referenced `mqtt-broker` config node
  - `topic` (string, optional) — MQTT topic to publish to. If empty, `msg.topic` is used
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
│  Topic                                        │
│  ┌────────────────────────────────────────┐   │
│  │ actuator/command                       │   │
│  └────────────────────────────────────────┘   │
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

### 4. Config Node Concept (new in LOOPZE)

Config nodes are a new architectural concept introduced with this issue:

- **No canvas element**: Config nodes have no visual representation in the flow editor
- **Standalone persistence**: Config nodes are stored in `workspace.json` as their own section (not within a flow)
- **Referencing**: Regular nodes reference config nodes via their ID
- **Shared instance**: Multiple nodes can reference the same config node — the engine creates only **one** instance per config node (e.g., one MQTT connection) and shares it among all referencing nodes
- **Lifecycle**: Config node instances are created on deploy and stopped on re-deploy/stop

### 5. Broker Connection Sharing

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
            "topic": "actuator/command",
            "qos": 1,
            "retain": false
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
- `internal/nodes/mqtt_out.go` — MQTT Publish node: publishes incoming flow messages via the shared broker client

### Backend – Adjustments

- `internal/server/server.go` — registration of `mqtt-in` and `mqtt-out` in `registerNodes()`
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

- `frontend/src/components/config/MqttNodeConfig.vue` — shared config component for `mqtt-in` and `mqtt-out` with broker dropdown + "+" button, topic input, QoS dropdown, retain toggle (mqtt-out only)
- `frontend/src/components/config/MqttBrokerConfig.vue` — Broker config dialog: form for host, port, client ID, credentials, TLS, keep-alive. Opens as a standalone properties panel via the "+" button

### Frontend – Adjustments

- `frontend/src/components/PropertyPanel.vue` — dispatch for `mqtt-in` and `mqtt-out` to `MqttNodeConfig`. Additionally: support for config node dialogs (broker configuration as a nested panel)
- `frontend/src/components/nodes/tokens.ts` — already present: `mqtt-in` → input (green), `mqtt-out` → output (orange). No change needed
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

## Dependencies

- **No dependencies** on existing issues — this is a standalone feature
- Introduces the **config node concept** that will be reused by future connector nodes (HTTP, TCP, Modbus, OPC UA, databases, etc.)
- Frontend tokens for `mqtt-in` and `mqtt-out` are already defined in `tokens.ts` — they appear in the palette automatically once the backend registers them

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
- **Extended TLS configuration**: client certificates, CA bundle, etc. — not in v1
- **Credential encryption**: passwords are stored in plaintext in the config for now. Encryption is addressed separately via a credential system
