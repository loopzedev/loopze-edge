<script setup lang="ts">
import { computed, nextTick } from 'vue'
import { useVueFlow } from '@vue-flow/core'
import { useFlowStore } from '@/stores/flowStore'
import { useNodeProperty } from '@/composables/useNodeProperty'
import FormField from '@/components/ui/FormField.vue'
import FormInput from '@/components/ui/FormInput.vue'
import FormSelect from '@/components/ui/FormSelect.vue'
import PropertyList from '@/components/ui/PropertyList.vue'
import PropertyListItem from '@/components/ui/PropertyListItem.vue'
import ValueTypeInput from '@/components/config/ValueTypeInput.vue'
import { STORAGE_TYPES, isContextScope } from '@/components/config/enums'

interface Rule {
  id: string
  t: string
  v: string
  vt: string
  vs: string
  v2: string
  v2t: string
  v2s: string
  case: boolean
}

const flowStore = useFlowStore()
const { updateNodeInternals } = useVueFlow('loopze-flow-editor')

const property = useNodeProperty<string>('property', 'payload')
const propertyType = useNodeProperty<string>('propertyType', 'msg')
const propertyStorage = useNodeProperty<string>('propertyStorage', 'memory')
const checkall = useNodeProperty<boolean>('checkall', false)

const propertyScopes = [
  { value: 'msg', label: 'msg.' },
  { value: 'flow', label: 'flow.' },
  { value: 'global', label: 'global.' },
]

// Operators grouped semantically; rendered flat in a single dropdown.
const operators = [
  { value: 'eq',      label: '==' },
  { value: 'neq',     label: '!=' },
  { value: 'lt',      label: '<' },
  { value: 'lte',     label: '<=' },
  { value: 'gt',      label: '>' },
  { value: 'gte',     label: '>=' },
  { value: 'btwn',    label: 'is between' },
  { value: 'cont',    label: 'contains' },
  { value: 'regex',   label: 'matches regex' },
  { value: 'true',    label: 'is true' },
  { value: 'false',   label: 'is false' },
  { value: 'null',    label: 'is null' },
  { value: 'nnull',   label: 'is not null' },
  { value: 'empty',   label: 'is empty' },
  { value: 'nempty',  label: 'is not empty' },
  { value: 'istype',  label: 'is of type' },
  { value: 'else',    label: 'otherwise' },
]

const istypeOptions = [
  { value: 'string',  label: 'string' },
  { value: 'number',  label: 'number' },
  { value: 'boolean', label: 'boolean' },
  { value: 'array',   label: 'array' },
  { value: 'object',  label: 'object' },
  { value: 'null',    label: 'null' },
]

const modeOptions = [
  { value: 'first', label: 'stop after first match' },
  { value: 'all',   label: 'check all rules' },
]

const rawRules = useNodeProperty<Rule[]>('rules', [])

// Read rules from the store, ensuring every entry has a stable id and the
// expected shape. Missing ids are filled in (handles imports / hand-edits).
const rules = computed<Rule[]>(() => {
  const arr = rawRules.value ?? []
  return arr.map((r: any, i: number) => ({
    id: r?.id || `r-${Date.now()}-${i}`,
    t: r?.t ?? 'eq',
    v: r?.v ?? '',
    vt: r?.vt ?? 'str',
    vs: r?.vs ?? 'memory',
    v2: r?.v2 ?? '',
    v2t: r?.v2t ?? 'str',
    v2s: r?.v2s ?? 'memory',
    case: !!r?.case,
  }))
})

function newRule(): Rule {
  return {
    id: crypto.randomUUID(),
    t: 'eq', v: '', vt: 'str', vs: 'memory',
    v2: '', v2t: 'str', v2s: 'memory',
    case: false,
  }
}

// Persist rules to the store, simultaneously mirroring the rule count onto
// data.outputs and remapping any output edges based on rule-id continuity.
// updateNodeData performs index-based edge cleanup; remap runs first so
// reordered edges are at the correct new index by the time cleanup hits.
function setRules(newRules: Rule[]) {
  const node = flowStore.selectedNode
  if (!node) return

  const oldRules = rules.value
  const mapping: Record<number, number | null> = {}
  oldRules.forEach((r, oldIdx) => {
    const newIdx = newRules.findIndex((nr) => nr.id === r.id)
    if (newIdx !== oldIdx) {
      mapping[oldIdx] = newIdx === -1 ? null : newIdx
    }
  })

  if (Object.keys(mapping).length > 0) {
    flowStore.remapOutputEdges(node.id, mapping)
  }

  const cfg = (node.data?.config ?? {}) as Record<string, unknown>
  flowStore.updateNodeData(node.id, {
    config: { ...cfg, rules: newRules },
    outputs: newRules.length,
  })

  nextTick(() => updateNodeInternals([node.id]))
}

function updateRule(idx: number, patch: Partial<Rule>) {
  if (idx < 0 || idx >= rules.value.length) return
  const updated = [...rules.value]
  const merged = { ...updated[idx], ...patch }
  // When switching to "is of type" without an existing type-name value,
  // seed it with "string" so the dropdown's displayed default is also persisted.
  if (patch.t === 'istype' && !merged.v) {
    merged.v = 'string'
  }
  updated[idx] = merged
  setRules(updated)
}

// What does a rule's right-hand side look like?
function needsValue(t: string): boolean {
  return ['eq', 'neq', 'lt', 'lte', 'gt', 'gte', 'cont', 'regex', 'btwn'].includes(t)
}
function needsSecondValue(t: string): boolean {
  return t === 'btwn'
}
function hasCaseToggle(t: string): boolean {
  return t === 'regex' || t === 'cont'
}
function isIstype(t: string): boolean {
  return t === 'istype'
}
</script>

<template>
  <div class="flex flex-col gap-3">
    <FormField label="Property">
      <div class="flex items-center gap-1.5">
        <FormSelect
          v-model="propertyType"
          :options="propertyScopes"
          width="72px"
        />
        <FormInput
          v-model="property"
          placeholder="property path"
          mono
          class="flex-1 min-w-0"
          :invalid="!property"
        />
        <FormSelect
          v-if="isContextScope(propertyType)"
          v-model="propertyStorage"
          :options="STORAGE_TYPES"
          width="80px"
        />
      </div>
    </FormField>

    <FormField :label="`Rules · ${rules.length}`">
      <PropertyList
        :model-value="rules"
        :item-key="(r) => r.id"
        :new-item="newRule"
        add-label="+ add rule"
        @update:model-value="setRules"
      >
        <template #default="{ item, index, isDragging, isDropTarget, remove, handleMousedown }">
          <PropertyListItem
            :is-dragging="isDragging"
            :is-drop-target="isDropTarget"
            @remove="remove"
            @handle-mousedown="handleMousedown"
          >
            <!-- Row 1: operator + (optional) value -->
            <div class="flex items-center gap-1.5">
              <FormSelect
                :model-value="item.t"
                :options="operators"
                width="120px"
                @update:model-value="updateRule(index, { t: String($event) })"
              />

              <!-- istype: type-name dropdown -->
              <FormSelect
                v-if="isIstype(item.t)"
                :model-value="item.v || 'string'"
                :options="istypeOptions"
                class="flex-1 min-w-0"
                @update:model-value="updateRule(index, { v: String($event) })"
              />

              <!-- compare/contains/regex/btwn (first value): typed value input -->
              <ValueTypeInput
                v-else-if="needsValue(item.t)"
                :value="item.v"
                :type="item.vt"
                :storage="item.vs"
                class="flex-1 min-w-0"
                @update:value="updateRule(index, { v: $event })"
                @update:type="updateRule(index, { vt: $event })"
                @update:storage="updateRule(index, { vs: $event })"
              />

              <!-- value-less operators: spacer keeps column alignment -->
              <span v-else class="flex-1" />

              <span class="text-[10px] text-terminal-text-dim shrink-0">
                → {{ index + 1 }}
              </span>
            </div>

            <!-- Row 2: between's upper bound -->
            <div v-if="needsSecondValue(item.t)" class="flex items-center gap-1.5 pl-3">
              <span class="text-[10px] text-terminal-text-dim shrink-0 w-[52px] text-right">and</span>
              <ValueTypeInput
                :value="item.v2"
                :type="item.v2t"
                :storage="item.v2s"
                class="flex-1 min-w-0"
                @update:value="updateRule(index, { v2: $event })"
                @update:type="updateRule(index, { v2t: $event })"
                @update:storage="updateRule(index, { v2s: $event })"
              />
            </div>

            <!-- Row 2 (alt): case-sensitivity toggle for regex/contains -->
            <label
              v-if="hasCaseToggle(item.t)"
              class="flex items-center gap-1.5 pl-3 text-[10px] text-terminal-text-dim cursor-pointer"
            >
              <input
                type="checkbox"
                :checked="item.case"
                class="accent-accent"
                @change="updateRule(index, { case: ($event.target as HTMLInputElement).checked })"
              />
              case-sensitive
            </label>
          </PropertyListItem>
        </template>
      </PropertyList>
    </FormField>

    <FormField label="Mode">
      <FormSelect
        :model-value="checkall ? 'all' : 'first'"
        :options="modeOptions"
        @update:model-value="checkall = $event === 'all'"
      />
    </FormField>
  </div>
</template>
