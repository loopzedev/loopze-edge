// Types mirror internal/dashboard/layout.go. Keep in sync.

export interface LayoutBase {
  id: string
  name: string
  path: string
  theme: string
  accentColor: string
  auth: string
  showNav: boolean
  density: string
}

export interface LayoutPage {
  id: string
  name: string
  path: string
  icon: string
  layout: string
  order: number
}

export interface LayoutGroup {
  id: string
  name: string
  pageId: string
  /** 0-based column index within the page's 12-col grid. */
  x: number
  /** 0-based row index within the page's grid (50 px row units). */
  y: number
  /** Grid columns to span (1–12). */
  width: number
  /** Grid rows to span. */
  height: number
  collapsible: boolean
  /** Legacy ordering field; renderer uses x/y. */
  order: number
}

export interface LayoutWidget {
  id: string
  type: string
  name: string
  label?: string
  tooltip?: string
  groupId: string
  /** 0-based column index within the group's 12-col grid. */
  x: number
  /** 0-based row index within the group's grid (50 px row units). */
  y: number
  /** Grid columns to span (1–12). 0 = full group width. */
  width: number
  /** Grid rows to span. */
  height: number
  /** Legacy ordering field; renderer uses x/y. */
  order: number
  config: Record<string, unknown>
}

export interface LayoutError {
  nodeId?: string
  message: string
}

export interface Snapshot {
  base?: LayoutBase
  pages: LayoutPage[]
  groups: LayoutGroup[]
  widgets: LayoutWidget[]
  errors?: LayoutError[]
}

export interface CacheEntry {
  value: unknown
  ts: number // unix milli
}

// ─── WS frames ────────────────────────────────────────────────────────────

export interface SnapshotFrame {
  type: 'snapshot'
  layout: Snapshot
  widgets: Record<string, CacheEntry>
  ts: number
}

export interface WidgetFrame {
  type: 'widget'
  id: string
  value: unknown
  ts: number
}

export interface DeployFrame {
  type: 'deploy'
  layoutChanged: boolean
}

export interface ErrorFrame {
  type: 'error'
  message: string
}

export type ServerFrame = SnapshotFrame | WidgetFrame | DeployFrame | ErrorFrame

export interface HelloFrame {
  type: 'hello'
}

export interface EventFrame {
  type: 'event'
  id: string
  value: unknown
}

export type ClientFrame = HelloFrame | EventFrame
