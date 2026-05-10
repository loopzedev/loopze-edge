<script setup lang="ts">
import { computed, ref } from 'vue'
import FormInput from '@/components/ui/FormInput.vue'
import FormSelect from '@/components/ui/FormSelect.vue'
import FormField from '@/components/ui/FormField.vue'
import FormCheckbox from '@/components/ui/FormCheckbox.vue'
import NumberInput from '@/components/ui/NumberInput.vue'
import PropertyList from '@/components/ui/PropertyList.vue'
import PropertyListItem from '@/components/ui/PropertyListItem.vue'
import { useNodeProperty } from '@/composables/useNodeProperty'
import { S7_DATA_TYPES } from './enums'

// Layout entry shape mirrors the on-wire JSON in workspace.json (see
// PARSER_S7_NODE.md). Offset is normalised to a string in the UI even when
// the user types a plain integer — the backend's parseS7Offset accepts both
// shapes, but a string round-trips cleanly through Vue's reactive store
// without precision loss for the dotted "byte.bit" form. We convert back to
// a number on save when the value is integral.
interface LayoutField {
  id: string
  offset: string  // "12" or "12.3" — see comment above
  name: string
  type: string
  length: number
  signed: boolean
  scale: number
  valueOffset: number
  unit: string
}

function defaultField(): LayoutField {
  return {
    id: crypto.randomUUID(),
    offset: '0',
    name: '',
    type: 'real',
    length: 1,
    signed: false,
    scale: 1,
    valueOffset: 0,
    unit: '',
  }
}

const rawLayout = useNodeProperty<unknown[]>('layout', [])

// Read defensively — fill in IDs and defaults so the drag-key is stable and
// the table never shows undefined inputs. Coerce both int and string offset
// forms into the string representation we use internally.
const layout = computed<LayoutField[]>({
  get: () =>
    (rawLayout.value ?? []).map((raw) => {
      const f = (raw ?? {}) as Record<string, unknown>
      return {
        id: (f.id as string) || crypto.randomUUID(),
        offset: stringifyOffset(f.offset),
        name: String(f.name ?? ''),
        type: String(f.type ?? 'real'),
        length: Number(f.length ?? 1),
        signed: Boolean(f.signed ?? false),
        scale: Number(f.scale ?? 1),
        valueOffset: Number(f.valueOffset ?? 0),
        unit: String(f.unit ?? ''),
      }
    }),
  set: (val) => {
    rawLayout.value = val.map(normaliseForStore)
  },
})

function stringifyOffset(o: unknown): string {
  if (typeof o === 'number') return String(o)
  if (typeof o === 'string') return o
  return '0'
}

// Strip UI-only fields and convert the offset back to a number when
// it's a plain integer (e.g. "12" → 12). Dotted forms ("12.3") stay as
// strings — the backend parses them.
function normaliseForStore(f: LayoutField): Record<string, unknown> {
  const out: Record<string, unknown> = {
    id: f.id,
    offset: looksLikeInt(f.offset) ? Number(f.offset) : f.offset,
    name: f.name,
    type: f.type,
  }
  if (needsLength(f.type)) out.length = f.length
  if (canBeSigned(f.type) && f.signed) out.signed = true
  if (f.scale !== 1) out.scale = f.scale
  if (f.valueOffset !== 0) out.valueOffset = f.valueOffset
  if (f.unit) out.unit = f.unit
  return out
}

function looksLikeInt(s: string): boolean {
  return /^\d+$/.test(s)
}

function update<K extends keyof LayoutField>(idx: number, key: K, value: LayoutField[K]) {
  const next = [...layout.value]
  next[idx] = { ...next[idx], [key]: value }
  layout.value = next
}

const advancedIdx = ref<number | null>(null)
function toggleAdvanced(idx: number) {
  advancedIdx.value = advancedIdx.value === idx ? null : idx
}

// Live duplicate-name detection — the backend rejects duplicates at deploy
// time, so flag them red here as well.
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

// Per-type UI gates — keep the row compact by hiding fields that don't
// apply to the chosen S7 type.
function needsLength(type: string): boolean {
  return type === 'string' || type === 'raw'
}

function canBeSigned(type: string): boolean {
  // Inherently-signed types (int/dint) reject the flag at the backend; we
  // also hide it here. The flag is only meaningful for byte/word/dword.
  return type === 'byte' || type === 'word' || type === 'dword'
}

function isBoolType(type: string): boolean {
  return type === 'bool'
}

// Live offset validation: BOOL needs the dotted form (with bit ∈ [0, 7]);
// other types need an integer ≥ 0.
function validateOffset(f: LayoutField): { ok: boolean; msg?: string } {
  const s = (f.offset ?? '').trim()
  if (!s) return { ok: false, msg: 'offset required' }
  if (isBoolType(f.type)) {
    const m = s.match(/^(\d+)\.([0-7])$/)
    if (!m) return { ok: false, msg: 'BOOL offset: byte.bit (e.g. 12.3) — bit ∈ [0, 7]' }
    return { ok: true }
  }
  if (!/^\d+$/.test(s)) return { ok: false, msg: 'integer byte offset (no dot)' }
  return { ok: true }
}
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
          :is-invalid="!item.name || isDuplicateName(item.name) || !validateOffset(item).ok"
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
              :options="S7_DATA_TYPES"
              width="220px"
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

          <!-- Row 2: offset (text) · length? · scale · unit -->
          <div class="flex items-center gap-2 text-[10px] text-terminal-text-dim">
            <div class="flex items-center gap-1">
              <span>off</span>
              <FormInput
                :model-value="item.offset"
                :placeholder="isBoolType(item.type) ? '12.3' : '12'"
                mono
                class="w-20"
                :invalid="!validateOffset(item).ok"
                :title="isBoolType(item.type) ? 'BOOL offset: byte.bit (e.g. 12.3)' : 'Integer byte offset'"
                @update:model-value="update(index, 'offset', String($event))"
              />
            </div>
            <div v-if="needsLength(item.type)" class="flex items-center gap-1">
              <span>len</span>
              <NumberInput
                :model-value="item.length"
                :min="1"
                :max="item.type === 'string' ? 254 : 65535"
                width="56px"
                :title="item.type === 'string' ? 'STRING max length (1..254)' : 'Raw byte count'"
                @update:model-value="update(index, 'length', Number($event))"
              />
            </div>
            <div
              v-if="!isBoolType(item.type) && item.type !== 'string' && item.type !== 'raw'"
              class="flex items-center gap-1"
            >
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

          <!-- Per-row error hint -->
          <div
            v-if="!validateOffset(item).ok && item.offset"
            class="text-[10px] text-red-400/80 leading-tight pl-1"
          >
            {{ validateOffset(item).msg }}
          </div>

          <!-- Advanced row: signed (byte/word/dword only) · valueOffset -->
          <div
            v-if="advancedIdx === index"
            class="flex flex-wrap items-center gap-2 pl-1 pt-1 border-t border-terminal-border/50 text-[10px] text-terminal-text-dim"
          >
            <FormCheckbox
              v-if="canBeSigned(item.type)"
              :model-value="item.signed"
              :label="`signed (treat ${item.type.toUpperCase()} as int${item.type === 'byte' ? '8' : item.type === 'word' ? '16' : '32'})`"
              @update:model-value="update(index, 'signed', Boolean($event))"
            />
            <div
              v-if="!isBoolType(item.type) && item.type !== 'string' && item.type !== 'raw'"
              class="flex items-center gap-1"
            >
              <span>offset (+):</span>
              <NumberInput
                :model-value="item.valueOffset"
                :step="0.01"
                width="80px"
                title="Additive offset, applied after scaling"
                @update:model-value="update(index, 'valueOffset', Number($event))"
              />
            </div>
          </div>
        </PropertyListItem>
      </template>
    </PropertyList>
  </FormField>
</template>
