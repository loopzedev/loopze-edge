<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useFlowStore } from '@/stores/flowStore'
import { useUiStore } from '@/stores/uiStore'
import { useApi } from '@/composables/useApi'
import FormInput from '@/components/ui/FormInput.vue'
import FormField from '@/components/ui/FormField.vue'
import FormSelect from '@/components/ui/FormSelect.vue'
import NumberInput from '@/components/ui/NumberInput.vue'
import SectionHeader from '@/components/ui/SectionHeader.vue'

const props = defineProps<{
  configId?: string
}>()

const flowStore = useFlowStore()
const ui = useUiStore()
const api = useApi()

const SECURITY_POLICIES = [
  { value: 'None', label: 'None' },
  { value: 'Basic128Rsa15', label: 'Basic128Rsa15' },
  { value: 'Basic256', label: 'Basic256' },
  { value: 'Basic256Sha256', label: 'Basic256Sha256' },
  { value: 'Aes128_Sha256_RsaOaep', label: 'Aes128_Sha256_RsaOaep' },
  { value: 'Aes256_Sha256_RsaPss', label: 'Aes256_Sha256_RsaPss' },
]

const SECURITY_MODES = [
  { value: 'None', label: 'None' },
  { value: 'Sign', label: 'Sign' },
  { value: 'SignAndEncrypt', label: 'Sign & Encrypt' },
]

const AUTH_MODES = [
  { value: 'anonymous', label: 'Anonymous' },
  { value: 'username', label: 'Username / Password' },
  { value: 'certificate', label: 'Certificate' },
]

const name = ref('')
const endpointUrl = ref('opc.tcp://localhost:4840')
const securityPolicy = ref('None')
const securityMode = ref('None')
const authMode = ref('anonymous')
const username = ref('')
const password = ref('')
const clientCertFile = ref('')
const clientKeyFile = ref('')
const applicationUri = ref('urn:loopze:client')
const applicationName = ref('LOOPZE OPC UA Client')
const sessionTimeout = ref(60000)
const requestTimeout = ref(5000)
const keepaliveInterval = ref(10000)

const isEditing = ref(false)

// SecurityPolicy=None forces SecurityMode=None — keep the UI in sync so
// the user is never confused by a "SignAndEncrypt" choice that the backend
// would silently coerce away.
const effectiveSecurityMode = computed(() => (securityPolicy.value === 'None' ? 'None' : securityMode.value))
watch(securityPolicy, (next) => {
  if (next === 'None') {
    securityMode.value = 'None'
  }
})

interface TestResult {
  ok: boolean
  message: string
}
const testResult = ref<TestResult | null>(null)
const testing = ref(false)

onMounted(() => {
  if (props.configId) {
    const existing = flowStore.configs.find((c) => c.id === props.configId)
    if (existing) {
      isEditing.value = true
      name.value = existing.name ?? ''
      const cfg = (existing.config ?? {}) as Record<string, unknown>
      endpointUrl.value = (cfg.endpointUrl as string) ?? endpointUrl.value
      securityPolicy.value = (cfg.securityPolicy as string) ?? 'None'
      securityMode.value = (cfg.securityMode as string) ?? 'None'
      authMode.value = (cfg.authMode as string) ?? 'anonymous'
      username.value = (cfg.username as string) ?? ''
      password.value = (cfg.password as string) ?? ''
      clientCertFile.value = (cfg.clientCertFile as string) ?? ''
      clientKeyFile.value = (cfg.clientKeyFile as string) ?? ''
      applicationUri.value = (cfg.applicationUri as string) ?? applicationUri.value
      applicationName.value = (cfg.applicationName as string) ?? applicationName.value
      sessionTimeout.value = (cfg.sessionTimeout as number) ?? sessionTimeout.value
      requestTimeout.value = (cfg.requestTimeout as number) ?? requestTimeout.value
      keepaliveInterval.value = (cfg.keepaliveInterval as number) ?? keepaliveInterval.value
    }
  }
})

function generateId(): string {
  return crypto.randomUUID()
}

function buildConfig() {
  return {
    endpointUrl: endpointUrl.value,
    securityPolicy: securityPolicy.value,
    securityMode: effectiveSecurityMode.value,
    authMode: authMode.value,
    username: username.value,
    password: password.value,
    clientCertFile: clientCertFile.value,
    clientKeyFile: clientKeyFile.value,
    applicationUri: applicationUri.value,
    applicationName: applicationName.value,
    sessionTimeout: sessionTimeout.value,
    requestTimeout: requestTimeout.value,
    keepaliveInterval: keepaliveInterval.value,
  }
}

async function testConnection() {
  testing.value = true
  testResult.value = null
  try {
    const res = await api.testOpcuaConnection({
      id: props.configId,
      name: name.value,
      config: buildConfig(),
    })
    if (res.ok) {
      const detail = res.serverInfo?.serverTime ? ` (server time ${res.serverInfo.serverTime})` : ''
      testResult.value = { ok: true, message: `Connected${detail}` }
    } else {
      testResult.value = { ok: false, message: res.error ?? 'connection failed' }
    }
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : String(err)
    testResult.value = { ok: false, message: msg }
  } finally {
    testing.value = false
  }
}

function save() {
  const config = buildConfig()

  if (isEditing.value && props.configId) {
    flowStore.updateConfig(props.configId, { name: name.value, config })
  } else {
    flowStore.addConfig({
      id: generateId(),
      type: 'opcua-server',
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
          {{ isEditing ? 'Edit' : 'New' }} OPC UA Server
        </span>
      </div>

      <FormField label="Name" :error="!name ? 'Name required' : ''">
        <FormInput v-model="name" placeholder="e.g. SPS Linie 3" :invalid="!name" />
      </FormField>

      <FormField label="Endpoint URL">
        <FormInput v-model="endpointUrl" placeholder="opc.tcp://host:4840" mono />
      </FormField>

      <SectionHeader title="Security">
        <div class="flex flex-col gap-2">
          <FormField label="Policy">
            <FormSelect v-model="securityPolicy" :options="SECURITY_POLICIES" width="100%" />
          </FormField>
          <FormField label="Mode">
            <FormSelect v-model="securityMode" :options="SECURITY_MODES" width="100%" />
          </FormField>
          <p v-if="securityPolicy === 'None'" class="text-[10px] text-terminal-text-dim">
            Security Mode is forced to None when Policy is None.
          </p>
        </div>
      </SectionHeader>

      <SectionHeader title="Authentication">
        <div class="flex flex-col gap-2">
          <FormField label="Mode">
            <FormSelect v-model="authMode" :options="AUTH_MODES" width="100%" />
          </FormField>

          <template v-if="authMode === 'username'">
            <FormField label="Username">
              <FormInput v-model="username" mono />
            </FormField>
            <FormField label="Password">
              <FormInput v-model="password" type="password" />
            </FormField>
          </template>

          <template v-if="authMode === 'certificate'">
            <FormField label="Client Cert File">
              <FormInput v-model="clientCertFile" placeholder="/path/to/cert.pem" mono />
            </FormField>
            <FormField label="Client Key File">
              <FormInput v-model="clientKeyFile" placeholder="/path/to/key.pem" mono />
            </FormField>
          </template>
        </div>
      </SectionHeader>

      <SectionHeader title="Client Identity (advanced)">
        <div class="flex flex-col gap-2">
          <FormField label="Application URI">
            <FormInput v-model="applicationUri" mono />
          </FormField>
          <FormField label="Application Name">
            <FormInput v-model="applicationName" />
          </FormField>
        </div>
      </SectionHeader>

      <SectionHeader title="Timing (advanced)">
        <div class="flex flex-col gap-2">
          <FormField label="Session Timeout">
            <NumberInput v-model="sessionTimeout" :min="0" unit="ms" />
          </FormField>
          <FormField label="Request Timeout">
            <NumberInput v-model="requestTimeout" :min="0" unit="ms" />
          </FormField>
          <FormField label="Keepalive Interval">
            <NumberInput v-model="keepaliveInterval" :min="0" unit="ms" />
          </FormField>
        </div>
      </SectionHeader>

      <div class="flex flex-col gap-1.5">
        <button
          class="self-start px-3 py-1 text-[10px] uppercase tracking-wider font-bold
                 border border-terminal-border text-terminal-text
                 hover:border-accent hover:text-accent transition-colors disabled:opacity-50"
          :disabled="testing || !endpointUrl"
          @click="testConnection"
        >
          {{ testing ? 'Testing…' : 'Test Connection' }}
        </button>
        <p
          v-if="testResult"
          class="text-[10px]"
          :class="testResult.ok ? 'text-green-400' : 'text-red-400'"
        >
          {{ testResult.message }}
        </p>
      </div>
    </div>

    <div class="shrink-0 flex items-center gap-2 px-4 py-2.5 border-t border-terminal-border bg-terminal-surface">
      <button
        class="px-3 py-1 text-[10px] uppercase tracking-wider font-bold
               bg-accent/10 text-accent border border-accent/30
               hover:bg-accent/20 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
        :disabled="!name || !endpointUrl"
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
