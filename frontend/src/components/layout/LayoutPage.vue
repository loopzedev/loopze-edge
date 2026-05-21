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

function onPageDragOver(e: DragEvent) {
  if (props.disabled) return
  const types = e.dataTransfer?.types
  if (!types || !Array.from(types).includes('application/loopze-group')) return
  e.preventDefault()
  if (e.dataTransfer) e.dataTransfer.dropEffect = 'move'
  dropActive.value = true
}

function onPageDragLeave(e: DragEvent) {
  const next = e.relatedTarget as HTMLElement | null
  if (!pageGridEl.value || !next || !pageGridEl.value.contains(next)) {
    dropActive.value = false
  }
}

function onPageDrop(e: DragEvent) {
  dropActive.value = false
  if (props.disabled) return
  const raw = e.dataTransfer?.getData('application/loopze-group')
  if (!raw) return
  try {
    const payload = JSON.parse(raw) as { pageId: string; groupId: string }
    if (payload.pageId !== props.layoutPage.page.id) return
    if (!pageGridEl.value) return
    e.preventDefault()
    const { x, y } = computeDropCell(pageGridEl.value, e.clientX, e.clientY)
    moveGroup(payload.groupId, x, y)
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
      :class="{ 'drop-active': dropActive }"
      @dragover="onPageDragOver"
      @dragleave="onPageDragLeave"
      @drop="onPageDrop"
    >
      <LayoutGroup
        v-for="g in layoutPage.groups"
        :key="g.group.id"
        :layout-group="g"
        :page-id="layoutPage.page.id"
        :disabled="disabled"
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
  grid-template-columns: repeat(12, 1fr);
  grid-auto-rows: 50px;
  gap: 0.5rem;
  position: relative;
}
.layout-page-grid.drop-active {
  background: rgba(88, 166, 255, 0.05);
  outline: 1px dashed rgba(88, 166, 255, 0.35);
  outline-offset: -2px;
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
