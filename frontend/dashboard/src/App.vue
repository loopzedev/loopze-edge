<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { fetchLayout } from './api'
import { createWsClient, type WsClient } from './ws'
import type {
  CacheEntry,
  LayoutGroup,
  LayoutWidget,
  ServerFrame,
  Snapshot,
} from './types'
import ButtonWidget from './widgets/ButtonWidget.vue'
import TextWidget from './widgets/TextWidget.vue'
import LedWidget from './widgets/LedWidget.vue'
import GaugeWidget from './widgets/GaugeWidget.vue'

// Widget type → renderer component. PR 4 will add ui-chart.
const WIDGET_COMPONENTS: Record<string, unknown> = {
  'ui-button': ButtonWidget,
  'ui-text':   TextWidget,
  'ui-led':    LedWidget,
  'ui-gauge':  GaugeWidget,
}

function componentFor(type: string): unknown | null {
  return WIDGET_COMPONENTS[type] ?? null
}

const layout = ref<Snapshot | null>(null)
const loadError = ref<string | null>(null)
const widgetValues = ref<Record<string, CacheEntry>>({})

let wsClient: WsClient | null = null
const wsState = ref<'connecting' | 'open' | 'closed'>('connecting')
const wsDownMs = ref(0)
let stateTickRunning = false

function onFrame(frame: ServerFrame) {
  switch (frame.type) {
    case 'snapshot':
      widgetValues.value = { ...frame.widgets }
      if (frame.layout) layout.value = frame.layout
      break
    case 'widget':
      widgetValues.value = {
        ...widgetValues.value,
        [frame.id]: { value: frame.value, ts: frame.ts },
      }
      break
    case 'deploy':
      // PR 5 will refetch layout on layoutChanged. For PR 2 the
      // snapshot frame on next connect already carries the latest
      // layout, so a no-op here is acceptable.
      break
    case 'error':
      // eslint-disable-next-line no-console
      console.warn('dashboard ws error frame:', frame.message)
      break
  }
}

function emitEvent(id: string, value: unknown) {
  wsClient?.send({ type: 'event', id, value })
}

function startStateTick() {
  if (stateTickRunning) return
  stateTickRunning = true
  const tick = () => {
    if (!wsClient) {
      stateTickRunning = false
      return
    }
    wsState.value = wsClient.state.value
    wsDownMs.value = wsClient.downSince.value
    requestAnimationFrame(tick)
  }
  requestAnimationFrame(tick)
}

onMounted(async () => {
  try {
    layout.value = await fetchLayout()
  } catch (err) {
    loadError.value = (err as Error).message
    return
  }
  wsClient = createWsClient(onFrame)
  startStateTick()
})

onBeforeUnmount(() => {
  wsClient?.close()
  wsClient = null
})

// ─── Derived layout views ──────────────────────────────────────────────────

const pagesSorted = computed(() => {
  if (!layout.value) return []
  return [...layout.value.pages].sort(
    (a, b) => a.order - b.order || a.name.localeCompare(b.name),
  )
})

function groupsForPage(pageId: string): LayoutGroup[] {
  if (!layout.value) return []
  return layout.value.groups
    .filter((g) => g.pageId === pageId)
    .sort((a, b) => a.order - b.order || a.name.localeCompare(b.name))
}

function widgetsForGroup(groupId: string): LayoutWidget[] {
  if (!layout.value) return []
  return layout.value.widgets
    .filter((w) => w.groupId === groupId)
    .sort((a, b) => a.order - b.order)
}

const dashboardName = computed(() => layout.value?.base?.name ?? 'LOOPZE Dashboard')
const accent = computed(() => layout.value?.base?.accentColor ?? '#58a6ff')
const reconnecting = computed(() => wsState.value !== 'open' && wsDownMs.value > 2000)
const hasBase = computed(() => Boolean(layout.value?.base))

// Per-widget grid-area style — explicit (x, y, w, h) coordinates,
// clamped to the parent group's column count so a widget that was
// authored wide and then placed into a narrow group still renders
// sensibly.
function widgetStyle(widget: LayoutWidget, groupCols: number): Record<string, string> {
  const cols = Math.max(1, groupCols)
  const w = widget.width > 0 ? Math.min(cols, widget.width) : cols
  const h = widget.height > 0 ? Math.min(48, widget.height) : 1
  const x = Math.max(0, Math.min(cols - w, widget.x))
  return {
    gridColumn: `${x + 1} / span ${w}`,
    gridRow: `${widget.y + 1} / span ${h}`,
  }
}

// Per-group style on the page grid — same idea but in page-cols.
function groupStyle(
  group: { x: number; y: number; width: number; height: number },
  pageCols: number,
): Record<string, string> {
  const cols = Math.max(1, pageCols)
  const w = Math.max(1, Math.min(cols, group.width || cols))
  const x = Math.max(0, Math.min(cols - w, group.x))
  const h = Math.max(1, group.height || 6)
  return {
    gridColumn: `${x + 1} / span ${w}`,
    gridRow: `${group.y + 1} / span ${h}`,
  }
}

// A group's internal column count: explicit width when set, else
// inherits the page's column count (mirrors backend logic).
function groupInternalCols(group: { width: number }, pageCols: number): number {
  return group.width > 0 ? Math.min(pageCols, group.width) : pageCols
}
</script>

<template>
  <div class="dashboard-root" :style="{ '--accent': accent }">
    <header class="dashboard-header">
      <span class="title">{{ dashboardName }}</span>
      <span v-if="wsState === 'open'" class="ws-status ok">● live</span>
      <span v-else-if="reconnecting" class="ws-status warn">● reconnecting…</span>
      <span v-else-if="wsState === 'connecting'" class="ws-status">● connecting…</span>
      <span v-else class="ws-status err">● offline</span>
    </header>

    <main class="dashboard-main">
      <section v-if="loadError" class="error-state">
        <h2>Failed to load dashboard layout</h2>
        <pre>{{ loadError }}</pre>
      </section>

      <section v-else-if="!layout" class="loading-state">
        <p>Loading…</p>
      </section>

      <section v-else-if="!hasBase" class="empty-state">
        <h2>No dashboard configured</h2>
        <p>
          Add a <code>ui-base</code> + <code>ui-page</code> + <code>ui-group</code>
          in the LOOPZE editor, drop a <code>ui-button</code> into the flow,
          and redeploy.
        </p>
      </section>

      <template v-else>
        <section v-for="page in pagesSorted" :key="page.id" class="page">
          <h2 class="page-title">{{ page.name }}</h2>
          <div
            class="groups"
            :style="{ gridTemplateColumns: `repeat(${page.cols || 12}, 1fr)` }"
          >
            <template v-if="groupsForPage(page.id).length === 0">
              <p class="muted">(no groups on this page)</p>
            </template>
            <div
              v-for="group in groupsForPage(page.id)"
              :key="group.id"
              class="group"
              :style="groupStyle(group, page.cols || 12)"
            >
              <h3 class="group-title">{{ group.name }}</h3>
              <div
                class="widgets"
                :style="{ gridTemplateColumns: `repeat(${groupInternalCols(group, page.cols || 12)}, 1fr)` }"
              >
                <template v-for="widget in widgetsForGroup(group.id)" :key="widget.id">
                  <div
                    class="widget-cell"
                    :style="widgetStyle(widget, groupInternalCols(group, page.cols || 12))"
                  >
                    <component
                      v-if="componentFor(widget.type)"
                      :is="componentFor(widget.type)"
                      :widget="widget"
                      :value="widgetValues[widget.id]?.value"
                      :emit-event="emitEvent"
                    />
                    <div v-else class="unknown-widget">unknown widget type: {{ widget.type }}</div>
                  </div>
                </template>
                <p v-if="widgetsForGroup(group.id).length === 0" class="muted">
                  (no widgets in this group)
                </p>
              </div>
            </div>
          </div>
        </section>

        <section v-if="(layout.errors ?? []).length > 0" class="validation-errors">
          <h3>Validation errors</h3>
          <ul>
            <li v-for="(err, i) in layout.errors" :key="i">
              <span v-if="err.nodeId" class="muted">{{ err.nodeId }}: </span>{{ err.message }}
            </li>
          </ul>
        </section>
      </template>
    </main>
  </div>
</template>

<style scoped>
.dashboard-root {
  min-height: 100%;
  display: flex;
  flex-direction: column;
}
.dashboard-header {
  display: flex;
  align-items: center;
  gap: 1rem;
  padding: 0.75rem 1.25rem;
  background: var(--surface);
  border-bottom: 1px solid var(--border);
}
.title {
  font-size: 1rem;
  font-weight: 600;
  color: var(--fg);
}
.ws-status {
  font-size: 0.8rem;
  color: var(--muted);
  margin-left: auto;
}
.ws-status.ok { color: #5eba7d; }
.ws-status.warn { color: #f5a623; }
.ws-status.err { color: #e15555; }

.dashboard-main {
  flex: 1 1 auto;
  padding: 1.25rem;
  overflow: auto;
}

.error-state, .loading-state, .empty-state {
  padding: 1.5rem;
  border: 1px dashed var(--border);
  border-radius: 6px;
  color: var(--muted);
}
.error-state pre {
  background: #000;
  padding: 0.5rem;
  border-radius: 3px;
  color: #e15555;
  overflow-x: auto;
}
.empty-state code {
  background: #000;
  padding: 0.15em 0.4em;
  border-radius: 3px;
  color: var(--accent);
}

.page { margin-bottom: 2rem; }
.page-title {
  margin: 0 0 0.75rem;
  font-size: 1.05rem;
  font-weight: 600;
  color: var(--fg);
}
.groups {
  display: grid;
  /* grid-template-columns is bound inline because page.cols is
     data-driven. minmax(50px, auto) for implicit rows so group cells
     grow to fit their content (header + padding + inner widget grid)
     instead of forcing the group to overflow the cell border. */
  grid-auto-rows: minmax(50px, auto);
  gap: 0.5rem;
}
.group {
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 6px;
  padding: 1rem;
}
.group-title {
  margin: 0 0 0.75rem;
  font-size: 0.85rem;
  font-weight: 500;
  letter-spacing: 0.04em;
  color: var(--muted);
  text-transform: uppercase;
}
.widgets {
  display: grid;
  /* grid-template-columns is bound inline because the group's
     internal column count is data-driven. minmax(50px, auto) for
     implicit rows so widget tracks grow to fit content (e.g. long
     text or JSON) instead of clipping. */
  grid-auto-rows: minmax(50px, auto);
  gap: 0.5rem;
}
.widget-cell {
  /* Fill the entire grid area; content scrolls inside the cell when
     it overflows so widget heights stay predictable. */
  display: flex;
  align-items: stretch;
  min-width: 0;
  overflow: auto;
}
.widget-cell > * {
  width: 100%;
  min-width: 0;
}
.muted { color: var(--muted); font-style: italic; margin: 0; }

.validation-errors {
  margin-top: 2rem;
  padding: 1rem;
  border: 1px solid #e15555;
  background: #2a1010;
  border-radius: 6px;
  color: #f5b5b5;
}
.validation-errors h3 { margin-top: 0; }
.validation-errors ul { margin: 0.5rem 0 0; padding-left: 1.25rem; }
.unknown-widget {
  padding: 0.5rem 0.75rem;
  border: 1px dashed var(--border);
  color: var(--muted);
  border-radius: 4px;
}
</style>
