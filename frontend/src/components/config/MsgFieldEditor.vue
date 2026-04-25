<script setup lang="ts">
import { computed } from 'vue'
import FormInput from '@/components/ui/FormInput.vue'
import FormSelect from '@/components/ui/FormSelect.vue'
import {
  VALUE_TYPES,
  TIMESTAMP_FORMATS,
  STORAGE_TYPES,
  VALUE_TYPE_FAMILY,
  isContextScope,
  placeholderFor,
  type OptionEntry,
} from '@/components/config/enums'

export interface MsgField {
  /** msg property path */
  p: string
  /** value type (str, num, flow, ...) */
  vt: string
  /** raw value or context key */
  v: string
  /** memory | persistent (for flow./global. only) */
  vs: string
}

const props = withDefaults(defineProps<{
  modelValue: MsgField
  /** Hide value-types that don't make sense in the current context (e.g. "msg" inside Inject). */
  excludeTypes?: string[]
  /** Optional override of available value types. */
  valueTypes?: OptionEntry[]
  /** Mark the property name as required for visual feedback. */
  propertyRequired?: boolean
}>(), {
  excludeTypes: () => [],
  valueTypes: () => VALUE_TYPES,
  propertyRequired: false,
})

const emit = defineEmits<{
  'update:modelValue': [field: MsgField]
}>()

const filteredTypes = computed(() =>
  props.valueTypes.filter(t => !props.excludeTypes.includes(t.value as string)),
)

const showStorage = computed(() => isContextScope(props.modelValue.vt))

const familyDot = computed(() => {
  const family = VALUE_TYPE_FAMILY[props.modelValue.vt] ?? 'literal'
  switch (family) {
    case 'context': return 'bg-accent'
    case 'dynamic': return 'bg-status-warning'
    case 'literal': default: return 'bg-terminal-text-dim'
  }
})

function patch(partial: Partial<MsgField>) {
  emit('update:modelValue', { ...props.modelValue, ...partial })
}
</script>

<template>
  <!-- Row 1: msg.<property> -->
  <div class="flex flex-col gap-0.5">
    <FormInput
      :model-value="modelValue.p"
      mono
      :invalid="propertyRequired && !modelValue.p"
      placeholder="property"
      @update:model-value="patch({ p: $event })"
    >
      <template #prefix>msg.</template>
    </FormInput>
    <div
      v-if="propertyRequired && !modelValue.p"
      class="text-[10px] text-status-error leading-tight"
    >
      Property name required
    </div>
  </div>

  <!-- Row 2: type-badge + value (+ storage when context) -->
  <div class="flex items-center gap-1.5">
    <div class="relative shrink-0">
      <FormSelect
        :model-value="modelValue.vt"
        :options="filteredTypes"
        width="92px"
        @update:model-value="patch({ vt: $event })"
      />
      <!-- Family dot overlay -->
      <span
        class="absolute left-1.5 top-1/2 -translate-y-1/2 w-1.5 h-1.5 rounded-full pointer-events-none"
        :class="familyDot"
      />
    </div>

    <FormSelect
      v-if="modelValue.vt === 'date'"
      :model-value="modelValue.v || 'epoch'"
      :options="TIMESTAMP_FORMATS"
      class="flex-1 min-w-0"
      @update:model-value="patch({ v: $event })"
    />
    <FormInput
      v-else
      :model-value="modelValue.v"
      mono
      class="flex-1 min-w-0"
      :placeholder="placeholderFor(modelValue.vt)"
      @update:model-value="patch({ v: $event })"
    />

    <FormSelect
      v-if="showStorage"
      :model-value="modelValue.vs"
      :options="STORAGE_TYPES"
      width="80px"
      @update:model-value="patch({ vs: $event })"
    />
  </div>
</template>

<style scoped>
/* Make space for the family dot overlay inside the FormSelect trigger */
.relative :deep(button) {
  padding-left: 12px;
}
</style>
