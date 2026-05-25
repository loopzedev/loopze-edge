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

interface StateRule {
  when: unknown
  color?: string
  label?: string
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
const states = useNodeProperty<StateRule[]>('states', [
  { when: true,  color: '#5eba7d', label: 'ON' },
  { when: false, color: '#444',    label: 'OFF' },
])
const offColor = useNodeProperty<string>('offColor', '#444')
const glow = useNodeProperty<boolean>('glow', false)

const groupOptions = computed(() => [
  { value: '', label: '— select group —' },
  ...groupSelectOptions.value,
])

// The `when` field is stored as a typed value (bool, string, number).
// The UI exposes a single text input + a type dropdown so the operator
// can pin "true"/"false" or "alarm" or 1 without ambiguity.
type WhenType = 'bool' | 'string' | 'number'

function whenTypeOf(v: unknown): WhenType {
  if (typeof v === 'boolean') return 'bool'
  if (typeof v === 'number') return 'number'
  return 'string'
}

function whenAsInput(v: unknown): string {
  if (typeof v === 'boolean') return v ? 'true' : 'false'
  return String(v ?? '')
}

function parseWhen(text: string, type: WhenType): unknown {
  switch (type) {
    case 'bool':   return text.trim().toLowerCase() === 'true'
    case 'number': {
      const n = Number(text)
      return Number.isFinite(n) ? n : 0
    }
    default: return text
  }
}

function updateRule(idx: number, patch: Partial<StateRule>) {
  const next = states.value.map((r, i) => (i === idx ? { ...r, ...patch } : r))
  states.value = next
}

function addRule() {
  states.value = [...states.value, { when: '', color: '#58a6ff', label: '' }]
}

function removeRule(idx: number) {
  states.value = states.value.filter((_, i) => i !== idx)
}

const whenTypes = [
  { value: 'bool',   label: 'Bool' },
  { value: 'string', label: 'String' },
  { value: 'number', label: 'Number' },
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

    <FormField label="State rules (first match wins)">
      <div class="flex flex-col gap-2">
        <div
          v-for="(rule, idx) in states"
          :key="idx"
          class="flex items-center gap-1 bg-terminal-bg border border-terminal-border rounded p-2"
        >
          <FormSelect
            :model-value="whenTypeOf(rule.when)"
            :options="whenTypes"
            width="80px"
            @update:model-value="(t) => updateRule(idx, { when: parseWhen(whenAsInput(rule.when), t as WhenType) })"
          />
          <FormInput
            :model-value="whenAsInput(rule.when)"
            placeholder="match"
            mono
            @update:model-value="(v: string) => updateRule(idx, { when: parseWhen(v, whenTypeOf(rule.when)) })"
          />
          <FormColorInput
            :model-value="rule.color ?? ''"
            swatch-only
            @update:model-value="(v: string) => updateRule(idx, { color: v })"
          />
          <FormInput
            :model-value="rule.label ?? ''"
            placeholder="label"
            @update:model-value="(v: string) => updateRule(idx, { label: v })"
          />
          <button
            class="text-xs px-2 text-terminal-text-dim hover:text-status-error"
            type="button"
            @click="removeRule(idx)"
          >
            ✕
          </button>
        </div>
        <button
          class="text-xs px-2 py-1 border border-terminal-border rounded text-terminal-text-dim hover:text-accent self-start"
          type="button"
          @click="addRule"
        >
          + add rule
        </button>
      </div>
    </FormField>

    <FormField label="Off color (no rule matched)">
      <FormColorInput v-model="offColor" placeholder="#444" />
    </FormField>

    <FormCheckbox v-model="glow" label="Glow halo around the indicator" />

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
