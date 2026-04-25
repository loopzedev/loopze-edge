<script setup lang="ts" generic="T">
import { ref } from 'vue'

const props = defineProps<{
  modelValue: T[]
  /** Function to derive a stable key per item; falls back to index. */
  itemKey?: (item: T, index: number) => string | number
  /** Add-button label. Hide button by leaving this empty. */
  addLabel?: string
  /** Provides a new item when the user clicks "add". */
  newItem?: () => T
}>()

const emit = defineEmits<{
  'update:modelValue': [items: T[]]
}>()

// ── Drag-drop state ────────────────────────────────────────────────

const dragIdx = ref<number | null>(null)
const dropIdx = ref<number | null>(null)
const handleActive = ref(false)

function getKey(item: T, idx: number): string | number {
  return props.itemKey ? props.itemKey(item, idx) : idx
}

function move(from: number, to: number) {
  if (to < 0 || to >= props.modelValue.length) return
  const next = [...props.modelValue]
  const [moved] = next.splice(from, 1)
  next.splice(to, 0, moved)
  emit('update:modelValue', next)
}

function remove(idx: number) {
  const next = [...props.modelValue]
  next.splice(idx, 1)
  emit('update:modelValue', next)
}

function add() {
  if (!props.newItem) return
  emit('update:modelValue', [...props.modelValue, props.newItem()])
}

function onHandleMouseDown() {
  handleActive.value = true
  const onUp = () => {
    handleActive.value = false
    document.removeEventListener('mouseup', onUp)
  }
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
  if (dragIdx.value !== null && dragIdx.value !== idx) move(dragIdx.value, idx)
  dragIdx.value = null
  dropIdx.value = null
}

function onDragEnd() {
  dragIdx.value = null
  dropIdx.value = null
}
</script>

<template>
  <div class="flex flex-col gap-1.5">
    <div
      v-for="(item, idx) in modelValue"
      :key="getKey(item, idx)"
      draggable="true"
      @dragstart="onDragStart(idx, $event)"
      @dragover="onDragOver(idx, $event)"
      @drop="onDrop(idx)"
      @dragend="onDragEnd"
    >
      <slot
        :item="item"
        :index="idx"
        :is-dragging="dragIdx === idx"
        :is-drop-target="dropIdx === idx"
        :remove="() => remove(idx)"
        :handle-mousedown="onHandleMouseDown"
      />
    </div>

    <button
      v-if="addLabel && newItem"
      type="button"
      class="w-full text-[10px] text-terminal-text-dim border border-dashed border-terminal-border hover:border-accent/50 hover:text-accent py-2 transition-colors"
      @click="add"
    >
      {{ addLabel }}
    </button>
  </div>
</template>
