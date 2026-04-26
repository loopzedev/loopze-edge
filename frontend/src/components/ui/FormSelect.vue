<script setup lang="ts">
import { computed } from 'vue'
import {
  SelectRoot,
  SelectTrigger,
  SelectValue,
  SelectPortal,
  SelectContent,
  SelectViewport,
  SelectItem,
  SelectItemText,
} from 'radix-vue'

const props = defineProps<{
  modelValue: string | number
  options: { value: string | number; label: string }[]
  width?: string
  placeholder?: string
}>()

const emit = defineEmits<{
  'update:modelValue': [value: string | number]
}>()

const selectedLabel = computed(() =>
  props.options.find(o => String(o.value) === String(props.modelValue))?.label ?? ''
)

// radix-vue's SelectRoot emits the picked value as a string (its model is
// always string-typed). We resolve it back to the original option value so a
// number-typed option round-trips as a number — otherwise consumers like
// node configs would silently store "2" instead of 2 and Go decoders looking
// for float64 would fall back to defaults.
function handleUpdate(rawValue: string) {
  const match = props.options.find(o => String(o.value) === rawValue)
  emit('update:modelValue', match ? match.value : rawValue)
}
</script>

<template>
  <SelectRoot
    :model-value="String(modelValue)"
    @update:model-value="handleUpdate"
  >
    <SelectTrigger
      class="inline-flex items-center justify-between gap-1
             bg-terminal-bg border border-terminal-border text-terminal-text
             px-2 py-1 text-[10px] outline-none shrink-0 cursor-pointer
             hover:border-terminal-text focus:border-accent
             data-[placeholder]:text-terminal-text-dim"
      :style="width ? { width } : undefined"
    >
      <SelectValue :placeholder="placeholder">
        {{ selectedLabel }}
      </SelectValue>
      <span class="text-[8px] text-terminal-text-dim">&#x25BE;</span>
    </SelectTrigger>

    <SelectPortal>
      <SelectContent
        class="bg-terminal-surface border border-terminal-border shadow-lg z-50 overflow-hidden min-w-[var(--radix-select-trigger-width)]"
        position="popper"
        :side-offset="4"
      >
        <SelectViewport class="p-0.5">
          <SelectItem
            v-for="opt in options"
            :key="String(opt.value)"
            :value="String(opt.value)"
            class="flex items-center px-2 py-1 text-[10px] text-terminal-text outline-none cursor-pointer
                   data-[highlighted]:bg-accent/20 data-[highlighted]:text-terminal-text-bright
                   data-[state=checked]:text-accent"
          >
            <SelectItemText>{{ opt.label }}</SelectItemText>
          </SelectItem>
        </SelectViewport>
      </SelectContent>
    </SelectPortal>
  </SelectRoot>
</template>
