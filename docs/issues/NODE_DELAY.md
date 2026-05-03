# Issue: Delay Node — Delay, Rate Limit, Queue

## Status: Done

## Problem Description

Currently there is no way in the flow to influence the message flow temporally. As soon as a message enters the flow, it runs synchronously through all downstream nodes. Three very common requirements are therefore not covered:

1. **Delay** — forward each message X time later (e.g. wait 500 ms before an MQTT publish follows)
2. **Rate limit** — protect downstream systems: let through at most N messages per time interval
3. **Random delay** — e.g. to stagger load on distributed triggers (jitter)

This is to be implemented as **one** node type `delay` with a mode switch — analogous to Node-RED. One node with three modes is easier to find, explain, and maintain than three separate node types that overlap by 80%.

`delay` is already declared in the frontend as a `NodeType` (`frontend/src/types/flow.ts`) and assigned to the `process` category in `tokens.ts`. Missing are the backend, the config UI, and the registration.

## Modes

### Mode 1: `delay` — fixed delay per message

Each incoming message is emitted delayed by `timeout`. Order is preserved (FIFO) since all messages have the same timeout.

```json
{
  "mode": "delay",
  "timeout": 500,
  "timeoutUnits": "milliseconds"
}
```

### Mode 2: `rate` — rate limiting

At most `rate` messages per `rateUnits` are let through. On overflow, two strategies:

| `behaviour` | Behavior |
|---|---|
| `queue` | Excess messages are buffered and emitted at an even pace |
| `drop` | Excess messages are discarded (with status update on the node) |

```json
{
  "mode": "rate",
  "rate": 10,
  "rateUnits": "second",
  "behaviour": "queue",
  "maxQueueLength": 1000
}
```

**Why queue limit:** without a limit a producer can blow up the heap. `maxQueueLength` (default `1000`) discards the oldest entries and reports `status: warning`.

### Mode 3: `random` — random delay

Random value from `[randomFirst, randomLast]` milliseconds per message.

```json
{
  "mode": "random",
  "randomFirst": 100,
  "randomLast": 5000,
  "randomUnits": "milliseconds"
}
```

Since the delays differ, **the order may change** — that is intentional and part of the use case (jitter).

## Override per Message

Consistent with Node-RED behavior. These fields, if set on the incoming message, override the configuration **only for this message**:

| Field | Effect |
|---|---|
| `msg.delay` | Delay in milliseconds for this message (overrides `timeout`/`random*`) |
| `msg.reset` | (truthy) — discards all currently buffered/waiting messages, message is **not** forwarded |
| `msg.flush` | (truthy) — emits all buffered messages **immediately** (bypasses timer/rate), message itself is not forwarded |

These three fields are **removed** from the message before further processing so they do not leak into subsequent nodes (project convention, cf. `_linkSource` in Link Out).

## Configuration (complete)

```json
{
  "mode": "delay",
  "timeout": 500,
  "timeoutUnits": "milliseconds",

  "rate": 1,
  "rateUnits": "second",
  "behaviour": "queue",
  "maxQueueLength": 1000,

  "randomFirst": 0,
  "randomLast": 1000,
  "randomUnits": "milliseconds"
}
```

Not every mode needs every field — the config UI shows the relevant fields for each. On the backend side, however, all fields are always parsed and only those matching the active `mode` are evaluated.

`*Units` is one of: `milliseconds`, `seconds`, `minutes`, `hours`, `day`. Backend converts to `time.Duration` once in `Init()`.

## Behavior in Detail

### Order

- **delay** (fixed timeout): FIFO is preserved. Simple `time.AfterFunc` per message is fine since all have the same wait time.
- **rate / queue**: strict FIFO. The queue is a `chan *flow.Message` of size `maxQueueLength`.
- **random**: can break the order. Document.

### Stop()

When the flow is stopped (redeploy, shutdown): **pending messages are discarded**, not let through. Rationale: whoever stops a flow wants a clean cut — otherwise messages would still fire after `Stop()` into a half-torn-down flow. The current node status should report this on stop.

### Status

| State | Status |
|---|---|
| Idle | empty |
| Pending messages > 0 | `blue ring` with count, e.g. "queued: 23" |
| Drop in `behaviour: drop` | `yellow dot` "rate-limited" for ~1 s after each drop |
| Queue full, oldest entry discarded | `yellow dot` "queue full: dropped oldest" |

## Implementation

### Backend (`internal/nodes/delay.go`)

Skeleton of the data structure:

```go
type DelayNode struct {
    config flow.NodeConfig
    send   flow.SendFunc
    status flow.StatusFunc

    mode           string         // "delay" | "rate" | "random"
    timeout        time.Duration  // for "delay"
    randomMin      time.Duration  // for "random"
    randomMax      time.Duration  // for "random"
    rateInterval   time.Duration  // for "rate" — derivable from rate/rateUnits
    behaviour      string         // "queue" | "drop"
    maxQueueLength int

    queue chan *flow.Message    // for "rate"
    done  chan struct{}
    wg    sync.WaitGroup
    rng   *rand.Rand             // for random mode (with mutex if shared)
}
```

A separate processing logic per mode:

- **delay / random:** `HandleMessage` starts a goroutine per message (`time.AfterFunc` or `time.NewTimer` + `select` on `done`) that calls `n.send(0, msg)` after expiry. Goroutines are cheap in Go — at realistic loads (<= 10k pending) completely unproblematic. **Optimize only when we actually see >100k pending — then a min-heap (`container/heap`) with a worker goroutine is the right answer. Until then YAGNI.**
- **rate:** `Start()` starts a worker goroutine that reads from `queue` on a `time.Ticker(rateInterval)` and sends. `HandleMessage` writes into `queue` (with drop or drop-oldest strategy when full).

### Override Handling

In `HandleMessage`, first read the three override fields and remove them from the message before further processing:

```go
if reset, _ := msg.GetBool("reset"); reset {
    n.resetQueue()
    return nil, nil
}
if flush, _ := msg.GetBool("flush"); flush {
    n.flushQueue()
    return nil, nil
}
if d, ok := msg.GetNumber("delay"); ok {
    msg.Delete("delay")
    n.scheduleAfter(time.Duration(d)*time.Millisecond, msg)
    return nil, nil
}
```

(Map the exact helper names — `GetBool`, `GetNumber`, `Delete` — to the existing `flow.Message` API.)

### Frontend (`frontend/src/components/config/DelayConfig.vue`)

The config UI switches the displayed form set based on `mode`:

```
+----------------------------------------------+
| Action                                       |
| ( ) Delay each message                       |
| ( ) Rate Limit messages                      |
| ( ) Random delay                             |
|                                              |
| -- (mode = delay) ---------------------      |
| For [   500   ] [milliseconds v]             |
|                                              |
| -- (mode = rate) ----------------------      |
| Rate     [   1   ]  msg / [second v]         |
| Queue    ( ) Drop intermediate               |
|          (o) Queue intermediate              |
| Max queue length [ 1000 ]                    |
|                                              |
| -- (mode = random) --------------------      |
| Between [  0  ] and [ 5000 ] [ms v]          |
+----------------------------------------------+
```

Validation in the frontend: positive numbers, `randomLast >= randomFirst`. On violation: `FormInput` with error border and submit block (pattern as in the other config components).

### Help Doc

Entry `delay` in `frontend/src/components/help/docs.ts` with overview, properties list, three examples (one per mode), and an explicit tip on `msg.delay` / `msg.reset` / `msg.flush`.

### Registration

`internal/server/server.go` in `registerNodes()`:

```go
registry.Register("delay", nodes.NewDelayNode, nodes.DelayTypeInfo())
```

`frontend/src/components/config/configEditors.ts`: dispatch for `delay` to `DelayConfig.vue`.

## Affected Files

| File | Change |
|---|---|
| `internal/nodes/delay.go` | **New** — node implementation, all three modes |
| `internal/nodes/delay_test.go` | **New** — tests see below |
| `internal/server/server.go` | Registration in `registerNodes()` |
| `frontend/src/components/config/DelayConfig.vue` | **New** — config UI with mode switch |
| `frontend/src/components/config/configEditors.ts` | Dispatch for `delay` |
| `frontend/src/components/help/docs.ts` | Help entry `delay` |
| `docs/MISSING_FUNCTIONALITY.md` / `PLANNING.md` | Mark delay as done |

## Tests

| Test | Checks |
|---|---|
| `TestDelayMode_FixedTimeout` | Timeout = 100 ms -> message arrives >= 100 ms later, order preserved |
| `TestDelayMode_MsgDelayOverride` | `msg.delay = 50` overrides config timeout |
| `TestDelayMode_RemovesOverrideField` | `msg.delay` is **no longer present** on the outgoing message |
| `TestRandomMode_WithinBounds` | 100 samples lie in `[min, max]` |
| `TestRateMode_QueueRespectsRate` | 10 fast messages at rate=1/100ms -> output spacing >= 100ms |
| `TestRateMode_DropBehaviour` | `behaviour=drop`: burst of 10 -> only 1 let through, 9 dropped |
| `TestRateMode_QueueOverflow` | Queue full -> oldest entry discarded, status update |
| `TestMsgFlush` | Queue with 5 items + `msg.flush=true` -> all 5 emitted immediately |
| `TestMsgReset` | Queue with 5 items + `msg.reset=true` -> all 5 discarded, no output |
| `TestStopDiscardsPending` | 100 messages in delay -> `Stop()` -> no more sends after stop |
| `TestUnitsConversion` | `timeoutUnits: "seconds", timeout: 2` -> 2 s delay |

## Deliberately Out of Scope

- **Per-topic rate limit** (Node-RED has this via checkbox). Used rarely enough to add later as a separate feature if anyone asks for it.
- **Persistent queue across restarts.** Would require NATS stream backing, overkill for the majority of use cases. Currently: stop = discard.
- **Backpressure on the producer side.** Nodes have no backpressure API; with `behaviour: drop` and `maxQueueLength` the behavior is defined.

## Dependencies

None. All required building blocks (`flow.SendFunc`, `flow.StatusFunc`, `flow.Message`, config editor dispatch, help doc system) are present.
