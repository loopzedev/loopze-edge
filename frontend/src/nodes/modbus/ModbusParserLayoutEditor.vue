<script setup lang="ts">
import { computed, ref } from 'vue'
import FormInput from '@/components/ui/FormInput.vue'
import FormSelect from '@/components/ui/FormSelect.vue'
import FormField from '@/components/ui/FormField.vue'
import NumberInput from '@/components/ui/NumberInput.vue'
import PropertyList from '@/components/ui/PropertyList.vue'
import PropertyListItem from '@/components/ui/PropertyListItem.vue'
import { useNodeProperty } from '@/composables/useNodeProperty'
import { MODBUS_DATA_TYPES, MODBUS_BYTE_ORDERS, MODBUS_WORD_ORDERS } from './enums'

interface LayoutField {
  id: string
  offset: number
  name: string
  type: string
  length: number
  byteOrder: string  // "" = inherit
  wordOrder: string  // "" = inherit
  scale: number
  offsetValue: number
  unit: string
  bit: number        // -1 = unset
}

// Default new field values. Bit = -1 sentinel matches the backend.
function defaultField(): LayoutField {
  return {
    id: crypto.randomUUID(),
    offset: 0,
    name: '',
    type: 'uint16',
    length: 1,
    byteOrder: '',
    wordOrder: '',
    scale: 1,
    offsetValue: 0,
    unit: '',
    bit: -1,
  }
}

// Read raw layout from the store, defensively fill in IDs and defaults so the
// drag-key is stable and the table never shows undefined inputs.
const rawLayout = useNodeProperty<Partial<LayoutField>[]>('layout', [])

const layout = computed<LayoutField[]>({
  get: () =>
    (rawLayout.value ?? []).map((f) => ({
      id: (f.id as string) || crypto.randomUUID(),
      offset: Number(f.offset ?? 0),
      name: String(f.name ?? ''),
      type: String(f.type ?? 'uint16'),
      length: Number(f.length ?? 1),
      byteOrder: String(f.byteOrder ?? ''),
      wordOrder: String(f.wordOrder ?? ''),
      scale: Number(f.scale ?? 1),
      offsetValue: Number(f.offsetValue ?? 0),
      unit: String(f.unit ?? ''),
      bit: f.bit === undefined ? -1 : Number(f.bit),
    })),
  set: (val) => {
    rawLayout.value = val
  },
})

function update<K extends keyof LayoutField>(idx: number, key: K, value: LayoutField[K]) {
  const next = [...layout.value]
  next[idx] = { ...next[idx], [key]: value }
  layout.value = next
}

// Per-row advanced-toggle state — only one row's drawer is open at a time.
const advancedIdx = ref<number | null>(null)
function toggleAdvanced(idx: number) {
  advancedIdx.value = advancedIdx.value === idx ? null : idx
}

// Live duplicate-name detection — flagged red on the row's invalid border.
const nameCounts = computed(() => {
  const counts: Record<string, number> = {}
  for (const f of layout.value) {
    if (f.name) counts[f.name] = (counts[f.name] ?? 0) + 1
  }
  return counts
})

function isDuplicateName(name: string): boolean {
  return !!name && (nameCounts.value[name] ?? 0) > 1
}

// Length is only meaningful for string and raw — hide the input otherwise.
function needsLength(type: string): boolean {
  return type === 'string' || type === 'raw'
}

// Bit only applies to type=bool.
function canHaveBit(type: string): boolean {
  return type === 'bool'
}

// Override-aware byte/word-order options: prepend an "(inherit)" entry that
// maps to "" — the backend treats empty string as "use node default".
const byteOrderOverrideOptions = computed(() => [
  { value: '', label: '(inherit default)' },
  ...MODBUS_BYTE_ORDERS,
])
const wordOrderOverrideOptions = computed(() => [
  { value: '', label: '(inherit default)' },
  ...MODBUS_WORD_ORDERS,
])
</script>

<template>
  <FormField :label="`Layout · ${layout.length} field${layout.length === 1 ? '' : 's'}`">
    <PropertyList
      v-model="layout"
      :item-key="(item) => item.id"
      :new-item="defaultField"
      add-label="+ add field"
    >
      <template #default="{ item, index, isDragging, isDropTarget, remove, handleMousedown }">
        <PropertyListItem
          :is-dragging="isDragging"
          :is-drop-target="isDropTarget"
          :is-invalid="!item.name || isDuplicateName(item.name)"
          @remove="remove"
          @handle-mousedown="handleMousedown"
        >
          <!-- Row 1: name (prominent) · type · ⚙ -->
          <div class="flex items-center gap-1.5">
            <FormInput
              :model-value="item.name"
              placeholder="field name"
              mono
              class="flex-1 min-w-0"
              :invalid="!item.name || isDuplicateName(item.name)"
              @update:model-value="update(index, 'name', String($event))"
            />
            <FormSelect
              :model-value="item.type"
              :options="MODBUS_DATA_TYPES"
              width="120px"
              @update:model-value="update(index, 'type', String($event))"
            />
            <button
              type="button"
              class="shrink-0 text-[11px] px-1 py-0.5 transition-colors"
              :class="advancedIdx === index ? 'text-accent' : 'text-terminal-text-dim hover:text-accent'"
              :title="advancedIdx === index ? 'Hide advanced options' : 'Show advanced options'"
              @click="toggleAdvanced(index)"
            >&#x2699;</button>
          </div>

          <!-- Row 2: address · length? · scale · unit (small labels, tight grid) -->
          <div class="flex items-center gap-2 text-[10px] text-terminal-text-dim">
            <div class="flex items-center gap-1">
              <span>addr</span>
              <NumberInput
                :model-value="item.offset"
                :min="0"
                :max="65535"
                width="64px"
                title="Address (0-based register offset)"
                @update:model-value="update(index, 'offset', Number($event))"
              />
            </div>
            <div v-if="needsLength(item.type)" class="flex items-center gap-1">
              <span>len</span>
              <NumberInput
                :model-value="item.length"
                :min="1"
                :max="125"
                width="56px"
                title="Length (registers)"
                @update:model-value="update(index, 'length', Number($event))"
              />
            </div>
            <div class="flex items-center gap-1">
              <span>scale</span>
              <NumberInput
                :model-value="item.scale"
                :step="0.01"
                width="72px"
                title="Scale (multiplicative)"
                @update:model-value="update(index, 'scale', Number($event))"
              />
            </div>
            <div class="flex items-center gap-1 flex-1 min-w-0">
              <span>unit</span>
              <FormInput
                :model-value="item.unit"
                placeholder="(display only)"
                class="flex-1 min-w-0"
                title="Unit (display only)"
                @update:model-value="update(index, 'unit', String($event))"
              />
            </div>
          </div>

          <!-- Row 2: advanced (per-field byte/word order · bit · additive offset) -->
          <div
            v-if="advancedIdx === index"
            class="flex flex-wrap items-center gap-2 pl-1 pt-1 border-t border-terminal-border/50 text-[10px] text-terminal-text-dim"
          >
            <div class="flex items-center gap-1">
              <span>byte:</span>
              <FormSelect
                :model-value="item.byteOrder"
                :options="byteOrderOverrideOptions"
                width="160px"
                @update:model-value="update(index, 'byteOrder', String($event))"
              />
            </div>
            <div class="flex items-center gap-1">
              <span>word:</span>
              <FormSelect
                :model-value="item.wordOrder"
                :options="wordOrderOverrideOptions"
                width="180px"
                @update:model-value="update(index, 'wordOrder', String($event))"
              />
            </div>
            <div v-if="canHaveBit(item.type)" class="flex items-center gap-1">
              <span>bit:</span>
              <NumberInput
                :model-value="item.bit"
                :min="-1"
                :max="15"
                width="56px"
                title="Bit (0..15, -1 = whole register)"
                @update:model-value="update(index, 'bit', Number($event))"
              />
            </div>
            <div class="flex items-center gap-1">
              <span>offset (+):</span>
              <NumberInput
                :model-value="item.offsetValue"
                :step="0.01"
                width="80px"
                title="Additive offset, applied after scaling"
                @update:model-value="update(index, 'offsetValue', Number($event))"
              />
            </div>
          </div>
        </PropertyListItem>
      </template>
    </PropertyList>
  </FormField>
</template>
