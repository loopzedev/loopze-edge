# Issue: Debug Node – ON/OFF State Persistierung & Backend-Logik

## Status: Open

## Problembeschreibung

Der Debug Node besitzt ausgangsseitig einen rastenden Toggle-Taster (ON/OFF). Aktuell ist dieser Zustand nur lokal im Vue-Component (`enabled = ref(true)`) gespeichert und geht beim Neuladen des Browsers verloren. Der Zustand muss in `workspace.json` über das `config.active`-Feld des Nodes persistiert werden.

## Anforderungen

### 1. Persistierung in workspace.json

- Der ON/OFF-Zustand wird im Node-Config-Feld `active` (boolean) abgebildet
- Beispiel workspace.json Struktur:
  ```json
  {
    "id": "node-uuid",
    "type": "debug",
    "config": {
      "property": "payload",
      "active": true
    }
  }
  ```
- Default-Wert bei fehlendem Feld: `true` (ON)

### 2. Toggle → Dirty → Deploy Zyklus

- Wird der Toggle-Taster betätigt, ändert sich `config.active` im Frontend-State
- Die Änderung markiert den Node als **dirty** (`flowStore.markNodeDirty(nodeId)`)
- Der blaue Dirty-Indicator wird am Node angezeigt
- Erst mit **Deploy** wird der geänderte Zustand in `workspace.json` geschrieben
- Nach erfolgreichem Deploy wird der Dirty-State zurückgesetzt

### 3. Initialer Zustand beim Laden

- Beim Öffnen von FLINT im Browser wird `config.active` aus den geladenen Flow-Daten gelesen
- Der Toggle-Taster zeigt den gespeicherten Zustand korrekt an (ON/OFF)
- Wurde noch nie deployed, gilt der Default `active: true`

### 4. Frontend-Filterung mit Dirty-State

- Die Filterung passiert **beim Eingang** neuer Nachrichten im Frontend (`addMessage`), nicht retroaktiv auf der bestehenden Liste
- Nachrichten die bereits in der Debug-Liste stehen **bleiben bei Filteränderung erhalten**
- Wird ein Debug Node deaktiviert, werden nur **neue** eingehende Nachrichten dieses Nodes verworfen
- Wird ein Debug Node wieder aktiviert, erscheinen ab sofort wieder neue Nachrichten in der Liste
- Dabei greift der **aktuelle Toggle-State des Frontends**, auch wenn dieser dirty (noch nicht deployed) ist
- Der Bediener kann Debug Nodes sofort aktivieren/deaktivieren ohne vorher deployen zu müssen
- Erst das Persistieren des Zustands erfordert ein Deploy – die Eingangs-Filterung wirkt sofort

### 5. Backend-Logik: Debug Stream

- Der Debug Node streamt **immer** in den DEBUG Stream – unabhängig davon ob `active` true oder false ist
- Das Backend ignoriert den `active`-State bei der Nachrichtenverarbeitung: jede eingehende Message wird über `DebugFunc` in den NATS-Subject `debug.{flowId}.{nodeId}` publiziert
- **Die Filterung (anzeigen/unterdrücken) findet ausschließlich im Frontend statt**, nicht im Backend

### 6. Architektur-Entscheidung: Immer streamen vs. Subscription anpassen

**Zwei Optionen wurden abgewogen:**

| | Option A: Immer streamen, Frontend filtert | Option B: Frontend passt Subscriptions an |
|---|---|---|
| **Prinzip** | Backend publiziert alle Debug-Messages, Frontend blendet deaktivierte Nodes aus | Frontend subscribed/unsubscribed pro Debug Node bei Toggle-Änderung |
| **Latenz beim Toggle** | Sofort – reine UI-Filterung | Verzögerung durch Subscribe/Unsubscribe Roundtrip |
| **Dirty-State Kompatibilität** | Trivial – Frontend kennt den lokalen State und filtert direkt | Komplex – Subscription-Änderung ohne Deploy erfordert separaten Signalweg zum WebSocket-Layer |
| **Nachrichtenverlust** | Keiner – Stream läuft durchgehend | Möglich – Messages während Unsubscribe/Subscribe-Transition gehen verloren |
| **Traffic** | Etwas mehr – auch deaktivierte Nodes senden | Weniger – nur aktive Nodes senden |
| **Komplexität** | Gering – keine Subscription-Verwaltung | Hoch – Subscription-State muss synchron zu Toggle-State gehalten werden |

**Entscheidung: Option A – Immer streamen, Frontend filtert**

Begründung:
- Debug-Nachrichten sind klein und typischerweise low-volume
- Der Toggle muss **sofort** wirken, auch im Dirty-State ohne Deploy – das schließt Subscription-Management praktisch aus
- Keine Race Conditions oder Nachrichtenverlust bei schnellem Hin- und Herschalten
- Deutlich weniger Komplexität im gesamten Stack

### 7. Debug-Ausgabe konfigurieren

Der Bediener kann im Properties-Panel konfigurieren, **welche Informationen** aus der Nachricht in den Debug geschrieben werden und **welcher Inhalt als Node-Status** angezeigt wird.

#### Debug-Ausgabe (`output`)

Dropdown-Feld **"Ausgabe"** mit folgenden Optionen:

| Wert | Label | Beschreibung |
|---|---|---|
| `property` | `msg.` (+ Eingabefeld) | Gibt ein einzelnes Property der Message aus (Default: `payload`) |
| `message` | Kompletten Nachrichten-Objekt | Gibt die gesamte `msg` als JSON aus |
| `gjson` | GJSON | Wertet einen GJSON-Pfadausdruck auf der Message aus |

- Bei `property`: zusätzliches Textfeld für den Property-Pfad (z.B. `payload`, `topic`, `payload.temperature`)
- Bei `gjson`: zusätzliches Textfeld für den GJSON-Ausdruck (z.B. `payload.items.#`, `payload.items.0.name`)
- Default: `property` mit Wert `payload`

#### Node-Status (`statusOutput`)

Checkbox **"Node-Status"** (max. 32 Zeichen) mit zugehörigem Dropdown:

| Wert | Label | Beschreibung |
|---|---|---|
| `same` | Identisch mit Debug-Ausgabe | Zeigt denselben Inhalt wie die Debug-Ausgabe als Status an |
| `property` | `msg.` (+ Eingabefeld) | Zeigt ein spezifisches Property als Status an |
| `gjson` | GJSON | Wertet einen GJSON-Pfadausdruck aus und zeigt das Ergebnis als Status |
| `count` | message count | Zeigt die Anzahl empfangener Nachrichten als Status |

- Node-Status ist optional (Checkbox aktiviert/deaktiviert die Anzeige)
- Der Status-Text wird auf **max. 32 Zeichen** gekürzt
- Default: deaktiviert

#### Config-Struktur in workspace.json

```json
{
  "id": "node-uuid",
  "type": "debug",
  "config": {
    "active": true,
    "output": "property",
    "property": "payload",
    "statusEnabled": false,
    "statusOutput": "same",
    "statusProperty": ""
  }
}
```

## Betroffene Dateien

### Frontend
- `frontend/src/components/nodes/DebugNode.vue` – Toggle-State aus Config lesen, bei Toggle Config ändern + dirty markieren
- `frontend/src/stores/flowStore.ts` – Node-Config-Update und Dirty-Tracking
- `frontend/src/stores/debugStore.ts` – Filterung der Debug-Messages basierend auf aktuellem Toggle-State (inkl. dirty)

### Backend
- `internal/nodes/debug.go` – `active`-Feld aus Config lesen, aber Nachrichten **immer** streamen
- `internal/flow/registry.go` – `DebugMessage`-Struct (kein `active`-Feld nötig, da Backend immer streamt)

---

## Erweiterung: Interaktiver JSON-Tree-View in der Debug-Sidebar

### Status: Open

### Ausgangslage

Aktuell wird jede Debug-Payload in `frontend/src/components/DebugPanel.vue` (Z. 59-69, 182-189) per `JSON.stringify(payload, null, 2)` in einem `<pre>`-Block als reiner, statischer Text gerendert. Bei verschachtelten Objekten und Arrays bedeutet das:

- Keine Möglichkeit, uninteressante Teilbäume einzuklappen
- Pfade müssen visuell aus der Einrückung abgeleitet werden
- Werte oder Pfade müssen manuell selektiert und kopiert werden – fehleranfällig bei langen Strings

### Ziel

Die Payload wird als interaktiver JSON-Tree dargestellt, ähnlich wie in Browser-DevTools oder Node-RED. Der Bediener kann Teilbäume aufklappen, Pfade und Werte mit einem Klick kopieren und so deutlich schneller mit Debug-Nachrichten arbeiten.

### Anforderungen

#### 1. Collapsible Tree

- Objekte und Arrays werden mit einem Twisty-Icon (`▶` / `▼`) als zuklappbare Knoten dargestellt
- Primitive Werte (string, number, boolean, null, undefined) werden inline ohne Toggle gerendert
- Initialer Expand-State: **Top-Level expanded, Kindknoten collapsed** (Tiefe 1 sichtbar)
- Bei eingeklapptem Knoten wird eine Vorschau angezeigt:
  - Objekt: `{ … } (5 keys)`
  - Array: `[ … ] (12 items)`
- Der Expand-State wird **pro Message lokal** gehalten und überlebt Re-Renders, aber nicht das Schließen des Panels (in-memory, nicht persistiert)

#### 2. Syntax-Coloring

Werte werden nach Typ farbig dargestellt, passend zum bestehenden Terminal-Theme:

| Typ | Stil |
|---|---|
| `string` | grün/türkis, in Anführungszeichen |
| `number` | orange/gelb |
| `boolean` | violett |
| `null` / `undefined` | dimmed grau, kursiv |
| Object/Array Header | terminal-text in normaler Farbe |
| Keys | terminal-text-dim |

Bestehende Status-Farben (`error`, `warn`) bleiben für die gesamte Message-Card erhalten – die Tree-Farben werden nur im Default-Status (`debug`) verwendet, bei Fehler/Warn wird der Tree in der jeweiligen Status-Farbe gerendert (so wie heute).

#### 3. Copy-Aktionen pro Knoten

Beim Hover über eine Tree-Zeile erscheinen rechts zwei kleine Icon-Buttons:

- **Copy Path** – kopiert den Property-Pfad relativ zur Wurzel der Message-Payload
  - Notation: JavaScript-Property-Style mit `[idx]` für Arrays, z.B. `payload.items[0].name` oder `temperature`
  - Wenn die Payload selbst nicht das gesamte `msg`-Objekt ist (also `msg.property !== 'payload'` bzw. konfigurierter Property-Pfad/GJSON), beginnt der Pfad **relativ zur angezeigten Payload** – die Sidebar zeigt was sie zeigt, der kopierte Pfad gilt im selben Bezugssystem
- **Copy Value** – kopiert den Wert des Knotens
  - Primitive: roher Wert (String ohne Anführungszeichen, Zahl als String, etc.)
  - Objekte/Arrays: kompaktes JSON ohne Einrückung (`JSON.stringify(value)`)

Visuelles Feedback nach dem Klick:
- Icon wechselt für ~1 Sekunde auf ein Häkchen
- Kein Toast, keine Modal – konsistent mit dem schlanken Terminal-Stil

#### 4. Verhalten bei Nicht-JSON Payloads

- `string`, `number`, `boolean` werden weiterhin inline ohne Tree dargestellt, aber mit **Copy Value**-Button am Zeilenrand (Hover)
- `null` / `undefined`: nur Anzeige, kein Copy nötig
- `format: 'buffer'`: out of scope – wird wie heute als String dargestellt
- Wenn `JSON.stringify` fehlschlägt (z.B. zirkuläre Referenz), Fallback auf die heutige `String(payload)`-Darstellung ohne Tree

#### 5. Performance

- Pro Message läuft die rekursive Komponente nur über die **expandierten** Pfade – Kinder collapsed Knoten werden nicht ins DOM gerendert
- Bei initialem Render der Liste sind also nur die Top-Level-Properties jeder Message materialisiert
- Lange String-Werte (> 200 Zeichen) werden mit `…` truncated, ein **Expand**-Klick zeigt den vollen String (innerhalb derselben Message)

#### 6. Toolbar-Erweiterung pro Message (Phase 2, optional)

In der Header-Zeile jeder Message (rechts neben dem Format-Badge in `DebugPanel.vue:176-178`) zwei zusätzliche Icon-Buttons beim Hover:

- **Expand All** – klappt alle Knoten dieser Message auf
- **Collapse All** – klappt alle bis auf Top-Level zu

Phase-1-Scope: nur per-Knoten Toggle. Phase 2 nur umsetzen, wenn sich im Realeinsatz Bedarf zeigt.

### Implementierungsplan

#### Neue Komponente: `JsonTreeView.vue`

Pfad: `frontend/src/components/debug/JsonTreeView.vue` (neuer Unterordner für Debug-spezifische UI-Bausteine)

Props:
```ts
interface Props {
  data: unknown
  path?: string                // aktueller Pfad (default '')
  rootKey?: string             // Anzeigename des Root-Knotens (z.B. 'payload' oder Property-Name)
  depth?: number               // aktuelle Rekursionstiefe (default 0)
  defaultExpandDepth?: number  // bis zu welcher Tiefe initial expanded (default 1)
}
```

Verhalten:
- Rekursive Selbstreferenz für Kinder
- Lokaler `expanded`-State per Knoten (`ref<boolean>`), initialisiert via `depth < defaultExpandDepth`
- Pfad-Aufbau:
  - Object-Key: `path === '' ? key : ${path}.${key}`
  - Array-Index: `${path}[${idx}]`
  - Keys mit Sonderzeichen (Punkt, Klammer, Whitespace): Bracket-Notation `${path}["weird.key"]`
- Copy-Funktion via `navigator.clipboard.writeText()` mit kurzem `copied`-Flag pro Button für das Icon-Feedback

#### Integration in `DebugPanel.vue`

`<pre>`-Block (Z. 182-189) ersetzen durch:

```vue
<JsonTreeView
  :data="msg.payload"
  :root-key="msg.property || 'payload'"
  :class="{
    'text-red-300': msg.status === 'error',
    'text-yellow-200': msg.status === 'warn',
  }"
/>
```

Die `formatPayload`-Funktion (Z. 59-69) entfällt für Objekte/Arrays. Für primitive Top-Level-Werte rendert die `JsonTreeView`-Komponente intern direkt die Inline-Darstellung mit Copy-Button.

#### Keine neue Dependency

Da der Stack auf TailwindCSS + Radix Vue + bewusst minimalem Footprint setzt und das Styling stark Terminal-themed ist, wird **bewusst keine externe JSON-Viewer-Lib** (vue-json-pretty etc.) eingeführt. Eine eigene rekursive Komponente in ~150–200 LOC fügt sich nahtlos ins bestehende Theme.

### Betroffene Dateien

#### Neu
- `frontend/src/components/debug/JsonTreeView.vue` – rekursive Tree-Komponente mit Collapse + Copy

#### Geändert
- `frontend/src/components/DebugPanel.vue` – `<pre>`-Block durch `<JsonTreeView>` ersetzen, `formatPayload` ggf. entfernen wenn nicht mehr benötigt

#### Optional
- `frontend/src/utils/clipboard.ts` (falls noch keine zentrale Copy-Hilfe existiert) – schmaler Wrapper um `navigator.clipboard.writeText` mit Fallback

### Out of Scope

- Persistieren des Expand-States über Session-Grenzen hinweg
- Such-/Filter-Funktion innerhalb eines einzelnen JSON-Trees
- Diff-View zwischen aufeinanderfolgenden Debug-Messages desselben Nodes
- Edit-Mode für Debug-Werte (das ist ein Inspector, kein Editor)

---

## Erweiterung: Hover-Highlight im Flow

### Status: Open

### Ausgangslage

Bei vielen Debug-Nachrichten in der Sidebar ist es schwer zu erkennen, von welchem Node im Flow eine bestimmte Message stammt. Der Node-Name in der Header-Zeile ist zwar sichtbar, aber bei mehreren gleichnamigen oder ähnlich benannten Nodes muss der Bediener im Flow suchen.

### Ziel

Beim Hover über eine Debug-Message-Zeile in der Sidebar wird der ursprungs-Node im Flow-Editor durch einen **gestrichelten Rahmen** (dashed border) hervorgehoben. Beim Verlassen der Zeile verschwindet der Indikator wieder. Damit lässt sich auf einen Blick zuordnen, welcher Node welche Nachricht produziert hat.

### Anforderungen

#### 1. Hover-Verhalten

- Mouse-Enter auf einer Debug-Row → Node mit passender `nodeId` bekommt dashed Border
- Mouse-Leave → Highlight wird zurückgenommen
- Schneller Wechsel zwischen Rows: das Highlight wandert sofort mit, ohne Flackern oder Nachglühen
- Kein Klick nötig, rein hover-basiert (Klick bleibt für Selection reserviert)

#### 2. Visuelles Erscheinungsbild

- **Border-Style**: `dashed`
- **Border-Color**: `accent` (`#58a6ff` aus dem Theme) — klar unterscheidbar von der grauen Default-Border
- **Border-Width**: 1px — identisch zur normalen Border, damit es keine Layout-Verschiebung der Nodes gibt
- **Kein** Box-Shadow / Glow — bewusst dezenter als der `selected`-State, damit die beiden Zustände visuell unterscheidbar bleiben
- Wenn der Node bereits `selected` ist: `selected`-Styling hat Vorrang (Glow + solid border bleibt), keine Überlagerung

#### 3. Edge-Cases

- Hovered Node existiert nicht im aktuell sichtbaren Flow (Multi-Flow / gewechselter Tab): Highlight greift einfach nicht — kein Fehler
- Debug-Message ohne `nodeId` (sollte nach dem ID-Fix nicht mehr vorkommen): kein Highlight
- Sidebar wird geschlossen während Hover aktiv: Highlight muss zurückgesetzt werden (über `onBeforeUnmount` der Sidebar oder per Mouse-Leave-Event)

### Implementierungsplan

#### State

In `frontend/src/stores/flowStore.ts` neu:

```ts
const hoveredDebugNodeId = ref<string | null>(null)

function setHoveredDebugNodeId(id: string | null) {
  hoveredDebugNodeId.value = id
}
```

Beide im Store-Return exportieren. Bewusst im `flowStore` (nicht `uiStore`), weil es semantisch ein Node-State ist und `BaseNode.vue` ohnehin auf den `flowStore` zugreift.

#### Sidebar (`DebugPanel.vue`)

Pro Message-Row:

```vue
<div
  v-for="msg in messages"
  :key="msg.id"
  v-memo="[msg.id]"
  @mouseenter="flowStore.setHoveredDebugNodeId(msg.nodeId)"
  @mouseleave="flowStore.setHoveredDebugNodeId(null)"
  ...
>
```

Hinweis: Listener werden bei Mount registriert und sind statisch — kompatibel mit `v-memo`.

Zusätzlich `onBeforeUnmount` im `<script>`-Block: `flowStore.setHoveredDebugNodeId(null)` aufrufen, falls die Sidebar geschlossen wird während ein Hover aktiv ist.

#### Node (`BaseNode.vue`)

Computed:

```ts
const isDebugHovered = computed(
  () => flowStore.hoveredDebugNodeId === props.id,
)
```

Class-Binding ergänzen:

```vue
:class="{ selected: props.selected, 'debug-hovered': isDebugHovered, ... }"
```

CSS-Regel im `<style>`-Block (nach `.selected`, damit `.selected`-Vorrang hat oder via spezifischem Selector):

```css
.flint-node.debug-hovered:not(.selected) {
  border-style: dashed;
  border-color: var(--color-accent, #58a6ff);
}
```

### Out of Scope für Phase 1

- **LinkNode-Highlight**: `LinkNode.vue` hat eigenes Wrapper-Styling ohne `.flint-node`-Klasse — kann nachgezogen werden, ist aber für die Diagnose-UX nicht kritisch (Link-Nodes erzeugen typischerweise keine Debug-Messages)
- **Auto-Pan/Scroll** im Flow zum hovered Node, wenn er außerhalb des Viewports liegt
- **Bidirektionalität** (Hover über Node → Highlight aller Messages dieses Nodes in der Sidebar)
- **Animation** / Übergang beim Highlight-Wechsel

### Betroffene Dateien

#### Geändert
- `frontend/src/stores/flowStore.ts` — neuer `hoveredDebugNodeId`-Ref + Setter
- `frontend/src/components/DebugPanel.vue` — Hover-Listener pro Row, Cleanup im Unmount
- `frontend/src/components/nodes/BaseNode.vue` — `isDebugHovered`-Computed, Class-Binding, CSS-Regel

---

## Erweiterung: Click-to-Jump zum Quell-Node

### Status: Open

### Ausgangslage

Die Debug-Sidebar zeigt pro Message den Node-Namen (bzw. die ersten 8 Zeichen der Node-ID als Fallback) als reine Text-Anzeige. Bei großen Flows oder mehreren Tabs kann es mühsam sein, den Quell-Node manuell zu suchen — insbesondere wenn er außerhalb des Viewports liegt oder in einem anderen Flow-Tab steckt.

### Ziel

Klick auf die Node-Identifizierung in einer Debug-Row springt direkt zum Quell-Node:
1. Wechselt bei Bedarf den aktiven Flow-Tab (`flowId` der Message)
2. Selektiert den Node (Property-Panel öffnet)
3. Pant/zoomt das Vue-Flow-Canvas, sodass der Node mittig im Viewport liegt

### Anforderungen

#### 1. Klick-Ziel

- Klickbar wird der **Node-Name-Span** in der Header-Zeile jeder Debug-Row (`DebugPanel.vue:156-165`) — also genau der Bereich, der heute schon `nodeName` bzw. den gekürzten `nodeId`-Fallback zeigt
- Visuell als interaktiv markieren: `cursor-pointer`, dezenter Hover-Effekt (z.B. Underline oder leichter Color-Shift)
- Tooltip beim Hover: `Jump to <nodeName> (<nodeId>)` — gibt dem Bediener Klarheit, dass es sich um eine Aktion handelt
- Keine Konflikte mit dem Hover-Highlight (separates Feature): Hover **highlightet** den Node, Click **springt hin** und selektiert

#### 2. Sprung-Verhalten

Bei Klick:

1. **Flow-Wechsel** (falls nötig): Wenn `msg.flowId !== flowStore.activeFlowId`, dann `flowStore.setActiveFlow(msg.flowId)` aufrufen. Vue-Flow rendert dann den anderen Flow.
2. **Selektion**: `flowStore.selectNode(msg.nodeId)` — Property-Panel öffnet, `selected`-State aktiviert
3. **Pan/Zoom auf Node**: Vue-Flow's `setCenter(x, y, { zoom })` ruft die Node-Position aus den geladenen Nodes ab und zentriert. Aktueller Zoom bleibt erhalten (sofern sinnvoll), alternativ ein moderater Default-Zoom (z.B. 1.0) — entscheiden wir bei Implementierung anhand des Look-and-Feel

#### 3. Edge-Cases

- **Node existiert nicht mehr** (gelöscht seit der Message): Flow-Wechsel passiert ggf., Selection schlägt fehl → kein Crash, optional kurze Notification ("Node nicht mehr im Flow vorhanden")
- **Flow existiert nicht mehr**: kein Tab-Wechsel, kein Crash, optional Notification
- **Node liegt außerhalb des aktuellen Zooms**: `setCenter` zentriert ihn, Zoom bleibt unverändert — der Node ist garantiert sichtbar
- **Klick während laufender Auto-Scroll-Burst**: keine Beeinflussung — Sidebar-Scroll-Verhalten bleibt unabhängig

### Implementierungsplan

#### Cross-Component-Brücke: Focus-Request über Store

Da `useVueFlow('flint-flow-editor')` zwar von überall aufrufbar ist, der Pan-Aufruf aber nach einem ggf. nötigen Flow-Wechsel **erst nach dem Render** des neuen Flows passieren darf, geht der Trigger über einen Store-State, der vom `FlowEditor` gewatcht wird:

In `flowStore.ts`:

```ts
const focusRequest = ref<{ nodeId: string; flowId: string; ts: number } | null>(null)

function focusNode(nodeId: string, flowId: string) {
  if (flowId !== activeFlowId.value) {
    setActiveFlow(flowId)
  }
  selectNode(nodeId)
  // ts erzwingt Reaktivität auch bei wiederholten Klicks auf dieselbe ID
  focusRequest.value = { nodeId, flowId, ts: performance.now() }
}
```

In `FlowEditor.vue`:

```ts
const { setCenter, getNode } = useVueFlow('flint-flow-editor')

watch(
  () => flowStore.focusRequest,
  async (req) => {
    if (!req) return
    await nextTick() // sicherstellen, dass nach Flow-Wechsel die Nodes gerendert sind
    const node = getNode.value(req.nodeId)
    if (!node) return
    const x = node.position.x + (node.dimensions?.width ?? 0) / 2
    const y = node.position.y + (node.dimensions?.height ?? 0) / 2
    setCenter(x, y, { duration: 300 })
  },
)
```

#### Sidebar (`DebugPanel.vue`)

Den Node-Name-Span in `<button>` umwandeln (oder `<span role="button" tabindex="0">`), Click-Handler:

```vue
<button
  class="text-[11px] font-medium truncate cursor-pointer hover:underline ..."
  :title="`Jump to ${msg.nodeName || msg.nodeId} (${msg.nodeId})`"
  @click="flowStore.focusNode(msg.nodeId, msg.flowId)"
>
  {{ msg.nodeName || msg.nodeId?.slice(0, 8) }}
</button>
```

`v-memo`-Kompatibilität: Click-Handler ist eine statische Property-Reference auf den Store — Vue mountet ihn einmal, kein Diff nötig.

### Out of Scope für Phase 1

- **Sprung in einen Sub-Flow** (z.B. wenn Sub-Flows künftig eingeführt werden)
- **Animations-Pfad** durch den Flow zum Node (z.B. animierter Pan über Zwischen-Nodes)
- **Highlighting nach dem Sprung** (Pulse / Glow zur Bestätigung) — das `selected`-Styling und der existierende Hover-Highlight reichen vorerst
- **Keyboard-Navigation** (Tab durch Messages, Enter zum Sprung) — sinnvoll, aber separates Feature
- **Notifications** bei verschwundenem Node/Flow — nice-to-have, erstmal stumm scheitern

### Betroffene Dateien

#### Geändert
- `frontend/src/stores/flowStore.ts` — `focusRequest`-Ref + `focusNode`-Action
- `frontend/src/components/DebugPanel.vue` — Node-Name-Span → klickbarer Button
- `frontend/src/views/FlowEditor.vue` — Watcher auf `focusRequest`, Aufruf von `setCenter`

---

## Erweiterung: Pin-Path — Attribut highlighten und in allen Messages auto-expanden

### Status: Open

### Ausgangslage

Der JSON-Tree-View erlaubt zwar Aufklappen einzelner Knoten, aber bei vielen aufeinanderfolgenden Debug-Messages desselben Nodes muss jede Message manuell aufgeklappt werden, um ein bestimmtes Attribut zu sehen. Wenn ein Bediener z.B. `payload.temperature` über die Zeit beobachten will, ist das mühsam.

### Ziel

Per Klick auf einen Knoten im JSON-Tree wird der Pfad **gepinnt**. Folge:
1. Der gepinnte Knoten ist visuell hervorgehoben
2. **Alle Messages desselben Nodes** (vergangene **und** zukünftige) klappen ihren Tree automatisch so auf, dass dieser Pfad sichtbar ist — sofern das Attribut existiert
3. Erneuter Klick → Pin entfernt, Auto-Expand verschwindet, Trees fallen auf den manuellen/Default-State zurück

Der Bediener kann so ein Property "verfolgen", ohne jede Message anzufassen.

### Anforderungen

#### 1. Pin-Aktion

- Neuer dritter Hover-Button rechts pro Tree-Zeile, neben den existierenden `path` / `val`-Buttons in `JsonTreeView.vue:147-156`. Beschriftung z.B. `pin` (uppercase, gleicher Stil)
- Klick auf `pin`:
  - Wenn der Pfad noch nicht gepinnt ist → pinnen
  - Wenn er gepinnt ist → unpinnen (Toggle)
- Pinning ist immer **pro `nodeId`**: derselbe Pfad in Messages eines anderen Nodes ist nicht betroffen
- **Mehrere Pins pro Node** sind erlaubt (Set-basiert) — z.B. `payload.temperature` UND `payload.humidity` parallel pinnen
- Ein Pin existiert nur in-memory; persistiert nicht über Session-Grenzen (out of scope für Phase 1)

#### 2. Visuelles Highlighting

- **Gepinnter Knoten**: dezent gefärbter Hintergrund (`bg-accent/10` oder ähnlich) plus linker 2px-Marker in `accent`-Farbe, sodass er auf einen Blick auffindbar ist
- Der `pin`-Button selbst zeigt im gepinnten Zustand statt des Wortes `pin` ein gefülltes Pin-Symbol oder `★` — eindeutig vom Default-Zustand unterscheidbar
- **Pfad-Vorfahren werden NICHT zusätzlich highlightet** — sonst wirkt der Tree schnell überladen. Nur der Endknoten markiert.

#### 3. Auto-Expand-Logik

- Ein gepinnter Pfad zwingt alle **Container-Knoten auf seinem Weg** in den Expanded-State
- Beispiel: Pin auf `payload.items[0].name` → forciert `root`, `payload`, `payload.items`, `payload.items[0]` als expanded; `name` selbst ist Leaf, kein Toggle nötig
- Der gepinnte Pfad **dominiert** den manuellen Toggle-State: solange der Pin aktiv ist, lässt sich ein Vorfahren-Container nicht zuklappen (Toggle wird ignoriert oder visuell deaktiviert). Das ist die klare Regel "pinned = always visible"
- Wenn das Attribut in einer bestimmten Message **nicht existiert** (z.B. die Payload hat `payload.foo` aber kein `payload.items`), bleibt die betroffene Message im Default-Zustand. Kein Fehler, kein Force-Expand auf etwas das nicht da ist.

#### 4. Geltungsbereich

- Pin gilt für **Messages des gleichen Nodes** (`nodeId`-Match)
- Andere Nodes in der Liste sind unbeeinflusst
- Filter (`debugStore.filter`) und Suppression (`active === false`) bleiben unverändert wirksam — Pin überschreibt sie nicht

#### 5. Aufräumen

- Wenn ein Debug-Node aus dem Flow gelöscht wird oder seine `active === false` gesetzt wird: bestehende Pins bleiben gespeichert (sie schaden nicht), könnten optional bereinigt werden — out of scope für Phase 1
- "Clear all messages" (CLR-Button) löscht den Message-Buffer, **lässt Pins aber bestehen** — Pins folgen Nodes, nicht Messages

### Implementierungsplan

#### State (`debugStore.ts`)

```ts
// nodeId → set of pinned paths (relative to message payload root)
const pinnedPaths = ref<Map<string, Set<string>>>(new Map())

// Bumped on every pin change. Used as v-memo dependency in DebugPanel
// to force re-render of message rows so the new pin state takes effect.
const pinnedPathsVersion = ref(0)

function togglePinnedPath(nodeId: string, path: string): void {
  const map = new Map(pinnedPaths.value)
  const existing = map.get(nodeId)
  if (existing && existing.has(path)) {
    existing.delete(path)
    if (existing.size === 0) map.delete(nodeId)
    else map.set(nodeId, new Set(existing))
  } else {
    const next = new Set(existing ?? [])
    next.add(path)
    map.set(nodeId, next)
  }
  pinnedPaths.value = map
  pinnedPathsVersion.value++
}

function pinnedPathsForNode(nodeId: string): Set<string> {
  return pinnedPaths.value.get(nodeId) ?? EMPTY_SET
}
```

Im Return: `pinnedPaths`, `pinnedPathsVersion`, `togglePinnedPath`, `pinnedPathsForNode` exportieren.

#### Tree-Komponente (`JsonTreeView.vue`)

- Neue Prop: `nodeId?: string` — vom `DebugPanel` beim Top-Level-Aufruf reingegeben, in rekursiven Aufrufen weitergereicht
- Computeds:
  ```ts
  const pinnedPaths = computed(() =>
    props.nodeId ? debugStore.pinnedPathsForNode(props.nodeId) : EMPTY_SET,
  )

  const isPinned = computed(() => pinnedPaths.value.has(props.path))

  const isOnPinnedPath = computed(() => {
    for (const p of pinnedPaths.value) {
      if (p === props.path) return true
      if (p.startsWith(props.path + '.')) return true
      if (p.startsWith(props.path + '[')) return true
    }
    return false
  })
  ```
- Bisheriger `expanded`-Ref bleibt (lokal manuell), aber das Template/Toggle-Verhalten nutzt einen Computed-Wrapper:
  ```ts
  const effectiveExpanded = computed(() => isOnPinnedPath.value || expanded.value)

  function toggle() {
    if (!isContainer.value) return
    if (isOnPinnedPath.value) return // pin dominates manual toggle
    expanded.value = !expanded.value
  }
  ```
- Pin-Button: `v-if="props.nodeId"` (nur in der Sidebar sinnvoll), klickt `debugStore.togglePinnedPath(props.nodeId, props.path)`
- Visuelle Hervorhebung des gepinnten Knotens: zusätzliche Klasse auf der Header-Zeile, z.B. `bg-accent/10 border-l-2 border-accent -ml-1 pl-0.5`

#### `DebugPanel.vue`

- `<JsonTreeView :node-id="msg.nodeId" ... />`
- `v-memo` der Message-Row erweitern auf `[msg.id, debugStore.pinnedPathsVersion]`. Damit invalidiert ein Pin-Toggle alle Items — funktional korrekt, weil Pins selten getoggelt werden (nicht im 100/s-Bereich)

### Edge-Cases

- **Pfad mit Sonderzeichen**: `JsonTreeView.buildChildPath` erzeugt schon korrekte Bracket-Notation für Keys mit Punkten/Klammern. Das gepinnte `props.path` matched also exakt.
- **Pin auf Root** (`path === ''`): theoretisch möglich, aber visuell sinnlos (Root ist eh expanded). Die `pin`-Schaltfläche kann am Root ausgeblendet werden (sie nutzt heute schon `v-if="path"` für `path`-Copy — analoge Logik).
- **Identische `nodeId` über Flows hinweg**: sollte nicht auftreten (UUIDs), aber falls doch: Pin würde "über" Flow-Grenzen wirken — akzeptabel, weil `nodeId` deterministisch eindeutig ist.

### Out of Scope für Phase 1

- **Persistenz** der Pins über Session-Grenzen (LocalStorage)
- **Cross-Node-Pins** (gleicher Pfad in Messages aller Nodes verfolgen)
- **Inline-Wert-Übersicht** in einer separaten "Pinned Values"-Leiste oben in der Sidebar (interessantes Phase-2-Feature)
- **Bereinigung** von Pins beim Löschen/Deaktivieren eines Nodes
- **Pin-Verwaltung** (Liste aller aktiven Pins, einzeln entfernen)

### Betroffene Dateien

#### Geändert
- `frontend/src/stores/debugStore.ts` — `pinnedPaths`-Map, `pinnedPathsVersion`-Counter, `togglePinnedPath`/`pinnedPathsForNode`
- `frontend/src/components/JsonTreeView.vue` — `nodeId`-Prop, Pin-Button, `effectiveExpanded`-Computed, Toggle-Block, Highlight-Styling
- `frontend/src/components/DebugPanel.vue` — `nodeId`-Prop an `JsonTreeView`, `v-memo` um Version-Counter ergänzen
