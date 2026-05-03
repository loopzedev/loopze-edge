# Issue: Node-Status nach Seitenaufruf wiederherstellen

## Status: Open

## Problembeschreibung

Nach dem Neuladen der Seite (F5 / Browser-Refresh) sind alle Node-Status-Anzeigen leer, obwohl die Nodes im Backend weiterhin laufen und aktiv Status melden. Der Status erscheint erst wieder, wenn ein Node erneut `n.status(fill, text)` aufruft — z.B. beim nächsten eingehenden Message.

**Ursache**: Der Node-Status wird aktuell nur als **Live-Stream** über WebSocket übermittelt. Es gibt keine Persistenz und keinen Mechanismus, den letzten bekannten Status beim Verbindungsaufbau abzurufen.

## Aktuelle Pipeline

```
Node ruft n.status("green", "42 msgs")
    ↓
Engine.makeStatusFunc → NATS Publish "status.<flowID>.<nodeID>"
    ↓
Server: conn.Subscribe("status.>") → hub.Broadcast(EventStatus, msg)
    ↓
WebSocket → Frontend: flowStore.updateNodeStatus(nodeId, status)
    ↓
BaseNode rendert Farbpunkt + Text
```

**Problem**: Beim Seitenaufruf ist der WebSocket noch nicht verbunden, wenn die Flows geladen werden. Status-Nachrichten die vor dem Verbindungsaufbau gesendet wurden, gehen verloren.

## Lösung: In-Memory Map in der Engine

Eine einfache Go-Map in der Engine speichert den **jeweils letzten Status** pro Node. Beim Seitenaufruf fragt das Frontend den aktuellen Status über einen neuen API-Endpoint ab.

### Warum Go-Map statt NATS KV?

- LOOPZE läuft als **einzelner Prozess** — kein verteiltes System das KV bräuchte
- Der Status ist **flüchtig** — geht bei Server-Neustart sowieso verloren (Engine startet neu, Nodes haben keinen Status)
- Die Engine hat bereits die `nodes`-Map — der Status-Cache lebt im selben Scope
- **Zero Overhead**: Kein Netzwerk-Roundtrip, kein Serialisieren, direkter Map-Zugriff

### Race Conditions

Die Status-Map wird von **mehreren Goroutinen** gleichzeitig beschrieben (jeder Node läuft in seiner eigenen Goroutine via `nodeLoop`). Gleichzeitig liest der API-Handler die Map bei HTTP-Requests.

**Lösung**: `sync.RWMutex` schützt die Map.

- **Schreibzugriff** (`Lock`): `makeStatusFunc` — wird aus Node-Goroutinen aufgerufen
- **Lesezugriff** (`RLock`): API-Handler — parallele Reads sind erlaubt

```go
type statusCache struct {
    mu      sync.RWMutex
    entries map[string]StatusMessage // nodeID → letzter Status
}

func (c *statusCache) Set(nodeID string, msg StatusMessage) {
    c.mu.Lock()
    c.entries[nodeID] = msg
    c.mu.Unlock()
}

func (c *statusCache) GetAll() map[string]StatusMessage {
    c.mu.RLock()
    defer c.mu.RUnlock()
    result := make(map[string]StatusMessage, len(c.entries))
    for k, v := range c.entries {
        result[k] = v
    }
    return result
}

func (c *statusCache) Clear() {
    c.mu.Lock()
    c.entries = make(map[string]StatusMessage)
    c.mu.Unlock()
}
```

## Betroffene Dateien

### Backend — Anpassungen

#### `internal/flow/engine.go`

Neues Feld in der Engine-Struct:

```go
type Engine struct {
    // ... bestehende Felder ...
    statusCache statusCache
}
```

Initialisierung in `NewEngine`:

```go
statusCache: statusCache{entries: make(map[string]StatusMessage)},
```

In `makeStatusFunc` den Status zusätzlich cachen:

```go
func (e *Engine) makeStatusFunc(nodeID string, rn *runningNode) StatusFunc {
    return func(fill string, text string) {
        msg := StatusMessage{
            NodeID: nodeID,
            FlowID: rn.flowID,
            Status: NodeStatusPayload{Fill: fill, Text: text},
        }
        e.statusCache.Set(nodeID, msg)
        if e.publishStatus != nil {
            subject := fmt.Sprintf("status.%s.%s", rn.flowID, nodeID)
            e.publishStatus(subject, msg)
        }
    }
}
```

In `stopNodes` den Cache leeren:

```go
e.statusCache.Clear()
```

Neue öffentliche Methode für den API-Handler:

```go
func (e *Engine) NodeStatuses() map[string]StatusMessage {
    return e.statusCache.GetAll()
}
```

#### `internal/api/handlers.go`

Neuer Endpoint:

```
GET /api/v1/status/nodes → { "statuses": { "<nodeId>": { "nodeId": "...", "flowId": "...", "status": { "fill": "green", "text": "42" } } } }
```

Der Handler ruft `engine.NodeStatuses()` auf und serialisiert das Ergebnis.

#### `internal/api/routes.go`

Neue Route registrieren:

```go
r.Get("/api/v1/status/nodes", handler.GetNodeStatuses)
```

### Frontend — Anpassungen

#### `frontend/src/composables/useApi.ts`

Neue Methode:

```typescript
async function getNodeStatuses(): Promise<Record<string, { fill: string; text: string }>> {
    const res = await fetch('/api/v1/status/nodes')
    const data = await res.json()
    return data.statuses ?? {}
}
```

#### `frontend/src/views/FlowEditor.vue`

Im `onMounted` nach `loadFlows` den Status abrufen und auf die Nodes anwenden:

```typescript
onMounted(async () => {
    const response = await api.getFlows()
    flowStore.loadFlows(response.flows, response.rev)

    // NEU: Letzten Node-Status wiederherstellen
    const statuses = await api.getNodeStatuses()
    for (const [nodeId, status] of Object.entries(statuses)) {
        flowStore.updateNodeStatus(nodeId, status)
    }
})
```

## Ablauf nach der Implementierung

```
Seite wird geladen
    ↓
GET /api/v1/flows → Flows + Nodes laden
    ↓
GET /api/v1/status/nodes → Letzten Status aller Nodes aus Engine-Cache abrufen
    ↓
flowStore.updateNodeStatus() für jeden Node mit Status
    ↓
BaseNode zeigt sofort den letzten bekannten Status an
    ↓
WebSocket verbindet → Live-Updates übernehmen ab jetzt
```

## Lifecycle-Hinweise

- **Deploy**: `stopNodes()` ruft `statusCache.Clear()` auf — alle Nodes starten neu und haben zunächst keinen Status
- **Server-Neustart**: Cache ist weg (in-memory) — gewollt, da die Engine auch keine laufenden Nodes mehr hat
- **Hochfrequente Updates**: `statusCache.Set()` überschreibt immer den letzten Wert — kein Speicherwachstum, egal wie oft ein Node seinen Status aktualisiert

## Abhängigkeiten

- Die bestehende Status-Pipeline (NATS Publish → WebSocket → Frontend) muss funktionieren — ist seit `NODE_STATUS_PIPELINE.md` implementiert
- Keine Frontend-Komponentenänderungen nötig — `updateNodeStatus()` und das BaseNode-Rendering existieren bereits
