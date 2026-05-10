<script setup lang="ts">
import FormSelect from '@/components/ui/FormSelect.vue'

defineProps<{
  modelValue: string
  width?: string
}>()

defineEmits<{
  'update:modelValue': [value: string | number]
}>()

// Mirror the backend's opcuaScalarTypes set (internal/nodes/opcua_types.go).
// Order is the spec's built-in order — easier to scan when picking by hand.
const OPCUA_DATA_TYPES = [
  { value: 'Boolean', label: 'Boolean' },
  { value: 'SByte', label: 'SByte (int8)' },
  { value: 'Byte', label: 'Byte (uint8)' },
  { value: 'Int16', label: 'Int16' },
  { value: 'UInt16', label: 'UInt16' },
  { value: 'Int32', label: 'Int32' },
  { value: 'UInt32', label: 'UInt32' },
  { value: 'Int64', label: 'Int64' },
  { value: 'UInt64', label: 'UInt64' },
  { value: 'Float', label: 'Float (single)' },
  { value: 'Double', label: 'Double' },
  { value: 'String', label: 'String' },
  { value: 'DateTime', label: 'DateTime' },
  { value: 'GUID', label: 'GUID' },
  { value: 'ByteString', label: 'ByteString' },
  { value: 'NodeID', label: 'NodeID' },
  { value: 'ExtensionObject', label: 'ExtensionObject (struct)' },
  { value: 'Variant', label: 'Variant (let server decide)' },
]
</script>

<template>
  <FormSelect
    :model-value="modelValue"
    :options="OPCUA_DATA_TYPES"
    :width="width"
    @update:model-value="$emit('update:modelValue', $event)"
  />
</template>
