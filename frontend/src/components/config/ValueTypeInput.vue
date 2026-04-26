<script setup lang="ts">
import { computed } from 'vue'
import FormInput from '@/components/ui/FormInput.vue'
import FormSelect from '@/components/ui/FormSelect.vue'

const props = withDefaults(defineProps<{
  value: string
  type: string
  storage?: string
  excludeTypes?: string[]
  label?: string
}>(), {
  storage: 'memory',
  excludeTypes: () => [],
  label: '',
})

const emit = defineEmits<{
  'update:value': [value: string]
  'update:type': [value: string]
  'update:storage': [value: string]
}>()

const allValueTypes = [
  { value: 'msg', label: 'msg.' },
  { value: 'flow', label: 'flow.' },
  { value: 'global', label: 'global.' },
  { value: 'str', label: 'string' },
  { value: 'num', label: 'number' },
  { value: 'bool', label: 'boolean' },
  { value: 'json', label: 'JSON' },
  { value: 'date', label: 'timestamp' },
  { value: 'env', label: 'env' },
]

const timestampFormats = [
  { value: 'epoch', label: 'milliseconds since epoch' },
  { value: 'rfc3339', label: 'YYYY-MM-DDTHH:mm:ss.sssZ' },
]

const storageTypes = [
  { value: 'memory', label: 'memory' },
  { value: 'persistent', label: 'persist' },
]

const filteredTypes = computed(() =>
  allValueTypes.filter(t => !props.excludeTypes.includes(t.value))
)

const isContextScope = computed(() =>
  props.type === 'flow' || props.type === 'global'
)

const placeholder = computed(() => {
  switch (props.type) {
    case 'json': return '{...}'
    case 'bool': return 'true / false'
    case 'env':  return 'ENV_VAR_NAME'
    case 'msg':  return 'property path'
    case 'flow': case 'global': return 'key'
    default: return 'value'
  }
})
</script>

<template>
  <div class="flex items-center gap-1.5">
    <span v-if="label" class="text-[10px] text-terminal-text-dim shrink-0 w-[52px] text-right">{{ label }}</span>

    <FormSelect
      :model-value="type"
      :options="filteredTypes"
      width="80px"
      @update:model-value="emit('update:type', String($event))"
    />

    <FormInput
      v-if="type !== 'date'"
      :model-value="value"
      mono
      class="flex-1 min-w-0"
      :placeholder="placeholder"
      @update:model-value="emit('update:value', $event)"
    />

    <FormSelect
      v-else
      :model-value="value || 'epoch'"
      :options="timestampFormats"
      class="flex-1 min-w-0"
      @update:model-value="emit('update:value', String($event))"
    />

    <FormSelect
      v-if="isContextScope"
      :model-value="storage"
      :options="storageTypes"
      width="80px"
      @update:model-value="emit('update:storage', String($event))"
    />
  </div>
</template>
