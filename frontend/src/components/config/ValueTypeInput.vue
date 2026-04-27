<script setup lang="ts">
import { computed, defineAsyncComponent } from 'vue'
import FormInput from '@/components/ui/FormInput.vue'
import FormSelect from '@/components/ui/FormSelect.vue'
import { VALUE_TYPES, TIMESTAMP_FORMATS, STORAGE_TYPES, placeholderFor } from './enums'

// Lazy — Monaco is heavy and most ValueTypeInput rows render plain inputs.
const CodeEditor = defineAsyncComponent(() =>
  import('@/components/ui/CodeEditor.vue')
)

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

const filteredTypes = computed(() =>
  VALUE_TYPES.filter(t => !props.excludeTypes.includes(String(t.value)))
)

const isContextScope = computed(() =>
  props.type === 'flow' || props.type === 'global'
)

const isExpr = computed(() => props.type === 'expr')

const placeholder = computed(() => placeholderFor(props.type))
</script>

<template>
  <div class="flex items-start gap-1.5">
    <span
      v-if="label"
      class="text-[10px] text-terminal-text-dim shrink-0 w-[52px] text-right pt-1"
    >{{ label }}</span>

    <FormSelect
      :model-value="type"
      :options="filteredTypes"
      width="80px"
      @update:model-value="emit('update:type', String($event))"
    />

    <!-- expr: small Monaco editor with the expr tokenizer -->
    <div v-if="isExpr" class="flex-1 min-w-0 flex flex-col">
      <CodeEditor
        :model-value="value"
        language="expr"
        :placeholder="placeholder"
        min-height="60px"
        @update:model-value="emit('update:value', $event)"
      />
    </div>

    <!-- date: timestamp format selector -->
    <FormSelect
      v-else-if="type === 'date'"
      :model-value="value || 'epoch'"
      :options="TIMESTAMP_FORMATS"
      class="flex-1 min-w-0"
      @update:model-value="emit('update:value', String($event))"
    />

    <!-- everything else: plain text input -->
    <FormInput
      v-else
      :model-value="value"
      mono
      class="flex-1 min-w-0"
      :placeholder="placeholder"
      @update:model-value="emit('update:value', $event)"
    />

    <FormSelect
      v-if="isContextScope"
      :model-value="storage"
      :options="STORAGE_TYPES"
      width="80px"
      @update:model-value="emit('update:storage', String($event))"
    />
  </div>
</template>
