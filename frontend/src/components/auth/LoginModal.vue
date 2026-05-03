<script setup lang="ts">
import { computed, ref } from 'vue'

import { useAuthStore } from '@/stores/authStore'
import { useApi, type ApiError } from '@/composables/useApi'

const auth = useAuthStore()
const api = useApi()

const username = ref('')
const password = ref('')
const submitting = ref(false)
const errorMessage = ref<string | null>(null)

const canSubmit = computed(
  () => !submitting.value && username.value.trim().length > 0 && password.value.length > 0,
)

async function handleSubmit() {
  if (!canSubmit.value) return
  submitting.value = true
  errorMessage.value = null
  try {
    await auth.login(username.value.trim(), password.value)
  } catch (err) {
    if (api.isApiError(err)) {
      const apiErr = err as ApiError
      // Map server status codes to user-facing messages. The server
      // intentionally returns the same "invalid credentials" text for
      // wrong password, missing user, and disabled user — so the
      // attacker cannot enumerate accounts.
      if (apiErr.status === 401) {
        errorMessage.value = 'Invalid username or password.'
      } else if (apiErr.status === 429) {
        errorMessage.value =
          'Too many failed attempts. Please wait a few minutes before trying again.'
      } else {
        errorMessage.value = apiErr.message || 'Login failed.'
      }
    } else {
      errorMessage.value = 'Login failed; please try again.'
    }
    // Clear the password field on any error so a stuck-keys keystroke
    // does not slowly walk through wrong passwords.
    password.value = ''
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div
    class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-sm"
  >
    <form
      class="w-[380px] max-w-[90vw] bg-terminal-surface border border-terminal-border rounded shadow-xl"
      @submit.prevent="handleSubmit"
    >
      <header class="px-6 py-6 border-b border-terminal-border flex justify-center">
        <div class="flex items-center gap-3">
          <svg xmlns="http://www.w3.org/2000/svg" class="w-12 h-12 text-accent" viewBox="0 0 24 24" fill="currentColor">
            <path d="M13 2L3 14h8l-1 8 10-12h-8l1-8z" />
          </svg>
          <span class="text-accent text-4xl font-bold tracking-widest font-mono">LOOPZE</span>
        </div>
      </header>

      <div class="px-6 py-5 space-y-4">
        <div>
          <label class="block text-[10px] uppercase tracking-wider text-terminal-text-dim mb-1">
            Username
          </label>
          <input
            v-model="username"
            type="text"
            autocomplete="username"
            autofocus
            :disabled="submitting"
            class="w-full bg-terminal-bg border border-terminal-border text-terminal-text px-3 py-2 text-sm outline-none focus:border-accent transition-colors disabled:opacity-50"
          />
        </div>

        <div>
          <label class="block text-[10px] uppercase tracking-wider text-terminal-text-dim mb-1">
            Password
          </label>
          <input
            v-model="password"
            type="password"
            autocomplete="current-password"
            :disabled="submitting"
            class="w-full bg-terminal-bg border border-terminal-border text-terminal-text px-3 py-2 text-sm outline-none focus:border-accent transition-colors disabled:opacity-50"
          />
        </div>

        <div
          v-if="errorMessage"
          class="text-[12px] text-status-error bg-status-error/10 border border-status-error/40 px-3 py-2 rounded-sm"
        >
          {{ errorMessage }}
        </div>
      </div>

      <footer class="px-6 py-4 border-t border-terminal-border flex justify-end">
        <button
          type="submit"
          :disabled="!canSubmit"
          class="px-4 py-1.5 text-sm bg-accent text-terminal-bg font-medium rounded-sm hover:bg-accent-dim transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
        >
          {{ submitting ? 'Signing in…' : 'Sign in' }}
        </button>
      </footer>
    </form>
  </div>
</template>
