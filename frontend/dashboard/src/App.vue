<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
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

const WIDGET_COMPONENTS: Record<string, unknown> = {
  'ui-button': ButtonWidget,
  'ui-text':   TextWidget,
  'ui-led':    LedWidget,
  'ui-gauge':  GaugeWidget,
}

function componentFor(type: string): unknown | null {
  return WIDGET_COMPONENTS[type] ?? null
}

// ─── State ────────────────────────────────────────────────────────────────────

const layout        = ref<Snapshot | null>(null)
const loadError     = ref<string | null>(null)
const widgetValues  = ref<Record<string, CacheEntry>>({})

let wsClient: WsClient | null = null
const wsState  = ref<'connecting' | 'open' | 'closed'>('connecting')
const wsDownMs = ref(0)
let stateTickRunning = false

// ─── Live clock ───────────────────────────────────────────────────────────────

const now = ref(new Date())
let clockInterval: ReturnType<typeof setInterval> | null = null

const timeStr = computed(() =>
  now.value.toLocaleTimeString('de-DE', { hour: '2-digit', minute: '2-digit', second: '2-digit' }),
)

// ─── WS handlers ──────────────────────────────────────────────────────────────

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
      break
    case 'error':
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
    if (!wsClient) { stateTickRunning = false; return }
    wsState.value  = wsClient.state.value
    wsDownMs.value = wsClient.downSince.value
    requestAnimationFrame(tick)
  }
  requestAnimationFrame(tick)
}

// ─── Lifecycle ────────────────────────────────────────────────────────────────

onMounted(async () => {
  clockInterval = setInterval(() => { now.value = new Date() }, 1000)
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
  if (clockInterval !== null) clearInterval(clockInterval)
  wsClient?.close()
  wsClient = null
})

// ─── Derived layout ───────────────────────────────────────────────────────────

const pagesSorted = computed(() => {
  if (!layout.value) return []
  return [...layout.value.pages].sort((a, b) => a.order - b.order || a.name.localeCompare(b.name))
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
const accent        = computed(() => layout.value?.base?.accentColor ?? '#58a6ff')
const reconnecting  = computed(() => wsState.value !== 'open' && wsDownMs.value > 2000)
const hasBase       = computed(() => Boolean(layout.value?.base))

// ─── Page navigation ──────────────────────────────────────────────────────────

const activePageId = ref<string | null>(null)

const activePage = computed(() => {
  const list = pagesSorted.value
  if (list.length === 0) return null
  if (!activePageId.value) return list[0]
  return list.find((p) => p.id === activePageId.value) ?? list[0]
})

const showNav = computed(() => {
  if (!layout.value?.base) return false
  if (pagesSorted.value.length <= 1) return false
  return layout.value.base.showNav !== false
})

const navStyle = computed<'tabs' | 'sidebar'>(() =>
  layout.value?.base?.navStyle === 'sidebar' ? 'sidebar' : 'tabs',
)

const SIDEBAR_STORAGE_KEY = 'loopze-dashboard-sidebar-collapsed'
const sidebarCollapsed = ref<boolean>((() => {
  try { return localStorage.getItem(SIDEBAR_STORAGE_KEY) === '1' } catch { return false }
})())

function toggleSidebar() {
  sidebarCollapsed.value = !sidebarCollapsed.value
  try { localStorage.setItem(SIDEBAR_STORAGE_KEY, sidebarCollapsed.value ? '1' : '0') } catch {}
}

function pageIconText(name: string): string {
  return (name || '?').trim().slice(0, 1).toUpperCase()
}

watch(
  () => pagesSorted.value.map((p) => p.id),
  (ids) => {
    if (ids.length === 0) { activePageId.value = null; return }
    if (!activePageId.value || !ids.includes(activePageId.value)) activePageId.value = ids[0]
  },
  { immediate: true },
)

// ─── Grid helpers ─────────────────────────────────────────────────────────────

function widgetStyle(widget: LayoutWidget, groupCols: number): Record<string, string> {
  const cols = Math.max(1, groupCols)
  const w = widget.width > 0 ? Math.min(cols, widget.width) : cols
  const h = widget.height > 0 ? Math.min(48, widget.height) : 1
  const x = Math.max(0, Math.min(cols - w, widget.x))
  return {
    gridColumn: `${x + 1} / span ${w}`,
    gridRow:    `${widget.y + 1} / span ${h}`,
  }
}

function groupStyle(
  group: { x: number; y: number; width: number; height: number },
  pageCols: number,
): Record<string, string> {
  const cols = Math.max(1, pageCols)
  const w    = Math.max(1, Math.min(cols, group.width || cols))
  const x    = Math.max(0, Math.min(cols - w, group.x))
  const h    = Math.max(1, group.height || 6)
  return {
    gridColumn: `${x + 1} / span ${w}`,
    gridRow:    `${group.y + 1} / span ${h}`,
  }
}

function groupInternalCols(group: { width: number }, pageCols: number): number {
  return group.width > 0 ? Math.min(pageCols, group.width) : pageCols
}

// ─── Group status ─────────────────────────────────────────────────────────────

const RUNTIME_STATUS_COLORS: Record<string, string> = {
  running: '#3fb950',
  idle:    '#d29922',
  warning: '#f0883e',
  fault:   '#f85149',
  ok:      '#39d3b0',
  off:     '#4a5568',
}
const RUNTIME_STATUS_LABELS: Record<string, string> = {
  running: 'RUNNING',
  idle:    'IDLE',
  warning: 'WARNING',
  fault:   'FAULT',
  ok:      'OK',
  off:     'OFF',
}

// Effective color: runtime status overrides configured statusColor.
function groupEffectiveColor(group: LayoutGroup): string | undefined {
  if (group.status) return RUNTIME_STATUS_COLORS[group.status]
  return group.statusColor || undefined
}

// Effective pill text: runtime status overrides configured statusText.
function groupEffectiveText(group: LayoutGroup): string | undefined {
  if (group.status) return RUNTIME_STATUS_LABELS[group.status]
  return group.statusText || undefined
}

// ─── Connection indicator ─────────────────────────────────────────────────────

const connLabel = computed(() =>
  wsState.value === 'open' ? 'ONLINE' : reconnecting.value ? 'RECONNECTING' : 'OFFLINE',
)
const connColor = computed(() =>
  wsState.value === 'open' ? 'var(--s-running)' : reconnecting.value ? 'var(--s-warning)' : 'var(--s-fault)',
)

</script>

<template>
  <div
    class="dash-root"
    :class="{ 'has-sidebar': showNav && navStyle === 'sidebar', 'sidebar-collapsed': sidebarCollapsed }"
    :style="{ '--accent': accent }"
  >
    <!-- ── SIDEBAR ─────────────────────────────────────────────────────────── -->
    <aside v-if="showNav && navStyle === 'sidebar' && !sidebarCollapsed" class="dash-sidebar">
      <div class="sidebar-head">
        <span class="sidebar-head-title">Pages</span>
        <button type="button" class="sidebar-close" @click="toggleSidebar">×</button>
      </div>
      <nav class="sidebar-nav">
        <button
          v-for="page in pagesSorted"
          :key="page.id"
          type="button"
          class="sidebar-item"
          :class="{ active: activePage?.id === page.id }"
          @click="activePageId = page.id"
        >
          <span class="sidebar-item-icon">{{ pageIconText(page.name) }}</span>
          <span class="sidebar-item-label">{{ page.name }}</span>
        </button>
      </nav>
    </aside>

    <!-- ── HEADER ─────────────────────────────────────────────────────────── -->
    <header class="dash-header">
      <!-- Left: brand -->
      <div class="dash-brand">
        <button
          v-if="showNav && navStyle === 'sidebar'"
          type="button"
          class="hamburger"
          @click="toggleSidebar"
        >
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
            <line x1="3" y1="6" x2="21" y2="6" /><line x1="3" y1="12" x2="21" y2="12" /><line x1="3" y1="18" x2="21" y2="18" />
          </svg>
        </button>
        <svg class="brand-bolt" viewBox="1.5 2 20 20" fill="var(--accent)">
          <path d="M13 2L3 14h8l-1 8 10-12h-8l1-8z"/>
        </svg>
        <span class="brand-name">LOOPZE</span>
        <span v-if="activePage" class="brand-sep">/</span>
        <span v-if="activePage" class="brand-page">{{ activePage.name }}</span>
      </div>

      <!-- Center: page tabs (tabs mode, multi-page) -->
      <nav v-if="showNav && navStyle === 'tabs'" class="dash-tabs">
        <button
          v-for="page in pagesSorted"
          :key="page.id"
          type="button"
          class="dash-tab"
          :class="{ active: activePage?.id === page.id }"
          @click="activePageId = page.id"
        >{{ page.name }}</button>
      </nav>

      <!-- Right: pill + clock -->
      <div class="dash-header-right">
        <span class="dash-pill" :style="{ '--pill-color': connColor }">
          <span class="pill-dot"></span>{{ connLabel }}
        </span>
        <span class="dash-clock">{{ timeStr }}</span>
      </div>
    </header>

    <!-- ── MAIN ───────────────────────────────────────────────────────────── -->
    <main class="dash-main">
      <section v-if="loadError" class="state-card state-error">
        <h2>Failed to load layout</h2>
        <pre>{{ loadError }}</pre>
      </section>

      <section v-else-if="!layout" class="state-card">
        <p>Loading…</p>
      </section>

      <section v-else-if="!hasBase" class="state-card">
        <h2>No dashboard configured</h2>
        <p>Add a <code>ui-base</code> + <code>ui-page</code> + <code>ui-group</code> in the LOOPZE editor and redeploy.</p>
      </section>

      <template v-else>
        <section v-if="activePage" :key="activePage.id" class="dash-page">
          <div
            class="dash-groups"
            :style="{ gridTemplateColumns: `repeat(${activePage.cols || 12}, 1fr)` }"
          >
            <template v-if="groupsForPage(activePage.id).length === 0">
              <p class="muted">(no groups on this page)</p>
            </template>

            <div
              v-for="group in groupsForPage(activePage.id)"
              :key="group.id"
              class="dash-group"
              :class="{
                'has-status':    !!(groupEffectiveColor(group)),
                'has-glow':      !!(group.glow && groupEffectiveColor(group)),
                'has-glow-flame':!!(group.glow && group.glowFlame && groupEffectiveColor(group)),
              }"
              :style="[
                groupStyle(group, activePage.cols || 12),
                groupEffectiveColor(group) ? { '--group-status-color': groupEffectiveColor(group) } : {},
              ]"
            >
              <!-- Group header: shown when showHeader=true, or when a
                   runtime status push arrived (group.status) so the
                   badge is always visible even without a header. -->
              <div v-if="group.showHeader || group.status" class="group-header">
                <div class="group-titles">
                  <span v-if="group.label" class="group-label">{{ group.label }}</span>
                  <h3 class="group-name">{{ group.name }}</h3>
                </div>
                <span
                  v-if="groupEffectiveText(group)"
                  class="status-badge"
                  :style="{ '--badge-color': groupEffectiveColor(group) }"
                >
                  <span class="badge-dot"></span>{{ groupEffectiveText(group) }}
                </span>
              </div>

              <!-- Widgets grid -->
              <div
                class="dash-widgets"
                :style="{ gridTemplateColumns: `repeat(${groupInternalCols(group, activePage.cols || 12)}, 1fr)` }"
              >
                <template v-for="widget in widgetsForGroup(group.id)" :key="widget.id">
                  <div
                    class="widget-cell"
                    :style="widgetStyle(widget, groupInternalCols(group, activePage.cols || 12))"
                  >
                    <component
                      v-if="componentFor(widget.type)"
                      :is="componentFor(widget.type)"
                      :widget="widget"
                      :value="widgetValues[widget.id]?.value"
                      :emit-event="emitEvent"
                    />
                    <div v-else class="unknown-widget">{{ widget.type }}</div>
                  </div>
                </template>
                <p v-if="widgetsForGroup(group.id).length === 0" class="muted">(no widgets)</p>
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
/* ── Root layout ─────────────────────────────────────────────────────────── */
.dash-root {
  min-height: 100%;
  display: flex;
  flex-direction: column;
  --sidebar-w: 0px;
  padding-left: var(--sidebar-w);
  transition: padding-left 0.15s ease;
}
.dash-root.has-sidebar         { --sidebar-w: 220px; }
.dash-root.has-sidebar.sidebar-collapsed { --sidebar-w: 0px; }

/* ── Sidebar ─────────────────────────────────────────────────────────────── */
.dash-sidebar {
  position: fixed;
  left: 0; top: 0; bottom: 0;
  width: 220px;
  background: var(--surface);
  border-right: 1px solid var(--border);
  display: flex;
  flex-direction: column;
  z-index: 20;
}
.sidebar-head {
  display: flex;
  align-items: center;
  padding: 0 1rem;
  height: 38px;
  border-bottom: 1px solid var(--border);
  gap: 0.5rem;
}
.sidebar-head-title {
  flex: 1;
  font-size: 0.65rem;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: var(--muted);
}
.sidebar-close {
  background: transparent;
  border: none;
  color: var(--muted);
  font-size: 1.1rem;
  line-height: 1;
  padding: 0.2rem 0.5rem;
  cursor: pointer;
  border-radius: 4px;
}
.sidebar-close:hover { color: var(--fg); background: rgba(255,255,255,.05); }

.sidebar-nav {
  display: flex;
  flex-direction: column;
  padding: 0.5rem 0;
  overflow-y: auto;
  flex: 1;
}
.sidebar-item {
  background: transparent;
  border: none;
  border-left: 2px solid transparent;
  color: var(--muted);
  padding: 0.6rem 1rem;
  display: flex;
  align-items: center;
  gap: 0.75rem;
  font-family: inherit;
  font-size: 0.85rem;
  text-align: left;
  cursor: pointer;
}
.sidebar-item:hover { color: var(--fg); background: rgba(255,255,255,.03); }
.sidebar-item.active {
  color: var(--accent);
  border-left-color: var(--accent);
  background: color-mix(in srgb, var(--accent) 8%, transparent);
}
.sidebar-item-icon {
  flex: 0 0 1.5rem;
  height: 1.5rem;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 1px solid currentColor;
  border-radius: 4px;
  font-size: 0.68rem;
  font-weight: 700;
}
.sidebar-item-label { overflow: hidden; text-overflow: ellipsis; }

/* ── Header ──────────────────────────────────────────────────────────────── */
.dash-header {
  position: sticky;
  top: 0;
  z-index: 10;
  height: 38px;
  display: flex;
  align-items: center;
  padding: 0 1rem;
  gap: 1rem;
  background: #0a0a10;
  border-bottom: 1px solid rgba(255,255,255,.06);
}

.dash-brand {
  display: flex;
  align-items: center;
  gap: 0.55rem;
  flex-shrink: 0;
}
.hamburger {
  background: transparent;
  border: 1px solid var(--border);
  color: var(--muted);
  padding: 0.25rem 0.35rem;
  border-radius: 4px;
  cursor: pointer;
  display: inline-flex;
  margin-right: 0.15rem;
}
.hamburger:hover { color: var(--fg); border-color: rgba(255,255,255,.2); }

.brand-bolt {
  width: 14px;
  height: 14px;
  flex-shrink: 0;
}
.brand-name {
  font-size: 0.78rem;
  font-weight: 700;
  letter-spacing: 0.1em;
  color: #ffffff;
}
.brand-sep {
  color: var(--muted);
  font-size: 0.75rem;
  font-weight: 300;
  opacity: 0.5;
}
.brand-page {
  font-size: 0.78rem;
  font-weight: 500;
  color: var(--fg);
  letter-spacing: 0.01em;
}

/* Page tabs */
.dash-tabs {
  display: flex;
  gap: 0;
  flex: 1;
  overflow: hidden;
  height: 100%;
}
.dash-tab {
  background: transparent;
  border: none;
  border-bottom: 2px solid transparent;
  color: var(--muted);
  padding: 0 0.9rem;
  height: 100%;
  font-family: inherit;
  font-size: 0.78rem;
  font-weight: 500;
  cursor: pointer;
  white-space: nowrap;
  transition: color 0.15s, border-color 0.15s;
}
.dash-tab:hover  { color: var(--fg); }
.dash-tab.active { color: var(--accent); border-bottom-color: var(--accent); }

/* Right section */
.dash-header-right {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  margin-left: auto;
  flex-shrink: 0;
}

.dash-pill {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 2px 9px 2px 7px;
  border-radius: 20px;
  font-size: 0.6rem;
  font-weight: 700;
  letter-spacing: 0.1em;
  font-family: 'IBM Plex Mono', monospace;
  border: 1px solid color-mix(in srgb, var(--pill-color, var(--s-running)) 50%, transparent);
  color: var(--pill-color, var(--s-running));
  background: color-mix(in srgb, var(--pill-color, var(--s-running)) 12%, transparent);
}
.pill-dot {
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: currentColor;
  flex-shrink: 0;
}

.dash-clock {
  font-family: 'IBM Plex Mono', monospace;
  font-size: 0.9rem;
  font-weight: 500;
  letter-spacing: 0.06em;
  color: #ffffff;
  font-variant-numeric: tabular-nums;
}

/* ── Main ────────────────────────────────────────────────────────────────── */
.dash-main {
  flex: 1;
  padding: 1rem 1.25rem 1.5rem;
  overflow: auto;
}

/* ── State cards ─────────────────────────────────────────────────────────── */
.state-card {
  padding: 1.5rem;
  border: 1px dashed var(--border);
  border-radius: 8px;
  color: var(--muted);
}
.state-card h2 { margin: 0 0 .5rem; font-size: 1rem; color: var(--fg); }
.state-card code {
  background: rgba(0,0,0,.4);
  padding: 0.15em 0.4em;
  border-radius: 3px;
  color: var(--accent);
  font-family: 'IBM Plex Mono', monospace;
}
.state-error { border-color: var(--s-fault); }
.state-error pre {
  background: rgba(0,0,0,.4);
  padding: .5rem;
  border-radius: 4px;
  color: var(--s-fault);
  overflow-x: auto;
  font-family: 'IBM Plex Mono', monospace;
  font-size: .75rem;
}

/* ── Page ────────────────────────────────────────────────────────────────── */
.dash-page { /* container; full width */ }

.dash-groups {
  display: grid;
  grid-auto-rows: minmax(50px, auto);
  gap: 0.625rem;
}

/* ── Group card ──────────────────────────────────────────────────────────── */
.dash-group {
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 8px;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  transition: border-color 0.2s;
}

/* Status left border */
.dash-group.has-status {
  border-left: 3px solid var(--group-status-color, var(--border));
}

/* Glow: inner shadow radiating from the frame inward */
.dash-group.has-glow {
  box-shadow:
    inset 0 0 40px -12px color-mix(in srgb, var(--group-status-color) 40%, transparent),
    inset 0 0  12px -4px color-mix(in srgb, var(--group-status-color) 25%, transparent);
}

/* Flame: glow flickers like a candle */
.dash-group.has-glow-flame {
  animation: glow-flame 2.4s ease-in-out infinite;
}

@keyframes glow-flame {
   0% { box-shadow:
          inset 0 0 40px -12px color-mix(in srgb, var(--group-status-color) 40%, transparent),
          inset 0 0  12px  -4px color-mix(in srgb, var(--group-status-color) 25%, transparent); }
  18% { box-shadow:
          inset 0 0 52px  -8px color-mix(in srgb, var(--group-status-color) 55%, transparent),
          inset 0 0  16px  -3px color-mix(in srgb, var(--group-status-color) 35%, transparent); }
  34% { box-shadow:
          inset 0 0 32px -14px color-mix(in srgb, var(--group-status-color) 28%, transparent),
          inset 0 0   8px  -5px color-mix(in srgb, var(--group-status-color) 18%, transparent); }
  55% { box-shadow:
          inset 0 0 58px  -6px color-mix(in srgb, var(--group-status-color) 60%, transparent),
          inset 0 0  18px  -2px color-mix(in srgb, var(--group-status-color) 40%, transparent); }
  72% { box-shadow:
          inset 0 0 36px -13px color-mix(in srgb, var(--group-status-color) 32%, transparent),
          inset 0 0  10px  -5px color-mix(in srgb, var(--group-status-color) 20%, transparent); }
  88% { box-shadow:
          inset 0 0 48px  -9px color-mix(in srgb, var(--group-status-color) 50%, transparent),
          inset 0 0  14px  -3px color-mix(in srgb, var(--group-status-color) 30%, transparent); }
 100% { box-shadow:
          inset 0 0 40px -12px color-mix(in srgb, var(--group-status-color) 40%, transparent),
          inset 0 0  12px  -4px color-mix(in srgb, var(--group-status-color) 25%, transparent); }
}

/* Group header */
.group-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 0.5rem;
  padding: 0.75rem 1rem 0.6rem;
  border-bottom: 1px solid var(--border);
  flex-shrink: 0;
}

.group-titles {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.group-label {
  font-size: 0.68rem;
  font-weight: 700;
  letter-spacing: 0.14em;
  text-transform: uppercase;
  color: var(--accent);
  font-family: 'IBM Plex Mono', monospace;
  line-height: 1;
  opacity: 0.9;
}

.group-name {
  margin: 0;
  font-size: 1.05rem;
  font-weight: 600;
  color: #ffffff;
  letter-spacing: -0.02em;
  line-height: 1.2;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* Status badge */
.status-badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 8px 2px 6px;
  border-radius: 20px;
  font-size: 0.6rem;
  font-weight: 700;
  letter-spacing: 0.1em;
  font-family: 'IBM Plex Mono', monospace;
  flex-shrink: 0;
  border: 1px solid color-mix(in srgb, var(--badge-color) 40%, transparent);
  color: var(--badge-color);
  background: color-mix(in srgb, var(--badge-color) 12%, transparent);
}
.badge-dot {
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: currentColor;
}

/* ── Widgets grid ────────────────────────────────────────────────────────── */
.dash-widgets {
  display: grid;
  grid-auto-rows: minmax(50px, auto);
  gap: 0.5rem;
  padding: 0.75rem;
  flex: 1;
}

.widget-cell {
  display: flex;
  align-items: stretch;
  min-width: 0;
  overflow: auto;
}
.widget-cell > * { width: 100%; min-width: 0; }

.unknown-widget {
  padding: 0.4rem 0.6rem;
  border: 1px dashed var(--border);
  border-radius: 4px;
  color: var(--muted);
  font-size: 0.75rem;
  font-family: 'IBM Plex Mono', monospace;
}

/* ── Validation errors ───────────────────────────────────────────────────── */
.validation-errors {
  margin-top: 1.5rem;
  padding: 1rem;
  border: 1px solid var(--s-fault);
  background: color-mix(in srgb, var(--s-fault) 8%, transparent);
  border-radius: 8px;
  color: #f5b5b5;
}
.validation-errors h3 { margin: 0 0 .5rem; font-size: .85rem; }
.validation-errors ul { margin: 0; padding-left: 1.25rem; font-size: .8rem; }

/* ── Misc ────────────────────────────────────────────────────────────────── */
.muted { color: var(--muted); font-style: italic; margin: 0; font-size: .8rem; }
</style>
