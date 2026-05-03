<script setup lang="ts">
import { computed } from "vue";
import { Handle, Position } from "@vue-flow/core";
import type { NodeProps } from "@vue-flow/core";
import NodeIcon from "@/components/nodes/NodeIcon.vue";
import { getTokens } from "@/components/nodes/tokens";
import { useFlowStore } from "@/stores/flowStore";

defineOptions({ inheritAttrs: false });

const props = defineProps<NodeProps>();
const flowStore = useFlowStore();

const nodeType = computed(() => props.type ?? "link-in");
const t = computed(() => getTokens(nodeType.value));
const isDirty = computed(() => flowStore.isNodeDirty(props.id));
const label = computed(() => props.data?.label || "");

const hasInput = computed(() => (props.data?.inputs ?? 0) > 0);
const hasOutput = computed(() => (props.data?.outputs ?? 0) > 0);
</script>

<template>
  <div
    class="loopze-link-node relative font-mono select-none flex items-center justify-center"
    :class="{ 'opacity-40': props.data?.disabled }"
    :style="{
      width: '48px',
      height: '48px',
      background: t.bgIcon,
      border: `1px ${props.data?.disabled ? 'dashed' : 'solid'} ${props.selected ? t.accent : t.border}`,
      borderRadius: '6px',
      boxShadow: props.selected
        ? `0 0 0 1px ${t.accentBdr}, 0 4px 12px ${t.accentGlow}`
        : 'none',
      color: t.accent,
    }"
  >
    <!-- Input Handle -->
    <Handle
      v-if="hasInput"
      id="input-0"
      type="target"
      :position="Position.Left"
      class="!w-2.5 !h-2.5 !rounded-full !border-2 transition-colors"
      :class="
        props.selected
          ? '!border-[#9ca3af] !bg-[#9ca3af]/20'
          : '!border-terminal-text-dim !bg-terminal-surface'
      "
    />

    <!-- Output Handle -->
    <Handle
      v-if="hasOutput"
      id="output-0"
      type="source"
      :position="Position.Right"
      class="!w-2.5 !h-2.5 !rounded-full !border-2 transition-colors"
      :class="
        props.selected
          ? '!border-[#9ca3af] !bg-[#9ca3af]/20'
          : '!border-terminal-text-dim !bg-terminal-surface'
      "
    />

    <!-- Icon -->
    <NodeIcon :type="nodeType" />

    <!-- Dirty indicator -->
    <span
      v-if="isDirty"
      class="absolute -top-1 -right-1 w-2 h-2 rounded-full"
      style="background: #58a6ff; box-shadow: 0 0 4px #58a6ff80"
      title="Undeployed changes"
    />

    <!-- Label tooltip (shown on hover via title) -->
    <div
      v-if="label"
      class="absolute -bottom-5 left-1/2 -translate-x-1/2 text-[9px] whitespace-nowrap"
      :style="{ color: t.textSub }"
    >
      {{ label }}
    </div>
  </div>
</template>

<style scoped>
.loopze-link-node {
  transition: box-shadow 0.15s;
}
</style>
