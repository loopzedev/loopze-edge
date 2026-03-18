# Issue: Node-Status End-to-End Pipeline

## Status: Open

## Problembeschreibung

Die Infrastruktur für Node-Status ist in Frontend und Backend bereits angelegt, aber die **End-to-End-Kette ist unterbrochen**. Nodes rufen `n.status(fill, text)` auf, der Status kommt aber nie im Browser an.

Die bestehende Debug-Pipeline zeigt das korrekte Muster:

```
Node → n.debug() → Engine.PublishDebugFunc → NATS "debug.>" → Server Subscriber → hub.Broadcast() → WebSocket → Browser
```

Für Status fehlt dieses Muster komplett — `makeStatusFunc` loggt nur via `slog.Debug`.

## Bestandsaufnahme

### Was bereits funktioniert

| Komponente | Status | Details |
|---|---|---|
| `StatusFunc` Definition | OK | `internal/flow/registry.go` — `func(fill string, text string)` |
| `SetStatus()` Lifecycle | OK | Engine ruft `SetStatus(makeStatusFunc(nodeID))` nach `Init()` auf |
| Nodes rufen `n.status()` auf | OK | Debug Node (grey, statusText), Function Node (`node.status(fill, text)` in JS) |
| WebSocket Hub Broadcast | OK | `hub.Broadcast(eventType, data)` in `internal/ws/hub.go` |
| `StatusEvent` TypeScript-Typen | OK | `frontend/src/types/events.ts` — `StatusEvent`, `NodeStatus` |
| WebSocket `onStatus` Dispatcher | OK | `frontend/src/composables/useWebSocket.ts` — dispatcht Status-Events korrekt |
| BaseNode Status-Rendering | OK | `frontend/src/components/nodes/BaseNode.vue` — Farbpunkt + Text |
| Status-Farben | OK | `frontend/src/components/nodes/tokens.ts` — red, green, yellow, blue, grey |

### Referenz: Debug-Pipeline (funktioniert)

```
Engine.makeDebugFunc(nodeID)                    → PublishDebugFunc(subject, msg)
    ↓
server.go: engine.SetPublishDebug(func(...) {
    conn.Publish("debug.<flowID>.<nodeID>", data)   → NATS
})
    ↓
server.go: conn.Subscribe("debug.>", func(m) {
    hub.Broadcast(ws.EventDebug, dbg)               → WebSocket Hub
})
    ↓
Browser: ws.onDebug → debugStore.addMessage()
```

### Die 4 fehlenden Verbindungen

#### Lücke 1: Engine — `PublishStatusFunc` Callback fehlt

**Analog zu:** `SetPublishDebug` / `PublishDebugFunc` in `internal/flow/engine.go`

Die Engine braucht einen `PublishStatusFunc` Callback (analog zu `PublishDebugFunc`), den der Server beim Start setzt.

```go
type PublishStatusFunc func(subject string, msg StatusMessage)
```

Neues Struct `StatusMessage` in `internal/flow/registry.go`:

```go
type StatusMessage struct {
    NodeID string `json:"nodeId"`
    FlowID string `json:"flowId"`
    Fill   string `json:"fill"`
    Text   string `json:"text"`
}
```

#### Lücke 2: Engine — `makeStatusFunc` muss über NATS publishen

**Datei:** `internal/flow/engine.go:344-350`

```go
// AKTUELL:
func (e *Engine) makeStatusFunc(nodeID string) StatusFunc {
    return func(fill string, text string) {
        slog.Debug("node status",
            "node_id", nodeID, "fill", fill, "text", text)
        // TODO: broadcast status via WebSocket to frontend
    }
}
```

**Fix:** Analog zu `makeDebugFunc` den `PublishStatusFunc` Callback aufrufen:

```go
func (e *Engine) makeStatusFunc(nodeID string) StatusFunc {
    return func(fill string, text string) {
        if e.publishStatus != nil {
            e.publishStatus("status."+e.flowID+"."+nodeID, StatusMessage{
                NodeID: nodeID,
                FlowID: e.flowID,
                Fill:   fill,
                Text:   text,
            })
        }
    }
}
```

#### Lücke 3: Server — NATS Publish + Subscribe für Status

**Datei:** `internal/server/server.go`

Analog zum Debug-Pattern zwei Ergänzungen:

1. **Publish-Callback setzen** (analog zu `SetPublishDebug`):
```go
s.engine.SetPublishStatus(func(subject string, msg flow.StatusMessage) {
    data, _ := json.Marshal(msg)
    conn.Publish(subject, data)
})
```

2. **NATS Subscriber** (analog zu `debug.>`):
```go
conn.Subscribe("status.>", func(m *nats.Msg) {
    var status flow.StatusMessage
    json.Unmarshal(m.Data, &status)
    s.hub.Broadcast(ws.EventStatus, status)
})
```

#### Lücke 4: Frontend — Listener + FlowStore Action

**Datei:** `frontend/src/App.vue` — `ws.onStatus()` Listener fehlt:

```typescript
ws.onStatus((event) => {
    flowStore.updateNodeStatus(event.nodeId, event.status)
})
```

**Datei:** `frontend/src/stores/flowStore.ts` — Action zum Aktualisieren:

```typescript
function updateNodeStatus(nodeId: string, status: NodeStatus) {
    const node = nodes.value.find(n => n.id === nodeId)
    if (node) {
        node.data = { ...node.data, status }
    }
}
```

## Betroffene Dateien

### Backend
- `internal/flow/registry.go` — `StatusMessage` Struct + `PublishStatusFunc` Type
- `internal/flow/engine.go` — `SetPublishStatus`, `makeStatusFunc` über NATS publishen
- `internal/server/server.go` — NATS Publish-Callback + `status.>` Subscriber
- `internal/ws/hub.go` — `EventStatus` Konstante existiert bereits

### Frontend
- `frontend/src/App.vue` — `ws.onStatus()` Listener hinzufügen
- `frontend/src/stores/flowStore.ts` — `updateNodeStatus()` Action hinzufügen

## Abhängigkeiten

- Der Debug Node (`internal/nodes/debug.go`) nutzt bereits `n.status("grey", statusText)` für die konfigurierbare Status-Ausgabe — wird erst sichtbar wenn diese Pipeline steht
- Der Function Node (`internal/nodes/function.go`) bietet `node.status(fill, text)` in der JS-Runtime an — ebenfalls blockiert

## Hinweise

- `NodeStatus` im Frontend erwartet ein `shape`-Feld (`ring` | `dot`), das Backend liefert aktuell nur `fill` + `text`. Default `dot` verwenden wenn nicht angegeben.
- Status-Updates sind hochfrequent möglich (z.B. bei jedem Message im Debug Node mit `count`-Modus) — ggf. Throttling/Debouncing auf Backend- oder Frontend-Seite erwägen.
