// Modbus group manifest. Mirrors internal/nodes/modbus/init.go.

import type { Component } from 'vue'
import type { NodeGroupManifest } from '../types'

export const manifest: NodeGroupManifest = {
  name: 'modbus',
  category: 'rust',
  flowEditors: {
    'modbus-read':   () => import('./ModbusNodeConfig.vue') as Promise<Component>,
    'modbus-write':  () => import('./ModbusNodeConfig.vue') as Promise<Component>,
    'modbus-parser': () => import('./ModbusParserConfig.vue') as Promise<Component>,
  },
  configEditors: {
    'modbus-server': () => import('./ModbusServerConfig.vue') as Promise<Component>,
  },
}
