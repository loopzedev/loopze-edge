// Core group manifest. Mirrors internal/nodes/core/init.go.
//
// Each core node has its own palette colour, so we use per-type
// `categories` rather than the group-wide `category`. Code/canvas editors
// (function*, statemachine) need full panel height — listed in
// `fullHeightEditors` so the PropertyPanel skips its collapsible wrapper.

import type { Component } from 'vue'
import type { NodeGroupManifest } from '../types'

export const manifest: NodeGroupManifest = {
  name: 'core',
  categories: {
    inject:          'inject',
    debug:           'debug',
    catch:           'error',
    status:          'input',
    'context-watch': 'context',
    function:        'function',
    'function-expr': 'function',
    'function-go':   'function',
    statemachine:    'statemachine',
    change:          'change',
    switch:          'switch',
    delay:           'process',
    template:        'template',
    json:            'process',
    xml:             'process',
    'link-in':       'link',
    'link-out':      'link',
    'link-call':     'link',
  },
  flowEditors: {
    inject:          () => import('./InjectConfig.vue') as Promise<Component>,
    debug:           () => import('./DebugConfig.vue') as Promise<Component>,
    'context-watch': () => import('./ContextWatchConfig.vue') as Promise<Component>,
    catch:           () => import('./CatchConfig.vue') as Promise<Component>,
    status:          () => import('./StatusConfig.vue') as Promise<Component>,
    function:        () => import('./FunctionConfig.vue') as Promise<Component>,
    'function-expr': () => import('./ExprFunctionConfig.vue') as Promise<Component>,
    'function-go':   () => import('./GoFunctionConfig.vue') as Promise<Component>,
    statemachine:    () => import('./StateMachineConfig.vue') as Promise<Component>,
    change:          () => import('./ChangeConfig.vue') as Promise<Component>,
    switch:          () => import('./SwitchConfig.vue') as Promise<Component>,
    delay:           () => import('./DelayConfig.vue') as Promise<Component>,
    template:        () => import('./TemplateConfig.vue') as Promise<Component>,
    json:            () => import('./JSONParserConfig.vue') as Promise<Component>,
    xml:             () => import('./XMLParserConfig.vue') as Promise<Component>,
    'link-in':       () => import('./LinkConfig.vue') as Promise<Component>,
    'link-out':      () => import('./LinkConfig.vue') as Promise<Component>,
    'link-call':     () => import('./LinkConfig.vue') as Promise<Component>,
  },
  fullHeightEditors: ['function', 'function-expr', 'function-go', 'statemachine'],
}
