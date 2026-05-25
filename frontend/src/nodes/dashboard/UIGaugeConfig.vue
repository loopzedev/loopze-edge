<script setup lang="ts">
import { computed } from 'vue'
import FormField from '@/components/ui/FormField.vue'
import FormInput from '@/components/ui/FormInput.vue'
import FormSelect from '@/components/ui/FormSelect.vue'
import FormCheckbox from '@/components/ui/FormCheckbox.vue'
import FormColorInput from '@/components/ui/FormColorInput.vue'
import NumberInput from '@/components/ui/NumberInput.vue'
import { useNodeProperty } from '@/composables/useNodeProperty'
import { useConfigSelector } from '@/composables/useConfigSelector'

interface Threshold {
  value: number
  color: string
}

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
const min = useNodeProperty<number>('min', 0)
const max = useNodeProperty<number>('max', 100)
const unit = useNodeProperty<string>('unit', '')
const decimals = useNodeProperty<number>('decimals', 1)
const style = useNodeProperty<string>('style', 'arc')
const showValue = useNodeProperty<boolean>('showValue', true)
const thresholds = useNodeProperty<Threshold[]>('thresholds', [])

const groupOptions = computed(() => [
  { value: '', label: '— select group —' },
  ...groupSelectOptions.value,
])

const styles = [
  { value: 'arc',  label: 'Arc (270°)' },
  { value: 'full', label: 'Full ring (360°) — Phase 2' },
  { value: 'dial', label: 'Dial — Phase 2' },
]

function updateThreshold(idx: number, patch: Partial<Threshold>) {
  thresholds.value = thresholds.value.map((t, i) => (i === idx ? { ...t, ...patch } : t))
}
function addThreshold() {
  const last = thresholds.value[thresholds.value.length - 1]
  const base = last ? last.value + 10 : Math.floor((min.value + max.value) / 2)
  thresholds.value = [...thresholds.value, { value: base, color: '#f59e0b' }]
}
function removeThreshold(idx: number) {
  thresholds.value = thresholds.value.filter((_, i) => i !== idx)
}
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

    <FormField label="Range">
      <div class="flex items-stretch gap-1">
        <div class="flex-1 min-w-0">
          <NumberInput v-model="min" />
        </div>
        <div class="flex-1 min-w-0">
          <NumberInput v-model="max" />
        </div>
      </div>
    </FormField>

    <FormField label="Unit">
      <FormInput v-model="unit" placeholder="e.g. °C, bar" />
    </FormField>

    <FormField label="Decimals">
      <NumberInput v-model="decimals" :min="0" :max="6" />
    </FormField>

    <FormField label="Style">
      <FormSelect v-model="style" :options="styles" />
    </FormField>

    <FormCheckbox v-model="showValue" label="Show numeric readout inside the gauge" />

    <FormField label="Thresholds (ascending)">
      <div class="flex flex-col gap-2">
        <div
          v-for="(t, idx) in thresholds"
          :key="idx"
          class="flex items-center gap-1 bg-terminal-bg border border-terminal-border rounded p-2"
        >
          <NumberInput
            :model-value="t.value"
            @update:model-value="(v) => updateThreshold(idx, { value: Number(v) })"
          />
          <FormColorInput
            :model-value="t.color"
            placeholder="#color"
            @update:model-value="(v: string) => updateThreshold(idx, { color: v })"
          />
          <button
            class="text-xs px-2 text-terminal-text-dim hover:text-status-error"
            type="button"
            @click="removeThreshold(idx)"
          >
            ✕
          </button>
        </div>
        <button
          class="text-xs px-2 py-1 border border-terminal-border rounded text-terminal-text-dim hover:text-accent self-start"
          type="button"
          @click="addThreshold"
        >
          + add threshold
        </button>
        <div class="text-[10px] text-terminal-text-dim leading-tight">
          Each threshold colours the arc once the value reaches it.
        </div>
      </div>
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
  </div>
</template>
