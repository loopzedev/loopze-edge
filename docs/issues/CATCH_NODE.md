# Catch Node

## Status: Open

## Beschreibung

Der Catch Node fängt **Laufzeit-Fehler aus anderen Nodes** ab und macht sie als reguläre Message weiterverarbeitbar. Heute werden Fehler in `internal/flow/engine.go` (im `nodeLoop`, beim Aufruf von `instance.HandleMessage`) lediglich geloggt und über `publishNodeError` als `DebugMessage` ans Frontend geschickt — danach ist die Message weg, der Flow hat keinen Hebel, etwas zu tun. Genau diese Lücke schließt der Catch Node.

`catch` ist im Frontend bereits als `NodeType` in `frontend/src/types/flow.ts` deklariert, hat aber weder Backend, Config-UI, Help-Doc noch Engine-Integration.

## Designvorlage: Status Node

Der Catch Node ist **strukturell identisch** zum bereits implementierten Status Node — er beobachtet Errors statt Status-Updates. Wo immer möglich wird das Status-Node-Pattern wiederverwendet:

- 0 Inputs, 1 Output, event-getrieben
- Listener-Pattern (`ErrorListenerProvider` analog zu `StatusListenerProvider`)
- Identisches Scope-Modell und Property-Schema
- Identische UI-Bausteine (Scope-Select, Multi-Select-Liste mit Highlight)
- Engine-seitiger Loop-Schutz (Catch-eigene Errors werden gefiltert, wie Status-Events von Status-Nodes)

## Motivation

Heute ist jeder Fehler ein stiller Tod der Message:

- Function-Node throwt → Message verschwindet, kein Logging im Flow möglich
- MQTT-Publish schlägt fehl → keine Notification möglich
- HTTP-Request timed out → keine Fallback-Route

Mit Catch lassen sich Standard-Patterns wie Dead-Letter-Queue, Notification oder Logging in einen Debug-Node trivial bauen.

## Verhalten

- **0 Inputs**, **1 Output** — Catch ist event-getrieben (analog Status Node, der ja auch keinen Input hat)
- `HandleMessage` ist no-op (return nil) — das Senden passiert im Listener-Callback
- Die ausgehende Message enthält die **Original-Message** plus `msg._error` (siehe unten)

### Scope (drei Optionen, analog Status Node)

| Scope | Beschreibung |
|---|---|
| **flow** (Default) | Fängt Fehler aller Nodes im selben Flow |
| **selected** | Fängt Fehler nur von explizit ausgewählten Nodes (immer im selben Flow) |
| **all** | Fängt Fehler aus allen Flows (Power-User) |

Mehrere Catch-Nodes sind erlaubt — jeder fängt unabhängig. Wenn ein Fehler zu mehreren Catch-Nodes passt, werden alle getriggert (jeder bekommt seinen eigenen COW-Clone der Message).

### Loop-Schutz

Engine-seitig gefiltert: Errors aus Catch-Nodes selbst werden im Listener-Fan-out **nicht** weitergegeben — exakt analog zum Status Node, wo Status-Events von Status-Nodes ausgeklammert werden, damit es keine Selbst-Trigger-Loops gibt.

Konsequenz: **Retry-Patterns mit Catch sind im MVP bewusst nicht möglich.** Wer Retry braucht, macht das in der Quell-Node selbst (eigenes Feature pro Node-Typ). Catch ist ein Logging-/Notification-Werkzeug.

### `msg._error` Format

Flach und minimal — Aufbau bewusst parallel zum `msg.status` des Status Nodes:

```json
{
  "_error": {
    "message": "TypeError: Cannot read property 'foo' of undefined",
    "source": {
      "id": "n_abc123",
      "type": "function",
      "name": "Parse Payload",
      "flowId": "f_xyz"
    }
  }
}
```

Außerdem bleibt `_id` (die Original-Message-ID) erhalten — `flow.Message.COWClone()` macht das bereits korrekt.

### Async-Errors

Viele Nodes (MQTT-Publish, HTTP-Request) senden asynchron im Hintergrund. Wenn `HandleMessage` schon zurückgekehrt ist und der Fehler erst danach auftritt, sieht der `nodeLoop`-Pfad ihn nicht. Damit Catch trotzdem greift, bekommen Nodes eine `flow.ErrorFunc` injiziert (genauso wie sie heute `flow.StatusFunc` über `SetStatus` bekommen):

```go
func (n *MyNode) SetError(fn flow.ErrorFunc) { n.errorFn = fn }
// ...
n.errorFn(err, msg)  // löst dieselbe Catch-Pipeline aus wie ein synchroner Return-Error
```

Engine fan-outet auf dieselben `errorListeners` wie bei synchronen Errors. Die konkrete Migration einzelner Nodes (HTTP, MQTT) erfolgt **in eigenen Issues** — der Catch-Node selbst bleibt davon unberührt.

## Konfiguration (Backend)

```json
{
  "scope": "flow",
  "targetNodes": []
}
```

| Feld | Beschreibung |
|---|---|
| `scope` | `flow` (Default), `selected` oder `all` |
| `targetNodes` | Bei `scope=selected`: Liste von Node-IDs |

## UI

Klon von `StatusConfig.vue`, nur Labels und Help-Text angepasst. Das `useNodeProperty`-Pattern, der Scope-Dropdown, die Checkbox-Liste mit Hover-Highlight (`setHoveredHighlightNodeId`) und der Inline-Hilfetext werden 1:1 übernommen.

Node-Darstellung:
- Category: **`common`** (wie Status — keine eigene Akzentfarbe nötig)
- Body: `scope=flow` → "this flow", `scope=selected` → "N nodes", `scope=all` → "all flows"
- Linker Anker (Input) wird nicht gerendert — `BaseNode` mit `:inputs="0"`

## Implementierung

### Engine-Erweiterung (`internal/flow/engine.go`)

Vollständige Spiegelung des Status-Listener-Mechanismus (`engine.go:102-104, 567-575, 914-939`):

- Neues Feld `errorListeners map[uint64]ErrorListenerFunc` + `errorListenersMu sync.RWMutex` + Sequence-Counter
- `registerErrorListener(fn) (unregister func())` analog zu `registerStatusListener`
- `fanoutError(msg ErrorMessage)` analog zu `fanoutStatus` — mit Filter: wenn die Quell-Node ein Catch ist, früh zurückkehren (Loop-Schutz)
- In `wireAllNodes` die ErrorListener-Provider verdrahten (zweite Schleife direkt nach der StatusListener-Schleife)
- `makeErrorFunc(nodeID, rn)` baut die per-Node-`ErrorFunc`-Closure (analog `makeStatusFunc`), die intern `publishNodeError` UND `fanoutError` aufruft
- Im `nodeLoop`-Error-Pfad: statt direkt `publishNodeError` zu rufen, ruft die Engine `errorFunc(err, msg)` auf — gleiche Pipeline für sync und async

Neue Typen in `internal/flow/registry.go` (analog Status):

```go
type ErrorMessage struct {
    NodeID     string
    FlowID     string
    SourceType string
    SourceName string
    Error      string
    Msg        *Message
}

type ErrorFunc func(err error, msg *Message)
type ErrorListenerFunc func(msg ErrorMessage)
type ErrorListenerProvider interface {
    SetErrorListener(register func(ErrorListenerFunc) (unregister func()))
}
```

### Catch-Node (`internal/nodes/catch.go`)

Klon von `internal/nodes/status.go` mit minimalen Änderungen:
- `handleStatus(sm StatusMessage)` → `handleError(em ErrorMessage)`
- Baut `_error`-Map statt `status`-Map auf der Output-Message
- `SourceType`-Filter: Status-Node filtert eigene Status-Events; Catch-Node muss das nicht selbst tun, weil die Engine das im `fanoutError` schon macht
- Idle-Status: `"listening for errors"` statt `"listening"`

### Registrierung

```go
registry.Register("catch", nodes.NewCatchNode, nodes.CatchTypeInfo())
```

### Frontend

- **`frontend/src/components/config/CatchConfig.vue`** — Klon von `StatusConfig.vue`, Filter `n.type !== 'catch'` statt `'status'`, Help-Text angepasst
- **`frontend/src/components/nodes/CatchNode.vue`** — Klon von `StatusNode.vue`, `node-type="catch"`
- **`frontend/src/components/help/docs.ts`** — Help-Eintrag `catch` mit Hinweis "kein Retry-Werkzeug"
- Vue-Flow-Node-Type-Map: `catch` → `CatchNode.vue` (an der Stelle, an der auch `status` registriert ist)
- Config-Editor-Dispatcher: `catch` → `CatchConfig.vue`

## Tests

Klone der Status-Node-Tests in `internal/nodes/catch_test.go`, plus Engine-Integrationstests in `internal/flow/engine_catch_test.go`:

| Test | Prüft |
|---|---|
| `TestCatchScopeFlow_TriggeredOnError` | Function-Node throwt → Catch (`scope=flow`) emittiert `_error.source.id == function_node_id` |
| `TestCatchScopeFlow_IgnoresOtherFlows` | Error in Flow A triggert keinen Catch in Flow B |
| `TestCatchScopeSelected_OnlyMatching` | `targetNodes=[A]` ignoriert Errors von Node B |
| `TestCatchScopeSelected_MultipleNodes` | `targetNodes=[A,B]` fängt von beiden |
| `TestCatchScopeAll_AcrossFlows` | `scope=all` fängt Errors aus allen Flows |
| `TestMultipleCatch_BothTriggered` | Zwei passende Catch-Nodes → beide triggern (separate COWClones) |
| `TestLoopGuard_CatchErrorsFiltered` | Fehler im Catch-Output-Branch triggert keinen Catch (Engine-Filter) |
| `TestErrorPayload_PreservesOriginalMsg` | `payload` und `_id` identisch zur Original-Message |
| `TestErrorPayload_ContainsErrorFields` | `_error.message`, `_error.source.{id,type,name,flowId}` korrekt |
| `TestSelectedNodes_UnknownIDIgnored` | Unbekannte ID in `targetNodes` → kein Crash |
| `TestStopReleasesListener` | Nach `Stop()` keine Catch-Triggers mehr |
| `TestAsyncError_TriggersCatch` | Stub-Node mit `SetError` ruft `errorFn(err, msg)` aus Goroutine → Catch wird getriggert |

## Bewusst nicht im Scope

- **Retry-Pattern über Catch** — Loop-Schutz verhindert das absichtlich
- **Filtern nach Error-Type/-Pattern** — der nachgeschaltete Switch-Node erledigt das
- **Migration aller bestehenden Nodes auf `SetError`** — die API wird bereitgestellt, konkrete Nutzung pro Node-Typ in eigenen Issues (HTTP zuerst, dann MQTT-Out)

## Abhängigkeiten

- `flow.Message.COWClone()` — vorhanden
- `StatusListenerProvider`-Pattern als Vorlage — vorhanden, wird gespiegelt
- `flow.NewMessage()` + `Set/SetPayload` — vorhanden
- `BaseNode` mit `:inputs="0"` — vorhanden (Status, Inject)
- `useNodeProperty`, `setHoveredHighlightNodeId` — vorhanden
