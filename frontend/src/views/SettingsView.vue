<script setup lang="ts">
import { ref } from 'vue'

const settings = ref({
  flowFile: 'flows.json',
  userDir: '~/.flint',
  httpAdminRoot: '/',
  httpNodeRoot: '/',
  debugMaxLength: 1000,
  mqttReconnectTime: 5000,
  serialReconnectTime: 5000,
})

const saved = ref(false)

function handleSave(): void {
  // TODO: POST settings to /api/v1/settings
  saved.value = true
  setTimeout(() => {
    saved.value = false
  }, 2000)
}

function handleReset(): void {
  // TODO: reload settings from backend
}
</script>

<template>
  <div class="h-full w-full overflow-y-auto bg-terminal-bg p-6 font-mono text-terminal-text">
    <!-- Page Header -->
    <div class="mb-6">
      <div class="flex items-center gap-3 mb-2">
        <router-link
          to="/"
          class="text-terminal-text-dim hover:text-accent text-xs uppercase tracking-wider transition-colors duration-100"
        >
          ← Back
        </router-link>
      </div>
      <h1 class="text-accent text-lg font-bold uppercase tracking-widest ">
        ⚙ Settings
      </h1>
      <p class="text-terminal-text-dim text-xs mt-1">
        Runtime configuration for the Flint engine
      </p>
      <div class="border-b border-terminal-border mt-3"></div>
    </div>

    <!-- Settings Form -->
    <div class="max-w-2xl space-y-6">
      <!-- Section: General -->
      <section>
        <h2 class="text-xs text-terminal-text-dim uppercase tracking-widest mb-3 flex items-center gap-2">
          <span class="w-2 h-2 bg-accent inline-block"></span>
          General
        </h2>

        <div class="space-y-3 pl-4 border-l border-terminal-border">
          <div class="flex flex-col gap-1">
            <label class="text-[10px] text-terminal-text-dim uppercase tracking-wider">
              Flow File
            </label>
            <input
              v-model="settings.flowFile"
              type="text"
              class="terminal-input w-full text-xs"
            />
          </div>

          <div class="flex flex-col gap-1">
            <label class="text-[10px] text-terminal-text-dim uppercase tracking-wider">
              User Directory
            </label>
            <input
              v-model="settings.userDir"
              type="text"
              class="terminal-input w-full text-xs"
            />
          </div>
        </div>
      </section>

      <!-- Section: HTTP -->
      <section>
        <h2 class="text-xs text-terminal-text-dim uppercase tracking-widest mb-3 flex items-center gap-2">
          <span class="w-2 h-2 bg-accent inline-block"></span>
          HTTP
        </h2>

        <div class="space-y-3 pl-4 border-l border-terminal-border">
          <div class="flex flex-col gap-1">
            <label class="text-[10px] text-terminal-text-dim uppercase tracking-wider">
              Admin Root Path
            </label>
            <input
              v-model="settings.httpAdminRoot"
              type="text"
              class="terminal-input w-full text-xs"
            />
          </div>

          <div class="flex flex-col gap-1">
            <label class="text-[10px] text-terminal-text-dim uppercase tracking-wider">
              Node Root Path
            </label>
            <input
              v-model="settings.httpNodeRoot"
              type="text"
              class="terminal-input w-full text-xs"
            />
          </div>
        </div>
      </section>

      <!-- Section: Debug -->
      <section>
        <h2 class="text-xs text-terminal-text-dim uppercase tracking-widest mb-3 flex items-center gap-2">
          <span class="w-2 h-2 bg-accent inline-block"></span>
          Debug
        </h2>

        <div class="space-y-3 pl-4 border-l border-terminal-border">
          <div class="flex flex-col gap-1">
            <label class="text-[10px] text-terminal-text-dim uppercase tracking-wider">
              Max Debug Message Length
            </label>
            <input
              v-model.number="settings.debugMaxLength"
              type="number"
              min="100"
              max="10000"
              class="terminal-input w-full text-xs"
            />
          </div>
        </div>
      </section>

      <!-- Section: Protocols -->
      <section>
        <h2 class="text-xs text-terminal-text-dim uppercase tracking-widest mb-3 flex items-center gap-2">
          <span class="w-2 h-2 bg-accent inline-block"></span>
          Protocols
        </h2>

        <div class="space-y-3 pl-4 border-l border-terminal-border">
          <div class="flex flex-col gap-1">
            <label class="text-[10px] text-terminal-text-dim uppercase tracking-wider">
              MQTT Reconnect Time (ms)
            </label>
            <input
              v-model.number="settings.mqttReconnectTime"
              type="number"
              min="1000"
              class="terminal-input w-full text-xs"
            />
          </div>

          <div class="flex flex-col gap-1">
            <label class="text-[10px] text-terminal-text-dim uppercase tracking-wider">
              Serial Reconnect Time (ms)
            </label>
            <input
              v-model.number="settings.serialReconnectTime"
              type="number"
              min="1000"
              class="terminal-input w-full text-xs"
            />
          </div>
        </div>
      </section>

      <!-- Actions -->
      <div class="flex items-center gap-3 pt-4 border-t border-terminal-border">
        <button
          class="terminal-btn-primary text-xs uppercase tracking-wider"
          @click="handleSave"
        >
          Save Settings
        </button>
        <button
          class="terminal-btn text-xs uppercase tracking-wider"
          @click="handleReset"
        >
          Reset
        </button>

        <span
          v-if="saved"
          class="text-green-400 text-xs uppercase tracking-wider ml-2 transition-opacity"
        >
          ✓ Saved
        </span>
      </div>

      <!-- System Info -->
      <section class="mt-8 pt-4 border-t border-terminal-border">
        <h2 class="text-xs text-terminal-text-dim uppercase tracking-widest mb-3 flex items-center gap-2">
          <span class="w-2 h-2 bg-terminal-text-dim inline-block"></span>
          System Information
        </h2>

        <div class="pl-4 border-l border-terminal-border space-y-1 text-xs">
          <div class="flex gap-4">
            <span class="text-terminal-text-dim w-28">Version</span>
            <span class="text-terminal-text">v0.1.0</span>
          </div>
          <div class="flex gap-4">
            <span class="text-terminal-text-dim w-28">Runtime</span>
            <span class="text-terminal-text">Go</span>
          </div>
          <div class="flex gap-4">
            <span class="text-terminal-text-dim w-28">Platform</span>
            <span class="text-terminal-text">—</span>
          </div>
          <div class="flex gap-4">
            <span class="text-terminal-text-dim w-28">Node Types</span>
            <span class="text-terminal-text">—</span>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>
