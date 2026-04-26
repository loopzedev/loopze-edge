<script setup lang="ts">
import { computed, onBeforeUnmount } from 'vue'
import FormField from '@/components/ui/FormField.vue'
import FormSelect from '@/components/ui/FormSelect.vue'
import FormCheckbox from '@/components/ui/FormCheckbox.vue'
import { useNodeProperty } from '@/composables/useNodeProperty'
import { useFlowStore } from '@/stores/flowStore'

type Scope = 'flow' | 'selected' | 'all'

const flowStore = useFlowStore()

const scope = useNodeProperty<Scope>('scope', 'flow')
const targetNodes = useNodeProperty<string[]>('targetNodes', [])

const scopeOptions = [
  { value: 'flow', label: 'Current flow' },
  { value: 'selected', label: 'Selected nodes' },
  { value: 'all', label: 'All flows' },
]

// Candidates for the multi-select: every node in the active flow except
// the Status Node itself and other Status Nodes (they cannot be observed).
const candidates = computed(() => {
  const selfId = flowStore.selectedNode?.id
  return flowStore.activeNodes
    .filter((n) => n.id !== selfId && n.type !== 'status')
    .map((n) => ({
      id: n.id,
      label: (n.data?.label as string) || n.type,
      type: n.type,
    }))
    .sort((a, b) => a.label.localeCompare(b.label))
})

const selectedSet = computed(() => new Set(targetNodes.value))

function toggleNode(id: string, checked: boolean) {
  const next = new Set(targetNodes.value)
  if (checked) next.add(id)
  else next.delete(id)
  targetNodes.value = Array.from(next)
}

function highlight(id: string | null) {
  flowStore.setHoveredHighlightNodeId(id)
}

onBeforeUnmount(() => {
  flowStore.setHoveredHighlightNodeId(null)
})
</script>

<template>
  <div class="flex flex-col gap-3">
    <FormField label="Scope">
      <FormSelect v-model="scope" :options="scopeOptions" />
    </FormField>

    <FormField v-if="scope === 'selected'" label="Observed nodes">
      <div
        v-if="candidates.length === 0"
        class="text-[11px] text-terminal-text-dim/70 italic"
      >
        No other nodes in this flow yet.
      </div>
      <div
        v-else
        class="flex flex-col gap-1.5 max-h-48 overflow-y-auto bg-terminal-bg border border-terminal-border rounded p-2"
      >
        <div
          v-for="c in candidates"
          :key="c.id"
          @mouseenter="highlight(c.id)"
          @mouseleave="highlight(null)"
        >
          <FormCheckbox
            :model-value="selectedSet.has(c.id)"
            :label="`${c.label} (${c.type})`"
            @update:model-value="(v: boolean) => toggleNode(c.id, v)"
          />
        </div>
      </div>
    </FormField>

    <p class="text-[10px] text-terminal-text-dim leading-relaxed">
      Emits one message per status update of any node within the selected scope.
      Status Nodes themselves are excluded — they cannot trigger each other.
      In <code class="text-terminal-text">Selected</code> mode, only nodes from
      this flow can be picked. The outgoing message carries
      <code class="text-terminal-text">msg.status</code> with
      <code class="text-terminal-text">fill</code>,
      <code class="text-terminal-text">text</code> and a
      <code class="text-terminal-text">source</code> object;
      <code class="text-terminal-text">msg.payload</code> mirrors the status text.
    </p>
  </div>
</template>
