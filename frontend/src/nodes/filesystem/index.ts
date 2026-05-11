// Filesystem group manifest. Mirrors internal/nodes/filesystem/init.go.
//
// Phase 1: skeleton — minimal placeholder configs. Full UIs land in
// subsequent phases (file-out → file-in read → file-in watch → file-in
// incremental → folder-in).

import type { Component } from 'vue'
import type { NodeGroupManifest } from '../types'

export const manifest: NodeGroupManifest = {
  name: 'filesystem',
  category: 'filesystem',
  flowEditors: {
    'file-read':  () => import('./FileReadConfig.vue') as Promise<Component>,
    'file-watch': () => import('./FileWatchConfig.vue') as Promise<Component>,
    'file-out':   () => import('./FileOutConfig.vue') as Promise<Component>,
  },
}
