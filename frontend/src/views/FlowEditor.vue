<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted, nextTick } from "vue";
import { VueFlow, useVueFlow, Panel } from "@vue-flow/core";
import { Background, BackgroundVariant } from "@vue-flow/background";
import { Controls } from "@vue-flow/controls";
import { MiniMap } from "@vue-flow/minimap";
import { useFlowStore } from "@/stores/flowStore";
import { useUiStore } from "@/stores/uiStore";
import { useApi } from "@/composables/useApi";
import BaseNode from "@/components/nodes/BaseNode.vue";
import InjectNode from "@/components/nodes/InjectNode.vue";
import DebugNode from "@/components/nodes/DebugNode.vue";
import FunctionNode from "@/components/nodes/FunctionNode.vue";
import ContextWatchNode from "@/components/nodes/ContextWatchNode.vue";
import ChangeNode from "@/components/nodes/ChangeNode.vue";
import LinkNode from "@/components/nodes/LinkNode.vue";
import LinkCallNode from "@/components/nodes/LinkCallNode.vue";

import "@vue-flow/core/dist/style.css";
import "@vue-flow/core/dist/theme-default.css";
import "@vue-flow/controls/dist/style.css";
import "@vue-flow/minimap/dist/style.css";

const flowStore = useFlowStore();
const uiStore = useUiStore();
const api = useApi();

const {
    onConnect,
    onNodeDragStop,
    screenToFlowCoordinate,
    setViewport,
    getSelectedNodes,
    viewport,
} = useVueFlow("flint-flow-editor");

const zoomPercent = computed(() => Math.round(viewport.value.zoom * 100));

const flowContainer = ref<HTMLElement | null>(null);
const lastMousePosition = ref<{ x: number; y: number } | null>(null);

onConnect((params) => {
    flowStore.connectNodes({
        source: params.source,
        target: params.target,
        sourceHandle: params.sourceHandle,
        targetHandle: params.targetHandle,
    });
});

onNodeDragStop((event) => {
    // event.nodes contains ALL dragged nodes (including multi-select)
    const draggedNodes = event.nodes ?? [event.node];
    for (const n of draggedNodes) {
        flowStore.updateNodePosition(n.id, n.position);
    }
});

function handleNodeClick(event: { node: any; event: MouseEvent | TouchEvent }): void {
    const shiftKey = event.event instanceof MouseEvent ? event.event.shiftKey : false;
    if (shiftKey) return;
    flowStore.selectNode(event.node.id);
}

function handleNodeDoubleClick(event: { node: any }): void {
    flowStore.selectNode(event.node.id);
    uiStore.clearFlowProperties();
    uiStore.openPropertiesPanel();
}

function handleSelectionChange(params: { nodes: any[]; edges: any[] }): void {
    const ids = params.nodes.map((n: any) => n.id);
    flowStore.setSelectedNodeIds(ids);
}

function onDragOver(event: DragEvent): void {
    event.preventDefault();
    if (event.dataTransfer) {
        event.dataTransfer.dropEffect = "move";
    }
}

function onDrop(event: DragEvent): void {
    event.preventDefault();

    if (!event.dataTransfer) return;

    const rawData = event.dataTransfer.getData("application/flint-node");
    if (!rawData) return;

    let nodeData: {
        type: string;
        label: string;
        inputs: number;
        outputs: number;
        defaults?: Record<string, any>;
    };
    try {
        nodeData = JSON.parse(rawData);
    } catch {
        return;
    }

    if (!flowContainer.value) return;

    const position = screenToFlowCoordinate({
        x: event.clientX,
        y: event.clientY,
    });

    flowStore.addNode(nodeData.type, position, {
        label: nodeData.label,
        inputs: nodeData.inputs,
        outputs: nodeData.outputs,
        config: nodeData.defaults ?? {},
    });
}

function onPaneClick(): void {
    flowStore.selectNode(null);
}

function onMouseMove(event: MouseEvent): void {
    lastMousePosition.value = { x: event.clientX, y: event.clientY };
}

function handleKeyDown(event: KeyboardEvent): void {
    // Skip if user is typing in an input/textarea
    const tag = (event.target as HTMLElement)?.tagName;
    if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT") return;

    const mod = event.ctrlKey || event.metaKey;
    if (!mod) return;

    // Sync Vue Flow's selection into the store before any clipboard operation
    const vfSelected = getSelectedNodes.value.map((n: any) => n.id);
    if (vfSelected.length > 0) {
        flowStore.setSelectedNodeIds(vfSelected);
    }

    switch (event.key.toLowerCase()) {
        case "c":
            event.preventDefault();
            flowStore.copySelectedNodes();
            break;
        case "v": {
            event.preventDefault();
            if (lastMousePosition.value) {
                const flowPos = screenToFlowCoordinate(lastMousePosition.value);
                flowStore.pasteNodes(flowPos);
            } else {
                flowStore.pasteNodes();
            }
            break;
        }
        case "x":
            event.preventDefault();
            flowStore.cutSelectedNodes();
            break;
        case "d":
            event.preventDefault();
            flowStore.duplicateSelectedNodes();
            break;
    }
}

// Reset viewport when switching flows
watch(() => flowStore.activeFlowId, async () => {
    await nextTick();
    setViewport({ x: 0, y: 0, zoom: 1 });
});

onUnmounted(() => {
    document.removeEventListener("keydown", handleKeyDown);
});

onMounted(async () => {
    document.addEventListener("keydown", handleKeyDown);
    try {
        const response = await api.getFlows();
        if (response.flows && response.flows.length > 0) {
            flowStore.loadFlows(response.flows, response.rev);
        } else {
            flowStore.addFlow("Flow 1");
        }
    } catch (err) {
        console.error("[FlowEditor] Failed to load flows from backend:", err);
        if (flowStore.flows.length === 0) {
            flowStore.addFlow("Flow 1");
        }
    }

    // Restore last known node statuses.
    try {
        const data = await api.getNodeStatuses();
        for (const [nodeId, entry] of Object.entries(data)) {
            flowStore.updateNodeStatus(nodeId, (entry as any).status);
        }
    } catch {
        // Non-critical — statuses will arrive via WebSocket
    }

    // Always start at top-left corner after loading.
    await nextTick();
    setViewport({ x: 0, y: 0, zoom: 1 });
});
</script>

<template>
    <div
        ref="flowContainer"
        class="w-full h-full bg-terminal-bg"
        @dragover="onDragOver"
        @drop="onDrop"
        @mousemove="onMouseMove"
    >
        <VueFlow
            id="flint-flow-editor"
            v-model:nodes="flowStore.nodes"
            v-model:edges="flowStore.edges"
            class="w-full h-full"
            :default-edge-options="{
                type: 'default',
                animated: false,
                style: { borderRadius: '16px' },
            }"
            :fit-view-on-init="false"
            :snap-to-grid="true"
            :snap-grid="[8, 8]"
            :delete-key-code="['Backspace', 'Delete']"
            :selection-on-drag="true"
            :pan-on-drag="[1, 2]"
            :selection-key-code="true"
            :multi-selection-key-code="null"
            :connection-line-type="'default' as any"
            :min-zoom="0.25"
            :max-zoom="1"
            :default-viewport="{ x: 0, y: 0, zoom: 1 }"
            :translate-extent="[
                [0, 0],
                [10000, 10000],
            ]"
            :prevent-scrolling="true"
            @pane-click="onPaneClick"
            @node-click="handleNodeClick"
            @node-double-click="handleNodeDoubleClick"
            @selection-change="handleSelectionChange"
        >
            <!-- Custom Node Types -->
            <template #node-inject="nodeProps">
                <InjectNode v-bind="nodeProps as any" />
            </template>

            <template #node-debug="nodeProps">
                <DebugNode v-bind="nodeProps as any" />
            </template>

            <template #node-function="nodeProps">
                <FunctionNode v-bind="nodeProps as any" />
            </template>

            <template #node-context-watch="nodeProps">
                <ContextWatchNode v-bind="nodeProps as any" />
            </template>

            <!-- Default fallback for all other node types -->
            <template #node-default="nodeProps">
                <BaseNode v-bind="nodeProps as any" />
            </template>

            <template #node-change="nodeProps">
                <ChangeNode v-bind="nodeProps as any" />
            </template>

            <template #node-switch="nodeProps">
                <BaseNode v-bind="nodeProps as any" />
            </template>

            <template #node-template="nodeProps">
                <BaseNode v-bind="nodeProps as any" />
            </template>

            <template #node-delay="nodeProps">
                <BaseNode v-bind="nodeProps as any" />
            </template>

            <template #node-filter="nodeProps">
                <BaseNode v-bind="nodeProps as any" />
            </template>

            <template #node-http-in="nodeProps">
                <BaseNode v-bind="nodeProps as any" />
            </template>

            <template #node-http-response="nodeProps">
                <BaseNode v-bind="nodeProps as any" />
            </template>

            <template #node-http-request="nodeProps">
                <BaseNode v-bind="nodeProps as any" />
            </template>

            <template #node-mqtt-in="nodeProps">
                <BaseNode v-bind="nodeProps as any" />
            </template>

            <template #node-mqtt-out="nodeProps">
                <BaseNode v-bind="nodeProps as any" />
            </template>

            <template #node-tcp-in="nodeProps">
                <BaseNode v-bind="nodeProps as any" />
            </template>

            <template #node-tcp-out="nodeProps">
                <BaseNode v-bind="nodeProps as any" />
            </template>

            <template #node-modbus-read="nodeProps">
                <BaseNode v-bind="nodeProps as any" />
            </template>

            <template #node-modbus-write="nodeProps">
                <BaseNode v-bind="nodeProps as any" />
            </template>

            <template #node-opc-ua="nodeProps">
                <BaseNode v-bind="nodeProps as any" />
            </template>

            <template #node-file-in="nodeProps">
                <BaseNode v-bind="nodeProps as any" />
            </template>

            <template #node-file-out="nodeProps">
                <BaseNode v-bind="nodeProps as any" />
            </template>

            <template #node-json="nodeProps">
                <BaseNode v-bind="nodeProps as any" />
            </template>

            <template #node-xml="nodeProps">
                <BaseNode v-bind="nodeProps as any" />
            </template>

            <template #node-csv="nodeProps">
                <BaseNode v-bind="nodeProps as any" />
            </template>

            <template #node-comment="nodeProps">
                <BaseNode v-bind="nodeProps as any" />
            </template>

            <template #node-link-in="nodeProps">
                <LinkNode v-bind="nodeProps as any" />
            </template>

            <template #node-link-out="nodeProps">
                <LinkNode v-bind="nodeProps as any" />
            </template>

            <template #node-link-call="nodeProps">
                <LinkCallNode v-bind="nodeProps as any" />
            </template>

            <template #node-catch="nodeProps">
                <BaseNode v-bind="nodeProps as any" />
            </template>

            <template #node-status="nodeProps">
                <BaseNode v-bind="nodeProps as any" />
            </template>

            <!-- Background grid -->
            <Background
                :variant="BackgroundVariant.Lines"
                :gap="24"
                :size="1"
                pattern-color="#30363d33"
            />

            <!-- Zoom / Fit controls -->
            <Controls position="bottom-left" />

            <!-- Mini map with zoom overlay -->
            <Panel position="bottom-right" class="!p-0">
                <div class="relative">
                    <MiniMap
                        :pannable="true"
                        :zoomable="true"
                        :width="160"
                        :height="100"
                        class="!relative !m-0"
                    />
                    <div class="absolute inset-0 flex items-center justify-center font-mono text-[10px] text-terminal-text-dim select-none pointer-events-none z-10">
                        {{ zoomPercent }}%
                    </div>
                </div>
            </Panel>
        </VueFlow>
    </div>
</template>

<style scoped>
.vue-flow {
    background-color: #0d1117;
}
</style>
