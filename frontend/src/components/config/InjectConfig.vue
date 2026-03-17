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

const once = computed({
  get: () => (config.value.once as boolean) ?? false,
  set: (v: boolean) => update('once', v),
})

const interval = computed({
  get: () => (config.value.interval as number) ?? 0,
  set: (v: number) => update('interval', v),
})

const payloadType = computed({
  get: () => (config.value.payloadType as string) ?? 'timestamp',
  set: (v: string) => update('payloadType', v),
})

const payload = computed({
  get: () => (config.value.payload as string) ?? '',
  set: (v: string) => update('payload', v),
})

const topic = computed({
  get: () => (config.value.topic as string) ?? '',
  set: (v: string) => update('topic', v),
})

const intervalPresets = [
  { label: 'None', value: 0 },
  { label: '100ms', value: 100 },
  { label: '500ms', value: 500 },
  { label: '1s', value: 1000 },
  { label: '5s', value: 5000 },
  { label: '10s', value: 10000 },
  { label: '30s', value: 30000 },
  { label: '1min', value: 60000 },
]

const payloadTypes = [
  { label: 'Timestamp', value: 'timestamp' },
  { label: 'String', value: 'string' },
  { label: 'Number', value: 'number' },
  { label: 'Boolean', value: 'boolean' },
  { label: 'JSON', value: 'json' },
]

function selectIntervalPreset(value: number) {
  interval.value = value
}
</script>

<template>
  <div class="space-y-3">
    <!-- Once at startup -->
    <div class="flex items-center gap-2">
      <input
        :checked="once"
        type="checkbox"
        class="terminal-checkbox"
        @change="once = ($event.target as HTMLInputElement).checked"
      />
      <label class="text-xs text-terminal-text cursor-pointer" @click="once = !once">
        Inject once at startup
      </label>
    </div>

    <!-- Interval -->
    <div class="flex flex-col gap-1">
      <label class="text-[10px] text-terminal-text-dim uppercase tracking-wider">
        Repeat Interval
      </label>
      <div class="flex flex-wrap gap-1 mb-1">
        <button
          v-for="preset in intervalPresets"
          :key="preset.value"
          class="px-1.5 py-0.5 text-[10px] border border-terminal-border transition-colors duration-75"
          :class="[
            interval === preset.value
              ? 'bg-amber text-terminal-bg border-amber'
              : 'bg-terminal-bg text-terminal-text-dim hover:text-terminal-text hover:border-terminal-text',
          ]"
          @click="selectIntervalPreset(preset.value)"
        >
          {{ preset.label }}
        </button>
      </div>
      <div class="flex items-center gap-1">
        <input
          :value="interval"
          type="number"
          min="0"
          step="100"
          class="terminal-input w-full text-xs"
          placeholder="Custom (ms)"
          @input="interval = Number(($event.target as HTMLInputElement).value)"
        />
        <span class="text-[10px] text-terminal-text-dim shrink-0">ms</span>
      </div>
    </div>

    <!-- Topic -->
    <div class="flex flex-col gap-0.5">
      <label class="text-[10px] text-terminal-text-dim uppercase tracking-wider">
        Topic
      </label>
      <input
        v-model="topic"
        type="text"
        class="terminal-input w-full text-xs"
        placeholder="msg.topic"
      />
    </div>

    <!-- Payload Type -->
    <div class="flex flex-col gap-0.5">
      <label class="text-[10px] text-terminal-text-dim uppercase tracking-wider">
        Payload Type
      </label>
      <select
        :value="payloadType"
        class="terminal-input w-full text-xs"
        @change="payloadType = ($event.target as HTMLSelectElement).value"
      >
        <option v-for="pt in payloadTypes" :key="pt.value" :value="pt.value">
          {{ pt.label }}
        </option>
      </select>
    </div>

    <!-- Payload Value (hidden for timestamp) -->
    <div v-if="payloadType !== 'timestamp'" class="flex flex-col gap-0.5">
      <label class="text-[10px] text-terminal-text-dim uppercase tracking-wider">
        Payload Value
      </label>
      <textarea
        v-if="payloadType === 'json'"
        v-model="payload"
        class="terminal-input w-full text-xs font-mono resize-y min-h-[60px]"
        placeholder='{"key": "value"}'
        rows="3"
      />
      <input
        v-else
        v-model="payload"
        :type="payloadType === 'number' ? 'number' : 'text'"
        class="terminal-input w-full text-xs"
        :placeholder="payloadType === 'boolean' ? 'true / false' : 'Value'"
      />
    </div>
  </div>
</template>
