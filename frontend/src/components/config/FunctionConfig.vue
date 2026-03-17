<script setup lang="ts">
import { computed, defineAsyncComponent } from 'vue'
import { useFlowStore } from '@/stores/flowStore'

const CodeEditor = defineAsyncComponent(() =>
  import('@/components/ui/CodeEditor.vue')
)

const flowStore = useFlowStore()

const node = computed(() => flowStore.selectedNode)
const config = computed(() => (node.value?.data?.config ?? {}) as Record<string, unknown>)

function update(key: string, value: unknown) {
  if (!node.value) return
  flowStore.updateNodeData(node.value.id, {
    config: { ...config.value, [key]: value },
  })
}

const funcCode = computed({
  get: () => (config.value.func as string) ?? 'return msg;',
  set: (v: string) => update('func', v),
})

const outputs = computed({
  get: () => (config.value.outputs as number) ?? 1,
  set: (v: number) => {
    if (!node.value) return
    const clamped = Math.max(1, Math.min(10, v))
    flowStore.updateNodeData(node.value.id, {
      config: { ...config.value, outputs: clamped },
      outputs: clamped,
    })
  },
})
</script>

<template>
  <div class="flex flex-col gap-3 flex-1 min-h-0">
    <!-- Code editor -->
    <div class="flex flex-col gap-1 flex-1 min-h-0">
      <div class="flex items-center justify-between">
        <label class="text-[10px] text-terminal-text-dim uppercase tracking-wider">
          Function Body
        </label>
        <span class="text-[10px] text-terminal-text-dim opacity-60">JavaScript</span>
      </div>
      <CodeEditor
        v-model="funcCode"
        placeholder="return msg;"
        min-height="180px"
      />
    </div>

    <!-- Outputs -->
    <div class="flex flex-col gap-1">
      <label class="text-[10px] text-terminal-text-dim uppercase tracking-wider">
        Outputs
      </label>
      <div class="flex items-center gap-2">
        <input
          :value="outputs"
          type="number"
          min="1"
          max="10"
          class="terminal-input w-20 text-xs text-center"
          @input="outputs = Number(($event.target as HTMLInputElement).value)"
        />
        <span class="text-[10px] text-terminal-text-dim">port{{ outputs !== 1 ? 's' : '' }}</span>
      </div>
      <div class="flex gap-1">
        <button
          v-for="n in [1, 2, 3]"
          :key="n"
          class="px-1.5 py-0.5 text-[10px] border border-terminal-border transition-colors duration-75"
          :class="[
            outputs === n
              ? 'bg-accent text-terminal-bg border-accent'
              : 'bg-terminal-bg text-terminal-text-dim hover:text-terminal-text hover:border-terminal-text',
          ]"
          @click="outputs = n"
        >
          {{ n }}
        </button>
      </div>
    </div>
  </div>
</template>
