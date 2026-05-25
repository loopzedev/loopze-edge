// Types mirror internal/dashboard/layout.go. Keep in sync.

export interface LayoutBase {
  id: string
  name: string
  path: string
  theme: string
  accentColor: string
  auth: string
  showNav: boolean
  /** 'tabs' (default top bar) or 'sidebar' (collapsible left rail). */
  navStyle: string
  density: string
}

export interface LayoutPage {
  id: string
  name: string
  path: string
  icon: string
  layout: string
  /** Column count of the page's grid. Default 12. */
  cols: number
  order: number
}

export interface LayoutGroup {
  id: string
  name: string
  /** Optional short identifier shown above the group name (e.g. "LINE 01"). */
  label?: string
  pageId: string
  /** 0-based column index within the page's 12-col grid. */
  x: number
  /** 0-based row index within the page's grid (50 px row units). */
  y: number
  /** Grid columns to span (1–12). */
  width: number
  /** Grid rows to span. */
  height: number
  showHeader: boolean
  /** Legacy ordering field; renderer uses x/y. */
  order: number
  /** Configured hex color for left border, label, and pill. Empty = no indicator. */
  statusColor?: string
  /** Configured pill label text (e.g. "RUNNING"). */
  statusText?: string
  /** When true, a radial color glow radiates from the left border. */
  glow?: boolean
  /** When true, the glow flickers like a flame. Requires glow=true. */
  glowFlame?: boolean
  /** Runtime status set via ui-group-status node. Not persisted. */
  status?: 'running' | 'idle' | 'warning' | 'fault' | 'ok' | 'off'
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
  /** Present when layoutChanged is true. Applied directly by the
   *  client to avoid a REST round-trip. */
  layout?: Snapshot
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
