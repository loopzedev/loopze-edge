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

const { options: groupSelectOptions, openNewConfig: openNewGroup, openEditConfig: openEditGroup } =
  useConfigSelector('ui-group')

// ─── Layout ──────────────────────────────────────────────────────────────────
const group   = useNodeProperty<string>('group', '')
const x       = useNodeProperty<number>('x', 0)
const y       = useNodeProperty<number>('y', 0)
const width   = useNodeProperty<number>('width', 3)
const height  = useNodeProperty<number>('height', 4)
const tooltip = useNodeProperty<string>('tooltip', '')

// ─── Label ───────────────────────────────────────────────────────────────────
const label          = useNodeProperty<string>('label', '')
const sublabel       = useNodeProperty<string>('sublabel', '')
const labelSeparator = useNodeProperty<string>('labelSeparator', '·')

// ─── Value & formatting ──────────────────────────────────────────────────────
const property           = useNodeProperty<string>('property', 'payload')
const decimals           = useNodeProperty<number>('decimals', 0)
const thousandsSeparator = useNodeProperty<string>('thousandsSeparator', 'space')
const decimalSeparator   = useNodeProperty<string>('decimalSeparator', 'dot')
const prefix             = useNodeProperty<string>('prefix', '')
const suffix             = useNodeProperty<string>('suffix', '')
const valueColor         = useNodeProperty<string>('valueColor', '')

// ─── Delta ───────────────────────────────────────────────────────────────────
const showDelta      = useNodeProperty<boolean>('showDelta', true)
const deltaProperty  = useNodeProperty<string>('deltaProperty', 'delta')
const deltaFormat    = useNodeProperty<string>('deltaFormat', 'percent')
const deltaDecimals  = useNodeProperty<number>('deltaDecimals', 1)
const deltaContext   = useNodeProperty<string>('deltaContext', '')
const deltaDirection = useNodeProperty<string>('deltaDirection', 'up-is-good')

// ─── Sparkline ───────────────────────────────────────────────────────────────
const showSparkline   = useNodeProperty<boolean>('showSparkline', true)
const sparklineWindow = useNodeProperty<number>('sparklineWindow', 60)
const sparklineColor  = useNodeProperty<string>('sparklineColor', '')
const sparklineFill   = useNodeProperty<boolean>('sparklineFill', true)

const groupOptions = computed(() => [
  { value: '', label: '— select group —' },
  ...groupSelectOptions.value,
])

const thousandsOptions = [
  { value: 'space', label: 'Space (63 704)' },
  { value: 'comma', label: 'Comma (63,704)' },
  { value: 'dot',   label: 'Dot (63.704)' },
  { value: 'none',  label: 'None (63704)' },
]
const decimalOptions = [
  { value: 'dot',   label: 'Dot (3.14)' },
  { value: 'comma', label: 'Comma (3,14)' },
]

const deltaFormatOptions = [
  { value: 'percent',  label: 'Percent (+8.4%)' },
  { value: 'absolute', label: 'Absolute (+8.4)' },
]
const deltaDirectionOptions = [
  { value: 'up-is-good',   label: 'Up is good (positive → green)' },
  { value: 'down-is-good', label: 'Down is good (positive → red)' },
  { value: 'neutral',      label: 'Neutral (no color)' },
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

    <!-- ── Label ── -->
    <FormField label="Label">
      <FormInput v-model="label" placeholder="e.g. THROUGHPUT" />
    </FormField>
    <FormField label="Sublabel">
      <FormInput v-model="sublabel" placeholder="e.g. TODAY" />
    </FormField>
    <FormField label="Separator">
      <FormInput v-model="labelSeparator" mono placeholder="·" />
    </FormField>

    <!-- ── Value ── -->
    <FormField label="Property">
      <FormInput v-model="property" mono placeholder="payload">
        <template #prefix>msg.</template>
      </FormInput>
    </FormField>
    <FormField label="Decimals">
      <NumberInput v-model="decimals" :min="0" :max="6" />
    </FormField>
    <FormField label="Thousands separator">
      <FormSelect v-model="thousandsSeparator" :options="thousandsOptions" />
    </FormField>
    <FormField label="Decimal separator">
      <FormSelect v-model="decimalSeparator" :options="decimalOptions" />
    </FormField>
    <FormField label="Prefix">
      <FormInput v-model="prefix" mono placeholder="e.g. € " />
    </FormField>
    <FormField label="Suffix">
      <FormInput v-model="suffix" mono placeholder="e.g.  kg" />
    </FormField>
    <FormField label="Value color">
      <FormColorInput v-model="valueColor" placeholder="defaults to white" clearable />
    </FormField>

    <!-- ── Delta ── -->
    <div class="section-divider">Delta (trend)</div>

    <FormCheckbox v-model="showDelta" label="Show delta row" />
    <FormField label="Delta property">
      <FormInput v-model="deltaProperty" mono placeholder="delta" :disabled="!showDelta">
        <template #prefix>msg.</template>
      </FormInput>
    </FormField>
    <FormField label="Format">
      <FormSelect v-model="deltaFormat" :options="deltaFormatOptions" :disabled="!showDelta" />
    </FormField>
    <FormField label="Decimals">
      <NumberInput v-model="deltaDecimals" :min="0" :max="6" :disabled="!showDelta" />
    </FormField>
    <FormField label="Context text">
      <FormInput v-model="deltaContext" placeholder="e.g. VS YESTERDAY" :disabled="!showDelta" />
    </FormField>
    <FormField label="Direction">
      <FormSelect v-model="deltaDirection" :options="deltaDirectionOptions" :disabled="!showDelta" />
    </FormField>

    <!-- ── Sparkline ── -->
    <div class="section-divider">Sparkline</div>

    <FormCheckbox v-model="showSparkline" label="Show sparkline" />
    <FormField label="Window (points)">
      <NumberInput v-model="sparklineWindow" :min="2" :max="2000" :disabled="!showSparkline" />
    </FormField>
    <FormField label="Color">
      <FormColorInput v-model="sparklineColor" placeholder="auto (matches delta or accent)" clearable />
    </FormField>
    <FormCheckbox v-model="sparklineFill" label="Gradient area fill" :disabled="!showSparkline" />

    <!-- ── Common ── -->
    <FormField label="Tooltip">
      <FormInput v-model="tooltip" placeholder="optional" />
    </FormField>
    <FormField label="Position (column × row, 0-based)">
      <div class="flex items-stretch gap-1">
        <div class="flex-1 min-w-0"><NumberInput v-model="x" :min="0" :max="47" unit="X" /></div>
        <div class="flex-1 min-w-0"><NumberInput v-model="y" :min="0" :max="100" unit="Y" /></div>
      </div>
    </FormField>
    <FormField label="Size (W × H grid units)">
      <div class="flex items-stretch gap-1">
        <div class="flex-1 min-w-0"><NumberInput v-model="width" :min="1" :max="48" unit="W" /></div>
        <div class="flex-1 min-w-0"><NumberInput v-model="height" :min="1" :max="48" unit="H" /></div>
      </div>
    </FormField>
  </div>
</template>

<style scoped>
.section-divider {
  font-size: 0.65rem;
  font-weight: 700;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: var(--color-text-dim, #5a6070);
  border-top: 1px solid var(--color-border, #2a2e38);
  padding-top: 0.6rem;
  margin-top: 0.25rem;
}
</style>
