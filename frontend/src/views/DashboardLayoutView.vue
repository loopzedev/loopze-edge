<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useDashboardLayout } from '@/composables/useDashboardLayout'
import { useFlowStore } from '@/stores/flowStore'
import { useUiStore } from '@/stores/uiStore'
import LayoutPage from '@/components/layout/LayoutPage.vue'
import LayoutBanner from '@/components/layout/LayoutBanner.vue'

const flowStore = useFlowStore()
const ui = useUiStore()
const { tree } = useDashboardLayout()

// Edit gating: any unsaved workspace change disables layout edits so
// the live dashboard cannot drift from what the user is rearranging.
// Matches the deploy contract the rest of the editor already uses.
const isLocked = computed(() => flowStore.dirty)

const hasBase = computed(() => Boolean(tree.value.base))

const orphanCount = computed(() => tree.value.orphans.length)

function openBaseConfig() {
  const base = tree.value.base
  ui.openConfigEditor('ui-base', base?.id)
  if (!ui.propertiesPanelOpen) ui.togglePropertiesPanel()
}

function openDashboardTab() {
  window.open('/dashboard/', '_blank')
}

onMounted(() => {
  // Keep the workspace from looking unfocused; the property panel
  // stays where it is, but the canvas should release any selection
  // so the property panel does not flash a stale node config when
  // entering the layout view.
  ui.clearFlowProperties()
})
</script>

<template>
  <div class="layout-view">
    <header class="layout-view-header">
      <div class="title-stack">
        <h1>
          <button
            v-if="hasBase"
            class="title-btn"
            type="button"
            :title="`Edit dashboard config: ${tree.base!.name || 'LOOPZE Dashboard'}`"
            @click="openBaseConfig"
          >{{ tree.base!.name || 'LOOPZE Dashboard' }}</button>
          <span v-else>Dashboard Layout</span>
        </h1>
        <div class="subtitle">
          Drag widgets to reorder · resize from the bottom-right corner ·
          double-click a widget or group header to edit its config.
        </div>
      </div>
      <div class="actions">
        <button
          type="button"
          class="action-btn"
          title="Open the live dashboard in a new tab"
          @click="openDashboardTab"
        >
          Open dashboard ↗
        </button>
      </div>
    </header>

    <LayoutBanner :visible="isLocked" />

    <section v-if="!hasBase" class="empty-state">
      <h2>No dashboard configured</h2>
      <p>
        Open the dashboard config from the header (or click the title
        above) to create a <code>ui-base</code>, then add a
        <code>ui-page</code>, a <code>ui-group</code>, and at least one
        widget (e.g. <code>ui-button</code>) to your flow.
      </p>
      <button class="primary-btn" type="button" @click="openBaseConfig">
        Create dashboard config
      </button>
    </section>

    <template v-else>
      <section v-if="tree.pages.length === 0" class="empty-state">
        <h2>No pages yet</h2>
        <p>
          A dashboard needs at least one <code>ui-page</code>. Add one
          via the dashboard config or directly from the property panel.
        </p>
      </section>

      <LayoutPage
        v-for="page in tree.pages"
        :key="page.page.id"
        :layout-page="page"
        :disabled="isLocked"
      />

      <section v-if="orphanCount > 0" class="orphans">
        <h3>Orphan widgets ({{ orphanCount }})</h3>
        <p>
          These widgets reference a group that no longer exists. Open
          the widget's config and assign it to a valid group, or remove
          the widget from the flow.
        </p>
        <ul>
          <li
            v-for="w in tree.orphans"
            :key="w.node.id"
          >
            <strong>{{ w.node.name || w.node.type }}</strong>
            ({{ w.node.type }}, flow {{ w.flowId.slice(0, 8) }})
          </li>
        </ul>
      </section>
    </template>
  </div>
</template>

<style scoped>
.layout-view {
  flex: 1 1 auto;
  overflow: auto;
  padding: 1.25rem 1.5rem 2rem;
  background: var(--color-terminal-bg, #0d1117);
  color: var(--color-terminal-text, #e6edf3);
}
.layout-view-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 1rem;
  margin-bottom: 1.25rem;
}
.title-stack h1 {
  margin: 0;
  font-size: 1.1rem;
  font-weight: 600;
  letter-spacing: 0.02em;
}
.title-btn {
  background: none;
  border: none;
  color: inherit;
  font: inherit;
  cursor: pointer;
  padding: 0;
}
.title-btn:hover {
  color: var(--color-accent, #58a6ff);
}
.subtitle {
  font-size: 0.78rem;
  color: var(--color-terminal-text-dim, #7d8590);
  margin-top: 0.25rem;
  max-width: 65ch;
}
.actions { display: flex; gap: 0.5rem; }
.action-btn,
.primary-btn {
  background: none;
  border: 1px solid var(--color-terminal-border, #30363d);
  color: var(--color-terminal-text, #e6edf3);
  padding: 0.4rem 0.85rem;
  font-size: 0.75rem;
  letter-spacing: 0.03em;
  border-radius: 3px;
  cursor: pointer;
  text-transform: uppercase;
  font-weight: 500;
}
.action-btn:hover,
.primary-btn:hover {
  border-color: var(--color-accent, #58a6ff);
  color: var(--color-accent, #58a6ff);
}
.primary-btn {
  background: var(--color-accent, #58a6ff);
  color: #0d1117;
  border-color: var(--color-accent, #58a6ff);
  margin-top: 0.75rem;
}
.primary-btn:hover {
  filter: brightness(1.1);
}
.empty-state {
  padding: 1.5rem;
  border: 1px dashed var(--color-terminal-border, #30363d);
  border-radius: 6px;
  color: var(--color-terminal-text-dim, #7d8590);
  max-width: 60ch;
}
.empty-state h2 {
  margin: 0 0 0.5rem;
  color: var(--color-terminal-text, #e6edf3);
  font-size: 0.95rem;
}
.empty-state code {
  background: rgba(0, 0, 0, 0.35);
  padding: 0.1em 0.4em;
  border-radius: 3px;
  color: var(--color-accent, #58a6ff);
}
.orphans {
  margin-top: 1.5rem;
  padding: 1rem;
  border: 1px solid var(--color-status-warning, #d29922);
  border-radius: 6px;
  color: var(--color-status-warning, #d29922);
  font-size: 0.85rem;
}
.orphans h3 { margin: 0 0 0.5rem; font-size: 0.9rem; }
.orphans ul {
  margin: 0.5rem 0 0;
  padding-left: 1.25rem;
}
.orphans li { margin-bottom: 0.2rem; }
</style>
