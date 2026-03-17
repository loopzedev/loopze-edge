<script setup lang="ts">
import { computed } from 'vue'
import { useFlowStore } from '@/stores/flowStore'

const flowStore = useFlowStore()

const node = computed(() => flowStore.selectedNode)
const config = computed(() => (node.value?.data?.config ?? {}) as Record<string, unknown>)

function update(key: string, value: unknown) {
  if (!node.value) return
  flowStore.updateNodeData(node.value.id, {
    config: { ...config.value, [key]: value },
  })
}

const scope = computed({
  get: () => (config.value.scope as string) ?? 'global',
  set: (v: string) => update('scope', v),
})

const storage = computed({
  get: () => (config.value.storage as string) ?? 'memory',
  set: (v: string) => update('storage', v),
})

const keyPattern = computed({
  get: () => (config.value.keyPattern as string) ?? '>',
  set: (v: string) => update('keyPattern', v),
})

const emitDeletes = computed({
  get: () => (config.value.emitDeletes as boolean) ?? false,
  set: (v: boolean) => update('emitDeletes', v),
})
</script>

<template>
  <div class="space-y-3">
    <!-- Scope -->
    <div class="flex flex-col gap-1">
      <label class="text-[10px] text-terminal-text-dim uppercase tracking-wider">
        Scope
      </label>
      <div class="flex gap-1">
        <button
          v-for="s in ['global', 'flow']"
          :key="s"
          class="flex-1 px-2 py-1 text-[10px] uppercase tracking-wider border border-terminal-border transition-colors"
          :class="scope === s
            ? 'bg-accent text-terminal-bg border-accent'
            : 'bg-terminal-bg text-terminal-text-dim hover:text-terminal-text hover:border-terminal-text'"
          @click="scope = s"
        >
          {{ s }}
        </button>
      </div>
    </div>

    <!-- Storage -->
    <div class="flex flex-col gap-1">
      <label class="text-[10px] text-terminal-text-dim uppercase tracking-wider">
        Storage
      </label>
      <div class="flex gap-1">
        <button
          v-for="s in ['memory', 'persistent']"
          :key="s"
          class="flex-1 px-2 py-1 text-[10px] uppercase tracking-wider border border-terminal-border transition-colors"
          :class="storage === s
            ? 'bg-accent text-terminal-bg border-accent'
            : 'bg-terminal-bg text-terminal-text-dim hover:text-terminal-text hover:border-terminal-text'"
          @click="storage = s"
        >
          {{ s }}
        </button>
      </div>
    </div>

    <!-- Key Pattern -->
    <div class="flex flex-col gap-0.5">
      <label class="text-[10px] text-terminal-text-dim uppercase tracking-wider">
        Key Pattern
      </label>
      <input
        v-model="keyPattern"
        type="text"
        class="terminal-input w-full text-xs font-mono"
        placeholder=">"
      />
      <span class="text-[9px] text-terminal-text-dim">
        Use <code class="font-mono">&gt;</code> for all keys, or a specific key name
      </span>
    </div>

    <!-- Emit Deletes -->
    <div class="flex items-center gap-2">
      <input
        :checked="emitDeletes"
        type="checkbox"
        @change="emitDeletes = ($event.target as HTMLInputElement).checked"
      />
      <label class="text-xs text-terminal-text cursor-pointer" @click="emitDeletes = !emitDeletes">
        Emit delete operations
      </label>
    </div>
  </div>
</template>
