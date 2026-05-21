<script setup lang="ts">
import { computed, ref } from 'vue'
import type { LayoutGroup as LayoutGroupT } from '@/composables/useDashboardLayout'
import { useDashboardLayout } from '@/composables/useDashboardLayout'
import { computeDropCell, readWidgetDragPayload, setWidgetDragPayload } from '@/composables/useDragResize'
import { useUiStore } from '@/stores/uiStore'
import LayoutWidgetCard from './LayoutWidgetCard.vue'

const props = defineProps<{
  layoutGroup: LayoutGroupT
  disabled?: boolean
  /** Page id — surfaced on the drag payload so LayoutPage can place
   *  the group at the dropped (x, y) coordinate. */
  pageId: string
}>()

const ui = useUiStore()
const { moveWidget } = useDashboardLayout()

const dropActive = ref(false)
const groupGridEl = ref<HTMLElement | null>(null)

// Position/size from the migrated LayoutTree — these come from the
// composable so legacy workspaces stack automatically.
const groupStyle = computed(() => {
  const w = Math.max(1, Math.min(12, props.layoutGroup.width))
  const x = Math.max(0, Math.min(12 - w, props.layoutGroup.x))
  const h = Math.max(1, props.layoutGroup.height)
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
}

function onDragLeave(e: DragEvent) {
  // dragleave fires when crossing into a child too — filter using
  // relatedTarget so we only clear when the pointer actually exits.
  const next = e.relatedTarget as HTMLElement | null
  if (!groupGridEl.value || !next || !groupGridEl.value.contains(next)) {
    dropActive.value = false
  }
}

function onDrop(e: DragEvent) {
  dropActive.value = false
  document.body.classList.remove('layout-dragging')
  if (props.disabled) return
  const payload = readWidgetDragPayload(e)
  if (!payload) return
  e.preventDefault()
  if (!groupGridEl.value) return
  const { x, y } = computeDropCell(groupGridEl.value, e.clientX, e.clientY)
  moveWidget(payload.nodeId, props.layoutGroup.group.id, x, y)
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
  e.stopPropagation()
}

function onHeaderDragEnd() {
  document.body.classList.remove('layout-dragging-group')
}

// ─── Header double-click → group config ──────────────────────────────────

function onHeaderDoubleClick() {
  if (props.disabled) return
  ui.openConfigEditor('ui-group', props.layoutGroup.group.id)
  if (!ui.propertiesPanelOpen) ui.togglePropertiesPanel()
}

// We accidentally use setWidgetDragPayload nowhere here, but keep the
// import to surface it via the LSP if you add widget-drag affordances
// to the header later. Trimming the import keeps tree-shaking happy.
void setWidgetDragPayload
</script>

<template>
  <section
    class="layout-group"
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
      @dragover="onDragOver"
      @dragleave="onDragLeave"
      @drop="onDrop"
    >
      <LayoutWidgetCard
        v-for="w in layoutGroup.widgets"
        :key="w.node.id"
        :node="w.node"
        :flow-id="w.flowId"
        :from-group-id="layoutGroup.group.id"
        :y="w.y"
        :disabled="disabled"
      />

      <p v-if="widgetCount === 0" class="empty-hint">
        Drop widgets here, or drag them in from another group.
      </p>
    </div>
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
  grid-template-columns: repeat(12, 1fr);
  /* 50 px fixed row units — single source of truth shared with the
     dashboard SPA (frontend/dashboard/src/App.vue). See
     frontend/src/nodes/dashboard/sizing.ts for the typed default
     table and the ROW_UNIT_PX export consumed by drag-resize. */
  grid-auto-rows: 50px;
  gap: 0.5rem;
  transition: background-color 0.1s;
}
.layout-group-grid.drop-active {
  background: rgba(88, 166, 255, 0.06);
  outline: 1px dashed rgba(88, 166, 255, 0.4);
  outline-offset: -4px;
}
.empty-hint {
  grid-column: span 12;
  margin: 0;
  padding: 1.5rem;
  text-align: center;
  font-style: italic;
  color: var(--color-terminal-text-dim, #7d8590);
  font-size: 0.8rem;
}
</style>
