# State Machine Node — Example

## Scenario: Door Lock with PIN Code

A door has 3 states: `locked`, `unlocked`, `alarm`.
- Unlock only with the correct PIN (guard)
- After 3 failed attempts -> alarm (action counts attempts)
- After 10 seconds of unlock -> automatically locked again (delayed transition)

---

## Machine Definition (JSON)

```json
{
  "id": "doorLock",
  "initial": "locked",
  "context": {
    "failedAttempts": 0,
    "maxAttempts": 3
  },
  "states": {
    "locked": {
      "onEntry": ["logLocked"],
      "on": {
        "UNLOCK": { "target": "unlocked", "guard": "pinCorrect" },
        "WRONG_PIN": { "target": "locked", "actions": ["countFailure"] },
        "TRIGGER_ALARM": "alarm"
      }
    },
    "unlocked": {
      "onEntry": ["resetFailures", "notifyOpen"],
      "after": {
        "10000": "locked"
      },
      "on": {
        "LOCK": "locked"
      }
    },
    "alarm": {
      "onEntry": ["triggerAlarm"],
      "on": {
        "RESET": { "target": "locked", "actions": ["resetFailures"] }
      }
    }
  }
}
```

## Guards & Actions (JavaScript)

```javascript
return {
  guards: {
    pinCorrect: function(ctx, event) {
      return event.pin === "1234";
    }
  },
  actions: {
    countFailure: function(ctx, event) {
      ctx.failedAttempts = (ctx.failedAttempts || 0) + 1;
      node.log("Failed attempt #" + ctx.failedAttempts);

      // After maxAttempts -> trigger alarm
      if (ctx.failedAttempts >= ctx.maxAttempts) {
        node.send({ topic: "TRIGGER_ALARM", payload: {} });
      }
    },
    resetFailures: function(ctx, event) {
      ctx.failedAttempts = 0;
    },
    notifyOpen: function(ctx, event) {
      // Send message on port 1 (e.g. to MQTT)
      node.send({ topic: "door/status", payload: { open: true } });
    },
    triggerAlarm: function(ctx, event) {
      node.warn("ALARM: too many failed attempts!");
      node.send({ topic: "alarm/door", payload: { reason: "too_many_attempts", attempts: ctx.failedAttempts } });
    },
    logLocked: function(ctx, event) {
      node.log("Door locked");
    }
  }
}
```

---

## Flow Example

```
[Inject: topic="UNLOCK", payload={"pin":"0000"}]  ->  [State Machine]  ->  Port 0: [Debug: State Changes]
[Inject: topic="LOCK"]                             ->        v
                                                      Port 1: [MQTT Out: Action Messages]
```

### Test Sequence

| # | Input (msg.topic / payload)             | Expected State   | Port 0 Output                        | Port 1 Output                    |
|---|----------------------------------------|------------------|--------------------------------------|----------------------------------|
| 1 | `UNLOCK` / `{"pin":"0000"}`            | `locked`         | `changed: false` (guard blocks)      | -                                |
| 2 | `WRONG_PIN` / `{}`                     | `locked`         | `changed: false`, failedAttempts=1   | -                                |
| 3 | `WRONG_PIN` / `{}`                     | `locked`         | failedAttempts=2                     | -                                |
| 4 | `WRONG_PIN` / `{}`                     | `locked`         | failedAttempts=3                     | `TRIGGER_ALARM` (from action)    |
| 5 | `TRIGGER_ALARM`                        | `alarm`          | `locked->alarm`                      | `alarm/door` message             |
| 6 | `RESET` / `{}`                         | `locked`         | `alarm->locked`                      | -                                |
| 7 | `UNLOCK` / `{"pin":"1234"}`            | `unlocked`       | `locked->unlocked`                   | `door/status: {open: true}`      |
| 8 | *(wait 10s)*                           | `locked`         | `unlocked->locked` (auto-timer)      | -                                |

---

## What This Demonstrates

- **Guards:** `pinCorrect` checks the PIN before the transition is allowed
- **Actions with context:** `countFailure` increments failed attempts in the machine context
- **Entry actions:** `notifyOpen` and `triggerAlarm` send messages when entering a state
- **node.send():** Actions can emit messages on port 1 (e.g. to MQTT, Debug, etc.)
- **node.log/warn():** Diagnostic output in the debug panel
- **Delayed transitions:** `after: {"10000": "locked"}` — automatic locking after 10s
- **Self-transitions:** `WRONG_PIN` stays in `locked` but executes the action
