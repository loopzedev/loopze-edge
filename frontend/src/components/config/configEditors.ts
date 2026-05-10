import type { Component } from 'vue'

/**
 * Registry mapping config node types to their editor components.
 * Lazy-loaded to keep bundle size small.
 *
 * To add a new config type, add one line here + create the Vue component.
 * No other files need to be modified.
 */
const CONFIG_EDITORS: Record<string, () => Promise<Component>> = {
  'mqtt-broker': () => import('./MqttBrokerConfig.vue') as Promise<Component>,
  'modbus-server': () => import('./ModbusServerConfig.vue') as Promise<Component>,
  'opcua-server': () => import('./OpcuaServerConfig.vue') as Promise<Component>,
  's7-plc': () => import('./S7PlcConfig.vue') as Promise<Component>,
}

export function getConfigEditor(configType: string): (() => Promise<Component>) | undefined {
  return CONFIG_EDITORS[configType]
}

export function hasConfigEditor(configType: string): boolean {
  return configType in CONFIG_EDITORS
}
