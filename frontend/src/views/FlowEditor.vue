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
import FunctionExprNode from "@/components/nodes/FunctionExprNode.vue";
import FunctionGoNode from "@/components/nodes/FunctionGoNode.vue";
import ContextWatchNode from "@/components/nodes/ContextWatchNode.vue";
import ChangeNode from "@/components/nodes/ChangeNode.vue";
import DelayNode from "@/components/nodes/DelayNode.vue";
import SwitchNode from "@/components/nodes/SwitchNode.vue";
import LinkNode from "@/components/nodes/LinkNode.vue";
import LinkCallNode from "@/components/nodes/LinkCallNode.vue";
import StateMachineNode from "@/components/nodes/StateMachineNode.vue";
import StatusNode from "@/components/nodes/StatusNode.vue";
import CatchNode from "@/components/nodes/CatchNode.vue";
import TemplateNode from "@/components/nodes/TemplateNode.vue";
import JSONParserNode from "@/components/nodes/JSONParserNode.vue";
import XMLParserNode from "@/components/nodes/XMLParserNode.vue";

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
    setCenter,
    findNode,
    getSelectedNodes,
    viewport,
} = useVueFlow("loopze-flow-editor");

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

    const rawData = event.dataTransfer.getData("application/loopze-node");
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
        case "e": {
            event.preventDefault();
            const selected = getSelectedNodes.value;
            if (selected.length === 0) break;
            const states = selected.map((n: any) => !!n.data?.disabled);
            const allSame = states.every((s: boolean) => s === states[0]);
            const next = allSame ? !states[0] : false;
            for (const n of selected) {
                if (!!n.data?.disabled !== next) {
                    flowStore.updateNodeData(n.id, { disabled: next });
                }
            }
            break;
        }
    }
}

// Reset viewport when switching flows
watch(() => flowStore.activeFlowId, async () => {
    await nextTick();
    setViewport({ x: 0, y: 0, zoom: 1 });
});

// Focus-request from outside (e.g. click on a node name in the debug panel):
// pan to the requested node. nextTick covers the case where activeFlowId
// changed in the same tick — Vue-Flow needs to mount the new flow first.
watch(
    () => flowStore.focusRequest,
    async (req) => {
        if (!req) return;
        await nextTick();
        const node = findNode(req.nodeId);
        if (!node) return;
        const w = node.dimensions?.width ?? 0;
        const h = node.dimensions?.height ?? 0;
        setCenter(node.position.x + w / 2, node.position.y + h / 2, {
            duration: 300,
        });
    },
);

onUnmounted(() => {
    document.removeEventListener("keydown", handleKeyDown);
});

onMounted(async () => {
    document.addEventListener("keydown", handleKeyDown);
    try {
        const [response] = await Promise.all([
            api.getFlows(),
            flowStore.loadNodeCatalog(),
        ]);
        if (response.flows && response.flows.length > 0) {
            flowStore.loadFlows(response.flows, response.rev, response.configs);
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
            id="loopze-flow-editor"
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

            <template #node-function-expr="nodeProps">
                <FunctionExprNode v-bind="nodeProps as any" />
            </template>

            <template #node-function-go="nodeProps">
                <FunctionGoNode v-bind="nodeProps as any" />
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
                <SwitchNode v-bind="nodeProps as any" />
            </template>

            <template #node-template="nodeProps">
                <TemplateNode v-bind="nodeProps as any" />
            </template>

            <template #node-delay="nodeProps">
                <DelayNode v-bind="nodeProps as any" />
            </template>

            <template #node-filter="nodeProps">
                <BaseNode v-bind="nodeProps as any" />
            </template>

            <template #node-http-in="nodeProps">
                <BaseNode
                    :id="nodeProps.id"
                    :label="nodeProps.data?.label"
                    node-type="http-in"
                    :selected="nodeProps.selected"
                    :inputs="0"
                    :outputs="1"
                    :status="nodeProps.data?.status"
                    :disabled="nodeProps.data?.disabled"
                >
                    <template #body>
                        <span class="truncate">{{
                            `${(nodeProps.data?.config?.method || 'GET')} ${nodeProps.data?.config?.path || '/'}`
                        }}</span>
                    </template>
                </BaseNode>
            </template>

            <template #node-http-response="nodeProps">
                <BaseNode
                    :id="nodeProps.id"
                    :label="nodeProps.data?.label"
                    node-type="http-response"
                    :selected="nodeProps.selected"
                    :inputs="1"
                    :outputs="0"
                    :status="nodeProps.data?.status"
                    :disabled="nodeProps.data?.disabled"
                >
                    <template #body>
                        <span class="truncate">{{ nodeProps.data?.config?.statusCode ?? 200 }}</span>
                    </template>
                </BaseNode>
            </template>

            <template #node-http-request="nodeProps">
                <BaseNode
                    :id="nodeProps.id"
                    :label="nodeProps.data?.label"
                    node-type="http-request"
                    :selected="nodeProps.selected"
                    :inputs="1"
                    :outputs="1"
                    :status="nodeProps.data?.status"
                    :disabled="nodeProps.data?.disabled"
                >
                    <template #body>
                        <span class="truncate">{{
                            `${(nodeProps.data?.config?.method || 'GET')} ${nodeProps.data?.config?.url || ''}`
                        }}</span>
                    </template>
                </BaseNode>
            </template>

            <template #node-mqtt-in="nodeProps">
                <BaseNode
                    :id="nodeProps.id"
                    :label="nodeProps.data?.label"
                    node-type="mqtt-in"
                    :selected="nodeProps.selected"
                    :inputs="nodeProps.data?.inputs ?? 0"
                    :outputs="1"
                    :status="nodeProps.data?.status"
                    :disabled="nodeProps.data?.disabled"
                >
                    <template #body>
                        <span class="truncate">{{
                            nodeProps.data?.config?.mode === 'dynamic'
                                ? 'dynamic'
                                : (nodeProps.data?.config?.topic || '')
                        }}</span>
                    </template>
                </BaseNode>
            </template>

            <template #node-mqtt-out="nodeProps">
                <BaseNode
                    :id="nodeProps.id"
                    :label="nodeProps.data?.label"
                    node-type="mqtt-out"
                    :selected="nodeProps.selected"
                    :inputs="1"
                    :outputs="0"
                    :status="nodeProps.data?.status"
                    :disabled="nodeProps.data?.disabled"
                >
                    <template #body>
                        <span class="truncate">{{
                            nodeProps.data?.config?.target === 'responseTopic'
                                ? '→ msg.responseTopic'
                                : (nodeProps.data?.config?.topic || '')
                        }}</span>
                    </template>
                </BaseNode>
            </template>

            <template #node-mqtt-request="nodeProps">
                <BaseNode
                    :id="nodeProps.id"
                    :label="nodeProps.data?.label"
                    node-type="mqtt-request"
                    :selected="nodeProps.selected"
                    :inputs="1"
                    :outputs="1"
                    :status="nodeProps.data?.status"
                    :disabled="nodeProps.data?.disabled"
                >
                    <template #body>
                        <span class="truncate">{{ nodeProps.data?.config?.topic || '' }}</span>
                    </template>
                </BaseNode>
            </template>

            <template #node-tcp-in="nodeProps">
                <BaseNode
                    :id="nodeProps.id"
                    :label="nodeProps.data?.label"
                    node-type="tcp-in"
                    :selected="nodeProps.selected"
                    :inputs="0"
                    :outputs="1"
                    :status="nodeProps.data?.status"
                    :disabled="nodeProps.data?.disabled"
                >
                    <template #body>
                        <span class="truncate">{{
                            `${nodeProps.data?.config?.mode === 'client' ? '→ ' : ':'}${nodeProps.data?.config?.host || '0.0.0.0'}:${nodeProps.data?.config?.port ?? ''}`
                        }}</span>
                    </template>
                </BaseNode>
            </template>

            <template #node-tcp-out="nodeProps">
                <BaseNode
                    :id="nodeProps.id"
                    :label="nodeProps.data?.label"
                    node-type="tcp-out"
                    :selected="nodeProps.selected"
                    :inputs="1"
                    :outputs="0"
                    :status="nodeProps.data?.status"
                    :disabled="nodeProps.data?.disabled"
                >
                    <template #body>
                        <span class="truncate">{{
                            nodeProps.data?.config?.mode === 'reply'
                                ? 'reply'
                                : nodeProps.data?.config?.mode === 'server-broadcast'
                                ? `broadcast → ${nodeProps.data?.config?.targetTcpIn || '?'}`
                                : `→ ${nodeProps.data?.config?.host || '?'}:${nodeProps.data?.config?.port ?? ''}`
                        }}</span>
                    </template>
                </BaseNode>
            </template>

            <template #node-tcp-request="nodeProps">
                <BaseNode
                    :id="nodeProps.id"
                    :label="nodeProps.data?.label"
                    node-type="tcp-request"
                    :selected="nodeProps.selected"
                    :inputs="1"
                    :outputs="1"
                    :status="nodeProps.data?.status"
                    :disabled="nodeProps.data?.disabled"
                >
                    <template #body>
                        <span class="truncate">{{
                            `${nodeProps.data?.config?.host || ''}${nodeProps.data?.config?.port ? ':' + nodeProps.data?.config?.port : ''}`
                        }}</span>
                    </template>
                </BaseNode>
            </template>

            <template #node-udp-in="nodeProps">
                <BaseNode
                    :id="nodeProps.id"
                    :label="nodeProps.data?.label"
                    node-type="udp-in"
                    :selected="nodeProps.selected"
                    :inputs="0"
                    :outputs="1"
                    :status="nodeProps.data?.status"
                    :disabled="nodeProps.data?.disabled"
                >
                    <template #body>
                        <span class="truncate">{{
                            `:${nodeProps.data?.config?.port ?? ''}${(nodeProps.data?.config?.multicastGroups?.length ?? 0) > 0 ? ' · mcast' : ''}`
                        }}</span>
                    </template>
                </BaseNode>
            </template>

            <template #node-udp-out="nodeProps">
                <BaseNode
                    :id="nodeProps.id"
                    :label="nodeProps.data?.label"
                    node-type="udp-out"
                    :selected="nodeProps.selected"
                    :inputs="1"
                    :outputs="0"
                    :status="nodeProps.data?.status"
                    :disabled="nodeProps.data?.disabled"
                >
                    <template #body>
                        <span class="truncate">{{
                            `${nodeProps.data?.config?.mode || 'unicast'} → ${nodeProps.data?.config?.host || '?'}:${nodeProps.data?.config?.port ?? ''}`
                        }}</span>
                    </template>
                </BaseNode>
            </template>

            <template #node-modbus-read="nodeProps">
                <BaseNode
                    :id="nodeProps.id"
                    :label="nodeProps.data?.label"
                    node-type="modbus-read"
                    :selected="nodeProps.selected"
                    :inputs="nodeProps.data?.inputs ?? 0"
                    :outputs="1"
                    :status="nodeProps.data?.status"
                    :disabled="nodeProps.data?.disabled"
                >
                    <template #body>
                        <span class="truncate">{{
                            nodeProps.data?.config?.mode === 'dynamic'
                                ? 'dynamic'
                                : `FC${nodeProps.data?.config?.fc ?? 3} @ ${nodeProps.data?.config?.address ?? 0}`
                        }}</span>
                    </template>
                </BaseNode>
            </template>

            <template #node-modbus-write="nodeProps">
                <BaseNode
                    :id="nodeProps.id"
                    :label="nodeProps.data?.label"
                    node-type="modbus-write"
                    :selected="nodeProps.selected"
                    :inputs="1"
                    :outputs="nodeProps.data?.outputs ?? 0"
                    :status="nodeProps.data?.status"
                    :disabled="nodeProps.data?.disabled"
                >
                    <template #body>
                        <span class="truncate">{{
                            `FC${nodeProps.data?.config?.fc ?? 16} @ ${nodeProps.data?.config?.address ?? 0}`
                        }}</span>
                    </template>
                </BaseNode>
            </template>

            <template #node-modbus-parser="nodeProps">
                <BaseNode
                    :id="nodeProps.id"
                    :label="nodeProps.data?.label"
                    node-type="modbus-parser"
                    :selected="nodeProps.selected"
                    :inputs="1"
                    :outputs="1"
                    :status="nodeProps.data?.status"
                    :disabled="nodeProps.data?.disabled"
                >
                    <template #body>
                        <span class="truncate">{{
                            `${(nodeProps.data?.config?.layout ?? []).length} field(s) · ${nodeProps.data?.config?.action ?? 'auto'}`
                        }}</span>
                    </template>
                </BaseNode>
            </template>

            <template #node-s7-read="nodeProps">
                <BaseNode
                    :id="nodeProps.id"
                    :label="nodeProps.data?.label"
                    node-type="s7-read"
                    :selected="nodeProps.selected"
                    :inputs="nodeProps.data?.inputs ?? 0"
                    :outputs="1"
                    :status="nodeProps.data?.status"
                    :disabled="nodeProps.data?.disabled"
                >
                    <template #body>
                        <span class="truncate">{{
                            nodeProps.data?.config?.mode === 'block'
                                ? `block ${nodeProps.data?.config?.block?.area ?? 'DB'}${nodeProps.data?.config?.block?.db ?? ''}@${nodeProps.data?.config?.block?.start ?? 0}`
                                : nodeProps.data?.config?.mode === 'dynamic'
                                    ? 'dynamic'
                                    : `${(nodeProps.data?.config?.variables ?? []).length} var(s) · ${nodeProps.data?.config?.pollInterval ?? 1000}ms`
                        }}</span>
                    </template>
                </BaseNode>
            </template>

            <template #node-s7-write="nodeProps">
                <BaseNode
                    :id="nodeProps.id"
                    :label="nodeProps.data?.label"
                    node-type="s7-write"
                    :selected="nodeProps.selected"
                    :inputs="1"
                    :outputs="nodeProps.data?.outputs ?? 0"
                    :status="nodeProps.data?.status"
                    :disabled="nodeProps.data?.disabled"
                >
                    <template #body>
                        <span class="truncate">{{
                            nodeProps.data?.config?.mode === 'block'
                                ? `block ${nodeProps.data?.config?.block?.area ?? 'DB'}${nodeProps.data?.config?.block?.db ?? ''}@${nodeProps.data?.config?.block?.start ?? 0}`
                                : nodeProps.data?.config?.mode === 'dynamic'
                                    ? 'dynamic'
                                    : `${(nodeProps.data?.config?.variables ?? []).length} var(s)`
                        }}</span>
                    </template>
                </BaseNode>
            </template>

            <template #node-s7-parser="nodeProps">
                <BaseNode
                    :id="nodeProps.id"
                    :label="nodeProps.data?.label"
                    node-type="s7-parser"
                    :selected="nodeProps.selected"
                    :inputs="1"
                    :outputs="1"
                    :status="nodeProps.data?.status"
                    :disabled="nodeProps.data?.disabled"
                >
                    <template #body>
                        <span class="truncate">{{
                            `${(nodeProps.data?.config?.layout ?? []).length} field(s) · ${nodeProps.data?.config?.action ?? 'auto'}`
                        }}</span>
                    </template>
                </BaseNode>
            </template>

            <template #node-opc-ua="nodeProps">
                <BaseNode v-bind="nodeProps as any" />
            </template>

            <template #node-opcua-read="nodeProps">
                <BaseNode
                    :id="nodeProps.id"
                    :label="nodeProps.data?.label"
                    node-type="opcua-read"
                    :selected="nodeProps.selected"
                    :inputs="nodeProps.data?.inputs ?? 1"
                    :outputs="1"
                    :status="nodeProps.data?.status"
                    :disabled="nodeProps.data?.disabled"
                >
                    <template #body>
                        <span class="truncate">{{
                            nodeProps.data?.config?.mode === 'dynamic'
                                ? 'dynamic'
                                : `${(nodeProps.data?.config?.nodeIds ?? []).length} node(s)`
                        }}</span>
                    </template>
                </BaseNode>
            </template>

            <template #node-opcua-subscribe="nodeProps">
                <BaseNode
                    :id="nodeProps.id"
                    :label="nodeProps.data?.label"
                    node-type="opcua-subscribe"
                    :selected="nodeProps.selected"
                    :inputs="nodeProps.data?.inputs ?? 0"
                    :outputs="1"
                    :status="nodeProps.data?.status"
                    :disabled="nodeProps.data?.disabled"
                >
                    <template #body>
                        <span class="truncate">{{
                            nodeProps.data?.config?.mode === 'dynamic'
                                ? 'dynamic'
                                : `${(nodeProps.data?.config?.monitoredItems ?? []).length} item(s)`
                        }}</span>
                    </template>
                </BaseNode>
            </template>

            <template #node-opcua-write="nodeProps">
                <BaseNode
                    :id="nodeProps.id"
                    :label="nodeProps.data?.label"
                    node-type="opcua-write"
                    :selected="nodeProps.selected"
                    :inputs="1"
                    :outputs="nodeProps.data?.outputs ?? 1"
                    :status="nodeProps.data?.status"
                    :disabled="nodeProps.data?.disabled"
                >
                    <template #body>
                        <span class="truncate">{{
                            nodeProps.data?.config?.mode === 'dynamic'
                                ? 'dynamic'
                                : `${(nodeProps.data?.config?.writes ?? []).length} write(s)`
                        }}</span>
                    </template>
                </BaseNode>
            </template>

            <template #node-file-in="nodeProps">
                <BaseNode v-bind="nodeProps as any" />
            </template>

            <template #node-file-out="nodeProps">
                <BaseNode v-bind="nodeProps as any" />
            </template>

            <template #node-folder-in="nodeProps">
                <BaseNode v-bind="nodeProps as any" />
            </template>

            <template #node-json="nodeProps">
                <JSONParserNode v-bind="nodeProps as any" />
            </template>

            <template #node-xml="nodeProps">
                <XMLParserNode v-bind="nodeProps as any" />
            </template>

            <template #node-csv="nodeProps">
                <BaseNode v-bind="nodeProps as any" />
            </template>

            <template #node-comment="nodeProps">
                <BaseNode v-bind="nodeProps as any" />
            </template>

            <template #node-statemachine="nodeProps">
                <StateMachineNode v-bind="nodeProps as any" />
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
                <CatchNode v-bind="nodeProps as any" />
            </template>

            <template #node-status="nodeProps">
                <StatusNode v-bind="nodeProps as any" />
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
