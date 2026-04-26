# Issue: Status Node — Status-Events anderer Nodes als Message ausgeben

## Status: Implemented

## Problembeschreibung

Aktuell ist der Node-Status (`n.status(fill, text)`) ausschließlich **visuelles Feedback** am Node selbst — ein farbiger Punkt mit Text. Status-Änderungen lassen sich nicht programmatisch im Flow weiterverarbeiten.

Damit fehlt eine ganze Klasse von Use Cases:

- **Watchdog**: Geht ein MQTT-In Node auf `red`, soll automatisch ein Alert über einen MQTT-Out Node oder eine Funktion versendet werden
- **Status-Aggregation**: Status mehrerer kritischer Nodes in einem zentralen Topic / Dashboard zusammenführen
- **Brücke nach außen**: Node-Status als MQTT-Nachricht oder Webhook in ein externes Monitoring-System schieben
- **Reaktive Flows**: Andere Nodes (z.B. State Machine, Function) sollen auf Status-Wechsel reagieren, nicht nur auf Payload

In Node-RED löst das der **Status Node**: Er steht im Flow, abonniert die Status-Events anderer Nodes und gibt jeden Status als Message am eigenen Output aus.

## Sichtweise / Begründung

Die Vorarbeit existiert bereits:

- **Status-Pipeline** (`NODE_STATUS_PIPELINE.md`): Engine → NATS `status.<flowID>.<nodeID>` → WebSocket → Frontend
- **Status-Cache** (`NODE_STATUS.md`): `engine.NodeStatuses()` hält den letzten Status pro Node
- **Standard-Node-Pattern**: `internal/nodes/*.go` zeigt klar, wie ein Node `Init` / `Start` / `HandleMessage` / `Stop` umsetzt

Ein Status Node ist im Wesentlichen ein **Status-Subscriber-Node**. Architektur-Entscheidung: Er hängt sich nicht an NATS, sondern an einen neuen **Engine-internen Fan-out** (siehe Technische Skizze) — passt zu den bestehenden Provider-Interfaces (`LinkProvider`, `ContextProvider`, `ConfigProvider`) und vermeidet einen NATS-Roundtrip pro Status-Event.

## Anforderungen

### 1. Scope-Konfiguration

Der Bediener wählt im Properties-Panel, **welche Status-Events** der Node aufgreift:

| Wert | Label | Verhalten |
|---|---|---|
| `flow` | Aktueller Flow | Status aller Nodes im selben Flow wie der Status Node (Default) |
| `selected` | Selected nodes | Status nur einer expliziten Auswahl von Nodes — strikt auf den eigenen Flow beschränkt |
| `all` | Alle Flows | Status sämtlicher Nodes systemweit |

- Default: `flow` — die häufigste Erwartung und entspricht Node-RED.
- `selected`: Multi-Select mit Checkbox-Liste. Auswahl ist als Set von Node-IDs in `config.targetNodes` (`string[]`) gespeichert. Die Liste enthält **nur Nodes des aktuellen Flows**, ohne den Status Node selbst und ohne andere Status Nodes (die kann er ohnehin nicht beobachten).

**Status Nodes empfangen niemals Status von anderen Status Nodes** — unabhängig vom Scope. Damit sind Endlosschleifen architektonisch ausgeschlossen, und ein Status Node ist für andere Status Nodes "unsichtbar". Die Filterung erfolgt zentral im Engine-Fan-out, nicht im einzelnen Node — siehe Technische Skizze.

**Doppelter Schutz für Selected-Mode**: Der Backend-Filter prüft zusätzlich `FlowID == config.FlowID`, selbst wenn das Frontend versehentlich eine fremde Node-ID lieferte oder der Node später in einen anderen Flow wandert. Selection bleibt strikt flow-lokal.

### 2. Output-Message

Pro empfangenem Status-Event wird **eine Message** ausgegeben:

```json
{
  "status": {
    "fill": "green",
    "text": "42 msgs",
    "source": {
      "id": "<nodeID>",
      "type": "mqtt-in",
      "name": "Sensor Living Room",
      "flowId": "<flowID>"
    }
  },
  "payload": "42 msgs"
}
```

- `msg.status` enthält das vollständige Status-Objekt inkl. Quelle (analog Node-RED `msg.status`)
- `msg.payload` enthält den `text` — bequem für direkte MQTT-Out / Debug-Verarbeitung ohne weiteren Change Node
- `source.name` wird aus dem Node-Namen (`config.name`) aufgelöst — Fallback: leerer String wenn unbekannt
- `source.type` ermöglicht Filterung im Flow (z.B. nur `mqtt-in`-Status auswerten)

### 3. Filter (optional, Phase 2)

- **Nur bei Status-Wechsel**: Checkbox "Nur bei Änderung emittieren" — der Node merkt sich den letzten Status pro Quell-Node und gibt eine Message nur aus, wenn `fill` oder `text` sich geändert hat. Verhindert Floods bei `count`-Status mit jeder Message.
- **Nur bei `fill`-Wert**: Multi-Select über die fünf Status-Farben (`red`, `green`, `yellow`, `blue`, `grey`). Default: alle.

Phase 1 implementiert beides **nicht** — erst beobachten, ob Bedarf entsteht.

### 4. Kein Input

Der Status Node hat **keinen Input-Handle**, nur einen Output. Anders als z.B. Inject ist er nicht durch eingehende Messages getriggert, sondern reagiert ausschließlich auf Status-Events.

### 5. Eigener Status

Der Status Node selbst zeigt am Frontend einen kurzen Status:

- Beim Start: `green` / `"listening"`
- Bei jedem Output: `grey` / `"<source.name>: <fill>"` für ~2 Sekunden, dann zurück auf `green`/`"listening"` (Truncate auf 32 Zeichen)
- Blink-Sequenz wird per `atomic.Uint64` gegen Race Conditions geschützt — bei schnellen Updates überschreibt nicht ein älterer Reset einen neueren Blink

Da Status Nodes im Engine-Fan-out ausgefiltert sind, lösen diese eigenen Status-Updates keine Re-Trigger anderer Status Nodes aus.

### 6. Hover-Highlight im Multi-Select

Beim Hover über ein Item in der Selected-Nodes-Checkbox-Liste wird der entsprechende Node im Flow-Editor mit einer **gestrichelten Outline** markiert (1px dashed in Accent-Farbe). Damit lässt sich beim Konfigurieren auf einen Blick zuordnen, welche Node welchem Listeneintrag entspricht — kein "Welcher mqtt-in war jetzt 'Sensor Living Room' und welcher 'Sensor Bedroom'?".

Der Mechanismus existierte bereits im DebugPanel (`hoveredDebugNodeId` mit `outline: dashed` an `BaseNode`). Für die zweite Verwendung wurde er auf einen generischen Namen umbenannt (`hoveredHighlightNodeId` / `setHoveredHighlightNodeId` / `isHighlighted`), damit er semantisch nicht mehr nur "Debug-Hover" suggeriert. DebugPanel und StatusConfig konsumieren denselben State.

## Technische Skizze

### Architektur-Entscheidung: Engine-internes Fan-out

Die Engine ist explizit so designt, dass Nodes **nichts** über NATS wissen — externe Kommunikation läuft ausschließlich über Engine-Callbacks (`SetSend`, `SetStatus`, `SetDebug`) und Provider-Interfaces (`ContextProvider`, `LinkProvider`, `ConfigProvider`). Der Status Node folgt diesem Pattern: Er bekommt Status-Events über einen neuen `StatusListener`-Mechanismus von der Engine geliefert, statt selbst auf NATS zu hören.

| | Engine-Fan-out (gewählt) | Node abonniert NATS direkt |
|---|---|---|
| Architektur-Konsistenz | passt zu LinkProvider/ContextProvider | bricht "Nodes kennen kein NATS" |
| Latenz | Go-Funktionsaufruf (ns) | NATS-Roundtrip (μs), JSON-Roundtrip |
| Daten-Doppel | nein — selbe `StatusMessage`-Struct | ja — Marshal/Unmarshal nochmal |
| Lifecycle | Engine räumt Listener beim Stop ab | Node muss Unsubscribe selbst koordinieren |

### Backend — `internal/flow/registry.go`

`StatusMessage` um `SourceType` und `SourceName` erweitert (rückwärtskompatibel via `omitempty`):

```go
type StatusMessage struct {
    NodeID     string            `json:"nodeId"`
    FlowID     string            `json:"flowId"`
    Status     NodeStatusPayload `json:"status"`
    SourceType string            `json:"sourceType,omitempty"`
    SourceName string            `json:"sourceName,omitempty"`
}
```

`NodeConfig` um `FlowID` erweitert, damit Nodes ihren eigenen Flow kennen (für den Scope-Filter):

```go
type NodeConfig struct {
    ID         string         `json:"id"`
    Type       string         `json:"type"`
    Name       string         `json:"name"`
    FlowID     string         `json:"flowId"`
    Properties map[string]any `json:"properties"`
}
```

Neues Provider-Interface analog zu `LinkProvider`:

```go
type StatusListenerFunc func(msg StatusMessage)

type StatusListenerProvider interface {
    SetStatusListener(register func(StatusListenerFunc) (unregister func()))
}
```

### Backend — `internal/flow/engine.go`

Engine hält eine Map registrierter Listener unter `sync.RWMutex`:

```go
type Engine struct {
    // ... bestehend ...
    statusListenersMu sync.RWMutex
    statusListenerSeq uint64
    statusListeners   map[uint64]StatusListenerFunc
}

func (e *Engine) registerStatusListener(fn StatusListenerFunc) func() {
    e.statusListenersMu.Lock()
    e.statusListenerSeq++
    id := e.statusListenerSeq
    e.statusListeners[id] = fn
    e.statusListenersMu.Unlock()
    return func() {
        e.statusListenersMu.Lock()
        delete(e.statusListeners, id)
        e.statusListenersMu.Unlock()
    }
}

func (e *Engine) fanoutStatus(msg StatusMessage) {
    e.statusListenersMu.RLock()
    defer e.statusListenersMu.RUnlock()
    for _, fn := range e.statusListeners {
        fn(msg)
    }
}
```

`makeStatusFunc` ruft `fanoutStatus` zusätzlich auf — aber **Status Nodes selbst werden vom Fan-out ausgenommen**, damit kein Status Node jemals den Status eines anderen Status Nodes empfängt:

```go
func (e *Engine) makeStatusFunc(nodeID string, rn *runningNode) StatusFunc {
    return func(fill string, text string) {
        msg := StatusMessage{
            NodeID:     nodeID,
            FlowID:     rn.flowID,
            Status:     NodeStatusPayload{Fill: fill, Text: text},
            SourceType: rn.config.Type,
            SourceName: rn.config.Name,
        }
        e.statusCache.Set(nodeID, msg)
        if rn.config.Type != "status" {
            e.fanoutStatus(msg)
        }
        if e.publishStatus != nil {
            subject := fmt.Sprintf("status.%s.%s", rn.flowID, nodeID)
            e.publishStatus(subject, msg)
        }
    }
}
```

Die Filterung im Fan-out (statt im Status Node selbst) hat zwei Vorteile: Sie ist DRY (eine Stelle, gilt für alle künftigen Status Nodes / Listener) und sie spart die Closure-Calls komplett — bei vielen Status Nodes kein O(n)-Aufwand pro Status-Event eines anderen Status Nodes.

In `wireAllNodes` Provider-Injection analog zu `LinkProvider`:

```go
for _, rn := range e.nodes {
    if slp, ok := rn.instance.(StatusListenerProvider); ok {
        slp.SetStatusListener(e.registerStatusListener)
    }
}
```

`instantiateNode` befüllt `NodeConfig.FlowID` aus dem `runningNode.flowID`.

### Backend — `internal/nodes/status.go` (neu)

```go
type StatusNode struct {
    config flow.NodeConfig
    send   flow.SendFunc
    status flow.StatusFunc
    debug  flow.DebugFunc

    scope       string              // "flow", "selected" oder "all"
    targetNodes map[string]struct{} // populated bei scope == "selected"

    register   func(flow.StatusListenerFunc) func()
    unregister func()

    blinkSeq atomic.Uint64
    mu       sync.Mutex
    started  bool
}

func (n *StatusNode) SetStatusListener(register func(flow.StatusListenerFunc) func()) {
    n.mu.Lock()
    defer n.mu.Unlock()
    n.register = register
    // Re-Wire bei modified-nodes deploy: alte Subscription droppen
    if n.started {
        if n.unregister != nil { n.unregister() }
        n.unregister = register(n.handleStatus)
    }
}

func (n *StatusNode) handleStatus(sm flow.StatusMessage) {
    switch n.scope {
    case "flow":
        if sm.FlowID != n.config.FlowID { return }
    case "selected":
        // Doppelter Schutz: Selection ist immer flow-lokal
        if sm.FlowID != n.config.FlowID { return }
        if _, ok := n.targetNodes[sm.NodeID]; !ok { return }
    case "all":
        // kein Filter
    }
    msg := flow.NewMessage()
    msg.Set("status", map[string]any{
        "fill": sm.Status.Fill,
        "text": sm.Status.Text,
        "source": map[string]any{
            "id": sm.NodeID, "type": sm.SourceType,
            "name": sm.SourceName, "flowId": sm.FlowID,
        },
    })
    msg.SetPayload(sm.Status.Text)
    n.send(0, msg)
    n.blink(sm)
}
```

Der Status Node hat `HandleMessage` als No-Op (returns `nil, nil`) — er ist Source-Only, aber `nodeLoop` der Engine bleibt für ihn aktiv und blockiert auf seiner leeren `inputCh` bis `stopCh` schließt.

### Frontend — `frontend/src/components/nodes/StatusNode.vue` (neu)

- Eigene Vue-Komponente analog `InjectNode.vue` — nur Output-Handle, kein Input
- Body-Anzeige zeigt den Scope: `this flow` / `<N> nodes` / `all flows`

### Frontend — `frontend/src/components/config/StatusConfig.vue` (neu)

- Scope-Dropdown (`flow` / `selected` / `all`) via `useNodeProperty`
- Bei `selected`: scrollbare Checkbox-Liste der Kandidaten
  - Quelle: `flowStore.activeNodes` — automatisch nur der aktuelle Flow
  - Filter: ohne den selektierten Status Node selbst, ohne andere Status Nodes (Type-Filter)
  - Sortiert nach Label, Anzeige `<label> (<type>)`
  - Toggle persistiert `targetNodes: string[]` in der Node-Config
- Beim Hover über ein Item: `flowStore.setHoveredHighlightNodeId(id)` → Flow zeigt gestrichelte Outline
- `onBeforeUnmount` resettet den Hover-State

### Frontend — Hover-Highlight Generalisierung

`flowStore`:
- `hoveredDebugNodeId` → `hoveredHighlightNodeId`
- `setHoveredDebugNodeId` → `setHoveredHighlightNodeId`

`BaseNode.vue`:
- `isDebugHovered` → `isHighlighted`
- Outline-Style-Binding angepasst

`DebugPanel.vue`: Setter-Aufrufe umbenannt (mouseenter/mouseleave + `onBeforeUnmount`).

Beide Konsumenten (DebugPanel, StatusConfig) nutzen denselben Mechanismus — kein Code-Duplikat.

### Frontend — Node-Palette

Neuer Eintrag erscheint **automatisch** unter "Common", weil die Palette über `api.getNodes()` aus dem Backend-Registry geladen wird (`StatusTypeInfo` mit `Category: "common"`). Kein zusätzliches Frontend-Mapping nötig.

### Frontend — Icon

`NodeIcon.vue` hatte den `status`-Eintrag (Heartbeat-Wave-Path) bereits seit längerem registriert — kein zusätzlicher Eintrag nötig.

## Betroffene Dateien

### Backend
- `internal/flow/registry.go` — `StatusListenerFunc`, `StatusListenerProvider`, `StatusMessage` um `SourceType`/`SourceName`, `NodeConfig` um `FlowID` erweitert
- `internal/flow/engine.go` — `statusListeners`-Map + Mutex + Sequence, `registerStatusListener`, `fanoutStatus`, Aufruf in `makeStatusFunc` (mit `Type != "status"`-Filter), Provider-Injection in `wireAllNodes`, `FlowID` in `instantiateNode` befüllt
- `internal/nodes/status.go` (neu) — `StatusNode` Source-Only mit Scope `flow`/`selected`/`all`, Re-Wire-sicher, Idle-Status + 2s-Blink
- `internal/server/server.go` — Registrierung `registry.Register("status", nodes.NewStatusNode, nodes.StatusTypeInfo())`

### Frontend
- `frontend/src/components/nodes/StatusNode.vue` (neu) — 0/1 Ports, Scope-Anzeige im Body
- `frontend/src/components/config/StatusConfig.vue` (neu) — Scope-Dropdown + Multi-Select-Checkbox-Liste mit Hover-Highlight
- `frontend/src/views/FlowEditor.vue` — Import + `<template #node-status>`
- `frontend/src/components/PropertyPanel.vue` — `<StatusConfig>` für `type === 'status'`
- `frontend/src/stores/flowStore.ts` — Hover-State umbenannt (generisch)
- `frontend/src/components/nodes/BaseNode.vue` — Highlight-Computed umbenannt, Style-Binding angepasst
- `frontend/src/components/DebugPanel.vue` — Setter-Aufrufe umbenannt

## Abhängigkeiten

- **Status-Pipeline** muss laufen (`NODE_STATUS_PIPELINE.md`) — sonst kommen keine Status-Events am Engine-Fan-out an
- **Status-Cache** (`NODE_STATUS.md`) ist **nicht** zwingend — der Status Node braucht nur Live-Events, keinen Replay. Beim Start hat er per Definition leeren Zustand, das ist akzeptiert
- Multi-Select greift auf `flowStore.activeNodes` zu — Liste der Nodes des aktiven Flows ist dort bereits verfügbar

## Out of Scope für Phase 1

- **Filter "nur bei Änderung"** — erst beobachten, ob Floods auftreten
- **Filter nach `fill`-Wert** — selten gebraucht, kann ein nachgeschalteter Switch Node lösen
- **Status-Source = anderer Status Node** — explizit ausgeschlossen, Endlosschleifen architektonisch verhindert
- **Persistente Subscription über Server-Restart** — bei Restart sind ohnehin alle laufenden Status weg
- **Status-Replay** beim Start des Status Nodes (würde den letzten gecachten Status aller Quell-Nodes als initiale Messages emitten) — denkbar, aber semantisch unsauber (Replay vs. Live-Events)
- **Selected-Mode flow-übergreifend** — Multi-Select ist strikt flow-lokal. Wer flow-übergreifend will, nimmt `scope: all` und filtert im Flow nachgelagert per Switch Node auf `msg.status.source.flowId`

## Offene Fragen

Keine.
