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
      { key: 'to',     desc: 'New value (literal, msg.path, flow./global. context, or expression).' },
      { key: 'tot',    desc: 'How "to" is interpreted (string, num, bool, json, env, msg, flow, global).' },
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
    ],
    tips: [
      'Use "delete" to strip sensitive fields before they are forwarded.',
      'Order matters: a later rule sees the result of all earlier rules in the same node.',
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
      'Publishes incoming messages as MQTT publishes on the configured broker.',
    inputs: [
      'msg.payload is sent as the publish body.',
      'If the configured Topic is empty, msg.topic is used as a fallback. If both are empty, the publish fails.',
    ],
    properties: [
      { key: 'broker',  desc: 'MQTT Broker config node.' },
      { key: 'topic',   desc: 'Fixed publish topic. Leave empty to use msg.topic from the incoming message.' },
      { key: 'qos',     desc: 'Publish QoS (0/1/2).' },
      { key: 'retain',  desc: 'Set retained flag — broker keeps the last value for late subscribers.' },
    ],
    tips: [
      'Leave Topic empty when the upstream flow already sets msg.topic — useful for routing where the topic is computed at runtime.',
      'Retained messages are great for "last known state" topics like device shadows.',
      'JSON payloads are auto-stringified; pass a Buffer for raw binary publishes.',
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
