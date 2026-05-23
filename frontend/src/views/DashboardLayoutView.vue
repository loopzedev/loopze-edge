<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useDashboardLayout } from '@/composables/useDashboardLayout'
import { useFlowStore } from '@/stores/flowStore'
import { useUiStore } from '@/stores/uiStore'
import LayoutPage from '@/components/layout/LayoutPage.vue'
import LayoutBanner from '@/components/layout/LayoutBanner.vue'

const flowStore = useFlowStore()
const ui = useUiStore()
const router = useRouter()
const { tree } = useDashboardLayout()

// Dirty-state indicator only — NOT a lock. The layout view stays
// fully editable while changes are pending; deploy is one explicit
// action that commits everything. Blocking edits on dirty was the
// original (overly cautious) design but it forced the user to deploy
// after every single drag.
const hasPendingChanges = computed(() => flowStore.dirty)

const hasBase = computed(() => Boolean(tree.value.base))

const orphanCount = computed(() => tree.value.orphans.length)

// Single-page view: the dropdown picks which page to edit. Defaults
// to the first page; falls back to the first available page if the
// selected one is removed.
const activePageId = ref<string | null>(null)

const pageOptions = computed(() =>
  tree.value.pages.map((p) => ({
    value: p.page.id,
    label: p.page.name || p.page.id.slice(0, 8),
  })),
)

const activePage = computed(() => {
  if (!activePageId.value) return tree.value.pages[0] ?? null
  return (
    tree.value.pages.find((p) => p.page.id === activePageId.value)
      ?? tree.value.pages[0]
      ?? null
  )
})

// Keep the dropdown selection in sync with what's actually available.
watch(
  () => tree.value.pages.map((p) => p.page.id),
  (ids) => {
    if (ids.length === 0) {
      activePageId.value = null
      return
    }
    if (!activePageId.value || !ids.includes(activePageId.value)) {
      activePageId.value = ids[0]
    }
  },
  { immediate: true },
)

function openBaseConfig() {
  const base = tree.value.base
  ui.openConfigEditor('ui-base', base?.id)
  if (!ui.propertiesPanelOpen) ui.togglePropertiesPanel()
}

function openDashboardTab() {
  window.open('/dashboard/', '_blank')
}

function closeLayoutView() {
  router.push('/')
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
        <label v-if="pageOptions.length > 1" class="page-picker">
          <span class="page-picker-label">Page</span>
          <select v-model="activePageId" class="page-select">
            <option
              v-for="opt in pageOptions"
              :key="opt.value"
              :value="opt.value"
            >{{ opt.label }}</option>
          </select>
        </label>
        <button
          type="button"
          class="action-btn"
          title="Open the live dashboard in a new tab"
          @click="openDashboardTab"
        >
          Open dashboard ↗
        </button>
        <button
          type="button"
          class="action-btn close-btn"
          title="Close layout view and return to the flow editor"
          @click="closeLayoutView"
        >
          ← Back to flows
        </button>
      </div>
    </header>

    <LayoutBanner :visible="hasPendingChanges" />

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
        v-if="activePage"
        :key="activePage.page.id"
        :layout-page="activePage"
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
  /* The parent <main> in App.vue is a flex-1 block (not a flex
     container), so flex: 1 on this element does nothing — instead
     we anchor to the parent's own constrained height. Without this,
     a tall page would grow the layout-view past <main>'s overflow:
     hidden boundary and the user couldn't scroll. */
  height: 100%;
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
.actions { display: flex; gap: 0.5rem; align-items: stretch; }
.page-picker {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.2rem 0.6rem;
  border: 1px solid var(--color-terminal-border, #30363d);
  border-radius: 3px;
  background: var(--color-terminal-surface, #161b22);
}
.page-picker-label {
  font-size: 0.7rem;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--color-terminal-text-dim, #7d8590);
}
.page-select {
  background: transparent;
  border: none;
  color: var(--color-terminal-text, #e6edf3);
  font-family: inherit;
  font-size: 0.78rem;
  padding: 0.15rem 0.25rem;
  cursor: pointer;
}
.page-select:focus { outline: 1px solid var(--color-accent, #58a6ff); border-radius: 2px; }
.close-btn:hover { color: var(--color-status-warning, #d29922); border-color: var(--color-status-warning, #d29922); }
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
