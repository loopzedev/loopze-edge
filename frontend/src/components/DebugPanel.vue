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
  <div class="flex flex-col h-full bg-terminal-bg font-mono text-xs">
    <!-- Toolbar -->
    <div class="flex items-center gap-1.5 px-2 py-1.5 border-b border-terminal-border bg-terminal-surface shrink-0">
      <input
        v-model="filterText"
        type="text"
        placeholder="filter..."
        class="flex-1 bg-terminal-bg border border-terminal-border text-terminal-text
               px-1.5 py-0.5 font-mono text-xs outline-none
               focus:border-amber placeholder:text-terminal-text-dim"
      />

      <button
        class="px-1.5 py-0.5 border text-[10px] uppercase tracking-wider font-bold transition-colors duration-100"
        :class="debugStore.isEnabled
          ? 'border-green-600 text-green-400 hover:bg-green-900/30'
          : 'border-terminal-border text-terminal-text-dim hover:bg-terminal-border'"
        :title="debugStore.isEnabled ? 'Pause debug output' : 'Resume debug output'"
        @click="toggleEnabled"
      >
        {{ debugStore.isEnabled ? 'ON' : 'OFF' }}
      </button>

      <button
        class="px-1.5 py-0.5 border border-terminal-border text-terminal-text-dim
               text-[10px] uppercase tracking-wider font-bold
               hover:bg-terminal-border hover:text-terminal-text transition-colors duration-100"
        title="Clear all messages"
        @click="clearMessages"
      >
        CLR
      </button>

      <span class="text-terminal-text-dim text-[10px] ml-1 whitespace-nowrap">
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
        <span class="text-terminal-text-dim text-xs">
          {{ debugStore.isEnabled ? '— no debug messages —' : '— debug paused —' }}
        </span>
      </div>

      <div
        v-for="msg in messages"
        :key="msg.id"
        class="border-b px-2 py-1.5 transition-colors duration-75"
        :class="{
          'border-terminal-border/50 hover:bg-terminal-surface/60': msg.status === 'debug' || !msg.status,
          'border-red-900/50 bg-red-950/30 hover:bg-red-950/50': msg.status === 'error',
          'border-yellow-900/50 bg-yellow-950/20 hover:bg-yellow-950/40': msg.status === 'warn',
        }"
      >
        <!-- Header line: timestamp + status + node name -->
        <div class="flex items-center gap-2 mb-0.5">
          <span class="text-terminal-text-dim text-[10px] shrink-0">
            {{ formatTimestamp(msg.timestamp) }}
          </span>
          <span
            v-if="msg.status === 'error'"
            class="text-red-400 text-[10px] font-bold uppercase tracking-wide shrink-0"
          >ERR</span>
          <span
            v-else-if="msg.status === 'warn'"
            class="text-yellow-400 text-[10px] font-bold uppercase tracking-wide shrink-0"
          >WRN</span>
          <span
            class="text-[10px] font-bold uppercase tracking-wide truncate"
            :class="{
              'text-red-400': msg.status === 'error',
              'text-yellow-400': msg.status === 'warn',
              'text-amber': msg.status === 'debug' || !msg.status,
            }"
          >
            {{ msg.nodeName || msg.nodeId }}
          </span>
          <span
            v-if="msg.property && msg.property !== 'payload'"
            class="text-terminal-text-dim text-[10px]"
          >
            .{{ msg.property }}
          </span>
          <span class="ml-auto text-terminal-text-dim text-[10px] shrink-0 uppercase">
            {{ msg.format }}
          </span>
        </div>

        <!-- Payload -->
        <pre
          class="text-xs whitespace-pre-wrap break-all leading-snug m-0 p-0"
          :class="{
            'text-red-300': msg.status === 'error',
            'text-yellow-300': msg.status === 'warn',
            'text-terminal-text-bright': msg.status === 'debug' || !msg.status,
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
        class="w-full py-0.5 text-[10px] text-terminal-text-dim uppercase tracking-wider
               hover:text-amber hover:bg-terminal-border/40 transition-colors duration-100"
        @click="autoScroll = true; scrollToBottom()"
      >
        ▼ scroll to latest
      </button>
    </div>
  </div>
</template>
