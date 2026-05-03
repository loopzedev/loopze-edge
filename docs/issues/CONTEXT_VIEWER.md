# Context Viewer: Werte anzeigen und manuell löschen

## Kontext

LOOPZE speichert Context-Daten in vier NATS JetStream KV-Buckets (siehe `internal/nats/context_store.go` und `internal/nats/broker.go`):

- **Global Memory** — `context-global-memory` (volatil)
- **Global Persistent** — `context-global-persistent` (datei-backed)
- **Flow Memory** — `context-flow-{flowID}-memory` (volatil, pro Flow)
- **Flow Persistent** — `context-flow-{flowID}-persistent` (datei-backed, pro Flow)

Function Nodes lesen/schreiben über `node.*`, `global.*` und `flow.*` (`internal/nodes/function.go`). Der Context-Watch Node beobachtet Änderungen.

**Problem:** Aktuell gibt es weder eine REST/WS-Schnittstelle noch eine UI, um die gespeicherten Context-Werte einzusehen oder gezielt zu löschen. Beim Debuggen von Flows ist man auf Watch-Nodes und Debug-Output angewiesen, was umständlich ist.

## Anforderung

Ein neuer Tab **"Context"** im Information Panel (rechte Sidebar), der die Context-Stores anzeigt und das gezielte Löschen einzelner Keys (sowie aller Keys eines Stores) erlaubt.

### Funktionalität

- **Browse**: Alle vier Store-Varianten anzeigen (Global Memory, Global Persistent, Flow Memory, Flow Persistent)
- **Flow-Auswahl**: Bei Flow-Stores wird der aktuell aktive Flow (oder eine Auswahl aus `flowStore.flows`) als Scope verwendet
- **Key-Liste**: Pro Store alle Keys mit aktuellem Wert (JSON-formatiert) anzeigen
- **Delete Single**: Einzelnen Key per Klick löschen (mit Bestätigung)
- **Delete All**: Alle Keys eines Stores löschen (mit Bestätigung — destruktiv)
- **Auto-Refresh-Toggle**: User kann das Sekundentakt-Polling an/aus schalten (Default: **aus** — User aktiviert bewusst, Zustand persistiert in `uiStore`)
- **Manuelles Refresh (gesamt)**: Button lädt alle Keys+Werte des aktuell gewählten Stores neu — funktioniert immer, unabhängig vom Auto-Refresh
- **Manuelles Refresh (einzeln)**: Pro Key-Zeile ein Refresh-Icon, das nur diesen einen Key neu lädt

### Update-Strategie

**Frontend-Polling statt WebSocket-Push** — im Sekundentakt (`setInterval(load, 1000)`), nur aktiv wenn alle Bedingungen erfüllt sind:

- Auto-Refresh-Toggle ist aktiv (`ui.contextAutoRefresh === true`)
- der Context-Tab im Information Panel offen ist (`ui.activeInfoTab === 'context'`)
- das Information Panel nicht geschlossen ist (`ui.infoPanelOpen`)

Ist Auto-Refresh deaktiviert, hat der User nur die manuellen Refresh-Buttons (gesamter Store / einzelner Key) zur Aktualisierung.

**Begründung:**

- Ein WebSocket-Watch (Backend pushed jede KV-Änderung) kann bei "heißen" Counter-Keys leicht hunderte Updates/Sekunde erzeugen — das wollen wir nicht durch den WS leiten
- Polling ist trivial, hat keinen Backend-State, stoppt automatisch beim Tab-Wechsel oder Schließen des Panels
- 1× HTTP GET/Sekunde pro offenem Panel ist vernachlässigbar; bei n offenen Browser-Tabs maximal n Requests/Sekunde
- Beim Verlassen des Tabs / Schließen des Panels wird das Intervall geclearht

**Optional (später):**

- Werte direkt im UI editieren (set)
- Filter / Suche über Keys
- Wenn Polling-Last doch zum Problem wird: WS-Push mit serverseitigem Throttling (max 1 Frame/Sekunde, aggregiert)

## Backend

### Neue REST Endpoints

Mount unter `/api/v1/context` (in `internal/api/routes.go`):

| Methode | Pfad | Zweck |
|---------|------|-------|
| `GET` | `/context/global/{storage}` | Alle Keys+Werte eines globalen Stores (`storage` ∈ `memory`, `persistent`) |
| `GET` | `/context/global/{storage}/{key}` | Einzelnen Key im globalen Store laden (für Single-Key-Refresh) |
| `GET` | `/context/flow/{flowID}/{storage}` | Alle Keys+Werte eines Flow-Stores |
| `GET` | `/context/flow/{flowID}/{storage}/{key}` | Einzelnen Key im Flow-Store laden |
| `DELETE` | `/context/global/{storage}/{key}` | Einzelnen Key im globalen Store löschen |
| `DELETE` | `/context/flow/{flowID}/{storage}/{key}` | Einzelnen Key im Flow-Store löschen |
| `DELETE` | `/context/global/{storage}` | Alle Keys im globalen Store löschen |
| `DELETE` | `/context/flow/{flowID}/{storage}` | Alle Keys im Flow-Store löschen |

**Response-Format GET:**

```json
{
  "scope": "global",
  "storage": "memory",
  "entries": [
    { "key": "counter", "value": 42 },
    { "key": "lastRun", "value": "2026-04-25T08:30:00Z" }
  ]
}
```

### Implementierung

- Neuer Handler in `internal/api/handlers.go` (z.B. `handleGetContext`, `handleDeleteContextKey`, `handleClearContext`)
- Zugriff auf die KV-Stores über den existierenden `ContextProvider` aus `internal/flow/context.go`
- `KVContextStore` hat bereits `Keys()`, `Get(key)`, `Delete(key)` — keine Backend-Änderung an der Storage-Schicht nötig
- "Delete All" iteriert über `Keys()` und ruft pro Key `Delete()` auf (alternativ KV-Bucket purgen, falls JetStream das einfach hergibt)
- Fehlerbehandlung: Unbekannter Flow → 404, unbekannter Storage-Name → 400

## Frontend

### Betroffene Dateien

| Datei | Änderung |
|-------|----------|
| `frontend/src/components/InformationSidebar.vue` | Neuen Tab "Context" zur Tab-Liste hinzufügen |
| `frontend/src/stores/uiStore.ts` | `InfoTab` um `'context'` erweitern |
| `frontend/src/components/ContextPanel.vue` | **Neu** — Tab-Inhalt |
| `frontend/src/stores/contextStore.ts` | **Neu** — Pinia-Store für Context-Daten |

### UI-Struktur

```
┌─ Information ────────────────────────────────┐
│  [Help] [Config] [Context] [Debug]           │
├──────────────────────────────────────────────┤
│ [Global Mem][Global Pers][Flow Mem][Flow Pers]│
│ Flow: [Aktueller Flow ▼]   (nur bei Flow-*)  │
│ [↻ Refresh]    Auto-Refresh 1s: [ ☐ ]        │
├──────────────────────────────────────────────┤
│ ▸ counter      42              [↻] [✕]      │
│ ▸ lastRun      "2026-04-25..." [↻] [✕]      │
│ ▸ user         {…}             [↻] [✕]      │
├──────────────────────────────────────────────┤
│                           [Clear All]        │
└──────────────────────────────────────────────┘
```

- **4-Knopf-Toggle** für Store-Auswahl (Global Mem / Global Pers / Flow Mem / Flow Pers) — ein einziger State, weniger Klicks als zwei Dropdowns
- Flow-Dropdown erscheint nur bei Flow-Scope
- Header-Refresh-Button (`↻ Refresh`) lädt den ganzen Store neu
- Pro Zeile ein kleines Refresh-Icon (`↻`) das nur diesen Key neu lädt — nützlich wenn Auto-Refresh aus ist
- Auto-Refresh-Checkbox toggelt das Sekundentakt-Polling (**Default: aus**), Zustand wird in `uiStore.contextAutoRefresh` persistiert

- Werte werden als kollabierte Zeile gerendert (kurze Vorschau), per Klick als JSON-Tree (vorhandene `JsonTreeView.vue` wiederverwenden)
- Delete-Button mit Bestätigungs-Dialog (existierende UI-Konvention beachten)
- "Clear All" rot/destruktiv markiert, mit zusätzlicher Bestätigung

### Pinia Store (Skizze)

```typescript
// contextStore.ts
const entries = ref<ContextEntry[]>([])
const scope = ref<'global' | 'flow'>('global')
const storage = ref<'memory' | 'persistent'>('memory')
const flowId = ref<string | null>(null)

async function loadAll() { /* GET /context/.../{storage} */ }
async function loadKey(key: string) { /* GET /context/.../{storage}/{key} → entries[key] aktualisieren */ }
async function deleteKey(key: string) { /* DELETE … */ }
async function clearAll() { /* DELETE … */ }

// in ContextPanel.vue: Polling nur wenn Auto-Refresh + Tab + Panel offen
let timer: ReturnType<typeof setInterval> | null = null
watchEffect(() => {
  const active =
    ui.contextAutoRefresh &&
    ui.activeInfoTab === 'context' &&
    ui.infoPanelOpen
  if (active && !timer) timer = setInterval(loadAll, 1000)
  if (!active && timer) { clearInterval(timer); timer = null }
})
```

**uiStore-Erweiterung:**

```typescript
const activeInfoTab = ref<InfoTab>('debug')   // erweitert um 'context'
const contextAutoRefresh = ref<boolean>(true) // Toggle, persistiert (localStorage)
```

## Acceptance Criteria

- [ ] Backend liefert für alle vier Store-Varianten Keys + Werte über `GET /api/v1/context/...`
- [ ] Backend löscht einzelne Keys und ganze Stores über `DELETE /api/v1/context/...`
- [ ] Neuer Tab "Context" im Information Panel sichtbar
- [ ] Scope/Storage/Flow im Panel umschaltbar
- [ ] Keys werden mit JSON-Wert angezeigt, einzelne Keys per Button löschbar
- [ ] "Clear All" mit Bestätigungs-Dialog funktioniert
- [ ] Auto-Refresh-Toggle aktualisiert die Werte im Sekundentakt, solange Context-Tab+Panel offen sind
- [ ] Auto-Refresh kann jederzeit deaktiviert werden, danach läuft kein Polling mehr
- [ ] Manueller Refresh-Button lädt den gesamten Store neu (auch wenn Auto-Refresh aus ist)
- [ ] Pro Key-Zeile lädt der Single-Key-Refresh nur diesen einen Key neu
- [ ] Polling stoppt beim Tab-Wechsel oder Schließen des Information Panels

## Abgrenzung

- **Kein Live-Update** — initial reicht manuelles Refresh
- **Kein Editieren** der Werte (nur lesen + löschen)
- **Kein `node.*`-Scope** — der ist im Function Node nur In-Memory pro Node und nicht über die KV-Stores erreichbar
- **Keine Filter/Suche** — kommt bei Bedarf später
