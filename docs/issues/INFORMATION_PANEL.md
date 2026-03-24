# Information Panel: Tab-System mit Help, Config und Debug

## Kontext

Das Information Panel (rechte Sidebar, `InformationSidebar.vue`) zeigt aktuell nur das Debug Panel als einzigen Inhalt. Es fehlt eine Tab-Navigation und weitere wichtige Panels fuer den taeglichen Workflow.

## Anforderung

Das Information Panel wird um ein **Tab-System** erweitert mit drei Tabs:

### Tab 1: Help

- Zeigt die **Hilfe/Dokumentation** fuer den aktuell selektierten Node-Typ
- Reagiert auf Node-Klick im Editor (`flowStore.selectedNode`)
- Wenn kein Node selektiert ist: Platzhalter-Text ("Klicke einen Node um die Hilfe anzuzeigen")
- Inhalt pro Node-Typ:
  - Beschreibung / Zweck des Nodes
  - Erklaerung der Input/Output Ports
  - Beschreibung der konfigurierbaren Properties
  - Beispiele / Hinweise zur Verwendung
- Die Hilfetexte koennen initial aus der `NodeTypeInfo` (Backend: `Description` Feld) kommen und spaeter um ausfuehrlichere Markdown-Dokumentation erweitert werden

### Tab 2: Config

- Listet alle **Config Nodes** des Workspace auf (z.B. MQTT Broker Verbindungen, DB Connections)
- Datenquelle: `flowStore.configs` (Array von `ConfigNode`)
- Jeder Eintrag zeigt: Name, Typ, Status (verbunden/getrennt falls verfuegbar)
- Klick auf einen Config-Eintrag oeffnet den Config-Editor im Property Panel (`ui.openConfigEditor(type, id)`)
- Button zum Anlegen neuer Config Nodes

### Tab 3: Debug (bestehend)

- Bereits implementiert als `DebugPanel.vue`
- Wird 1:1 in den Tab uebernommen
- Bestehendes Verhalten bleibt unveraendert (Filter, ON/OFF, CLR, Auto-Scroll, Message-Anzeige)

## Technische Details

### Betroffene Dateien

| Datei | Aenderung |
|-------|-----------|
| `frontend/src/components/InformationSidebar.vue` | Tab-Navigation hinzufuegen, Tabs rendern |
| `frontend/src/components/HelpPanel.vue` | **Neu** — Hilfe-Anzeige fuer selektierten Node |
| `frontend/src/components/ConfigPanel.vue` | **Neu** — Config Node Liste |
| `frontend/src/components/DebugPanel.vue` | Keine Aenderung, wird als Tab eingebunden |
| `frontend/src/stores/uiStore.ts` | `activeInfoTab` State hinzufuegen |

### State

```typescript
// uiStore
const activeInfoTab = ref<'help' | 'config' | 'debug'>('debug')
```

### Vorhandene Infrastruktur

- **Node Selection**: `flowStore.selectedNode` — reaktiv, bereits implementiert
- **Config Nodes**: `flowStore.configs` — bereits geladen und verfuegbar
- **Config Editor**: `ui.openConfigEditor(type, id)` — bereits implementiert
- **Node Type Info**: Kommt vom Backend via Registry, enthaelt `Description`, `Category`, `Inputs`, `Outputs`

## UI Mockup

```
┌─ Information ──────────────────┐
│  [Help]  [Config]  [Debug]     │  ← Tab-Leiste
├────────────────────────────────┤
│                                │
│  Tab-Inhalt                    │
│                                │
│                                │
└────────────────────────────────┘
```

## Abgrenzung

- Die ausfuehrliche Markdown-Hilfe pro Node-Typ ist NICHT Teil dieses Issues — initial reicht die `Description` aus der `NodeTypeInfo`
- Config Node Status-Anzeige (verbunden/getrennt) ist optional und kann spaeter ergaenzt werden
