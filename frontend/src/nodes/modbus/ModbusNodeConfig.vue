<script setup lang="ts">
import { computed } from 'vue'
import { useFlowStore } from '@/stores/flowStore'
import { useConfigSelector } from '@/composables/useConfigSelector'
import FormSelect from '@/components/ui/FormSelect.vue'
import FormCheckbox from '@/components/ui/FormCheckbox.vue'
import FormField from '@/components/ui/FormField.vue'
import NumberInput from '@/components/ui/NumberInput.vue'
import SectionHeader from '@/components/ui/SectionHeader.vue'
import ToggleGroup from '@/components/ui/ToggleGroup.vue'
import { useNodeProperty } from '@/composables/useNodeProperty'
import { useStructuralProperty } from '@/composables/useStructuralProperty'
import {
  MODBUS_READ_FCS,
  MODBUS_WRITE_FCS,
  MODBUS_COIL_FCS,
  MODBUS_DATA_TYPES,
  MODBUS_BYTE_ORDERS,
  MODBUS_WORD_ORDERS,
} from './enums'

const flowStore = useFlowStore()
const { options: serverOptions, openNewConfig, openEditConfig } = useConfigSelector('modbus-server')

const node = computed(() => flowStore.selectedNode)
const isWrite = computed(() => (node.value?.data?.nodeType ?? node.value?.type) === 'modbus-write')

const server = useNodeProperty<string>('server', '')
const unitId = useNodeProperty<number>('unitId', 0)
const fc = useNodeProperty<number>('fc', isWrite.value ? 16 : 3)
const address = useNodeProperty<number>('address', 0)
const quantity = useNodeProperty<number>('quantity', 1)
const dataType = useNodeProperty<string>('dataType', 'raw')
const byteOrder = useNodeProperty<string>('byteOrder', 'bigEndian')
const wordOrder = useNodeProperty<string>('wordOrder', 'bigEndian')
const scale = useNodeProperty<number>('scale', 1)
const offset = useNodeProperty<number>('offset', 0)

// Read-specific: mode toggle drives input port count (static=0, dynamic=1),
// but only on read nodes — modbus-write has no such mode dimension.
const mode = useStructuralProperty<string>('mode', 'static', {
  port: 'inputs',
  derive: (v) => (v === 'dynamic' ? 1 : 0),
  enabled: () => !isWrite.value,
})
const pollInterval = useNodeProperty<number>('pollInterval', 1000)
const emitOnChange = useNodeProperty<boolean>('emitOnChange', false)
const emitOnError = useNodeProperty<boolean>('emitOnError', false)

// Write-specific: emitAck toggle drives output port count, only on write nodes.
const emitAck = useStructuralProperty<boolean>('emitAck', false, {
  port: 'outputs',
  derive: (v) => (v ? 1 : 0),
  enabled: () => isWrite.value,
})

const fcOptions = computed(() => (isWrite.value ? MODBUS_WRITE_FCS : MODBUS_READ_FCS))
const isCoilFC = computed(() => MODBUS_COIL_FCS.has(Number(fc.value)))

const modePresets = [
  { label: 'Static (poll)', value: 'static'  },
  { label: 'Dynamic (on input)', value: 'dynamic' },
]

const isDynamic = computed(() => !isWrite.value && mode.value === 'dynamic')

// dataType=bool only makes sense for FC1/FC2 read or FC5 write. The codec
// rejects mismatches at runtime; the UI just hints at it.
const dataTypeError = computed(() => {
  if (dataType.value === 'bool' && !isCoilFC.value) {
    return 'Bool requires FC1/FC2 (read) or FC5 (write)'
  }
  return ''
})

// Multi-register types use word order; single-register / coil types don't.
const showWordOrder = computed(() => {
  return ['int32', 'uint32', 'float32', 'int64', 'uint64', 'float64', 'string', 'raw'].includes(dataType.value)
})

const showQuantity = computed(() => {
  // Quantity matters for raw and string; for typed scalars it's derived.
  return dataType.value === 'raw' || dataType.value === 'string'
})
</script>

<template>
  <div class="flex flex-col gap-3">
    <FormField label="Server">
      <template #action>
        <button
          v-if="server"
          class="text-[9px] uppercase tracking-wider text-terminal-text-dim hover:text-accent transition-colors"
          @click="openEditConfig(server)"
        >Edit</button>
        <button
          class="text-[9px] uppercase tracking-wider text-terminal-text-dim hover:text-accent transition-colors"
          @click="openNewConfig()"
        >+ New</button>
      </template>
      <FormSelect
        v-model="server"
        :options="serverOptions"
        placeholder="Select server..."
      />
    </FormField>

    <FormField v-if="!isWrite" label="Mode">
      <ToggleGroup v-model="mode" :options="modePresets" />
    </FormField>

    <div
      v-if="isDynamic"
      class="text-[10px] text-terminal-text-dim leading-relaxed border border-terminal-border bg-terminal-bg p-2 rounded"
    >
      Send any message to trigger a read. Override per-call via
      <span class="font-mono text-accent">msg.fc</span>,
      <span class="font-mono text-accent">msg.address</span>,
      <span class="font-mono text-accent">msg.quantity</span>,
      <span class="font-mono text-accent">msg.dataType</span>,
      <span class="font-mono text-accent">msg.unitId</span>.
    </div>

    <FormField label="Unit ID" hint="0 = use server default">
      <NumberInput v-model="unitId" :min="0" :max="255" />
    </FormField>

    <FormField label="Function Code">
      <FormSelect v-model="fc" :options="fcOptions" />
    </FormField>

    <FormField label="Address" hint="0-based (40001 → 0)">
      <NumberInput v-model="address" :min="0" :max="65535" />
    </FormField>

    <FormField v-if="showQuantity" label="Quantity">
      <NumberInput v-model="quantity" :min="1" :max="125" />
    </FormField>

    <FormField label="Data Type" :error="dataTypeError">
      <FormSelect v-model="dataType" :options="MODBUS_DATA_TYPES" />
    </FormField>

    <FormField v-if="!isCoilFC && dataType !== 'bool'" label="Byte Order">
      <FormSelect v-model="byteOrder" :options="MODBUS_BYTE_ORDERS" />
    </FormField>

    <FormField v-if="!isCoilFC && showWordOrder" label="Word Order">
      <FormSelect v-model="wordOrder" :options="MODBUS_WORD_ORDERS" />
    </FormField>

    <SectionHeader v-if="!isCoilFC && dataType !== 'bool' && dataType !== 'string' && dataType !== 'raw'" title="Scaling">
      <div class="flex flex-col gap-2">
        <FormField label="Scale" hint="payload = scale * raw + offset">
          <NumberInput v-model="scale" :step="0.01" />
        </FormField>
        <FormField label="Offset">
          <NumberInput v-model="offset" :step="0.01" />
        </FormField>
      </div>
    </SectionHeader>

    <!-- Read-only: polling settings -->
    <SectionHeader v-if="!isWrite && mode === 'static'" title="Polling">
      <div class="flex flex-col gap-2">
        <FormField label="Interval">
          <NumberInput v-model="pollInterval" :min="50" unit="ms" />
        </FormField>
        <FormCheckbox v-model="emitOnChange" label="Emit only on change" />
        <FormCheckbox v-model="emitOnError" label="Emit error message on read failure" />
      </div>
    </SectionHeader>

    <!-- Write-only: ACK toggle -->
    <FormCheckbox
      v-if="isWrite"
      v-model="emitAck"
      label="Emit ACK message after successful write"
    />

    <div
      v-if="isWrite"
      class="text-[10px] text-terminal-text-dim leading-relaxed border border-terminal-border bg-terminal-bg p-2 rounded"
    >
      <span class="font-mono text-accent">msg.payload</span> carries the value to write.
      Override per-call via
      <span class="font-mono text-accent">msg.fc</span>,
      <span class="font-mono text-accent">msg.address</span>,
      <span class="font-mono text-accent">msg.dataType</span>,
      <span class="font-mono text-accent">msg.unitId</span>.
    </div>
  </div>
</template>
