<script setup lang="ts">
import { computed } from 'vue'
import FormField from '@/components/ui/FormField.vue'
import FormInput from '@/components/ui/FormInput.vue'
import FormSelect from '@/components/ui/FormSelect.vue'
import FormColorInput from '@/components/ui/FormColorInput.vue'
import NumberInput from '@/components/ui/NumberInput.vue'
import { useNodeProperty } from '@/composables/useNodeProperty'
import { useConfigSelector } from '@/composables/useConfigSelector'

const { options: groupSelectOptions, openNewConfig: openNewGroup, openEditConfig: openEditGroup } =
  useConfigSelector('ui-group')

const group = useNodeProperty<string>('group', '')
const x = useNodeProperty<number>('x', 0)
const y = useNodeProperty<number>('y', 0)
const width = useNodeProperty<number>('width', 0)
const height = useNodeProperty<number>('height', 0)
const label = useNodeProperty<string>('label', 'Click me')
const tooltip = useNodeProperty<string>('tooltip', '')
const payloadType = useNodeProperty<string>('payloadType', 'bool')
const payload = useNodeProperty<string>('payload', '')
const topic = useNodeProperty<string>('topic', '')
const color = useNodeProperty<string>('color', '')

const payloadTypes = [
  { value: 'bool',      label: 'Boolean (true / false)' },
  { value: 'string',    label: 'String' },
  { value: 'number',    label: 'Number' },
  { value: 'timestamp', label: 'Timestamp (ms)' },
]

// Dropdown options come from useConfigSelector; prepend an empty
// placeholder so the operator sees that nothing is selected yet.
const groupOptions = computed(() => [
  { value: '', label: '— select group —' },
  ...groupSelectOptions.value,
])

const payloadHint = computed(() => {
  switch (payloadType.value) {
    case 'bool':      return 'Emits true unless "false" is entered.'
    case 'string':    return 'Emits the literal string.'
    case 'number':    return 'Parsed as a float; non-numeric → 0.'
    case 'timestamp': return 'Emits the click time in unix milliseconds.'
    default:          return ''
  }
})
</script>

<template>
  <div class="flex flex-col gap-3">
    <FormField label="Group" :error="!group ? 'Group required' : ''">
      <template #action>
        <button
          v-if="group"
          type="button"
          class="text-[9px] uppercase tracking-wider text-terminal-text-dim hover:text-accent transition-colors"
          @click="openEditGroup(group)"
        >Edit</button>
        <button
          type="button"
          class="text-[9px] uppercase tracking-wider text-terminal-text-dim hover:text-accent transition-colors"
          @click="openNewGroup()"
        >+ New</button>
      </template>
      <FormSelect v-model="group" :options="groupOptions" />
    </FormField>

    <FormField label="Label">
      <FormInput v-model="label" placeholder="Click me" />
    </FormField>

    <FormField label="Tooltip">
      <FormInput v-model="tooltip" placeholder="optional" />
    </FormField>

    <FormField label="Payload type">
      <FormSelect v-model="payloadType" :options="payloadTypes" />
    </FormField>

    <FormField label="Payload">
      <FormInput v-model="payload" mono placeholder="bool/string/number value" />
      <div class="text-[10px] text-terminal-text-dim leading-tight">{{ payloadHint }}</div>
    </FormField>

    <FormField label="Topic (msg.topic)">
      <FormInput v-model="topic" mono placeholder="optional" />
    </FormField>

    <FormField label="Position (column × row, 0-based)">
      <div class="flex items-stretch gap-1">
        <div class="flex-1 min-w-0">
          <NumberInput v-model="x" :min="0" :max="11" unit="X" />
        </div>
        <div class="flex-1 min-w-0">
          <NumberInput v-model="y" :min="0" :max="100" unit="Y" />
        </div>
      </div>
    </FormField>

    <FormField label="Size (W × H grid units, W=0 = full row)">
      <div class="flex items-stretch gap-1">
        <div class="flex-1 min-w-0">
          <NumberInput v-model="width" :min="0" :max="12" unit="W" />
        </div>
        <div class="flex-1 min-w-0">
          <NumberInput v-model="height" :min="1" :max="12" unit="H" />
        </div>
      </div>
    </FormField>

    <FormField label="Color">
      <FormColorInput v-model="color" placeholder="#58a6ff (defaults to accent)" clearable />
    </FormField>
  </div>
</template>
