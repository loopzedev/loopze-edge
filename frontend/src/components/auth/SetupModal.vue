<script setup lang="ts">
import { computed, ref } from 'vue'

import { useAuthStore } from '@/stores/authStore'
import { useApi, type ApiError } from '@/composables/useApi'

const auth = useAuthStore()
const api = useApi()

const username = ref('')
const password = ref('')
const passwordConfirm = ref('')
const submitting = ref(false)
const errorMessage = ref<string | null>(null)

const usernameTrimmed = computed(() => username.value.trim())
const passwordTooShort = computed(
  () => password.value.length > 0 && password.value.length < 8,
)
const passwordsDiffer = computed(
  () => passwordConfirm.value.length > 0 && passwordConfirm.value !== password.value,
)
const canSubmit = computed(
  () =>
    !submitting.value &&
    usernameTrimmed.value.length > 0 &&
    password.value.length >= 8 &&
    passwordConfirm.value === password.value,
)

async function handleSubmit() {
  if (!canSubmit.value) return
  submitting.value = true
  errorMessage.value = null
  try {
    await auth.setup(usernameTrimmed.value, password.value)
  } catch (err) {
    const apiErr = err as ApiError
    errorMessage.value = api.isApiError(err)
      ? apiErr.message
      : 'Setup failed; please try again.'
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
      class="w-[420px] max-w-[90vw] bg-terminal-surface border border-terminal-border rounded shadow-xl"
      @submit.prevent="handleSubmit"
    >
      <header class="px-6 py-4 border-b border-terminal-border">
        <h1 class="text-base text-terminal-text-bright font-medium">
          Welcome to LOOPZE
        </h1>
        <p class="mt-1 text-xs text-terminal-text-dim">
          No administrator account exists yet. Create the first one to continue.
        </p>
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
            autocomplete="new-password"
            :disabled="submitting"
            class="w-full bg-terminal-bg border text-terminal-text px-3 py-2 text-sm outline-none focus:border-accent transition-colors disabled:opacity-50"
            :class="passwordTooShort ? 'border-status-error' : 'border-terminal-border'"
          />
          <p
            v-if="passwordTooShort"
            class="mt-1 text-[11px] text-status-error"
          >
            Password must be at least 8 characters.
          </p>
          <p
            v-else
            class="mt-1 text-[11px] text-terminal-text-dim"
          >
            Minimum 8 characters. No other complexity rules.
          </p>
        </div>

        <div>
          <label class="block text-[10px] uppercase tracking-wider text-terminal-text-dim mb-1">
            Confirm password
          </label>
          <input
            v-model="passwordConfirm"
            type="password"
            autocomplete="new-password"
            :disabled="submitting"
            class="w-full bg-terminal-bg border text-terminal-text px-3 py-2 text-sm outline-none focus:border-accent transition-colors disabled:opacity-50"
            :class="passwordsDiffer ? 'border-status-error' : 'border-terminal-border'"
          />
          <p
            v-if="passwordsDiffer"
            class="mt-1 text-[11px] text-status-error"
          >
            Passwords do not match.
          </p>
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
          {{ submitting ? 'Creating…' : 'Create admin' }}
        </button>
      </footer>
    </form>
  </div>
</template>
