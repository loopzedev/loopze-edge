<script setup lang="ts">
import { computed, ref } from 'vue'
import { useFlowStore } from '@/stores/flowStore'
import FormLabel from '@/components/ui/FormLabel.vue'
import FormInput from '@/components/ui/FormInput.vue'
import FormSelect from '@/components/ui/FormSelect.vue'
import IconButton from '@/components/ui/IconButton.vue'

const flowStore = useFlowStore()

const node = computed(() => flowStore.selectedNode)
const config = computed(() => (node.value?.data?.config ?? {}) as Record<string, unknown>)

interface Rule {
  t: string   // "set" | "change" | "delete" | "move"
  p: string   // property name
  pt: string  // "msg" | "flow" | "global"
  ps: string  // "memory" | "persistent" (when pt = flow/global)
  to: string
  tot: string // "msg" | "flow" | "global" | "str" | "num" | "bool" | "json" | "date" | "env"
  tos: string // "memory" | "persistent" (when tot = flow/global)
  from: string
  fromt: string // "str" | "re" | "num" | "bool" | "env"
  froms: string // "memory" | "persistent" (when fromt = flow/global)
}

const defaultRule: Rule = {
  t: 'set', p: 'payload', pt: 'msg', ps: 'memory',
  to: '', tot: 'str', tos: 'memory', from: '', fromt: 'str', froms: 'memory',
}

function isContextScope(scope: string): boolean {
  return scope === 'flow' || scope === 'global'
}

const rules = computed<Rule[]>({
  get: () => {
    const raw = config.value.rules
    if (!Array.isArray(raw)) return [{ ...defaultRule }]
    return raw.map((r: any) => ({
      t: r.t ?? 'set',
      p: r.p ?? 'payload',
      pt: r.pt ?? 'msg',
      ps: r.ps ?? 'memory',
      to: r.to ?? '',
      tot: r.tot ?? 'str',
      tos: r.tos ?? 'memory',
      from: r.from ?? '',
      fromt: r.fromt ?? 'str',
      froms: r.froms ?? 'memory',
    }))
  },
  set: (val: Rule[]) => {
    if (!node.value) return
    flowStore.updateNodeData(node.value.id, {
      config: { ...config.value, rules: val },
    })
  },
})

function updateRule(index: number, field: keyof Rule, value: string) {
  const updated = [...rules.value]
  updated[index] = { ...updated[index], [field]: value }
  rules.value = updated
}

function addRule() {
  rules.value = [...rules.value, { ...defaultRule }]
}

function removeRule(index: number) {
  const updated = [...rules.value]
  updated.splice(index, 1)
  rules.value = updated.length > 0 ? updated : [{ ...defaultRule }]
}

function moveRule(from: number, to: number) {
  if (to < 0 || to >= rules.value.length) return
  const updated = [...rules.value]
  const [moved] = updated.splice(from, 1)
  updated.splice(to, 0, moved)
  rules.value = updated
}

// ── Drag & Drop ──────────────────────────────────────────────────
const dragIdx = ref<number | null>(null)
const dropIdx = ref<number | null>(null)

function onDragStart(idx: number, e: DragEvent) {
  dragIdx.value = idx
  if (e.dataTransfer) {
    e.dataTransfer.effectAllowed = 'move'
    e.dataTransfer.setData('text/plain', String(idx))
  }
}

function onDragOver(idx: number, e: DragEvent) {
  e.preventDefault()
  if (e.dataTransfer) e.dataTransfer.dropEffect = 'move'
  dropIdx.value = idx
}

function onDrop(idx: number) {
  if (dragIdx.value !== null && dragIdx.value !== idx) {
    moveRule(dragIdx.value, idx)
  }
  dragIdx.value = null
  dropIdx.value = null
}

function onDragEnd() {
  dragIdx.value = null
  dropIdx.value = null
}

const operations = [
  { value: 'set', label: 'Setze' },
  { value: 'change', label: 'Ändere' },
  { value: 'delete', label: 'Lösche' },
  { value: 'move', label: 'Verschiebe' },
]

const scopes = [
  { value: 'msg', label: 'msg.' },
  { value: 'flow', label: 'flow.' },
  { value: 'global', label: 'global.' },
]

const valueTypes = [
  { value: 'msg', label: 'msg.' },
  { value: 'flow', label: 'flow.' },
  { value: 'global', label: 'global.' },
  { value: 'str', label: 'string' },
  { value: 'num', label: 'number' },
  { value: 'bool', label: 'boolean' },
  { value: 'json', label: 'JSON' },
  { value: 'date', label: 'timestamp' },
  { value: 'env', label: 'env' },
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

const replaceTypes = [
  { value: 'msg', label: 'msg.' },
  { value: 'flow', label: 'flow.' },
  { value: 'global', label: 'global.' },
  { value: 'str', label: 'string' },
  { value: 'num', label: 'number' },
  { value: 'bool', label: 'boolean' },
  { value: 'env', label: 'env' },
]

const storageTypes = [
  { value: 'memory', label: 'memory' },
  { value: 'persistent', label: 'persist' },
]

const timestampFormats = [
  { value: 'epoch', label: 'milliseconds since epoch' },
  { value: 'rfc3339', label: 'YYYY-MM-DDTHH:mm:ss.sssZ' },
]
</script>

<template>
  <div class="flex flex-col gap-2">
    <FormLabel>Rules</FormLabel>

    <div
      v-for="(rule, idx) in rules"
      :key="idx"
      draggable="true"
      class="border bg-terminal-bg p-2 flex flex-col gap-1.5 relative transition-all duration-100"
      :class="[
        dragIdx === idx ? 'opacity-40 border-terminal-border' : '',
        dropIdx === idx && dragIdx !== idx ? 'border-accent' : 'border-terminal-border',
      ]"
      @dragstart="onDragStart(idx, $event)"
      @dragover="onDragOver(idx, $event)"
      @drop="onDrop(idx)"
      @dragend="onDragEnd"
    >
      <!-- Row 1: Operation + Scope + Property + Delete -->
      <div class="flex items-center gap-1">
        <span class="cursor-grab active:cursor-grabbing text-terminal-text-dim hover:text-terminal-text text-[10px] mr-0.5 select-none">&#x2261;</span>

        <FormSelect :model-value="rule.t" :options="operations" width="72px"
          @update:model-value="updateRule(idx, 't', $event)" />

        <FormSelect :model-value="rule.pt" :options="scopes" width="64px"
          @update:model-value="updateRule(idx, 'pt', $event)" />

        <FormInput :model-value="rule.p" placeholder="property" mono
          @update:model-value="updateRule(idx, 'p', $event)" />

        <FormSelect v-if="isContextScope(rule.pt)" :model-value="rule.ps" :options="storageTypes" width="76px"
          @update:model-value="updateRule(idx, 'ps', $event)" />

        <IconButton variant="danger" title="Remove rule" @click="removeRule(idx)">&#x2715;</IconButton>
      </div>

      <!-- Row 2: Value (for "set") -->
      <div v-if="rule.t === 'set'" class="flex items-center gap-1 pl-5">
        <span class="text-[10px] text-terminal-text-dim shrink-0 w-14">to value</span>

        <FormSelect :model-value="rule.tot" :options="valueTypes" width="64px"
          @update:model-value="updateRule(idx, 'tot', $event)" />

        <FormInput v-if="rule.tot !== 'date'" :model-value="rule.to" mono
          :placeholder="rule.tot === 'json' ? '{...}' : rule.tot === 'bool' ? 'true / false' : 'value'"
          @update:model-value="updateRule(idx, 'to', $event)" />

        <FormSelect v-else :model-value="rule.to || 'epoch'" :options="timestampFormats"
          @update:model-value="updateRule(idx, 'to', $event)" />

        <FormSelect v-if="isContextScope(rule.tot)" :model-value="rule.tos" :options="storageTypes" width="76px"
          @update:model-value="updateRule(idx, 'tos', $event)" />
      </div>

      <!-- Row 2-3: Search + Replace (for "change") -->
      <template v-if="rule.t === 'change'">
        <div class="flex items-center gap-1 pl-5">
          <span class="text-[10px] text-terminal-text-dim shrink-0 w-14">search</span>

          <FormSelect :model-value="rule.fromt" :options="searchTypes" width="64px"
            @update:model-value="updateRule(idx, 'fromt', $event)" />

          <FormInput :model-value="rule.from" mono
            :placeholder="rule.fromt === 're' ? 'regex pattern' : 'search text'"
            @update:model-value="updateRule(idx, 'from', $event)" />

          <FormSelect v-if="isContextScope(rule.fromt)" :model-value="rule.froms" :options="storageTypes" width="76px"
            @update:model-value="updateRule(idx, 'froms', $event)" />
        </div>
        <div class="flex items-center gap-1 pl-5">
          <span class="text-[10px] text-terminal-text-dim shrink-0 w-14">replace</span>

          <FormSelect :model-value="rule.tot" :options="replaceTypes" width="64px"
            @update:model-value="updateRule(idx, 'tot', $event)" />

          <FormInput :model-value="rule.to" mono placeholder="replacement"
            @update:model-value="updateRule(idx, 'to', $event)" />

          <FormSelect v-if="isContextScope(rule.tot)" :model-value="rule.tos" :options="storageTypes" width="76px"
            @update:model-value="updateRule(idx, 'tos', $event)" />
        </div>
      </template>

      <!-- Row 2: Target (for "move") -->
      <div v-if="rule.t === 'move'" class="flex items-center gap-1 pl-5">
        <span class="text-[10px] text-terminal-text-dim shrink-0 w-14">to</span>

        <FormSelect :model-value="rule.tot" :options="scopes" width="64px"
          @update:model-value="updateRule(idx, 'tot', $event)" />

        <FormInput :model-value="rule.to" mono placeholder="target property"
          @update:model-value="updateRule(idx, 'to', $event)" />

        <FormSelect v-if="isContextScope(rule.tot)" :model-value="rule.tos" :options="storageTypes" width="76px"
          @update:model-value="updateRule(idx, 'tos', $event)" />
      </div>
    </div>

    <button
      class="text-[10px] text-terminal-text-dim hover:text-terminal-text border border-terminal-border hover:border-terminal-text px-2 py-1 transition-colors self-start"
      @click="addRule"
    >
      + add rule
    </button>
  </div>
</template>
