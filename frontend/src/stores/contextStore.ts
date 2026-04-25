import { defineStore } from 'pinia'
import { ref } from 'vue'
import { useApi, type ContextEntry, type ContextScope, type ContextStorage } from '@/composables/useApi'

export type ContextView = 'global' | 'flow'

export interface TaggedContextEntry extends ContextEntry {
  storage: ContextStorage
}

export const useContextStore = defineStore('context', () => {
  const api = useApi()

  const view = ref<ContextView>('global')
  const flowId = ref<string | null>(null)
  const entries = ref<TaggedContextEntry[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)

  function selectView(v: ContextView) {
    view.value = v
  }

  function setFlowId(id: string | null) {
    flowId.value = id
  }

  function sortByKey(arr: TaggedContextEntry[]) {
    arr.sort((a, b) => a.key.localeCompare(b.key))
  }

  // Load both memory and persistent for a given scope, merge + tag.
  async function loadCombined(scope: ContextScope, id: string | null) {
    const [mem, pers] = await Promise.all([
      api.getContext(scope, 'memory', id),
      api.getContext(scope, 'persistent', id),
    ])
    const merged: TaggedContextEntry[] = [
      ...(mem.entries ?? []).map(e => ({ ...e, storage: 'memory' as const })),
      ...(pers.entries ?? []).map(e => ({ ...e, storage: 'persistent' as const })),
    ]
    sortByKey(merged)
    entries.value = merged
  }

  async function loadAll() {
    error.value = null

    if (view.value === 'flow' && !flowId.value) {
      entries.value = []
      return
    }

    loading.value = true
    try {
      const scope: ContextScope = view.value
      const id = scope === 'flow' ? flowId.value : null
      await loadCombined(scope, id)
    } catch (err) {
      error.value = api.isApiError(err) ? err.message : 'Failed to load context'
      entries.value = []
    } finally {
      loading.value = false
    }
  }

  async function loadKey(entry: TaggedContextEntry) {
    const scope: ContextScope = view.value
    const id = scope === 'flow' ? flowId.value : null
    if (scope === 'flow' && !id) return
    try {
      const fresh = await api.getContextKey(scope, entry.storage, entry.key, id)
      const idx = entries.value.findIndex(e => e.key === entry.key && e.storage === entry.storage)
      const tagged: TaggedContextEntry = { ...fresh, storage: entry.storage }
      if (idx >= 0) entries.value[idx] = tagged
      else {
        entries.value.push(tagged)
        sortByKey(entries.value)
      }
    } catch (err) {
      if (api.isApiError(err) && err.status === 404) {
        entries.value = entries.value.filter(
          e => !(e.key === entry.key && e.storage === entry.storage),
        )
        return
      }
      error.value = api.isApiError(err) ? err.message : 'Failed to refresh key'
    }
  }

  async function deleteKey(entry: TaggedContextEntry) {
    const scope: ContextScope = view.value
    const id = scope === 'flow' ? flowId.value : null
    if (scope === 'flow' && !id) return
    try {
      await api.deleteContextKey(scope, entry.storage, entry.key, id)
      entries.value = entries.value.filter(
        e => !(e.key === entry.key && e.storage === entry.storage),
      )
    } catch (err) {
      error.value = api.isApiError(err) ? err.message : 'Failed to delete key'
    }
  }

  async function clearAll() {
    error.value = null
    const scope: ContextScope = view.value
    const id = scope === 'flow' ? flowId.value : null
    if (scope === 'flow' && !id) return
    try {
      await Promise.all([
        api.clearContext(scope, 'memory', id),
        api.clearContext(scope, 'persistent', id),
      ])
      entries.value = []
    } catch (err) {
      error.value = api.isApiError(err) ? err.message : 'Failed to clear context'
    }
  }

  return {
    view,
    flowId,
    entries,
    loading,
    error,
    selectView,
    setFlowId,
    loadAll,
    loadKey,
    deleteKey,
    clearAll,
  }
})
