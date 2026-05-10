<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useFlowStore } from '@/stores/flowStore'
import { useUiStore } from '@/stores/uiStore'
import { useApi } from '@/composables/useApi'
import FormInput from '@/components/ui/FormInput.vue'
import FormField from '@/components/ui/FormField.vue'
import FormSelect from '@/components/ui/FormSelect.vue'
import NumberInput from '@/components/ui/NumberInput.vue'
import SectionHeader from '@/components/ui/SectionHeader.vue'
import { S7_CONNECTION_TYPES } from './enums'

const props = defineProps<{
  configId?: string
}>()

const flowStore = useFlowStore()
const ui = useUiStore()
const api = useApi()

// Connection type controls whether rack/slot are auto-derived or
// user-supplied. The backend's s7ConnectionDefaults() in s7_plc.go has
// the canonical mapping; the dropdown labels mention the auto-defaults
// so the user knows what they're getting.
const CONNECT_TYPES = [
  { value: 1, label: 'PG (default)' },
  { value: 2, label: 'OP (operator panel)' },
  { value: 3, label: 'S7Basic (LOGO! / S7-200 Smart)' },
]

const name = ref('')
const host = ref('localhost')
const port = ref(102)
const connection = ref<string>('s7-1200-1500')
const rack = ref(0)
const slot = ref(1)
const connectType = ref<number>(1)
const pduSize = ref(480)
const timeout = ref(2000)
const idleTimeout = ref(60)
const reconnectBackoff = ref(5)

const isEditing = ref(false)
const isCustom = computed(() => connection.value === 'custom')
const isLogo = computed(() => connection.value === 'logo')

const testing = ref(false)
const testResult = ref<{ ok: boolean; message: string; details?: Record<string, string> } | null>(null)

onMounted(() => {
  if (props.configId) {
    const existing = flowStore.configs.find((c) => c.id === props.configId)
    if (existing) {
      isEditing.value = true
      name.value = existing.name ?? ''
      const cfg = (existing.config ?? {}) as Record<string, unknown>
      host.value = (cfg.host as string) ?? 'localhost'
      port.value = (cfg.port as number) ?? 102
      connection.value = (cfg.connection as string) ?? 's7-1200-1500'
      rack.value = (cfg.rack as number) ?? 0
      slot.value = (cfg.slot as number) ?? 1
      connectType.value = (cfg.connectType as number) ?? 1
      pduSize.value = (cfg.pduSize as number) ?? 480
      timeout.value = (cfg.timeout as number) ?? 2000
      idleTimeout.value = (cfg.idleTimeout as number) ?? 60
      reconnectBackoff.value = (cfg.reconnectBackoff as number) ?? 5
    }
  }
})

function generateId(): string {
  return crypto.randomUUID()
}

function buildConfig(): Record<string, unknown> {
  const cfg: Record<string, unknown> = {
    host: host.value,
    port: port.value,
    connection: connection.value,
    pduSize: pduSize.value,
    timeout: timeout.value,
    idleTimeout: idleTimeout.value,
    reconnectBackoff: reconnectBackoff.value,
  }
  if (isCustom.value) {
    cfg.rack = rack.value
    cfg.slot = slot.value
    cfg.connectType = connectType.value
  }
  return cfg
}

async function testConnection() {
  testing.value = true
  testResult.value = null
  try {
    const res = await api.testS7Connection({
      id: props.configId,
      name: name.value,
      config: buildConfig(),
    })
    if (res.ok && res.info) {
      testResult.value = {
        ok: true,
        message: `Connected (PDU ${res.info.negotiatedPduSize} bytes)`,
        details: {
          'CPU': res.info.cpuType ?? '—',
          'Order': res.info.orderCode ?? '—',
          'Module': res.info.moduleName ?? '—',
          'Serial': res.info.serialNumber ?? '—',
        },
      }
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
      type: 's7-plc',
      name: name.value,
      config,
    })
  }
  cancel()
}

function cancel() {
  ui.clearConfigEditor()
}

const canSave = computed(() => Boolean(name.value && host.value))
</script>

<template>
  <div class="flex flex-col h-full">
    <div class="flex-1 overflow-y-auto px-4 py-3 flex flex-col gap-3">
      <div class="flex items-center gap-2 mb-1">
        <span class="text-accent text-sm font-bold uppercase tracking-wider">
          {{ isEditing ? 'Edit' : 'New' }} S7 PLC
        </span>
      </div>

      <FormField label="Name" :error="!name ? 'Name required' : ''">
        <FormInput v-model="name" placeholder="e.g. Press Line 2" :invalid="!name" />
      </FormField>

      <FormField label="Server" :error="!host ? 'Host required' : ''">
        <div class="flex items-stretch gap-1">
          <div class="flex-1 min-w-0">
            <FormInput v-model="host" placeholder="192.168.1.20" mono :invalid="!host" />
          </div>
          <div class="w-24 shrink-0">
            <NumberInput v-model="port" :min="1" :max="65535" />
          </div>
        </div>
      </FormField>

      <FormField label="Connection" hint="Auto-derives rack / slot. Pick `Custom` to override.">
        <FormSelect v-model="connection" :options="S7_CONNECTION_TYPES" />
      </FormField>

      <SectionHeader v-if="isCustom" title="Custom Connection">
        <div class="flex flex-col gap-2">
          <div class="flex items-stretch gap-2">
            <div class="flex-1">
              <FormField label="Rack">
                <NumberInput v-model="rack" :min="0" :max="7" />
              </FormField>
            </div>
            <div class="flex-1">
              <FormField label="Slot">
                <NumberInput v-model="slot" :min="0" :max="31" />
              </FormField>
            </div>
          </div>
          <FormField label="Connect Type">
            <FormSelect v-model="connectType" :options="CONNECT_TYPES" />
          </FormField>
        </div>
      </SectionHeader>

      <div
        v-if="isLogo"
        class="text-[10px] text-terminal-text-dim leading-relaxed border border-terminal-border bg-terminal-bg p-2 rounded"
      >
        LOGO! / S7-200 Smart use the Basic connection type. Free-form local /
        remote TSAPs are not exposed in v1 — pick <span class="font-mono text-accent">Custom</span> if
        you need to override rack / slot.
      </div>

      <SectionHeader title="Advanced">
        <div class="flex flex-col gap-2">
          <FormField label="PDU Size" hint="Requested; PLC may negotiate down (typical: 240 / 480)">
            <NumberInput v-model="pduSize" :min="64" :max="960" unit="bytes" />
          </FormField>
          <FormField label="Request Timeout">
            <NumberInput v-model="timeout" :min="100" unit="ms" />
          </FormField>
          <FormField label="Idle Timeout" hint="Close TCP after inactivity (0 = never close)">
            <NumberInput v-model="idleTimeout" :min="0" unit="sec" />
          </FormField>
          <FormField label="Reconnect Backoff">
            <NumberInput v-model="reconnectBackoff" :min="0" unit="sec" />
          </FormField>
        </div>
      </SectionHeader>

      <SectionHeader title="Test Connection">
        <div class="flex flex-col gap-2">
          <button
            class="px-3 py-1 text-[10px] uppercase tracking-wider font-bold
                   bg-terminal-surface text-terminal-text border border-terminal-border
                   hover:border-accent hover:text-accent transition-colors
                   disabled:opacity-50 disabled:cursor-not-allowed self-start"
            :disabled="testing || !host"
            @click="testConnection"
          >
            {{ testing ? 'Testing…' : 'Test Connection' }}
          </button>
          <div
            v-if="testResult"
            class="text-[10px] leading-relaxed p-2 border rounded"
            :class="testResult.ok
              ? 'border-green-700/40 text-green-400 bg-green-900/10'
              : 'border-red-700/40 text-red-400 bg-red-900/10'"
          >
            <div class="font-bold uppercase tracking-wider mb-1">
              {{ testResult.ok ? 'OK' : 'Failed' }}
            </div>
            <div>{{ testResult.message }}</div>
            <div v-if="testResult.details" class="mt-1 grid grid-cols-[auto_1fr] gap-x-2 gap-y-0.5 font-mono">
              <template v-for="(v, k) in testResult.details" :key="k">
                <span class="text-terminal-text-dim">{{ k }}:</span>
                <span>{{ v }}</span>
              </template>
            </div>
          </div>
        </div>
      </SectionHeader>
    </div>

    <div class="shrink-0 flex items-center gap-2 px-4 py-2.5 border-t border-terminal-border bg-terminal-surface">
      <button
        class="px-3 py-1 text-[10px] uppercase tracking-wider font-bold
               bg-accent/10 text-accent border border-accent/30
               hover:bg-accent/20 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
        :disabled="!canSave"
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
