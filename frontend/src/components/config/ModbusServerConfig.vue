<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useFlowStore } from '@/stores/flowStore'
import { useUiStore } from '@/stores/uiStore'
import FormInput from '@/components/ui/FormInput.vue'
import FormField from '@/components/ui/FormField.vue'
import FormSelect from '@/components/ui/FormSelect.vue'
import NumberInput from '@/components/ui/NumberInput.vue'
import SectionHeader from '@/components/ui/SectionHeader.vue'
import ToggleGroup from '@/components/ui/ToggleGroup.vue'

const props = defineProps<{
  configId?: string
}>()

const flowStore = useFlowStore()
const ui = useUiStore()

const TRANSPORTS = [
  { value: 'tcp', label: 'TCP' },
  { value: 'rtu', label: 'RTU (Serial)' },
]

const BAUD_RATES = [
  { value: 1200, label: '1200' },
  { value: 2400, label: '2400' },
  { value: 4800, label: '4800' },
  { value: 9600, label: '9600' },
  { value: 19200, label: '19200' },
  { value: 38400, label: '38400' },
  { value: 57600, label: '57600' },
  { value: 115200, label: '115200' },
]

const DATA_BITS = [
  { value: 7, label: '7' },
  { value: 8, label: '8' },
]

const STOP_BITS = [
  { value: 1, label: '1' },
  { value: 2, label: '2' },
]

const PARITY_OPTIONS = [
  { value: 'none', label: 'None' },
  { value: 'even', label: 'Even' },
  { value: 'odd', label: 'Odd' },
]

const name = ref('')
const transport = ref<string>('tcp')

const host = ref('localhost')
const port = ref(502)

const serialPort = ref('')
const baudRate = ref<number>(9600)
const dataBits = ref<number>(8)
const stopBits = ref<number>(1)
const parity = ref<string>('none')

const timeout = ref(1000)
const idleTimeout = ref(60)
const defaultUnitId = ref(1)
const reconnectBackoff = ref(5)

const isEditing = ref(false)

const isTcp = computed(() => transport.value === 'tcp')

onMounted(() => {
  if (props.configId) {
    const existing = flowStore.configs.find((c) => c.id === props.configId)
    if (existing) {
      isEditing.value = true
      name.value = existing.name ?? ''
      const cfg = (existing.config ?? {}) as Record<string, unknown>
      transport.value = (cfg.transport as string) ?? 'tcp'
      host.value = (cfg.host as string) ?? 'localhost'
      port.value = (cfg.port as number) ?? 502
      serialPort.value = (cfg.serialPort as string) ?? ''
      baudRate.value = (cfg.baudRate as number) ?? 9600
      dataBits.value = (cfg.dataBits as number) ?? 8
      stopBits.value = (cfg.stopBits as number) ?? 1
      parity.value = (cfg.parity as string) ?? 'none'
      timeout.value = (cfg.timeout as number) ?? 1000
      idleTimeout.value = (cfg.idleTimeout as number) ?? 60
      defaultUnitId.value = (cfg.defaultUnitId as number) ?? 1
      reconnectBackoff.value = (cfg.reconnectBackoff as number) ?? 5
    }
  }
})

function generateId(): string {
  return crypto.randomUUID()
}

function save() {
  const config = {
    transport: transport.value,
    host: host.value,
    port: port.value,
    serialPort: serialPort.value,
    baudRate: baudRate.value,
    dataBits: dataBits.value,
    stopBits: stopBits.value,
    parity: parity.value,
    timeout: timeout.value,
    idleTimeout: idleTimeout.value,
    defaultUnitId: defaultUnitId.value,
    reconnectBackoff: reconnectBackoff.value,
  }

  if (isEditing.value && props.configId) {
    flowStore.updateConfig(props.configId, { name: name.value, config })
  } else {
    flowStore.addConfig({
      id: generateId(),
      type: 'modbus-server',
      name: name.value,
      config,
    })
  }

  cancel()
}

function cancel() {
  ui.clearConfigEditor()
}

const canSave = computed(() => {
  if (!name.value) return false
  if (isTcp.value && !host.value) return false
  if (!isTcp.value && !serialPort.value) return false
  return true
})
</script>

<template>
  <div class="flex flex-col h-full">
    <div class="flex-1 overflow-y-auto px-4 py-3 flex flex-col gap-3">
      <div class="flex items-center gap-2 mb-1">
        <span class="text-accent text-sm font-bold uppercase tracking-wider">
          {{ isEditing ? 'Edit' : 'New' }} Modbus Server
        </span>
      </div>

      <FormField label="Name" :error="!name ? 'Name required' : ''">
        <FormInput v-model="name" placeholder="e.g. SPS Halle 1" :invalid="!name" />
      </FormField>

      <FormField label="Transport">
        <ToggleGroup v-model="transport" :options="TRANSPORTS" />
      </FormField>

      <SectionHeader v-if="isTcp" title="TCP">
        <div class="flex flex-col gap-2">
          <FormField label="Server" :error="!host ? 'Host required' : ''">
            <div class="flex items-stretch gap-1">
              <div class="flex-1 min-w-0">
                <FormInput v-model="host" placeholder="192.168.1.50" mono :invalid="!host" />
              </div>
              <div class="w-24 shrink-0">
                <NumberInput v-model="port" :min="1" :max="65535" />
              </div>
            </div>
          </FormField>
        </div>
      </SectionHeader>

      <SectionHeader v-else title="RTU (Serial)">
        <div class="flex flex-col gap-2">
          <FormField label="Serial Port" :error="!serialPort ? 'Serial port required' : ''">
            <FormInput v-model="serialPort" placeholder="/dev/ttyUSB0 or COM3" mono :invalid="!serialPort" />
          </FormField>
          <div class="flex items-stretch gap-2">
            <div class="flex-1">
              <FormField label="Baud">
                <FormSelect v-model="baudRate" :options="BAUD_RATES" width="100%" />
              </FormField>
            </div>
            <div class="flex-1">
              <FormField label="Data Bits">
                <FormSelect v-model="dataBits" :options="DATA_BITS" width="100%" />
              </FormField>
            </div>
          </div>
          <div class="flex items-stretch gap-2">
            <div class="flex-1">
              <FormField label="Parity">
                <FormSelect v-model="parity" :options="PARITY_OPTIONS" width="100%" />
              </FormField>
            </div>
            <div class="flex-1">
              <FormField label="Stop Bits">
                <FormSelect v-model="stopBits" :options="STOP_BITS" width="100%" />
              </FormField>
            </div>
          </div>
        </div>
      </SectionHeader>

      <SectionHeader title="Common">
        <div class="flex flex-col gap-2">
          <FormField label="Request Timeout">
            <NumberInput v-model="timeout" :min="100" unit="ms" />
          </FormField>
          <FormField
            v-if="isTcp"
            label="Idle Timeout"
            hint="Close TCP connection after inactivity (0 = never close)"
          >
            <NumberInput v-model="idleTimeout" :min="0" unit="sec" />
          </FormField>
          <FormField label="Default Unit ID" hint="Slave ID used when a node leaves it empty">
            <NumberInput v-model="defaultUnitId" :min="0" :max="255" />
          </FormField>
          <FormField label="Reconnect Backoff">
            <NumberInput v-model="reconnectBackoff" :min="0" unit="sec" />
          </FormField>
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
