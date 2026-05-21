<script setup lang="ts">
import { computed, ref } from 'vue'
import type { LayoutGroup as LayoutGroupT } from '@/composables/useDashboardLayout'
import { useDashboardLayout } from '@/composables/useDashboardLayout'
import {
  computeDropCell,
  readWidgetDragPayload,
  setWidgetDragPayload,
  useResizeGesture,
  type ResizeDelta,
} from '@/composables/useDragResize'
import { useUiStore } from '@/stores/uiStore'
import LayoutWidgetCard from './LayoutWidgetCard.vue'

const props = defineProps<{
  layoutGroup: LayoutGroupT
  disabled?: boolean
  /** Page id — surfaced on the drag payload so LayoutPage can place
   *  the group at the dropped (x, y) coordinate. */
  pageId: string
  /** Parent page's column count — needed for the pointer-resize math
   *  so the group snaps to page-grid columns. */
  pageCols: number
}>()

const ui = useUiStore()
const { moveWidget, resizeGroup } = useDashboardLayout()

const dropActive = ref(false)
const groupGridEl = ref<HTMLElement | null>(null)

// Hover preview during widget drag: tracks the cell-aligned drop
// position + the dragged widget's footprint so the user sees exactly
// where the widget would land. Reset on dragleave/drop.
interface HoverPreview {
  x: number
  y: number
  width: number
  height: number
}
const hoverPreview = ref<HoverPreview | null>(null)

// The number of row tracks the grid renders. Sources, in priority:
//   - previewH (during resize gesture) so the inner grid shrinks
//     in lockstep with the outer cell preview, otherwise minmax-auto
//     would balloon the cell back to the persisted content height.
//   - persisted height
//   - dragRowsOverride bumps it during a widget drag so the user can
//     drop into "fresh territory" past the current bottom edge.
const dragRowsOverride = ref<number | null>(null)
const gridRowCount = computed(() => {
  const base = Math.max(1, previewH.value ?? props.layoutGroup.height)
  if (dragRowsOverride.value !== null) {
    return Math.max(base, dragRowsOverride.value)
  }
  return base
})

// Background grid cells: `cols × gridRowCount` rows. Rendered only
// while a drag is active — the rest of the time the grid is invisible
// so the regular widget render dominates.
const backgroundCells = computed(() => {
  if (!dropActive.value) return []
  const cells: { x: number; y: number }[] = []
  const cols = props.layoutGroup.cols
  for (let y = 0; y < gridRowCount.value; y++) {
    for (let x = 0; x < cols; x++) {
      cells.push({ x, y })
    }
  }
  return cells
})

// Position/size from the migrated LayoutTree — these come from the
// composable already clamped against the parent page's cols, so we
// can render them as-is without further clamping here.
//
// Resize live-preview: while the SE handle is being dragged,
// previewW/previewH override the persisted values so the user gets
// immediate visual feedback. Commit happens on pointerup.
const previewW = ref<number | null>(null)
const previewH = ref<number | null>(null)
const groupStyle = computed(() => {
  const w = Math.max(1, previewW.value ?? props.layoutGroup.width)
  const x = Math.max(0, props.layoutGroup.x)
  const h = Math.max(1, previewH.value ?? props.layoutGroup.height)
  return {
    gridColumn: `${x + 1} / span ${w}`,
    gridRow: `${props.layoutGroup.y + 1} / span ${h}`,
  }
})

const collapsible = computed(() => Boolean(props.layoutGroup.group.config?.collapsible))
const collapsed = ref(false)

const widgetCount = computed(() => props.layoutGroup.widgets.length)

// ─── Widget drop ─────────────────────────────────────────────────────────

function onDragOver(e: DragEvent) {
  if (props.disabled) return
  // Only react if a widget is being dragged (not a group header drag).
  const types = e.dataTransfer?.types
  if (!types || !Array.from(types).includes('application/loopze-widget')) return
  e.preventDefault()
  if (e.dataTransfer) e.dataTransfer.dropEffect = 'move'
  dropActive.value = true

  // Update the hover preview. The dragged widget's w/h came in via
  // dragstart, but dataTransfer.getData('application/loopze-widget')
  // returns '' during dragover (security restriction); we can only
  // read the MIME types and the text/plain fallback. Hence we
  // track the dragged widget's footprint via the body-level state
  // set by LayoutWidgetCard.onDragStart (see CSS variables below).
  if (!groupGridEl.value) return
  const cols = props.layoutGroup.cols
  const { x, y } = computeDropCell(groupGridEl.value, e.clientX, e.clientY, cols)
  const rawW = readDraggedWidth()
  const w = Math.min(cols, rawW)
  const h = readDraggedHeight()
  hoverPreview.value = {
    x: Math.max(0, Math.min(cols - w, x)),
    y,
    width: w,
    height: h,
  }
  // If the hover is past the visible bottom, expand the drop grid by
  // one row so the user can drop into "fresh territory".
  const neededRows = y + h
  if (neededRows > props.layoutGroup.height) {
    dragRowsOverride.value = neededRows
  } else {
    dragRowsOverride.value = null
  }
}

function readDraggedWidth(): number {
  const v = parseInt(document.body.dataset.loopzeDragW ?? '0', 10)
  // Clamp to this group's internal column count — a widget from a
  // wider group keeps its w in the source, but the preview here
  // shows the post-drop clamped footprint.
  return v > 0 ? Math.min(props.layoutGroup.cols, v) : 1
}
function readDraggedHeight(): number {
  const v = parseInt(document.body.dataset.loopzeDragH ?? '0', 10)
  return v > 0 ? Math.min(48, v) : 1
}

function onDragLeave(e: DragEvent) {
  // dragleave fires when crossing into a child too — filter using
  // relatedTarget so we only clear when the pointer actually exits.
  const next = e.relatedTarget as HTMLElement | null
  if (!groupGridEl.value || !next || !groupGridEl.value.contains(next)) {
    dropActive.value = false
    hoverPreview.value = null
    dragRowsOverride.value = null
  }
}

function onDrop(e: DragEvent) {
  dropActive.value = false
  hoverPreview.value = null
  dragRowsOverride.value = null
  document.body.classList.remove('layout-dragging')
  if (props.disabled) return
  const payload = readWidgetDragPayload(e)
  if (!payload) return
  e.preventDefault()
  if (!groupGridEl.value) return
  const cols = props.layoutGroup.cols
  const raw = computeDropCell(groupGridEl.value, e.clientX, e.clientY, cols)
  // Clamp x so the dragged widget's full width still fits horizontally.
  const w = Math.min(cols, payload.width)
  const x = Math.max(0, Math.min(cols - w, raw.x))
  moveWidget(payload.nodeId, props.layoutGroup.group.id, x, raw.y)
}

// ─── Group header drag (for page-level group reorder) ────────────────────

function onHeaderDragStart(e: DragEvent) {
  if (props.disabled) return
  if (!e.dataTransfer) return
  e.dataTransfer.effectAllowed = 'move'
  e.dataTransfer.setData(
    'application/loopze-group',
    JSON.stringify({ pageId: props.pageId, groupId: props.layoutGroup.group.id }),
  )
  e.dataTransfer.setData('text/plain', `group:${props.layoutGroup.group.id}`)
  document.body.classList.add('layout-dragging-group')
  // Same dataset bridge as the widget drag — page-level dragover
  // handlers read these to draw an accurate footprint preview
  // (dataTransfer.getData returns '' during dragover).
  // The composable already clamped layoutGroup.width to page.cols
  // when building the tree; write the value as-is so LayoutPage's
  // dragover handler shows the correct footprint preview.
  document.body.dataset.loopzeGroupDragW = String(Math.max(1, props.layoutGroup.width))
  document.body.dataset.loopzeGroupDragH = String(Math.max(1, props.layoutGroup.height))
  e.stopPropagation()
}

function onHeaderDragEnd() {
  document.body.classList.remove('layout-dragging-group')
  delete document.body.dataset.loopzeGroupDragW
  delete document.body.dataset.loopzeGroupDragH
}

// ─── Header double-click → group config ──────────────────────────────────

function onHeaderDoubleClick() {
  if (props.disabled) return
  ui.openConfigEditor('ui-group', props.layoutGroup.group.id)
  if (!ui.propertiesPanelOpen) ui.togglePropertiesPanel()
}

// ─── Pointer-resize (SE handle) ──────────────────────────────────────────

const { active: resizing, start: startResize } = useResizeGesture({
  onPreview(delta: ResizeDelta) {
    previewW.value = delta.width
    previewH.value = delta.height
  },
  onCommit(delta: ResizeDelta) {
    previewW.value = null
    previewH.value = null
    resizeGroup(props.layoutGroup.group.id, delta.width, delta.height)
  },
  onCancel() {
    previewW.value = null
    previewH.value = null
  },
})

function onResizeStart(e: PointerEvent) {
  if (props.disabled) return
  const groupEl = (e.currentTarget as HTMLElement).closest<HTMLElement>('.layout-group')
  const pageEl = groupEl?.closest<HTMLElement>('.layout-page-grid')
  if (!groupEl || !pageEl) return
  startResize(
    {
      // Use the page-grid as the metric source so the column width
      // matches the page-level grid the group occupies.
      groupEl: pageEl,
      widgetEl: groupEl,
      startWidth: props.layoutGroup.width,
      startHeight: props.layoutGroup.height,
      cols: props.pageCols,
    },
    e,
  )
}

// We accidentally use setWidgetDragPayload nowhere here, but keep the
// import to surface it via the LSP if you add widget-drag affordances
// to the header later. Trimming the import keeps tree-shaking happy.
void setWidgetDragPayload
</script>

<template>
  <section
    class="layout-group"
    :class="{ resizing }"
    :style="groupStyle"
  >
    <header
      class="layout-group-header"
      :draggable="!disabled"
      :title="disabled ? '' : 'Drag to reorder groups · double-click to edit'"
      @dragstart="onHeaderDragStart"
      @dragend="onHeaderDragEnd"
      @dblclick="onHeaderDoubleClick"
    >
      <button
        v-if="collapsible"
        class="caret"
        type="button"
        @click="collapsed = !collapsed"
      >{{ collapsed ? '▸' : '▾' }}</button>
      <span class="name">{{ layoutGroup.group.name || 'Group' }}</span>
      <span class="count">{{ widgetCount }} widget{{ widgetCount === 1 ? '' : 's' }}</span>
    </header>

    <div
      v-show="!collapsed"
      ref="groupGridEl"
      class="layout-group-grid"
      :class="{ 'drop-active': dropActive }"
      :style="{
        gridTemplateColumns: `repeat(${layoutGroup.cols}, 1fr)`,
        /* minmax(50px, auto) so a row track grows when a widget's
           natural content exceeds 50 px (long text, json view).
           Without this the widget visually overflows the group. */
        gridTemplateRows: `repeat(${gridRowCount}, minmax(50px, auto))`,
      }"
      @dragover="onDragOver"
      @dragleave="onDragLeave"
      @drop="onDrop"
    >
      <!-- Drop-affordance grid: one transparent cell per (x, y)
           position, only visible while a drag is happening. Layered
           behind the widgets via z-index. -->
      <div
        v-for="cell in backgroundCells"
        :key="`cell-${cell.x}-${cell.y}`"
        class="drop-cell"
        :style="{
          gridColumn: `${cell.x + 1} / span 1`,
          gridRow: `${cell.y + 1} / span 1`,
        }"
      />

      <!-- Hover preview footprint: shows exactly where the dragged
           widget would land. Outlined in the accent colour and sized
           to the dragged widget's actual w/h. -->
      <div
        v-if="hoverPreview"
        class="drop-preview"
        :style="{
          gridColumn: `${hoverPreview.x + 1} / span ${hoverPreview.width}`,
          gridRow: `${hoverPreview.y + 1} / span ${hoverPreview.height}`,
        }"
      />

      <LayoutWidgetCard
        v-for="w in layoutGroup.widgets"
        :key="w.node.id"
        :node="w.node"
        :flow-id="w.flowId"
        :from-group-id="layoutGroup.group.id"
        :y="w.y"
        :parent-cols="layoutGroup.cols"
        :disabled="disabled"
      />

      <p
        v-if="widgetCount === 0 && !dropActive"
        class="empty-hint"
        :style="{ gridColumn: `1 / span ${layoutGroup.cols}` }"
      >
        Drop widgets here, or drag them in from another group.
      </p>
    </div>

    <!-- Resize handle (SE corner of the group). Sits on top of the
         widget grid so it's reachable even when a widget fills the
         bottom-right corner. -->
    <div
      v-if="!disabled"
      class="group-resize-handle"
      title="Drag to resize group"
      @pointerdown="onResizeStart"
    />
  </section>
</template>

<style scoped>
.layout-group {
  background: var(--color-terminal-surface, #161b22);
  border: 1px solid var(--color-terminal-border, #30363d);
  border-radius: 6px;
  display: flex;
  flex-direction: column;
  min-width: 0;
  /* Anchor for the absolutely-positioned SE resize handle. */
  position: relative;
}
.layout-group.resizing {
  outline: 1px dashed var(--color-accent, #58a6ff);
  outline-offset: 1px;
}
.layout-group.resizing .layout-group-grid {
  /* During a resize gesture, clip any widgets that fall outside the
     preview height. Without this, items in implicit tracks past the
     preview would extend the grid back to its persisted height via
     the minmax(50px, auto) row sizing — the user would never see
     the group shrink until release. The handle is a sibling of the
     grid, so it isn't clipped. */
  overflow: hidden;
}
.group-resize-handle {
  position: absolute;
  right: 1px;
  bottom: 1px;
  width: 14px;
  height: 14px;
  cursor: nwse-resize;
  border-right: 2px solid var(--color-terminal-text-dim, #7d8590);
  border-bottom: 2px solid var(--color-terminal-text-dim, #7d8590);
  opacity: 0.6;
  touch-action: none;
  /* Sits above the widget grid so it's reachable even when a widget
     fills the bottom-right corner. */
  z-index: 5;
}
.group-resize-handle:hover {
  opacity: 1;
  border-color: var(--color-accent, #58a6ff);
}
.layout-group-header {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  padding: 0.5rem 0.85rem;
  border-bottom: 1px solid var(--color-terminal-border, #30363d);
  cursor: grab;
  user-select: none;
}
.layout-group-header .caret {
  background: none;
  border: none;
  color: inherit;
  font-size: 0.7rem;
  cursor: pointer;
  width: 14px;
  text-align: left;
}
.layout-group-header .name {
  font-size: 0.85rem;
  font-weight: 500;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--color-terminal-text-dim, #7d8590);
  flex: 1 1 auto;
}
.layout-group-header .count {
  font-size: 0.7rem;
  color: var(--color-terminal-text-dim, #7d8590);
  opacity: 0.7;
}
.layout-group-grid {
  flex: 1 1 auto;
  padding: 0.65rem;
  display: grid;
  /* grid-template-columns and grid-template-rows are bound inline
     because both depend on the group's data (cols field + dynamic
     row count during drag). 50 px row unit matches the dashboard
     SPA + the ROW_UNIT_PX export in
     frontend/src/nodes/dashboard/sizing.ts. */
  grid-auto-rows: 50px;
  gap: 0.5rem;
  transition: background-color 0.1s;
  position: relative;
}
.layout-group-grid.drop-active {
  background: rgba(88, 166, 255, 0.04);
}
.drop-cell {
  /* Cell-sized translucent box rendered behind the widgets to give
     the operator a visible grid while dragging. Border-only so it
     doesn't compete with the widget bodies. */
  border: 1px dashed rgba(88, 166, 255, 0.25);
  border-radius: 3px;
  background: rgba(88, 166, 255, 0.02);
  pointer-events: none;
  z-index: 0;
}
.drop-preview {
  /* Footprint of the dragged widget. Solid accent outline + filled
     background so it's distinct from the empty drop cells. */
  border: 2px solid var(--color-accent, #58a6ff);
  border-radius: 4px;
  background: rgba(88, 166, 255, 0.18);
  pointer-events: none;
  z-index: 1;
  transition: none;
}
.layout-widget-card {
  z-index: 2;
}
.empty-hint {
  grid-column: 1 / span 12;
  grid-row: 1 / span 1;
  margin: 0;
  padding: 1.5rem;
  text-align: center;
  font-style: italic;
  color: var(--color-terminal-text-dim, #7d8590);
  font-size: 0.8rem;
}
</style>
