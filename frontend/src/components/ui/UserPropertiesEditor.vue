<script setup lang="ts">
import { ref, watch } from 'vue'
import FormInput from './FormInput.vue'
import IconButton from './IconButton.vue'

interface Row { key: string; value: string }

const props = withDefaults(defineProps<{
  modelValue: Record<string, string>
  keyPlaceholder?: string
  valuePlaceholder?: string
}>(), {
  keyPlaceholder: 'key',
  valuePlaceholder: 'value',
})

const emit = defineEmits<{
  'update:modelValue': [value: Record<string, string>]
}>()

const rows = ref<Row[]>([])

// Sync rows from the prop only when the prop content actually differs from
// what the rows currently produce — without this guard every emit would round-
// trip through the parent and reset the rows mid-typing.
watch(() => props.modelValue, (val) => {
  if (sameMap(rowsToMap(rows.value), val)) return
  rows.value = Object.entries(val ?? {}).map(([key, value]) => ({ key, value }))
}, { immediate: true, deep: true })

function rowsToMap(list: Row[]): Record<string, string> {
  const out: Record<string, string> = {}
  for (const r of list) {
    if (r.key) out[r.key] = r.value
  }
  return out
}

function sameMap(a: Record<string, string>, b: Record<string, string>): boolean {
  const ak = Object.keys(a)
  const bk = Object.keys(b ?? {})
  if (ak.length !== bk.length) return false
  for (const k of ak) {
    if (a[k] !== (b ?? {})[k]) return false
  }
  return true
}

function emitChange() {
  emit('update:modelValue', rowsToMap(rows.value))
}

function setKey(idx: number, v: string) {
  rows.value[idx].key = v
  emitChange()
}

function setValue(idx: number, v: string) {
  rows.value[idx].value = v
  emitChange()
}

function addRow() {
  rows.value.push({ key: '', value: '' })
}

function removeRow(idx: number) {
  rows.value.splice(idx, 1)
  emitChange()
}
</script>

<template>
  <div class="flex flex-col gap-1">
    <div
      v-for="(row, idx) in rows"
      :key="idx"
      class="flex items-stretch gap-1"
    >
      <div class="flex-1 min-w-0">
        <FormInput
          :model-value="row.key"
          :placeholder="keyPlaceholder"
          mono
          @update:model-value="(v: string) => setKey(idx, v)"
        />
      </div>
      <div class="flex-1 min-w-0">
        <FormInput
          :model-value="row.value"
          :placeholder="valuePlaceholder"
          @update:model-value="(v: string) => setValue(idx, v)"
        />
      </div>
      <IconButton variant="danger" title="Remove" @click="removeRow(idx)">
        <span class="text-base leading-none">×</span>
      </IconButton>
    </div>
    <button
      type="button"
      class="self-start text-[10px] uppercase tracking-wider text-terminal-text-dim hover:text-accent transition-colors mt-0.5"
      @click="addRow"
    >
      + Add property
    </button>
  </div>
</template>
