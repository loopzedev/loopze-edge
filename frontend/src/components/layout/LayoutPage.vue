<script setup lang="ts">
import { computed, ref } from 'vue'
import type { LayoutPage as LayoutPageT } from '@/composables/useDashboardLayout'
import { useDashboardLayout } from '@/composables/useDashboardLayout'
import { computeDropCell } from '@/composables/useDragResize'
import { useUiStore } from '@/stores/uiStore'
import LayoutGroup from './LayoutGroup.vue'

const props = defineProps<{
  layoutPage: LayoutPageT
  disabled?: boolean
}>()

const ui = useUiStore()
const { moveGroup } = useDashboardLayout()

const pageName = computed(() => props.layoutPage.page.name || 'Page')
const groupCount = computed(() => props.layoutPage.groups.length)

const pageGridEl = ref<HTMLElement | null>(null)
const dropActive = ref(false)
// Counts the groups whose resize gesture is currently active. Lets
// the page show its drop-cell scaffolding during a resize too — the
// user gets the same visual reference grid as during a group move.
const resizingGroupsCount = ref(0)
const gridScaffoldVisible = computed(
  () => dropActive.value || resizingGroupsCount.value > 0,
)

interface HoverPreview {
  x: number
  y: number
  width: number
  height: number
}
const hoverPreview = ref<HoverPreview | null>(null)
const dragRowsOverride = ref<number | null>(null)

// Visible row count for the page grid:
//   - normally the bottom of the last group + a 2-row buffer so the
//     user can always drag below to extend
//   - while dragging, expand further if the hover preview reaches
//     past the current visible bottom
const baseRowCount = computed(() => {
  if (props.layoutPage.groups.length === 0) return 6
  const maxBottom = Math.max(
    ...props.layoutPage.groups.map((g) => g.y + g.height),
  )
  return Math.max(6, maxBottom + 2)
})

const gridRowCount = computed(() => {
  if (dragRowsOverride.value !== null) {
    return Math.max(baseRowCount.value, dragRowsOverride.value)
  }
  return baseRowCount.value
})

// Drop-affordance cells: always rendered (except when disabled).
// Faint in idle so the operator sees the authoring grid as
// persistent scaffolding; accented during drag/resize via the
// .drop-active class on the parent. Requires fixed 50px row tracks
// (see gridTemplateRows below) so a tall group can't balloon empty
// cells in its row alongside it.
const backgroundCells = computed(() => {
  if (props.disabled) return []
  const cells: { x: number; y: number }[] = []
  for (let y = 0; y < gridRowCount.value; y++) {
    for (let x = 0; x < props.layoutPage.cols; x++) {
      cells.push({ x, y })
    }
  }
  return cells
})

function onGroupResizeActive(active: boolean) {
  resizingGroupsCount.value = Math.max(
    0,
    resizingGroupsCount.value + (active ? 1 : -1),
  )
}

function readDraggedGroupWidth(): number {
  const v = parseInt(document.body.dataset.loopzeGroupDragW ?? '0', 10)
  return v > 0 ? Math.min(props.layoutPage.cols, v) : Math.min(props.layoutPage.cols, 6)
}
function readDraggedGroupHeight(): number {
  const v = parseInt(document.body.dataset.loopzeGroupDragH ?? '0', 10)
  return v > 0 ? v : 6
}

function onPageDragOver(e: DragEvent) {
  if (props.disabled) return
  const types = e.dataTransfer?.types
  if (!types || !Array.from(types).includes('application/loopze-group')) return
  e.preventDefault()
  if (e.dataTransfer) e.dataTransfer.dropEffect = 'move'
  dropActive.value = true

  if (!pageGridEl.value) return
  const cols = props.layoutPage.cols
  const { x, y } = computeDropCell(pageGridEl.value, e.clientX, e.clientY, cols)
  const w = readDraggedGroupWidth()
  const h = readDraggedGroupHeight()
  hoverPreview.value = {
    x: Math.max(0, Math.min(cols - w, x)),
    y,
    width: w,
    height: h,
  }
  const neededRows = y + h
  dragRowsOverride.value = neededRows > baseRowCount.value ? neededRows + 1 : null
}

function onPageDragLeave(e: DragEvent) {
  const next = e.relatedTarget as HTMLElement | null
  if (!pageGridEl.value || !next || !pageGridEl.value.contains(next)) {
    dropActive.value = false
    hoverPreview.value = null
    dragRowsOverride.value = null
  }
}

function onPageDrop(e: DragEvent) {
  dropActive.value = false
  hoverPreview.value = null
  dragRowsOverride.value = null
  if (props.disabled) return
  const raw = e.dataTransfer?.getData('application/loopze-group')
  if (!raw) return
  try {
    const payload = JSON.parse(raw) as { pageId: string; groupId: string }
    if (payload.pageId !== props.layoutPage.page.id) return
    if (!pageGridEl.value) return
    e.preventDefault()
    const cols = props.layoutPage.cols
    const cell = computeDropCell(pageGridEl.value, e.clientX, e.clientY, cols)
    // Clamp x so the group still fits horizontally.
    const w = Math.min(cols, readDraggedGroupWidth())
    const x = Math.max(0, Math.min(cols - w, cell.x))
    moveGroup(payload.groupId, x, cell.y)
  } catch {
    // ignore malformed
  }
}

function onTitleDoubleClick() {
  if (props.disabled) return
  ui.openConfigEditor('ui-page', props.layoutPage.page.id)
  if (!ui.propertiesPanelOpen) ui.togglePropertiesPanel()
}
</script>

<template>
  <section class="layout-page">
    <header class="layout-page-header" @dblclick="onTitleDoubleClick">
      <h2 class="title">{{ pageName }}</h2>
      <span class="sub">{{ groupCount }} group{{ groupCount === 1 ? '' : 's' }}</span>
    </header>

    <div v-if="groupCount === 0" class="empty">
      <p>No groups on this page. Open a widget's config and add it to a new group, or create a ui-group config directly.</p>
    </div>

    <div
      v-else
      ref="pageGridEl"
      class="layout-page-grid"
      :class="{ 'drop-active': gridScaffoldVisible }"
      :style="{
        gridTemplateColumns: `repeat(${layoutPage.cols}, 1fr)`,
        /* minmax(50px, auto) matches the dashboard SPA. Required so
           each group cell can absorb its own header + padding on
           top of h*50px for the widget rows; otherwise the inner
           content overflows the cell by exactly that overhead
           (~1 row) and the bottom-most widget escapes the group.
           Drop-cells in a row shared with a content-tall group
           grow with it — acceptable since the row genuinely is
           that tall. */
        gridTemplateRows: `repeat(${gridRowCount}, minmax(50px, auto))`,
      }"
      @dragover="onPageDragOver"
      @dragleave="onPageDragLeave"
      @drop="onPageDrop"
    >
      <!-- Drop-affordance grid (only during drag). -->
      <div
        v-for="cell in backgroundCells"
        :key="`cell-${cell.x}-${cell.y}`"
        class="drop-cell"
        :style="{
          gridColumn: `${cell.x + 1} / span 1`,
          gridRow: `${cell.y + 1} / span 1`,
        }"
      />

      <!-- Footprint preview at the pointer. -->
      <div
        v-if="hoverPreview"
        class="drop-preview"
        :style="{
          gridColumn: `${hoverPreview.x + 1} / span ${hoverPreview.width}`,
          gridRow: `${hoverPreview.y + 1} / span ${hoverPreview.height}`,
        }"
      />

      <LayoutGroup
        v-for="g in layoutPage.groups"
        :key="g.group.id"
        :layout-group="g"
        :page-id="layoutPage.page.id"
        :page-cols="layoutPage.cols"
        :disabled="disabled"
        @resize-active="onGroupResizeActive"
      />
    </div>
  </section>
</template>

<style scoped>
.layout-page {
  margin-bottom: 2rem;
}
.layout-page-header {
  display: flex;
  align-items: baseline;
  gap: 0.75rem;
  padding: 0.5rem 0.25rem 0.75rem;
  cursor: default;
}
.layout-page-header .title {
  margin: 0;
  font-size: 1rem;
  font-weight: 600;
  letter-spacing: 0.02em;
  color: var(--color-terminal-text, #e6edf3);
  cursor: pointer;
}
.layout-page-header .title:hover {
  text-decoration: underline;
}
.layout-page-header .sub {
  font-size: 0.7rem;
  color: var(--color-terminal-text-dim, #7d8590);
  text-transform: uppercase;
  letter-spacing: 0.04em;
}
.layout-page-grid {
  display: grid;
  /* Columns and explicit rows are bound inline because both are
     data-driven (page.cols + dynamic row count during drag). The
     implicit row size matches the explicit minmax(50px, auto)
     tracks so an item dropped past the explicit range stays on the
     same scale. */
  grid-auto-rows: minmax(50px, auto);
  gap: 0.5rem;
  position: relative;
}
.layout-page-grid.drop-active {
  background: rgba(88, 166, 255, 0.03);
}
.drop-cell {
  /* Always rendered. Idle = very faint neutral border so the grid
     reads as authoring scaffolding without competing with the
     widgets. Accent + tinted background kicks in under .drop-active
     while a group is being dragged or resized. */
  border: 1px dashed rgba(125, 133, 144, 0.18);
  border-radius: 3px;
  pointer-events: none;
  z-index: 0;
  transition: border-color 0.15s, background-color 0.15s;
}
.layout-page-grid.drop-active .drop-cell {
  border-color: rgba(88, 166, 255, 0.25);
  background: rgba(88, 166, 255, 0.02);
}
.drop-preview {
  border: 2px solid var(--color-accent, #58a6ff);
  border-radius: 4px;
  background: rgba(88, 166, 255, 0.18);
  pointer-events: none;
  z-index: 1;
}
.layout-group {
  z-index: 2;
}
.empty {
  padding: 1rem;
  border: 1px dashed var(--color-terminal-border, #30363d);
  border-radius: 6px;
  color: var(--color-terminal-text-dim, #7d8590);
  font-size: 0.85rem;
}
.empty p { margin: 0; }
</style>
