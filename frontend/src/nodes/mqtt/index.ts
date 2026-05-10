// MQTT group manifest. Mirrors internal/nodes/mqtt/init.go.

import type { Component } from 'vue'
import type { NodeGroupManifest } from '../types'

export const manifest: NodeGroupManifest = {
  name: 'mqtt',
  category: 'mqtt',
  flowEditors: {
    'mqtt-in':      () => import('./MqttNodeConfig.vue') as Promise<Component>,
    'mqtt-out':     () => import('./MqttNodeConfig.vue') as Promise<Component>,
    'mqtt-request': () => import('./MqttRequestConfig.vue') as Promise<Component>,
  },
  configEditors: {
    'mqtt-broker': () => import('./MqttBrokerConfig.vue') as Promise<Component>,
  },
}
