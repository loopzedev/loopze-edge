// SIEMENS S7 group manifest. Mirrors the backend group registered in
// internal/nodes/s7/init.go.

import type { Component } from 'vue'
import type { NodeGroupManifest } from '../types'

export const manifest: NodeGroupManifest = {
  name: 's7',
  category: 'rust',
  flowEditors: {
    's7-read':   () => import('./S7NodeConfig.vue') as Promise<Component>,
    's7-write':  () => import('./S7NodeConfig.vue') as Promise<Component>,
    's7-parser': () => import('./S7ParserConfig.vue') as Promise<Component>,
  },
  configEditors: {
    's7-plc': () => import('./S7PlcConfig.vue') as Promise<Component>,
  },
}
