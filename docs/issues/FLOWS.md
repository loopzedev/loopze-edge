# Issue: Flow-Management – Tabs, Erstellen, Löschen, Sortieren

## Status: Open

## Problembeschreibung

Ein Workspace kann mehrere Flows enthalten. Die Infrastruktur dafür existiert bereits im Backend (`workspace.json` speichert ein Array von Flows) und im FlowStore (`flows[]`, `activeFlowId`, `setActiveFlow()`). Es fehlt jedoch die **UI zum Verwalten mehrerer Flows** — aktuell wird nur der erste Flow geladen und es gibt keine Möglichkeit, zwischen Flows zu wechseln, neue anzulegen oder bestehende zu löschen.

## Anforderungen

### 1. Flow-Tab-Bar

- Horizontale Leiste **oberhalb des FlowEditors**, unterhalb der HeaderBar
- Zeigt alle Flows des Workspace als Tabs an
- Der aktive Flow-Tab ist visuell hervorgehoben
- Klick auf einen Tab wechselt den aktiven Flow (`flowStore.setActiveFlow(flowId)`)
- Am **rechten Ende** der Tab-Leiste befindet sich ein Tab mit **Plus-Icon** (+) zum Anlegen eines neuen Flows

### 2. Neuen Flow anlegen

- Klick auf den Plus-Tab (+) öffnet die **Properties Sidebar** mit einem Flow-Erstellungsformular
- Das Formular enthält:
  - **Name**: Textfeld für den Flow-Namen (Pflichtfeld)
  - **Anlegen-Button**: Erstellt den Flow und aktiviert ihn
  - **Abbrechen-Button**: Schließt das Formular ohne Änderung
- Nach dem Anlegen:
  - Der neue Flow wird in `flowStore.flows` hinzugefügt (`flowStore.addFlow(label)`)
  - Der neue Flow wird automatisch als aktiver Flow gesetzt
  - Der Workspace wird als **dirty** markiert
  - Der neue Flow-Tab erscheint in der Tab-Leiste
  - Der FlowEditor zeigt eine leere Canvas

### 3. Flow bearbeiten (Properties)

- **Doppelklick** auf einen vorhandenen Flow-Tab öffnet die Properties Sidebar mit den Flow-Einstellungen
- Die Flow-Properties zeigen:
  - **Name**: Textfeld zum Umbenennen des Flows
  - **Aktiviert/Deaktiviert**: Toggle zum Deaktivieren des gesamten Flows
  - **Löschen-Button**: Löscht den Flow nach Bestätigung
- Änderungen am Namen oder Aktiviert-Status markieren den Workspace als **dirty**
- Ein deaktivierter Flow wird beim Deploy an das Backend übermittelt, aber die Engine startet keine Nodes für diesen Flow

### 4. Flow deaktivieren

- Ein deaktivierter Flow erhält `disabled: true` in der Flow-Konfiguration
- Der Tab eines deaktivierten Flows wird visuell abgeschwächt dargestellt (z.B. reduzierte Opacity, durchgestrichener Name oder ausgegraut)
- Der Bediener kann einen deaktivierten Flow weiterhin öffnen und bearbeiten
- Erst nach Deploy wird der deaktivierte Flow vom Backend ignoriert
- Reaktivierung setzt `disabled: false` und markiert als dirty

### 5. Flow löschen

- Der Löschen-Button in den Flow-Properties öffnet einen **Bestätigungsdialog**
- Nach Bestätigung:
  - Der Flow wird aus `flowStore.flows` entfernt (`flowStore.removeFlow(flowId)`)
  - Ist der gelöschte Flow der aktive Flow, wird automatisch zum nächsten (oder vorherigen) Flow gewechselt
  - Der Workspace wird als dirty markiert
  - Der letzte verbleibende Flow kann **nicht** gelöscht werden (Button deaktiviert oder Hinweis anzeigen)

### 6. Tabs per Drag & Drop sortieren

- Flow-Tabs lassen sich durch **Drag & Drop** innerhalb der Tab-Leiste neu sortieren
- Beim Draggen wird der Tab visuell angehoben und ein Drop-Indikator zeigt die Zielposition
- Nach dem Drop wird die Reihenfolge in `flowStore.flows` aktualisiert
- Die neue Reihenfolge wird beim nächsten Deploy in `workspace.json` persistiert
- Die Sortieränderung markiert den Workspace als dirty

## UI-Konzept

```
┌─────────────────────────────────────────────────────────────────────┐
│ HeaderBar                                                           │
├────────────┬────────────┬────────────┬─────┬────────────────────────┤
│ ▪ Flow 1   │  Flow 2    │  Flow 3    │  +  │                        │  ← Flow-Tab-Bar
├────────────┴────────────┴────────────┴─────┴────────────────────────┤
│                                                                     │
│                         FlowEditor Canvas                           │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

**Tab-Zustände:**
- **Aktiv**: Hervorgehobener Hintergrund, stärkere Schrift
- **Inaktiv**: Normaler Hintergrund
- **Deaktiviert**: Ausgegraut / reduzierte Opacity
- **Dirty**: Blauer Punkt (●) wie bei Nodes
- **Dragging**: Leichte Elevation / Schatten

**Properties Sidebar – Neuer Flow:**
```
┌──────────────────────┐
│  Neuer Flow          │
├──────────────────────┤
│  Name                │
│  ┌────────────────┐  │
│  │                │  │
│  └────────────────┘  │
│                      │
│  [Anlegen] [Abbrech] │
└──────────────────────┘
```

**Properties Sidebar – Flow bearbeiten:**
```
┌──────────────────────┐
│  Flow-Einstellungen  │
├──────────────────────┤
│  Name                │
│  ┌────────────────┐  │
│  │ Flow 1         │  │
│  └────────────────┘  │
│                      │
│  Status              │
│  [●] Aktiviert       │
│                      │
│  ──────────────────  │
│  [Flow löschen]      │
└──────────────────────┘
```

## Datenstruktur

### workspace.json (bestehend, keine Änderung)

```json
[
  {
    "id": "flow-uuid-1",
    "type": "tab",
    "label": "Flow 1",
    "disabled": false,
    "nodes": [...],
    "wires": [...]
  },
  {
    "id": "flow-uuid-2",
    "type": "tab",
    "label": "Flow 2",
    "disabled": true,
    "nodes": [],
    "wires": []
  }
]
```

Die Flow-Reihenfolge entspricht der Reihenfolge im Array — Drag & Drop ändert die Array-Position.

## Betroffene Dateien

### Neue Dateien

- `frontend/src/components/FlowTabBar.vue` — Horizontale Tab-Leiste mit Drag & Drop
- `frontend/src/components/FlowProperties.vue` — Flow-Formular für Properties Sidebar (Erstellen/Bearbeiten)

### Bestehende Dateien (Anpassungen)

- `frontend/src/App.vue` — FlowTabBar zwischen HeaderBar und FlowEditor einbinden
- `frontend/src/stores/flowStore.ts` — Neue Actions: `reorderFlows(fromIndex, toIndex)`, `updateFlowLabel(flowId, label)`, `toggleFlowDisabled(flowId)`
- `frontend/src/components/PropertyPanel.vue` — Flow-Properties rendern wenn kein Node sondern ein Flow ausgewählt ist
- `frontend/src/stores/uiStore.ts` — Neuer State: `selectedFlowForProperties: string | null` um zwischen Node-Properties und Flow-Properties zu unterscheiden
- `frontend/src/views/FlowEditor.vue` — Flow-Name aus HeaderBar entfernen (wird jetzt im Tab angezeigt), initiales Laden aller Flows beibehalten

### Backend

- Keine Backend-Änderungen nötig — die bestehende API (`GET/POST /api/v1/flows`) und die Engine unterstützen bereits mehrere Flows mit `disabled`-Feld

## Technische Hinweise

### Drag & Drop Sortierung

Für die Tab-Sortierung empfiehlt sich nativer HTML5 Drag & Drop oder eine leichtgewichtige Library wie `vuedraggable`. Die Implementation sollte:
- `dragstart`, `dragover`, `drop` Events auf den Tabs handhaben
- Während des Drags einen visuellen Indikator an der Drop-Position zeigen
- Nach dem Drop `flowStore.reorderFlows(fromIndex, toIndex)` aufrufen

### Properties Sidebar Kontext

Die Properties Sidebar muss zwischen zwei Modi unterscheiden:
1. **Node-Properties** (bestehend): Wenn ein Node im Canvas ausgewählt ist
2. **Flow-Properties** (neu): Wenn ein Flow-Tab doppelgeklickt oder der Plus-Tab geklickt wird

Prioritätsregel: Flow-Properties überschreiben Node-Properties temporär. Beim Schließen oder Anlegen/Abbrechen kehrt die Sidebar zum vorherigen Node-Kontext zurück.

### Flow-Name Validierung

- Der Flow-Name darf nicht leer sein
- Duplikate sind erlaubt (Flows werden über ihre ID identifiziert, nicht über den Namen)
- Maximale Länge: 50 Zeichen (für die Tab-Darstellung)

## Abhängigkeiten

- Keine blockierenden Abhängigkeiten — kann unabhängig von anderen Issues implementiert werden
- Die bestehende `flowStore.addFlow()` und `flowStore.removeFlow()` Methoden bilden die Grundlage
- Das `disabled`-Feld existiert bereits im Flow-Typ (`types.go`) und in `workspace.json`
