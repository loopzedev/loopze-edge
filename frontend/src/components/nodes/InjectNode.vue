<script setup lang="ts">
import { computed } from "vue";
import type { NodeProps } from "@vue-flow/core";
import { useApi } from "@/composables/useApi";
import { useAuthStore } from "@/stores/authStore";
import BaseNode from "@/components/nodes/BaseNode.vue";

defineOptions({ inheritAttrs: false });

const props = defineProps<NodeProps>();
const api = useApi();
const auth = useAuthStore();

// The action button (TRIG) only makes sense for users who can fire
// inject nodes. For viewers we hide it; the rest of the node still
// renders so they can see what is on the canvas.
const actionButton = computed(() =>
  auth.can("inject")
    ? { label: "TRIG", title: "Trigger inject" }
    : null,
);

const label = computed(() => props.data?.label);

const intervalLabel = computed(() => {
    const cfg = props.data?.config ?? {};
    const interval = cfg.interval as number | undefined;
    const once = cfg.once as boolean | undefined;
    const parts: string[] = [];
    if (once) parts.push("once");
    if (interval && interval > 0) {
        if (interval >= 60000) parts.push(`${interval / 60000}min`);
        else if (interval >= 1000) parts.push(`${interval / 1000}s`);
        else parts.push(`${interval}ms`);
    }
    return parts.length > 0 ? parts.join(" + ") : "manual";
});

function handleTrigger(): void {
    api.triggerInject(props.id).catch((err) => {
        console.error("[InjectNode] Trigger failed:", err);
    });
}
</script>

<template>
    <BaseNode
        :id="props.id"
        :label="label"
        node-type="inject"
        :selected="props.selected"
        :inputs="0"
        :outputs="1"
        :status="props.data?.status"
        :disabled="props.data?.disabled"
        :action-button="actionButton"
        @action="handleTrigger"
    >
        <template #body>
            <div class="flex items-center justify-between gap-1">
                <span class="uppercase tracking-wider">{{ intervalLabel }}</span>
                <span class="truncate">{{ props.data?.config?.payloadType ?? "timestamp" }}</span>
            </div>
        </template>
    </BaseNode>
</template>
