import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { DebugMessage } from '@/types/events'
import { useFlowStore } from './flowStore'

const MAX_MESSAGES = 1000

export const useDebugStore = defineStore('debug', () => {
  // ── State ──────────────────────────────────────────────────────────
  const messages = ref<DebugMessage[]>([])
  const isEnabled = ref(true)
  const filter = ref('')

  // ── Getters ────────────────────────────────────────────────────────

  /** Set of debug node IDs whose output should be suppressed (deactivated). */
  const suppressedDebugNodeIds = computed(() => {
    const flowStore = useFlowStore()
    const ids = new Set<string>()
    for (const node of flowStore.nodes) {
      if (node.type === 'debug' && node.data?.config?.active === false) {
        ids.add(node.id)
      }
    }
    return ids
  })

  const filteredMessages = computed(() => {
    const suppressed = suppressedDebugNodeIds.value
    let result = messages.value.filter((msg) => !suppressed.has(msg.nodeId))

    if (filter.value) {
      const term = filter.value.toLowerCase()
      result = result.filter(
        (msg) =>
          msg.nodeName.toLowerCase().includes(term) ||
          msg.nodeId.toLowerCase().includes(term) ||
          String(msg.payload).toLowerCase().includes(term),
      )
    }

    return result
  })

  const messageCount = computed(() => messages.value.length)

  const filteredCount = computed(() => filteredMessages.value.length)

  // ── Actions ────────────────────────────────────────────────────────

  /**
   * Add a debug message to the store.
   * Maintains a ring-buffer of MAX_MESSAGES entries — when the limit
   * is reached the oldest message is dropped.
   */
  function addMessage(message: DebugMessage): void {
    if (!isEnabled.value) return
    if (suppressedDebugNodeIds.value.has(message.nodeId)) return

    messages.value.push(message)

    // Ring-buffer: trim from the front when we exceed the cap
    if (messages.value.length > MAX_MESSAGES) {
      messages.value = messages.value.slice(messages.value.length - MAX_MESSAGES)
    }
  }

  /** Remove all stored debug messages. */
  function clear(): void {
    messages.value = []
  }

  /** Toggle the enabled flag. When disabled, incoming messages are ignored. */
  function toggleEnabled(): void {
    isEnabled.value = !isEnabled.value
  }

  /** Programmatically set the enabled state. */
  function setEnabled(value: boolean): void {
    isEnabled.value = value
  }

  /** Set the free-text filter applied to the message list. */
  function setFilter(value: string): void {
    filter.value = value
  }

  return {
    // state
    messages,
    isEnabled,
    filter,

    // getters
    filteredMessages,
    messageCount,
    filteredCount,

    // actions
    addMessage,
    clear,
    toggleEnabled,
    setEnabled,
    setFilter,
  }
})
