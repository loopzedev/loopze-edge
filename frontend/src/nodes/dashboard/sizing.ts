// Shared dashboard widget sizing helpers.
//
// Both the Layout View (editor) and the Dashboard SPA render widgets
// on a 12-column × 50px-row grid. This file is the single source of
// truth for:
//   - per-widget-type size defaults (used as fallback when config
//     omits width/height OR carries legacy zero values)
//   - clamping bounds (1..12 row units, 0=full / 1..12 columns)
//   - the row-unit pixel size (50px) consumed by drag-resize math
//
// The matching backend table lives in internal/dashboard/layout.go.
// If a widget type is added in one place, add it in the other too.

export interface WidgetSizeDefault {
  /** 0 = full row (12 cols); 1..12 = explicit column span. */
  width: number
  /** Row-units to span. Always ≥ 1 — there is no "auto" height. */
  height: number
}

const DEFAULTS: Record<string, WidgetSizeDefault> = {
  'ui-button': { width: 0, height: 1 },
  'ui-text':   { width: 0, height: 1 },
  'ui-led':    { width: 0, height: 1 },
  'ui-gauge':  { width: 6, height: 4 },
  // PR 4 adds 'ui-chart': { width: 6, height: 4 }
}

const FALLBACK: WidgetSizeDefault = { width: 0, height: 1 }

/** Row unit height in pixels. Both grids use this; drag-resize snaps
 *  to it. */
export const ROW_UNIT_PX = 50

export function defaultSize(widgetType: string): WidgetSizeDefault {
  return DEFAULTS[widgetType] ?? FALLBACK
}

/** Resolve the effective column span (1..12) for a widget. width=0
 *  in the config means "full row" → returns 12. */
export function effectiveWidth(
  widgetType: string,
  cfg: Record<string, unknown> | undefined,
): number {
  const raw = cfg?.width
  if (typeof raw === 'number' && raw > 0) {
    return clamp(Math.round(raw), 1, 12)
  }
  const def = defaultSize(widgetType).width
  return def === 0 ? 12 : clamp(def, 1, 12)
}

/** Resolve the effective row-unit span (1..12) for a widget. Old
 *  height=0 values fall back to the type default. */
export function effectiveHeight(
  widgetType: string,
  cfg: Record<string, unknown> | undefined,
): number {
  const raw = cfg?.height
  if (typeof raw === 'number' && raw > 0) {
    return clamp(Math.round(raw), 1, 12)
  }
  return clamp(defaultSize(widgetType).height, 1, 12)
}

function clamp(v: number, lo: number, hi: number): number {
  if (v < lo) return lo
  if (v > hi) return hi
  return v
}

/** Resolve the effective x (0–11). Defaults to 0 if missing. */
export function effectiveX(cfg: Record<string, unknown> | undefined): number {
  const raw = cfg?.x
  if (typeof raw === 'number' && raw >= 0) return clamp(Math.round(raw), 0, 11)
  return 0
}

/** Resolve the effective y. Defaults to 0 if missing. The "stack
 *  vertically" migration for legacy workspaces happens in
 *  resolveLegacyPositions below — this raw read returns 0 when y is
 *  unset and the migration layer takes over from there. */
export function effectiveY(cfg: Record<string, unknown> | undefined): number {
  const raw = cfg?.y
  if (typeof raw === 'number' && raw >= 0) return clamp(Math.round(raw), 0, 10000)
  return 0
}

/** Reports whether the config has an explicit y (vs. needs migration). */
export function hasExplicitY(cfg: Record<string, unknown> | undefined): boolean {
  return typeof cfg?.y === 'number' && (cfg.y as number) >= 0
}

// ─── Position migration + push-down collision resolver ─────────────────────

export interface PositionedItem {
  id: string
  /** True if the config carries an explicit y (the user pinned it). */
  hasY: boolean
  x: number
  y: number
  width: number
  height: number
  /** Legacy ordering field; used as tiebreaker for slots without y. */
  order: number
}

/** Migrate legacy positions: items without explicit y get stacked
 *  starting at y=cumulativeHeightOfEarlierSiblings. Items with
 *  explicit y act as anchors (their y stays, cumulative advances
 *  past their footprint). Same algorithm as the Go-side
 *  migrateWidgetPositions in internal/dashboard/layout.go — keep
 *  both in sync. */
export function migratePositions(items: PositionedItem[]): PositionedItem[] {
  const sorted = [...items].sort((a, b) => {
    if (a.hasY !== b.hasY) return a.hasY ? -1 : 1
    if (a.hasY) return a.y - b.y
    if (a.order !== b.order) return a.order - b.order
    return a.id.localeCompare(b.id)
  })
  let cumY = 0
  const byId = new Map<string, PositionedItem>()
  for (const s of sorted) {
    let y = s.y
    if (s.hasY) {
      if (s.y + s.height > cumY) cumY = s.y + s.height
    } else {
      y = cumY
      cumY += s.height
    }
    byId.set(s.id, { ...s, y })
  }
  // Return in original input order so callers can map back by index.
  return items.map((s) => byId.get(s.id)!)
}

/** Overlap test on a 12-col grid: rectangles (x..x+w-1, y..y+h-1). */
function overlaps(a: PositionedItem, b: PositionedItem): boolean {
  if (a.x + a.width <= b.x) return false
  if (b.x + b.width <= a.x) return false
  if (a.y + a.height <= b.y) return false
  if (b.y + b.height <= a.y) return false
  return true
}

/** Push-down collision resolver. The `anchorId` is the just-moved
 *  or just-resized item — it stays put. Every other item shifts
 *  downward (y increases) until no overlap with already-placed
 *  items remains. Returns items with possibly updated y values.
 *
 *  Same algorithm shape as gridstack.js. O(n²) but n is small. */
export function resolveCollisions(
  items: PositionedItem[],
  anchorId: string,
): PositionedItem[] {
  const anchor = items.find((i) => i.id === anchorId)
  if (!anchor) return items
  const others = items.filter((i) => i.id !== anchorId)
  // Sort others by (y, x) so we resolve top-to-bottom, left-to-right.
  others.sort((a, b) => {
    if (a.y !== b.y) return a.y - b.y
    if (a.x !== b.x) return a.x - b.x
    return a.id.localeCompare(b.id)
  })
  const placed: PositionedItem[] = [anchor]
  const out: PositionedItem[] = [anchor]
  for (const w of others) {
    let y = w.y
    // Push down until no collision with anything placed so far.
    while (placed.some((p) => overlaps({ ...w, y }, p))) {
      y += 1
      if (y > 10000) break // safety net
    }
    const settled = { ...w, y }
    placed.push(settled)
    out.push(settled)
  }
  // Restore input order so callers can match by id.
  const byId = new Map(out.map((i) => [i.id, i]))
  return items.map((i) => byId.get(i.id)!)
}
