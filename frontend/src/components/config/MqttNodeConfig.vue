<script setup lang="ts">
import { computed } from 'vue'
import { useFlowStore } from '@/stores/flowStore'
import { useConfigSelector } from '@/composables/useConfigSelector'
import FormInput from '@/components/ui/FormInput.vue'
import FormSelect from '@/components/ui/FormSelect.vue'
import FormCheckbox from '@/components/ui/FormCheckbox.vue'
import FormField from '@/components/ui/FormField.vue'
import { useNodeProperty } from '@/composables/useNodeProperty'
import { QOS_LEVELS } from '@/components/config/enums'

const flowStore = useFlowStore()
const { options: brokerOptions, openNewConfig, openEditConfig } = useConfigSelector('mqtt-broker')

const node = computed(() => flowStore.selectedNode)
const isMqttOut = computed(() => (node.value?.data?.nodeType ?? node.value?.type) === 'mqtt-out')

const broker = useNodeProperty<string>('broker', '')
const topic = useNodeProperty<string>('topic', '')
const qos = useNodeProperty<number>('qos', 0)
const retain = useNodeProperty<boolean>('retain', false)
</script>

<template>
  <div class="flex flex-col gap-3">
    <FormField label="Broker">
      <template #action>
        <button
          v-if="broker"
          class="text-[9px] uppercase tracking-wider text-terminal-text-dim hover:text-accent transition-colors"
          @click="openEditConfig(broker)"
        >Edit</button>
        <button
          class="text-[9px] uppercase tracking-wider text-terminal-text-dim hover:text-accent transition-colors"
          @click="openNewConfig()"
        >+ New</button>
      </template>
      <FormSelect
        v-model="broker"
        :options="brokerOptions"
        placeholder="Select broker..."
      />
    </FormField>

    <FormField
      label="Topic"
      :error="!topic && !isMqttOut ? 'Topic required for subscription' : ''"
    >
      <FormInput
        v-model="topic"
        :placeholder="isMqttOut ? 'topic (or from msg.topic)' : 'sensor/temperature'"
        mono
        :invalid="!topic && !isMqttOut"
      />
    </FormField>

    <FormField label="QoS">
      <FormSelect v-model="qos" :options="QOS_LEVELS" />
    </FormField>

    <FormCheckbox v-if="isMqttOut" v-model="retain" label="Retain message on broker" />
  </div>
</template>
