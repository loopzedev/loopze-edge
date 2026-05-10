// Network group manifest (HTTP / TCP / UDP). Mirrors
// internal/nodes/network/init.go.
//
// Each transport family has its own palette colour, so we use per-type
// `categories` rather than the group-wide `category`.

import type { Component } from 'vue'
import type { NodeGroupManifest } from '../types'

export const manifest: NodeGroupManifest = {
  name: 'network',
  categories: {
    'http-in':       'http',
    'http-response': 'http',
    'http-request':  'http',
    'tcp-in':        'tcp',
    'tcp-out':       'tcp',
    'tcp-request':   'tcp',
    'udp-in':        'udp',
    'udp-out':       'udp',
  },
  flowEditors: {
    'http-in':       () => import('./HttpInConfig.vue') as Promise<Component>,
    'http-response': () => import('./HttpResponseConfig.vue') as Promise<Component>,
    'http-request':  () => import('./HttpRequestConfig.vue') as Promise<Component>,
    'tcp-in':        () => import('./TcpInConfig.vue') as Promise<Component>,
    'tcp-out':       () => import('./TcpOutConfig.vue') as Promise<Component>,
    'tcp-request':   () => import('./TcpRequestConfig.vue') as Promise<Component>,
    'udp-in':        () => import('./UdpInConfig.vue') as Promise<Component>,
    'udp-out':       () => import('./UdpOutConfig.vue') as Promise<Component>,
  },
}
