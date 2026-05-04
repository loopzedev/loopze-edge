# Tooltip Migration: native `title` → `AppTooltip`

## Description

Tooltips in the codebase are currently rendered via two paths:

1. **`AppTooltip`** (`components/ui/AppTooltip.vue`) — Radix-based, styled in the terminal theme (`bg-terminal-surface`, `border-terminal-border`, `font-mono`, 400ms delay). Currently used: PropertyPanel (2 places), BaseNode output handles (Switch node).
2. **Native `title="..."`** — browser default styling, ~500ms delay (browser-dependent), no theming, appears in ~30 places.

Goal: render all tooltips via `AppTooltip` so look-and-feel is consistent and tooltip behavior (delay, side, animation) can be changed centrally.

## What is *not* a Tooltip

`title=` also appears in non-tooltip contexts — these **remain unchanged**:

- `<PanelHeader title="Properties" />` — `title` as prop = header text
- `<SectionHeader title="Configuration" />` — same
- `<a title="...">` without a visible label, only serving a11y

The places meant here are exclusively `title=` attributes on interactive elements that are supposed to display a hover hint.

## Acceptance Criteria

- All places listed below render the hint via `AppTooltip` instead of native `title`.
- Visual appearance: uniformly terminal theme.
- A11y not degraded: where `title` previously also served as a screen-reader hint (icon buttons without visible label), `aria-label` is set with the same text.
- No duplicate tooltips (do not accidentally leave AppTooltip + remaining `title` on the same element).

## Migration Pattern

### Standard case (icon button with tooltip)

```vue
<!-- Before -->
<button title="Close panel" @click="...">×</button>

<!-- After -->
<AppTooltip text="Close panel">
  <button aria-label="Close panel" @click="...">×</button>
</AppTooltip>
```

Add `aria-label` because `title` is gone and the button has no visible text.

### Element with visible text + additional hint

```vue
<!-- Before -->
<span :title="full">{{ short }}</span>

<!-- After -->
<AppTooltip :text="full">
  <span>{{ short }}</span>
</AppTooltip>
```

No `aria-label` needed because visible text is present.

### Special case: central wrapper component (`IconButton`)

`IconButton.vue` currently takes a `title` prop and forwards it to `<button title="...">`. Instead of migrating every caller: **switch `IconButton` internally to `AppTooltip`**, the API stays the same. Saves ~10 call-site edits.

```vue
<!-- IconButton.vue, new internal -->
<AppTooltip :text="title">
  <button :aria-label="title" ...>
    <slot />
  </button>
</AppTooltip>
```

## Migration Checklist

One checkbox per component. Order chosen so that central bottlenecks (IconButton, PropertyListItem) come first — they cascade to many places.

### High leverage (central components)

- [ ] **`components/ui/IconButton.vue`** (line 18) — internal refactor; all callers benefit without change
- [ ] **`components/ui/PropertyListItem.vue`** (L. 27 "Drag to reorder", L. 48 "Remove")
- [ ] **`components/nodes/BaseNode.vue`** (L. 150 actionButton, L. 223 "Undeployed changes" dot, L. 256 toggle ON/OFF)

### Header / global UI

- [ ] **`components/HeaderBar.vue`** (L. 76, 101, 119, 133, 147, 171, 223) — 7 tooltips. L. 101 is data-bound (`Status: ${connectionLabel}`)
- [ ] **`components/FlowTabBar.vue`** (L. 90 "New flow")
- [ ] **`components/PanelHeader.vue`** (L. 22 "Close panel")

### Panels

- [ ] **`components/DebugPanel.vue`** (L. 97 pause/resume, L. 107 "Clear all messages", L. 175 dynamic "Jump to ...")
- [ ] **`components/ContextPanel.vue`** (L. 256 "Refresh", L. 266 "Delete")
- [ ] **`components/JsonTreeView.vue`** (L. 168 collapse/expand, L. 213 "Copy path", L. 218 "Copy value", L. 225 pin/unpin)
- [ ] **`components/PropertyPanel.vue`** (L. 193 "Revert changes…")
- [ ] **`components/FlowProperties.vue`** (L. 169 "Delete flow / Last flow…")

### Nodes

- [ ] **`components/nodes/LinkNode.vue`** (L. 75 "Undeployed changes")
- [ ] **`components/nodes/FunctionNode.vue`** (L. 39 code preview tooltip)

### Palette

- [ ] **`components/NodePalette.vue`** (L. 157 `node.description`)

## PR Order

Proposal — can also be one PR if manageable:

1. **PR A — Central components**: IconButton, PropertyListItem, BaseNode. Largest leverage here; everything else gets smaller after this.
2. **PR B — Panels & header**: HeaderBar, DebugPanel, ContextPanel, JsonTreeView, PropertyPanel, FlowProperties, FlowTabBar, PanelHeader.
3. **PR C — Rest**: LinkNode, FunctionNode, NodePalette.

## Pitfalls

1. **AppTooltip wraps the slot in a Radix `TooltipTrigger as-child`** — this only works cleanly when the slot renders a DOM element (no component delivering multiple root elements). For components without a single root a `<span>`/`<div>` wrapper is needed.
2. **VueFlow `Handle`** works (see BaseNode outputs as example) — the handle internally renders a single div and accepts the trigger props cleanly.
3. **Touch devices**: `AppTooltip` shows nothing on touch (Radix default). `title` at least showed on long-press. If important on touch, decide individually — usually acceptable, since touch users rarely need the hint anyway (buttons have visible labels or speaking icons).
4. **Performance**: every `AppTooltip` mounts a `TooltipProvider`. With many tooltips in a list (e.g., NodePalette with 20+ nodes) consider rendering a single `TooltipProvider` as a wrapper and only `TooltipRoot` per item. If relevant, `AppTooltip` can be extended with a `provider="external"` mode — measure first whether needed.
5. **`title` attributes on `<a>` with URL** for external links: do not migrate, that is the conventional way and browsers display it without JS.

## Dependencies

- `AppTooltip` recently supports an empty `text` (then renders only the slot) — important for optional tooltips. No further API changes needed.
- All migrated places need `import AppTooltip from '@/components/ui/AppTooltip.vue'`.

## Out of Scope

- New tooltip features (multi-line, rich content, clickable tooltips). First finish the migration cleanly, then separately.
- A global `tooltip` directive (`v-tooltip="text"`) — possible, but component wrapping is explicit and forces the spot where the trigger sits. Evaluate after migration whether the directive would add value.
