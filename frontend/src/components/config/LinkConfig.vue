<script setup lang="ts">
import { computed } from 'vue'
import { useFlowStore } from '@/stores/flowStore'
import FormLabel from '@/components/ui/FormLabel.vue'
import FormInput from '@/components/ui/FormInput.vue'
import SectionHeader from '@/components/ui/SectionHeader.vue'

const flowStore = useFlowStore()

const node = computed(() => flowStore.selectedNode)
const config = computed(() => (node.value?.data?.config ?? {}) as Record<string, unknown>)
const nodeType = computed(() => node.value?.type ?? '')

function update(key: string, value: unknown) {
  if (!node.value) return
  flowStore.updateNodeData(node.value.id, {
    config: { ...config.value, [key]: value },
  })
}

// Which type of nodes to show in the table?
const targetType = computed(() => {
  if (nodeType.value === 'link-in') return 'link-out'
  return 'link-in' // link-out and link-call both show link-in nodes
})

const singleSelect = computed(() => nodeType.value === 'link-call')

const tableLabel = computed(() => {
  return targetType.value === 'link-in' ? 'Link Inputs' : 'Link Outputs'
})

// Collect all matching nodes from all flows
const availableNodes = computed(() => {
  // Sync current canvas so active flow has up-to-date data
  flowStore.syncCanvasToActiveFlow()

  const results: Array<{ nodeId: string; nodeName: string; flowLabel: string }> = []
  for (const flow of flowStore.flows) {
    for (const flintNode of flow.nodes) {
      if (flintNode.type === targetType.value) {
        results.push({
          nodeId: flintNode.id,
          nodeName: flintNode.name || flintNode.id.slice(0, 12),
          flowLabel: flow.label,
        })
      }
    }
  }
  return results
})

// Current selection
const selectedLinks = computed<string[]>(() => {
  if (singleSelect.value) {
    const target = config.value.linkTarget as string
    return target ? [target] : []
  }
  const links = config.value.links
  return Array.isArray(links) ? links as string[] : []
})

function isSelected(nodeId: string): boolean {
  return selectedLinks.value.includes(nodeId)
}

function toggleLink(targetNodeId: string) {
  const currentlySelected = isSelected(targetNodeId)
  const ownNodeId = node.value?.id
  if (!ownNodeId) return

  if (singleSelect.value) {
    // Radio: select or deselect
    update('linkTarget', currentlySelected ? '' : targetNodeId)
  } else {
    // Checkbox: toggle in links array
    const current = [...selectedLinks.value]
    if (currentlySelected) {
      update('links', current.filter((id) => id !== targetNodeId))
    } else {
      update('links', [...current, targetNodeId])
    }

    // Bidirectional mirroring (only for link-in <-> link-out)
    mirrorLinkConfig(targetNodeId, !currentlySelected, ownNodeId)
  }
}

function mirrorLinkConfig(targetNodeId: string, selected: boolean, ownNodeId: string) {
  for (const flow of flowStore.flows) {
    const targetFlowNode = flow.nodes.find((n) => n.id === targetNodeId)
    if (!targetFlowNode) continue

    const targetConfig = targetFlowNode.config ?? {}
    const targetLinks = Array.isArray(targetConfig.links)
      ? [...(targetConfig.links as string[])]
      : []

    if (selected && !targetLinks.includes(ownNodeId)) {
      targetLinks.push(ownNodeId)
    } else if (!selected) {
      const idx = targetLinks.indexOf(ownNodeId)
      if (idx !== -1) targetLinks.splice(idx, 1)
    }

    targetFlowNode.config = { ...targetConfig, links: targetLinks }
    flowStore.markNodeDirty(targetNodeId)

    // If the target node is on the active canvas, also update the Vue Flow node
    const canvasNode = flowStore.nodes.find((n) => n.id === targetNodeId)
    if (canvasNode) {
      flowStore.updateNodeData(targetNodeId, {
        config: { ...targetConfig, links: targetLinks },
      })
    }
    break
  }
}
</script>

<template>
  <div class="flex flex-col gap-3">
    <div class="flex flex-col gap-1">
      <FormLabel>Name</FormLabel>
      <FormInput
        :model-value="(node?.data?.label as string) ?? ''"
        placeholder="Node name"
        @update:model-value="flowStore.updateNodeData(node!.id, { label: $event })"
      />
    </div>

    <SectionHeader :title="tableLabel">
      <div v-if="availableNodes.length === 0" class="text-[10px] text-terminal-text-dim italic">
        Keine {{ targetType }} Nodes vorhanden
      </div>

      <div v-else class="border border-terminal-border">
        <!-- Header -->
        <div class="flex text-[10px] text-terminal-text-dim uppercase tracking-wider border-b border-terminal-border">
          <div class="w-8 px-2 py-1 flex-shrink-0"></div>
          <div class="flex-1 px-2 py-1">Flow</div>
          <div class="flex-1 px-2 py-1">Node</div>
        </div>
        <!-- Rows -->
        <div
          v-for="item in availableNodes"
          :key="item.nodeId"
          class="flex items-center text-[10px] border-b border-terminal-border last:border-b-0 hover:bg-terminal-bg/50 cursor-pointer transition-colors duration-100"
          @click="toggleLink(item.nodeId)"
        >
          <div class="w-8 px-2 py-1.5 flex-shrink-0 flex items-center justify-center">
            <input
              v-if="singleSelect"
              type="radio"
              :name="'link-target-' + node?.id"
              :checked="isSelected(item.nodeId)"
              class="accent-accent pointer-events-none"
            />
            <input
              v-else
              type="checkbox"
              :checked="isSelected(item.nodeId)"
              class="accent-accent pointer-events-none"
            />
          </div>
          <div class="flex-1 px-2 py-1.5 text-terminal-text-dim truncate">{{ item.flowLabel }}</div>
          <div class="flex-1 px-2 py-1.5 text-terminal-text truncate">{{ item.nodeName }}</div>
        </div>
      </div>
    </SectionHeader>
  </div>
</template>
