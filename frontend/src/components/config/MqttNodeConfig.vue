<script setup lang="ts">
import { computed } from 'vue'
import { useFlowStore } from '@/stores/flowStore'
import { useConfigSelector } from '@/composables/useConfigSelector'
import FormInput from '@/components/ui/FormInput.vue'
import FormSelect from '@/components/ui/FormSelect.vue'
import FormCheckbox from '@/components/ui/FormCheckbox.vue'
import FormField from '@/components/ui/FormField.vue'
import NumberInput from '@/components/ui/NumberInput.vue'
import SectionHeader from '@/components/ui/SectionHeader.vue'
import ToggleGroup from '@/components/ui/ToggleGroup.vue'
import UserPropertiesEditor from '@/components/ui/UserPropertiesEditor.vue'
import { useNodeProperty } from '@/composables/useNodeProperty'
import { useStructuralProperty } from '@/composables/useStructuralProperty'
import { QOS_LEVELS, RETAIN_HANDLING_OPTIONS, PAYLOAD_FORMAT_OPTIONS, MQTT_IN_OUTPUT_FORMATS, MQTT_OUT_TARGETS } from '@/components/config/enums'

const flowStore = useFlowStore()
const { options: brokerOptions, openNewConfig, openEditConfig } = useConfigSelector('mqtt-broker')

const node = computed(() => flowStore.selectedNode)
const isMqttOut = computed(() => (node.value?.data?.nodeType ?? node.value?.type) === 'mqtt-out')

const broker = useNodeProperty<string>('broker', '')
const target = useNodeProperty<string>('target', 'topic')
const topic = useNodeProperty<string>('topic', '')
const qos = useNodeProperty<number>('qos', 0)
const retain = useNodeProperty<boolean>('retain', false)

// mqtt-out has no input port that the mode switch should grow; mqtt-in does.
const mode = useStructuralProperty<string>('mode', 'static', {
  port: 'inputs',
  derive: (v) => (v === 'dynamic' ? 1 : 0),
  enabled: () => !isMqttOut.value,
})
const outputFormat = useNodeProperty<string>('outputFormat', 'string')

// mqtt-in v5 subscription options
const noLocal = useNodeProperty<boolean>('noLocal', false)
const retainAsPublished = useNodeProperty<boolean>('retainAsPublished', false)
const retainHandling = useNodeProperty<number>('retainHandling', 0)
const subscriptionIdentifier = useNodeProperty<number>('subscriptionIdentifier', 0)
const subscribeUserProperties = useNodeProperty<Record<string, string>>('subscribeUserProperties', {})

// mqtt-out v5 default publish properties
const defaultUserProperties = useNodeProperty<Record<string, string>>('defaultUserProperties', {})
const defaultContentType = useNodeProperty<string>('defaultContentType', '')
const defaultResponseTopic = useNodeProperty<string>('defaultResponseTopic', '')
const defaultMessageExpiry = useNodeProperty<number>('defaultMessageExpiry', 0)
const defaultPayloadFormat = useNodeProperty<number>('defaultPayloadFormat', 0)

const modePresets = [
  { label: 'Static',  value: 'static'  },
  { label: 'Dynamic', value: 'dynamic' },
]

const isDynamic = computed(() => !isMqttOut.value && mode.value === 'dynamic')
const isResponseTopicTarget = computed(() => isMqttOut.value && target.value === 'responseTopic')
const showTopic = computed(() => {
  if (isMqttOut.value) return !isResponseTopicTarget.value
  return mode.value === 'static'
})
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

    <FormField v-if="isMqttOut" label="Publish target">
      <ToggleGroup v-model="target" :options="MQTT_OUT_TARGETS" />
    </FormField>

    <div
      v-if="isResponseTopicTarget"
      class="text-[10px] text-terminal-text-dim leading-relaxed border border-terminal-border bg-terminal-bg p-2 rounded space-y-1"
    >
      <p class="m-0">
        Publishes to <span class="font-mono text-accent">msg.responseTopic</span>; the configured topic
        and <span class="font-mono text-accent">msg.topic</span> are ignored.
      </p>
      <p class="m-0">
        <span class="font-mono text-accent">msg.correlationData</span> is forwarded as the v5
        Correlation Data property — pair with an inbound <span class="font-mono">mqtt-in</span>
        carrying both fields from a remote <span class="font-mono">mqtt-request</span>.
      </p>
    </div>

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

    <FormField
      v-if="!isMqttOut"
      label="Output Format"
      hint="JSON falls back to String on parse failure (sets msg.parseError)"
    >
      <FormSelect v-model="outputFormat" :options="MQTT_IN_OUTPUT_FORMATS" />
    </FormField>

    <FormCheckbox v-if="isMqttOut" v-model="retain" label="Retain message on broker" />

    <!-- mqtt-in: MQTT v5 subscription options -->
    <SectionHeader v-if="!isMqttOut" title="MQTT v5 Subscription Options">
      <div class="flex flex-col gap-2">
        <FormCheckbox v-model="noLocal" label="No Local (don't echo own publishes)" />
        <FormCheckbox v-model="retainAsPublished" label="Retain As Published (preserve retain flag)" />
        <FormField label="Retain Handling">
          <FormSelect v-model="retainHandling" :options="RETAIN_HANDLING_OPTIONS" />
        </FormField>
        <FormField
          label="Subscription Identifier"
          hint="0 = not set; broker echoes the ID on every matching publish"
        >
          <NumberInput v-model="subscriptionIdentifier" :min="0" />
        </FormField>
        <p
          v-if="isDynamic"
          class="text-[9px] text-terminal-text-dim leading-relaxed mt-1"
        >
          In Dynamic mode, every option above can be overridden per call via
          <span class="font-mono text-accent">msg.noLocal</span>,
          <span class="font-mono text-accent">msg.retainAsPublished</span>,
          <span class="font-mono text-accent">msg.retainHandling</span>,
          <span class="font-mono text-accent">msg.subscriptionIdentifier</span>.
        </p>
      </div>
    </SectionHeader>

    <SectionHeader v-if="!isMqttOut" title="SUBSCRIBE User Properties (rare)">
      <UserPropertiesEditor v-model="subscribeUserProperties" />
    </SectionHeader>

    <!-- mqtt-out: MQTT v5 default publish properties -->
    <SectionHeader v-if="isMqttOut" title="MQTT v5 Default Properties">
      <div class="flex flex-col gap-2">
        <FormField label="Content Type">
          <FormInput v-model="defaultContentType" placeholder="e.g. application/json" mono />
        </FormField>
        <FormField label="Response Topic">
          <FormInput v-model="defaultResponseTopic" placeholder="e.g. reply/topic" mono />
        </FormField>
        <FormField label="Message Expiry">
          <NumberInput v-model="defaultMessageExpiry" :min="0" unit="sec" />
        </FormField>
        <FormField label="Payload Format">
          <FormSelect v-model="defaultPayloadFormat" :options="PAYLOAD_FORMAT_OPTIONS" />
        </FormField>
        <p class="text-[9px] text-terminal-text-dim leading-relaxed mt-1">
          msg fields override these defaults per message:
          <span class="font-mono text-accent">msg.contentType</span>,
          <span class="font-mono text-accent">msg.responseTopic</span>,
          <span class="font-mono text-accent">msg.messageExpiry</span>,
          <span class="font-mono text-accent">msg.payloadFormat</span>.
          <span class="font-mono text-accent">msg.correlationData</span> is per-message only (no default).
        </p>
      </div>
    </SectionHeader>

    <SectionHeader v-if="isMqttOut" title="Default User Properties">
      <div class="flex flex-col gap-1">
        <UserPropertiesEditor v-model="defaultUserProperties" />
        <p class="text-[9px] text-terminal-text-dim leading-relaxed mt-1">
          <span class="font-mono text-accent">msg.userProperties</span> is merged with these defaults per
          message — msg keys override config keys at the same name; non-overlapping keys from both sides survive.
        </p>
      </div>
    </SectionHeader>
  </div>
</template>
