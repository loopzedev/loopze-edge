<script setup lang="ts">
import { computed } from 'vue'
import FormField from '@/components/ui/FormField.vue'
import FormInput from '@/components/ui/FormInput.vue'
import FormSelect from '@/components/ui/FormSelect.vue'
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
const label = useNodeProperty<string>('label', '')
const tooltip = useNodeProperty<string>('tooltip', '')
const property = useNodeProperty<string>('property', 'payload')
const layout = useNodeProperty<string>('layout', 'row-spread')
const format = useNodeProperty<string>('format', 'text')
const decimals = useNodeProperty<number>('decimals', 2)
const unit = useNodeProperty<string>('unit', '')
const color = useNodeProperty<string>('color', '')

const groupOptions = computed(() => [
  { value: '', label: '— select group —' },
  ...groupSelectOptions.value,
])

const layouts = [
  { value: 'row-spread', label: 'Row: label left, value right' },
  { value: 'row-left',   label: 'Row: label-value left' },
  { value: 'row-right',  label: 'Row: value-label right' },
  { value: 'row-center', label: 'Row: centred' },
  { value: 'col-center', label: 'Stacked, centred' },
]
const formats = [
  { value: 'text',   label: 'Text' },
  { value: 'number', label: 'Number (with decimals)' },
  { value: 'json',   label: 'JSON (pretty-printed)' },
]
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
      <FormInput v-model="label" placeholder="optional" />
    </FormField>

    <FormField label="Tooltip">
      <FormInput v-model="tooltip" placeholder="optional" />
    </FormField>

    <FormField label="Property">
      <FormInput v-model="property" mono placeholder="payload">
        <template #prefix>msg.</template>
      </FormInput>
    </FormField>

    <FormField label="Layout">
      <FormSelect v-model="layout" :options="layouts" />
    </FormField>

    <FormField label="Format">
      <FormSelect v-model="format" :options="formats" />
    </FormField>

    <FormField v-if="format === 'number'" label="Decimals">
      <NumberInput v-model="decimals" :min="0" :max="6" />
    </FormField>

    <FormField label="Unit">
      <FormInput v-model="unit" placeholder="e.g. °C" />
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

    <FormField label="Color (hex)">
      <FormInput v-model="color" mono placeholder="defaults to terminal fg" />
    </FormField>
  </div>
</template>
