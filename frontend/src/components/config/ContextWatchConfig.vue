<script setup lang="ts">
import { computed } from 'vue'
import { useFlowStore } from '@/stores/flowStore'
import FormLabel from '@/components/ui/FormLabel.vue'
import FormInput from '@/components/ui/FormInput.vue'
import FormCheckbox from '@/components/ui/FormCheckbox.vue'
import ToggleGroup from '@/components/ui/ToggleGroup.vue'

const flowStore = useFlowStore()

const node = computed(() => flowStore.selectedNode)
const config = computed(() => (node.value?.data?.config ?? {}) as Record<string, unknown>)

function update(key: string, value: unknown) {
  if (!node.value) return
  flowStore.updateNodeData(node.value.id, {
    config: { ...config.value, [key]: value },
  })
}

const scope = computed({
  get: () => (config.value.scope as string) ?? 'global',
  set: (v: string) => update('scope', v),
})

const storage = computed({
  get: () => (config.value.storage as string) ?? 'memory',
  set: (v: string) => update('storage', v),
})

const keyPattern = computed({
  get: () => (config.value.keyPattern as string) ?? '>',
  set: (v: string) => update('keyPattern', v),
})

const emitDeletes = computed({
  get: () => (config.value.emitDeletes as boolean) ?? false,
  set: (v: boolean) => update('emitDeletes', v),
})

const scopeOptions = [
  { label: 'global', value: 'global' },
  { label: 'flow', value: 'flow' },
]

const storageOptions = [
  { label: 'memory', value: 'memory' },
  { label: 'persistent', value: 'persistent' },
]
</script>

<template>
  <div class="flex flex-col gap-2">
    <!-- Scope -->
    <div class="flex flex-col gap-1">
      <FormLabel>Scope</FormLabel>
      <ToggleGroup v-model="scope" :options="scopeOptions" />
    </div>

    <!-- Storage -->
    <div class="flex flex-col gap-1">
      <FormLabel>Storage</FormLabel>
      <ToggleGroup v-model="storage" :options="storageOptions" />
    </div>

    <!-- Key Pattern -->
    <div class="flex flex-col gap-1">
      <FormLabel>Key Pattern</FormLabel>
      <FormInput v-model="keyPattern" placeholder=">" mono />
      <span class="text-[10px] text-terminal-text-dim">
        Use <code class="font-mono">&gt;</code> for all keys, or a specific key name
      </span>
    </div>

    <!-- Emit Deletes -->
    <FormCheckbox v-model="emitDeletes" label="Emit delete operations" />
  </div>
</template>
