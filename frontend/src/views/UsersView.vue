<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'

import { useAuthStore } from '@/stores/authStore'
import { useApi, type ApiError } from '@/composables/useApi'
import type { Role, User } from '@/types/auth'

const auth = useAuthStore()
const api = useApi()

// ── Table data ───────────────────────────────────────────────────────────
const users = ref<User[]>([])
const loading = ref(true)
const tableError = ref<string | null>(null)

async function reload() {
  loading.value = true
  tableError.value = null
  try {
    users.value = await api.listUsers()
  } catch (err) {
    tableError.value = api.isApiError(err)
      ? (err as ApiError).message
      : 'Failed to load users.'
  } finally {
    loading.value = false
  }
}

onMounted(reload)

// ── Per-row inline actions ───────────────────────────────────────────────
async function applyUpdate(u: User, patch: { role?: Role; disabled?: boolean }) {
  try {
    const updated = await api.updateUser(u.id, patch)
    const i = users.value.findIndex((x) => x.id === u.id)
    if (i !== -1) users.value[i] = updated
  } catch (err) {
    const apiErr = err as ApiError
    const msg = api.isApiError(err) ? apiErr.message : 'Update failed.'
    // Re-fetch on conflict so the UI reflects the actual server state.
    if (apiErr.status === 409) await reload()
    window.alert(msg)
  }
}

async function changeRole(u: User, newRole: Role) {
  if (newRole === u.role) return
  // Promoting to admin is a meaningful action; require an explicit
  // confirmation. Demoting / lateral changes go through silently.
  if (newRole === 'admin' && !window.confirm(
    `Promote "${u.username}" to admin? Admins can manage all users.`,
  )) {
    return
  }
  await applyUpdate(u, { role: newRole })
}

async function toggleDisabled(u: User) {
  await applyUpdate(u, { disabled: !u.disabled })
}

// ── Create-user modal ────────────────────────────────────────────────────
const showCreate = ref(false)
const createUsername = ref('')
const createPassword = ref('')
const createRole = ref<Role>('viewer')
const createSubmitting = ref(false)
const createError = ref<string | null>(null)

const canSubmitCreate = computed(
  () =>
    !createSubmitting.value &&
    createUsername.value.trim().length > 0 &&
    createPassword.value.length >= 8,
)

function openCreate() {
  createUsername.value = ''
  createPassword.value = ''
  createRole.value = 'viewer'
  createError.value = null
  showCreate.value = true
}

async function submitCreate() {
  if (!canSubmitCreate.value) return
  if (createRole.value === 'admin' && !window.confirm(
    'Create as admin? Admins can manage all users.',
  )) {
    return
  }
  createSubmitting.value = true
  createError.value = null
  try {
    const u = await api.createUser(
      createUsername.value.trim(),
      createPassword.value,
      createRole.value,
    )
    users.value.push(u)
    users.value.sort((a, b) => a.username.localeCompare(b.username))
    showCreate.value = false
  } catch (err) {
    createError.value = api.isApiError(err)
      ? (err as ApiError).message
      : 'Failed to create user.'
  } finally {
    createSubmitting.value = false
  }
}

// ── Set-password modal ───────────────────────────────────────────────────
const passwordTarget = ref<User | null>(null)
const passwordValue = ref('')
const passwordSubmitting = ref(false)
const passwordError = ref<string | null>(null)

const canSubmitPassword = computed(
  () => !passwordSubmitting.value && passwordValue.value.length >= 8,
)

function openPasswordReset(u: User) {
  passwordTarget.value = u
  passwordValue.value = ''
  passwordError.value = null
}

async function submitPassword() {
  if (!passwordTarget.value || !canSubmitPassword.value) return
  passwordSubmitting.value = true
  passwordError.value = null
  try {
    await api.setUserPassword(passwordTarget.value.id, passwordValue.value)
    passwordTarget.value = null
  } catch (err) {
    passwordError.value = api.isApiError(err)
      ? (err as ApiError).message
      : 'Failed to set password.'
  } finally {
    passwordSubmitting.value = false
  }
}

const ROLE_OPTIONS: Role[] = ['viewer', 'editor', 'admin']

function isSelf(u: User): boolean {
  return auth.user?.id === u.id
}
</script>

<template>
  <div class="h-full flex flex-col bg-terminal-bg text-terminal-text overflow-hidden">
    <header class="flex items-center justify-between px-6 py-4 border-b border-terminal-border">
      <div>
        <h1 class="text-base text-terminal-text-bright font-medium">Users</h1>
        <p class="text-xs text-terminal-text-dim mt-0.5">
          Manage local accounts. Disabled users cannot sign in; sessions are
          revoked on disable and on password reset.
        </p>
      </div>
      <div class="flex items-center gap-2">
        <router-link
          to="/"
          class="px-3 py-1.5 text-xs text-terminal-text-dim hover:text-terminal-text border border-terminal-border rounded-sm hover:bg-terminal-surface-alt"
        >
          Back to editor
        </router-link>
        <button
          class="px-3 py-1.5 text-xs bg-accent text-terminal-bg font-medium rounded-sm hover:bg-accent-dim"
          @click="openCreate"
        >
          + New user
        </button>
      </div>
    </header>

    <div class="flex-1 overflow-auto p-6">
      <div v-if="tableError" class="mb-4 text-xs text-status-error bg-status-error/10 border border-status-error/40 px-3 py-2 rounded-sm">
        {{ tableError }}
      </div>

      <div v-if="loading" class="text-terminal-text-dim text-sm">Loading…</div>

      <table v-else class="w-full text-xs border border-terminal-border">
        <thead class="bg-terminal-surface text-terminal-text-dim uppercase text-[10px] tracking-wider">
          <tr>
            <th class="text-left px-3 py-2">Username</th>
            <th class="text-left px-3 py-2">Role</th>
            <th class="text-left px-3 py-2">Provider</th>
            <th class="text-left px-3 py-2">Status</th>
            <th class="text-left px-3 py-2">Created</th>
            <th class="text-right px-3 py-2">Actions</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="u in users"
            :key="u.id"
            class="border-t border-terminal-border hover:bg-terminal-surface/50"
          >
            <td class="px-3 py-2 font-mono text-terminal-text">
              {{ u.username }}
              <span v-if="isSelf(u)" class="ml-1 text-[10px] text-terminal-text-dim">(you)</span>
            </td>
            <td class="px-3 py-2">
              <select
                :value="u.role"
                class="bg-terminal-bg border border-terminal-border text-terminal-text px-2 py-1 text-xs focus:border-accent outline-none"
                @change="changeRole(u, ($event.target as HTMLSelectElement).value as Role)"
              >
                <option v-for="r in ROLE_OPTIONS" :key="r" :value="r">{{ r }}</option>
              </select>
            </td>
            <td class="px-3 py-2 text-terminal-text-dim">{{ u.authProvider }}</td>
            <td class="px-3 py-2">
              <button
                class="px-2 py-0.5 text-[10px] uppercase tracking-wider rounded-sm border"
                :class="u.disabled
                  ? 'border-status-error/40 text-status-error bg-status-error/10'
                  : 'border-status-success/40 text-status-success bg-status-success/10'"
                @click="toggleDisabled(u)"
              >
                {{ u.disabled ? 'Disabled' : 'Active' }}
              </button>
            </td>
            <td class="px-3 py-2 text-terminal-text-dim font-mono">
              {{ new Date(u.createdAt).toISOString().slice(0, 10) }}
            </td>
            <td class="px-3 py-2 text-right">
              <button
                class="px-2 py-1 text-[11px] text-terminal-text-dim hover:text-terminal-text border border-terminal-border rounded-sm hover:bg-terminal-surface-alt"
                @click="openPasswordReset(u)"
              >
                Reset password
              </button>
            </td>
          </tr>
          <tr v-if="users.length === 0">
            <td colspan="6" class="px-3 py-6 text-center text-terminal-text-dim">
              No users yet.
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Create-user modal -->
    <div
      v-if="showCreate"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-sm"
    >
      <form
        class="w-[400px] max-w-[90vw] bg-terminal-surface border border-terminal-border rounded shadow-xl"
        @submit.prevent="submitCreate"
      >
        <header class="px-6 py-4 border-b border-terminal-border">
          <h2 class="text-base text-terminal-text-bright font-medium">New user</h2>
        </header>
        <div class="px-6 py-5 space-y-4">
          <div>
            <label class="block text-[10px] uppercase tracking-wider text-terminal-text-dim mb-1">Username</label>
            <input
              v-model="createUsername"
              type="text"
              autofocus
              :disabled="createSubmitting"
              class="w-full bg-terminal-bg border border-terminal-border text-terminal-text px-3 py-2 text-sm outline-none focus:border-accent disabled:opacity-50"
            />
          </div>
          <div>
            <label class="block text-[10px] uppercase tracking-wider text-terminal-text-dim mb-1">Password</label>
            <input
              v-model="createPassword"
              type="password"
              autocomplete="new-password"
              :disabled="createSubmitting"
              class="w-full bg-terminal-bg border border-terminal-border text-terminal-text px-3 py-2 text-sm outline-none focus:border-accent disabled:opacity-50"
            />
            <p class="mt-1 text-[11px] text-terminal-text-dim">Minimum 8 characters.</p>
          </div>
          <div>
            <label class="block text-[10px] uppercase tracking-wider text-terminal-text-dim mb-1">Role</label>
            <select
              v-model="createRole"
              :disabled="createSubmitting"
              class="w-full bg-terminal-bg border border-terminal-border text-terminal-text px-3 py-2 text-sm outline-none focus:border-accent disabled:opacity-50"
            >
              <option value="viewer">viewer — read-only</option>
              <option value="editor">editor — deploy & run</option>
              <option value="admin">admin — full control</option>
            </select>
          </div>
          <div v-if="createError" class="text-[12px] text-status-error bg-status-error/10 border border-status-error/40 px-3 py-2 rounded-sm">
            {{ createError }}
          </div>
        </div>
        <footer class="px-6 py-4 border-t border-terminal-border flex justify-end gap-2">
          <button
            type="button"
            :disabled="createSubmitting"
            class="px-3 py-1.5 text-sm text-terminal-text-dim hover:text-terminal-text border border-terminal-border rounded-sm hover:bg-terminal-surface-alt disabled:opacity-50"
            @click="showCreate = false"
          >
            Cancel
          </button>
          <button
            type="submit"
            :disabled="!canSubmitCreate"
            class="px-4 py-1.5 text-sm bg-accent text-terminal-bg font-medium rounded-sm hover:bg-accent-dim disabled:opacity-40 disabled:cursor-not-allowed"
          >
            {{ createSubmitting ? 'Creating…' : 'Create' }}
          </button>
        </footer>
      </form>
    </div>

    <!-- Reset-password modal -->
    <div
      v-if="passwordTarget"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-sm"
    >
      <form
        class="w-[400px] max-w-[90vw] bg-terminal-surface border border-terminal-border rounded shadow-xl"
        @submit.prevent="submitPassword"
      >
        <header class="px-6 py-4 border-b border-terminal-border">
          <h2 class="text-base text-terminal-text-bright font-medium">
            Reset password
          </h2>
          <p class="text-xs text-terminal-text-dim mt-0.5">
            For <span class="font-mono">{{ passwordTarget.username }}</span>.
            Active sessions will be terminated.
          </p>
        </header>
        <div class="px-6 py-5 space-y-4">
          <div>
            <label class="block text-[10px] uppercase tracking-wider text-terminal-text-dim mb-1">New password</label>
            <input
              v-model="passwordValue"
              type="password"
              autocomplete="new-password"
              autofocus
              :disabled="passwordSubmitting"
              class="w-full bg-terminal-bg border border-terminal-border text-terminal-text px-3 py-2 text-sm outline-none focus:border-accent disabled:opacity-50"
            />
            <p class="mt-1 text-[11px] text-terminal-text-dim">Minimum 8 characters.</p>
          </div>
          <div v-if="passwordError" class="text-[12px] text-status-error bg-status-error/10 border border-status-error/40 px-3 py-2 rounded-sm">
            {{ passwordError }}
          </div>
        </div>
        <footer class="px-6 py-4 border-t border-terminal-border flex justify-end gap-2">
          <button
            type="button"
            :disabled="passwordSubmitting"
            class="px-3 py-1.5 text-sm text-terminal-text-dim hover:text-terminal-text border border-terminal-border rounded-sm hover:bg-terminal-surface-alt disabled:opacity-50"
            @click="passwordTarget = null"
          >
            Cancel
          </button>
          <button
            type="submit"
            :disabled="!canSubmitPassword"
            class="px-4 py-1.5 text-sm bg-accent text-terminal-bg font-medium rounded-sm hover:bg-accent-dim disabled:opacity-40 disabled:cursor-not-allowed"
          >
            {{ passwordSubmitting ? 'Saving…' : 'Save' }}
          </button>
        </footer>
      </form>
    </div>
  </div>
</template>
