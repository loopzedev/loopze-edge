import type { NodeHelpDoc } from './types'

export const nodeHelpDocs: Record<string, NodeHelpDoc> = {
  debug: {
    overview:
      'Displays incoming messages in the Debug panel of the Information Sidebar. Useful for inspecting payloads, verifying transformations, and tracing flow execution.',
    inputs: ['Any message — the node simply prints whatever it receives.'],
    properties: [
      { key: 'output',     desc: 'What to display: a single property (default), the complete msg object, or a GJSON path expression.' },
      { key: 'property',   desc: 'For "property" / "gjson" output: which field of msg to print (e.g. payload, payload.user.name).' },
      { key: 'console',    desc: 'Also write the value to the server console / log.' },
      { key: 'status',     desc: 'Show a short status text underneath the node (e.g. last value preview).' },
      { key: 'maxLength',  desc: 'Truncate long string values for display.' },
    ],
    tips: [
      'Toggle ON/OFF in the Debug panel to silence noisy nodes without redeploying.',
      'Click a key in the JSON tree to pin its path — the value is then highlighted on each new message.',
      'Use the search/filter at the top of the Debug panel to find specific messages.',
    ],
  },

  switch: {
    overview:
      'Routes the incoming message to one or more outputs based on rules evaluated against a single property. Each rule corresponds to one output port (top to bottom). Use it as if/elseif/else for messages.',
    inputs: ['Any message — the configured property is read and compared against each rule.'],
    outputs: ['One port per rule. The same message is forwarded unchanged to every matching port.'],
    properties: [
      { key: 'property',     desc: 'Path of the value to test (e.g. payload, payload.status.code).' },
      { key: 'propertyType', desc: 'Scope of the property: msg / flow / global.' },
      { key: 'rules',        desc: 'Ordered list of rules; each rule maps to one output port. Drag to reorder.' },
      { key: 't (op)',       desc: 'Operator: ==, !=, <, <=, >, >=, between, contains, regex, is true/false/null/empty, is of type, otherwise.' },
      { key: 'v / vt',       desc: 'Comparison value and its type (str/num/bool/json/msg/flow/global/env).' },
      { key: 'case',         desc: 'For regex/contains: enable case-sensitive matching (default off).' },
      { key: 'checkall',     desc: 'false (default): stop after first match. true: forward to every matching output.' },
    ],
    examples: [
      {
        title: 'Status routing',
        config: 'msg.payload.status   == "ok" → 1   == "warn" → 2   == "error" → 3   otherwise → 4',
        result: 'Splits a status field into four downstream branches.',
      },
      {
        title: 'Threshold split',
        config: 'msg.payload   < 10 → low   between 10..50 → mid   > 50 → high',
        result: 'Routes a numeric value into three buckets.',
      },
      {
        title: 'Regex topic filter',
        config: 'msg.topic   matches /^sensor\\/temp\\// → 1   matches /^sensor\\/hum\\// → 2   otherwise → 3',
        result: 'Forwards messages by topic pattern.',
      },
    ],
    tips: [
      '"otherwise" only fires when no other rule matched — works in both modes.',
      'Equality uses loose comparison: "10" == 10 is true.',
      'Regex defaults to case-insensitive — toggle "case-sensitive" if you need strict matching.',
      'Reordering rules also reorders the output ports; existing wires follow their rule.',
    ],
  },

  change: {
    overview:
      'Sets, changes, deletes, or moves properties on the message. Multiple rules are applied top-to-bottom — the result of one rule is the input of the next.',
    inputs: ['Any message — properties are read and written on this object.'],
    outputs: ['The transformed message.'],
    properties: [
      { key: 'rules',  desc: 'Ordered list of operations. Drag to reorder.' },
      { key: 't (op)', desc: 'Operation type: set, change (replace text), delete, move.' },
      { key: 'p',      desc: 'Target property path (dotted, e.g. payload.user.name).' },
      { key: 'pt',     desc: 'Target context: msg / flow / global.' },
      { key: 'to',     desc: 'New value (literal, msg.path, flow./global. context, or expr expression).' },
      { key: 'tot',    desc: 'How "to" is interpreted (string, num, bool, json, env, msg, flow, global, expr).' },
    ],
    examples: [
      {
        title: 'Set msg.topic',
        config: 'set · p=topic · to="sensors/temp"',
        result: 'Adds or replaces msg.topic with the literal string.',
      },
      {
        title: 'Move payload to data',
        config: 'move · p=payload · to=data',
        result: 'Renames msg.payload → msg.data, original key is removed.',
      },
      {
        title: 'Read from flow context',
        config: 'set · p=payload · tot=flow · to=lastValue',
        result: 'Reads flow.lastValue and assigns it to msg.payload.',
      },
      {
        title: 'Inline expression — Celsius → Fahrenheit',
        config: 'set · p=payload · tot=expr · to=payload * 1.8 + 32',
        result: 'Computes a fresh value from the current message — no Function Node needed.',
      },
      {
        title: 'Conditional severity tag',
        config: 'set · p=severity · tot=expr · to=payload.value > 50 ? "high" : "low"',
        result: 'Adds a severity field based on a payload threshold.',
      },
    ],
    tips: [
      'Use "delete" to strip sensitive fields before they are forwarded.',
      'Order matters: a later rule sees the result of all earlier rules in the same node.',
      'expr value-type: the expression sees `payload`, `topic`, and `msg` (full message map). Compile errors surface at deploy time as a red node status.',
    ],
  },

  delay: {
    overview:
      'Holds, paces, or jitters the message stream. Three modes: fixed delay (every message held for the same duration), rate limit (at most N messages per interval, with queue or drop overflow), and random delay (uniform jitter between two bounds).',
    inputs: ['Any message. Three optional control fields are honoured and stripped before forwarding: msg.delay (ms — overrides this message\'s wait), msg.flush (releases all pending now), msg.reset (discards all pending).'],
    outputs: ['The forwarded message, after the configured wait. On reset/flush the control message itself is consumed, not forwarded.'],
    properties: [
      { key: 'mode',           desc: '"delay" | "rate" | "random". Selects which set of fields is used.' },
      { key: 'timeout',        desc: '(delay) Hold duration per message.' },
      { key: 'timeoutUnits',   desc: '(delay) milliseconds | seconds | minutes | hours | day.' },
      { key: 'rate',           desc: '(rate) Messages per rateUnits.' },
      { key: 'rateUnits',      desc: '(rate) second | minute | hour | day.' },
      { key: 'behaviour',      desc: '(rate) "queue" drops oldest on overflow; "drop" rejects new ones.' },
      { key: 'maxQueueLength', desc: '(rate, queue behaviour) Buffer size — guards against unbounded memory growth.' },
      { key: 'randomFirst',    desc: '(random) Lower bound of the uniform delay range.' },
      { key: 'randomLast',     desc: '(random) Upper bound. Must be ≥ randomFirst.' },
      { key: 'randomUnits',    desc: '(random) Same unit set as timeoutUnits.' },
    ],
    examples: [
      {
        title: 'Delay every message by 500 ms',
        config: 'mode=delay · timeout=500 · units=milliseconds',
        result: 'FIFO preserved. Stop() discards anything still pending.',
      },
      {
        title: 'Rate-limit a chatty source to 10 msg/s',
        config: 'mode=rate · rate=10 · rateUnits=second · behaviour=queue',
        result: 'Bursts are buffered up to maxQueueLength. Once full, the oldest is dropped to make room.',
      },
      {
        title: 'Stagger trigger fan-out (jitter)',
        config: 'mode=random · randomFirst=0 · randomLast=2000 · units=milliseconds',
        result: 'Each message is held for a uniformly random delay between 0 and 2 s. Order is not preserved.',
      },
    ],
    tips: [
      'msg.delay (number, ms) overrides this single message\'s wait — useful for "delay until" patterns driven from upstream.',
      'msg.flush sends a control message that releases all pending messages immediately. The flush message itself is not forwarded.',
      'msg.reset discards everything currently held without sending. Same control-message semantics as flush.',
      'On flow stop or redeploy, all pending messages are discarded — never replayed.',
    ],
  },

  function: {
    overview:
      'Executes JavaScript code for each incoming message. The variable msg holds the message; return msg (or an array for multiple outputs) to send it on. Returning null suppresses the message.',
    inputs: ['Any message — passed in as the variable msg.'],
    outputs: [
      'Configurable (1+). Return msg for one output, or [msgA, msgB, …] for multiple outputs.',
    ],
    properties: [
      { key: 'code',     desc: 'JavaScript body executed once per message.' },
      { key: 'outputs',  desc: 'Number of output ports (1–N).' },
      { key: 'name',     desc: 'Optional label shown on the node.' },
    ],
    examples: [
      {
        title: 'Pass-through',
        config: 'return msg;',
        result: 'Forwards the message unchanged.',
      },
      {
        title: 'Counter in node-local context',
        config: 'const n = (node.get("count") ?? 0) + 1;\nnode.set("count", n);\nmsg.payload = n;\nreturn msg;',
        result: 'Increments a per-node counter and emits it as payload.',
      },
      {
        title: 'Branch into two outputs',
        config: 'return msg.payload > 100 ? [msg, null] : [null, msg];',
        result: 'High values go out port 1, others out port 2.',
      },
    ],
    tips: [
      'Available context APIs: node.get/set/delete (per-node, in-memory), flow.* and global.* (configurable memory or persistent storage).',
      'Throwing or returning a Promise that rejects routes the message to the catch flow (if any).',
      'Heavy work? Prefer a dedicated node — long-running JS can stall the flow.',
    ],
  },

  'function-go': {
    overview:
      'Runs Go code (interpreted by Yaegi) for each incoming message. Best for batch-numeric work, binary parsing, and algorithms with real control flow that would be slow in JavaScript. Slower than native Go but much faster than the JS Function for compute-heavy workloads.',
    inputs: ['Any message — payload is bound to the handle function\'s first argument.'],
    outputs: [
      'Configurable (1+). Either return a value (goes to port 0, replaces payload) or call node.Send(port, msg) for multi-output / explicit routing.',
    ],
    properties: [
      { key: 'code',    desc: 'Go source. Must be `package main` with a `func handle(...)` whose signature is one of the supported shapes (see examples).' },
      { key: 'outputs', desc: 'Number of output ports (1–N). node.Send(port, …) writes to a specific port.' },
    ],
    examples: [
      {
        title: 'Pass-through',
        config: 'package main\n\nfunc handle(payload any) any {\n    return payload\n}',
        result: 'Forwards the message unchanged.',
      },
      {
        title: 'Filter typed structs',
        config: 'package main\n\ntype Reading struct {\n    Temperature float64 `json:"temperature"`\n}\n\nfunc handle(payload []Reading) []Reading {\n    out := []Reading{}\n    for _, r := range payload {\n        if r.Temperature > 25 { out = append(out, r) }\n    }\n    return out\n}',
        result: 'JSON-marshals incoming records into typed Reading structs, filters in native Go.',
      },
      {
        title: 'Binary parsing',
        config: 'package main\n\nimport "encoding/binary"\n\nfunc handle(payload []byte) uint64 {\n    return binary.BigEndian.Uint64(payload[:8])\n}',
        result: 'Reads a uint64 from the first 8 bytes of an mqtt-in buffer payload.',
      },
      {
        title: 'Multi-output via node.Send',
        config: 'package main\n\nimport "loopzenode"\n\nfunc handle(payload any, node loopzenode.Node) {\n    if v, ok := payload.(int); ok && v > 50 {\n        node.Send(0, payload)\n    } else {\n        node.Send(1, payload)\n    }\n}',
        result: 'Routes high values to port 0, low values to port 1.',
      },
    ],
    tips: [
      'Allowed imports: bytes, encoding/{binary,base64,hex,json}, errors, fmt, math, math/big, math/bits, math/rand, regexp, sort, strconv, strings, time (no Sleep), unicode, unicode/utf8, unicode/utf16. Imports outside this list cause a compile error.',
      'For typed payloads, JSON tags drive the boundary conversion — make sure your struct fields have matching `json:"…"` tags.',
      '[]byte input maps to mqtt-in\'s buffer wire format ([]int) automatically; returning []byte converts back transparently.',
      'loopzenode.Node also exposes Get/Set/Delete (node-scope), Flow{Get,Set,Delete}/FlowGetP (flow scope, P = persistent), and Global{Get,Set,Delete}/GlobalGetP (global scope).',
    ],
  },

  'function-expr': {
    overview:
      'Evaluates a single expr-lang expression for each incoming message. Idiomatic for pipeline transforms (map/filter/reduce), aggregates, and conditional value construction. Faster than the JS Function for batch-numeric work, slower than Go Function — pick by use case.',
    inputs: ['Any message — fields are exposed via the env (payload, topic, msg).'],
    outputs: [
      'One port. Default: a fresh message with topic + the result on the configured output property. With pass-through enabled: the original message with the output property overwritten.',
    ],
    properties: [
      { key: 'expression',     desc: 'expr-lang expression. Compile errors surface at deploy time as a red status.' },
      { key: 'outputProperty', desc: 'Where the result is written on the outgoing message. Dotted paths (payload.value) build nested maps.' },
      { key: 'passThrough',    desc: 'When true, the original message survives and only the output property is overwritten. When false, a new message is emitted carrying just topic + the result.' },
    ],
    examples: [
      {
        title: 'Pipeline aggregate',
        config: 'expression: { avg: mean(map(payload, .temperature)), max: max(map(payload, .temperature)) }',
        result: 'Computes average and max temperature across a list of records.',
      },
      {
        title: 'Conditional severity',
        config: 'expression: payload.value > 50 ? "high" : "low"\noutputProperty: severity\npassThrough: true',
        result: 'Tags the original message with a severity field.',
      },
      {
        title: 'Topic rewrite',
        config: 'expression: topic + "/converted"\noutputProperty: topic\npassThrough: true',
        result: 'Appends a suffix to msg.topic.',
      },
    ],
    tips: [
      'Use map(list, .field) and filter(list, .field > x) for pipeline-style data work.',
      'msg gives you the full message map as an escape-hatch for fields outside payload/topic.',
      'For control flow (loops, mutations) prefer the JS Function or Go Function nodes.',
    ],
  },

  'link-in': {
    overview:
      'Receives messages from one or more Link Output nodes — possibly across different flows. Acts as a virtual entry point without visible wires.',
    outputs: ['The received message, forwarded unchanged.'],
    properties: [
      { key: 'links', desc: 'List of Link Output nodes that send to this Link In.' },
      { key: 'name',  desc: 'Optional label, useful when picking targets in Link Out / Link Call.' },
    ],
    tips: [
      'Use Link nodes to keep large flows readable without spaghetti wiring.',
      'Each Link In can be the target of any number of Link Out / Link Call nodes.',
    ],
  },

  'link-out': {
    overview:
      'Sends messages to one or more Link Input nodes — possibly across different flows. Acts as a virtual exit point without visible wires.',
    inputs: ['Any message — forwarded to the configured Link In(s).'],
    properties: [
      { key: 'links', desc: 'List of Link In targets to forward the message to.' },
      { key: 'name',  desc: 'Optional label.' },
    ],
    tips: [
      'A Link Out with zero targets is a no-op — useful as a placeholder.',
      'Combine with Link Call when you need a request/response pattern instead of fire-and-forget.',
    ],
  },

  'link-call': {
    overview:
      'Sends a request to a Link In and waits for a response. The downstream flow ends with a "return" Function or a flow path that loops back, completing the request. Pairs nicely with reusable subroutines.',
    inputs: ['Request message.'],
    outputs: ['Response message returned by the called sub-flow.'],
    properties: [
      { key: 'linkTarget', desc: 'The Link In node to call.' },
      { key: 'timeout',    desc: 'Maximum wait time (ms) for a response. On timeout the message is rejected.' },
      { key: 'name',       desc: 'Optional label.' },
    ],
    tips: [
      'Treat the called Link In flow like a function: keep it stateless or use flow context for short-term state.',
      'Set a sensible timeout — runaway sub-flows otherwise pile up.',
    ],
  },

  'mqtt-in': {
    overview:
      'Subscribes to one or more MQTT topics on the configured broker and emits a message for every received publish.\n\nTwo modes:\n• Static — one fixed topic from configuration; subscribed at deploy.\n• Dynamic — wait for control messages on the input port; replace subscriptions on each msg.action="subscribe".',
    inputs: [
      '(Dynamic mode only) Control message: msg.action = "subscribe", msg.payload = topic (string) or topics (string[]). Replaces all current subscriptions.',
      '(Dynamic mode only) Optional msg.qos = 0 | 1 | 2 — overrides the configured QoS for this subscribe call. Falls back to the configured QoS if missing or out of range.',
    ],
    outputs: [
      'For each received publish: msg.topic = full topic, msg.payload = body (string), msg.qos, msg.retain.',
    ],
    properties: [
      { key: 'broker', desc: 'MQTT Broker config node (connection details).' },
      { key: 'mode',   desc: 'static (default) — subscribe at deploy. dynamic — wait for control msgs on the input.' },
      { key: 'topic',  desc: 'Static-mode subscription topic. Wildcards + and # are supported.' },
      { key: 'qos',    desc: 'Default Quality of Service: 0 (at most once), 1 (at least once), 2 (exactly once). In dynamic mode, msg.qos can override this per subscribe call.' },
    ],
    tips: [
      'Wildcards: + matches one segment, # matches the rest of the path.',
      'For high-throughput topics consider QoS 0 to avoid broker-side state.',
      'Dynamic mode: send msg.payload = "" or [] to clear all subscriptions. Control messages are NOT forwarded to the output.',
      'Dynamic mode: each subscribe call replaces the previous list — use a single message with all desired topics in an array.',
    ],
  },

  'mqtt-out': {
    overview:
      'Publishes incoming messages as MQTT publishes on the configured broker.\n\nTwo target modes:\n• Topic — publish to the configured topic (or msg.topic).\n• Response to responseTopic — publish to msg.responseTopic and forward msg.correlationData as the v5 Correlation Data property. Pairs with mqtt-request on the requester side.',
    inputs: [
      'msg.payload is sent as the publish body.',
      'Target=Topic: if the configured Topic is empty, msg.topic is used as fallback.',
      'Target=responseTopic: msg.responseTopic must be set (typically delivered by an upstream mqtt-in carrying a v5 Response Topic property). msg.correlationData is forwarded automatically.',
    ],
    properties: [
      { key: 'broker',  desc: 'MQTT Broker config node.' },
      { key: 'target',  desc: 'topic (default) or responseTopic. In responseTopic mode the static topic is ignored and the publish targets msg.responseTopic.' },
      { key: 'topic',   desc: 'Fixed publish topic (Topic mode only). Leave empty to use msg.topic from the incoming message.' },
      { key: 'qos',     desc: 'Publish QoS (0/1/2).' },
      { key: 'retain',  desc: 'Set retained flag — broker keeps the last value for late subscribers.' },
    ],
    tips: [
      'Leave Topic empty when the upstream flow already sets msg.topic — useful for routing where the topic is computed at runtime.',
      'Retained messages are great for "last known state" topics like device shadows.',
      'JSON payloads are auto-stringified; pass a Buffer for raw binary publishes.',
      'Response mode requires MQTT v5 — the responder reads msg.responseTopic / msg.correlationData from a paired mqtt-in.',
    ],
  },

  'mqtt-request': {
    overview:
      'Implements the MQTT v5 request/response pattern. For each input message it generates a unique response topic and 16-byte correlation data, subscribes to the response topic, publishes the request with v5 Response Topic + Correlation Data properties, and waits for the matching response or for the timeout. Multiple inflight requests are supported in parallel.',
    inputs: [
      'msg.payload — the request body (encoded the same way as mqtt-out).',
      'msg.topic — overrides the configured request topic.',
      'msg.qos — overrides the configured QoS for this single request and its response subscription.',
      'msg.userProperties / msg.contentType / msg.messageExpiry / msg.payloadFormat override the configured defaults.',
      'msg.responseTopic and msg.correlationData are IGNORED — the node always generates them itself.',
    ],
    outputs: [
      'On response: the input message is forwarded with msg.payload replaced by the decoded response, msg.topic set to the response topic, msg.requestTopic preserved, plus msg.qos / msg.retain / msg.correlationData and any v5 properties from the response.',
      'On timeout (passthrough mode): the input message with msg.timedOut=true; in error mode no message is emitted (a catchable error is raised instead).',
    ],
    properties: [
      { key: 'broker',              desc: 'MQTT Broker config node. MQTT v5 is required for the request/response properties.' },
      { key: 'topic',               desc: 'Request topic. Falls back to msg.topic when empty.' },
      { key: 'qos',                 desc: 'QoS for both the request publish and the response subscription.' },
      { key: 'retain',              desc: 'Retain flag on the request publish (rare to retain a request).' },
      { key: 'responseTopicPrefix', desc: 'Prefix used to build the random response topic. Default: loopze/response. Full topic: <prefix>/<random>.' },
      { key: 'timeout',             desc: 'How long to wait for the response, in seconds. 0 = no timeout.' },
      { key: 'timeoutMode',         desc: 'error (default) — emit a catchable error on timeout; passthrough — emit msg.timedOut=true on the regular output.' },
      { key: 'responseFormat',      desc: 'string (default) | json | buffer — how msg.payload is decoded for the response.' },
    ],
    tips: [
      'Pair with a remote responder built from mqtt-in → function → mqtt-out (target=Response to responseTopic). The mqtt-out node forwards msg.correlationData automatically so the response matches the request.',
      'Catch errors with a Catch node downstream when timeoutMode=error — the error type is "mqtt-request: timeout".',
      'Use timeoutMode=passthrough to keep request/response and timeout flows on the same wire — a Switch node downstream can branch on msg.timedOut.',
    ],
  },

  'context-watch': {
    overview:
      'Watches a context store for changes and emits a message every time a matching key is set or deleted. Acts like a reactive trigger sourced from context.',
    outputs: [
      'msg.topic = key, msg.payload = new value (or null on delete), msg.operation = "put" | "delete".',
    ],
    properties: [
      { key: 'scope',      desc: 'Which context: global or flow.' },
      { key: 'storage',    desc: 'memory (volatile) or persistent (file-backed).' },
      { key: 'keyPattern', desc: 'NATS KV pattern. Examples: ">" all keys, "user.*" one segment, "session.>" rest of path.' },
    ],
    tips: [
      'Watchers fire for every change — use specific patterns to avoid noise.',
      'Combine with the Context tab in the Information Sidebar to verify keys exist.',
    ],
  },

  catch: {
    overview:
      'Emits a message whenever another node in scope raises an error. Use it to log failures, send notifications or write a dead-letter queue. The Catch Node has no input — it is triggered by error events from the engine, not by upstream wires.',
    outputs: [
      'For each caught error: the original message that was being processed (when available), enriched with msg._error = { message, source: { id, type, name, flowId } }.',
    ],
    properties: [
      { key: 'scope',       desc: 'flow (default): catch errors from this flow. selected: catch only from a chosen list of nodes. all: catch errors across every flow.' },
      { key: 'targetNodes', desc: 'Used when scope = selected. Multi-select of nodes from this flow whose errors should trigger the catch.' },
    ],
    examples: [
      {
        title: 'Dead-letter queue',
        config: 'scope = flow → Change (wrap original + _error) → MQTT out: deadletter',
        result: 'Every failing message in the flow is forwarded to a dead-letter topic instead of being silently dropped.',
      },
      {
        title: 'Error notifications for critical nodes',
        config: 'scope = selected, targetNodes = [HTTP request, DB insert] → Function (format) → MQTT out: alerts',
        result: 'Only failures of the picked nodes raise an alert; routine errors elsewhere stay quiet.',
      },
    ],
    tips: [
      'Catch is for logging and notifications, not retries. Errors raised along a catch branch are intentionally not re-caught — wiring a catch back into the failing node would otherwise loop.',
      'Multiple Catch Nodes can coexist; each one independently fans out for every matching error.',
      'Errors from Catch Nodes themselves are filtered at the engine level — they never trigger another catch.',
    ],
  },

  's7-plc': {
    overview:
      'Connection-config node for a SIEMENS S7 PLC over RFC1006/ISO-on-TCP (port 102). Holds host/port/rack/slot, manages the gos7 client lifecycle, exposes the negotiated PDU size and reconnect state to read/write nodes that reference this PLC, and offers a Test Connection endpoint for round-trip verification.',
    properties: [
      { key: 'name',           desc: 'Display name (used in topics like s7/<name> and as the PLC label in dropdowns).' },
      { key: 'host',           desc: 'PLC IP address or hostname.' },
      { key: 'port',           desc: 'TCP port. Default 102 (ISO-on-TCP). The demo PLC uses 1102 to avoid sudo.' },
      { key: 'connectionType', desc: 'CPU family preset: S7-1200/1500 (rack 0, slot 1), S7-300/400 (rack 0, slot 2), LOGO!/S7-200 Smart, or Custom (manual rack/slot).' },
      { key: 'rack / slot',    desc: 'Custom-mode only. Auto-derived for the other presets.' },
      { key: 'timeout',        desc: 'Connect/read/write timeout in milliseconds. Defaults to 5000 ms.' },
      { key: 'reconnectDelay', desc: 'Backoff between reconnect attempts after the link drops.' },
    ],
    tips: [
      'For S7-1200/1500: in TIA Portal you must un-tick "Optimized block access" on each DB you want to address with classical wire forms (DB10.DBD0, …) — Optimized DBs are only reachable via OPC UA.',
      '"Test Connection" sends a Connect + GetCpuInfo round-trip and surfaces the PLC firmware/order-code so you can confirm the right CPU is reachable.',
      'Multiple read/write nodes can share one PLC config — the underlying client is pooled and serialised per connection (matching real S7 PLCs).',
      'PLC connection status is propagated to every referencing node\'s status pill (green = connected, red = link down with reason).',
    ],
  },

  's7-read': {
    overview:
      'Reads variables from a SIEMENS S7 PLC. Three modes: Static (cyclic poll of a fixed list), Dynamic (read on every input message — list comes from the message, falls back to the configured list), Block (raw byte fetch from a contiguous area, typically piped into an s7-parser).',
    inputs: [
      '(Dynamic mode) Any message triggers a read. Override the list per message via msg.variables = [{name?, address, dataType}] or use the convenience form msg.address + msg.dataType for a single read.',
      '(Block mode + triggerOnInput) Any message forces an extra read on top of the cyclic poll. Override the area/db/start/length per message via msg.s7.{area,db,start,length}.',
    ],
    outputs: [
      'Variables modes: msg.payload shape depends on Output shape (single = bare value, array = per-variable list, object = name-keyed map). msg.s7 carries the per-variable metadata (address, dataType, value or error).',
      'Block mode: msg.payload = []int (byte array as numbers, JSON-friendly). msg.s7 carries the area/db/start/length descriptor. Pipe into s7-parser to decode.',
    ],
    properties: [
      { key: 'plc',           desc: 'Reference to an s7-plc config node.' },
      { key: 'mode',          desc: 'static (poll) | dynamic (msg-triggered) | block (raw bytes).' },
      { key: 'variables',     desc: 'List of {name, address, dataType, scale?, offset?} entries. Required for static, optional for dynamic (used as fallback).' },
      { key: 'block',         desc: 'Block-mode descriptor: { area: DB|M|I|Q, db?, start, length, triggerOnInput? }. Length is capped by the negotiated PDU; oversized blocks are auto-split.' },
      { key: 'outputShape',   desc: 'Variables-mode output: single (1 var only) | array | object. Defaults: single for 1 var, object for 2+.' },
      { key: 'pollInterval',  desc: 'Static/block mode polling period in ms. Default 1000.' },
      { key: 'emitOnChange',  desc: 'Suppress duplicate emits when the read result is unchanged.' },
      { key: 'emitOnError',   desc: 'Emit a separate error message on read failure (in addition to routing to a Catch node).' },
      { key: 'topicTemplate', desc: 'Optional topic format. Placeholders: <plc-name>, <address>, <name>, <area>, <db>, <start>, <length>.' },
    ],
    examples: [
      {
        title: 'Cyclic poll, single value',
        config: 'mode=static · variables=[{name:"Temp", address:"DB1.DBD0", dataType:"real"}] · pollInterval=1000',
        result: 'Emits msg.payload = <float> every second.',
      },
      {
        title: 'Multi-variable object read',
        config: 'mode=static · 3 variables · outputShape=object',
        result: 'msg.payload = { Temp: 21.3, Pressure: 1.02, Tick: 1234 } per poll.',
      },
      {
        title: 'On-demand single read via dynamic mode',
        config: 'mode=dynamic · upstream sends {address:"DB10.DBD0", dataType:"real"}',
        result: 'Each trigger reads the requested address and emits the decoded value.',
      },
      {
        title: 'Block read for parser pipeline',
        config: 'mode=block · area=DB · db=3 · start=0 · length=600',
        result: 'Emits msg.payload = []int (600 bytes), auto-split across PDUs. Pipe into s7-parser with a layout to decode fields.',
      },
    ],
    tips: [
      'Address forms: DB.<DBX|DBB|DBW|DBD|DBL|DTL|STRING|WSTRING>, M/MB/MW/MD, I/IB/IW/ID, Q/QB/QW/QD, C, T. The spec at specifications/issues/NODE_S7.md has the full table.',
      'Multi-variable reads use a single AGReadMulti round-trip when they fit the negotiated PDU (typically 462 bytes payload at PDU=480) — much cheaper than N separate reads.',
      'Per-variable scaling: value = raw × scale + offset. Skipped automatically for string/raw/date/dt/dtl/wchar/counter/timer (no numeric meaning).',
      '64-bit types (LREAL, LINT, ULINT, LWORD, LTIME, LTOD, LDT, DT) use the LOOPZE-coined DBL form (e.g. DB10.DBL16 for 8-byte access). DTL has its own DTL form (12 bytes).',
      'Block mode emits []int (not []byte) so the Debug panel shows decimal bytes instead of base64 — the s7-parser accepts both forms.',
    ],
  },

  's7-write': {
    overview:
      'Writes variables to a SIEMENS S7 PLC. Three modes: Static (fixed list, values from baked-in config or msg fields), Dynamic (variable list comes from the message — sidebar list is ignored), Block (raw bytes from msg.payload sent in one AGWriteArea call).',
    inputs: [
      'Static mode: triggers one write of the configured list. Per-variable values come from valueSource: static value baked into config, or msg.<valuePath>.',
      'Dynamic mode: msg.variables = [{address, dataType, value}] for the full form, or msg.address + msg.dataType + msg.payload for a single write. The configured sidebar list is NOT used.',
      'Block mode: msg.<inputProperty> (default payload) carries the byte slice. Override target via msg.s7.{area,db,start} per message.',
    ],
    outputs: [
      'Default: silent (no output). Enable Emit ACK or Pass-through on the Output panel to surface results.',
      'Emit ACK: a fresh message with msg.payload = allOk (boolean) and msg.s7Write = { plc, results: [{address, dataType, ok, error?}], allOk }.',
      'Pass-through: the input message is forwarded unchanged with msg.s7Write enriched.',
    ],
    properties: [
      { key: 'plc',          desc: 'Reference to an s7-plc config node.' },
      { key: 'mode',         desc: 'static | dynamic | block.' },
      { key: 'variables',    desc: 'Static-mode list: [{address, dataType, valueSource: "static"|"msg", value? | valuePath?, scale?, offset?}]. Hidden in dynamic mode (the runtime ignores it there).' },
      { key: 'block',        desc: 'Block-mode descriptor: { area, db?, start, length?, inputProperty? }. length=0 means use the incoming buffer length.' },
      { key: 'emitAck',      desc: 'Emit a fresh ACK message after every write attempt. Mutually exclusive with passthrough.' },
      { key: 'passthrough',  desc: 'Forward the input message with msg.s7Write metadata added. Mutually exclusive with emitAck.' },
    ],
    examples: [
      {
        title: 'Static write, value from message',
        config: 'mode=static · variables=[{address:"DB100.DBW12", dataType:"int", valueSource:"msg", valuePath:"payload.setpoint"}]',
        result: 'Writes msg.payload.setpoint as a 16-bit signed int to DB100.DBW12 on every input.',
      },
      {
        title: 'Static write, baked-in value',
        config: 'mode=static · variables=[{address:"M0.0", dataType:"bool", valueSource:"static", value:true}]',
        result: 'Sets the bit on every input message — useful as an "arm" command on a trigger.',
      },
      {
        title: 'Dynamic single write (convenience form)',
        config: 'mode=dynamic · upstream sends {address:"DB100.DBW12", dataType:"int", payload:1500}',
        result: 'Writes 1500 to the address provided in the message. No sidebar config needed.',
      },
      {
        title: 'Block write of pre-encoded bytes',
        config: 'mode=block · area=DB · db=1 · start=12 · upstream sends msg.payload=[]int',
        result: 'Sends the byte slice in one AGWriteArea call (auto-split on PDU). Pair with s7-parser action=encode upstream to build the buffer.',
      },
    ],
    tips: [
      'Per-variable encoding errors (out-of-range, type mismatch) land in msg.s7Write.results[i].error — the surviving items still get written. Whole-transaction failures (transport, lost connection) escalate to a catchable error and no ACK.',
      'Counters and Timers are NOT writable — Siemens treats them as CPU-internal state. The encoder rejects with a clear message so misuse is obvious.',
      'Date / DT / LDT / DTL accept RFC3339 strings (e.g. "2026-05-10T12:34:56Z"); DATE additionally accepts ISO date strings ("2026-05-10").',
      'Block mode on area=I (Inputs / PE) is rejected — inputs are read-only on the wire.',
      'Use Static mode with valueSource="msg" when you have a fixed address but the value flows through messages (the most common pattern). Dynamic mode is for cases where the address itself varies per message.',
    ],
  },

  's7-parser': {
    overview:
      'Parses raw S7 byte blocks into structured objects (parse direction) or builds raw byte blocks from objects (encode direction). The layout is configured declaratively as a list of fields and is shared by both directions — pair it with s7-read block mode (parse) or s7-write block mode (encode).',
    inputs: [
      'Parse direction: msg.<parseFrom> (default payload) carries the byte block — accepts []byte, []int, or []any with numeric elements. Typical upstream is s7-read in block mode.',
      'Encode direction: msg.<encodeFrom> (default payload) carries an object whose keys match the layout field names. Missing keys are emitted as zero bytes (sparse-zero semantics).',
    ],
    outputs: [
      'Parse direction: msg.payload = { fieldName: value, … } based on the layout. With preserveBytes enabled the raw bytes are forwarded as msg.bytes ([]int).',
      'Encode direction: msg.payload = []int (the wire bytes) and msg.bytes mirrors the same value. Pipe directly into s7-write block mode.',
    ],
    properties: [
      { key: 'action',        desc: 'auto (sniff input shape) | parse (force decode) | encode (force build). Auto: object input → encode; everything else → parse.' },
      { key: 'parseFrom',     desc: 'Message field carrying the byte block on parse. Default payload.' },
      { key: 'encodeFrom',    desc: 'Message field carrying the object on encode. Default payload.' },
      { key: 'blockLength',   desc: 'Fixed output buffer size in bytes (0 = derive from layout extent). Larger than the layout pads with zeros; smaller than required is rejected at deploy.' },
      { key: 'preserveBytes', desc: 'Parse only — also forward the raw bytes as msg.bytes ([]int).' },
      { key: 'layout',        desc: 'Ordered list of {name, offset, type, length?, signed?, scale?, valueOffset?, unit?}. Offsets are buffer-relative (0-based), independent of the PLC byte address.' },
    ],
    examples: [
      {
        title: 'Decode a sensor block from s7-read',
        config: 'layout=[{name:"temp", offset:0, type:"real"}, {name:"pressure", offset:4, type:"real"}, {name:"setpoint", offset:8, type:"int"}, {name:"running", offset:10.0, type:"bool"}]',
        result: 'Parses 11 raw bytes into { temp, pressure, setpoint, running }.',
      },
      {
        title: 'Build a write buffer for s7-write block mode',
        config: 'action=encode · same layout · upstream sends payload={temp: 21.5, pressure: 1.02, setpoint: 200, running: true}',
        result: 'Emits msg.payload = []int (wire bytes). Pipe into s7-write with block.start matching the layout\'s zero offset.',
      },
      {
        title: 'Pack BOOL status word',
        config: 'layout=[{name:"alarm", offset:"0.0", type:"bool"}, {name:"warn", offset:"0.1", type:"bool"}, {name:"running", offset:"0.7", type:"bool"}]',
        result: 'Multiple BOOLs sharing one host byte are OR-aggregated on encode.',
      },
    ],
    tips: [
      'Field offsets are buffer-relative — a layout starting at offset 0 is portable across DBs. The s7-write block.start positions the buffer at the right PLC byte.',
      'BOOL offsets use the dotted "byte.bit" form (e.g. "12.3" = byte 12 bit 3); other types take a plain integer.',
      'Auto mode dispatches based on input shape — same node serves both directions. Use parse / encode to force one path when the input shape is ambiguous.',
      'Scaling (scale / valueOffset) applies to numeric scalars only. String/raw/date/dt/dtl/wchar/counter/timer fields skip it.',
      'Sparse-zero on encode: missing keys leave their byte range at zero. Useful when only some fields of a larger DB layout are mutating.',
    ],
  },

  statemachine: {
    overview:
      'Drives a finite state machine. Each incoming message is treated as an event; the machine consumes it, optionally runs guards/actions, and transitions to a new state. Supports delayed transitions.',
    inputs: ['msg.payload (or msg.event) is interpreted as the event name.'],
    outputs: [
      'On state change: a message describing the transition (from, to, event).',
    ],
    properties: [
      { key: 'machine', desc: 'JSON definition: { initial, states: { name: { on: { event: { target, guard?, action? } } } } }.' },
      { key: 'persist', desc: 'Persist current state in flow context across restarts.' },
      { key: 'name',    desc: 'Optional label for the node.' },
    ],
    tips: [
      'Guards are JS expressions evaluated against the message — return falsy to block the transition.',
      'Use delayed transitions ("after": ms) for timeout behaviours.',
      'A Debug node downstream is useful for tracing transitions during development.',
    ],
  },
}

export function getNodeHelpDoc(nodeType: string | undefined | null): NodeHelpDoc | null {
  if (!nodeType) return null
  return nodeHelpDocs[nodeType] ?? null
}
