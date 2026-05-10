<script setup lang="ts">
import FormField from '@/components/ui/FormField.vue'
import FormSelect from '@/components/ui/FormSelect.vue'
import ToggleGroup from '@/components/ui/ToggleGroup.vue'
import NumberInput from '@/components/ui/NumberInput.vue'
import { useNodeProperty } from '@/composables/useNodeProperty'
import { computed } from 'vue'

type DelayMode = 'delay' | 'rate' | 'random'
type Behaviour = 'queue' | 'drop'
type DelayUnit = 'milliseconds' | 'seconds' | 'minutes' | 'hours' | 'day'
type RateUnit = 'second' | 'minute' | 'hour' | 'day'

const mode = useNodeProperty<DelayMode>('mode', 'delay')

// Mode "delay" — fixed timeout
const timeout = useNodeProperty<number>('timeout', 500)
const timeoutUnits = useNodeProperty<DelayUnit>('timeoutUnits', 'milliseconds')

// Mode "rate" — N messages per unit
const rate = useNodeProperty<number>('rate', 1)
const rateUnits = useNodeProperty<RateUnit>('rateUnits', 'second')
const behaviour = useNodeProperty<Behaviour>('behaviour', 'queue')
const maxQueueLength = useNodeProperty<number>('maxQueueLength', 1000)

// Mode "random" — uniform delay in [first, last]
const randomFirst = useNodeProperty<number>('randomFirst', 0)
const randomLast = useNodeProperty<number>('randomLast', 1000)
const randomUnits = useNodeProperty<DelayUnit>('randomUnits', 'milliseconds')

const modeOptions = [
  { value: 'delay', label: 'Delay each message' },
  { value: 'rate', label: 'Rate limit' },
  { value: 'random', label: 'Random delay' },
]

const delayUnitOptions = [
  { value: 'milliseconds', label: 'milliseconds' },
  { value: 'seconds', label: 'seconds' },
  { value: 'minutes', label: 'minutes' },
  { value: 'hours', label: 'hours' },
  { value: 'day', label: 'day' },
]

const rateUnitOptions = [
  { value: 'second', label: 'second' },
  { value: 'minute', label: 'minute' },
  { value: 'hour', label: 'hour' },
  { value: 'day', label: 'day' },
]

const behaviourOptions = [
  { value: 'queue', label: 'Queue intermediate' },
  { value: 'drop', label: 'Drop intermediate' },
]

// Validation: random range must be ordered.
const randomRangeInvalid = computed(() => randomLast.value < randomFirst.value)
</script>

<template>
  <div class="flex flex-col gap-3">
    <FormField label="Action">
      <ToggleGroup v-model="mode" :options="modeOptions" />
    </FormField>

    <!-- Mode: delay ------------------------------------------------------- -->
    <template v-if="mode === 'delay'">
      <FormField label="For">
        <div class="flex gap-1">
          <NumberInput v-model="timeout" :min="0" :step="100" />
          <FormSelect v-model="timeoutUnits" :options="delayUnitOptions" />
        </div>
      </FormField>
    </template>

    <!-- Mode: rate -------------------------------------------------------- -->
    <template v-if="mode === 'rate'">
      <FormField label="Rate">
        <div class="flex gap-1 items-center">
          <NumberInput v-model="rate" :min="1" :step="1" />
          <span class="text-[10px] text-terminal-text-dim px-1">msg /</span>
          <FormSelect v-model="rateUnits" :options="rateUnitOptions" />
        </div>
      </FormField>

      <FormField label="Overflow behaviour">
        <ToggleGroup v-model="behaviour" :options="behaviourOptions" />
      </FormField>

      <FormField v-if="behaviour === 'queue'" label="Max queue length">
        <NumberInput v-model="maxQueueLength" :min="1" :step="100" unit="msg" />
      </FormField>
    </template>

    <!-- Mode: random ------------------------------------------------------ -->
    <template v-if="mode === 'random'">
      <FormField label="Between">
        <div class="flex gap-1 items-center">
          <NumberInput v-model="randomFirst" :min="0" :step="100" />
          <span class="text-[10px] text-terminal-text-dim px-1">and</span>
          <NumberInput
            v-model="randomLast"
            :min="0"
            :step="100"
            :invalid="randomRangeInvalid"
          />
          <FormSelect v-model="randomUnits" :options="delayUnitOptions" />
        </div>
        <p
          v-if="randomRangeInvalid"
          class="text-[10px] text-status-error mt-1"
        >
          Upper bound must be greater than or equal to lower bound.
        </p>
      </FormField>
    </template>

    <!-- Hint about per-message overrides --------------------------------- -->
    <p class="text-[10px] text-terminal-text-dim leading-relaxed">
      Per-message overrides:
      <code class="text-terminal-text">msg.delay</code> (ms),
      <code class="text-terminal-text">msg.flush</code>,
      <code class="text-terminal-text">msg.reset</code>.
    </p>
  </div>
</template>
