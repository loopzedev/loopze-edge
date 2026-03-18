<script setup lang="ts">
import { computed, ref } from 'vue'
import { useFlowStore } from '@/stores/flowStore'

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
</script>

<template>
  <div class="flex flex-col gap-2">
    <label class="text-[10px] text-terminal-text-dim uppercase tracking-wider">Rules</label>

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
        <!-- Drag handle -->
        <span class="cursor-grab active:cursor-grabbing text-terminal-text-dim hover:text-terminal-text text-[10px] mr-0.5 select-none">≡</span>

        <!-- Operation -->
        <select
          :value="rule.t"
          class="terminal-input text-[10px] w-[72px] shrink-0"
          @change="updateRule(idx, 't', ($event.target as HTMLSelectElement).value)"
        >
          <option v-for="op in operations" :key="op.value" :value="op.value">{{ op.label }}</option>
        </select>

        <!-- Scope -->
        <select
          :value="rule.pt"
          class="terminal-input text-[10px] w-[64px] shrink-0"
          @change="updateRule(idx, 'pt', ($event.target as HTMLSelectElement).value)"
        >
          <option v-for="s in scopes" :key="s.value" :value="s.value">{{ s.label }}</option>
        </select>

        <!-- Property -->
        <input
          :value="rule.p"
          type="text"
          class="terminal-input text-[10px] flex-1 min-w-0"
          placeholder="property"
          @change="updateRule(idx, 'p', ($event.target as HTMLInputElement).value)"
        />

        <!-- Storage selector (only for flow/global scope) -->
        <select
          v-if="isContextScope(rule.pt)"
          :value="rule.ps"
          class="terminal-input text-[10px] w-[76px] shrink-0"
          @change="updateRule(idx, 'ps', ($event.target as HTMLSelectElement).value)"
        >
          <option value="memory">memory</option>
          <option value="persistent">persist</option>
        </select>

        <!-- Delete rule -->
        <button
          class="text-terminal-text-dim hover:text-red-400 text-xs shrink-0 w-5 h-5 flex items-center justify-center"
          title="Remove rule"
          @click="removeRule(idx)"
        >✕</button>
      </div>

      <!-- Row 2: Value (for "set") -->
      <div v-if="rule.t === 'set'" class="flex items-center gap-1 pl-5">
        <span class="text-[9px] text-terminal-text-dim shrink-0 w-14">to value</span>
        <select
          :value="rule.tot"
          class="terminal-input text-[10px] w-[64px] shrink-0"
          @change="updateRule(idx, 'tot', ($event.target as HTMLSelectElement).value)"
        >
          <option v-for="vt in valueTypes" :key="vt.value" :value="vt.value">{{ vt.label }}</option>
        </select>
        <input
          v-if="rule.tot !== 'date'"
          :value="rule.to"
          type="text"
          class="terminal-input text-[10px] flex-1 min-w-0 font-mono"
          :placeholder="rule.tot === 'json' ? '{...}' : rule.tot === 'bool' ? 'true / false' : 'value'"
          @change="updateRule(idx, 'to', ($event.target as HTMLInputElement).value)"
        />
        <select
          v-else
          :value="rule.to || 'epoch'"
          class="terminal-input text-[10px] flex-1 min-w-0"
          @change="updateRule(idx, 'to', ($event.target as HTMLSelectElement).value)"
        >
          <option value="epoch">milliseconds since epoch</option>
          <option value="rfc3339">YYYY-MM-DDTHH:mm:ss.sssZ</option>
        </select>
        <select
          v-if="isContextScope(rule.tot)"
          :value="rule.tos"
          class="terminal-input text-[10px] w-[76px] shrink-0"
          @change="updateRule(idx, 'tos', ($event.target as HTMLSelectElement).value)"
        >
          <option value="memory">memory</option>
          <option value="persistent">persist</option>
        </select>
      </div>

      <!-- Row 2-3: Search + Replace (for "change") -->
      <template v-if="rule.t === 'change'">
        <div class="flex items-center gap-1 pl-5">
          <span class="text-[9px] text-terminal-text-dim shrink-0 w-14">search</span>
          <select
            :value="rule.fromt"
            class="terminal-input text-[10px] w-[64px] shrink-0"
            @change="updateRule(idx, 'fromt', ($event.target as HTMLSelectElement).value)"
          >
            <option v-for="st in searchTypes" :key="st.value" :value="st.value">{{ st.label }}</option>
          </select>
          <input
            :value="rule.from"
            type="text"
            class="terminal-input text-[10px] flex-1 min-w-0 font-mono"
            :placeholder="rule.fromt === 're' ? 'regex pattern' : 'search text'"
            @change="updateRule(idx, 'from', ($event.target as HTMLInputElement).value)"
          />
          <select
            v-if="isContextScope(rule.fromt)"
            :value="rule.froms"
            class="terminal-input text-[10px] w-[76px] shrink-0"
            @change="updateRule(idx, 'froms', ($event.target as HTMLSelectElement).value)"
          >
            <option value="memory">memory</option>
            <option value="persistent">persist</option>
          </select>
        </div>
        <div class="flex items-center gap-1 pl-5">
          <span class="text-[9px] text-terminal-text-dim shrink-0 w-14">replace</span>
          <select
            :value="rule.tot"
            class="terminal-input text-[10px] w-[64px] shrink-0"
            @change="updateRule(idx, 'tot', ($event.target as HTMLSelectElement).value)"
          >
            <option v-for="rt in replaceTypes" :key="rt.value" :value="rt.value">{{ rt.label }}</option>
          </select>
          <input
            :value="rule.to"
            type="text"
            class="terminal-input text-[10px] flex-1 min-w-0 font-mono"
            placeholder="replacement"
            @change="updateRule(idx, 'to', ($event.target as HTMLInputElement).value)"
          />
          <select
            v-if="isContextScope(rule.tot)"
            :value="rule.tos"
            class="terminal-input text-[10px] w-[76px] shrink-0"
            @change="updateRule(idx, 'tos', ($event.target as HTMLSelectElement).value)"
          >
            <option value="memory">memory</option>
            <option value="persistent">persist</option>
          </select>
        </div>
      </template>

      <!-- Row 2: Target (for "move") -->
      <div v-if="rule.t === 'move'" class="flex items-center gap-1 pl-5">
        <span class="text-[9px] text-terminal-text-dim shrink-0 w-14">to</span>
        <select
          :value="rule.tot"
          class="terminal-input text-[10px] w-[64px] shrink-0"
          @change="updateRule(idx, 'tot', ($event.target as HTMLSelectElement).value)"
        >
          <option v-for="s in scopes" :key="s.value" :value="s.value">{{ s.label }}</option>
        </select>
        <input
          :value="rule.to"
          type="text"
          class="terminal-input text-[10px] flex-1 min-w-0 font-mono"
          placeholder="target property"
          @change="updateRule(idx, 'to', ($event.target as HTMLInputElement).value)"
        />
        <select
          v-if="isContextScope(rule.tot)"
          :value="rule.tos"
          class="terminal-input text-[10px] w-[76px] shrink-0"
          @change="updateRule(idx, 'tos', ($event.target as HTMLSelectElement).value)"
        >
          <option value="memory">memory</option>
          <option value="persistent">persist</option>
        </select>
      </div>

      <!-- "delete" has no extra rows -->
    </div>

    <!-- Add rule button -->
    <button
      class="text-[10px] text-terminal-text-dim hover:text-terminal-text border border-terminal-border hover:border-terminal-text px-2 py-1 transition-colors self-start"
      @click="addRule"
    >
      + add rule
    </button>
  </div>
</template>
