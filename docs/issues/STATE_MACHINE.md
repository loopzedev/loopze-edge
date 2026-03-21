# State Machine Node — Beispiel

## Szenario: Türschloss mit PIN-Code

Eine Tür hat 3 Zustände: `locked`, `unlocked`, `alarm`.
- Entsperren nur mit korrektem PIN (Guard)
- Nach 3 Fehlversuchen → Alarm (Action zählt Versuche)
- Nach 10 Sekunden Unlock → automatisch wieder locked (Delayed Transition)

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
      node.log("Fehlversuch #" + ctx.failedAttempts);

      // Nach maxAttempts → Alarm auslösen
      if (ctx.failedAttempts >= ctx.maxAttempts) {
        node.send({ topic: "TRIGGER_ALARM", payload: {} });
      }
    },
    resetFailures: function(ctx, event) {
      ctx.failedAttempts = 0;
    },
    notifyOpen: function(ctx, event) {
      // Nachricht auf Port 1 senden (z.B. an MQTT)
      node.send({ topic: "door/status", payload: { open: true } });
    },
    triggerAlarm: function(ctx, event) {
      node.warn("ALARM: Zu viele Fehlversuche!");
      node.send({ topic: "alarm/door", payload: { reason: "too_many_attempts", attempts: ctx.failedAttempts } });
    },
    logLocked: function(ctx, event) {
      node.log("Tür verriegelt");
    }
  }
}
```

---

## Flow-Beispiel

```
[Inject: topic="UNLOCK", payload={"pin":"0000"}]  →  [State Machine]  →  Port 0: [Debug: State Changes]
[Inject: topic="LOCK"]                             →        ↓
                                                      Port 1: [MQTT Out: Action Messages]
```

### Test-Sequenz

| # | Input (msg.topic / payload)             | Erwarteter State | Port 0 Output                        | Port 1 Output                    |
|---|----------------------------------------|------------------|--------------------------------------|----------------------------------|
| 1 | `UNLOCK` / `{"pin":"0000"}`            | `locked`         | `changed: false` (Guard blockiert)   | —                                |
| 2 | `WRONG_PIN` / `{}`                     | `locked`         | `changed: false`, failedAttempts=1   | —                                |
| 3 | `WRONG_PIN` / `{}`                     | `locked`         | failedAttempts=2                     | —                                |
| 4 | `WRONG_PIN` / `{}`                     | `locked`         | failedAttempts=3                     | `TRIGGER_ALARM` (von Action)     |
| 5 | `TRIGGER_ALARM`                        | `alarm`          | `locked→alarm`                       | `alarm/door` Nachricht           |
| 6 | `RESET` / `{}`                         | `locked`         | `alarm→locked`                       | —                                |
| 7 | `UNLOCK` / `{"pin":"1234"}`            | `unlocked`       | `locked→unlocked`                    | `door/status: {open: true}`      |
| 8 | *(10s warten)*                         | `locked`         | `unlocked→locked` (auto-timer)       | —                                |

---

## Was hier demonstriert wird

- **Guards:** `pinCorrect` prüft den PIN bevor die Transition erlaubt wird
- **Actions mit Context:** `countFailure` zählt Fehlversuche im Machine-Context hoch
- **Entry-Actions:** `notifyOpen` und `triggerAlarm` senden Nachrichten beim Betreten eines States
- **node.send():** Actions können Messages auf Port 1 ausgeben (z.B. an MQTT, Debug, etc.)
- **node.log/warn():** Diagnostic-Output im Debug-Panel
- **Delayed Transitions:** `after: {"10000": "locked"}` — automatisches Verriegeln nach 10s
- **Self-Transitions:** `WRONG_PIN` bleibt in `locked`, führt aber die Action aus
