# Issue: Dashboard Stat Widget (`ui-stat`)

## Status: Proposed

---

## Locked decisions (resolved before implementation)

| # | Decision | Rationale |
|---|----------|-----------|
| L-1 | Widget type ID is `ui-stat` (Grafana convention) | Short, recognisable. Not `ui-kpi` (too business-y), not `ui-metric` (collides with telemetry vocabulary), not `ui-bignumber` (only describes the value, not the trend/sparkline). |
| L-2 | Main value, delta, and sparkline are **three independent pieces of one widget**, each toggleable | Operators want different combos: just a big number for ops-room TVs; number + delta for KPI cards; everything for an executive overview. One widget with toggles beats three near-duplicate widgets. |
| L-3 | Sparkline data lives in a **server-side windowed cache**, like `ui-chart` | Consistency with the planned chart widget. The dashboard SPA must not be responsible for buffering history because (a) fresh clients need replay on connect and (b) the operator can step away and the buffer survives. |
| L-4 | The widget is fed by **one message per update**, carrying value + optional delta in one go | Atomic update — the displayed value and its delta are always consistent. Avoids the "delta updated, then value updated" flicker. |
| L-5 | Value source: `msg[property]` (default `payload`); delta source: `msg[deltaProperty]` (default `delta`) | Mirrors the pattern used by `ui-text` and `ui-gauge`. Operators already know `msg.payload`; the deltaProperty escape hatch lets them split their data without a Change node. |
| L-6 | **Delta semantics are configurable**: `up-is-good` (default), `down-is-good`, or `neutral` (always gray) | An "error rate" KPI inverts the color logic — a 10 % rise is bad. A "noise level" should be neutral. Hardcoding green=up burns half the use-cases. |
| L-7 | **Sparkline window is point-based, not time-based**, for v1 | Time windows require a server-side timer; points only require a ring buffer. Time-window can be added later as `windowDuration` without breaking `windowSize`. |
| L-8 | The sparkline **shares its color with the delta tint by default**, override per-config | Operator-friendly: a "rising and bad" widget gets a red number, red delta, AND red sparkline — single visual cue. Configurable for cases where the sparkline should stay brand-accent regardless. |
| L-9 | Number formatting is **explicit (separator + decimals)**, not locale-derived | Locale-derived means the same widget renders differently for two operators looking at the same screen. Explicit config makes "63 704" the same everywhere. |
| L-10 | Widget defaults are **chosen to match the screenshot**: vertical layout, area sparkline, space-separated thousands, +/- prefix on delta | The screenshot is the reference "looks good out of the box" target. Fresh users drop a `ui-stat`, hook up `msg.payload`, and get something usable. |

---

## Context

The dashboard has display widgets for single discrete states (`ui-text`,
`ui-led`) and for analog readouts (`ui-gauge`). It has no widget for the
most common operations-dashboard pattern: a **headline KPI** —
production count, throughput, revenue, alarms — shown big, with trend
context and recent history.

The screenshot (`THROUGHPUT · TODAY → 63 704 · +8.4% vs yesterday +
sparkline`) is the reference design. The widget must render this exact
layout out of the box, while exposing enough config to cover related
patterns (no sparkline, inverted color logic, horizontal layout, etc.).

---

## Reference design

```
┌──────────────────────────────────────────────┐
│  THROUGHPUT · TODAY                          │  ← label · sublabel (small, muted, uppercase)
│                                              │
│  63 704                                      │  ← big value (~3rem, bold, white)
│  +8.4% VS YESTERDAY                          │  ← delta (colored) + period suffix (muted)
│                                              │
│   ╱╲    ╱╲     ╱╲╱╲      ╱╲   ╱╲╱╲          │  ← sparkline (area chart, accent-colored)
│ ╱   ╲╱╲╱  ╲╱╲╱╲    ╲╱╲╱╲╱  ╲╱╲    ╲╱╲       │
└──────────────────────────────────────────────┘
```

All four sections are independently toggleable.

---

## Problem description

The current widget set forces operators to build KPI cards as
multi-widget compositions: a `ui-text` for the label, another for the
value, a third for the delta. The result is layout-fragile, can't share
a server-side history window, and produces a visual mismatch (different
fonts, alignment drift, no atomic updates).

A first-class `ui-stat` widget eliminates this.

---

## View / rationale

`ui-stat` is the **headline-metric widget**. It's optimised for "one
number that matters" — visible from across the room, with just enough
context to interpret it at a glance.

- **Vertical layout (default)**: label → value → delta → sparkline.
  Matches the reference and dense dashboard grids.
- **Horizontal layout (alternative)**: label + value on the left,
  delta + sparkline on the right. Use when the widget cell is wider
  than tall.

The widget is **read-only** — no input port, no event emission. It
mirrors a single incoming message into its display.

---

## Functional scope reference

| Feature | Scope |
|---------|-------|
| Big numeric value with formatting (thousands separator, decimals) | In scope |
| Static unit prefix/suffix (€, $, kg, °C) | In scope |
| Two-part label with separator | In scope |
| Trend delta value (absolute, percentage, or both) | In scope |
| Delta color logic with three modes (up-is-good / down-is-good / neutral) | In scope |
| Trailing delta-context text (`VS YESTERDAY`, `vs target`, …) | In scope |
| Area sparkline with point-based window | In scope |
| Sparkline color independent or auto-matched to delta | In scope |
| Horizontal layout option | In scope |
| Three section toggles (label, delta, sparkline) | In scope |
| Server-side sparkline cache with replay on fresh client connect | In scope |
| Time-based sparkline window (`windowDuration`) | Out of scope (Phase 2) |
| Comparison-line / target overlay on sparkline | Out of scope |
| Icon next to label | Out of scope (defer; can ship as Phase 2 polish) |
| Drill-down click handler / navigation | Out of scope |

---

## Architecture overview

```
Flow
  [any node] ──msg──▶ [ui-stat]
                          │
                          ├─ resolve value:  msg[property]      → number
                          ├─ resolve delta:  msg[deltaProperty] → number | undefined
                          ├─ hub.PushWidgetValue(nodeID, {value, delta, ts}, ts)
                          └─ hub.AppendStatSample(nodeID, value, ts)  ◀─── new method
                                              │
                                              ▼
                                       Hub windowed cache
                                       (ring buffer, configured size)
                                              │
                                              ▼
                                       Broadcast WS frame:
                                       {type:"widget", id, value:{value, delta, ts}}
                                       {type:"stat-sample", id, sample, ts}
                                              │
                                              ▼
                                       Dashboard SPA
                                       └─ StatWidget renders:
                                            label / value / delta / sparkline
```

### Snapshot replay

When a fresh client connects, the snapshot handshake includes:

- Current `widgets[id]` entry → main value + delta (existing path).
- Current `statSamples[id]` ring buffer → sparkline history (new field
  on the snapshot frame).

---

## Wire protocol

### Backend → client

Existing frame for value+delta (no protocol change):

```json
{"type":"widget", "id":"<node-id>", "value":{"value":63704,"delta":8.4}, "ts":1234567890}
```

New frame for sparkline updates:

```json
{"type":"stat-sample", "id":"<node-id>", "sample":63704, "ts":1234567890}
```

New field on the snapshot frame:

```json
{
  "type":"snapshot",
  "layout":{ /* … */ },
  "widgets":{ "node-id":{"value":{"value":63704,"delta":8.4}, "ts":1234567890} },
  "statSamples":{ "node-id":[{"v":63100,"t":...}, {"v":63240,"t":...}, …] },
  "ts":1234567890
}
```

### Flow → backend (message arriving at `ui-stat`)

```js
msg = {
  payload: 63704,              // main value (path configurable via `property`)
  delta:   8.4,                // optional trend value (path configurable via `deltaProperty`)
  // … any other fields are ignored
}
```

If `delta` is absent, the widget shows the value + sparkline but
no delta row.

---

## Requirements

### 1. Node registration

- Type: `"ui-stat"`
- Label: `"Stat"`
- Category: `"dashboard"`
- Inputs: 1
- Outputs: 0
- Default size: width 3, height 4 (fits the reference design at typical
  page widths)

### 2. Config fields

| Field | Type | Default | Description |
|---|---|---|---|
| `group` | config-ID | `""` | Parent group (required). |
| `x`, `y`, `width`, `height` | number | grid defaults | Standard layout fields. |
| `label` | string | `""` | Primary label (e.g. `"THROUGHPUT"`). |
| `sublabel` | string | `""` | Secondary label (e.g. `"TODAY"`). |
| `labelSeparator` | string | `"·"` | Character between label and sublabel. |
| `tooltip` | string | `""` | Standard hover tooltip. |
| `property` | string | `"payload"` | Path within `msg` for the main value. |
| `deltaProperty` | string | `"delta"` | Path within `msg` for the delta value. |
| `decimals` | number | `0` | Decimal places on the main value. |
| `thousandsSeparator` | enum | `"space"` | One of `"space"`, `"comma"`, `"dot"`, `"none"`. |
| `decimalSeparator` | enum | `"dot"` | One of `"dot"`, `"comma"`. |
| `prefix` | string | `""` | Static prefix (e.g. `"€"`, `"$"`). |
| `suffix` | string | `""` | Static suffix (e.g. `"kg"`, `" pcs"`). |
| `showDelta` | boolean | `true` | Render the delta row. |
| `deltaFormat` | enum | `"percent"` | One of `"percent"`, `"absolute"`, `"both"`. |
| `deltaDecimals` | number | `1` | Decimal places on the delta value. |
| `deltaContext` | string | `""` | Trailing context text (e.g. `"VS YESTERDAY"`). |
| `deltaDirection` | enum | `"up-is-good"` | One of `"up-is-good"`, `"down-is-good"`, `"neutral"`. Controls color choice. |
| `showSparkline` | boolean | `true` | Render the sparkline. |
| `sparklineWindow` | number | `60` | Number of points retained in the rolling window. |
| `sparklineColor` | string | `""` | Override hex. Empty = match delta tint (or accent if neutral). |
| `sparklineFill` | boolean | `true` | Render the gradient fill below the line (area chart vs line chart). |
| `layout` | enum | `"vertical"` | One of `"vertical"`, `"horizontal"`. |
| `valueColor` | string | `""` | Override hex on the main number. Empty = `--fg`. |

### 3. Value resolution

```ts
function resolveValue(msg, property) {
  const v = lookupPath(msg, property)            // 'payload' or 'data.metrics.throughput'
  return typeof v === 'number' ? v : null
}
```

If `null` (missing or non-numeric), the widget renders `—` for the
value, no delta, no new sparkline sample.

### 4. Delta color logic

```ts
function deltaColor(value, direction) {
  if (value === 0 || direction === 'neutral') return MUTED
  const isPositive = value > 0
  const isGood = direction === 'up-is-good' ? isPositive : !isPositive
  return isGood ? STATUS_RUNNING : STATUS_FAULT
}
```

Where `STATUS_RUNNING` / `STATUS_FAULT` come from the existing dashboard
status palette (`#3fb950` / `#f85149`).

### 5. Sparkline rendering

- SVG `<path>` for the line, `<path>` with linear gradient `fill` for
  the area below.
- Auto-scaled Y axis (min/max of current window).
- No grid, no axis labels, no tooltips. The widget is a glanceable
  trend indicator, not an analytical chart.
- Reserved height inside the widget: 40 % of the widget's content area
  when vertical, full height when horizontal.

### 6. Number formatting

```ts
function formatNumber(value, decimals, thousandsSep, decimalSep) {
  const fixed = value.toFixed(decimals)
  const [intPart, decPart] = fixed.split('.')
  const sepMap = { space: ' ', comma: ',', dot: '.', none: '' }
  const grouped = intPart.replace(/\B(?=(\d{3})+(?!\d))/g, sepMap[thousandsSep])
  return decPart ? grouped + sepMap[decimalSep] + decPart : grouped
}
```

Reference: `63704` with `space`/`dot`/`0 decimals` → `"63 704"`.
Reference: `1234.5` with `dot`/`comma`/`1 decimal` → `"1.234,5"`.

### 7. Server-side sparkline cache

- New `StatSampleStore` in `internal/dashboard/`: ring buffer per widget
  ID, keyed by node ID, sized by `sparklineWindow`.
- Existing `Hub` gains `AppendStatSample(nodeID, value, ts)`.
- On `Cache.Retain(IDs)`, also retain matching `StatSampleStore` entries.
- Snapshot frame includes the current ring buffer for each `ui-stat`.

### 8. Layout: vertical (default)

```
┌──────────────────────────┐
│  LABEL · SUBLABEL        │  ← small, muted, uppercase
│                          │
│  BIG VALUE               │  ← clamp(2rem, 8vw, 4rem), bold
│  +8.4% CONTEXT           │  ← medium, colored
│                          │
│  ────sparkline────       │  ← 40 % of widget content area
└──────────────────────────┘
```

### 9. Layout: horizontal

```
┌────────────────────────────────────────────┐
│  LABEL · SUBLABEL    │                     │
│  BIG VALUE           │   ────sparkline──── │
│  +8.4% CONTEXT       │                     │
└────────────────────────────────────────────┘
```

50 / 50 column split. Sparkline takes the full height of the
widget content area on the right.

---

## Technical sketch

### Backend

**New file:** `internal/nodes/dashboard/stat.go`

```go
type UIStat struct{}

func (n *UIStat) TypeInfo() flow.NodeTypeInfo {
    return flow.NodeTypeInfo{
        Type:     "ui-stat",
        Label:    "Stat",
        Inputs:   1,
        Outputs:  0,
        Category: "dashboard",
    }
}

func (n *UIStat) Process(ctx flow.NodeContext, msg flow.Message) {
    cfg := ctx.Config()
    property := cfg.String("property", "payload")
    deltaProperty := cfg.String("deltaProperty", "delta")

    value, ok := numericFromMsg(msg, property)
    if !ok { return }
    delta, hasDelta := numericFromMsg(msg, deltaProperty)

    payload := map[string]any{"value": value}
    if hasDelta { payload["delta"] = delta }

    dashboard.GlobalHub.PushWidgetValue(ctx.NodeID(), payload, msg.Timestamp())
    dashboard.GlobalHub.AppendStatSample(ctx.NodeID(), value, msg.Timestamp(),
        cfg.Int("sparklineWindow", 60))
}
```

**Modify:** `internal/dashboard/hub.go` — add `statSamples` map (ring
buffer per widget), `AppendStatSample` method, snapshot serialisation,
broadcast frame.

**Modify:** `internal/dashboard/layout.go` — recognise `ui-stat` as a
widget type (`dashboardWidgetTypes`), set `widgetSizeDefault["ui-stat"] = {Width: 3, Height: 4}`.

### Frontend (editor)

**New file:** `frontend/src/nodes/dashboard/UIStatConfig.vue`

Standard config panel layout. Sections (collapsible would be nice, but
flat-list is fine for v1):

- Common: group, label, sublabel, tooltip
- Value: property, prefix, suffix, decimals, thousandsSeparator,
  decimalSeparator, valueColor
- Delta: showDelta, deltaProperty, deltaFormat, deltaDecimals,
  deltaContext, deltaDirection
- Sparkline: showSparkline, sparklineWindow, sparklineColor (uses
  `FormColorInput` with `clearable`), sparklineFill
- Layout: layout (vertical / horizontal), position, size

### Frontend (dashboard SPA)

**New file:** `frontend/dashboard/src/widgets/StatWidget.vue`

- Reads `widget.config` for all rendering options.
- Reads main value from the `value` prop (the payload pushed by
  `PushWidgetValue`).
- Reads sparkline samples from a new prop `samples` (an array
  maintained by the layout store, fed by the new `stat-sample`
  WS frame).

**Modify:** `frontend/dashboard/src/App.vue` — register `StatWidget`
in `WIDGET_COMPONENTS`, wire the new `samples` prop, handle the
`stat-sample` WS frame.

**Modify:** `frontend/dashboard/src/types.ts` — add `StatSampleFrame`
to `ServerFrame`, add `statSamples` to `SnapshotFrame`.

---

## Affected files

### Backend

| File | Change |
|------|--------|
| `internal/nodes/dashboard/stat.go` | **New** — `UIStat` node |
| `internal/nodes/dashboard/register.go` | Register `ui-stat` |
| `internal/dashboard/hub.go` | New `statSamples` field, `AppendStatSample`, `stat-sample` broadcast, snapshot replay |
| `internal/dashboard/cache.go` | New `StatSampleStore` (or extend `Cache`) |
| `internal/dashboard/layout.go` | Add `ui-stat` to `dashboardWidgetTypes`, default size |

### Frontend — editor

| File | Change |
|------|--------|
| `frontend/src/nodes/dashboard/UIStatConfig.vue` | **New** — config panel |
| `frontend/src/nodes/dashboard/index.ts` | Register node + config |
| `frontend/src/nodes/dashboard/sizing.ts` | Add default size |

### Frontend — dashboard SPA

| File | Change |
|------|--------|
| `frontend/dashboard/src/widgets/StatWidget.vue` | **New** — runtime widget |
| `frontend/dashboard/src/App.vue` | Register widget, wire `samples` prop, handle `stat-sample` frame |
| `frontend/dashboard/src/types.ts` | New frame type, snapshot extension |
| `frontend/dashboard/src/api.ts` / store | Persist `statSamples` map alongside `widgetValues` |

---

## Examples

### Example 1 — Reference design (the screenshot)

Config:

```yaml
label: THROUGHPUT
sublabel: TODAY
labelSeparator: '·'
thousandsSeparator: space
decimals: 0
showDelta: true
deltaFormat: percent
deltaContext: VS YESTERDAY
deltaDirection: up-is-good
showSparkline: true
sparklineWindow: 60
sparklineColor: ''       # auto-match delta tint
```

Incoming message:

```json
{"payload": 63704, "delta": 8.4}
```

Rendered: exactly the screenshot.

### Example 2 — Error rate (down-is-good)

```yaml
label: ERROR RATE
suffix: ' %'
decimals: 2
showDelta: true
deltaDirection: down-is-good     # rising = bad → red
sparklineColor: ''               # auto, will tint red if delta is positive
```

Incoming `{payload: 0.42, delta: 0.08}` renders as `"0.42 %"` with a
red `+0.08% VS …` row and a red sparkline.

### Example 3 — Headline number only

```yaml
label: ACTIVE USERS
showDelta: false
showSparkline: false
thousandsSeparator: comma
```

Incoming `{payload: 1248}` renders `"1,248"` with the label, no
delta, no sparkline.

### Example 4 — Horizontal layout for a wide cell

```yaml
layout: horizontal
label: REVENUE
prefix: '€ '
thousandsSeparator: dot
decimalSeparator: comma
decimals: 2
showDelta: true
showSparkline: true
```

Incoming `{payload: 12345.67, delta: -2.1}` in a 8×3 cell renders as
`"€ 12.345,67"` with delta on the left half, sparkline filling the
right half. Delta in red (down-is-good defaults off; with `up-is-good`
a negative delta is red).

---

## Tests

| # | Test | Verification |
|---|------|-------------|
| T-1 | Incoming `{payload: 63704}` → widget displays `"63 704"` (defaults) | Browser smoke + unit test on formatNumber |
| T-2 | Incoming `{payload: 1234.56, decimals: 2, thousandsSeparator: 'dot', decimalSeparator: 'comma'}` → `"1.234,56"` | Unit test |
| T-3 | Incoming `{payload: 100, delta: 5, deltaDirection: 'up-is-good'}` → delta row green | Unit test on `deltaColor` + render snapshot |
| T-4 | Incoming `{payload: 100, delta: 5, deltaDirection: 'down-is-good'}` → delta row red | Unit test |
| T-5 | Incoming `{payload: 100, delta: 0}` → delta row gray (muted) | Unit test |
| T-6 | Missing `delta` field → delta row not rendered | Render snapshot |
| T-7 | Non-numeric `payload` → value shows `"—"`, no sparkline sample appended | Unit test on `resolveValue` + hub test |
| T-8 | Append 100 samples with window=60 → ring buffer holds last 60 | Hub test |
| T-9 | Fresh client connect → receives `statSamples` in snapshot, sparkline immediately populated | Integration: connect, assert SVG path has expected point count |
| T-10 | `showSparkline=false` → no SVG rendered, no WS subscription overhead difference (samples still cached server-side, optimisation deferred) | Render snapshot |
| T-11 | `layout=horizontal` in a wide cell → 50/50 split, sparkline on right | Visual / snapshot |
| T-12 | `sparklineColor=''` follows the delta tint as it changes between updates | Component test: push positive delta → blue/green path stroke; push negative delta → red |
| T-13 | Hot reload (per `DASHBOARD_HOT_RELOAD.md`) preserves the sparkline history | Push samples, redeploy with stat ID unchanged, assert ring buffer survives and DOM-diff reuses the component (state preserved) |

---

## Dependencies

- No new external libraries. The sparkline is hand-rolled SVG, like
  `ui-gauge`.
- Reuses existing `FormColorInput` (`clearable`) for the optional
  color overrides.

---

## Out of scope for v1

- Time-based windows (`windowDuration`). Deferred — add when a use-case
  appears that can't be served by point-count windows.
- Icon next to the label. Defer — adds a font/icon-set dependency
  decision, distracts from shipping.
- Click-through / drill-down. The widget is read-only for v1.
- Target / threshold overlay on the sparkline. Defer.
- Per-message ad-hoc label override (e.g. `msg.label`). The label is
  config, not data, for now — keeps the protocol simple.

---

## Open questions

| # | Question | Suggestion |
|---|----------|------------|
| Q-1 | Should the sparkline subscribe to a separate `stat-sample` frame, or piggy-back on the existing `widget` frame as `value.sample`? | Separate frame is cleaner — `widget` carries the LATEST atomic display state, `stat-sample` is incremental history. Mixing them means the snapshot replay logic has to distinguish them anyway. |
| Q-2 | Should very large windows (>500 points) be downsampled before WS push for fresh-client replay? | Probably yes when we add `windowDuration`, where users might pick "last 24 h × 5 sec samples" = 17 k points. For v1's point-count default 60, irrelevant. |
| Q-3 | Should the delta context (`VS YESTERDAY`) be inferable from a configured period (e.g. `period: "day"` → "VS YESTERDAY"), or always literal? | Literal for v1 — the user knows their own context. Inference adds locale + translation complexity for marginal benefit. |
| Q-4 | If both `sparklineColor` is empty AND delta is absent, what color does the sparkline take? | Use the configured `--accent`. Documented behaviour. |
