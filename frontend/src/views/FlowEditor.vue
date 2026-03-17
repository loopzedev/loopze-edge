<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { VueFlow, useVueFlow } from '@vue-flow/core'
import { Background, BackgroundVariant } from '@vue-flow/background'
import { Controls } from '@vue-flow/controls'
import { MiniMap } from '@vue-flow/minimap'
import { useFlowStore } from '@/stores/flowStore'
import { useUiStore } from '@/stores/uiStore'
import BaseNode from '@/components/nodes/BaseNode.vue'
import InjectNode from '@/components/nodes/InjectNode.vue'
import DebugNode from '@/components/nodes/DebugNode.vue'
import FunctionNode from '@/components/nodes/FunctionNode.vue'

import '@vue-flow/core/dist/style.css'
import '@vue-flow/core/dist/theme-default.css'
import '@vue-flow/controls/dist/style.css'
import '@vue-flow/minimap/dist/style.css'

const flowStore = useFlowStore()
const uiStore = useUiStore()

const { onConnect, onNodeDragStop, screenToFlowCoordinate } = useVueFlow({
  id: 'flint-flow-editor',
  defaultEdgeOptions: {
    type: 'smoothstep',
    animated: false,
  },
  fitViewOnInit: false,
  snapToGrid: true,
  snapGrid: [16, 16] as [number, number],
  deleteKeyCode: ['Backspace', 'Delete'],
  multiSelectionKeyCode: 'Shift',
  connectionLineType: 'smoothstep' as any,
  minZoom: 0.15,
  maxZoom: 3,
})

const flowContainer = ref<HTMLElement | null>(null)

onConnect((params) => {
  flowStore.connectNodes({
    source: params.source,
    target: params.target,
    sourceHandle: params.sourceHandle,
    targetHandle: params.targetHandle,
  })
})

onNodeDragStop((event) => {
  if (Array.isArray(event)) {
    for (const e of event) {
      flowStore.updateNodePosition(e.node.id, e.node.position)
    }
  } else {
    flowStore.updateNodePosition(event.node.id, event.node.position)
  }
})

function handleSelectionChange(params: { nodes: any[]; edges: any[] }): void {
  if (params.nodes.length === 1) {
    flowStore.selectNode(params.nodes[0].id)
    uiStore.openRightPanel('properties')
  } else if (params.nodes.length === 0) {
    flowStore.selectNode(null)
  }
}

function onDragOver(event: DragEvent): void {
  event.preventDefault()
  if (event.dataTransfer) {
    event.dataTransfer.dropEffect = 'move'
  }
}

function onDrop(event: DragEvent): void {
  event.preventDefault()

  if (!event.dataTransfer) return

  const rawData = event.dataTransfer.getData('application/flint-node')
  if (!rawData) return

  let nodeData: { type: string; label: string; inputs: number; outputs: number }
  try {
    nodeData = JSON.parse(rawData)
  } catch {
    return
  }

  if (!flowContainer.value) return

  const position = screenToFlowCoordinate({
    x: event.clientX,
    y: event.clientY,
  })

  flowStore.addNode(nodeData.type, position, {
    label: nodeData.label,
    inputs: nodeData.inputs,
    outputs: nodeData.outputs,
  })
}

function onPaneClick(): void {
  flowStore.selectNode(null)
}

onMounted(() => {
  if (flowStore.flows.length === 0) {
    flowStore.addFlow('Flow 1')
  }
})
</script>

<template>
  <div
    ref="flowContainer"
    class="w-full h-full bg-terminal-bg"
    @dragover="onDragOver"
    @drop="onDrop"
  >
    <VueFlow
      v-model:nodes="flowStore.nodes"
      v-model:edges="flowStore.edges"
      class="w-full h-full"
      @pane-click="onPaneClick"
      @selection-change="handleSelectionChange"
    >
      <!-- Custom Node Types -->
      <template #node-inject="nodeProps">
        <InjectNode v-bind="(nodeProps as any)" />
      </template>

      <template #node-debug="nodeProps">
        <DebugNode v-bind="(nodeProps as any)" />
      </template>

      <template #node-function="nodeProps">
        <FunctionNode v-bind="(nodeProps as any)" />
      </template>

      <!-- Default fallback for all other node types -->
      <template #node-default="nodeProps">
        <BaseNode v-bind="(nodeProps as any)" />
      </template>

      <template #node-change="nodeProps">
        <BaseNode v-bind="(nodeProps as any)" />
      </template>

      <template #node-switch="nodeProps">
        <BaseNode v-bind="(nodeProps as any)" />
      </template>

      <template #node-template="nodeProps">
        <BaseNode v-bind="(nodeProps as any)" />
      </template>

      <template #node-delay="nodeProps">
        <BaseNode v-bind="(nodeProps as any)" />
      </template>

      <template #node-filter="nodeProps">
        <BaseNode v-bind="(nodeProps as any)" />
      </template>

      <template #node-http-in="nodeProps">
        <BaseNode v-bind="(nodeProps as any)" />
      </template>

      <template #node-http-response="nodeProps">
        <BaseNode v-bind="(nodeProps as any)" />
      </template>

      <template #node-http-request="nodeProps">
        <BaseNode v-bind="(nodeProps as any)" />
      </template>

      <template #node-mqtt-in="nodeProps">
        <BaseNode v-bind="(nodeProps as any)" />
      </template>

      <template #node-mqtt-out="nodeProps">
        <BaseNode v-bind="(nodeProps as any)" />
      </template>

      <template #node-tcp-in="nodeProps">
        <BaseNode v-bind="(nodeProps as any)" />
      </template>

      <template #node-tcp-out="nodeProps">
        <BaseNode v-bind="(nodeProps as any)" />
      </template>

      <template #node-modbus-read="nodeProps">
        <BaseNode v-bind="(nodeProps as any)" />
      </template>

      <template #node-modbus-write="nodeProps">
        <BaseNode v-bind="(nodeProps as any)" />
      </template>

      <template #node-opc-ua="nodeProps">
        <BaseNode v-bind="(nodeProps as any)" />
      </template>

      <template #node-file-in="nodeProps">
        <BaseNode v-bind="(nodeProps as any)" />
      </template>

      <template #node-file-out="nodeProps">
        <BaseNode v-bind="(nodeProps as any)" />
      </template>

      <template #node-json="nodeProps">
        <BaseNode v-bind="(nodeProps as any)" />
      </template>

      <template #node-xml="nodeProps">
        <BaseNode v-bind="(nodeProps as any)" />
      </template>

      <template #node-csv="nodeProps">
        <BaseNode v-bind="(nodeProps as any)" />
      </template>

      <template #node-comment="nodeProps">
        <BaseNode v-bind="(nodeProps as any)" />
      </template>

      <template #node-link-in="nodeProps">
        <BaseNode v-bind="(nodeProps as any)" />
      </template>

      <template #node-link-out="nodeProps">
        <BaseNode v-bind="(nodeProps as any)" />
      </template>

      <template #node-catch="nodeProps">
        <BaseNode v-bind="(nodeProps as any)" />
      </template>

      <template #node-status="nodeProps">
        <BaseNode v-bind="(nodeProps as any)" />
      </template>

      <!-- Background grid -->
      <Background
        :variant="BackgroundVariant.Dots"
        :gap="24"
        :size="1"
        pattern-color="#3a3a2844"
      />

      <!-- Zoom / Fit controls -->
      <Controls position="bottom-left" />

      <!-- Mini map -->
      <MiniMap
        position="bottom-right"
        :pannable="true"
        :zoomable="true"
        :width="160"
        :height="100"
      />
    </VueFlow>
  </div>
</template>

<style scoped>
.vue-flow {
  background-color: #1a1a0e;
}
</style>
