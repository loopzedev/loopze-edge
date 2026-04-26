<script setup lang="ts">
import { computed, ref } from "vue";
import { Handle, Position } from "@vue-flow/core";
import NodeIcon from "@/components/nodes/NodeIcon.vue";
import AppTooltip from "@/components/ui/AppTooltip.vue";
import { getTokens, STATUS_COLORS } from "@/components/nodes/tokens";
import { useFlowStore } from "@/stores/flowStore";

export interface ActionButton {
    /** Label displayed vertically on the button. */
    label: string;
    /** Tooltip text. */
    title?: string;
}

export interface BaseNodeProps {
    id: string;
    label?: string;
    nodeType?: string;
    inputs?: number;
    outputs?: number;
    /** Optional per-output tooltips. Index matches the port index. */
    outputLabels?: string[];
    selected?: boolean;
    disabled?: boolean;
    actionButton?: ActionButton | null;
    toggleButton?: boolean;
    toggleState?: boolean;
    status?: {
        fill?: "red" | "green" | "yellow" | "blue" | "grey";
        shape?: "ring" | "dot";
        text?: string;
    } | null;
}

const props = withDefaults(defineProps<BaseNodeProps>(), {
    label: "",
    nodeType: "unknown",
    inputs: 0,
    outputs: 0,
    selected: false,
    disabled: false,
    actionButton: null,
    toggleButton: false,
    toggleState: true,
    status: null,
});

const emit = defineEmits<{
    action: [];
    toggle: [value: boolean];
}>();

const actionActive = ref(false);
function handleAction() {
    actionActive.value = true;
    emit('action');
    setTimeout(() => { actionActive.value = false }, 80);
}

function handleToggle() {
    emit('toggle', !props.toggleState);
}

const flowStore = useFlowStore();

const t = computed(() => getTokens(props.nodeType));
const typeLabel = computed(() => {
    const labels: Record<string, string> = {
        inject: "Inject",
        debug: "Debug",
        function: "Function",
        "context-watch": "Context Watch",
        change: "Change",
        switch: "Switch",
        template: "Template",
        delay: "Delay",
        filter: "Filter",
        comment: "Comment",
        "link-in": "Link In",
        "link-out": "Link Out",
        "link-call": "Link Call",
        "mqtt-in": "MQTT Subscribe",
        "mqtt-out": "MQTT Publish",
        statemachine: "State Machine",
    };
    return labels[props.nodeType] ?? props.nodeType;
});
const hasCustomLabel = computed(() => {
    if (!props.label) return false;
    const l = props.label.toLowerCase();
    return (
        l !== props.nodeType.toLowerCase() &&
        l !== typeLabel.value.toLowerCase()
    );
});
const isDirty = computed(() => flowStore.isNodeDirty(props.id));

const isHighlighted = computed(
    () => flowStore.hoveredHighlightNodeId === props.id && !props.selected,
);

const statusColor = computed(() =>
    STATUS_COLORS[props.status?.fill ?? ''] ?? STATUS_COLORS.grey
);

const PORT_SPACING = 24;

const maxPorts = computed(() => Math.max(props.inputs, props.outputs, 1));
const nodeMinHeight = computed(() =>
    `${(maxPorts.value + 1) * PORT_SPACING}px`,
);

const inputHandles = computed(() =>
    Array.from({ length: props.inputs }, (_, i) => ({
        id: `input-${i}`,
        style: {
            top: `${(i + 1) * PORT_SPACING}px`,
        },
    })),
);

const outputHandles = computed(() =>
    Array.from({ length: props.outputs }, (_, i) => ({
        id: `output-${i}`,
        title: props.outputLabels?.[i] ?? "",
        style: {
            top: `${(i + 1) * PORT_SPACING}px`,
        },
    })),
);
</script>

<template>
  <div
    class="flex flex-col items-start gap-1.5"
    :class="{ 'opacity-40': props.disabled }"
  >
    <!-- Row: action button + main node + toggle button -->
    <div class="flex items-stretch">
      <!-- Action button (optional, displayed left of the node) -->
      <button
        v-if="props.actionButton"
        class="shrink-0 w-9 flex items-center justify-center transition-all duration-100 nopan nodrag"
        :style="{
          background: actionActive ? t.accent + '22' : '#161b22',
          border: `1px solid ${actionActive ? t.accent : t.border}`,
          borderRight: 'none',
          borderRadius: '6px 0 0 6px',
        }"
        :title="props.actionButton.title ?? props.actionButton.label"
        @click.stop="handleAction"
        @dblclick.stop
        @mousedown.stop
        @pointerdown.stop
      >
        <span
          class="text-[9px] font-bold tracking-widest transition-colors"
          :style="{ color: actionActive ? t.accent : t.textSub }"
          style="writing-mode: vertical-lr; text-orientation: mixed"
        >{{ props.actionButton.label }}</span>
      </button>

      <!-- Main node body -->
      <div
        class="flint-node relative flex flex-col rounded-md min-w-[210px] select-none"
        :class="{ selected: props.selected }"
        :style="{
          background: '#161b22',
          minHeight: nodeMinHeight,
          border: `1px solid ${props.selected ? t.accent : t.border}`,
          boxShadow: props.selected ? `0 0 0 1px ${t.accent}, 0 4px 20px ${t.accentGlow}` : 'none',
          outline: isHighlighted ? `1px dashed ${t.accent}` : undefined,
          outlineOffset: isHighlighted ? '-1px' : undefined,
        }"
      >
        <!-- Input Handles -->
        <Handle
          v-for="h in inputHandles"
          :key="h.id"
          :id="h.id"
          type="target"
          :position="Position.Left"
          :style="{
            ...h.style,
            borderColor: selected ? t.accent : '#7d8590',
            background: selected ? t.accent + '33' : '#161b22',
          }"
          class="!w-2.5 !h-2.5 !rounded-full !border-2 transition-colors"
        />

        <!-- Output Handles -->
        <AppTooltip
          v-for="h in outputHandles"
          :key="h.id"
          :text="h.title"
          side="right"
        >
          <Handle
            :id="h.id"
            type="source"
            :position="Position.Right"
            :style="{
              ...h.style,
              borderColor: selected ? t.accent : '#7d8590',
              background: selected ? t.accent + '33' : '#161b22',
            }"
            class="!w-2.5 !h-2.5 !rounded-full !border-2 transition-colors"
          />
        </AppTooltip>

        <!-- Header: icon + typeLabel + badge + dirty -->
        <div class="flex items-center gap-2 px-3 pt-2.5">
          <slot name="icon">
            <span :style="{ color: t.accent }"><NodeIcon :type="props.nodeType" /></span>
          </slot>
          <span
            class="text-[11px] font-semibold tracking-wide truncate flex-1"
            :style="{ color: t.accent }"
          >
            {{ typeLabel }}
          </span>
          <slot name="badge" />
          <span
            v-if="isDirty"
            class="w-2 h-2 shrink-0 rounded-full"
            style="background: #58a6ff; box-shadow: 0 0 4px #58a6ff80"
            title="Undeployed changes"
          />
        </div>

        <!-- Body: custom label takes priority, otherwise slot content -->
        <div
          v-if="hasCustomLabel || $slots.body"
          class="px-3 py-1 text-[10px] font-mono"
          style="color: #b1bac2"
        >
          <span v-if="hasCustomLabel">{{ props.label }}</span>
          <slot v-else name="body" />
        </div>

        <!-- Actions (optional) -->
        <div v-if="$slots.actions" class="px-3 pb-1">
          <slot name="actions" />
        </div>

        <!-- Bottom accent line — pinned to the bottom even when min-height kicks in -->
        <div class="h-[2px] rounded-b-md mt-auto" :style="{ background: t.accent }" />
      </div>

      <!-- Toggle button (optional, displayed right of the node) -->
      <button
        v-if="props.toggleButton"
        class="shrink-0 w-9 flex items-center justify-center transition-all duration-100 nopan nodrag"
        :style="{
          background: props.toggleState ? t.accent + '22' : '#161b22',
          border: `1px solid ${props.toggleState ? t.accent : t.border}`,
          borderLeft: 'none',
          borderRadius: '0 6px 6px 0',
        }"
        :title="props.toggleState ? 'Enabled — click to disable' : 'Disabled — click to enable'"
        @click.stop="handleToggle"
        @dblclick.stop
        @mousedown.stop
        @pointerdown.stop
      >
        <span
          class="text-[9px] font-bold tracking-widest transition-colors"
          :style="{ color: props.toggleState ? t.accent : t.textSub }"
          style="writing-mode: vertical-lr; text-orientation: mixed"
        >{{ props.toggleState ? 'ON' : 'OFF' }}</span>
      </button>
    </div>

    <!-- Status pill — lives outside the node body -->
    <div
      v-if="props.status"
      class="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-[10px] ml-2"
      :style="{
        background: statusColor + '22',
        color: statusColor,
        border: `1px solid ${statusColor}55`,
        whiteSpace: 'nowrap',
      }"
    >
      <span
        class="w-1.5 h-1.5 rounded-full"
        :style="{ background: statusColor, boxShadow: `0 0 4px ${statusColor}` }"
      />
      <span>{{ props.status.text ?? "" }}</span>
    </div>
  </div>
</template>

<style scoped>
.flint-node {
    transition: box-shadow 0.15s;
}
</style>
