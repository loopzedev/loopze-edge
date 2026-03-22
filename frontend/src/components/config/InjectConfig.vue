<script setup lang="ts">
import { computed, ref } from 'vue'
import { useFlowStore } from '@/stores/flowStore'
import FormLabel from '@/components/ui/FormLabel.vue'
import FormInput from '@/components/ui/FormInput.vue'
import FormCheckbox from '@/components/ui/FormCheckbox.vue'
import ToggleGroup from '@/components/ui/ToggleGroup.vue'
import IconButton from '@/components/ui/IconButton.vue'
import ValueTypeInput from '@/components/config/ValueTypeInput.vue'

const flowStore = useFlowStore()

const node = computed(() => flowStore.selectedNode)
const config = computed(() => (node.value?.data?.config ?? {}) as Record<string, unknown>)

function update(key: string, value: unknown) {
  if (!node.value) return
  flowStore.updateNodeData(node.value.id, {
    config: { ...config.value, [key]: value },
  })
}

const once = computed({
  get: () => (config.value.once as boolean) ?? false,
  set: (v: boolean) => update('once', v),
})

const interval = computed({
  get: () => (config.value.interval as number) ?? 0,
  set: (v: number) => update('interval', v),
})

// ── Props (inject rules) ──────────────────────────────────────────

interface InjectProp {
  p: string   // property name on msg
  vt: string  // value type
  v: string   // value
  vs: string  // storage (memory/persistent)
}

const defaultProp: InjectProp = { p: 'payload', vt: 'date', v: 'rfc3339', vs: 'memory' }

const props = computed<InjectProp[]>({
  get: () => {
    const raw = config.value.props
    if (!Array.isArray(raw) || raw.length === 0) {
      return [
        { p: 'payload', vt: 'date', v: 'rfc3339', vs: 'memory' },
        { p: 'topic', vt: 'str', v: '', vs: 'memory' },
      ]
    }
    return raw.map((r: any) => ({
      p: r.p ?? 'payload',
      vt: r.vt ?? 'str',
      v: r.v ?? '',
      vs: r.vs ?? 'memory',
    }))
  },
  set: (val: InjectProp[]) => update('props', val),
})

function updateProp(index: number, field: keyof InjectProp, value: string) {
  const updated = [...props.value]
  updated[index] = { ...updated[index], [field]: value }
  props.value = updated
}

function addProp() {
  props.value = [...props.value, { ...defaultProp, p: '', vt: 'str', v: '' }]
}

function removeProp(index: number) {
  const updated = [...props.value]
  updated.splice(index, 1)
  props.value = updated.length > 0 ? updated : [{ ...defaultProp }]
}

function moveProp(from: number, to: number) {
  if (to < 0 || to >= props.value.length) return
  const updated = [...props.value]
  const [moved] = updated.splice(from, 1)
  updated.splice(to, 0, moved)
  props.value = updated
}

// ── Drag & Drop ──────────────────────────────────────────────────

const dragIdx = ref<number | null>(null)
const dropIdx = ref<number | null>(null)
const handleActive = ref(false)

function onHandleMouseDown() {
  handleActive.value = true
  const onUp = () => { handleActive.value = false; document.removeEventListener('mouseup', onUp) }
  document.addEventListener('mouseup', onUp)
}

function onDragStart(idx: number, e: DragEvent) {
  if (!handleActive.value) {
    e.preventDefault()
    return
  }
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
    moveProp(dragIdx.value, idx)
  }
  dragIdx.value = null
  dropIdx.value = null
}

function onDragEnd() {
  dragIdx.value = null
  dropIdx.value = null
}

// No 'msg' type since inject has no input message
const excludeTypes = ['msg']

const intervalPresets = [
  { label: 'None', value: 0 },
  { label: '100ms', value: 100 },
  { label: '500ms', value: 500 },
  { label: '1s', value: 1000 },
  { label: '5s', value: 5000 },
  { label: '10s', value: 10000 },
  { label: '30s', value: 30000 },
  { label: '1min', value: 60000 },
]
</script>

<template>
  <div class="flex flex-col gap-2">
    <!-- Once at startup -->
    <FormCheckbox v-model="once" label="Inject once at startup" />

    <!-- Interval -->
    <div class="flex flex-col gap-1">
      <FormLabel>Repeat Interval</FormLabel>
      <ToggleGroup v-model="interval" :options="intervalPresets" />
      <div class="flex items-center gap-1">
        <FormInput
          :model-value="String(interval)"
          type="number"
          placeholder="Custom (ms)"
          @update:model-value="interval = Number($event)"
        />
        <span class="text-[10px] text-terminal-text-dim shrink-0">ms</span>
      </div>
    </div>

    <!-- Properties -->
    <FormLabel>Properties</FormLabel>

    <div
      v-for="(prop, idx) in props"
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
      <!-- Row 1: Drag handle + msg.property + delete -->
      <div class="flex items-center gap-1.5">
        <span
          class="cursor-grab active:cursor-grabbing text-terminal-text-dim hover:text-terminal-text text-[10px] select-none shrink-0"
          @mousedown="onHandleMouseDown"
        >&#x2261;</span>

        <span class="text-[10px] text-terminal-text-dim shrink-0">msg.</span>
        <FormInput
          :model-value="prop.p"
          placeholder="property"
          mono
          class="flex-1 min-w-0"
          @update:model-value="updateProp(idx, 'p', $event)"
        />

        <IconButton variant="danger" title="Remove property" @click="removeProp(idx)">&#x2715;</IconButton>
      </div>

      <!-- Row 2: Value type + value + storage -->
      <div class="pl-4">
        <ValueTypeInput
          :value="prop.v"
          :type="prop.vt"
          :storage="prop.vs"
          :exclude-types="excludeTypes"
          @update:value="updateProp(idx, 'v', $event)"
          @update:type="updateProp(idx, 'vt', $event)"
          @update:storage="updateProp(idx, 'vs', $event)"
        />
      </div>
    </div>

    <button
      class="text-[10px] text-terminal-text-dim hover:text-terminal-text border border-terminal-border hover:border-terminal-text px-2 py-1 transition-colors self-start"
      @click="addProp"
    >
      + add property
    </button>
  </div>
</template>
