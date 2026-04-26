# Issue: Delay Node — Verzögerung, Rate-Limit, Queue

## Status: Done

## Problembeschreibung

Aktuell gibt es im Flow keine Möglichkeit, den Nachrichtenfluss zeitlich zu beeinflussen. Sobald eine Message in den Flow eintritt, läuft sie synchron durch alle nachgelagerten Nodes durch. Drei sehr häufige Anforderungen sind damit nicht abgedeckt:

1. **Verzögerung** — jede Message um X Zeit später weiterleiten (z.B. 500 ms warten, bevor ein MQTT-Publish folgt)
2. **Rate-Limit** — Schutz nachgelagerter Systeme: höchstens N Messages pro Zeitintervall durchlassen
3. **Random Delay** — z.B. um Last bei verteilten Triggern zu staffeln (Jitter)

Dies ist als **ein** Node-Typ `delay` mit Mode-Switch umzusetzen — analog zu Node-RED. Ein Node mit drei Modi ist einfacher zu finden, zu erklären und zu warten als drei separate Node-Typen, die sich zu 80 % überschneiden.

`delay` ist im Frontend bereits als `NodeType` deklariert (`frontend/src/types/flow.ts`) und in `tokens.ts` der `process`-Kategorie zugeordnet. Es fehlt das Backend, die Config-UI und die Registrierung.

## Modi

### Mode 1: `delay` — feste Verzögerung pro Message

Jede eingehende Message wird um `timeout` verzögert ausgegeben. Reihenfolge bleibt erhalten (FIFO), da alle Messages denselben Timeout haben.

```json
{
  "mode": "delay",
  "timeout": 500,
  "timeoutUnits": "milliseconds"
}
```

### Mode 2: `rate` — Rate-Limiting

Maximal `rate` Messages pro `rateUnits` werden durchgelassen. Bei Überschreitung zwei Strategien:

| `behaviour` | Verhalten |
|---|---|
| `queue` | Überzählige Messages werden gepuffert und gleichmäßig ausgegeben |
| `drop` | Überzählige Messages werden verworfen (mit Status-Update am Node) |

```json
{
  "mode": "rate",
  "rate": 10,
  "rateUnits": "second",
  "behaviour": "queue",
  "maxQueueLength": 1000
}
```

**Warum Queue-Limit:** Ohne Limit kann ein Producer den Heap zur Explosion bringen. `maxQueueLength` (Default `1000`) verwirft die ältesten Einträge und meldet `status: warning`.

### Mode 3: `random` — zufällige Verzögerung

Zufallswert aus `[randomFirst, randomLast]` Millisekunden pro Message.

```json
{
  "mode": "random",
  "randomFirst": 100,
  "randomLast": 5000,
  "randomUnits": "milliseconds"
}
```

Da die Verzögerungen unterschiedlich sind, kann **die Reihenfolge ändern** — das ist Absicht und Teil des Use-Case (Jitter).

## Override per Message

Konsistent zum Node-RED-Verhalten. Diese Felder, falls auf der eingehenden Message gesetzt, überschreiben die Konfiguration **nur für diese Message**:

| Feld | Wirkung |
|---|---|
| `msg.delay` | Verzögerung in Millisekunden für diese Message (überschreibt `timeout`/`random*`) |
| `msg.reset` | (truthy) — verwirft alle aktuell gepufferten/wartenden Messages, Message wird **nicht** weitergeleitet |
| `msg.flush` | (truthy) — gibt alle gepufferten Messages **sofort** aus (umgeht Timer/Rate), Message selbst wird nicht weitergeleitet |

Diese drei Felder werden vor der weiteren Verarbeitung von der Message **entfernt**, damit sie nicht in nachfolgende Nodes leaken (Konvention im Projekt, vgl. `_linkSource` im Link-Out).

## Konfiguration (vollständig)

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

Nicht jeder Mode benötigt jedes Feld — die Config-UI blendet die jeweils relevanten Felder ein. Backend-seitig werden aber immer alle Felder geparst und nur die zum aktiven `mode` passenden ausgewertet.

`*Units` ist eines von: `milliseconds`, `seconds`, `minutes`, `hours`, `day`. Backend rechnet beim `Init()` einmalig in `time.Duration` um.

## Verhalten im Detail

### Reihenfolge

- **delay** (fixer Timeout): FIFO bleibt erhalten. Einfache `time.AfterFunc` pro Message ist okay, da alle gleiche Wartezeit haben.
- **rate / queue**: FIFO strikt. Die Queue ist eine `chan *flow.Message` mit Größe `maxQueueLength`.
- **random**: kann Reihenfolge brechen. Dokumentieren.

### Stop()

Beim Stoppen des Flows (Redeploy, Shutdown) gilt: **pending Messages werden verworfen**, nicht durchgelassen. Begründung: Wer einen Flow stoppt, will einen sauberen Schnitt — sonst feuern nach `Stop()` noch Nachrichten in einen halb-abgebauten Flow. Der aktuelle Status der Node sollte das beim Stoppen melden.

### Status

| Zustand | Status |
|---|---|
| Idle | leer |
| Pending Messages > 0 | `blue ring` mit Anzahl, z.B. „queued: 23" |
| Drop in `behaviour: drop` | `yellow dot` „rate-limited" für ~1 s nach jedem Drop |
| Queue voll, ältester Eintrag verworfen | `yellow dot` „queue full: dropped oldest" |

## Umsetzung

### Backend (`internal/nodes/delay.go`)

Skelett der Datenstruktur:

```go
type DelayNode struct {
    config flow.NodeConfig
    send   flow.SendFunc
    status flow.StatusFunc

    mode           string         // "delay" | "rate" | "random"
    timeout        time.Duration  // for "delay"
    randomMin      time.Duration  // for "random"
    randomMax      time.Duration  // for "random"
    rateInterval   time.Duration  // for "rate" — ableitbar aus rate/rateUnits
    behaviour      string         // "queue" | "drop"
    maxQueueLength int

    queue chan *flow.Message    // for "rate"
    done  chan struct{}
    wg    sync.WaitGroup
    rng   *rand.Rand             // für random mode (mit Mutex falls geteilt)
}
```

Pro Mode eine eigene Verarbeitungslogik:

- **delay / random:** `HandleMessage` startet eine Goroutine pro Message (`time.AfterFunc` oder `time.NewTimer` + `select` auf `done`), die nach Ablauf `n.send(0, msg)` aufruft. Goroutines sind in Go billig — bei realistischen Lasten (≤ 10 k pending) völlig unproblematisch. **Erst optimieren, wenn wir tatsächlich >100 k pending sehen — dann ist ein Min-Heap (`container/heap`) mit einem Worker-Goroutine die richtige Antwort. Vorher YAGNI.**
- **rate:** `Start()` startet eine Worker-Goroutine, die im `time.Ticker(rateInterval)` aus `queue` liest und sendet. `HandleMessage` schreibt in `queue` (mit Drop- bzw. Drop-oldest-Strategie wenn voll).

### Override-Handling

In `HandleMessage` zuerst die drei Override-Felder lesen und vor dem Weiterverarbeiten von der Message entfernen:

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

(Die genauen Helper-Namen — `GetBool`, `GetNumber`, `Delete` — auf das vorhandene `flow.Message`-API mappen.)

### Frontend (`frontend/src/components/config/DelayConfig.vue`)

Die Config-UI wechselt das angezeigte Formular-Set basierend auf `mode`:

```
┌──────────────────────────────────────────────┐
│ Action                                       │
│ ( ) Delay each message                       │
│ ( ) Rate Limit messages                      │
│ ( ) Random delay                             │
│                                              │
│ ── (mode = delay) ────────────────────       │
│ For [   500   ] [milliseconds ▾]             │
│                                              │
│ ── (mode = rate) ─────────────────────       │
│ Rate     [   1   ]  msg / [second ▾]         │
│ Queue    ( ) Drop intermediate               │
│          (•) Queue intermediate              │
│ Max queue length [ 1000 ]                    │
│                                              │
│ ── (mode = random) ───────────────────       │
│ Between [  0  ] and [ 5000 ] [ms ▾]          │
└──────────────────────────────────────────────┘
```

Validierung im Frontend: positive Zahlen, `randomLast >= randomFirst`. Bei Verstoß: `FormInput` mit Fehler-Border und Submit-Block (Pattern wie in den anderen Config-Komponenten).

### Help-Doc

Eintrag `delay` in `frontend/src/components/help/docs.ts` mit Overview, Properties-Liste, drei Beispielen (einer pro Mode), und einem expliziten Tipp zu `msg.delay` / `msg.reset` / `msg.flush`.

### Registrierung

`internal/server/server.go` in `registerNodes()`:

```go
registry.Register("delay", nodes.NewDelayNode, nodes.DelayTypeInfo())
```

`frontend/src/components/config/configEditors.ts`: Dispatch für `delay` auf `DelayConfig.vue`.

## Betroffene Dateien

| Datei | Änderung |
|---|---|
| `internal/nodes/delay.go` | **Neu** — Node-Implementierung, alle drei Modi |
| `internal/nodes/delay_test.go` | **Neu** — Tests siehe unten |
| `internal/server/server.go` | Registrierung in `registerNodes()` |
| `frontend/src/components/config/DelayConfig.vue` | **Neu** — Config-UI mit Mode-Switch |
| `frontend/src/components/config/configEditors.ts` | Dispatch für `delay` |
| `frontend/src/components/help/docs.ts` | Help-Eintrag `delay` |
| `docs/MISSING_FUNCTIONALITY.md` / `PLANNING.md` | Delay als erledigt markieren |

## Tests

| Test | Prüft |
|---|---|
| `TestDelayMode_FixedTimeout` | Timeout = 100 ms → Message kommt ≥ 100 ms später, Reihenfolge bleibt |
| `TestDelayMode_MsgDelayOverride` | `msg.delay = 50` überschreibt Config-Timeout |
| `TestDelayMode_RemovesOverrideField` | `msg.delay` ist auf der ausgehenden Message **nicht mehr vorhanden** |
| `TestRandomMode_WithinBounds` | 100 Samples liegen in `[min, max]` |
| `TestRateMode_QueueRespectsRate` | 10 schnelle Messages bei Rate=1/100ms → Ausgabe-Abstände ≥ 100ms |
| `TestRateMode_DropBehaviour` | `behaviour=drop`: Burst von 10 → nur 1 durchgelassen, 9 gedropped |
| `TestRateMode_QueueOverflow` | Queue voll → ältester Eintrag verworfen, Status-Update |
| `TestMsgFlush` | Queue mit 5 Items + `msg.flush=true` → alle 5 sofort ausgegeben |
| `TestMsgReset` | Queue mit 5 Items + `msg.reset=true` → alle 5 verworfen, keine Ausgabe |
| `TestStopDiscardsPending` | 100 Messages in delay → `Stop()` → keine Sends mehr nach Stop |
| `TestUnitsConversion` | `timeoutUnits: "seconds", timeout: 2` → 2 s Verzögerung |

## Bewusst nicht im Scope

- **Per-Topic Rate-Limit** (Node-RED hat das via Checkbox). Selten genug genutzt, um es als separates Feature später nachzuziehen, falls jemand danach fragt.
- **Persistente Queue über Restarts hinweg.** Würde NATS-Stream-Backing erfordern, ist Overkill für die Mehrheit der Use-Cases. Aktuell: Stop = Verwerfen.
- **Backpressure auf Producer-Seite.** Nodes haben keine Backpressure-API; mit `behaviour: drop` und `maxQueueLength` ist das Verhalten definiert.

## Abhängigkeiten

Keine. Alle benötigten Bausteine (`flow.SendFunc`, `flow.StatusFunc`, `flow.Message`, Config-Editor-Dispatch, Help-Doc-System) sind vorhanden.
