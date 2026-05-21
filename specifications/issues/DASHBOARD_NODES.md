# Issue: Dashboard Nodes — Real-time dashboards built from the flow, rendered with Apache ECharts

## Status: Proposed

## Locked decisions (resolved before implementation)

This section records the decisions made after the implementation-plan
review of the spec. They override any conflicting text further down.

### Architecture corrections

1. **No `flow.InitContext` accessor.** Dependency injection follows the
   existing **Provider interface + setter** pattern (`ConfigProvider`,
   `HTTPMuxProvider`, `SessionRegistryProvider`, `ErrorProvider`, …).
   A new `flow.DashboardHub` interface and `flow.DashboardHubProvider`
   live in `internal/flow/dashboard.go`; the engine grows
   `SetDashboardHub(hub flow.DashboardHub)` mirroring
   `SetHTTPMuxBuilder` (`internal/flow/engine.go:208`) and injects the
   hub in `wireAllNodes` (`engine.go:607–726`).

2. **`ui-base` / `ui-page` / `ui-group` / `ui-spacer` are config nodes**
   (`flow.ConfigInstance`, `internal/flow/config_instance.go:16`), not
   regular flow nodes. They have no wires by construction, persist in
   `workspace.json.Configs`, and are resolved via `ConfigLookupFunc`.
   The `group` field on widget nodes is a config-ID string;
   `UIGroupConfig.vue` includes a child editor for spacers (since
   spacers are layout-only). The Phase-1 spec section that describes
   them as "drag onto canvas" is superseded — only widgets land on the
   canvas.

3. **Two `embed.FS`, two Vite builds, one `npm run build`.** Editor
   stays in `web/dist/` (existing `//go:embed dist/*` at
   `web/embed.go:23`); dashboard lands in `web/dist-dashboard/` behind
   a sibling `//go:embed dist-dashboard/*` + `GetDashboardFS()`.
   `frontend/vite.dashboard.config.ts` is a separate config; the
   `build` npm script chains `build:editor && build:dashboard`. Both
   builds keep `emptyOutDir: true` safely.

### Open-question resolutions

| Question | Decision |
|---|---|
| ECharts bundle delivery | Tree-shake via a central `frontend/dashboard/src/echarts.ts` that calls `echarts.use([...])` for every chart type the build can produce. Phase 1 registers `LineChart, BarChart, ScatterChart, PieChart, RadarChart, HeatmapChart, GaugeChart` even though only `ui-chart`+`ui-gauge` use them in Phase 1 — saves a Phase-2 schema change. Spike measurement before PR 4 confirms the < 600 KB gzip budget. |
| Auth default | `session`. Operators who need kiosk access set `auth=none` explicitly; the editor surfaces a warning at deploy time. |
| Multi-dashboard | One `ui-base` per deployment in Phase 1. Multi-dashboard adds path-routing complexity and is deferred. |
| Canvas mini-preview for `ui-chart` | Skip the sparkline in Phase 1 — render a static "chart" icon in the node body. Live sparkline is Phase 2+ once viewport-visibility plumbing is built. |
| Input passthrough loops | Document the `change`-node pattern in the widget docs. No cycle detection inside the widget. |
| Last-value cache cap | Hard cap **10 000 points OR 5 MB JSON-encoded per widget**, enforced inside `Cache.PutChartPoint`. Deploy-time warning when configured window would exceed the cap (added in PR 6). |
| WebSocket reconnect | Exponential backoff 250 ms → 8 s with a "reconnecting…" banner shown after 2 s of disconnect. |
| `ui-base` placement | Header-launched config panel (analogue to "Users"/"Certs"). Not drag-droppable. |
| Widget name vs. dashboard label | One `name` field. Each widget additionally has an optional `label` override. |
| `ui-base.path` value | Hard-coded `/dashboard/*` mount path in Phase 1. The `path` field is stored but only `/dashboard` is honored; any other value emits a deploy warning "Phase 2 only". Avoids double-touching the static handler and the SPA's `<base href>` simultaneously. |
| Widget sizing | **12-column grid, fixed 50 px row units (one "unit" = 50 px)**. `width` ∈ {0, 1…12} — `0` resolves to a full row (12 cols), `1–12` is explicit. `height` ∈ {1…12} — number of 50 px row units to span. **No "auto" height.** Each widget type has a sensible default (button/text/led: `h=1`; gauge/chart: `h=4`), seeded at create time by the TypeInfo Defaults and used as the fallback if a config omits or zeroes `height`. Long widget content scrolls inside the card; charts and gauges fit their canvas to the widget's pixel area via ResizeObserver. Layout View and Dashboard SPA render with the **same** 50 px row unit — they are byte-identical layouts of the same data. |
| Widget positioning | **Explicit (x, y) grid coordinates**, not auto-flow. Both widgets and groups carry `x` (column index 0–11) and `y` (row index ≥ 0) alongside `width`/`height`. CSS Grid renders via `grid-column: x+1 / span w`, `grid-row: y+1 / span h`. Auto-flow / `order` is **deprecated** — `order` is still read for legacy workspaces but ignored once `x`/`y` are present. |
| Collision handling | **Push-down** (Grafana / gridstack.js style). When a drag/resize moves a widget into a slot that overlaps another widget, the overlapped widget(s) get their `y` increased until no overlap remains; cascades down. The just-moved widget is the anchor and stays put. Push-down runs in `useDashboardLayout` after every position write and writes back any displaced widgets through the same `flowStore` API. Groups follow the same rule at page level. |
| Position migration | Old workspaces have `order` but no `x`/`y`. Backend `BuildLayout` synthesises `(x=0, y=cumulative_height_of_lower_order_siblings)` when `x`/`y` are missing — old layouts stack vertically full-width, which is what the old auto-flow produced for the most common case (single-column groups). Partial-width widgets land at `x=0` and the user repositions them on first edit. The synthesised values are NOT written back; they are recomputed deterministically on every render. The first drag/resize writes explicit `x`/`y` and pins the widget. |
| `cast` heuristics for `null` (parser-related, kept for symmetry with PARSER_CSV decisions) | n/a — not applicable to dashboard. |

### Build & runtime contracts

- **Layout source of truth**: `dashboard.Hub` stores a
  `*flow.LayoutSnapshot` computed by `dashboard.BuildLayout(workspace)`
  on every successful `engine.Deploy` (via a `DeployListener`
  callback set on the engine, mirroring `SetHTTPMuxBuilder`). Both
  `GET /api/dashboard/layout` and the WS `snapshot` handshake reply
  read from this single snapshot.
- **Hot-deploy signal**: the dashboard hub broadcasts its own
  `{type:"deploy", layoutChanged: bool}` event computed from
  `reflect.DeepEqual(oldSnapshot, newSnapshot)`. The editor's
  `ws.EventDeploy` is not forwarded.
- **Auth**: dashboard WS auth reuses `Server.wsAuthFunc()`
  (`internal/server/server.go:340`) with a wrapper that returns a
  synthetic anon ID when the current `ui-base.auth == "none"`.

## Context

LOOPZE turns sensor/PLC/MQTT/HTTP data into actionable streams. The
missing half of the picture is **showing** that data — line charts of
the last hour of tank pressure, a gauge for current throughput, a button
to acknowledge an alarm, a numeric input that re-targets a setpoint.
Today every LOOPZE deployment that needs a UI bolts on a separate Grafana
or a one-off Vue app, and bidirectionality (button → flow) drops out
entirely.

Node-RED solves this with **Dashboard 2.0** (`@flowfuse/node-red-dashboard`):
a set of nodes that the flow author drags onto the canvas to declare
pages, groups, charts, inputs, and outputs. The widgets are auto-served
under a separate URL path and stay in sync over WebSocket — both
directions: incoming `msg.payload` updates the widget, user interaction
emits an outgoing `msg.payload` from the widget node.

This issue specifies the LOOPZE equivalent: a **`ui-*` node family**
that turns the flow itself into the dashboard's source of truth, served
at `/dashboard/...`, with **Apache ECharts** as the sole visualization
engine for chart-shaped widgets.

## Problem description

A typical LOOPZE flow today ends with a `debug` node, a `file-out`, an
`mqtt-out`, or an `http-out`. None of these let a non-developer operator
see the data live. Conversely, **no** node today allows the flow to
**receive** an interactive input from a human (button press, set-point
slider, dropdown choice) — operator interaction must be tunnelled
through HTTP-In + a separately written page.

A dashboard node family fixes both directions of the gap:

- **Out** — `[mqtt-in] → [ui-chart]` renders a live line chart of the
  topic; `[change: msg.payload = {value: 42}] → [ui-gauge]` updates a
  gauge.
- **In** — `[ui-button] → [function]` fires a message when the operator
  clicks; `[ui-slider] → [opcua-out]` writes a setpoint to the PLC.

The dashboard URL is multi-client safe: any number of browsers can open
the same dashboard, and all of them see the same live data and the same
input events.

## View / rationale

- **Flow is the source of truth.** Pages, groups, widgets, and their
  bindings are declared by dragging nodes onto the LOOPZE canvas — no
  separate dashboard-config UI. Same mental model the operator already
  knows.
- **Apache ECharts only.** One charting engine, deeply integrated.
  ECharts handles line/bar/area/scatter/pie/polar/radar/gauge/heatmap,
  is performant at high update rates, runs offline, and is permissively
  licensed (Apache 2.0). We do not add a second charting layer.
- **Real-time, bidirectional, multi-client.** Push via WebSocket, with
  per-widget last-value caching so a freshly opened browser shows the
  current state instantly without having to wait for the next update.
- **Operator-first**: every widget has sane defaults, opens in
  dark-theme that matches LOOPZE, and respects per-page responsive
  layout without the operator having to think in CSS.
- **Node-RED Dashboard 2 feature parity in scope** for the common 90 %
  of widgets. The 10 % that do not fit cleanly (custom layouts,
  third-party widget plugins, ui-template arbitrary HTML) are explicit
  Phase 4+ items, not Phase 1.

## Functional scope reference

For each in-scope widget the following table maps the LOOPZE node type
to the equivalent Node-RED Dashboard 2 node. The Node-RED column is for
**operator-side feature reference**; the implementation is independent
and uses Apache ECharts wherever a chart is involved.

| LOOPZE type | Node-RED D2 equivalent | Category | Phase |
|---|---|---|---|
| `ui-base` | `ui-base` (global config) | dashboard-config | 1 |
| `ui-page` | `ui-page` | dashboard-config | 1 |
| `ui-group` | `ui-group` | dashboard-config | 1 |
| `ui-spacer` | `ui-spacer` | dashboard | 1 |
| `ui-text` | `ui-text` | dashboard-display | 1 |
| `ui-button` | `ui-button` | dashboard-input | 1 |
| `ui-chart` | `ui-chart` (line/bar/scatter/pie/polar/radar) | dashboard-display | 1 |
| `ui-gauge` | `ui-gauge` | dashboard-display | 1 |
| `ui-led` | `ui-led` | dashboard-display | 1 |
| `ui-numerical-input` | `ui-numerical-input` | dashboard-input | 2 |
| `ui-text-input` | `ui-text-input` | dashboard-input | 2 |
| `ui-switch` | `ui-switch` | dashboard-input | 2 |
| `ui-slider` | `ui-slider` | dashboard-input | 2 |
| `ui-dropdown` | `ui-dropdown` | dashboard-input | 2 |
| `ui-radio-group` | `ui-radio-group` | dashboard-input | 2 |
| `ui-notification` | `ui-notification` | dashboard-display | 2 |
| `ui-table` | `ui-table` | dashboard-display | 2 |
| `ui-form` | `ui-form` | dashboard-input | 3 |
| `ui-markdown` | `ui-markdown` | dashboard-display | 3 |
| `ui-template` | `ui-template` (Vue/HTML) | dashboard-template | 4 |
| `ui-control` | `ui-control` | dashboard-input | 4 |
| `ui-event` | `ui-event` | dashboard-input | 4 |
| `ui-iframe` | `ui-iframe` | dashboard-display | 4 |
| `ui-file-input` | `ui-file-input` | dashboard-input | 4 |

Everything is grouped by **page**; pages are grouped under a single
**base** (per deployment), which carries the global theme, title, and
mount path.

## Architecture overview

### Three layers

```
+--------------------------+
| Browser (dashboard SPA)  |  /dashboard/*  — Vue 3 + ECharts
|  - Renders widgets       |
|  - WS client (subscribe) |
|  - Emits user events     |
+------------+-------------+
             | WebSocket  /api/dashboard/ws
+------------v-------------+
|  Dashboard Hub (backend) |  internal/dashboard
|  - Widget registry       |
|  - Last-value cache      |
|  - Auth check            |
|  - Topic fan-out         |
+------------+-------------+
             | flow.Message
+------------v-------------+
|  LOOPZE flow runtime     |  internal/nodes/dashboard
|  ui-* node implementations|
+--------------------------+
```

### Single-process design

The dashboard frontend is a **second SPA bundle** inside the existing
LOOPZE binary, served at `/dashboard/*`. It is **not** a separate
deployable. It shares the auth backend, the WebSocket-upgrade plumbing,
and the configuration store with the editor.

The editor (`/`) and the dashboard (`/dashboard/...`) are independent
Vue apps to keep their bundle sizes apart — the dashboard does not need
Vue Flow, Monaco, etc.; the editor does not need ECharts (~1 MB
unminified). Both apps live in `frontend/` with separate Vite entry
points.

### Why a second SPA, not a route

Bundle size, dependency isolation, and operator vs. author audience.
Operators open the dashboard from a kiosk, mobile, or shop-floor PC;
they do not need the 5 MB editor bundle. The editor is opened by flow
authors during deploy/debug only.

### Why ECharts specifically

| Requirement | ECharts |
|---|---|
| Line / bar / area / scatter / pie / polar / radar / gauge / heatmap | All native |
| Real-time append + sliding window | `setOption({series:[{data}]}, true)` is the official path; ~5 k pts at 60 fps |
| Tooltip / zoom / pan | First-class |
| Offline / no CDN | Yes, vendored as `echarts/core` + per-chart imports for tree-shaking |
| License | Apache 2.0 (compatible with LOOPZE's AGPL) |
| Vue wrapper | `vue-echarts` exists but is not required — direct `init()` keeps the dep surface small |
| Dark theme | Built-in plus full custom-theme support |

Other engines (Chart.js, Recharts, Plotly) were considered:
- **Chart.js** — smaller bundle but no polar/radar/gauge out of the box;
  worse at 5 k+ live points.
- **Plotly** — better stats/3D but ~3× the bundle size; license
  ambiguity for the `dash` extensions.
- **Recharts** — React-only.

ECharts is the right anchor for "industrial-style operator dashboards".

## Requirements

### Layer 1 — Configuration nodes

#### `ui-base` (singleton per deployment, dashboard-config category)

Global dashboard configuration. There is exactly **one** `ui-base` per
LOOPZE instance — multiple `ui-base` nodes are a validation error at
deploy time. The base does not have inputs or outputs and does not
appear in flows as a regular node; it lives in a dedicated "Dashboard"
config panel reachable from the global header (analogous to the future
"Themes" or existing "Users" panels). It is, however, persisted as a
node in `workspace.json` like any other node for transport reasons.

| Field | Description | Default |
|---|---|---|
| `name` | Dashboard display title (browser tab + header bar). | `"LOOPZE Dashboard"` |
| `path` | URL prefix under which the dashboard is served. Must start with `/`. | `/dashboard` |
| `theme` | `dark` / `light` / `custom`. Custom requires a theme JSON pasted in the editor. | `dark` |
| `accentColor` | Primary chart/UI accent. Hex. | `#58a6ff` |
| `auth` | `none` / `session`. `session` requires the user to be logged in via the editor's auth. | `session` |
| `showNav` | Show side navigation listing all pages. | `true` |
| `density` | `compact` / `default` / `comfortable` — sets widget padding. | `default` |

`auth=none` is allowed and **explicit** — for shop-floor kiosks. A
warning is shown in the editor when set.

#### `ui-page` (dashboard-config category)

| Field | Description | Default |
|---|---|---|
| `name` | Page display name. | `"Page 1"` |
| `path` | URL segment under the base path. URL-safe slug. | derived from name |
| `icon` | Optional lucide-style icon name shown in the nav. | `""` |
| `layout` | `grid` (responsive 12-col) / `flex` (free) / `tabs` (groups become tabs). | `grid` |
| `order` | Sort key in the nav. | `0` |

Multiple pages are allowed. Pages are served at `<base.path>/<page.path>`.

Page nodes have **no input or output handles** in the flow — they
contribute structure only.

#### `ui-group` (dashboard-config category)

| Field | Description | Default |
|---|---|---|
| `name` | Group label (rendered as group title). | `"Group 1"` |
| `page` | The `ui-page` this group belongs to. Drop-down of all `ui-page` nodes. | _(required)_ |
| `width` | 1–12 grid columns. Only meaningful in `grid` layout. | `6` |
| `collapsible` | Allow the user to collapse this group. | `false` |
| `order` | Sort order within the page. | `0` |

Like pages, groups carry no flow handles.

#### `ui-spacer`

A vertical spacer between groups or widgets. Field: `height` (1–10).
Useful for grid layouts where visual breathing room matters. No flow
handles.

### Layer 2 — Display widgets (Phase 1)

All display widgets share:

- **One input, no output** (unless explicitly stated).
- A required `group` field — the `ui-group` this widget belongs to,
  same UX as `ui-group.page`.
- An `order` field for placement within the group.
- **Size** — both axes are integer grid units:
  - `width` ∈ {0, 1…12} columns of the group's grid (`0` = full row).
  - `height` ∈ {1…12} row units of **50 px** each.

  Each widget type has a sensible default (button/text/led: `h=1` ≈
  50 px ; gauge/chart: `h=4` ≈ 200 px). When a config carries
  `height=0` or omits the field, the renderer falls back to the
  type's default — this keeps old workspaces forward-compatible and
  avoids a "0 vs 1?" UX question for the operator.

  Content that doesn't fit the chosen cell scrolls inside the widget
  card; charts and gauges fit their canvas to the available pixel
  area via ResizeObserver. **Layout View (editor) and Dashboard SPA
  (`/dashboard/`) use the same 50 px row unit** so the same data
  renders byte-identically in both places.
- A `name`/`label` field shown above or inside the widget.
- A `property` field naming the `msg` property to read (default
  `payload`), same convention as `debug`/`change`/`csv`.
- A `tooltip` field (optional, shown on hover).
- Live-state behavior: when a new browser connects, the **last value**
  the widget received is replayed to it immediately (so the dashboard
  is never blank after a refresh).

#### `ui-text`

Displays a single value as plain text or a simple label/value pair.

| Field | Description | Default |
|---|---|---|
| `label` | Static label rendered next to the value. | `""` |
| `layout` | `row-left` / `row-right` / `row-center` / `row-spread` / `col-center` — where the label sits relative to the value. | `row-spread` |
| `format` | `text` / `number` / `json`. `number` honors `decimals`. `json` pretty-prints objects. | `text` |
| `decimals` | Decimal places when `format=number`. | `2` |
| `unit` | Suffix string appended to the value (e.g. `"°C"`). | `""` |
| `color` | Optional accent color for the value. | `""` |

#### `ui-chart`

The flagship widget. Renders an Apache ECharts instance of the
configured type, fed by incoming messages.

| Field | Description | Default |
|---|---|---|
| `chartType` | `line` / `bar` / `area` / `scatter` / `pie` / `polar` / `radar` / `heatmap`. | `line` |
| `xAxis` | `time` / `category` / `linear`. Drives ECharts' xAxis type. | `time` |
| `yAxis` | `linear` / `log`. | `linear` |
| `yMin` / `yMax` | Fixed y-axis bounds. Empty = auto. | `""` |
| `series` | Array of `{ key, label, color }`. Each `key` is a property name read from the **incoming object** (when payload is an object), or `0`/`1`/`2` for tuple-shaped payloads. | one default series |
| `windowSize` | Sliding window: keep only the last N data points. `0` = unlimited (capped at 10 k for safety). | `100` |
| `windowDuration` | Sliding window by time: keep only the last `<duration>` worth of data. Mutually exclusive with `windowSize`. Format: `30s`, `5m`, `1h`. | `""` |
| `aggregation` | `none` / `avg` / `sum` / `min` / `max` / `last` — applied per bucket of `bucketSize`. | `none` |
| `bucketSize` | Bucket width for aggregation. | `""` |
| `legend` | Show ECharts legend. | `true` |
| `tooltip` | Show ECharts tooltip on hover. | `true` |
| `animation` | Enable ECharts entry/update animation. Disable for high update rates. | `false` |
| `interpolation` | For `line`/`area`: `linear` / `smooth` / `step-before` / `step-after`. | `linear` |
| `stack` | Stack series (for `bar`/`area`). | `false` |

##### Update protocol

The chart accepts these incoming `msg.payload` shapes:

| Shape | Effect |
|---|---|
| Scalar (`number`) | Append a point `{x: now, y: payload}` to the default series. |
| `{x, y}` | Append a point to the default series. |
| `{x, y, series}` | Append to the named series. |
| `[{x, y, series?}, ...]` | Append multiple points (one ECharts update). |
| `{action: "set", data: [...]}` | Replace the entire chart data. `data` is series-shaped. |
| `{action: "clear"}` | Empty the chart. |
| `{action: "remove", series: "x"}` | Drop a single series. |
| `null` / `undefined` payload | Ignored — keeps the chart stable through propagation. |

Same payload shape across all chart types; `chartType=pie` interprets
`{label, value}` instead of `{x, y}` (documented in-product).

##### Performance

- Append-only updates use ECharts' `appendData` API where supported
  (line/scatter/bar/heatmap); other types use `setOption(..., {lazyUpdate:
  true})`.
- The dashboard SPA debounces visible-frame updates to the next
  `requestAnimationFrame`, so a burst of 100 messages in 16 ms results
  in one repaint.
- Windowing happens **server-side** in the hub's last-value cache (so a
  late-arriving client gets a windowed dataset, not 10 k stale points),
  and **client-side** for live updates after the initial replay.

#### `ui-gauge`

ECharts gauge widget. Shows a single numeric value within configured
bounds.

| Field | Description | Default |
|---|---|---|
| `min` / `max` | Gauge range. | `0` / `100` |
| `unit` | Suffix. | `""` |
| `decimals` | Decimal places shown. | `1` |
| `style` | `arc` / `full` / `dial` — ECharts gauge variants. | `arc` |
| `thresholds` | Array of `{value, color}` segments. | empty (single color) |
| `showValue` | Show the numeric value inside the gauge. | `true` |

Payload: scalar number, or `{value}`.

#### `ui-led`

A status indicator: a colored circle plus an optional label.

| Field | Description | Default |
|---|---|---|
| `states` | Array of `{when, color, label?}` rules. Each `when` is a JS-style equality (`true`, `"alarm"`, `> 30`). The first matching rule wins; non-matching → `off`. | sensible default (true=green/false=gray) |
| `offColor` | Color when no rule matches. | `#444` |
| `glow` | Add a soft halo. | `false` |

Payload: any value evaluated against the rules.

### Layer 3 — Input widgets (Phase 1 subset, Phase 2 remainder)

All input widgets share:

- **No input, one output**.
- Same `group` / `order` / `width` / `height` / `label` / `tooltip`
  fields — `width` and `height` follow the display-widget contract
  above and must be applied to the rendered widget element, not just
  persisted.
- A `topic` field whose value becomes `msg.topic` on emitted messages
  (so multiple widgets can fan in to one `function` node and be
  distinguished). Default: empty.
- All widget interactions emit a `msg` that includes:
  - `msg.payload` — the new value / event.
  - `msg.topic` — the configured topic.
  - `msg._client` — `{user?, sessionId, socketId}` describing which
    browser caused this. Useful for audit logs and per-user gating.
  - `msg._widget` — `{nodeId, type}` for debugging.

#### `ui-button` (Phase 1)

| Field | Description | Default |
|---|---|---|
| `label` | Button text. Supports `{{mustache}}` against the last received `msg`. | `"Click me"` |
| `payload` | Static payload to emit on click. Empty = `true`. | `""` |
| `payloadType` | `bool` / `string` / `number` / `json` / `timestamp` / `flow` / `msg`. | `bool` |
| `color` | Button color (background). | accent |
| `icon` | Optional icon name. | `""` |
| `confirm` | Show a confirm dialog before emitting. | `false` |
| `confirmText` | Dialog body text. | `"Confirm?"` |

`payloadType=msg` passes the incoming `msg` along; useful only if the
button receives messages on a separate input (Phase 4 — currently
inputs are not modelled on input widgets).

#### `ui-numerical-input` (Phase 2)

| Field | Description | Default |
|---|---|---|
| `min` / `max` / `step` | Numeric bounds. | `0` / `100` / `1` |
| `unit` | Display suffix in the input. | `""` |
| `passthrough` | Echo external `msg.payload` into the field. | `true` |
| `emit` | `change` / `enter` / `debounced` (300 ms). | `debounced` |

#### `ui-text-input` (Phase 2)

| Field | Description | Default |
|---|---|---|
| `maxLength` | Char limit. `0` = unlimited. | `0` |
| `passthrough` | Echo external `msg.payload` into the field. | `true` |
| `emit` | `change` / `enter` / `debounced`. | `enter` |
| `multiline` | Render a textarea. | `false` |

#### `ui-switch` (Phase 2)

| Field | Description | Default |
|---|---|---|
| `onPayload` / `offPayload` | Payloads emitted when toggled. | `true` / `false` |
| `onLabel` / `offLabel` | Labels in the two states. | `"ON"` / `"OFF"` |
| `passthrough` | Sync external state into the switch (when an incoming msg arrives the switch position updates without emitting). | `true` |

#### `ui-slider` (Phase 2)

| Field | Description | Default |
|---|---|---|
| `min` / `max` / `step` | Bounds. | `0` / `100` / `1` |
| `orientation` | `horizontal` / `vertical`. | `horizontal` |
| `showValue` | Show the numeric label. | `true` |
| `emit` | `change` / `release` / `debounced`. | `release` |

#### `ui-dropdown` (Phase 2)

| Field | Description | Default |
|---|---|---|
| `options` | Array of `{label, value}`. | `[]` |
| `multiple` | Allow multi-select; payload becomes an array. | `false` |
| `placeholder` | Empty-state text. | `"Select…"` |

#### `ui-radio-group` (Phase 2)

Same as dropdown but rendered as radio buttons / pill toggles.

### Layer 4 — Other Phase 2/3 widgets

#### `ui-notification` (Phase 2)

Pop-up toast on incoming messages. No input visualization in the
dashboard layout — the widget is invisible until a message fires.

| Field | Description | Default |
|---|---|---|
| `severity` | `info` / `success` / `warn` / `error` / property of `msg`. | `info` |
| `position` | `top-right` / `top-center` / `bottom-right` / `bottom-center`. | `top-right` |
| `duration` | Auto-dismiss after N ms. `0` = sticky. | `4000` |
| `scope` | `all` (all clients see it) / `caller` (only the client whose action triggered the upstream — requires `msg._client`). | `all` |

#### `ui-table` (Phase 2)

Tabular view of an array payload. Columns derived from object keys or
explicit `columns` config. Built on a thin custom virtual-scroll table
(no third-party data grid in Phase 2).

#### `ui-form` (Phase 3)

A grouped collection of inputs that emits a single composite message
on submit. Useful for "set parameter set" workflows. Each field is
declared as one of the supported input widget types and stays internal
to the form (no separate flow nodes per field). The submit button emits
`msg.payload = { fieldName: value, ... }`.

#### `ui-markdown` (Phase 3)

Static or template-driven markdown. Mustache against the last received
`msg`. Renderer: a small sanitized-markdown lib (e.g.
`micromark` minimal preset). No raw HTML.

#### `ui-template` (Phase 4)

Embed an arbitrary Vue/HTML snippet with access to incoming `msg`
data and the ability to emit messages. Sandboxed via Vue's compiler
(no `eval`, no external network). Phase 4 because the security and
sandboxing requirements are substantial.

#### `ui-iframe`, `ui-control`, `ui-event`, `ui-file-input` (Phase 4)

Stubbed for completeness — implementations are deferred until Phase
1–3 land and concrete use cases drive their shape.

## Editor-side Layout View

### Status: Proposed (Phase 1.5 — follows PR 3, precedes Phase 2)

### Motivation

Configuring widget size (`width`/`height`) and `order` via NumberInputs
on the property panel is correct but tedious: the operator changes a
number, redeploys, opens a separate dashboard tab, sees the result,
flips back, repeats. The dashboard SPA itself cannot host an editor
without breaking "flow is the source of truth", introducing a second
auth model, and bloating the kiosk bundle.

The **Layout View** is a new tab inside the existing flow editor that
renders the deployed dashboard as a drag/resize-able grid and writes
changes back to the **same flow-node configs** the property panel
already edits. No new persistence, no new auth, no new deploy concept.

### UX

- A "Layout" entry sits next to each `ui-page` in the FlowTabBar (or a
  dedicated "Dashboard" tab listing all pages — TBD at impl time;
  per-page tabs match the existing flow-tab pattern more naturally).
- Inside the tab, each `ui-group` renders as a 12-column CSS grid
  identical to the live dashboard, occupied by widget placeholders that
  show the widget's name, type icon, and category accent.
- **Drag** a widget within a group → updates `order` (and reassigns
  group membership if dragged into a different group).
- **Resize handles** on the SE corner of every widget → updates
  `width`/`height`. Snaps to grid tracks.
- **Double-click** a widget → opens the existing property panel
  config for that node (same component as on the canvas). Saves
  propagate the same way they do from the flow canvas.
- Page-level drag-to-reorder of groups → updates `order` on each
  `ui-group` config.
- The Layout View is read-only when the workspace is dirty in a way
  the layout cannot represent (e.g. an undeployed wire change in the
  Flow tab); a banner offers "Deploy first to enable layout edit".

### Data flow

Every drag / resize / drop is a single `flowStore.updateNodeData(...)`
or `flowStore.updateConfig(...)` call against the **already-deployed
flow node** or `ui-group` config. The workspace is marked dirty, the
operator clicks the existing Deploy button, the engine's normal deploy
path runs, the dashboard hub's `RebuildLayout` fires, the live
`/dashboard/*` SPA receives the new layout.

This means the Layout View is **fundamentally a different rendering of
the same data the canvas already manages** — no parallel state, no
sync logic, no extra round-trips.

### Out of scope

- **Cross-flow widget moves** — widgets can only be reordered/resized
  within their flow. Moving a widget to another flow is a canvas
  operation.
- **In-dashboard editing** (Edit Mode in the live `/dashboard/*` SPA).
  Deferred indefinitely: the live dashboard stays read-only and
  kiosk-safe. If Phase 3 ACLs introduce per-role write permissions,
  reconsider then.
- **New widget creation from the Layout View** — that's a canvas
  operation. The Layout View only arranges what's already there.
- **Live preview during drag** — the SPA at `/dashboard/*` does not
  see uncommitted drag positions; only after Deploy. Live preview
  would require a separate uncommitted-state channel that is not
  worth the complexity.
- **Per-widget snap toggles, free-form positioning, z-order, overlap
  detection** — the grid is the contract.

### Affected files (sketch)

#### New
- `frontend/src/views/DashboardLayoutView.vue` — the routable view
  (or component, mounted alongside `FlowEditor.vue`)
- `frontend/src/components/layout/LayoutGrid.vue` — 12-col grid
  renderer with drop zones
- `frontend/src/components/layout/LayoutWidgetCard.vue` — placeholder
  card for a widget (name, icon, resize handle, drag handle)
- `frontend/src/composables/useDragResize.ts` — pointer-event-based
  drag and resize, no external library (Vue Flow already proves
  this is tractable in this codebase)

#### Changed
- `frontend/src/components/FlowTabBar.vue` — surface the Layout
  entry per `ui-page`, or a single "Dashboard" tab
- `frontend/src/router/index.ts` — new route if mounted as its own
  view
- No backend changes — the existing API endpoints suffice

### Risks / open questions

- **Drag-resize without a library**: pointer events, grid snapping,
  and a clean keyboard a11y story are substantial work (~500 LOC).
  Worth measuring the alternative (a small library like `interactjs`)
  before committing to in-house code. Current preference: in-house,
  consistent with the rest of the codebase, but revisit after a spike.
- **Mobile / touch**: drag-resize on touch screens is harder than
  with a mouse. Phase 1.5 targets desktop authoring; mobile authoring
  is explicit non-goal.
- **Read-only mode trigger**: what exactly counts as "dirty in a way
  the layout cannot represent"? Suggestion: only allow layout edits
  when the per-widget configs and `ui-group`/`ui-page` configs match
  the last deployed state. Other dirty changes (wires, code, etc.)
  block layout edit until Deploy.
- **Empty groups**: drop zones for empty groups need a placeholder
  hint. Same for pages with no groups.

### Verification

- Open the Layout tab → widgets appear in the same positions as the
  live dashboard
- Drag widget A right by one column → property panel of A would now
  show `width` and `order` reflecting the new placement
- Click Deploy → live dashboard updates
- Double-click widget → property panel opens for that node (same
  component as from canvas double-click)
- Undeployed wire change in Flow tab → Layout tab shows banner,
  cannot drag

---

## Real-time wire protocol

### WebSocket

URL: `/api/dashboard/ws` (separate from the editor's `/api/ws` so the
auth, rate-limit, and origin policies can differ).

Same `gorilla/websocket` upgrader and same origin-allowlist mechanism
as the editor hub (see `internal/ws/hub.go`).

Handshake: client sends `{type:"hello", dashboardId, pageId?}`; hub
replies with a `snapshot` containing the last cached value of every
widget on that page (or all pages, if `pageId` is omitted).

### Messages

**Server → client** (widget updates):

```json
{
  "type": "widget",
  "id": "<widget nodeId>",
  "value": <last value sent into the widget node>,
  "ts": 1747657892341
}
```

**Server → client** (snapshot):

```json
{
  "type": "snapshot",
  "widgets": {
    "<widget1Id>": { "value": ..., "ts": ... },
    "<widget2Id>": { "value": ..., "ts": ... }
  }
}
```

**Client → server** (user interaction):

```json
{
  "type": "event",
  "id": "<input widget nodeId>",
  "value": <new value>
}
```

The hub validates the client is authorized to address `id` (the widget
belongs to a page the client is currently viewing), then injects the
event into the flow via the registered input widget node, which emits
the configured payload on its output.

### Last-value cache

The hub stores, in memory, `widgetID → {value, ts}` for every widget
that has ever emitted. This is the snapshot replayed to fresh clients.

- Capped per widget at one entry (latest only); not a history.
- Chart widgets store **the window** (server-side enforcement of
  `windowSize` / `windowDuration`), not the latest point, so a new
  client sees the full history immediately.
- Cleared on deploy when a widget's `nodeId` is removed.
- Optionally persisted to disk per `ui-base.persistCache=true` config
  (Phase 3) so dashboards survive a restart with the last values
  visible. Phase 1: in-memory only.

### Auth

When `ui-base.auth=session`, the WebSocket upgrade requires the same
session cookie used by the editor (set on login via
`internal/auth/`). When `auth=none`, the upgrade is open but the editor
shows an explicit warning at deploy time.

Per-user authorization (page-level ACLs, button gating by role) is
Phase 3 — Phase 1 is "logged in = full access".

## Deploy & lifecycle

### Validation at deploy

- Exactly one `ui-base` node. Zero → no dashboard mount. Two or more →
  deploy error.
- Every `ui-page` must reference a valid `ui-base` (implicit: there is
  only one).
- Every `ui-group` must reference a valid `ui-page`.
- Every widget must reference a valid `ui-group`.
- Widget paths derived from `ui-page.path` slugs must not collide.

Validation errors surface on the affected nodes with the standard
red-status mechanism plus a deploy-blocking entry in the deploy log.

### Hot-deploy

When a flow is re-deployed:

- The dashboard SPA receives a `{type: "deploy", layoutChanged: bool}`
  message.
- If layout did not change (only flow logic), connected browsers keep
  their current view and continue receiving updates.
- If layout did change, the SPA reloads its layout from
  `/api/dashboard/layout` (a small JSON describing pages/groups/widgets)
  and re-renders without dropping the WebSocket.

This avoids the "full page refresh on every deploy" experience and
makes iterative dashboard authoring tolerable.

### Removal

When a widget node is removed from the flow:

- Its last-value cache entry is dropped.
- Any client currently rendering it removes the widget from the DOM
  on the next layout sync.

## Examples

### Example 1 — Live tank-pressure line chart

```
[mqtt-in: topic=plant/tank/pressure]
  →  [change: msg.payload = Number(msg.payload)]
  →  [ui-chart: chartType=line, xAxis=time, windowDuration=10m]
```

Operator opens `/dashboard/overview` and sees the last 10 minutes of
pressure samples; new samples slide in from the right. A second
operator opens the same page on a tablet — they see the same chart,
including the 10-min history, populated from the server-side window
cache on connect.

### Example 2 — Acknowledge an alarm

```
[mqtt-in: topic=alarms/+]  →  [ui-led: states=[{when:true,color:red}]]

[ui-button: label="ACK", payload="ack", topic="alarms"]
  →  [mqtt-out: topic=alarms/ack]
```

A red LED lights up when an alarm topic fires; the operator clicks
ACK; the MQTT publish round-trips, the alarm topic clears, the LED
returns to off — all without a page reload.

### Example 3 — Setpoint slider to PLC

```
[ui-slider: min=0, max=100, step=1, topic="setpoint/main"]
  →  [function: msg.payload = {NodeId: "ns=2;s=Setpoint", Value: msg.payload}]
  →  [opcua-out]
```

Operator drags the slider; release emits a value; `opcua-out` writes
the setpoint to the PLC. The current value on the PLC can be fed back
into the same slider via passthrough:

```
[opcua-in: NodeId="ns=2;s=Setpoint"]  →  [ui-slider: passthrough=true, ...]
```

(For passthrough into an input widget, route the message through a
`change` node that strips the topic to avoid feedback loops.)

### Example 4 — Multi-series chart with categorical x-axis

```
[function: emits { x: "shift A", series: "throughput", y: 120 }]
[function: emits { x: "shift A", series: "rejects",    y: 4   }]
[function: emits { x: "shift B", series: "throughput", y: 137 }]
[function: emits { x: "shift B", series: "rejects",    y: 2   }]
   ↓ all into:
[ui-chart: chartType=bar, xAxis=category, stack=false,
           series=[{key:"throughput"},{key:"rejects"}]]
```

Bar chart with two series per shift, color-coded by series.

### Example 5 — Gauge with thresholds

```
[modbus-in: holding 100]
  →  [change: msg.payload = msg.payload / 10]
  →  [ui-gauge: min=0, max=10,
                thresholds=[{value:6,color:"#22c55e"},
                            {value:8,color:"#f59e0b"},
                            {value:10,color:"#ef4444"}]]
```

### Example 6 — Replace chart contents with a query result

```
[inject every 60s]
  →  [http-request: GET /api/lab/last-batch]
  →  [csv: action=parse, output=array]
  →  [function: msg.payload = { action:"set", data: msg.payload.map(...) }]
  →  [ui-chart: chartType=line]
```

A periodic refresh replaces the entire chart dataset rather than
appending — useful for batch-style data sources.

## Technical sketch

### Backend — `internal/dashboard/` (new package)

```
internal/dashboard/
  hub.go           // websocket hub for dashboard clients
  cache.go         // per-widget last-value / window cache
  layout.go        // computes the dashboard layout from the deployed flow
  registry.go      // widget-input node registry (for event dispatch)
  auth.go          // session-cookie validation for dashboard WS
```

The hub mirrors `internal/ws/hub.go` but is a **separate** instance —
different URL, different message types, different cache. Sharing the
auth helpers (`internal/auth`) and the origin checker
(`internal/ws.buildOriginChecker`) keeps the surface small.

### Backend — `internal/nodes/dashboard/` (new package)

```
internal/nodes/dashboard/
  init.go              // node registry entries for all ui-* types
  base.go              // ui-base (singleton config)
  page.go              // ui-page (layout container, no IO)
  group.go             // ui-group
  spacer.go            // ui-spacer
  text.go              // ui-text  (display)
  chart.go             // ui-chart (display) — feeds the hub's window cache
  gauge.go             // ui-gauge (display)
  led.go               // ui-led  (display)
  button.go            // ui-button (input)
  numerical_input.go   // ui-numerical-input (input, Phase 2)
  text_input.go        // ui-text-input (input, Phase 2)
  switch.go            // ui-switch (input, Phase 2)
  slider.go            // ui-slider (input, Phase 2)
  dropdown.go          // ui-dropdown (input, Phase 2)
  radio_group.go       // ui-radio-group (input, Phase 2)
  notification.go      // ui-notification (display, Phase 2)
  table.go             // ui-table (display, Phase 2)
  common.go            // shared helpers (cache key, hub injection)
```

Display widgets implement a thin `flow.NodeInstance`:

```go
func (n *UIChartNode) HandleMessage(msg *flow.Message) ([][]*flow.Message, error) {
    if msg == nil { return nil, nil }
    val, _ := msg.Get(n.property)
    n.hub.PushWidgetValue(n.ID(), val, time.Now())  // routes to all clients
    return nil, nil   // display widgets have no output
}
```

Input widgets register with the hub at `Init` so the hub can dispatch
incoming events:

```go
func (n *UIButtonNode) Init(ctx flow.InitContext) error {
    n.hub = ctx.DashboardHub()
    n.hub.RegisterInputWidget(n.ID(), n.onEvent)
    return nil
}

func (n *UIButtonNode) onEvent(evt dashboard.WidgetEvent) {
    msg := flow.NewMessage()
    msg.Set("payload", n.buildPayload(evt))
    msg.Set("topic", n.topic)
    msg.Set("_client", evt.Client)
    n.Send(0, msg)
}
```

`ui-base`, `ui-page`, `ui-group`, `ui-spacer` are **pure config nodes** —
they implement `flow.NodeInstance` only to participate in the lifecycle
and to expose their config to the layout builder; their `HandleMessage`
is a no-op and they declare zero inputs/outputs.

### Backend — `internal/api/dashboard_handlers.go` (new)

- `GET /api/dashboard/layout` → JSON describing pages/groups/widgets
  for the SPA bootstrap.
- `GET /api/dashboard/theme` → resolved theme config (palette,
  density, etc.).
- `GET /dashboard/ws` → upgrade to the dashboard hub.

The Vite-built dashboard SPA assets are served by the existing static
handler under the configured `ui-base.path` prefix (default
`/dashboard`).

### Frontend — `frontend/dashboard/` (new SPA)

Separate Vite entry point. Structure:

```
frontend/dashboard/
  index.html
  src/
    main.ts              // app bootstrap, router
    App.vue              // shell, nav, theme
    router.ts            // /dashboard/<pagePath>
    api.ts               // layout fetch + ws connect
    stores/
      layout.ts          // pages/groups/widgets layout from server
      widgets.ts         // per-widget reactive value map
      ws.ts              // websocket client (reconnect, snapshot replay)
    components/
      DashboardShell.vue
      PageView.vue
      GroupView.vue
      SpacerView.vue
      widgets/
        TextWidget.vue
        ChartWidget.vue   // wraps echarts/core
        GaugeWidget.vue
        LedWidget.vue
        ButtonWidget.vue
        NumericalInputWidget.vue
        TextInputWidget.vue
        SwitchWidget.vue
        SliderWidget.vue
        DropdownWidget.vue
        RadioGroupWidget.vue
        NotificationLayer.vue   // listens globally
        TableWidget.vue
```

ECharts is imported per-chart-type for tree-shaking:

```ts
// ChartWidget.vue
import * as echarts from 'echarts/core'
import { LineChart, BarChart, ScatterChart, PieChart,
         RadarChart, HeatmapChart, GaugeChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, LegendComponent,
         PolarComponent, DataZoomComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
echarts.use([LineChart, BarChart, ScatterChart, PieChart, RadarChart,
             HeatmapChart, GaugeChart, GridComponent, TooltipComponent,
             LegendComponent, PolarComponent, DataZoomComponent,
             CanvasRenderer])
```

The dashboard does **not** import Vue Flow, Monaco, or any editor-side
state — `package.json` adds `echarts: ^5.5` as its primary new
dependency. Bundle size budget: dashboard SPA < 600 KB gzip.

### Frontend — Editor config panels

For each `ui-*` type, a `frontend/src/nodes/dashboard/<Type>Config.vue`
exists, registered in a new manifest:

```ts
// frontend/src/nodes/dashboard/index.ts
export const manifest: NodeGroupManifest = {
  name: 'dashboard',
  categories: {
    'ui-base':              'dashboard-config',
    'ui-page':              'dashboard-config',
    'ui-group':             'dashboard-config',
    'ui-spacer':            'dashboard',
    'ui-text':              'dashboard-display',
    'ui-chart':             'dashboard-display',
    'ui-gauge':             'dashboard-display',
    'ui-led':               'dashboard-display',
    'ui-button':            'dashboard-input',
    // ... Phase 2 entries
  },
  flowEditors: { /* dynamic imports */ },
}
```

Two new palette categories: `dashboard-display` (cyan/teal accent),
`dashboard-input` (purple accent), plus `dashboard-config` (gray).
Adds three palette entries — see `frontend/src/nodes/index.ts`.

The `ui-base` config opens through a "Dashboard" button in the global
header (analogous to "Users" / "Certs"); it is **not** drag-droppable
onto the canvas. `ui-page`, `ui-group`, and widgets are normal nodes.

### Canvas rendering

`ui-base`, `ui-page`, `ui-group`, `ui-spacer` render as compact
**structural** nodes on the canvas — small badges with a "📋 page" /
"🗂 group" decoration (icon naming TBD; emoji-free as per project
style). They have no input/output ports.

Widgets render as standard nodes with:

- One input port (or one output port for input widgets).
- A live preview in the node body: latest value for `ui-text` /
  `ui-led`, a mini sparkline for `ui-chart`, a tiny gauge arc for
  `ui-gauge`. The preview is updated from the same flow-status WS
  stream as today's status badges — no extra plumbing.

The mini-preview is opt-out via a per-node setting (off by default for
charts at high update rates).

## Affected files

### Backend (new)
- `internal/dashboard/hub.go`
- `internal/dashboard/cache.go`
- `internal/dashboard/layout.go`
- `internal/dashboard/registry.go`
- `internal/dashboard/auth.go`
- `internal/nodes/dashboard/init.go`
- `internal/nodes/dashboard/base.go`
- `internal/nodes/dashboard/page.go`
- `internal/nodes/dashboard/group.go`
- `internal/nodes/dashboard/spacer.go`
- `internal/nodes/dashboard/text.go`
- `internal/nodes/dashboard/chart.go`
- `internal/nodes/dashboard/gauge.go`
- `internal/nodes/dashboard/led.go`
- `internal/nodes/dashboard/button.go`
- `internal/nodes/dashboard/common.go`
- `internal/api/dashboard_handlers.go`

### Backend (changed)
- `internal/api/routes.go` — register `/api/dashboard/*` and `/dashboard/*`
  routes; serve the dashboard SPA bundle.
- `internal/nodes/registry.go` — register the dashboard group.
- `internal/server/*.go` — wire the dashboard hub into the runtime so
  `flow.InitContext` exposes `DashboardHub()`.
- `internal/flow/*.go` — `InitContext` interface gains a
  `DashboardHub()` accessor.

### Frontend (new, editor side)
- `frontend/src/nodes/dashboard/index.ts` — manifest
- `frontend/src/nodes/dashboard/UIBaseConfig.vue`
- `frontend/src/nodes/dashboard/UIPageConfig.vue`
- `frontend/src/nodes/dashboard/UIGroupConfig.vue`
- `frontend/src/nodes/dashboard/UISpacerConfig.vue`
- `frontend/src/nodes/dashboard/UITextConfig.vue`
- `frontend/src/nodes/dashboard/UIChartConfig.vue`
- `frontend/src/nodes/dashboard/UIGaugeConfig.vue`
- `frontend/src/nodes/dashboard/UILedConfig.vue`
- `frontend/src/nodes/dashboard/UIButtonConfig.vue`
- `frontend/src/components/nodes/UI*Node.vue` — canvas renderings (text,
  chart preview, gauge preview, led, button, base/page/group/spacer
  badges)
- `frontend/src/components/DashboardLauncher.vue` — header button that
  opens `ui-base` config and a link to the dashboard URL.

### Frontend (new, dashboard SPA)
- `frontend/dashboard/index.html`
- `frontend/dashboard/src/main.ts`
- `frontend/dashboard/src/App.vue`
- `frontend/dashboard/src/router.ts`
- `frontend/dashboard/src/api.ts`
- `frontend/dashboard/src/stores/layout.ts`
- `frontend/dashboard/src/stores/widgets.ts`
- `frontend/dashboard/src/stores/ws.ts`
- `frontend/dashboard/src/components/DashboardShell.vue`
- `frontend/dashboard/src/components/PageView.vue`
- `frontend/dashboard/src/components/GroupView.vue`
- `frontend/dashboard/src/components/SpacerView.vue`
- `frontend/dashboard/src/components/widgets/TextWidget.vue`
- `frontend/dashboard/src/components/widgets/ChartWidget.vue`
- `frontend/dashboard/src/components/widgets/GaugeWidget.vue`
- `frontend/dashboard/src/components/widgets/LedWidget.vue`
- `frontend/dashboard/src/components/widgets/ButtonWidget.vue`

### Frontend (changed)
- `frontend/package.json` — add `echarts: ^5.5`. Possibly add
  `micromark`/`marked` for Phase 3 markdown (deferred).
- `frontend/vite.config.ts` — second `build.rollupOptions.input`
  pointing at the dashboard SPA, separate output dir served at
  `<base.path>`.
- `frontend/src/nodes/index.ts` — register the `dashboard` manifest.

## Tests

### Backend

| Test | Verifies |
|---|---|
| `TestHub_NewClient_ReceivesSnapshot` | Connecting client gets every widget's last cached value |
| `TestHub_WidgetPush_FansOutToAllClients` | Two clients on the same page receive the same update |
| `TestHub_UnauthorizedEvent_Rejected` | Client cannot send an event for a widget that does not exist |
| `TestHub_AuthSession_RejectsAnonWhenRequired` | `auth=session` blocks anonymous WS upgrade |
| `TestHub_AuthNone_AllowsAnon` | `auth=none` permits the upgrade |
| `TestCache_LastValue_Replaced` | Cache stores only the latest value (display widgets) |
| `TestCache_ChartWindow_Sized` | Chart cache enforces `windowSize` server-side |
| `TestCache_ChartWindow_Duration` | Chart cache enforces `windowDuration` server-side |
| `TestCache_ClearedOnNodeRemoval` | Removed widget purges its cache entry |
| `TestLayout_BuildFromFlow` | Layout JSON correctly groups widgets by `ui-group`/`ui-page` |
| `TestLayout_OrphanWidget_Skipped` | Widget without `ui-group` is left out with a deploy warning |
| `TestUIBase_SingletonValidation` | Two `ui-base` nodes → deploy error |
| `TestUIChart_AppendNumberPayload` | Scalar payload appends `{x: now, y: payload}` |
| `TestUIChart_AppendObjectSeries` | `{x, y, series}` routes to named series |
| `TestUIChart_ActionSet_ReplacesData` | `{action:"set", data:[...]}` replaces cache contents |
| `TestUIChart_ActionClear_EmptiesCache` | `{action:"clear"}` empties series |
| `TestUIChart_WindowSize_Trims` | After N appends, oldest points are dropped server-side |
| `TestUIChart_NullPayload_Ignored` | `null` payload is a no-op |
| `TestUIGauge_NumberPayload_PushedAsIs` | Scalar payload reaches the hub unchanged |
| `TestUILed_RuleFirstMatch_Wins` | First matching `states` rule sets the color |
| `TestUIButton_OnEvent_EmitsConfiguredPayload` | Event with `payloadType=bool` emits `true` |
| `TestUIButton_OnEvent_PassesClientInfo` | Output `msg._client` populated from the WS connection |
| `TestUIInputs_OnEvent_TopicSet` | `msg.topic` reflects the configured `topic` |
| `TestDeploy_HotSwap_LayoutChange_TriggersReload` | Layout change signals a SPA layout reload |
| `TestDeploy_HotSwap_FlowOnlyChange_NoReload` | Flow-only redeploy does not trigger a layout reload |
| `TestDashboardSPA_AssetsServed` | Static bundle served at `<base.path>` returns index.html |
| `TestRoutes_NoUIBase_NoDashboardMount` | Without `ui-base`, the `/dashboard/*` route 404s |

### Frontend — editor

| Test | Verifies |
|---|---|
| `ChartConfig_RendersSeriesEditor` | Series editor renders for the configured chart type |
| `BaseConfig_PathValidation` | Invalid path (no leading `/`) is rejected |
| `WidgetNode_LivePreview_UpdatesFromStatusStream` | Mini-preview reacts to the status WS stream |
| `DashboardLauncher_OpensConfigPanel` | Header button opens the `ui-base` panel |

### Frontend — dashboard SPA

| Test | Verifies |
|---|---|
| `LayoutStore_LoadsFromAPI` | `/api/dashboard/layout` is fetched and stored |
| `WsStore_ReconnectsAfterDrop` | WS reconnect with exponential backoff |
| `WsStore_AppliesSnapshotOnConnect` | Snapshot replay populates the widgets store |
| `ChartWidget_AppendsLineSeries` | Append message triggers ECharts `appendData` |
| `ChartWidget_RAFBatching` | Burst of messages results in one repaint |
| `ButtonWidget_Click_EmitsEvent` | Click sends an `{type:"event"}` WS message |
| `GaugeWidget_ThresholdColors` | Configured threshold ranges color the gauge |
| `Notification_Toast_Shown` | Display widget pops up the toast on incoming message |

## Dependencies

### New
- **`echarts: ^5.5`** (frontend dashboard SPA only) — Apache 2.0.
- No new Go dependencies; `gorilla/websocket` is already in use.

### Existing
- `internal/ws` (origin checker, websocket upgrader pattern — reuse,
  do not extend the editor hub).
- `internal/auth` (session validation helpers).
- `internal/flow.Message` / `flow.NodeInstance` / `flow.InitContext`.
- `internal/api/routes.go` for HTTP route mounting.
- Vite multi-entry config (Vite 6 supports this natively).

## Out of scope for Phase 1

- **Phase 2 / 3 / 4 widgets** listed in the functional-scope table.
- **Per-user / per-role ACLs on widgets and pages** — Phase 3.
- **Persistent last-value cache across restarts** — Phase 3.
- **Custom themes loaded from disk** — Phase 1 supports `dark`,
  `light`, and `custom` (paste JSON). External theme files: Phase 3.
- **Mobile-specific layouts** (different grid for mobile) — Phase 3.
- **Dashboard layout editor (drag widgets within a page in the
  dashboard itself)** — the layout is authored in the LOOPZE editor
  via node positions/order fields. A dashboard-side layout editor is
  Phase 4 if ever.
- **Plug-in widget loading** (third-party `ui-*` packages) — Phase 4+.
- **Server-side rendering / PDF export of dashboards** — out of scope
  entirely.
- **Historical-data backend** (querying old data for chart bootstrap
  beyond the in-memory window) — out of scope; users wire a periodic
  `set` message from their own historian.
- **Per-widget data persistence** — the chart window cache is RAM,
  not disk. A chart that needs 24 h of history pre-load on connect
  must be re-hydrated by an upstream node.
- **Translations / i18n** — single-language for Phase 1 (English).
- **Dashboard nodes inside subflows** — subflows are not yet a LOOPZE
  primitive; revisit when they land.

## Open questions

- **Bundle delivery for ECharts**: tree-shake per chart type (current
  proposal) versus ship the umbrella bundle. Tree-shaking saves ~400 KB
  but requires every widget to declare its `echarts.use([...])` calls
  centrally to avoid runtime "chart type not registered" surprises.
  **Suggestion**: central `dashboard/src/echarts.ts` that registers all
  chart types the build can produce; widgets just `import { echarts }`.
- **Auth default**: `session` (current proposal) versus `none`. Shop
  floor kiosks are a common case for `none`, but defaulting to open
  access on a feature that ships with the product is risky.
  **Suggestion**: keep `session` as default; document the kiosk
  workflow.
- **Multi-tenant / multi-dashboard**: only one `ui-base` per
  deployment? Or one per "workspace"/flow tab? **Suggestion**: one per
  deployment for Phase 1 — multi-dashboard adds path-routing
  complexity that is not load-bearing for early users.
- **Chart widget on the canvas — mini live sparkline**: useful for
  authors during development, but at 100 Hz update rates the canvas
  re-render cost is non-trivial. **Suggestion**: render the sparkline
  only when the node is in the current viewport AND the editor is
  focused (existing visibility plumbing for the debug stream applies).
- **Input passthrough loops**: a slider whose value also feeds back
  from the PLC creates a write→read→write cycle. **Suggestion**:
  document the pattern (strip `msg.topic` in a `change` node before
  echoing back) rather than build cycle-detection into the widget;
  the same caveat exists in Node-RED dashboard 2.
- **Last-value cache size cap**: chart windows of 10 k points × N
  widgets could grow large. **Suggestion**: hard cap per widget at
  10 k points or 5 MB JSON-encoded — whichever first — with a deploy
  warning when the configured window would exceed the cap.
- **WebSocket reconnect strategy**: exponential backoff (current
  proposal) or fixed 1 s. **Suggestion**: exponential 250 ms → 8 s,
  with a "reconnecting…" banner in the SPA after 2 s of disconnect.
- **`ui-base` placement**: header-launched config panel (current
  proposal) versus a regular but unique node on the canvas.
  **Suggestion**: header-launched — keeps the canvas clean and avoids
  the "where do I drop the base" question for new operators.
- **Widget names as canvas labels vs. dashboard labels**: should they
  diverge? **Suggestion**: one `name` field; the operator chooses
  whether it makes sense for both views. A `label` override per
  widget covers the divergent case without doubling every field.

---

# Phase 2 — Input widgets, table, notifications

## Status: Proposed

Adds `ui-numerical-input`, `ui-text-input`, `ui-switch`, `ui-slider`,
`ui-dropdown`, `ui-radio-group`, `ui-notification`, `ui-table`.

The protocol, hub, cache, and SPA shell from Phase 1 already support
input events end-to-end (proven by `ui-button`). Phase 2 is widget
implementations and their config panels — no further architecture work.

### Specifics worth calling out

- **Two-way state for inputs**: when `passthrough=true`, an incoming
  message on the widget node updates the visible state of the input on
  every connected client **without** echoing an output event. This
  uses the same `widget` WS message as display widgets — the SPA
  distinguishes update-from-flow from user-typed-value by source and
  suppresses the round-trip emit.
- **`ui-notification`** is unusual: it has an input but no
  in-page rendering. The dashboard SPA registers a global
  `<NotificationLayer>` component that listens for notification
  events and surfaces toasts via the existing toast mechanism (one
  reusable component, configured per ui-notification node).
- **`ui-table`** Phase 2 is intentionally minimal: virtual scroll, sort
  by column header, single-row selection emits a message. No editing,
  no in-cell formatters beyond type-based defaults. Filtering, paging,
  exports are Phase 3.

---

# Phase 3 — Markdown, forms, theming, persistence, ACL

## Status: Proposed

- `ui-form`: composite-input widget.
- `ui-markdown`: read-only markdown with mustache substitution.
- External theme files (`/etc/loopze/dashboard-theme.json`).
- Persistent last-value cache survives restarts (configurable on
  `ui-base`).
- Per-user / per-role ACLs on pages and widgets.
- Mobile-specific layout override (per-page).
- `ui-table` Phase 3 additions: column filters, paging, CSV export.

Phase 3 is **operator-facing polish** and **security gating**. The
underlying widget framework is unchanged.

---

# Phase 4 — Template, iframe, control, event, file-input

## Status: Proposed

- `ui-template`: arbitrary Vue/HTML with sandboxed access to `msg` —
  requires a strict CSP and a compiled-only Vue template path.
- `ui-iframe`: embed an external URL with sandbox attributes.
- `ui-control`: programmatic page navigation, theme switching, etc.
  from within the flow.
- `ui-event`: emit messages on dashboard lifecycle events (page
  enter/leave, client connect/disconnect, idle).
- `ui-file-input`: file upload from the dashboard into the flow
  (binary `msg.payload` with metadata).

Phase 4 widgets each have non-trivial security and lifecycle surface
area. They are listed here so the architecture stays consistent, but
implementation gates on Phase 1–3 maturity.
