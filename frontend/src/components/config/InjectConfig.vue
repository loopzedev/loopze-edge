<script setup lang="ts">
import { computed } from 'vue'
import { useFlowStore } from '@/stores/flowStore'
import FormLabel from '@/components/ui/FormLabel.vue'
import FormInput from '@/components/ui/FormInput.vue'
import FormSelect from '@/components/ui/FormSelect.vue'
import FormCheckbox from '@/components/ui/FormCheckbox.vue'
import ToggleGroup from '@/components/ui/ToggleGroup.vue'

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
  set: (v: string) => {
    update('payloadType', v)
    if (v === 'timestamp') {
      update('payload', null)
    }
  },
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
</script>

<template>
  <div class="flex flex-col gap-2">
    <!-- Once at startup -->
    <FormCheckbox v-model="once" label="Inject once at startup" />

    <!-- Interval -->
    <div class="flex flex-col gap-1">
      <FormLabel>Repeat Interval</FormLabel>
      <ToggleGroup v-model="interval" :options="intervalPresets" />
      <div class="flex items-center gap-1">
        <FormInput
          :model-value="String(interval)"
          type="number"
          placeholder="Custom (ms)"
          @update:model-value="interval = Number($event)"
        />
        <span class="text-[10px] text-terminal-text-dim shrink-0">ms</span>
      </div>
    </div>

    <!-- Topic -->
    <div class="flex flex-col gap-1">
      <FormLabel>Topic</FormLabel>
      <FormInput v-model="topic" placeholder="msg.topic" />
    </div>

    <!-- Payload Type -->
    <div class="flex flex-col gap-1">
      <FormLabel>Payload Type</FormLabel>
      <FormSelect v-model="payloadType" :options="payloadTypes" />
    </div>

    <!-- Payload Value (hidden for timestamp) -->
    <div v-if="payloadType !== 'timestamp'" class="flex flex-col gap-1">
      <FormLabel>Payload Value</FormLabel>
      <textarea
        v-if="payloadType === 'json'"
        v-model="payload"
        class="bg-terminal-bg border border-terminal-border text-terminal-text
               px-2 py-1 text-[10px] font-mono outline-none resize-y min-h-[60px]
               focus:border-accent focus:ring-0 placeholder:text-terminal-text-dim"
        placeholder='{"key": "value"}'
        rows="3"
      />
      <FormInput
        v-else
        v-model="payload"
        :type="payloadType === 'number' ? 'number' : 'text'"
        :placeholder="payloadType === 'boolean' ? 'true / false' : 'Value'"
      />
    </div>
  </div>
</template>
