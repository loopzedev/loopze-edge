// Dashboard group manifest. Mirrors internal/nodes/dashboard/init.go.
//
// Phase 1 ships ui-base / ui-page / ui-group as config-node editors
// (opened from the global "Dashboard" header button, the page editor in
// the flow tabs panel, and the group editor inside ui-page respectively)
// and ui-button as the lone flow-node widget.
//
// Display widgets (ui-text, ui-led, ui-gauge, ui-chart) land in PR 3/4.

import type { Component } from 'vue'
import type { NodeGroupManifest } from '../types'

export const manifest: NodeGroupManifest = {
  name: 'dashboard',
  categories: {
    'ui-button': 'dashboard-input',
    'ui-text':   'dashboard-display',
    'ui-led':    'dashboard-display',
    'ui-gauge':  'dashboard-display',
  },
  flowEditors: {
    'ui-button': () => import('./UIButtonConfig.vue') as Promise<Component>,
    'ui-text':   () => import('./UITextConfig.vue') as Promise<Component>,
    'ui-led':    () => import('./UILedConfig.vue') as Promise<Component>,
    'ui-gauge':  () => import('./UIGaugeConfig.vue') as Promise<Component>,
  },
  configEditors: {
    'ui-base':  () => import('./UIBaseConfig.vue') as Promise<Component>,
    'ui-page':  () => import('./UIPageConfig.vue') as Promise<Component>,
    'ui-group': () => import('./UIGroupConfig.vue') as Promise<Component>,
  },
}
