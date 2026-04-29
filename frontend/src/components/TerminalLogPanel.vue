<script setup lang="ts">
import { ref, computed, watch, onUnmounted, nextTick, inject } from 'vue'
import { useUiStore, type LogsLimit } from '@/stores/uiStore'
import { useApi } from '@/composables/useApi'
import LogLine from '@/components/LogLine.vue'
import type { LogEntry, LogLevel } from '@/types/events'

type OnLog = (cb: (e: LogEntry) => void) => () => void
const injectedOnLog = inject<OnLog>('onLog')
if (!injectedOnLog) {
  throw new Error('TerminalLogPanel requires an `onLog` provider in an ancestor component')
}
const onLog: OnLog = injectedOnLog

const ui = useUiStore()
const api = useApi()

const entries = ref<LogEntry[]>([])
const levelFilter = ref<Set<LogLevel>>(new Set(['DEBUG', 'INFO', 'WARN', 'ERROR']))
const searchQuery = ref('')
const autoScroll = ref(true)
const scrollEl = ref<HTMLElement | null>(null)

let unsubscribe: (() => void) | null = null
let staging: LogEntry[] = []
let mode: 'staging' | 'streaming' = 'staging'
let lastSeqFromHttp = 0
let loadEpoch = 0

function entryHaystack(e: LogEntry): string {
  let s = e.message
  if (e.attrs) {
    for (const [k, v] of Object.entries(e.attrs)) {
      s += ' ' + k + '=' + (typeof v === 'string' ? v : JSON.stringify(v))
    }
  }
  return s
}

const visibleEntries = computed<LogEntry[]>(() => {
  const needle = searchQuery.value.trim().toLowerCase()
  return entries.value.filter((e) => {
    if (!levelFilter.value.has(e.level)) return false
    if (!needle) return true
    return entryHaystack(e).toLowerCase().includes(needle)
  })
})

async function open(limit: LogsLimit) {
  const myEpoch = ++loadEpoch
  staging = []
  mode = 'staging'

  // Subscribe FIRST so any entry produced between here and the HTTP
  // response lands in the staging array — never lost.
  unsubscribe?.()
  unsubscribe = onLog((e) => {
    if (mode === 'staging') staging.push(e)
    else appendOne(e)
  })

  let initial: LogEntry[] = []
  try {
    initial = await api.getLogs(limit)
  } catch (err) {
    console.error('[TerminalLogPanel] Failed to load logs:', err)
  }
  // A newer open() may have started while we awaited; bail out cleanly.
  if (myEpoch !== loadEpoch) return

  entries.value = initial
  lastSeqFromHttp = initial.length > 0 ? (initial[initial.length - 1]?.seq ?? 0) : 0

  // Merge anything that arrived during the fetch, deduped by seq.
  for (const e of staging) {
    if (e.seq > lastSeqFromHttp) entries.value.push(e)
  }
  staging = []
  mode = 'streaming'

  await nextTick()
  scrollToBottom()
}

function appendOne(e: LogEntry) {
  const last = entries.value[entries.value.length - 1]
  if (last && e.seq <= last.seq) return // defensive: drop duplicates / out-of-order
  entries.value.push(e)
  // Keep the visible buffer from growing unbounded during high-volume bursts.
  const cap = ui.logsLimit + 200
  if (entries.value.length > cap) {
    entries.value.splice(0, entries.value.length - cap)
  }
  if (autoScroll.value) requestScroll()
}

function close() {
  unsubscribe?.()
  unsubscribe = null
  entries.value = []
  staging = []
  mode = 'staging'
  loadEpoch++
}

watch(
  () => ui.logsPanelOpen,
  (isOpen) => {
    if (isOpen) open(ui.logsLimit)
    else close()
  },
  { immediate: true },
)

watch(
  () => ui.logsLimit,
  (n) => {
    if (ui.logsPanelOpen) open(n)
  },
)

// Auto-scroll: rAF-throttled, suppressed briefly after a programmatic scroll
// so the scroll handler does not flip autoScroll off again.
let scrollFrame = 0
let scrollSuppressUntil = 0

function requestScroll() {
  if (scrollFrame !== 0) return
  scrollFrame = requestAnimationFrame(() => {
    scrollFrame = 0
    if (autoScroll.value) scrollToBottom()
  })
}

function scrollToBottom() {
  if (!scrollEl.value) return
  scrollSuppressUntil = performance.now() + 150
  scrollEl.value.scrollTop = scrollEl.value.scrollHeight
}

function handleScroll() {
  if (performance.now() < scrollSuppressUntil) return
  if (!scrollEl.value) return
  const { scrollTop, scrollHeight, clientHeight } = scrollEl.value
  autoScroll.value = scrollHeight - scrollTop - clientHeight < 40
}

function clearDisplay() {
  entries.value = []
}

function jumpToLatest() {
  autoScroll.value = true
  scrollToBottom()
}

function toggleLevel(l: LogLevel) {
  const next = new Set(levelFilter.value)
  if (next.has(l)) next.delete(l)
  else next.add(l)
  levelFilter.value = next
}

function handleKey(e: KeyboardEvent) {
  if (e.key !== 'Escape' || !ui.logsPanelOpen) return
  // Esc with active search clears the search first, otherwise closes the panel.
  if (searchQuery.value) {
    e.preventDefault()
    searchQuery.value = ''
    return
  }
  const t = e.target as HTMLElement | null
  if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable)) {
    return
  }
  e.preventDefault()
  ui.closeLogsPanel()
}

window.addEventListener('keydown', handleKey)

onUnmounted(() => {
  window.removeEventListener('keydown', handleKey)
  unsubscribe?.()
})

const levels: LogLevel[] = ['DEBUG', 'INFO', 'WARN', 'ERROR']
const levelColor: Record<LogLevel, string> = {
  DEBUG: 'text-zinc-500',
  INFO: 'text-blue-400',
  WARN: 'text-yellow-400',
  ERROR: 'text-red-400',
}
</script>

<template>
  <div
    v-if="ui.logsPanelOpen"
    class="absolute inset-0 z-20 bg-terminal-bg flex flex-col"
  >
    <!-- Toolbar -->
    <header
      class="flex items-center gap-2 px-3 py-2 border-b border-terminal-border bg-terminal-surface shrink-0"
    >
      <span class="text-[11px] uppercase tracking-wider text-terminal-text-dim font-semibold mr-2">
        Terminal Log
      </span>

      <select
        :value="ui.logsLimit"
        class="bg-terminal-bg border border-terminal-border rounded px-2 py-1 text-[11px] text-terminal-text focus:outline-none focus:border-accent"
        @change="ui.setLogsLimit(Number(($event.target as HTMLSelectElement).value) as LogsLimit)"
      >
        <option :value="100">100 lines</option>
        <option :value="200">200 lines</option>
        <option :value="500">500 lines</option>
        <option :value="1000">1000 lines</option>
      </select>

      <div class="flex items-center gap-1 ml-1">
        <button
          v-for="l in levels"
          :key="l"
          class="text-[10px] uppercase tracking-wide px-2 py-1 rounded font-semibold transition-all"
          :class="
            levelFilter.has(l)
              ? `${levelColor[l]} bg-terminal-bg`
              : 'text-terminal-text-dim/40 hover:text-terminal-text-dim bg-terminal-bg/40'
          "
          @click="toggleLevel(l)"
        >
          {{ l }}
        </button>
      </div>

      <div class="relative ml-1">
        <svg
          xmlns="http://www.w3.org/2000/svg"
          class="absolute left-2 top-1/2 -translate-y-1/2 w-3 h-3 text-terminal-text-dim pointer-events-none"
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
          stroke-width="2"
        >
          <circle cx="11" cy="11" r="7" />
          <path stroke-linecap="round" d="M21 21l-4.3-4.3" />
        </svg>
        <input
          v-model="searchQuery"
          type="text"
          placeholder="search"
          class="bg-terminal-bg border border-terminal-border rounded pl-7 pr-6 py-1 text-[11px] text-terminal-text placeholder:text-terminal-text-dim/50 focus:outline-none focus:border-accent w-44"
        />
        <button
          v-if="searchQuery"
          class="absolute right-1.5 top-1/2 -translate-y-1/2 text-terminal-text-dim hover:text-terminal-text"
          title="Clear search"
          @click="searchQuery = ''"
        >
          <svg
            xmlns="http://www.w3.org/2000/svg"
            class="w-3 h-3"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            stroke-width="2.5"
          >
            <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      </div>

      <button
        class="px-2 py-1 rounded text-[10px] uppercase tracking-wider font-semibold bg-terminal-bg text-terminal-text-dim hover:bg-terminal-surface-alt"
        title="Clear display (server buffer untouched)"
        @click="clearDisplay"
      >
        CLR
      </button>

      <span class="text-terminal-text-dim text-[10px] font-mono tabular-nums ml-auto">
        {{ visibleEntries.length }}/{{ entries.length }}
      </span>

      <button
        class="p-1 rounded text-terminal-text-dim hover:text-terminal-text hover:bg-terminal-surface-alt"
        title="Close (Esc)"
        @click="ui.closeLogsPanel()"
      >
        <svg
          xmlns="http://www.w3.org/2000/svg"
          class="w-4 h-4"
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
          stroke-width="2"
        >
          <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
        </svg>
      </button>
    </header>

    <!-- Log content -->
    <div
      ref="scrollEl"
      class="flex-1 overflow-y-auto overflow-x-auto"
      @scroll="handleScroll"
    >
      <LogLine v-for="e in visibleEntries" :key="e.seq" :entry="e" />
      <div
        v-if="visibleEntries.length === 0"
        class="px-3 py-4 text-[11px] text-terminal-text-dim font-mono italic"
      >
        no log entries
      </div>
    </div>

    <!-- "Scroll to latest" affordance -->
    <div v-if="!autoScroll" class="shrink-0 border-t border-terminal-border bg-terminal-surface">
      <button
        class="w-full py-1 text-[10px] text-terminal-text-dim uppercase tracking-wider font-medium hover:text-accent hover:bg-accent/5 transition-all flex items-center justify-center gap-1"
        @click="jumpToLatest"
      >
        scroll to latest
      </button>
    </div>
  </div>
</template>
