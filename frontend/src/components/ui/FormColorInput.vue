<script setup lang="ts">
import { computed } from 'vue'
import FormInput from './FormInput.vue'

const props = defineProps<{
  modelValue: string
  placeholder?: string
  /** Shows a ✕ button to clear the value. Use when empty = "no color". */
  clearable?: boolean
  /** Show only the color swatch, no text input. For compact inline layouts. */
  swatchOnly?: boolean
}>()

const emit = defineEmits<{ 'update:modelValue': [value: string] }>()

// Native <input type="color"> requires a valid 6-digit hex string.
// Fall back silently so the swatch never shows an error state.
const swatchValue = computed(() =>
  /^#[0-9a-fA-F]{6}$/.test(props.modelValue) ? props.modelValue : '#000000',
)
</script>

<template>
  <div class="color-row">
    <input
      type="color"
      class="color-swatch"
      :value="swatchValue"
      @input="emit('update:modelValue', ($event.target as HTMLInputElement).value)"
    />
    <FormInput
      v-if="!swatchOnly"
      :model-value="modelValue"
      :placeholder="placeholder ?? '#rrggbb'"
      mono
      class="flex-1"
      @update:model-value="emit('update:modelValue', $event as string)"
    />
    <button
      v-if="clearable && modelValue"
      type="button"
      class="color-clear"
      title="Clear"
      @click="emit('update:modelValue', '')"
    >✕</button>
  </div>
</template>

<style scoped>
.color-row {
  display: flex;
  align-items: center;
  gap: 0.4rem;
}

.color-swatch {
  width: 28px;
  height: 28px;
  padding: 2px;
  border: 1px solid var(--color-border, #2a2e38);
  border-radius: 5px;
  background: transparent;
  cursor: pointer;
  flex-shrink: 0;
}
.color-swatch::-webkit-color-swatch-wrapper { padding: 0; border-radius: 3px; }
.color-swatch::-webkit-color-swatch { border: none; border-radius: 3px; }

.color-clear {
  background: transparent;
  border: none;
  color: var(--color-text-dim, #5a6070);
  font-size: 0.7rem;
  cursor: pointer;
  padding: 0.2rem 0.35rem;
  border-radius: 3px;
  flex-shrink: 0;
  line-height: 1;
}
.color-clear:hover {
  color: var(--color-text, #e2e8f0);
  background: rgba(255, 255, 255, 0.06);
}
</style>
