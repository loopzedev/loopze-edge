<script setup lang="ts">
import FormInput from '@/components/ui/FormInput.vue'
import FormSelect from '@/components/ui/FormSelect.vue'
import FormField from '@/components/ui/FormField.vue'
import PropertyList from '@/components/ui/PropertyList.vue'
import PropertyListItem from '@/components/ui/PropertyListItem.vue'
import ValueTypeInput from '@/components/config/ValueTypeInput.vue'
import { useNodeProperty } from '@/composables/useNodeProperty'
import { STORAGE_TYPES, isContextScope } from '@/components/config/enums'
import { computed } from 'vue'

interface Rule {
  t: string   // "set" | "change" | "delete" | "move"
  p: string
  pt: string
  ps: string
  to: string
  tot: string
  tos: string
  from: string
  fromt: string
  froms: string
}

const defaultRule: Rule = {
  t: 'set', p: 'payload', pt: 'msg', ps: 'memory',
  to: '', tot: 'str', tos: 'memory',
  from: '', fromt: 'str', froms: 'memory',
}

const rawRules = useNodeProperty<Rule[]>('rules', [{ ...defaultRule }])

const rules = computed<Rule[]>({
  get: () => (rawRules.value ?? []).map((r: any) => ({
    t: r?.t ?? 'set',
    p: r?.p ?? 'payload',
    pt: r?.pt ?? 'msg',
    ps: r?.ps ?? 'memory',
    to: r?.to ?? '',
    tot: r?.tot ?? 'str',
    tos: r?.tos ?? 'memory',
    from: r?.from ?? '',
    fromt: r?.fromt ?? 'str',
    froms: r?.froms ?? 'memory',
  })),
  set: (val) => { rawRules.value = val },
})

function newRule(): Rule {
  return { ...defaultRule }
}

function updateRule(idx: number, field: keyof Rule, value: string) {
  const updated = [...rules.value]
  updated[idx] = { ...updated[idx], [field]: value }
  rules.value = updated
}

const operations = [
  { value: 'set', label: 'Set' },
  { value: 'change', label: 'Change' },
  { value: 'delete', label: 'Delete' },
  { value: 'move', label: 'Move' },
]

const scopes = [
  { value: 'msg', label: 'msg.' },
  { value: 'flow', label: 'flow.' },
  { value: 'global', label: 'global.' },
]

const searchTypes = [
  { value: 'msg', label: 'msg.' },
  { value: 'flow', label: 'flow.' },
  { value: 'global', label: 'global.' },
  { value: 'str', label: 'string' },
  { value: 're', label: 'regex' },
  { value: 'num', label: 'number' },
  { value: 'bool', label: 'boolean' },
  { value: 'env', label: 'env' },
]
</script>

<template>
  <FormField :label="`Rules · ${rules.length}`">
    <PropertyList
      v-model="rules"
      :new-item="newRule"
      add-label="+ add rule"
    >
      <template #default="{ item, index, isDragging, isDropTarget, remove, handleMousedown }">
        <PropertyListItem
          :is-dragging="isDragging"
          :is-drop-target="isDropTarget"
          :is-invalid="!item.p"
          @remove="remove"
          @handle-mousedown="handleMousedown"
        >
          <!-- Row 1: operation + scope + property -->
          <div class="flex items-center gap-1.5">
            <FormSelect
              :model-value="item.t"
              :options="operations"
              width="84px"
              @update:model-value="updateRule(index, 't', String($event))"
            />
            <FormSelect
              :model-value="item.pt"
              :options="scopes"
              width="72px"
              @update:model-value="updateRule(index, 'pt', String($event))"
            />
            <FormInput
              :model-value="item.p"
              placeholder="property"
              mono
              class="flex-1 min-w-0"
              :invalid="!item.p"
              @update:model-value="updateRule(index, 'p', $event)"
            />
            <FormSelect
              v-if="isContextScope(item.pt)"
              :model-value="item.ps"
              :options="STORAGE_TYPES"
              width="80px"
              @update:model-value="updateRule(index, 'ps', String($event))"
            />
          </div>
          <div v-if="!item.p" class="text-[10px] text-status-error leading-tight">
            Property name required
          </div>

          <!-- Row 2: value (for "set") -->
          <div v-if="item.t === 'set'" class="pl-3">
            <ValueTypeInput
              :value="item.to"
              :type="item.tot"
              :storage="item.tos"
              label="to value"
              @update:value="updateRule(index, 'to', $event)"
              @update:type="updateRule(index, 'tot', $event)"
              @update:storage="updateRule(index, 'tos', $event)"
            />
          </div>

          <!-- Row 2-3: search + replace (for "change") -->
          <template v-if="item.t === 'change'">
            <div class="flex items-center gap-1.5 pl-3">
              <span class="text-[10px] text-terminal-text-dim shrink-0 w-[52px] text-right">search</span>
              <FormSelect
                :model-value="item.fromt"
                :options="searchTypes"
                width="80px"
                @update:model-value="updateRule(index, 'fromt', String($event))"
              />
              <FormInput
                :model-value="item.from"
                mono
                class="flex-1 min-w-0"
                :placeholder="item.fromt === 're' ? 'regex pattern' : 'search text'"
                @update:model-value="updateRule(index, 'from', $event)"
              />
              <FormSelect
                v-if="isContextScope(item.fromt)"
                :model-value="item.froms"
                :options="STORAGE_TYPES"
                width="80px"
                @update:model-value="updateRule(index, 'froms', String($event))"
              />
            </div>
            <div class="pl-3">
              <ValueTypeInput
                :value="item.to"
                :type="item.tot"
                :storage="item.tos"
                label="replace"
                @update:value="updateRule(index, 'to', $event)"
                @update:type="updateRule(index, 'tot', $event)"
                @update:storage="updateRule(index, 'tos', $event)"
              />
            </div>
          </template>

          <!-- Row 2: target (for "move") -->
          <div v-if="item.t === 'move'" class="flex items-center gap-1.5 pl-3">
            <span class="text-[10px] text-terminal-text-dim shrink-0 w-[52px] text-right">to</span>
            <FormSelect
              :model-value="item.tot"
              :options="scopes"
              width="80px"
              @update:model-value="updateRule(index, 'tot', String($event))"
            />
            <FormInput
              :model-value="item.to"
              mono
              class="flex-1 min-w-0"
              placeholder="target property"
              @update:model-value="updateRule(index, 'to', $event)"
            />
            <FormSelect
              v-if="isContextScope(item.tot)"
              :model-value="item.tos"
              :options="STORAGE_TYPES"
              width="80px"
              @update:model-value="updateRule(index, 'tos', String($event))"
            />
          </div>
        </PropertyListItem>
      </template>
    </PropertyList>
  </FormField>
</template>
