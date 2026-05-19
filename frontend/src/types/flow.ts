/** Core flow types matching the Go backend models */

export type NodeCategory =
  | 'common'
  | 'function'
  | 'network'
  | 'industrial'
  | 'storage'
  | 'parser'

export type NodeType =
  | 'inject'
  | 'debug'
  | 'function'
  | 'http-in'
  | 'http-response'
  | 'http-request'
  | 'mqtt-in'
  | 'mqtt-out'
  | 'mqtt-request'
  | 'tcp-in'
  | 'tcp-out'
  | 'tcp-request'
  | 'udp-in'
  | 'udp-out'
  | 'modbus-read'
  | 'modbus-write'
  | 's7-read'
  | 's7-write'
  | 's7-parser'
  | 'opc-ua'
  | 'change'
  | 'switch'
  | 'template'
  | 'delay'
  | 'filter'
  | 'json'
  | 'xml'
  | 'csv'
  | 'csv-out'
  | 'file-read'
  | 'file-out'
  | 'file-watch'
  | 'catch'
  | 'status'
  | 'link-in'
  | 'link-out'
  | 'link-call'
  | 'comment'
  | 'statemachine'
  | string

export interface Port {
  name: string
  label?: string
  type?: string
}

export interface Wire {
  id: string
  sourceNode: string
  sourcePort: number
  targetNode: string
  targetPort: number
}

export interface NodeConfig {
  [key: string]: unknown
}

export interface NodeStatus {
  fill?: 'red' | 'green' | 'yellow' | 'blue' | 'grey'
  shape?: 'ring' | 'dot'
  text?: string
}

export interface Node {
  id: string
  type: NodeType
  name: string
  label?: string
  category?: NodeCategory
  x: number
  y: number
  z: string // flow id this node belongs to
  inputs: number
  outputs: number
  inputLabels?: string[]
  outputLabels?: string[]
  wires: string[][] // wires[outputIndex] = [targetNodeId, ...]
  config: NodeConfig
  status?: NodeStatus
  disabled?: boolean
  info?: string
}

export interface Flow {
  id: string
  type: 'tab'
  label: string
  disabled?: boolean
  info?: string
  nodes: Node[]
  wires: Wire[]
}

/** A config node is a workspace-global configuration entity (e.g. MQTT broker, DB connection). */
export interface ConfigNode {
  id: string
  type: string
  name?: string
  config: Record<string, unknown>
}

export interface Message {
  _msgid: string
  topic?: string
  payload: unknown
  [key: string]: unknown
}

export interface NodeDefinition {
  type: NodeType
  category: NodeCategory
  label: string
  icon?: string
  color?: string
  inputs: number
  outputs: number
  defaults: Record<string, NodePropertyDefault>
  paletteLabel?: string
  info?: string
}

export interface NodePropertyDefault {
  value: unknown
  required?: boolean
  type?: string
  validate?: string
}

export type DeployModeType = 'nodes' | 'flows' | 'full' | 'restart'

export interface DeployPayload {
  flows: Flow[]
  configs?: ConfigNode[]
  rev?: string
  deployMode?: DeployModeType
}

export interface DeployResponse {
  rev: string
  flows: Flow[]
  configs?: ConfigNode[]
  success: boolean
  error?: string
}

export interface FlowsResponse {
  rev: string
  flows: Flow[]
  configs?: ConfigNode[]
}

export interface NodeCatalogEntry {
  type: NodeType
  category: NodeCategory
  label: string
  description?: string
  icon?: string
  color?: string
  inputs: number
  outputs: number
  defaults?: Record<string, any>
}

export interface NodeCatalog {
  [category: string]: NodeCatalogEntry[]
}
