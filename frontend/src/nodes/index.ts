// Aggregator for all frontend node groups.
//
// To add a new group:
//   1. Create frontend/src/nodes/<group>/ with the editor components
//   2. Export a NodeGroupManifest from <group>/index.ts
//   3. Add an import line to GROUPS below
//
// The PropertyPanel and BaseNode read the registries built here — no other
// touchpoints are needed when a contributor adds a node group.

import type { Component } from 'vue'
import type { NodeGroupManifest } from './types'

import { manifest as coreManifest } from './core'
import { manifest as filesystemManifest } from './filesystem'
import { manifest as modbusManifest } from './modbus'
import { manifest as mqttManifest } from './mqtt'
import { manifest as networkManifest } from './network'
import { manifest as opcuaManifest } from './opcua'
import { manifest as s7Manifest } from './s7'

export const GROUPS: NodeGroupManifest[] = [
  coreManifest,
  filesystemManifest,
  modbusManifest,
  mqttManifest,
  networkManifest,
  opcuaManifest,
  s7Manifest,
]

export interface FlowNodeEditorEntry {
  loader: () => Promise<Component>
  fullHeight: boolean
}

const FLOW_EDITORS: Record<string, FlowNodeEditorEntry> = {}
const CONFIG_EDITORS: Record<string, () => Promise<Component>> = {}
const TYPE_CATEGORY: Record<string, string> = {}

for (const group of GROUPS) {
  const fullHeightSet = new Set(group.fullHeightEditors ?? [])

  for (const [type, loader] of Object.entries(group.flowEditors)) {
    FLOW_EDITORS[type] = { loader, fullHeight: fullHeightSet.has(type) }
    TYPE_CATEGORY[type] = group.categories?.[type] ?? group.category ?? 'process'
  }
  for (const [type, loader] of Object.entries(group.configEditors ?? {})) {
    CONFIG_EDITORS[type] = loader
  }
}

/** Returns the editor entry for a flow-node type (or undefined). */
export function getFlowNodeEditor(nodeType: string): FlowNodeEditorEntry | undefined {
  return FLOW_EDITORS[nodeType]
}

/** Returns the lazy loader for a config-node type (or undefined). */
export function getConfigNodeEditor(configType: string): (() => Promise<Component>) | undefined {
  return CONFIG_EDITORS[configType]
}

/** Returns the palette category key for a node type (defaults to "process"). */
export function getCategory(nodeType: string): string {
  return TYPE_CATEGORY[nodeType] ?? 'process'
}
