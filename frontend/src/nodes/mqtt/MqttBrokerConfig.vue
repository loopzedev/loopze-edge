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
import ToggleGroup from '@/components/ui/ToggleGroup.vue'
import CertSelector from '@/components/config/CertSelector.vue'
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

// TLS state — modelled as the structured `tls` block the broker
// expects since 0.0.8. The legacy `useTLS` boolean still works on the
// backend (one release of grace) and is honoured when loading an old
// config so existing flows are not silently downgraded.
type TLSMode = 'disabled' | 'ref' | 'file'
const tlsMode = ref<TLSMode>('disabled')
const tlsServerName = ref('')
const tlsCaBundleRef = ref('')
const tlsClientPairRef = ref('')
const tlsCaBundleFile = ref('')
const tlsClientCertFile = ref('')
const tlsClientKeyFile = ref('')
const tlsInsecureSkipVerify = ref(false)
// Tracks the inferred legacy state so the UI can show a one-shot
// deprecation hint while the user migrates an older config to the new
// TLS section.
const legacyUseTLSDetected = ref(false)

function setTlsMode(next: TLSMode) {
  if (next === tlsMode.value) return
  tlsMode.value = next
  if (next !== 'ref') {
    tlsCaBundleRef.value = ''
    tlsClientPairRef.value = ''
  }
  if (next !== 'file') {
    tlsCaBundleFile.value = ''
    tlsClientCertFile.value = ''
    tlsClientKeyFile.value = ''
  }
  if (next === 'disabled') {
    tlsServerName.value = ''
    tlsInsecureSkipVerify.value = false
  }
  legacyUseTLSDetected.value = false
}

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

      // TLS: the new `tls` block wins over the legacy `useTLS`
      // boolean. When neither is set the section starts disabled.
      const tlsBlock = (cfg.tls as Record<string, unknown> | undefined) ?? {}
      const tlsEnabled = !!tlsBlock.enabled
      const legacy = !!cfg.useTLS
      if (tlsEnabled) {
        const caRef = (tlsBlock.caBundleRef as string) ?? ''
        const cliRef = (tlsBlock.clientPairRef as string) ?? ''
        const caFile = (tlsBlock.caBundleFile as string) ?? ''
        const cliCertFile = (tlsBlock.clientCertFile as string) ?? ''
        const cliKeyFile = (tlsBlock.clientKeyFile as string) ?? ''
        if (caRef || cliRef) {
          tlsMode.value = 'ref'
          tlsCaBundleRef.value = caRef
          tlsClientPairRef.value = cliRef
        } else if (caFile || cliCertFile || cliKeyFile) {
          tlsMode.value = 'file'
          tlsCaBundleFile.value = caFile
          tlsClientCertFile.value = cliCertFile
          tlsClientKeyFile.value = cliKeyFile
        } else {
          // Block enabled with no material — default to ref mode so
          // the cert selectors are visible.
          tlsMode.value = 'ref'
        }
        tlsServerName.value = (tlsBlock.serverName as string) ?? ''
        tlsInsecureSkipVerify.value = !!tlsBlock.insecureSkipVerify
      } else if (legacy) {
        // Legacy path: surface the section in ref mode with a one-shot
        // hint so the operator can migrate.
        tlsMode.value = 'ref'
        legacyUseTLSDetected.value = true
      } else {
        tlsMode.value = 'disabled'
      }

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

function buildTLSBlock(): Record<string, unknown> | undefined {
  if (tlsMode.value === 'disabled') return undefined
  const block: Record<string, unknown> = { enabled: true }
  if (tlsServerName.value) block.serverName = tlsServerName.value
  if (tlsInsecureSkipVerify.value) block.insecureSkipVerify = true
  if (tlsMode.value === 'ref') {
    if (tlsCaBundleRef.value) block.caBundleRef = tlsCaBundleRef.value
    if (tlsClientPairRef.value) block.clientPairRef = tlsClientPairRef.value
  } else {
    if (tlsCaBundleFile.value) block.caBundleFile = tlsCaBundleFile.value
    if (tlsClientCertFile.value) block.clientCertFile = tlsClientCertFile.value
    if (tlsClientKeyFile.value) block.clientKeyFile = tlsClientKeyFile.value
  }
  return block
}

function save() {
  const tls = buildTLSBlock()
  const config: Record<string, unknown> = {
    host: host.value,
    port: port.value,
    clientId: clientId.value,
    protocolVersion: String(protocolVersion.value),
    username: username.value,
    password: password.value,
    keepalive: keepalive.value,
    cleanStart: cleanStart.value,
    sessionExpiry: sessionExpiry.value,
    // The legacy useTLS boolean is dropped on save — the structured
    // block is the new contract. An undefined `tls` means "no TLS".
    useTLS: false,
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
  if (tls) config.tls = tls

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

      <SectionHeader title="TLS">
        <div class="flex flex-col gap-2">
          <FormField label="Mode">
            <ToggleGroup
              :model-value="tlsMode"
              :options="[
                { value: 'disabled', label: 'Disabled' },
                { value: 'ref',      label: 'Stored cert' },
                { value: 'file',     label: 'File path' },
              ]"
              @update:model-value="(v) => setTlsMode(v as TLSMode)"
            />
            <div v-if="legacyUseTLSDetected" class="text-[10px] text-status-warn leading-tight">
              ⚠ Legacy <code>useTLS=true</code> detected. Pick a CA bundle
              below — saving converts this config to the new <code>tls</code>
              block.
            </div>
            <div v-else class="text-[10px] text-terminal-text-dim leading-tight">
              Stored refs resolve against the
              <router-link to="/certs" class="text-accent hover:underline">
                central cert store
              </router-link>
              and rotate independently of this flow. File paths are read fresh
              on every connection init — handy for cert-manager / Let's Encrypt
              setups that swap files on disk.
            </div>
          </FormField>

          <template v-if="tlsMode !== 'disabled'">
            <FormField label="Server name (SNI)">
              <FormInput v-model="tlsServerName" placeholder="broker.example.com" mono />
              <div class="text-[10px] text-terminal-text-dim leading-tight">
                Falls back to the host above when empty.
              </div>
            </FormField>

            <template v-if="tlsMode === 'ref'">
              <FormField label="CA bundle">
                <CertSelector
                  v-model="tlsCaBundleRef"
                  type="ca-bundle"
                  placeholder="— system roots —"
                />
              </FormField>
              <FormField label="Client cert + key (mTLS)">
                <CertSelector
                  v-model="tlsClientPairRef"
                  type="client-pair"
                  placeholder="— no client auth —"
                />
              </FormField>
            </template>

            <template v-else>
              <FormField label="CA bundle file (absolute path, optional)">
                <FormInput v-model="tlsCaBundleFile" placeholder="/etc/loopze/certs/ca.pem" mono />
                <div class="text-[10px] text-terminal-text-dim leading-tight">
                  Empty = system trust roots.
                </div>
              </FormField>
              <FormField label="Client certificate file (optional)">
                <FormInput v-model="tlsClientCertFile" placeholder="/etc/loopze/certs/client.pem" mono />
              </FormField>
              <FormField label="Client key file (optional)">
                <FormInput v-model="tlsClientKeyFile" placeholder="/etc/loopze/certs/client.key" mono />
                <div class="text-[10px] text-terminal-text-dim leading-tight">
                  Set both cert and key for mTLS, or leave both empty.
                </div>
              </FormField>
            </template>

            <FormField>
              <FormCheckbox
                :model-value="tlsInsecureSkipVerify"
                label="Skip TLS verification (insecure)"
                @update:model-value="tlsInsecureSkipVerify = Boolean($event)"
              />
              <div v-if="tlsInsecureSkipVerify" class="text-[10px] text-status-warn leading-tight">
                ⚠ Disables certificate verification — only use in development.
              </div>
            </FormField>
          </template>
        </div>
      </SectionHeader>

      <SectionHeader title="onConnect Message (optional)">
        <div class="flex flex-col gap-2">
          <FormField label="Topic">
            <FormInput v-model="onConnectTopic" placeholder="e.g. status/loopze" mono />
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
            <FormInput v-model="onDisconnectTopic" placeholder="e.g. status/loopze" mono />
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
            <FormInput v-model="lastWillTopic" placeholder="e.g. status/loopze" mono />
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
