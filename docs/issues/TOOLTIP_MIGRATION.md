# Tooltip-Migration: native `title` → `AppTooltip`

## Beschreibung

Im Codebase werden Tooltips heute über zwei Wege gerendert:

1. **`AppTooltip`** (`components/ui/AppTooltip.vue`) — Radix-basiert, im Terminal-Theme gestaltet (`bg-terminal-surface`, `border-terminal-border`, `font-mono`, 400ms Delay). Aktuell genutzt: PropertyPanel (2 Stellen), BaseNode-Output-Handles (Switch-Node).
2. **Natives `title="..."`** — Browser-Default-Styling, ~500ms Delay (browser-abhängig), kein Theming, taucht an ~30 Stellen auf.

Ziel: alle Tooltips über `AppTooltip` rendern, damit Look-and-Feel konsistent ist und sich Tooltip-Verhalten (Delay, Side, Animation) zentral ändern lässt.

## Was *kein* Tooltip ist

`title=` taucht auch in nicht-Tooltip-Kontexten auf — diese **bleiben unverändert**:

- `<PanelHeader title="Properties" />` — `title` als Prop = Header-Text
- `<SectionHeader title="Konfiguration" />` — dito
- `<a title="...">` ohne sichtbares Label, das nur a11y dient

Die hier gemeinten Stellen sind ausschließlich `title=`-Attribute auf interaktiven Elementen, die einen Hover-Hinweis anzeigen sollen.

## Akzeptanzkriterien

- Alle nachfolgend gelisteten Stellen rendern den Hinweis über `AppTooltip` statt nativem `title`.
- Visuelles Erscheinungsbild: einheitlich Terminal-Theme.
- A11y nicht verschlechtert: wo `title` bisher zugleich als Screen-Reader-Hint diente (Icon-Buttons ohne sichtbares Label), wird `aria-label` mit dem gleichen Text gesetzt.
- Keine doppelten Tooltips (nicht versehentlich AppTooltip + verbleibendes `title` auf demselben Element).

## Migrations-Pattern

### Standard-Fall (Icon-Button mit Tooltip)

```vue
<!-- Vorher -->
<button title="Close panel" @click="...">×</button>

<!-- Nachher -->
<AppTooltip text="Close panel">
  <button aria-label="Close panel" @click="...">×</button>
</AppTooltip>
```

`aria-label` ergänzen, weil das `title` weg ist und der Button keinen sichtbaren Text hat.

### Element mit sichtbarem Text + zusätzlichem Hint

```vue
<!-- Vorher -->
<span :title="full">{{ short }}</span>

<!-- Nachher -->
<AppTooltip :text="full">
  <span>{{ short }}</span>
</AppTooltip>
```

Kein `aria-label` nötig, weil sichtbarer Text vorhanden.

### Sonderfall: zentraler Wrapper-Component (`IconButton`)

`IconButton.vue` nimmt heute eine `title`-Prop und gibt sie auf `<button title="...">` weiter. Statt jeden Caller zu migrieren: **`IconButton` intern auf `AppTooltip` umstellen**, API bleibt gleich. Spart ~10 Call-Site-Edits.

```vue
<!-- IconButton.vue, neu intern -->
<AppTooltip :text="title">
  <button :aria-label="title" ...>
    <slot />
  </button>
</AppTooltip>
```

## Migrations-Checkliste

Pro Component eine Checkbox. Reihenfolge so gewählt, dass zentrale Bottlenecks (IconButton, PropertyListItem) zuerst kommen — die schlagen auf viele Stellen durch.

### Hoher Hebel (zentrale Components)

- [ ] **`components/ui/IconButton.vue`** (Zeile 18) — internes Refactor; alle Aufrufer profitieren ohne Änderung
- [ ] **`components/ui/PropertyListItem.vue`** (Z. 27 "Drag to reorder", Z. 48 "Remove")
- [ ] **`components/nodes/BaseNode.vue`** (Z. 150 actionButton, Z. 223 "Undeployed changes" Dot, Z. 256 toggle ON/OFF)

### Header / globale UI

- [ ] **`components/HeaderBar.vue`** (Z. 76, 101, 119, 133, 147, 171, 223) — 7 Tooltips. Z. 101 ist datengebunden (`Status: ${connectionLabel}`)
- [ ] **`components/FlowTabBar.vue`** (Z. 90 "New flow")
- [ ] **`components/PanelHeader.vue`** (Z. 22 "Close panel")

### Panels

- [ ] **`components/DebugPanel.vue`** (Z. 97 pause/resume, Z. 107 "Clear all messages", Z. 175 dynamisches "Jump to ...")
- [ ] **`components/ContextPanel.vue`** (Z. 256 "Refresh", Z. 266 "Delete")
- [ ] **`components/JsonTreeView.vue`** (Z. 168 collapse/expand, Z. 213 "Copy path", Z. 218 "Copy value", Z. 225 pin/unpin)
- [ ] **`components/PropertyPanel.vue`** (Z. 193 "Revert changes…")
- [ ] **`components/FlowProperties.vue`** (Z. 169 "Flow löschen / Letzter Flow…")

### Nodes

- [ ] **`components/nodes/LinkNode.vue`** (Z. 75 "Undeployed changes")
- [ ] **`components/nodes/FunctionNode.vue`** (Z. 39 Code-Preview-Tooltip)

### Palette

- [ ] **`components/NodePalette.vue`** (Z. 157 `node.description`)

## Reihenfolge der PRs

Vorschlag — kann auch eine PR sein, falls überschaubar:

1. **PR A — Zentrale Components**: IconButton, PropertyListItem, BaseNode. Hier liegt der größte Hebel; alle anderen werden danach kleiner.
2. **PR B — Panels & Header**: HeaderBar, DebugPanel, ContextPanel, JsonTreeView, PropertyPanel, FlowProperties, FlowTabBar, PanelHeader.
3. **PR C — Reste**: LinkNode, FunctionNode, NodePalette.

## Stolperfallen

1. **AppTooltip wrappt den Slot in einen Radix-`TooltipTrigger as-child`** — das funktioniert nur sauber, wenn der Slot ein DOM-Element rendert (kein Component, das mehrere Root-Elemente liefert). Bei Komponenten ohne Single-Root muss ein `<span>`/`<div>` außenrum.
2. **VueFlow-`Handle`** funktioniert (siehe BaseNode-Outputs als Beispiel) — der Handle rendert intern ein einzelnes div und nimmt die Trigger-Props sauber an.
3. **Touch-Geräte**: `AppTooltip` zeigt nichts auf Touch (Radix-Default). `title` zeigte immerhin auf Long-Press. Falls auf Touch wichtig, jeweils einzeln entscheiden — meist akzeptabel, weil Touch-Nutzer den Hinweis ohnehin selten brauchen (Buttons haben sichtbare Labels oder sprechende Icons).
4. **Performance**: jeder `AppTooltip` mountet einen `TooltipProvider`. Bei vielen Tooltips in einer Liste (z.B. NodePalette mit 20+ Nodes) ggf. einen einzelnen `TooltipProvider` als Wrapper rendern und nur `TooltipRoot` pro Item. Falls relevant, kann `AppTooltip` um einen `provider="external"`-Modus erweitert werden — erst messen, ob nötig.
5. **`title`-Attribute auf `<a>` mit URL** für externe Links: nicht migrieren, das ist der konventionelle Weg und Browser zeigen es ohne JS.

## Abhängigkeiten

- `AppTooltip` unterstützt seit kurzem leeren `text` (rendert dann nur den Slot) — wichtig für optionale Tooltips. Keine weiteren API-Änderungen nötig.
- Alle migrierten Stellen brauchen `import AppTooltip from '@/components/ui/AppTooltip.vue'`.

## Out of Scope

- Neue Tooltip-Features (Multi-Line, Rich Content, klickbare Tooltips). Erst Migration sauber abschließen, dann separat.
- Eine globale `tooltip`-Directive (`v-tooltip="text"`) — möglich, aber Component-Wrapping ist explizit und zwingt zur Stelle, wo der Trigger steht. Erst nach Migration evaluieren, ob die Directive Mehrwert hätte.
