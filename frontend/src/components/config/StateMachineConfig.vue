<script setup lang="ts">
import { computed, ref, defineAsyncComponent } from 'vue'
import { useFlowStore } from '@/stores/flowStore'
import FormLabel from '@/components/ui/FormLabel.vue'
import FormCheckbox from '@/components/ui/FormCheckbox.vue'

const SimpleEditor = defineAsyncComponent(() =>
  import('@/components/ui/SimpleEditor.vue')
)

const flowStore = useFlowStore()

const node = computed(() => flowStore.selectedNode)
const config = computed(() => (node.value?.data?.config ?? {}) as Record<string, unknown>)

function update(key: string, value: unknown) {
  if (!node.value) return
  flowStore.updateNodeData(node.value.id, {
    config: { ...config.value, [key]: value },
  })
}

const activeTab = ref<'machine' | 'guards' | 'actions'>('machine')

const machineJSON = computed({
  get: () => {
    const raw = config.value.machine as string | undefined
    if (!raw) return ''
    try { return JSON.stringify(JSON.parse(raw), null, 2) } catch { return raw }
  },
  set: (v: string) => {
    try { update('machine', JSON.stringify(JSON.parse(v))) } catch { update('machine', v) }
  },
})

const guardsCode = computed({
  get: () => (config.value.guards as string) ?? '',
  set: (v: string) => update('guards', v),
})

const actionsCode = computed({
  get: () => (config.value.actions as string) ?? '',
  set: (v: string) => update('actions', v),
})

const persist = computed({
  get: () => (config.value.persist as boolean) ?? false,
  set: (v: boolean) => update('persist', v),
})

const jsonError = computed(() => {
  try {
    if (machineJSON.value) JSON.parse(machineJSON.value)
    return ''
  } catch (e: any) {
    return e.message ?? 'Invalid JSON'
  }
})
</script>

<template>
  <div class="flex flex-col gap-2 flex-1 min-h-0">
    <!-- Tabs -->
    <div class="flex gap-0.5 border-b border-terminal-border flex-shrink-0">
      <button
        v-for="tab in [
          { key: 'machine', label: 'Machine' },
          { key: 'guards', label: 'Guards' },
          { key: 'actions', label: 'Actions' },
        ]"
        :key="tab.key"
        class="px-2 py-1 text-[10px] uppercase tracking-wider transition-colors"
        :class="activeTab === tab.key
          ? 'text-accent border-b border-accent -mb-px'
          : 'text-terminal-text-dim hover:text-terminal-text'"
        @click="activeTab = tab.key as any"
      >{{ tab.label }}</button>
    </div>

    <!-- Machine Definition -->
    <div v-show="activeTab === 'machine'" class="flex flex-col gap-1 flex-1 min-h-0">
      <div class="flex items-center justify-between flex-shrink-0">
        <FormLabel>Machine Definition</FormLabel>
        <span class="text-[10px]" :class="jsonError ? 'text-red-400' : 'text-terminal-text-dim'">
          {{ jsonError || 'JSON' }}
        </span>
      </div>
      <SimpleEditor
        :model-value="machineJSON"
        language="json"
        @update:model-value="machineJSON = $event"
      />
    </div>

    <!-- Guards -->
    <div v-show="activeTab === 'guards'" class="flex flex-col gap-1 flex-1 min-h-0">
      <div class="flex items-center justify-between flex-shrink-0">
        <FormLabel>Guards</FormLabel>
        <span class="text-[10px] text-terminal-text-dim">JavaScript</span>
      </div>
      <SimpleEditor
        :model-value="guardsCode"
        language="javascript"
        @update:model-value="guardsCode = $event"
      />
    </div>

    <!-- Actions -->
    <div v-show="activeTab === 'actions'" class="flex flex-col gap-1 flex-1 min-h-0">
      <div class="flex items-center justify-between flex-shrink-0">
        <FormLabel>Actions</FormLabel>
        <span class="text-[10px] text-terminal-text-dim">JavaScript</span>
      </div>
      <SimpleEditor
        :model-value="actionsCode"
        language="javascript"
        @update:model-value="actionsCode = $event"
      />
    </div>

    <!-- Options -->
    <div class="flex flex-col gap-2 pt-1 border-t border-terminal-border flex-shrink-0">
      <FormCheckbox v-model="persist" label="Persist state across deploys" />
    </div>
  </div>
</template>
