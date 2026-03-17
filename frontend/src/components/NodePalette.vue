<script setup lang="ts">
import { ref } from 'vue'

interface PaletteNode {
  type: string
  label: string
  inputs: number
  outputs: number
}

interface PaletteCategory {
  name: string
  label: string
  expanded: boolean
  nodes: PaletteNode[]
}

const categories = ref<PaletteCategory[]>([
  {
    name: 'common',
    label: 'Common',
    expanded: true,
    nodes: [
      { type: 'inject', label: 'Inject', inputs: 0, outputs: 1 },
      { type: 'debug', label: 'Debug', inputs: 1, outputs: 0 },
      { type: 'comment', label: 'Comment', inputs: 0, outputs: 0 },
      { type: 'link-in', label: 'Link In', inputs: 0, outputs: 1 },
      { type: 'link-out', label: 'Link Out', inputs: 1, outputs: 0 },
      { type: 'catch', label: 'Catch', inputs: 0, outputs: 1 },
      { type: 'status', label: 'Status', inputs: 0, outputs: 1 },
    ],
  },
  {
    name: 'function',
    label: 'Function',
    expanded: false,
    nodes: [
      { type: 'function', label: 'Function', inputs: 1, outputs: 1 },
      { type: 'change', label: 'Change', inputs: 1, outputs: 1 },
      { type: 'switch', label: 'Switch', inputs: 1, outputs: 2 },
      { type: 'template', label: 'Template', inputs: 1, outputs: 1 },
      { type: 'delay', label: 'Delay', inputs: 1, outputs: 1 },
      { type: 'filter', label: 'Filter', inputs: 1, outputs: 1 },
    ],
  },
  {
    name: 'network',
    label: 'Network',
    expanded: false,
    nodes: [
      { type: 'http-in', label: 'HTTP In', inputs: 0, outputs: 1 },
      { type: 'http-response', label: 'HTTP Response', inputs: 1, outputs: 0 },
      { type: 'http-request', label: 'HTTP Request', inputs: 1, outputs: 1 },
      { type: 'mqtt-in', label: 'MQTT In', inputs: 0, outputs: 1 },
      { type: 'mqtt-out', label: 'MQTT Out', inputs: 1, outputs: 0 },
      { type: 'tcp-in', label: 'TCP In', inputs: 0, outputs: 1 },
      { type: 'tcp-out', label: 'TCP Out', inputs: 1, outputs: 0 },
    ],
  },
  {
    name: 'industrial',
    label: 'Industrial',
    expanded: false,
    nodes: [
      { type: 'modbus-read', label: 'Modbus Read', inputs: 1, outputs: 1 },
      { type: 'modbus-write', label: 'Modbus Write', inputs: 1, outputs: 1 },
      { type: 'opc-ua', label: 'OPC-UA', inputs: 1, outputs: 1 },
    ],
  },
  {
    name: 'storage',
    label: 'Storage',
    expanded: false,
    nodes: [
      { type: 'file-in', label: 'File In', inputs: 1, outputs: 1 },
      { type: 'file-out', label: 'File Out', inputs: 1, outputs: 0 },
    ],
  },
  {
    name: 'parser',
    label: 'Parser',
    expanded: false,
    nodes: [
      { type: 'json', label: 'JSON', inputs: 1, outputs: 1 },
      { type: 'xml', label: 'XML', inputs: 1, outputs: 1 },
      { type: 'csv', label: 'CSV', inputs: 1, outputs: 1 },
    ],
  },
])

function toggleCategory(category: PaletteCategory): void {
  category.expanded = !category.expanded
}

function onDragStart(event: DragEvent, node: PaletteNode): void {
  if (!event.dataTransfer) return

  event.dataTransfer.setData(
    'application/flint-node',
    JSON.stringify({
      type: node.type,
      label: node.label,
      inputs: node.inputs,
      outputs: node.outputs,
    })
  )
  event.dataTransfer.effectAllowed = 'move'
}

function getNodeTypeIcon(type: string): string {
  const icons: Record<string, string> = {
    inject: '⏱',
    debug: '⬤',
    comment: '✎',
    'link-in': '←',
    'link-out': '→',
    catch: '⚡',
    status: '◈',
    function: 'ƒ',
    change: '⇄',
    switch: '⑂',
    template: '▤',
    delay: '⏲',
    filter: '⧩',
    'http-in': '▶',
    'http-response': '◀',
    'http-request': '⇆',
    'mqtt-in': '▼',
    'mqtt-out': '▲',
    'tcp-in': '⇊',
    'tcp-out': '⇈',
    'modbus-read': '⎍',
    'modbus-write': '⎌',
    'opc-ua': '⚙',
    'file-in': '⤓',
    'file-out': '⤒',
    json: '{}',
    xml: '⟨⟩',
    csv: '⊞',
  }
  return icons[type] ?? '●'
}
</script>

<template>
  <aside class="h-full w-[220px] bg-terminal-surface border-r border-terminal-border flex flex-col overflow-hidden select-none">
    <!-- Panel Header -->
    <div class="flex items-center px-3 py-2 border-b border-terminal-border shrink-0">
      <span class="text-terminal-text text-xs font-bold uppercase tracking-widest">Nodes</span>
    </div>

    <!-- Search (optional filter area) -->
    <div class="px-2 py-1.5 border-b border-terminal-border shrink-0">
      <input
        type="text"
        placeholder="Filter nodes..."
        class="terminal-input w-full text-xs py-1"
      />
    </div>

    <!-- Categories List -->
    <div class="flex-1 overflow-y-auto">
      <div
        v-for="category in categories"
        :key="category.name"
        class="border-b border-terminal-border"
      >
        <!-- Category Header -->
        <button
          class="w-full flex items-center justify-between px-3 py-1.5 text-xs font-bold uppercase tracking-wider text-terminal-text-dim hover:text-terminal-text hover:bg-terminal-border/30 transition-colors cursor-pointer"
          @click="toggleCategory(category)"
        >
          <span>{{ category.label }}</span>
          <span
            class="text-[10px] transition-transform duration-150"
            :class="category.expanded ? 'rotate-0' : '-rotate-90'"
          >
            ▼
          </span>
        </button>

        <!-- Category Nodes -->
        <div
          v-show="category.expanded"
          class="pb-1"
        >
          <div
            v-for="node in category.nodes"
            :key="node.type"
            draggable="true"
            class="flex items-center gap-2 mx-1 px-2 py-1 cursor-grab text-xs text-terminal-text hover:bg-terminal-border/40 active:cursor-grabbing transition-colors group"
            @dragstart="onDragStart($event, node)"
          >
            <!-- Node Icon -->
            <span class="w-5 h-5 flex items-center justify-center border border-terminal-border bg-terminal-bg text-[10px] text-terminal-text-dim group-hover:border-amber group-hover:text-terminal-text shrink-0 transition-colors">
              {{ getNodeTypeIcon(node.type) }}
            </span>

            <!-- Node Label -->
            <span class="truncate">{{ node.label }}</span>

            <!-- Port Indicators -->
            <span class="ml-auto flex items-center gap-0.5 text-[9px] text-terminal-text-dim">
              <span v-if="node.inputs > 0" title="inputs">▸{{ node.inputs }}</span>
              <span v-if="node.outputs > 0" title="outputs">{{ node.outputs }}▸</span>
            </span>
          </div>
        </div>
      </div>
    </div>

    <!-- Footer -->
    <div class="px-3 py-1.5 border-t border-terminal-border text-[10px] text-terminal-text-dim shrink-0">
      {{ categories.reduce((sum, c) => sum + c.nodes.length, 0) }} nodes available
    </div>
  </aside>
</template>
