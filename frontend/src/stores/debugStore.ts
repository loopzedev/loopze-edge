import { defineStore } from 'pinia'
import { computed, markRaw, ref, shallowRef, triggerRef } from 'vue'
import type { DebugMessage } from '@/types/events'
import { useFlowStore } from './flowStore'

const MAX_MESSAGES = 1000

interface MessageSearchIndex {
  nodeName: string
  nodeId: string
  payload?: string
}

export const useDebugStore = defineStore('debug', () => {
  // ── State ──────────────────────────────────────────────────────────
  // shallowRef + markRaw on entries: messages are immutable after insert,
  // so we skip Vue's deep proxy installation entirely. Mutations are
  // signaled explicitly via triggerRef.
  const messages = shallowRef<DebugMessage[]>([])
  const isEnabled = ref(true)
  const filter = ref('')

  // Pinned paths per nodeId. A pinned path forces the JsonTreeView in every
  // message of that node to auto-expand all containers along the path.
  const pinnedPaths = ref<Map<string, Set<string>>>(new Map())
  // Bumped on every pin change. Used as v-memo dependency so existing message
  // rows re-render and pick up the new pin state.
  const pinnedPathsVersion = ref(0)
  const EMPTY_PIN_SET: ReadonlySet<string> = new Set()

  // Lazy lowercase search index, keyed by message identity. Built once per
  // message on first filter pass; not reactive (would defeat the purpose).
  const searchIndex = new WeakMap<DebugMessage, MessageSearchIndex>()

  function indexFor(msg: DebugMessage): MessageSearchIndex {
    let entry = searchIndex.get(msg)
    if (!entry) {
      entry = {
        nodeName: msg.nodeName.toLowerCase(),
        nodeId: msg.nodeId.toLowerCase(),
      }
      searchIndex.set(msg, entry)
    }
    return entry
  }

  function payloadSearchString(msg: DebugMessage): string {
    const entry = indexFor(msg)
    if (entry.payload !== undefined) return entry.payload
    const p = msg.payload
    let str: string
    if (p === null || p === undefined) {
      str = ''
    } else if (typeof p === 'object') {
      try {
        str = JSON.stringify(p).toLowerCase()
      } catch {
        str = ''
      }
    } else {
      str = String(p).toLowerCase()
    }
    entry.payload = str
    return str
  }

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
    const all = messages.value
    const term = filter.value.toLowerCase()

    const result: DebugMessage[] = []
    for (let i = 0; i < all.length; i++) {
      const msg = all[i]
      if (suppressed.has(msg.nodeId)) continue
      if (term) {
        const idx = indexFor(msg)
        if (
          !idx.nodeName.includes(term) &&
          !idx.nodeId.includes(term) &&
          !payloadSearchString(msg).includes(term)
        ) {
          continue
        }
      }
      result.push(msg)
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

    // Defensive: backend should always provide an ID, but if missing, generate
    // a stable one so :key / v-memo can identify the row uniquely.
    if (!message.id) message.id = crypto.randomUUID()

    // markRaw prevents Vue from wrapping the (immutable) payload tree in proxies.
    const arr = messages.value
    arr.push(markRaw(message))

    if (arr.length > MAX_MESSAGES) {
      arr.splice(0, arr.length - MAX_MESSAGES)
    }
    triggerRef(messages)
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

  /** Read-only view of pinned paths for a given node. */
  function pinnedPathsForNode(nodeId: string): ReadonlySet<string> {
    return pinnedPaths.value.get(nodeId) ?? EMPTY_PIN_SET
  }

  /** Toggle a path pin for a node. Multiple pins per node are allowed. */
  function togglePinnedPath(nodeId: string, path: string): void {
    const map = new Map(pinnedPaths.value)
    const existing = map.get(nodeId)
    if (existing && existing.has(path)) {
      const next = new Set(existing)
      next.delete(path)
      if (next.size === 0) map.delete(nodeId)
      else map.set(nodeId, next)
    } else {
      const next = new Set(existing ?? [])
      next.add(path)
      map.set(nodeId, next)
    }
    pinnedPaths.value = map
    pinnedPathsVersion.value++
  }

  return {
    // state
    messages,
    isEnabled,
    filter,
    pinnedPaths,
    pinnedPathsVersion,

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
    pinnedPathsForNode,
    togglePinnedPath,
  }
})
