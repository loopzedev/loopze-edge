<script setup lang="ts">
import { computed, ref } from 'vue'
import type { Node as LoopzeNode } from '@/types/flow'
import NodeIcon from '@/components/nodes/NodeIcon.vue'
import { getTokens } from '@/components/nodes/tokens'
import { setWidgetDragPayload, useResizeGesture, type ResizeDelta } from '@/composables/useDragResize'
import { useDashboardLayout } from '@/composables/useDashboardLayout'
import { useFlowStore } from '@/stores/flowStore'
import { useUiStore } from '@/stores/uiStore'
import { effectiveHeight, effectiveWidth, effectiveX } from '@/nodes/dashboard/sizing'

const props = defineProps<{
  node: LoopzeNode
  flowId: string
  fromGroupId: string
  /** Migrated y from the LayoutTree — needed because the raw config
   *  may still carry the legacy value of 0 for unmigrated widgets,
   *  but the layout composable has computed the proper stacked y. */
  y: number
  disabled?: boolean
}>()

const flowStore = useFlowStore()
const ui = useUiStore()
const { resizeWidget } = useDashboardLayout()

const tokens = computed(() => getTokens(props.node.type))

// Persisted size — resolved through the shared sizing helper so we
// pick up per-type defaults for old workspaces and never see "0" at
// the render layer.
const persistedW = computed(() => effectiveWidth(props.node.type, props.node.config))
const persistedH = computed(() => effectiveHeight(props.node.type, props.node.config))
const persistedX = computed(() => effectiveX(props.node.config))
// y is provided by the parent because it may be migrated from order.

// Live preview overrides the persisted size while a resize gesture
// is in flight, so the user sees immediate feedback even though we
// only commit on pointerup.
const previewW = ref<number | null>(null)
const previewH = ref<number | null>(null)

const effectiveW = computed(() => previewW.value ?? persistedW.value)
const effectiveH = computed(() => previewH.value ?? persistedH.value)

const cellStyle = computed(() => {
  const w = effectiveW.value
  const x = Math.max(0, Math.min(12 - w, persistedX.value))
  return {
    gridColumn: `${x + 1} / span ${w}`,
    gridRow: `${props.y + 1} / span ${effectiveH.value}`,
  }
})

const cardLabel = computed(() => {
  if (props.node.name) return props.node.name
  const label = props.node.config?.label
  if (typeof label === 'string' && label) return label
  return props.node.type
})

const sizeBadge = computed(() => `${effectiveW.value} × ${effectiveH.value || 'auto'}`)

// ─── Drag (whole card) ───────────────────────────────────────────────────

function onDragStart(e: DragEvent) {
  if (props.disabled) return
  setWidgetDragPayload(e, { nodeId: props.node.id, fromGroupId: props.fromGroupId })
  // Add a body class so empty drop zones can light up without each
  // group having to track its own drag state.
  document.body.classList.add('layout-dragging')
}

function onDragEnd() {
  document.body.classList.remove('layout-dragging')
}

// ─── Resize (SE handle) ──────────────────────────────────────────────────

const { active: resizing, start: startResize } = useResizeGesture({
  onPreview(delta: ResizeDelta) {
    previewW.value = delta.width
    previewH.value = delta.height
  },
  onCommit(delta: ResizeDelta) {
    previewW.value = null
    previewH.value = null
    resizeWidget(props.node.id, delta.width, delta.height)
  },
  onCancel() {
    previewW.value = null
    previewH.value = null
  },
})

function onResizeStart(e: PointerEvent) {
  if (props.disabled) return
  const cardEl = (e.currentTarget as HTMLElement).closest<HTMLElement>('.layout-widget-card')
  const groupEl = cardEl?.closest<HTMLElement>('.layout-group-grid')
  if (!cardEl || !groupEl) return
  startResize(
    {
      groupEl,
      widgetEl: cardEl,
      startWidth: persistedW.value,
      startHeight: persistedH.value,
    },
    e,
  )
}

// ─── Double-click → property panel ───────────────────────────────────────

function onDoubleClick() {
  if (props.disabled) return
  ui.clearConfigEditor()
  flowStore.focusNode(props.node.id, props.flowId)
  ui.openPropertiesPanel()
}
</script>

<template>
  <div
    class="layout-widget-card"
    :class="{ resizing, disabled }"
    :style="[cellStyle, {
      background: tokens.bg,
      border: `1px solid ${tokens.border}`,
      borderRadius: '4px',
    }]"
    :draggable="!disabled"
    @dragstart="onDragStart"
    @dragend="onDragEnd"
    @dblclick="onDoubleClick"
  >
    <div
      class="card-body"
      :style="{ color: tokens.textSub }"
    >
      <div class="icon-slot" :style="{ background: tokens.bgIcon, color: tokens.accent }">
        <NodeIcon :type="node.type" />
      </div>
      <div class="meta">
        <div class="meta-name" :style="{ color: tokens.accent }">{{ cardLabel }}</div>
        <div class="meta-sub">{{ node.type }} · {{ sizeBadge }}</div>
      </div>
    </div>

    <div
      v-if="!disabled"
      class="resize-handle"
      title="Drag to resize"
      :style="{ borderColor: tokens.border }"
      @pointerdown="onResizeStart"
    />
  </div>
</template>

<style scoped>
.layout-widget-card {
  position: relative;
  min-width: 0;
  /* Fill the grid track(s) exactly — default `align-self: stretch`
     does this for height too, but being explicit guards against any
     parent flex/grid quirks that might shrink the card to content. */
  height: 100%;
  display: flex;
  flex-direction: column;
  cursor: grab;
  user-select: none;
  overflow: hidden;
}
.layout-widget-card.resizing {
  cursor: nwse-resize;
}
.layout-widget-card.disabled {
  cursor: not-allowed;
  opacity: 0.55;
}
.card-body {
  flex: 1 1 auto;
  display: flex;
  align-items: center;
  gap: 0.6rem;
  padding: 0.5rem 0.65rem;
  min-width: 0;
}
.icon-slot {
  width: 28px;
  height: 28px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 4px;
  flex-shrink: 0;
}
.meta {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 0.1rem;
}
.meta-name {
  font-size: 0.78rem;
  font-weight: 600;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.meta-sub {
  font-size: 0.65rem;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}
.resize-handle {
  position: absolute;
  right: 1px;
  bottom: 1px;
  width: 12px;
  height: 12px;
  cursor: nwse-resize;
  border-right: 2px solid currentColor;
  border-bottom: 2px solid currentColor;
  opacity: 0.55;
  touch-action: none;
}
.resize-handle:hover {
  opacity: 1;
}
</style>
