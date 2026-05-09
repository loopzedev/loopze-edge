import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

import { useApi, type ApiError } from '@/composables/useApi'
import type { CertEntryInput, CertEntrySummary, CertType } from '@/types/cert'

/**
 * Cert-store state for the editor SPA.
 *
 * Maintains a cached list of stored certificates so node config panels
 * can populate selectors without each one hitting the backend
 * independently. The list is invalidated on every mutation; concurrent
 * editors (rare, but possible) refresh on demand via reload().
 */
export const useCertsStore = defineStore('certs', () => {
  const api = useApi()

  const certs = ref<CertEntrySummary[]>([])
  const loading = ref(false)
  const loaded = ref(false)
  const error = ref<string | null>(null)

  function failureMessage(err: unknown, fallback: string): string {
    if (api.isApiError(err)) {
      return (err as ApiError).message || fallback
    }
    return fallback
  }

  async function reload(): Promise<void> {
    loading.value = true
    error.value = null
    try {
      certs.value = await api.listCerts()
      loaded.value = true
    } catch (err) {
      error.value = failureMessage(err, 'Failed to load certificates.')
    } finally {
      loading.value = false
    }
  }

  /** Lazy-load: callers (selectors, the manager view) hit ensureLoaded
   *  on mount; subsequent calls reuse the cached list. */
  async function ensureLoaded(): Promise<void> {
    if (loaded.value || loading.value) return
    await reload()
  }

  async function create(entry: CertEntryInput): Promise<CertEntrySummary> {
    const created = await api.createCert(entry)
    certs.value = [...certs.value, created].sort(byId)
    return created
  }

  async function update(id: string, entry: CertEntryInput): Promise<CertEntrySummary> {
    const updated = await api.updateCert(id, entry)
    certs.value = certs.value.map((c) => (c.id === id ? updated : c))
    return updated
  }

  async function remove(id: string): Promise<void> {
    await api.deleteCert(id)
    certs.value = certs.value.filter((c) => c.id !== id)
  }

  /** Filter the cache by cert type. Used by CertSelector to populate a
   *  type-appropriate dropdown (e.g. only ca-bundle entries for the CA
   *  slot, only client-pair entries for the client-pair slot). */
  const byType = computed(() => (type: CertType): CertEntrySummary[] =>
    certs.value.filter((c) => c.type === type),
  )

  return {
    certs,
    loading,
    loaded,
    error,
    reload,
    ensureLoaded,
    create,
    update,
    remove,
    byType,
  }
})

function byId(a: CertEntrySummary, b: CertEntrySummary): number {
  return a.id.localeCompare(b.id)
}
