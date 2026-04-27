import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

import { setApiHooks, useApi, type ApiError } from '@/composables/useApi'
import { roleHasAction, type AuthAction, type Role, type User } from '@/types/auth'

/**
 * Auth state for the editor SPA.
 *
 *   - `loading=true` until init() has resolved the very first /auth/me call.
 *     Components should suspend rendering during this phase so they don't
 *     paint a half-authenticated UI.
 *   - `needsSetup=true` when the backend returned 503 (no admin yet).
 *     The Setup modal listens to this and blocks the UI.
 *   - `user=null` after init() if there is no valid session, or after a
 *     401 anywhere in the app (handled centrally via setApiHooks).
 *
 * One-time wiring: registerHooks() is called from init() so that *any*
 * subsequent API request that returns 401/503 funnels back into the
 * store without each call site needing to handle it.
 */
export const useAuthStore = defineStore('auth', () => {
  const api = useApi()

  const user = ref<User | null>(null)
  const loading = ref<boolean>(true)
  const needsSetup = ref<boolean>(false)

  let hooksRegistered = false

  function registerHooks(): void {
    if (hooksRegistered) return
    hooksRegistered = true
    setApiHooks({
      onUnauthorized: () => {
        user.value = null
      },
      onSetupRequired: () => {
        needsSetup.value = true
      },
    })
  }

  async function init(): Promise<void> {
    registerHooks()
    loading.value = true
    try {
      // Single call returns both pieces of state, so we don't have to
      // disambiguate between "no admin yet" (needsSetup) and "logged
      // out" (regular 401). The endpoint is open and always returns 200.
      const status = await api.getAuthStatus()
      needsSetup.value = status.needsSetup
      user.value = status.user ?? null
    } catch (err) {
      console.error('authStore.init failed', err as ApiError)
    } finally {
      loading.value = false
    }
  }

  async function setup(username: string, password: string): Promise<void> {
    const u = await api.setupAdmin(username, password)
    user.value = u
    needsSetup.value = false
  }

  async function login(username: string, password: string): Promise<void> {
    const u = await api.login(username, password)
    user.value = u
  }

  async function logout(): Promise<void> {
    try {
      await api.logout()
    } finally {
      user.value = null
    }
  }

  // ── Getters ────────────────────────────────────────────────────────────

  const isAuthenticated = computed<boolean>(() => user.value !== null)

  const role = computed<Role | null>(() => user.value?.role ?? null)

  function can(action: AuthAction): boolean {
    return user.value !== null && roleHasAction(user.value.role, action)
  }

  return {
    // state
    user,
    loading,
    needsSetup,
    // actions
    init,
    setup,
    login,
    logout,
    // getters
    isAuthenticated,
    role,
    can,
  }
})
