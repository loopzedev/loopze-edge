<script setup lang="ts">
import { computed } from "vue";
import type { NodeProps } from "@vue-flow/core";
import BaseNode from "@/components/nodes/BaseNode.vue";
import { useFlowStore } from "@/stores/flowStore";

defineOptions({ inheritAttrs: false });

const props = defineProps<NodeProps>();
const flowStore = useFlowStore();

const label = computed(() => props.data?.label);
const config = computed(() => (props.data?.config ?? {}) as Record<string, unknown>);

// Resolve the linked target node name and flow
const targetInfo = computed(() => {
  const targetId = config.value.linkTarget as string;
  if (!targetId) return null;

  for (const flow of flowStore.flows) {
    const node = flow.nodes.find((n) => n.id === targetId);
    if (node) {
      return {
        flowLabel: flow.label,
        nodeName: node.name || node.id.slice(0, 8),
      };
    }
  }
  return null;
});
</script>

<template>
  <BaseNode
    :id="props.id"
    :label="label"
    node-type="link-call"
    :selected="props.selected"
    :inputs="props.data?.inputs ?? 1"
    :outputs="props.data?.outputs ?? 1"
    :status="props.data?.status"
    :disabled="props.data?.disabled"
  >
    <template #body>
      <div v-if="targetInfo" class="flex items-center gap-1 truncate">
        <span>{{ targetInfo.flowLabel }}</span>
        <span>/</span>
        <span class="text-terminal-text">{{ targetInfo.nodeName }}</span>
      </div>
      <span v-else class="italic">nicht verknüpft</span>
    </template>
  </BaseNode>
</template>
