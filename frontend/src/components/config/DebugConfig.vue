<script setup lang="ts">
import { computed } from 'vue'
import { useFlowStore } from '@/stores/flowStore'
import FormLabel from '@/components/ui/FormLabel.vue'
import FormInput from '@/components/ui/FormInput.vue'
import FormSelect from '@/components/ui/FormSelect.vue'
import FormCheckbox from '@/components/ui/FormCheckbox.vue'

const flowStore = useFlowStore()

const node = computed(() => flowStore.selectedNode)
const config = computed(() => (node.value?.data?.config ?? {}) as Record<string, unknown>)

function update(key: string, value: unknown) {
  if (!node.value) return
  flowStore.updateNodeData(node.value.id, {
    config: { ...config.value, [key]: value },
  })
}

const output = computed({
  get: () => (config.value.output as string) ?? 'property',
  set: (v: string) => update('output', v),
})

const property = computed({
  get: () => (config.value.property as string) ?? 'payload',
  set: (v: string) => update('property', v),
})

const statusEnabled = computed({
  get: () => (config.value.statusEnabled as boolean) ?? false,
  set: (v: boolean) => update('statusEnabled', v),
})

const statusOutput = computed({
  get: () => (config.value.statusOutput as string) ?? 'same',
  set: (v: string) => update('statusOutput', v),
})

const statusProperty = computed({
  get: () => (config.value.statusProperty as string) ?? '',
  set: (v: string) => update('statusProperty', v),
})

const outputOptions = [
  { label: 'msg.', value: 'property' },
  { label: 'Komplettes Nachrichten-Objekt', value: 'message' },
  { label: 'GJSON', value: 'gjson' },
]

const statusOutputOptions = [
  { label: 'Identisch mit Debug-Ausgabe', value: 'same' },
  { label: 'msg.', value: 'property' },
  { label: 'GJSON', value: 'gjson' },
  { label: 'message count', value: 'count' },
]
</script>

<template>
  <div class="flex flex-col gap-2">
    <!-- Output mode -->
    <div class="flex flex-col gap-1">
      <FormLabel>Ausgabe</FormLabel>
      <FormSelect v-model="output" :options="outputOptions" />
    </div>

    <!-- Property path (when output === 'property') -->
    <div v-if="output === 'property'" class="flex flex-col gap-1">
      <FormLabel>msg.</FormLabel>
      <FormInput v-model="property" placeholder="payload" />
    </div>

    <!-- GJSON expression (when output === 'gjson') -->
    <div v-if="output === 'gjson'" class="flex flex-col gap-1">
      <FormLabel>GJSON Path</FormLabel>
      <FormInput v-model="property" placeholder="payload.items.#" />
    </div>

    <!-- Node Status -->
    <FormCheckbox v-model="statusEnabled" label="Node-Status (max. 32 Zeichen)" />

    <template v-if="statusEnabled">
      <div class="flex flex-col gap-1">
        <FormSelect v-model="statusOutput" :options="statusOutputOptions" />
      </div>

      <!-- Status property (when statusOutput === 'property') -->
      <div v-if="statusOutput === 'property'" class="flex flex-col gap-1">
        <FormLabel>msg.</FormLabel>
        <FormInput v-model="statusProperty" placeholder="payload" />
      </div>

      <!-- Status GJSON (when statusOutput === 'gjson') -->
      <div v-if="statusOutput === 'gjson'" class="flex flex-col gap-1">
        <FormLabel>GJSON Path</FormLabel>
        <FormInput v-model="statusProperty" placeholder="payload.items.#" />
      </div>
    </template>
  </div>
</template>
