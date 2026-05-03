<script setup lang="ts">
import { computed } from 'vue'
import { useFlowStore } from '@/stores/flowStore'
import FormField from '@/components/ui/FormField.vue'

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

const targetType = computed(() => {
  if (nodeType.value === 'link-in') return 'link-out'
  return 'link-in'
})

const singleSelect = computed(() => nodeType.value === 'link-call')

const tableLabel = computed(() =>
  targetType.value === 'link-in' ? 'Link Inputs' : 'Link Outputs',
)

const availableNodes = computed(() => {
  const results: Array<{ nodeId: string; nodeName: string; flowLabel: string }> = []
  for (const flow of flowStore.flows) {
    if (flow.id === flowStore.activeFlowId) {
      for (const vfNode of flowStore.nodes) {
        const type = vfNode.data?.nodeType ?? vfNode.type
        if (type === targetType.value) {
          results.push({
            nodeId: vfNode.id,
            nodeName: (vfNode.data?.label as string) || vfNode.id.slice(0, 12),
            flowLabel: flow.label,
          })
        }
      }
    } else {
      for (const loopzeNode of flow.nodes) {
        if (loopzeNode.type === targetType.value) {
          results.push({
            nodeId: loopzeNode.id,
            nodeName: loopzeNode.name || loopzeNode.id.slice(0, 12),
            flowLabel: flow.label,
          })
        }
      }
    }
  }
  return results
})

const selectedLinks = computed<string[]>(() => {
  if (singleSelect.value) {
    const target = config.value.linkTarget as string
    return target ? [target] : []
  }
  const links = config.value.links
  return Array.isArray(links) ? (links as string[]) : []
})

function isSelected(nodeId: string): boolean {
  return selectedLinks.value.includes(nodeId)
}

function toggleLink(targetNodeId: string) {
  const currentlySelected = isSelected(targetNodeId)
  const ownNodeId = node.value?.id
  if (!ownNodeId) return

  if (singleSelect.value) {
    update('linkTarget', currentlySelected ? '' : targetNodeId)
  } else {
    const current = [...selectedLinks.value]
    if (currentlySelected) {
      update('links', current.filter((id) => id !== targetNodeId))
    } else {
      update('links', [...current, targetNodeId])
    }
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
  <FormField :label="`${tableLabel} · ${selectedLinks.length} selected`">
    <div v-if="availableNodes.length === 0" class="text-[10px] text-terminal-text-dim italic">
      No {{ targetType }} nodes available
    </div>

    <div v-else class="border border-terminal-border bg-terminal-bg">
      <div class="flex text-[10px] text-terminal-text-dim uppercase tracking-wider border-b border-terminal-border bg-terminal-surface-alt/30">
        <div class="w-8 px-2 py-1 flex-shrink-0"></div>
        <div class="flex-1 px-2 py-1">Flow</div>
        <div class="flex-1 px-2 py-1">Node</div>
      </div>
      <div
        v-for="item in availableNodes"
        :key="item.nodeId"
        class="flex items-center text-[10px] border-b border-terminal-border last:border-b-0 hover:bg-terminal-surface-alt/40 cursor-pointer transition-colors duration-100"
        :class="isSelected(item.nodeId) ? 'bg-accent/5' : ''"
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
  </FormField>
</template>
