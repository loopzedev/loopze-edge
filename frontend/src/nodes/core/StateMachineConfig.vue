<script setup lang="ts">
import { computed, ref, defineAsyncComponent } from 'vue'
import FormCheckbox from '@/components/ui/FormCheckbox.vue'
import TabsBar from '@/components/ui/TabsBar.vue'
import { useNodeProperty } from '@/composables/useNodeProperty'

const SimpleEditor = defineAsyncComponent(() =>
  import('@/components/ui/SimpleEditor.vue')
)

const machineRaw = useNodeProperty<string>('machine', '')
const guardsCode = useNodeProperty<string>('guards', '')
const actionsCode = useNodeProperty<string>('actions', '')
const persist = useNodeProperty<boolean>('persist', false)

const activeTab = ref<'machine' | 'guards' | 'actions'>('machine')

// Pretty-printed view of the machine JSON; on save, normalise back to compact.
const machineJSON = computed({
  get: () => {
    const raw = machineRaw.value
    if (!raw) return ''
    try { return JSON.stringify(JSON.parse(raw), null, 2) } catch { return raw }
  },
  set: (v: string) => {
    try { machineRaw.value = JSON.stringify(JSON.parse(v)) } catch { machineRaw.value = v }
  },
})

const jsonError = computed(() => {
  try {
    if (machineJSON.value) JSON.parse(machineJSON.value)
    return ''
  } catch (e: any) {
    return e.message ?? 'Invalid JSON'
  }
})

const tabs = computed(() => [
  { id: 'machine', label: 'Machine', badge: jsonError.value ? '!' : '' },
  { id: 'guards', label: 'Guards' },
  { id: 'actions', label: 'Actions' },
])
</script>

<template>
  <div class="flex flex-col gap-2 flex-1 min-h-0">
    <TabsBar v-model="activeTab as any" :tabs="tabs" />

    <!-- Machine -->
    <div v-show="activeTab === 'machine'" class="flex flex-col gap-1 flex-1 min-h-0">
      <div class="flex items-center justify-between shrink-0">
        <span class="text-[10px] text-terminal-text-dim uppercase tracking-wider font-semibold">Machine Definition</span>
        <span class="text-[10px] font-mono" :class="jsonError ? 'text-status-error' : 'text-terminal-text-dim'">
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
      <div class="flex items-center justify-between shrink-0">
        <span class="text-[10px] text-terminal-text-dim uppercase tracking-wider font-semibold">Guards</span>
        <span class="text-[10px] text-terminal-text-dim font-mono">JavaScript</span>
      </div>
      <SimpleEditor
        :model-value="guardsCode"
        language="javascript"
        @update:model-value="guardsCode = $event"
      />
    </div>

    <!-- Actions -->
    <div v-show="activeTab === 'actions'" class="flex flex-col gap-1 flex-1 min-h-0">
      <div class="flex items-center justify-between shrink-0">
        <span class="text-[10px] text-terminal-text-dim uppercase tracking-wider font-semibold">Actions</span>
        <span class="text-[10px] text-terminal-text-dim font-mono">JavaScript</span>
      </div>
      <SimpleEditor
        :model-value="actionsCode"
        language="javascript"
        @update:model-value="actionsCode = $event"
      />
    </div>

    <div class="pt-2 border-t border-terminal-border shrink-0">
      <FormCheckbox v-model="persist" label="Persist state across deploys" />
    </div>
  </div>
</template>
