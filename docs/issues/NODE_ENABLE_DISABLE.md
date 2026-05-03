# Issue: Node Enable/Disable — einzelne Nodes deaktivieren ohne Löschen

## Status: Done

## Problembeschreibung

Aktuell muss eine Node, die im Flow temporär nicht laufen soll, **gelöscht** werden — alternativ auch der ganze Flow deaktiviert werden (`Flow.Disabled` existiert auf Datenebene, aber nicht als UI-Toggle). Beides ist unangemessen für den häufigen Fall:

- **Debugging**: einen problematischen MQTT-Out vorübergehend stilllegen, ohne die Konfiguration zu verlieren
- **Selektives Testen**: in einem Flow mit mehreren parallelen Zweigen einen Zweig isolieren
- **Wartung**: einen Inject-Trigger pausieren, während der Backend-Zielservice gerade neu gestartet wird
- **Schrittweises Einschalten** beim Aufbau eines Flows: Nodes deaktiviert anlegen, später aktivieren

In Node-RED ist das `disable`/`enable` über Properties-Panel und Rechtsklick eine der meistgenutzten Editor-Operationen. Für LOOPZE fehlt sie komplett, obwohl die Datenstruktur und der Engine-Skip-Pfad bereits vorbereitet sind.

## Sichtweise / Begründung

Die Vorarbeit existiert bereits — sowohl Backend als auch Frontend tragen das Feld bereits, ohne dass es im UI bedienbar wäre:

- `internal/flow/types.go:80` — `Node.Disabled bool` mit JSON-Tag, Teil der Persistenz
- `internal/flow/engine.go:465-468` — `instantiateNode` überspringt disabled Nodes vor Init/Start
- `frontend/src/types/flow.ts:84` — `disabled?: boolean` im Node-Typ
- `frontend/src/components/nodes/BaseNode.vue:25,42` — Prop `disabled?: boolean` deklariert, aber visuell unbenutzt
- `internal/flow/diff.go` + `Engine.Deploy(..., DeployModifiedNodes)` — Partial Deploy stoppt/startet einzelne Nodes ohne Flow-Restart

Damit ist der Engine-Pfad sauber: Toggle der `Disabled`-Flag macht die Node beim nächsten Partial Deploy zur "modified node", `stopAndRemoveNodes` stoppt sie, `instantiateNode` überspringt sie. Kein neuer Lifecycle-Zweig nötig.

Was fehlt, ist die **UI-Bedienung**, die **visuelle Darstellung**, und das **saubere Routing-Verhalten** (keine Wire-Warnings für vorhersehbar fehlende Targets).

## Anforderungen

### 1. Toggle im Properties-Panel

Im Properties-Panel der ausgewählten Node erscheint **oberhalb** der typ-spezifischen Config-Sektion ein allgemeiner Bereich „Node" mit einer Checkbox `Enabled` (default `true`, gespiegelt zu `disabled === false`).

- Toggle persistiert direkt in `node.disabled` über den Flow-Store (`flowStore.updateNode`)
- Setzt die Node als `dirty` (existierender Mechanismus `dirtyNodeIds`), damit sie beim nächsten Deploy berücksichtigt wird
- Auch vorhandene Felder wie `name` und (perspektivisch) `info` gehören in dieses allgemeine Properties-Segment — ein einzelner Toggle rechtfertigt keine eigene Sektion, aber als Aufhänger für künftige allgemeine Node-Eigenschaften ist die Stelle die richtige

### 2. Visuelle Darstellung am Node

Eine deaktivierte Node muss auf den ersten Blick als solche erkennbar sein:

| Element | Aktiv | Deaktiviert |
|---|---|---|
| Body-Opacity | `1.0` | `0.4` |
| Border-Style | `solid` | `dashed` |
| Status-Punkt | wie bisher | **ausgeblendet** (eine deaktivierte Node hat keinen Live-Status) |
| Action/Toggle-Buttons | aktiv | gerendert, aber `pointer-events: none` und ebenfalls `opacity: 0.4` |

Die Werte werden in `BaseNode.vue` über die bereits vorhandene `disabled`-Prop konsumiert. Selektion und Highlight bleiben sichtbar — eine deaktivierte Node muss anwählbar bleiben, sonst kann sie nicht reaktiviert werden.

### 3. Wire-Verhalten

Eingehende Wires zu einer deaktivierten Node:
- Backend: Messages werden im Engine-Routing **stillschweigend** verworfen (kein `slog.Warn`). Aktuell warnt `makeSendFunc` bei unbekannten Targets — das ist für vom Bediener bewusst deaktivierte Nodes Lärm.
- Frontend: Wire bleibt sichtbar, aber gestrichelt und mit reduzierter Opacity (analog zur Node), damit die Unterbrechung im Flow erkennbar ist.

Ausgehende Wires einer deaktivierten Node sind belanglos — die Node läuft nicht, sie produziert keine Messages. Kein zusätzlicher Code.

### 4. Keyboard-Shortcut

`Ctrl+E` / `Cmd+E` toggelt den `disabled`-Zustand der aktuell selektierten Node(s).

**Bulk-Semantik bei Multi-Select:**
- Sind **alle** selektierten Nodes aktiv → alle deaktivieren
- Sind **alle** selektierten Nodes deaktiviert → alle aktivieren
- **Gemischt** → alle aktivieren (das ist die geringst-überraschende Wahl: man kommt aus dem Mischzustand zuverlässig wieder raus, indem man zweimal drückt)

Der Shortcut wird in `FlowEditor.vue` analog zu den vorhandenen Ctrl+C/V/X/D-Bindings registriert — gleiche Skip-Logik bei Input/Textarea-Fokus.

### 5. Beim Deploy

Kein Sonderfall:
- Modus `nodes` (Default): geänderte `disabled`-Flag macht die Node zur modifizierten Node, der bestehende `deployModifiedNodes`-Pfad erledigt Stop bzw. Start.
- Modus `flows` / `full`: ohnehin alles neu — disabled Nodes werden beim Re-Instantiation übersprungen.

Es gibt **keinen** "Live-Disable"-Pfad, der die Node ohne Deploy aushebelt — Konsistenz mit dem Rest des Systems: Änderungen werden erst nach Deploy wirksam, der `dirty`-Indikator zeigt das an.

### 6. Persistenz und Workspace-Diff

- Toggle erzeugt einen `dirty`-State im Frontend (`flowStore.markNodeDirty`)
- `WorkspaceDiff.ModifiedNodes` enthält die Node, sobald `disabled` zwischen letztem deployten und aktuellem Stand abweicht — das fällt automatisch durch den existierenden Diff-Mechanismus, da `Disabled` Teil des Node-Hash ist, sofern dieser auf der vollen Struktur arbeitet. Falls der Diff aktuell nur `Config` hasht, muss `Disabled` hier mit aufgenommen werden — siehe Technische Skizze.

## Technische Skizze

### Backend — `internal/flow/engine.go`

`makeSendFunc` (Z. 855-876): die `slog.Warn`-Zeile bei `targetNode == nil` differenziert behandeln. Variante: vor dem Loggen prüfen, ob das Target zur Engine-Wires-Map gehört, dort aber als `disabled` bekannt ist. Sauberer: in `wireAllNodes` die `targets`-Map nur mit aktiv laufenden Nodes befüllen (das ist sie de-facto schon, da disabled Nodes nicht in `e.nodes` landen), und die **Warning auf Debug-Level** runtersetzen — denn das Fehlen eines Targets ist bei aktiviertem Disable-Feature ein Normalfall, nicht Warnung-würdig:

```go
for _, targetID := range wires[port] {
    targetNode := targets[targetID]
    if targetNode == nil {
        slog.Debug("wire target not active (disabled or unknown)",
            "source", sourceID, "target", targetID, "port", port)
        continue
    }
    ...
}
```

Falls echte "Wire ins Leere" (Frontend-Bug, korrupter Flow) später noch separat geloggt werden sollen, kann ein zweites Set `e.knownNodeIDs` (alle IDs aus dem Flow, auch disabled) helfen — das ist aber out of scope.

### Backend — `internal/flow/diff.go`

Sicherstellen, dass `Disabled` Bestandteil der Node-Vergleichslogik ist. Falls der bisherige Diff Nodes über JSON-Roundtrip oder Struct-Equality vergleicht, ist `Disabled` automatisch dabei (es ist ein exportiertes Feld). Falls er ausschließlich `Config` vergleicht, ergänzen.

### Backend — Tests (`internal/flow/engine_test.go`)

| Test | Prüft |
|---|---|
| `TestDisabledNodeNotInstantiated` | Flow mit `Node.Disabled = true` → Node nicht in `e.nodes` (existiert vermutlich schon implizit) |
| `TestWireToDisabledTargetDropsSilently` | Aktive Source → disabled Target: keine Panic, keine Warning, Message verworfen |
| `TestPartialDeployDisableStopsRunningNode` | Node läuft → Deploy mit `Disabled: true` (`DeployModifiedNodes`) → Node-Goroutine beendet |
| `TestPartialDeployEnableStartsNode` | Node ist disabled, Deploy mit `Disabled: false` → Node läuft, empfängt Messages |
| `TestDisableMidFlowDoesNotKillUpstream` | Mittlere Node von 3-Hop-Flow disablen → erste Node läuft weiter, dritte erhält keine Messages |

### Frontend — `frontend/src/components/PropertyPanel.vue`

Neue Sektion oberhalb des typ-spezifischen Editors:

```
┌────────────────────────────────┐
│ Node                           │
│ Name  [_____________________]  │
│ ☑ Enabled                      │
└────────────────────────────────┘
```

`Name` existiert vermutlich schon als Edit-Feld irgendwo — wenn ja, bleibt er dort und nur die Checkbox kommt neu. Die Checkbox bindet auf `!nodeData.disabled` und ruft beim Toggle `flowStore.updateNode(nodeId, { disabled: <bool> })`.

### Frontend — `frontend/src/components/nodes/BaseNode.vue`

Bestehende `disabled`-Prop (Z. 25, 42) wird visuell genutzt:

```vue
<div
  class="node-body"
  :class="{ 'node-body--disabled': disabled }"
  :style="{ minHeight: nodeMinHeight }"
>
```

Im Style-Block:

```css
.node-body--disabled {
  opacity: 0.4;
  border-style: dashed;
}
.node-body--disabled .status-dot { display: none; }
.node-body--disabled .action-btn,
.node-body--disabled .toggle-btn { pointer-events: none; }
```

Die Prop wird in den konkreten Node-Komponenten (`MqttInNode.vue`, `InjectNode.vue`, etc.) durchgereicht — bei den meisten passiert das schon implizit über `v-bind="$props"` an `<BaseNode>`. Wo das fehlt, ergänzen.

### Frontend — Wire-Styling

In der Vue-Flow-Edge-Konfiguration (vermutlich `FlowEditor.vue` oder eine `customEdges`-Datei) Edges, deren Source **oder** Target eine disabled Node ist, mit `stroke-dasharray: 4 4` und reduzierter Opacity rendern. Computed über `flowStore.activeNodes` als Lookup.

### Frontend — Keyboard-Shortcut (`frontend/src/views/FlowEditor.vue`)

Ergänzen im bestehenden `keydown`-Handler (Z. 134-172):

```ts
case 'e': {
  event.preventDefault()
  const ids = flowStore.selectedNodeIds
  if (ids.length === 0) return
  const nodes = ids.map(id => flowStore.getNode(id)).filter(Boolean)
  const allDisabled = nodes.every(n => n.disabled)
  const target = !allDisabled ? true : false
  // wenn alle gleich: kippen; wenn gemischt: auf "false" (alle aktivieren) — siehe Anforderung
  const next = nodes.every(n => !!n.disabled === !!nodes[0].disabled)
    ? !nodes[0].disabled
    : false
  for (const n of nodes) flowStore.updateNode(n.id, { disabled: next })
  break
}
```

### Frontend — Store-Helper (`frontend/src/stores/flowStore.ts`)

Falls `updateNode` noch nicht existiert oder zu generisch ist, eine konkrete Methode:

```ts
function setNodeDisabled(nodeId: string, disabled: boolean) {
  const node = getNode(nodeId)
  if (!node) return
  if (!!node.disabled === disabled) return
  node.disabled = disabled
  markNodeDirty(nodeId)
}
```

## Betroffene Dateien

### Backend
- `internal/flow/engine.go` — `makeSendFunc`: Warning auf Debug-Level für nicht-aktive Targets
- `internal/flow/diff.go` — sicherstellen, dass `Disabled` Bestandteil des Node-Diffs ist (ggf. Anpassung)
- `internal/flow/engine_test.go` — neue Tests (siehe Tabelle oben)

### Frontend
- `frontend/src/components/PropertyPanel.vue` — neue „Node"-Sektion mit Enabled-Checkbox
- `frontend/src/components/nodes/BaseNode.vue` — visuelles Styling für `disabled`
- `frontend/src/components/nodes/*.vue` — Sicherstellen, dass `disabled` an `BaseNode` durchgereicht wird (sofern nicht via `$props`)
- `frontend/src/views/FlowEditor.vue` — Ctrl+E Shortcut, Edge-Styling für disabled Endpunkte
- `frontend/src/stores/flowStore.ts` — `setNodeDisabled` Helper, falls nicht über generisches `updateNode` lösbar
- `frontend/src/components/help/docs.ts` — kurzer Hinweis im allgemeinen Editor-Hilfeeintrag, dass `Ctrl+E` Nodes ein-/ausschaltet

### Doku
- `docs/MISSING_FUNCTIONALITY.md` — Eintrag „Node Enable/Disable" auf erledigt setzen, Link auf dieses Issue

## Abhängigkeiten

- **Partial Deploy** (`DeployModifiedNodes`) muss laufen — ist erledigt (`internal/flow/diff.go`, Z. 369ff.)
- **Multi-Select** im Frontend — vorhanden (`flowStore.selectedNodeIds`)
- **Dirty-Tracking** — vorhanden (`dirtyNodeIds`)
- Keine neuen externen Abhängigkeiten

## Out of Scope für Phase 1

- **Kontextmenü (Rechtsklick)** mit „Disable selected" / „Enable selected" — kein Kontextmenü-System im Editor existiert. Eigenes Issue, weil eigenes UX-Subsystem (Menü-Komponente, Positionierung, Schließverhalten, weitere Einträge wie Copy/Paste/Delete).
- **Bypass-Mode** (Messages durch deaktivierte Node *durchreichen* statt droppen) — semantisch kontrovers (Output-Port-Mapping bei mehreren Inputs/Outputs?). Erst implementieren, wenn ein realer Use-Case kommt.
- **Disabled für einzelne Wires** — Node-RED hat das nicht, wir auch nicht.
- **Disabled-Status im Status-Cache** — eine deaktivierte Node hat keinen Live-Status; der letzte vor dem Disable bekannte Status wird beim Stop ohnehin verworfen, das ist konsistent.
- **Flow-weiter Toggle in der Tab-Leiste** — `Flow.Disabled` existiert auf Datenebene; das UI dafür ist eigene Sache (Tab-Kontextmenü o.ä.).
- **Per-Subflow-Instanz Toggle** — Subflows existieren noch nicht.

## Offene Fragen

- **Soll der `dirty`-State beim Toggle auch optisch markiert werden** (z.B. blinkender Punkt), oder reicht der bestehende Indikator? → Bestehender Indikator reicht, keine Sonderbehandlung.
- **Sollen disabled Nodes von den Status-Aggregationen** (Status Node, Catch Node) ignoriert werden? → Ja, automatisch — sie laufen nicht und erzeugen keine Status/Error-Events. Kein zusätzlicher Code nötig.
