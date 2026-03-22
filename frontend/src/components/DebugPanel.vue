<script setup lang="ts">
import { ref, watch, nextTick, computed } from 'vue'
import { useDebugStore } from '@/stores/debugStore'

const debugStore = useDebugStore()
const scrollContainer = ref<HTMLElement | null>(null)
const autoScroll = ref(true)
const filterText = ref('')

const messages = computed(() => debugStore.filteredMessages)

watch(filterText, (val) => {
  debugStore.setFilter(val)
})

watch(
  () => debugStore.messages.length,
  async () => {
    if (autoScroll.value) {
      await nextTick()
      scrollToBottom()
    }
  }
)

function scrollToBottom() {
  if (scrollContainer.value) {
    scrollContainer.value.scrollTop = scrollContainer.value.scrollHeight
  }
}

function handleScroll() {
  if (!scrollContainer.value) return
  const { scrollTop, scrollHeight, clientHeight } = scrollContainer.value
  autoScroll.value = scrollHeight - scrollTop - clientHeight < 40
}

function clearMessages() {
  debugStore.clear()
}

function toggleEnabled() {
  debugStore.toggleEnabled()
}

function formatTimestamp(ts: string): string {
  try {
    const date = new Date(ts)
    const h = date.getHours().toString().padStart(2, '0')
    const m = date.getMinutes().toString().padStart(2, '0')
    const s = date.getSeconds().toString().padStart(2, '0')
    const ms = date.getMilliseconds().toString().padStart(3, '0')
    return `${h}:${m}:${s}.${ms}`
  } catch {
    return ts
  }
}

function formatPayload(payload: unknown): string {
  if (payload === null) return 'null'
  if (payload === undefined) return 'undefined'
  if (typeof payload === 'string') return payload
  if (typeof payload === 'number' || typeof payload === 'boolean') return String(payload)
  try {
    return JSON.stringify(payload, null, 2)
  } catch {
    return String(payload)
  }
}
</script>

<template>
  <div class="flex flex-col h-full bg-terminal-bg text-xs">
    <!-- Toolbar -->
    <div class="flex items-center gap-1.5 px-3 py-2 border-b border-terminal-border bg-terminal-surface shrink-0">
      <input
        v-model="filterText"
        type="text"
        placeholder="Filter messages..."
        class="terminal-input flex-1 py-1 text-[11px]"
      />

      <button
        class="px-2 py-1 rounded text-[10px] uppercase tracking-wider font-semibold transition-all duration-100"
        :class="debugStore.isEnabled
          ? 'bg-status-success/15 text-status-success hover:bg-status-success/25'
          : 'bg-terminal-bg text-terminal-text-dim hover:bg-terminal-surface-alt'"
        :title="debugStore.isEnabled ? 'Pause debug output' : 'Resume debug output'"
        @click="toggleEnabled"
      >
        {{ debugStore.isEnabled ? 'ON' : 'OFF' }}
      </button>

      <button
        class="px-2 py-1 rounded text-[10px] uppercase tracking-wider font-semibold
               bg-terminal-bg text-terminal-text-dim
               hover:bg-terminal-surface-alt hover:text-terminal-text transition-all duration-100"
        title="Clear all messages"
        @click="clearMessages"
      >
        CLR
      </button>

      <span class="text-terminal-text-dim text-[10px] font-mono tabular-nums ml-0.5 whitespace-nowrap">
        {{ debugStore.filteredCount }}/{{ debugStore.messageCount }}
      </span>
    </div>

    <!-- Message list -->
    <div
      ref="scrollContainer"
      class="flex-1 overflow-y-auto overflow-x-hidden"
      @scroll="handleScroll"
    >
      <div v-if="messages.length === 0" class="flex items-center justify-center h-full">
        <div class="text-center">
          <div class="text-terminal-text-dim/40 text-2xl mb-2">
            <svg xmlns="http://www.w3.org/2000/svg" class="w-8 h-8 mx-auto" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
              <path stroke-linecap="round" stroke-linejoin="round" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
            </svg>
          </div>
          <span class="text-terminal-text-dim text-xs">
            {{ debugStore.isEnabled ? 'No debug messages' : 'Debug paused' }}
          </span>
        </div>
      </div>

      <div
        v-for="(msg, idx) in messages"
        :key="msg.id"
        class="border-b px-3 py-2 transition-colors duration-75"
        :class="{
          'border-terminal-border/40': msg.status === 'debug' || !msg.status,
          'border-red-900/50 bg-red-950/20': msg.status === 'error',
          'border-yellow-900/50 bg-yellow-950/15': msg.status === 'warn',
          'bg-terminal-surface/30': (msg.status === 'debug' || !msg.status) && idx % 2 === 0,
        }"
      >
        <!-- Header line: timestamp + node name + format -->
        <div class="flex items-center gap-2 mb-1">
          <span class="text-terminal-text-dim text-[10px] font-mono tabular-nums shrink-0">
            {{ formatTimestamp(msg.timestamp) }}
          </span>

          <!-- Status badge -->
          <span
            v-if="msg.status === 'error'"
            class="text-[9px] font-bold uppercase tracking-wide px-1 py-px rounded bg-status-error/15 text-status-error shrink-0"
          >ERR</span>
          <span
            v-else-if="msg.status === 'warn'"
            class="text-[9px] font-bold uppercase tracking-wide px-1 py-px rounded bg-status-warning/15 text-status-warning shrink-0"
          >WRN</span>

          <!-- Node name -->
          <span
            class="text-[11px] font-medium truncate"
            :class="{
              'text-status-error': msg.status === 'error',
              'text-status-warning': msg.status === 'warn',
              'text-accent': msg.status === 'debug' || !msg.status,
            }"
          >
            {{ msg.nodeName || msg.nodeId?.slice(0, 8) }}
          </span>

          <!-- Property -->
          <span
            v-if="msg.property && msg.property !== 'payload'"
            class="text-terminal-text-dim text-[10px] font-mono"
          >
            .{{ msg.property }}
          </span>

          <!-- Format badge -->
          <span class="ml-auto text-terminal-text-dim/60 text-[9px] font-mono shrink-0 uppercase">
            {{ msg.format }}
          </span>
        </div>

        <!-- Payload -->
        <pre
          class="text-[11px] font-mono whitespace-pre-wrap break-all leading-relaxed m-0 p-0"
          :class="{
            'text-red-300': msg.status === 'error',
            'text-yellow-200': msg.status === 'warn',
            'text-terminal-text': msg.status === 'debug' || !msg.status,
          }"
        >{{ formatPayload(msg.payload) }}</pre>
      </div>
    </div>

    <!-- Auto-scroll indicator -->
    <div
      v-if="!autoScroll && messages.length > 0"
      class="shrink-0 border-t border-terminal-border bg-terminal-surface"
    >
      <button
        class="w-full py-1 text-[10px] text-terminal-text-dim uppercase tracking-wider font-medium
               hover:text-accent hover:bg-accent/5 transition-all duration-100 flex items-center justify-center gap-1"
        @click="autoScroll = true; scrollToBottom()"
      >
        <svg xmlns="http://www.w3.org/2000/svg" class="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M19 14l-7 7m0 0l-7-7m7 7V3" />
        </svg>
        scroll to latest
      </button>
    </div>
  </div>
</template>
