# Split & Join Nodes

## Description

Two complementary nodes for **sequence processing** — one splits a message into many, the other reassembles many messages into one. They are the standard pattern for processing lists/streams/batches in a flow:

```
[ Source ] -> [ Split ] -> [ N processing steps ] -> [ Join ] -> [ Sink ]
```

Both nodes are based on a new message concept **`msg.parts`** that describes a message's membership in a sequence.

> Prerequisite for the concept: switch/filter operators like `head`/`tail` (see `SWITCH_NODE.md` — deliberately omitted there) can only be sensibly implemented once `msg.parts` exists.

## The `msg.parts` Concept

When Split divides a message into N parts, it attaches a `parts` object to each outgoing message:

| Field | Type | Description |
|---|---|---|
| `id` | string | Common sequence ID (all messages of the same split share this ID) |
| `index` | number | 0-based position in the sequence |
| `count` | number | Total number of messages in the sequence (may be missing for streams) |
| `type` | string | `array`, `string`, `object`, `buffer` — how it was split |
| `ch` | string | Separator (only for `type=string`, for reconstruction) |
| `key` | string | Original key (only for `type=object`, for reconstruction) |
| `len` | number | Length of the original data field (for buffer reconstruction) |

`parts` travels with the message through the flow — intermediate nodes (Function, Change, ...) leave it untouched, so Join can reconstruct the sequence later.

## Split Node

### Behavior

- **1 input**, **1 output**
- Per incoming message: splits `msg.payload` (or a configurable property) and sends one separate message per element
- Original message fields are **copied**, only `payload` (or the split field) is replaced
- `msg.parts` is set on every output message
- Existing `msg.parts` of an incoming message is nested into `msg.parts.parts` (for nested splits) — see Node-RED behavior

### Split Modes (depending on payload type)

| Payload type | Splitting | Configuration |
|---|---|---|
| **Array** | One message per element | Optional: chunks of N elements |
| **String** | Split at separator | Separator (default: `\n`), optional regex |
| **Object** | One message per key, key lands in `msg.parts.key` (or configurable in a field) | Key property name |
| **Buffer** | In chunks of N bytes or at a byte sequence | Chunk size or separator byte sequence |

For unsupported types: message is passed through unchanged + warning logged.

### Configuration (Backend)

```json
{
  "property": "payload",
  "splt": "\n",
  "spltType": "str",
  "arraySplt": 1,
  "arraySpltType": "len",
  "stream": false,
  "addname": ""
}
```

| Field | Description |
|---|---|
| `property` | Which property to split (default `payload`) |
| `splt` | Separator (string) or chunk size (buffer) |
| `spltType` | `str`, `bin` (buffer pattern), `len` (number of bytes) |
| `arraySplt` | For arrays: size of sub-arrays (1 = one element per message) |
| `arraySpltType` | `len` (fixed size) |
| `stream` | `true` = `msg.parts.count` is not set (stream mode), `false` = completed sequence |
| `addname` | For object split: which property the original key is written to (e.g. `topic`). Empty = only in `msg.parts.key` |

### Examples

**Split array:**
```
Input:  msg.payload = [1, 2, 3]
Output: 3 messages with payload=1/2/3 and parts.{id, index, count=3, type:"array"}
```

**Split string (lines):**
```
Input:  msg.payload = "a\nb\nc"
Output: 3 messages with payload="a"/"b"/"c" and parts.{..., type:"string", ch:"\n"}
```

**Split object with key in topic:**
```
Input:  msg.payload = {a:1, b:2}
Config: addname = "topic"
Output: 2 messages with payload=1/2, topic="a"/"b", parts.{..., type:"object", key:"a"/"b"}
```

## Join Node

### Behavior

- **1 input**, **1 output**
- Collects incoming messages, combines them into a single output message
- Has **internal state** (per-node buffer per sequence ID or per topic)
- Sends the combined message when a **trigger** fires (count, timeout, parts-complete, ...)

### Modes

| Mode | Description |
|---|---|
| **automatic** | Uses `msg.parts` from a previous Split. Sequence ID groups, `count` triggers send. No further configuration needed — exact counterpart to Split. |
| **manual** | Ignores `msg.parts`. User configures output type and trigger explicitly. Also for messages that never went through a Split (e.g. sensor aggregation per time window). |
| **reduce sequence** | Applies a reduction function to the sequence (e.g. sum, min/max, concat). Inspired by Node-RED, but optional in MVP — see Open Questions. |

### Manual Mode: Output Types

| Type | Description |
|---|---|
| **string** | Concatenate with separator (e.g. `\n`) |
| **array** | Combine values into an array |
| **object** | Key/value object — key comes from a configurable property (e.g. `msg.topic`) or from `msg.parts.key` |
| **buffer** | Concatenate with optional separator byte sequence |
| **merged object** | Like object, but values are deep-merged when keys match |

### Triggers (when is the combined message sent?)

| Trigger | Description |
|---|---|
| **automatic** | As soon as `count` from `msg.parts` is reached (only in automatic mode) |
| **count N** | After exactly N received messages |
| **after timeout** | After X seconds of inactivity (no new message for the sequence) |
| **after specific message** | When a message arrives with a particular property value (e.g. `msg.complete = true`) |
| **manual reset** | Only when a reset message arrives (e.g. with `msg.reset = true`) |

Multiple triggers can be combined (whichever fires first).

### Per-Topic Grouping (optional)

When enabled, Join keeps a separate buffer **per `msg.topic`** and triggers independently. Useful for example to aggregate sensor values per sensor topic.

### Configuration (Backend)

```json
{
  "mode": "auto",
  "build": "string",
  "property": "payload",
  "propertyType": "msg",
  "key": "topic",
  "joiner": "\\n",
  "joinerType": "str",
  "accumulate": false,
  "timeout": 0,
  "count": 0,
  "reduceRight": false
}
```

| Field | Description |
|---|---|
| `mode` | `auto`, `custom` (= manual), `reduce` |
| `build` | Output type: `string`, `array`, `object`, `merged`, `buffer` (only with `custom`) |
| `property` | Which property from each message is combined (default `payload`) |
| `propertyType` | `msg` (always for source) |
| `key` | Property name for object keys (default `topic`, falls back to `parts.key`) |
| `joiner` | Separator (string/buffer) |
| `joinerType` | `str`, `bin` |
| `accumulate` | `true` = sequence is not cleared after each send but kept (sliding-window-like) |
| `timeout` | Timeout in seconds (0 = off) |
| `count` | Trigger count (0 = off) |
| `reduceRight` | Only `reduce` mode: reduction from the right |

### Examples

**Automatic join after split:**
```
Source -> Split -> Function (processes each element) -> Join (auto) -> Sink
```
Join reconstructs the original array/string/object 1:1 from `msg.parts`.

**Sensor aggregation per topic with timeout:**
```
mode: custom
build: array
key: topic
timeout: 5
```
-> Per topic, messages are collected for 5 seconds, then sent as an array.

**Until sentinel message:**
```
mode: custom
build: array
trigger: after specific message -> msg.eof === true
```

## Implementation

### Backend

#### `internal/nodes/split.go`
- `flow.NodeInstance`, **Inputs:** 1, **Outputs:** 1
- `OnMessage`:
  1. Get property value
  2. Detect type (array/string/object/buffer)
  3. Split into parts, per part new message via `msg.Clone()` + set property
  4. Set `parts` field (nested if already present)
  5. Sequence ID via `generateID()` (same function as message IDs)
  6. Send sequentially

#### `internal/nodes/join.go`
- `flow.NodeInstance`, **Inputs:** 1, **Outputs:** 1
- Holds `map[string]*sequenceBuffer` (key = sequence ID or topic)
- Per sequence: collect messages, check trigger
- On trigger: combine, send, clear buffer (unless `accumulate=true`)
- Timeout: `time.AfterFunc` per sequence, cancel on trigger
- Thread safety: `sync.Mutex` around the buffer map

#### Extension of `flow.Message`
Currently `Message.data` is a flat `map[string]any` — `parts` can live in there as a regular key. No API change needed, only document the convention:
- `msg.Get("parts.id")`, `msg.Get("parts.index")`, ... already works thanks to dot path
- Suggested helper function in `flow` package: `msg.Parts() *Parts` for type-safe access

For `Clone()`, ensure `parts` is copied along (happens automatically because it is part of `data`).

### Frontend

#### `SplitConfig.vue`
- Property selection (`MsgFieldEditor`)
- Auto-detection of payload type in the UI with hint text
- Fields shown dynamically based on expected type:
  - String -> separator input + regex toggle
  - Array -> chunk size input
  - Object -> "key in property" input
  - Buffer -> chunk size or byte pattern
- Stream toggle (checkbox)

#### `JoinConfig.vue`
- Mode tabs (Auto / Manual / Reduce)
- In manual mode: output type dropdown, different fields depending on type
- Trigger section: count, timeout, "complete on property", reset
- Per-topic grouping as checkbox

#### Node Components
- BaseNode with category `function` (or new category `sequence`)
- Split body: shows separator/chunk size compactly
- Join body: shows mode + trigger compactly

### Node Registration

```go
registry.Register("split", nodes.NewSplitNode, nodes.SplitTypeInfo())
registry.Register("join",  nodes.NewJoinNode,  nodes.JoinTypeInfo())
```

## Interaction With Other Nodes

- **Switch node**: as soon as `msg.parts` exists, operators like `head N`, `tail N`, `index between` can be added there (see `SWITCH_NODE.md` Open Questions).
- **Function node**: can deliberately manipulate `parts` (e.g. synthesize its own sequences) — no special handling needed.
- **Change node**: can delete `parts` if a sequence should be intentionally "cut off".
- **Debug node**: should make `parts` visible in the tree view (happens automatically since it is a regular property).

## Dependencies

- `flow.Message.Clone()` — present
- `flow.Message.Get/Set` with dot path — present
- `generateID()` for sequence IDs — present
- Per-node state in Join: engine already allows this (Function node has state)
- Frontend: `MsgFieldEditor`, `FormSelect` from `components/config/`

## Open Questions

1. **`reduce sequence` mode** in Join — MVP or later? Requires an embedded expression engine (JSONata in Node-RED). Suggestion: **later**, MVP only `auto` + `custom`.
2. **Nested splits** — should `parts.parts` nesting be explicitly supported, or only one level in the first MVP?
3. **Buffer support** — how important? MQTT payloads often arrive as buffers. Suggestion: **MVP yes**, since the additional effort is small.
4. **Per-topic grouping** — standalone mode or option in `custom` mode? Currently modeled as an option.
5. **Helper `msg.Parts()`** in `flow` package — type-safe is good, but breaks the "everything is the same" principle of the flat map. Alternative: only convention + constants for key names.
6. **Memory protection in Join**: what happens for triggers that never fire (sequence ID never reaches `count`)? Suggestion: configurable max buffer age (default: 10 min) -> discarded + warning.
