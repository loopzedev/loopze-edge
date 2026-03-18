<script setup lang="ts">
import { computed } from "vue";
import type { NodeProps } from "@vue-flow/core";
import { useApi } from "@/composables/useApi";
import BaseNode from "@/components/nodes/BaseNode.vue";

defineOptions({ inheritAttrs: false });

const props = defineProps<NodeProps>();
const api = useApi();

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

async function handleTrigger(): Promise<void> {
    try {
        await api.triggerInject(props.id);
    } catch (err) {
        console.error("[InjectNode] Trigger failed:", err);
    }
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
        :action-button="{ label: 'TRIG', title: 'Trigger inject' }"
        @action="handleTrigger"
    >
        <template #body>
            <div class="flex items-center justify-between gap-1">
                <span class="uppercase tracking-wider opacity-60">{{ intervalLabel }}</span>
                <span class="opacity-60 truncate">{{ props.data?.config?.payloadType ?? "timestamp" }}</span>
            </div>
        </template>
    </BaseNode>
</template>
