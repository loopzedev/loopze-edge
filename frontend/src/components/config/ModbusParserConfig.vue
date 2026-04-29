<script setup lang="ts">
import { computed } from 'vue'
import FormInput from '@/components/ui/FormInput.vue'
import FormSelect from '@/components/ui/FormSelect.vue'
import FormField from '@/components/ui/FormField.vue'
import ToggleGroup from '@/components/ui/ToggleGroup.vue'
import SectionHeader from '@/components/ui/SectionHeader.vue'
import ModbusParserLayoutEditor from './ModbusParserLayoutEditor.vue'
import { useNodeProperty } from '@/composables/useNodeProperty'
import { MODBUS_BYTE_ORDERS, MODBUS_WORD_ORDERS } from '@/components/config/enums'

const action = useNodeProperty<string>('action', 'auto')
const parseFrom = useNodeProperty<string>('parseFrom', 'bytes')
const encodeFrom = useNodeProperty<string>('encodeFrom', 'payload')
const byteOrder = useNodeProperty<string>('byteOrder', 'bigEndian')
const wordOrder = useNodeProperty<string>('wordOrder', 'bigEndian')

const ACTIONS = [
  { value: 'auto', label: 'Auto' },
  { value: 'parse', label: 'Parse' },
  { value: 'encode', label: 'Encode' },
]

const showParseFrom = computed(() => action.value !== 'encode')
const showEncodeFrom = computed(() => action.value !== 'parse')
</script>

<template>
  <div class="flex flex-col gap-3">
    <FormField label="Action" hint="Auto: map → encode, array/buffer → parse">
      <ToggleGroup v-model="action" :options="ACTIONS" />
    </FormField>

    <FormField v-if="showParseFrom" label="Parse from">
      <FormInput v-model="parseFrom" placeholder="bytes" mono>
        <template #prefix>msg.</template>
      </FormInput>
    </FormField>

    <FormField v-if="showEncodeFrom" label="Encode from">
      <FormInput v-model="encodeFrom" placeholder="payload" mono>
        <template #prefix>msg.</template>
      </FormInput>
    </FormField>

    <SectionHeader title="Default Byte / Word Order">
      <div class="flex flex-col gap-2">
        <FormField label="Byte Order">
          <FormSelect v-model="byteOrder" :options="MODBUS_BYTE_ORDERS" />
        </FormField>
        <FormField label="Word Order">
          <FormSelect v-model="wordOrder" :options="MODBUS_WORD_ORDERS" />
        </FormField>
      </div>
    </SectionHeader>

    <ModbusParserLayoutEditor />
  </div>
</template>
