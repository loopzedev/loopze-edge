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

const funcCode = computed({
  get: () => (config.value.func as string) ?? 'return msg;',
  set: (v: string) => update('func', v),
})

const outputs = computed({
  get: () => (config.value.outputs as number) ?? 1,
  set: (v: number) => {
    if (!node.value) return
    const clamped = Math.max(1, Math.min(10, v))
    // Update both config.outputs and data.outputs so VueFlow re-renders handles.
    flowStore.updateNodeData(node.value.id, {
      config: { ...config.value, outputs: clamped },
      outputs: clamped,
    })
  },
})
</script>

<template>
  <div class="space-y-3">
    <!-- Code editor -->
    <div class="flex flex-col gap-1">
      <div class="flex items-center justify-between">
        <label class="text-[10px] text-terminal-text-dim uppercase tracking-wider">
          Function Body
        </label>
        <span class="text-[10px] text-terminal-text-dim opacity-60">JS</span>
      </div>
      <div class="relative">
        <!-- Line-number gutter -->
        <div
          aria-hidden="true"
          class="absolute top-0 left-0 bottom-0 w-6 bg-terminal-bg border-r border-terminal-border flex flex-col items-end pr-1 pt-1 pointer-events-none overflow-hidden"
        >
          <span
            v-for="i in funcCode.split('\n').length"
            :key="i"
            class="text-[9px] text-terminal-text-dim leading-[1.45rem] opacity-50 select-none"
          >{{ i }}</span>
        </div>
        <textarea
          v-model="funcCode"
          class="terminal-input w-full text-xs font-mono resize-y leading-[1.45rem] pl-8 py-1"
          style="min-height: 180px; tab-size: 2;"
          spellcheck="false"
          autocomplete="off"
          autocorrect="off"
          autocapitalize="off"
          placeholder="return msg;"
          @keydown.tab.prevent="
            (e) => {
              const el = e.target as HTMLTextAreaElement
              const start = el.selectionStart
              const end = el.selectionEnd
              funcCode = funcCode.slice(0, start) + '  ' + funcCode.slice(end)
              $nextTick(() => { el.selectionStart = el.selectionEnd = start + 2 })
            }
          "
        />
      </div>
      <p class="text-[9px] text-terminal-text-dim opacity-60">
        Available: <code class="text-amber">msg</code>,
        <code class="text-amber">node.send()</code>,
        <code class="text-amber">node.log/warn/error()</code>,
        <code class="text-amber">console.log()</code>
      </p>
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
              ? 'bg-amber text-terminal-bg border-amber'
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
