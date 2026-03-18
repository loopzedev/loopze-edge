<script setup lang="ts">
import { computed, ref } from "vue";
import { Handle, Position } from "@vue-flow/core";
import NodeIcon from "@/components/nodes/NodeIcon.vue";
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
    if (actionActive.value) return;
    actionActive.value = true;
    emit('action');
    setTimeout(() => { actionActive.value = false }, 200);
}

function handleToggle() {
    emit('toggle', !props.toggleState);
}

const flowStore = useFlowStore();

const t = computed(() => getTokens(props.nodeType));
const typeLabel = computed(() => {
    // Map node type to a readable display name
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

const statusColor = computed(() =>
    STATUS_COLORS[props.status?.fill ?? ''] ?? STATUS_COLORS.grey
);

const inputHandles = computed(() =>
    Array.from({ length: props.inputs }, (_, i) => ({
        id: `input-${i}`,
        style: {
            top:
                props.inputs === 1
                    ? "50%"
                    : `${20 + (60 / Math.max(props.inputs - 1, 1)) * i}%`,
        },
    })),
);

const outputHandles = computed(() =>
    Array.from({ length: props.outputs }, (_, i) => ({
        id: `output-${i}`,
        style: {
            top:
                props.outputs === 1
                    ? "50%"
                    : `${20 + (60 / Math.max(props.outputs - 1, 1)) * i}%`,
        },
    })),
);
</script>

<template>
  <div class="flex items-stretch">
    <!-- Action button (optional, displayed left of the node) -->
    <button
      v-if="props.actionButton"
      class="shrink-0 w-10 flex flex-col items-center justify-center rounded-l-[8px] transition-all duration-100 font-mono nopan nodrag"
      :style="{
        background: actionActive ? t.accent + '22' : t.bgIcon,
        border: `1px solid ${actionActive ? t.accent : t.border}`,
        borderRight: 'none',
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

    <div
        class="flint-node min-w-[196px] w-max relative font-mono select-none flex"
        :class="{ selected: props.selected, 'opacity-40': props.disabled }"
        :style="{
            border: `1px solid ${props.selected ? t.accent : t.border}`,
            borderLeft: `3px solid ${t.accent}`,
            boxShadow: props.selected
                ? `0 0 0 1px ${t.accentBdr}, 0 4px 20px ${t.accentGlow}`
                : 'none',
        }"
    >
        <!-- Input Handles -->
        <Handle
            v-for="h in inputHandles"
            :key="h.id"
            :id="h.id"
            type="target"
            :position="Position.Left"
            :style="h.style"
            class="!w-2.5 !h-2.5 !rounded-full !border-2 transition-colors"
            :class="
                selected
                    ? '!border-accent !bg-accent/20'
                    : '!border-terminal-text-dim !bg-terminal-surface'
            "
        />

        <!-- Output Handles -->
        <Handle
            v-for="h in outputHandles"
            :key="h.id"
            :id="h.id"
            type="source"
            :position="Position.Right"
            :style="h.style"
            class="!w-2.5 !h-2.5 !rounded-full !border-2 transition-colors"
            :class="
                selected
                    ? '!border-accent !bg-accent/20'
                    : '!border-terminal-text-dim !bg-terminal-surface'
            "
        />

        <!-- Dirty indicator (undeployed changes) -->
        <span
            v-if="isDirty"
            class="absolute -top-1 -right-1 w-2.5 h-2.5 rounded-full z-10"
            style="background: #58a6ff; box-shadow: 0 0 4px #58a6ff80"
            title="Undeployed changes"
        />

        <!-- Left icon column -->
        <div
            class="w-10 shrink-0 flex items-center justify-center"
            :style="{ background: t.bgIcon, color: t.accent }"
        >
            <slot name="icon"><NodeIcon :type="props.nodeType" /></slot>
        </div>

        <!-- Right content area -->
        <div class="flex-1 min-w-0 flex flex-col">
            <!-- Header -->
            <div
                class="flex items-center gap-1.5 px-2 py-1.5"
                :style="{
                    background: t.bgHdr,
                    borderBottom: `1px solid ${t.border}`,
                }"
            >
                <span
                    class="text-[11px] font-medium tracking-wide truncate flex-1"
                    :style="{ color: t.accent }"
                >
                    {{ typeLabel }}
                </span>
                <slot name="badge" />
            </div>

            <!-- Body: custom label takes priority, otherwise slot content -->
            <div
                v-if="hasCustomLabel || $slots.body"
                class="px-2 py-1 text-[10px]"
                :style="{ background: t.bg, color: t.textSub }"
            >
                <span v-if="hasCustomLabel">{{ props.label }}</span>
                <slot v-else name="body" />
            </div>

            <!-- Actions (optional) -->
            <div
                v-if="$slots.actions"
                :style="{
                    background: t.bg,
                    borderTop: `1px solid ${t.border}`,
                }"
            >
                <slot name="actions" />
            </div>

            <!-- Status bar -->
            <div
                v-if="props.status"
                class="flex items-center gap-1.5 px-2 py-1 text-[10px]"
                :style="{
                    background: t.bg,
                    borderTop: `1px solid ${t.border}`,
                    color: t.textSub,
                }"
            >
                <span
                    class="w-1.5 h-1.5 shrink-0 rounded-full"
                    :style="{ background: statusColor }"
                />
                <span class="truncate">{{ props.status.text ?? "" }}</span>
            </div>
        </div>

        <!-- Disabled overlay -->
        <div
            v-if="props.disabled"
            class="absolute inset-0 bg-terminal-bg/60 flex items-center justify-center"
        >
            <span
                class="text-[9px] text-terminal-text-dim uppercase tracking-widest"
                >disabled</span
            >
        </div>
    </div>

    <!-- Toggle button (optional, displayed right of the node) -->
    <button
      v-if="props.toggleButton"
      class="shrink-0 w-10 flex flex-col items-center justify-center rounded-r-[8px] transition-all duration-100 font-mono nopan nodrag"
      :style="{
        background: props.toggleState ? t.accent + '22' : t.bg,
        border: `1px solid ${props.toggleState ? t.accent : t.border}`,
        borderLeft: 'none',
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
</template>

<style scoped>
.flint-node {
    transition: box-shadow 0.15s;
    border-radius: 0px;
}
</style>
