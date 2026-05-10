// OPC UA group manifest. Mirrors internal/nodes/opcua/init.go.

import type { Component } from 'vue'
import type { NodeGroupManifest } from '../types'

export const manifest: NodeGroupManifest = {
  name: 'opcua',
  category: 'opcua',
  flowEditors: {
    'opcua-read':      () => import('./OpcuaReadConfig.vue') as Promise<Component>,
    'opcua-write':     () => import('./OpcuaWriteConfig.vue') as Promise<Component>,
    'opcua-subscribe': () => import('./OpcuaSubscribeConfig.vue') as Promise<Component>,
  },
  configEditors: {
    'opcua-server': () => import('./OpcuaServerConfig.vue') as Promise<Component>,
  },
}
