<script setup lang="ts">
import { computed, nextTick } from 'vue'
import { useVueFlow } from '@vue-flow/core'
import { useFlowStore } from '@/stores/flowStore'
import { useConfigSelector } from '@/composables/useConfigSelector'
import FormInput from '@/components/ui/FormInput.vue'
import FormSelect from '@/components/ui/FormSelect.vue'
import FormCheckbox from '@/components/ui/FormCheckbox.vue'
import FormField from '@/components/ui/FormField.vue'
import ToggleGroup from '@/components/ui/ToggleGroup.vue'
import { useNodeProperty } from '@/composables/useNodeProperty'
import { QOS_LEVELS } from '@/components/config/enums'

const flowStore = useFlowStore()
const { updateNodeInternals } = useVueFlow('flint-flow-editor')
const { options: brokerOptions, openNewConfig, openEditConfig } = useConfigSelector('mqtt-broker')

const node = computed(() => flowStore.selectedNode)
const isMqttOut = computed(() => (node.value?.data?.nodeType ?? node.value?.type) === 'mqtt-out')

const broker = useNodeProperty<string>('broker', '')
const topic = useNodeProperty<string>('topic', '')
const qos = useNodeProperty<number>('qos', 0)
const retain = useNodeProperty<boolean>('retain', false)

const rawMode = useNodeProperty<string>('mode', 'static')

// Mode is a structural property — switching it changes the input port count.
// Mirror it onto node.inputs and trigger updateNodeInternals so the handles
// re-render and any edges into the (vanishing) input port are pruned by
// flowStore.updateNodeData.
const mode = computed({
  get: () => rawMode.value,
  set: (v: string) => {
    if (v !== 'static' && v !== 'dynamic') return
    rawMode.value = v
    if (isMqttOut.value) return
    const n = flowStore.selectedNode
    if (!n) return
    const desired = v === 'dynamic' ? 1 : 0
    if ((n.data?.inputs ?? 0) === desired) return
    flowStore.updateNodeData(n.id, { inputs: desired })
    nextTick(() => updateNodeInternals([n.id]))
  },
})

const modePresets = [
  { label: 'Static',  value: 'static'  },
  { label: 'Dynamic', value: 'dynamic' },
]

const isDynamic = computed(() => !isMqttOut.value && mode.value === 'dynamic')
const showTopic = computed(() => isMqttOut.value || mode.value === 'static')
const topicError = computed(() =>
  !topic.value && !isMqttOut.value && mode.value === 'static'
    ? 'Topic required for subscription'
    : '',
)
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

    <FormField v-if="!isMqttOut" label="Mode">
      <ToggleGroup v-model="mode" :options="modePresets" />
    </FormField>

    <div
      v-if="isDynamic"
      class="text-[10px] text-terminal-text-dim leading-relaxed border border-terminal-border bg-terminal-bg p-2 rounded space-y-1"
    >
      <p class="m-0">
        Send <span class="font-mono text-accent">msg.action = "subscribe"</span> with
        <span class="font-mono text-accent">msg.payload</span> as a topic string or an array of topics.
        Existing subscriptions are replaced on each call.
      </p>
      <p class="m-0">
        <span class="font-mono text-accent">msg.qos</span> (0/1/2) overrides the QoS below for that
        subscribe call. If missing or out of range, the configured QoS is used.
      </p>
    </div>

    <FormField
      v-if="showTopic"
      label="Topic"
      :error="topicError"
    >
      <FormInput
        v-model="topic"
        :placeholder="isMqttOut ? 'topic (or from msg.topic)' : 'sensor/temperature'"
        mono
        :invalid="!!topicError"
      />
    </FormField>

    <FormField label="QoS">
      <FormSelect v-model="qos" :options="QOS_LEVELS" />
    </FormField>

    <FormCheckbox v-if="isMqttOut" v-model="retain" label="Retain message on broker" />
  </div>
</template>
