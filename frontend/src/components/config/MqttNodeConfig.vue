<script setup lang="ts">
import { computed } from 'vue'
import { useFlowStore } from '@/stores/flowStore'
import { useConfigSelector } from '@/composables/useConfigSelector'
import FormLabel from '@/components/ui/FormLabel.vue'
import FormInput from '@/components/ui/FormInput.vue'
import FormSelect from '@/components/ui/FormSelect.vue'
import FormCheckbox from '@/components/ui/FormCheckbox.vue'

const flowStore = useFlowStore()
const { options: brokerOptions, openNewConfig, openEditConfig } = useConfigSelector('mqtt-broker')

const node = computed(() => flowStore.selectedNode)
const config = computed(() => (node.value?.data?.config ?? {}) as Record<string, unknown>)
const isMqttOut = computed(() => (node.value?.data?.nodeType ?? node.value?.type) === 'mqtt-out')

function update(key: string, value: unknown) {
  if (!node.value) return
  flowStore.updateNodeData(node.value.id, {
    config: { ...config.value, [key]: value },
  })
}

const broker = computed({
  get: () => (config.value.broker as string) ?? '',
  set: (v: string) => update('broker', v),
})

const topic = computed({
  get: () => (config.value.topic as string) ?? '',
  set: (v: string) => update('topic', v),
})

const qos = computed({
  get: () => String((config.value.qos as number) ?? 0),
  set: (v: string) => update('qos', Number(v)),
})

const retain = computed({
  get: () => (config.value.retain as boolean) ?? false,
  set: (v: boolean) => update('retain', v),
})

const qosOptions = [
  { label: '0 - At most once', value: '0' },
  { label: '1 - At least once', value: '1' },
  { label: '2 - Exactly once', value: '2' },
]
</script>

<template>
  <div class="flex flex-col gap-2">
    <!-- Broker selector -->
    <div class="flex flex-col gap-1">
      <FormLabel>Broker</FormLabel>
      <FormSelect
        v-model="broker"
        :options="brokerOptions"
        placeholder="Select broker..."
      />
      <div class="flex items-center gap-1">
        <button
          v-if="broker"
          class="flex-1 h-6 flex items-center justify-center
                 border border-terminal-border text-[9px] uppercase tracking-wider
                 text-terminal-text-dim
                 hover:text-accent hover:border-accent transition-colors"
          title="Edit broker settings"
          @click="openEditConfig(broker)"
        >Edit</button>
        <button
          class="flex-1 h-6 flex items-center justify-center
                 border border-terminal-border text-[9px] uppercase tracking-wider
                 text-terminal-text-dim
                 hover:text-accent hover:border-accent transition-colors"
          title="Add new broker"
          @click="openNewConfig()"
        >+ New</button>
      </div>
    </div>

    <!-- Topic -->
    <div class="flex flex-col gap-1">
      <FormLabel>Topic</FormLabel>
      <FormInput
        v-model="topic"
        :placeholder="isMqttOut ? 'topic (or from msg.topic)' : 'sensor/temperature'"
      />
    </div>

    <!-- QoS -->
    <div class="flex flex-col gap-1">
      <FormLabel>QoS</FormLabel>
      <FormSelect v-model="qos" :options="qosOptions" />
    </div>

    <!-- Retain (mqtt-out only) -->
    <FormCheckbox v-if="isMqttOut" v-model="retain" label="Retain" />
  </div>
</template>
