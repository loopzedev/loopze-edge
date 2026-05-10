<script setup lang="ts">
import FormCheckbox from '@/components/ui/FormCheckbox.vue'
import FormField from '@/components/ui/FormField.vue'
import FormInput from '@/components/ui/FormInput.vue'
import ToggleGroup from '@/components/ui/ToggleGroup.vue'
import NumberInput from '@/components/ui/NumberInput.vue'
import PropertyList from '@/components/ui/PropertyList.vue'
import PropertyListItem from '@/components/ui/PropertyListItem.vue'
import MsgFieldEditor, { type MsgField } from '@/components/config/MsgFieldEditor.vue'
import { useNodeProperty } from '@/composables/useNodeProperty'
import { INTERVAL_PRESETS } from '@/components/config/enums'
import { computed } from 'vue'
import cronstrue from 'cronstrue'
import { CronExpressionParser } from 'cron-parser'

const TRIGGER_MODES = [
  { value: 'interval', label: 'Interval' },
  { value: 'cron', label: 'Cron' },
]

const once = useNodeProperty<boolean>('once', false)
const mode = useNodeProperty<string>('mode', 'interval')
const interval = useNodeProperty<number>('interval', 0)
const cronExpr = useNodeProperty<string>('cron', '')

const rawProps = useNodeProperty<MsgField[]>('props', [
  { p: 'payload', vt: 'date', v: 'rfc3339', vs: 'memory' },
  { p: 'topic', vt: 'str', v: '', vs: 'memory' },
])

// Normalise: ensure every entry has all four fields (older configs may miss vs).
const fields = computed<MsgField[]>({
  get: () => (rawProps.value ?? []).map((r) => ({
    p: r?.p ?? '',
    vt: r?.vt ?? 'str',
    v: r?.v ?? '',
    vs: r?.vs ?? 'memory',
  })),
  set: (val) => { rawProps.value = val },
})

function newField(): MsgField {
  return { p: '', vt: 'str', v: '', vs: 'memory' }
}

function updateField(idx: number, next: MsgField) {
  const updated = [...fields.value]
  updated[idx] = next
  fields.value = updated
}

// Inject has no input message → exclude "msg" as a value source.
const excludeTypes = ['msg']

// Decode the cron expression: human-readable description + next 5 fire times.
// Backend uses robfig/cron with 6-field (seconds) parser; cron-parser and
// cronstrue both default to that shape too.
const cronInfo = computed(() => {
  const expr = (cronExpr.value ?? '').trim()
  if (!expr) {
    return { description: '', nextRuns: [] as string[], error: '' }
  }
  let description = ''
  try {
    description = cronstrue.toString(expr, { use24HourTimeFormat: true })
  } catch (e) {
    const msg = e instanceof Error ? e.message : String(e)
    return { description: '', nextRuns: [], error: msg }
  }
  try {
    const it = CronExpressionParser.parse(expr)
    const nextRuns: string[] = []
    for (let i = 0; i < 5; i++) {
      nextRuns.push(it.next().toDate().toLocaleString())
    }
    return { description, nextRuns, error: '' }
  } catch (e) {
    const msg = e instanceof Error ? e.message : String(e)
    return { description, nextRuns: [], error: msg }
  }
})
</script>

<template>
  <div class="flex flex-col gap-3">
    <!-- Trigger: once -->
    <FormCheckbox v-model="once" label="Inject once at startup" />

    <!-- Trigger mode toggle -->
    <FormField label="Trigger Mode">
      <ToggleGroup v-model="mode" :options="TRIGGER_MODES" />
    </FormField>

    <!-- Interval mode -->
    <FormField v-if="mode !== 'cron'" label="Repeat Interval">
      <ToggleGroup v-model="interval" :options="INTERVAL_PRESETS" />
      <NumberInput
        v-model="interval"
        :min="0"
        :step="100"
        unit="ms"
        placeholder="Custom interval"
      />
    </FormField>

    <!-- Cron mode -->
    <template v-else>
      <FormField
        label="Cron Expression"
        hint="6 fields: sec min hour dom month dow (e.g. 0 */5 * * * *)"
        :error="cronInfo.error"
      >
        <FormInput
          v-model="cronExpr"
          mono
          placeholder="0 */5 * * * *"
          :invalid="!!cronInfo.error"
        />
      </FormField>

      <div
        v-if="cronInfo.description"
        class="border border-terminal-border bg-terminal-surface-alt/30 px-2 py-1.5 text-[10px] flex flex-col gap-1"
      >
        <span class="text-terminal-text">{{ cronInfo.description }}</span>
        <div v-if="cronInfo.nextRuns.length" class="flex flex-col gap-0.5 text-terminal-text-dim">
          <span class="uppercase tracking-wider text-[9px]">Next runs</span>
          <span
            v-for="(run, i) in cronInfo.nextRuns"
            :key="i"
            class="font-mono"
          >
            {{ run }}
          </span>
        </div>
      </div>
    </template>

    <!-- Message fields -->
    <FormField :label="`Message Fields · ${fields.length}`">
      <PropertyList
        v-model="fields"
        :new-item="newField"
        add-label="+ add field"
      >
        <template #default="{ item, index, isDragging, isDropTarget, remove, handleMousedown }">
          <PropertyListItem
            :is-dragging="isDragging"
            :is-drop-target="isDropTarget"
            :is-invalid="!item.p"
            @remove="remove"
            @handle-mousedown="handleMousedown"
          >
            <MsgFieldEditor
              :model-value="item"
              :exclude-types="excludeTypes"
              property-required
              @update:model-value="updateField(index, $event)"
            />
          </PropertyListItem>
        </template>
      </PropertyList>
    </FormField>
  </div>
</template>
