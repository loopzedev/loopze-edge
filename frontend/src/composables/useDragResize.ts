// Drag/resize primitives for the dashboard Layout View.
//
// Two independent mechanisms:
//
//   1. HTML5 DnD for "move widget within / across groups". Native is
//      enough because the FlowTabBar (frontend/src/components/
//      FlowTabBar.vue:24-53) already uses it for tab reordering — same
//      pattern, same browser quirks.
//
//   2. Pointer events for "resize a widget by its SE handle". Mirrors
//      the panel resizer in PropertyPanel.vue. Works on touch and
//      mouse without library overhead.
//
// Grid math is encapsulated so the components stay declarative.

import { onBeforeUnmount, ref } from 'vue'
import { ROW_UNIT_PX } from '@/nodes/dashboard/sizing'

// ─── DnD payload helpers ────────────────────────────────────────────────────

/** Serialised on dragstart, deserialised on drop. */
export interface WidgetDragPayload {
  nodeId: string
  fromGroupId: string
}

const DRAG_MIME = 'application/loopze-widget'

export function setWidgetDragPayload(e: DragEvent, payload: WidgetDragPayload): void {
  if (!e.dataTransfer) return
  e.dataTransfer.effectAllowed = 'move'
  e.dataTransfer.setData(DRAG_MIME, JSON.stringify(payload))
  // Some browsers require a text/plain fallback so the drag image
  // and cross-window drops still work. Same hint, plain text.
  e.dataTransfer.setData('text/plain', payload.nodeId)
}

export function readWidgetDragPayload(e: DragEvent): WidgetDragPayload | null {
  if (!e.dataTransfer) return null
  const raw = e.dataTransfer.getData(DRAG_MIME)
  if (!raw) return null
  try {
    const parsed = JSON.parse(raw) as WidgetDragPayload
    if (typeof parsed.nodeId !== 'string') return null
    return parsed
  } catch {
    return null
  }
}

// ─── Grid-cell drop math ────────────────────────────────────────────────────

/** Compute the (x, y) grid coordinate of a pointer position relative
 *  to a 12-column grid container with 50 px row units. Used by both
 *  widget-into-group drops and group-into-page drops. */
export function computeDropCell(
  gridEl: HTMLElement,
  clientX: number,
  clientY: number,
): { x: number; y: number } {
  const rect = gridEl.getBoundingClientRect()
  const colWidth = rect.width / 12
  const relX = Math.max(0, clientX - rect.left)
  const relY = Math.max(0, clientY - rect.top)
  const x = Math.max(0, Math.min(11, Math.floor(relX / Math.max(1, colWidth))))
  const y = Math.max(0, Math.floor(relY / ROW_UNIT_PX))
  return { x, y }
}

// ─── Resize via pointer events ──────────────────────────────────────────────

export interface ResizeStart {
  /** Group container — used to derive column/row track sizes. */
  groupEl: HTMLElement
  /** Widget element being resized. */
  widgetEl: HTMLElement
  /** Widget's initial width/height (in grid units). */
  startWidth: number
  startHeight: number
}

export interface ResizeDelta {
  width: number
  height: number
}

/** Compute the current width/height in grid tracks for a live pointer
 *  delta. Cols snap to 1..12; rows snap to 1..N (no upper limit here —
 *  callers may clamp). */
export function computeResize(
  start: ResizeStart,
  dx: number,
  dy: number,
): ResizeDelta {
  const groupRect = start.groupEl.getBoundingClientRect()
  const colWidth = groupRect.width / 12
  // Single source of truth for the row unit — same value used by the
  // CSS grid-auto-rows declaration in both the Layout View and the
  // Dashboard SPA. See frontend/src/nodes/dashboard/sizing.ts.
  const rowHeight = ROW_UNIT_PX

  const widthDelta = Math.round(dx / colWidth)
  const heightDelta = Math.round(dy / rowHeight)

  const w = Math.max(1, Math.min(12, start.startWidth + widthDelta))
  // height=0 in the data model means "content-driven" — during a
  // resize gesture we treat the user's intent as explicit, so the
  // floor is 1.
  const h = Math.max(1, Math.min(12, start.startHeight + heightDelta))
  return { width: w, height: h }
}

/** Set up a pointer-event-based resize gesture. Returns a `start`
 *  helper to bind to the resize handle's `pointerdown`. Pointermove
 *  and pointerup listeners are attached to `window` so the gesture
 *  survives leaving the widget bounds. Automatically cleaned up on
 *  component unmount.
 *
 *  `onPreview` fires on every pointermove with the current snapped
 *  width/height — components show a visual ghost.
 *  `onCommit` fires on pointerup with the final values — components
 *  call resizeWidget() here. */
export function useResizeGesture(handlers: {
  onPreview: (delta: ResizeDelta) => void
  onCommit: (delta: ResizeDelta) => void
  onCancel?: () => void
}) {
  const active = ref(false)
  let startCtx: ResizeStart | null = null
  let originX = 0
  let originY = 0

  function onMove(e: PointerEvent) {
    if (!startCtx) return
    const delta = computeResize(startCtx, e.clientX - originX, e.clientY - originY)
    handlers.onPreview(delta)
  }

  function onUp(e: PointerEvent) {
    if (!startCtx) return
    const delta = computeResize(startCtx, e.clientX - originX, e.clientY - originY)
    cleanup()
    handlers.onCommit(delta)
  }

  function onCancel() {
    if (!startCtx) return
    cleanup()
    handlers.onCancel?.()
  }

  function cleanup() {
    window.removeEventListener('pointermove', onMove)
    window.removeEventListener('pointerup', onUp)
    window.removeEventListener('pointercancel', onCancel)
    active.value = false
    startCtx = null
  }

  function start(ctx: ResizeStart, e: PointerEvent) {
    if (active.value) return
    active.value = true
    startCtx = ctx
    originX = e.clientX
    originY = e.clientY
    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', onUp)
    window.addEventListener('pointercancel', onCancel)
    e.preventDefault()
    e.stopPropagation()
  }

  onBeforeUnmount(cleanup)

  return { active, start }
}
