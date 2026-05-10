import type { Component } from 'vue'

/**
 * Registry mapping flow node types to their editor components.
 * Lazy-loaded to keep bundle size small.
 *
 * To add a new node type, add one or two lines here + create the Vue component.
 * No other files need to be modified.
 *
 * fullHeight: true — bypasses the collapsible SectionHeader wrapper; use for
 * editors that need the full panel height (code editors, canvas-like layouts).
 */
export interface NodeEditorEntry {
  loader: () => Promise<Component>
  fullHeight?: boolean
}

const NODE_EDITORS: Record<string, NodeEditorEntry> = {
  // ── Core ────────────────────────────────────────────────────────────────
  inject:          { loader: () => import('./InjectConfig.vue') as Promise<Component> },
  debug:           { loader: () => import('./DebugConfig.vue') as Promise<Component> },
  'context-watch': { loader: () => import('./ContextWatchConfig.vue') as Promise<Component> },
  catch:           { loader: () => import('./CatchConfig.vue') as Promise<Component> },
  status:          { loader: () => import('./StatusConfig.vue') as Promise<Component> },

  // ── Transformation / Logic ───────────────────────────────────────────────
  function:        { loader: () => import('./FunctionConfig.vue') as Promise<Component>,     fullHeight: true },
  'function-expr': { loader: () => import('./ExprFunctionConfig.vue') as Promise<Component>, fullHeight: true },
  'function-go':   { loader: () => import('./GoFunctionConfig.vue') as Promise<Component>,   fullHeight: true },
  statemachine:    { loader: () => import('./StateMachineConfig.vue') as Promise<Component>, fullHeight: true },
  change:          { loader: () => import('./ChangeConfig.vue') as Promise<Component> },
  switch:          { loader: () => import('./SwitchConfig.vue') as Promise<Component> },
  delay:           { loader: () => import('./DelayConfig.vue') as Promise<Component> },
  template:        { loader: () => import('./TemplateConfig.vue') as Promise<Component> },
  json:            { loader: () => import('./JSONParserConfig.vue') as Promise<Component> },

  // ── Link ────────────────────────────────────────────────────────────────
  'link-in':       { loader: () => import('./LinkConfig.vue') as Promise<Component> },
  'link-out':      { loader: () => import('./LinkConfig.vue') as Promise<Component> },
  'link-call':     { loader: () => import('./LinkConfig.vue') as Promise<Component> },

  // ── MQTT ────────────────────────────────────────────────────────────────
  'mqtt-in':       { loader: () => import('./MqttNodeConfig.vue') as Promise<Component> },
  'mqtt-out':      { loader: () => import('./MqttNodeConfig.vue') as Promise<Component> },
  'mqtt-request':  { loader: () => import('./MqttRequestConfig.vue') as Promise<Component> },

  // ── Modbus ──────────────────────────────────────────────────────────────
  'modbus-read':   { loader: () => import('./ModbusNodeConfig.vue') as Promise<Component> },
  'modbus-write':  { loader: () => import('./ModbusNodeConfig.vue') as Promise<Component> },
  'modbus-parser': { loader: () => import('./ModbusParserConfig.vue') as Promise<Component> },

  // ── Siemens S7 ──────────────────────────────────────────────────────────
  's7-read':       { loader: () => import('./S7NodeConfig.vue') as Promise<Component> },
  's7-write':      { loader: () => import('./S7NodeConfig.vue') as Promise<Component> },
  's7-parser':     { loader: () => import('./S7ParserConfig.vue') as Promise<Component> },

  // ── OPC UA ──────────────────────────────────────────────────────────────
  'opcua-read':       { loader: () => import('./OpcuaReadConfig.vue') as Promise<Component> },
  'opcua-write':      { loader: () => import('./OpcuaWriteConfig.vue') as Promise<Component> },
  'opcua-subscribe':  { loader: () => import('./OpcuaSubscribeConfig.vue') as Promise<Component> },

  // ── HTTP ────────────────────────────────────────────────────────────────
  'http-in':       { loader: () => import('./HttpInConfig.vue') as Promise<Component> },
  'http-response': { loader: () => import('./HttpResponseConfig.vue') as Promise<Component> },
  'http-request':  { loader: () => import('./HttpRequestConfig.vue') as Promise<Component> },

  // ── TCP ─────────────────────────────────────────────────────────────────
  'tcp-in':        { loader: () => import('./TcpInConfig.vue') as Promise<Component> },
  'tcp-out':       { loader: () => import('./TcpOutConfig.vue') as Promise<Component> },
  'tcp-request':   { loader: () => import('./TcpRequestConfig.vue') as Promise<Component> },

  // ── UDP ─────────────────────────────────────────────────────────────────
  'udp-in':        { loader: () => import('./UdpInConfig.vue') as Promise<Component> },
  'udp-out':       { loader: () => import('./UdpOutConfig.vue') as Promise<Component> },
}

export function getNodeEditor(nodeType: string): NodeEditorEntry | undefined {
  return NODE_EDITORS[nodeType]
}
