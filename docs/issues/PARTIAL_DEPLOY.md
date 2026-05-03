# Issue: Partial Deploy – Drei Deployment-Modi

## Status: Open

## Problembeschreibung

Aktuell verwendet LOOPZE eine **Full-Restart-Strategie**: Bei jedem Deploy werden **alle** laufenden Nodes gestoppt und der gesamte Workspace neu instantiiert (`engine.go:204-205`). Das ist bei größeren Workspaces problematisch:

- **Downtime**: Alle Flows werden kurzzeitig unterbrochen — auch solche, die sich nicht geändert haben
- **Message-Verlust**: Messages in Node-Buffern (`inputCh`) gehen beim Stop verloren
- **Zustandsverlust**: In-Memory Node-State (z.B. Zähler in Function-Nodes) wird bei jedem Deploy zurückgesetzt
- **Config-Node Restart**: Shared Resources (MQTT-Verbindungen etc.) werden unnötig getrennt und neu aufgebaut

Node-RED löst das mit drei Deploy-Modi, die der User per Dropdown neben dem Deploy-Button auswählen kann.

## Die drei Deploy-Modi

### 1. Modified Nodes (Standard)
Nur Nodes/Flows die sich tatsächlich geändert haben werden neu deployed.

**Logik:**
- Diff zwischen aktuellem und neuem Workspace berechnen
- Nur betroffene Nodes stoppen und neu starten
- Unveränderte Nodes laufen unterbrechungsfrei weiter
- Config-Nodes nur neu starten wenn sich deren Config geändert hat

### 2. Modified Flows
Alle Flows die mindestens eine Änderung enthalten werden komplett neu deployed.

**Logik:**
- Flow-Level-Diff: Hat sich ein Flow geändert? (Nodes, Wires, Properties)
- Betroffene Flows komplett stoppen und neu starten
- Unveränderte Flows laufen weiter

### 3. Full (Restart All)
Kompletter Workspace wird gestoppt und neu deployed. Alle Nodes werden gestoppt, neu instantiiert und gestartet. Entspricht dem aktuellen Verhalten.

### 4. Restart
Engine wird komplett heruntergefahren und neu gestartet (`Engine.Stop()` → `Engine.Start()` → `Engine.Deploy()`). Setzt **alles** zurück — inklusive Engine-State, Context-Stores (Memory), NATS-Verbindungen und Config-Node-Ressourcen. Nützlich wenn sich das System in einen inkonsistenten Zustand gebracht hat oder ein sauberer Neustart gewünscht ist.

## Technische Umsetzung

### Phase 1: Diff-Engine (Backend)

Neues Paket/Datei `internal/flow/diff.go`:

```go
type DeployMode string

const (
    DeployModifiedNodes DeployMode = "nodes"    // nur geänderte Nodes
    DeployModifiedFlows DeployMode = "flows"    // ganze Flows mit Änderungen
    DeployFull          DeployMode = "full"     // alles neu (aktuelles Verhalten)
    DeployRestart       DeployMode = "restart"  // Engine Stop → Start → Deploy
)

type WorkspaceDiff struct {
    AddedFlows    []string   // neue Flow-IDs
    RemovedFlows  []string   // gelöschte Flow-IDs
    ModifiedFlows []string   // Flows mit Änderungen

    AddedNodes    []string   // neue Node-IDs
    RemovedNodes  []string   // gelöschte Node-IDs
    ModifiedNodes []string   // Nodes mit geänderter Config/Wires

    AddedConfigs    []string // neue Config-Node-IDs
    RemovedConfigs  []string // gelöschte Config-Nodes
    ModifiedConfigs []string // Config-Nodes mit Änderungen
}

func DiffWorkspaces(old, new Workspace) WorkspaceDiff { ... }
```

**Diff-Kriterien für Nodes:**
- Config-Map geändert (Deep-Equal)
- Wires geändert (Verbindungen umgesteckt)
- Disabled-Flag geändert
- Position (X/Y) ist **kein** Diff-Kriterium (nur visuell)

**Diff-Kriterien für Flows:**
- Nodes hinzugefügt/entfernt
- Mindestens ein Node geändert (s.o.)
- Flow Disabled-Flag geändert
- Flow Env-Vars geändert

### Phase 2: Selektiver Node-Lifecycle (Backend)

`Engine.Deploy()` refactoren zu `Engine.Deploy(flows, configs, mode)`:

#### Mode: `DeployRestart`
```
1. Engine.Stop() — alle Nodes stoppen, State clearen, configInstances nil
2. Engine.Start() — Engine frisch initialisieren
3. Engine.Deploy(flows, configs, DeployFull) — alles neu aufbauen
```
Härtester Reset: Engine-Lifecycle wird komplett durchlaufen. Alle In-Memory-Zustände (Node-Context, Status-Cache) gehen verloren.

#### Mode: `DeployFull`
Bisheriges Verhalten — `stopNodes()` → alles neu aufbauen. Engine bleibt running.

#### Mode: `DeployModifiedFlows`
```
1. Diff berechnen
2. Nur Nodes in betroffenen Flows stoppen (stopNodesInFlows)
3. Config-Nodes prüfen: geänderte Config-Nodes neu starten
4. Betroffene Flows neu instantiieren + wiren
5. Neue Node-Goroutines starten
6. Unveränderte Flows bleiben running
```

#### Mode: `DeployModifiedNodes`
```
1. Diff berechnen
2. Removed Nodes stoppen + Channels schließen
3. Modified Nodes stoppen (aber nicht aus maps entfernen)
4. Config-Nodes prüfen: geänderte neu starten
5. Added + Modified Nodes instantiieren + Init
6. Wires für betroffene Nodes neu aufbauen
7. Betroffene Nodes starten + Goroutines launchen
8. Downstream-Nodes von geänderten Wires: SendFunc aktualisieren
```

**Herausforderungen beim selektiven Stop:**
- `stopCh` wird aktuell für **alle** Nodes geteilt — muss pro Node/Flow werden
- `wg.Wait()` wartet auf alle Goroutines — muss selektiv werden
- Wires von unveränderten Nodes können auf geänderte Nodes zeigen → SendFunc muss aktualisiert werden
- Link-Registry muss partiell aktualisiert werden

#### Vorgeschlagene Änderungen an `runningNode`:

```go
type runningNode struct {
    instance NodeInstance
    config   NodeConfig
    flowID   string
    inputCh  chan *Message
    stopCh   chan struct{}  // NEU: pro-Node stop channel (statt global)
    done     chan struct{}  // NEU: signalisiert dass Goroutine beendet ist
}
```

#### Vorgeschlagene neue Engine-Methoden:

```go
// stopNode stoppt einen einzelnen Node und seine Goroutine
func (e *Engine) stopNode(nodeID string) error

// stopFlow stoppt alle Nodes eines Flows
func (e *Engine) stopFlow(flowID string) error

// rewireNode aktualisiert die SendFunc eines Nodes mit neuen Wires
func (e *Engine) rewireNode(nodeID string, wires [][]string)

// startNode instantiiert, initialisiert und startet einen einzelnen Node
func (e *Engine) startNode(nodeID string, node Node, flowID string) error
```

### Phase 3: API-Erweiterung

`deployRequest` erweitern:

```go
type deployRequest struct {
    Flows   []flow.Flow       `json:"flows"`
    Configs []flow.ConfigNode `json:"configs,omitempty"`
    Rev     string            `json:"rev,omitempty"`
    Mode    string            `json:"deployMode,omitempty"` // "nodes", "flows", "full", "restart"
}
```

Default wenn `Mode` leer: `"nodes"` (Modified Nodes).

Der Handler leitet den Mode an `Engine.Deploy()` weiter. Bei `"restart"` ruft der Handler `Engine.Stop()` → `Engine.Start()` vor dem Deploy auf:

```go
// Engine-Signatur
func (e *Engine) Deploy(flows []Flow, configs []ConfigNode, mode DeployMode) error

// Handler-Logik für Restart
if req.Mode == "restart" {
    d.Engine.Stop()
    d.Engine.Start()
}
d.Engine.Deploy(req.Flows, req.Configs, flow.DeployFull)
```

### Phase 4: Frontend

#### Deploy-Button mit Dropdown
Der Deploy-Button bekommt einen Dropdown-Pfeil (Split-Button) wie in Node-RED:

```
┌──────────┬───┐
│  Deploy  │ ▾ │
└──────────┴───┘
              │
              ├─ ● Modified Nodes  (Standard)
              ├─ ○ Modified Flows
              ├─ ○ Full Deploy
              └─ ○ Restart
```

- Klick auf "Deploy" → deployed mit dem aktuell gewähltem Modus
- Klick auf ▾ → Dropdown öffnet sich, Modus kann gewechselt werden
- Gewählter Modus wird in `localStorage` gespeichert

#### flowStore Änderungen

```typescript
// Neuer State
const deployMode = ref<'nodes' | 'flows' | 'full' | 'restart'>('nodes')

// Deploy-Payload erweitern
const payload: DeployPayload = {
    flows: flows.value,
    configs: configs.value.length > 0 ? configs.value : undefined,
    rev: revision.value ?? undefined,
    deployMode: deployMode.value,
}
```

Die vorhandenen `dirtyNodeIds` und `dirtyFlowIds` Sets werden bereits getrackt und können für visuelle Hinweise genutzt werden (z.B. geänderte Nodes/Flows markieren).

### Phase 5: Deploy-Feedback

WebSocket-Event erweitern um den Deploy-Modus und betroffene Flows/Nodes:

```json
{
    "action": "deployed",
    "revision": "a1b2c3d4",
    "mode": "nodes",
    "affected": {
        "added": ["node-id-1"],
        "modified": ["node-id-2", "node-id-3"],
        "removed": ["node-id-4"]
    }
}
```

## Implementierungsreihenfolge

1. **Diff-Engine** (`diff.go` + Tests) — Grundlage für alles
2. **Per-Node Stop-Channel** — `stopCh` von global auf pro-Node umbauen
3. **`DeployModifiedFlows`** — einfacher als Node-Level, guter Zwischenschritt
4. **`DeployModifiedNodes`** — der eigentliche Kern
5. **API-Erweiterung** — `deployMode` Parameter
6. **Frontend Dropdown** — Split-Button am Deploy-Button
7. **Deploy-Feedback** — Erweiterte WebSocket-Events

## Betroffene Dateien

### Backend
- `internal/flow/diff.go` — **NEU**: Workspace-Diff-Logik
- `internal/flow/diff_test.go` — **NEU**: Tests für Diff
- `internal/flow/engine.go` — Refactoring Deploy/Stop für selektiven Lifecycle
- `internal/flow/types.go` — `DeployMode` Type
- `internal/api/handlers.go` — `deployMode` aus Request lesen + weiterleiten

### Frontend
- `frontend/src/stores/flowStore.ts` — `deployMode` State + Payload
- `frontend/src/types/flow.ts` — `DeployPayload` Type erweitern
- `frontend/src/components/HeaderBar.vue` — Split-Button mit Dropdown

## Edge Cases

- **Erster Deploy** (kein alter State): Immer Full Deploy
- **Flow hinzugefügt/gelöscht**: Neuer Flow wird gestartet, gelöschter wird gestoppt, Rest bleibt
- **Config-Node geändert**: Alle Nodes die diese Config referenzieren müssen ebenfalls neu gestartet werden (Cascading Restart)
- **Wire auf gelöschten Node**: SendFunc muss graceful mit fehlenden Targets umgehen (tut sie bereits via `e.nodes[targetID]` Lookup)
- **Link-Nodes**: Änderung an Link-In/Out betrifft Cross-Flow-Kommunikation → Link-Registry partiell updaten
- **Disabled-Flag Toggle**: Node disablen = Node stoppen; Node enablen = Node starten
