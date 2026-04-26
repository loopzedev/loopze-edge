<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useFlowStore } from '@/stores/flowStore'
import { useUiStore } from '@/stores/uiStore'
import FormInput from '@/components/ui/FormInput.vue'
import FormCheckbox from '@/components/ui/FormCheckbox.vue'
import FormField from '@/components/ui/FormField.vue'
import FormSelect from '@/components/ui/FormSelect.vue'
import NumberInput from '@/components/ui/NumberInput.vue'
import SectionHeader from '@/components/ui/SectionHeader.vue'
import { QOS_LEVELS } from './enums'

const props = defineProps<{
  configId?: string
}>()

const flowStore = useFlowStore()
const ui = useUiStore()

const PROTOCOL_VERSIONS = [
  { value: '5', label: 'MQTT v5 (default)' },
  { value: '3.1.1', label: 'MQTT v3.1.1' },
]

const name = ref('')
const host = ref('localhost')
const port = ref(1883)
const clientId = ref('')
const protocolVersion = ref<string | number>('5')
const username = ref('')
const password = ref('')
const keepalive = ref(60)
const cleanStart = ref(true)
const sessionExpiry = ref(0)
const useTLS = ref(false)

const onConnectTopic = ref('')
const onConnectPayload = ref('')
const onConnectQoS = ref<string | number>(0)
const onConnectRetain = ref(false)

const onDisconnectTopic = ref('')
const onDisconnectPayload = ref('')
const onDisconnectQoS = ref<string | number>(0)
const onDisconnectRetain = ref(false)

const lastWillTopic = ref('')
const lastWillPayload = ref('')
const lastWillQoS = ref<string | number>(0)
const lastWillRetain = ref(false)
const lastWillDelayInterval = ref(0)

const isEditing = ref(false)

onMounted(() => {
  if (props.configId) {
    const existing = flowStore.configs.find((c) => c.id === props.configId)
    if (existing) {
      isEditing.value = true
      name.value = existing.name ?? ''
      const cfg = (existing.config ?? {}) as Record<string, unknown>
      host.value = (cfg.host as string) ?? 'localhost'
      port.value = (cfg.port as number) ?? 1883
      clientId.value = (cfg.clientId as string) ?? ''
      protocolVersion.value = (cfg.protocolVersion as string) ?? '5'
      username.value = (cfg.username as string) ?? ''
      password.value = (cfg.password as string) ?? ''
      keepalive.value = (cfg.keepalive as number) ?? 60
      // Migrate legacy `cleanSession` field name if present.
      cleanStart.value =
        (cfg.cleanStart as boolean | undefined) ??
        (cfg.cleanSession as boolean | undefined) ??
        true
      sessionExpiry.value = (cfg.sessionExpiry as number) ?? 0
      useTLS.value = (cfg.useTLS as boolean) ?? false

      onConnectTopic.value = (cfg.onConnectTopic as string) ?? ''
      onConnectPayload.value = (cfg.onConnectPayload as string) ?? ''
      onConnectQoS.value = (cfg.onConnectQoS as number) ?? 0
      onConnectRetain.value = (cfg.onConnectRetain as boolean) ?? false

      onDisconnectTopic.value = (cfg.onDisconnectTopic as string) ?? ''
      onDisconnectPayload.value = (cfg.onDisconnectPayload as string) ?? ''
      onDisconnectQoS.value = (cfg.onDisconnectQoS as number) ?? 0
      onDisconnectRetain.value = (cfg.onDisconnectRetain as boolean) ?? false

      lastWillTopic.value = (cfg.lastWillTopic as string) ?? ''
      lastWillPayload.value = (cfg.lastWillPayload as string) ?? ''
      lastWillQoS.value = (cfg.lastWillQoS as number) ?? 0
      lastWillRetain.value = (cfg.lastWillRetain as boolean) ?? false
      lastWillDelayInterval.value = (cfg.lastWillDelayInterval as number) ?? 0
    }
  }
})

function generateId(): string {
  return crypto.randomUUID()
}

function save() {
  const config = {
    host: host.value,
    port: port.value,
    clientId: clientId.value,
    protocolVersion: String(protocolVersion.value),
    username: username.value,
    password: password.value,
    keepalive: keepalive.value,
    cleanStart: cleanStart.value,
    sessionExpiry: sessionExpiry.value,
    useTLS: useTLS.value,
    onConnectTopic: onConnectTopic.value,
    onConnectPayload: onConnectPayload.value,
    onConnectQoS: Number(onConnectQoS.value),
    onConnectRetain: onConnectRetain.value,
    onDisconnectTopic: onDisconnectTopic.value,
    onDisconnectPayload: onDisconnectPayload.value,
    onDisconnectQoS: Number(onDisconnectQoS.value),
    onDisconnectRetain: onDisconnectRetain.value,
    lastWillTopic: lastWillTopic.value,
    lastWillPayload: lastWillPayload.value,
    lastWillQoS: Number(lastWillQoS.value),
    lastWillRetain: lastWillRetain.value,
    lastWillDelayInterval: lastWillDelayInterval.value,
  }

  if (isEditing.value && props.configId) {
    flowStore.updateConfig(props.configId, { name: name.value, config })
  } else {
    flowStore.addConfig({
      id: generateId(),
      type: 'mqtt-broker',
      name: name.value,
      config,
    })
  }

  cancel()
}

function cancel() {
  ui.clearConfigEditor()
}
</script>

<template>
  <div class="flex flex-col h-full">
    <div class="flex-1 overflow-y-auto px-4 py-3 flex flex-col gap-3">
      <div class="flex items-center gap-2 mb-1">
        <span class="text-accent text-sm font-bold uppercase tracking-wider">
          {{ isEditing ? 'Edit' : 'New' }} MQTT Broker
        </span>
      </div>

      <FormField label="Name" :error="!name ? 'Name required' : ''">
        <FormInput v-model="name" placeholder="e.g. Production Broker" :invalid="!name" />
      </FormField>

      <FormField label="Server">
        <div class="flex items-stretch gap-1">
          <div class="flex-1 min-w-0">
            <FormInput v-model="host" placeholder="mqtt.example.com" mono />
          </div>
          <div class="w-24 shrink-0">
            <NumberInput v-model="port" :min="1" :max="65535" />
          </div>
        </div>
      </FormField>

      <FormField label="Client ID">
        <FormInput v-model="clientId" placeholder="auto-generated if empty" mono />
      </FormField>

      <FormField label="Protocol Version">
        <FormSelect v-model="protocolVersion" :options="PROTOCOL_VERSIONS" width="100%" />
      </FormField>

      <FormField label="Username">
        <FormInput v-model="username" placeholder="optional" mono />
      </FormField>

      <FormField label="Password">
        <FormInput v-model="password" type="password" placeholder="optional" />
      </FormField>

      <FormField label="Keep-Alive">
        <NumberInput v-model="keepalive" :min="0" unit="sec" />
      </FormField>

      <FormField label="Session Expiry">
        <NumberInput v-model="sessionExpiry" :min="0" unit="sec" />
      </FormField>

      <FormCheckbox v-model="cleanStart" label="Clean Start (Clean Session in v3.1.1)" />
      <FormCheckbox v-model="useTLS" label="Use TLS" />

      <SectionHeader title="onConnect Message (optional)">
        <div class="flex flex-col gap-2">
          <FormField label="Topic">
            <FormInput v-model="onConnectTopic" placeholder="e.g. status/flint" mono />
          </FormField>
          <FormField label="Payload">
            <FormInput v-model="onConnectPayload" placeholder="e.g. online" />
          </FormField>
          <div class="flex items-stretch gap-2">
            <div class="flex-1">
              <FormField label="QoS">
                <FormSelect v-model="onConnectQoS" :options="QOS_LEVELS" width="100%" />
              </FormField>
            </div>
            <div class="self-end pb-1">
              <FormCheckbox v-model="onConnectRetain" label="Retain" />
            </div>
          </div>
        </div>
      </SectionHeader>

      <SectionHeader title="onDisconnect Message (optional)">
        <div class="flex flex-col gap-2">
          <FormField label="Topic">
            <FormInput v-model="onDisconnectTopic" placeholder="e.g. status/flint" mono />
          </FormField>
          <FormField label="Payload">
            <FormInput v-model="onDisconnectPayload" placeholder="e.g. offline" />
          </FormField>
          <div class="flex items-stretch gap-2">
            <div class="flex-1">
              <FormField label="QoS">
                <FormSelect v-model="onDisconnectQoS" :options="QOS_LEVELS" width="100%" />
              </FormField>
            </div>
            <div class="self-end pb-1">
              <FormCheckbox v-model="onDisconnectRetain" label="Retain" />
            </div>
          </div>
        </div>
      </SectionHeader>

      <SectionHeader title="LastWill (unclean disconnect, MQTT-spec)">
        <div class="flex flex-col gap-2">
          <FormField label="Topic">
            <FormInput v-model="lastWillTopic" placeholder="e.g. status/flint" mono />
          </FormField>
          <FormField label="Payload">
            <FormInput v-model="lastWillPayload" placeholder="e.g. offline" />
          </FormField>
          <div class="flex items-stretch gap-2">
            <div class="flex-1">
              <FormField label="QoS">
                <FormSelect v-model="lastWillQoS" :options="QOS_LEVELS" width="100%" />
              </FormField>
            </div>
            <div class="self-end pb-1">
              <FormCheckbox v-model="lastWillRetain" label="Retain" />
            </div>
          </div>
          <FormField label="Delay Interval (v5)">
            <NumberInput v-model="lastWillDelayInterval" :min="0" unit="sec" />
          </FormField>
        </div>
      </SectionHeader>
    </div>

    <!-- Sticky action footer -->
    <div class="shrink-0 flex items-center gap-2 px-4 py-2.5 border-t border-terminal-border bg-terminal-surface">
      <button
        class="px-3 py-1 text-[10px] uppercase tracking-wider font-bold
               bg-accent/10 text-accent border border-accent/30
               hover:bg-accent/20 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
        :disabled="!name"
        @click="save"
      >
        {{ isEditing ? 'Update' : 'Create' }}
      </button>
      <button
        class="px-3 py-1 text-[10px] uppercase tracking-wider
               text-terminal-text-dim border border-terminal-border
               hover:text-terminal-text hover:border-terminal-text-dim transition-colors"
        @click="cancel"
      >
        Cancel
      </button>
    </div>
  </div>
</template>
