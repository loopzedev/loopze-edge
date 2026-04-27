// Node design tokens — one palette per category.
// Every color is intentionally dark/muted to avoid eye strain.

export interface NodeTokens {
  accent:     string
  accentDim:  string
  accentBdr:  string
  accentGlow: string
  bg:         string
  bgHdr:      string
  bgIcon:     string
  border:     string
  textSub:    string
}

export const TOKENS: Record<string, NodeTokens> = {
  link: {
    accent:     '#9ca3af',
    accentDim:  '#9ca3af12',
    accentBdr:  '#9ca3af2a',
    accentGlow: '#9ca3af12',
    bg:         '#0a0a0b',
    bgHdr:      '#161618',
    bgIcon:     '#1c1c1f',
    border:     '#2e2e33',
    textSub:    '#71717a',
  },
  input: {
    accent:     '#4dff8f',
    accentDim:  '#4dff8f15',
    accentBdr:  '#4dff8f33',
    accentGlow: '#4dff8f14',
    bg:         '#080f09',
    bgHdr:      '#0b1f13',
    bgIcon:     '#0f2e1c',
    border:     '#1e4a26',
    textSub:    '#5aad62',
  },
  process: {
    accent:     '#38b6ff',
    accentDim:  '#38b6ff12',
    accentBdr:  '#38b6ff2a',
    accentGlow: '#38b6ff12',
    bg:         '#060c14',
    bgHdr:      '#0c1a2e',
    bgIcon:     '#101828',
    border:     '#1a3050',
    textSub:    '#5a8ab8',
  },
  context: {
    accent:     '#c084fc',
    accentDim:  '#c084fc12',
    accentBdr:  '#c084fc2a',
    accentGlow: '#c084fc12',
    bg:         '#0a0612',
    bgHdr:      '#160e28',
    bgIcon:     '#1a1030',
    border:     '#2e1a50',
    textSub:    '#9a6abf',
  },
  output: {
    accent:     '#ff6b2b',
    accentDim:  '#ff6b2b12',
    accentBdr:  '#ff6b2b2a',
    accentGlow: '#ff6b2b12',
    bg:         '#090503',
    bgHdr:      '#1a0e08',
    bgIcon:     '#150b06',
    border:     '#4a2010',
    textSub:    '#b8704a',
  },
  error: {
    accent:     '#ef4444',
    accentDim:  '#ef444412',
    accentBdr:  '#ef44442a',
    accentGlow: '#ef444414',
    bg:         '#0e0606',
    bgHdr:      '#1f0c0c',
    bgIcon:     '#2a0f0f',
    border:     '#4a1a1a',
    textSub:    '#b86060',
  },
  // Tech-specific palette: MQTT brand magenta-purple from the mqtt.org logo,
  // brightened so it reads on a dark surface.
  mqtt: {
    accent:     '#c026d3',
    accentDim:  '#c026d312',
    accentBdr:  '#c026d32a',
    accentGlow: '#c026d312',
    bg:         '#0c060e',
    bgHdr:      '#1a0a1c',
    bgIcon:     '#22122a',
    border:     '#3a1a40',
    textSub:    '#a85aaa',
  },
}

// Map node type → category
const TYPE_CATEGORY: Record<string, string> = {
  inject:          'input',
  'mqtt-in':       'mqtt',
  'http-in':       'input',
  'tcp-in':        'input',
  'udp-in':        'input',
  'modbus-read':   'input',
  'file-in':       'input',
  'link-in':       'link',
  catch:           'error',
  status:          'input',

  function:        'process',
  'function-expr': 'process',
  'function-go':   'process',
  change:          'process',
  switch:          'process',
  template:        'process',
  delay:           'process',
  filter:          'process',
  json:            'process',
  xml:             'process',
  csv:             'process',
  'link-call':     'link',
  comment:         'process',
  'opc-ua':        'process',

  'context-watch': 'context',

  statemachine:    'process',

  debug:           'output',
  'mqtt-out':      'mqtt',
  'http-response': 'output',
  'http-request':  'output',
  'tcp-out':       'output',
  'udp-out':       'output',
  'modbus-write':  'output',
  'file-out':      'output',
  'link-out':      'link',
}

export const STATUS_COLORS: Record<string, string> = {
  red: '#e24b4a',
  green: '#4ade80',
  yellow: '#ef9f27',
  blue: '#60a5fa',
  grey: '#6b7280',
}

export function getTokens(nodeType: string): NodeTokens {
  const cat = TYPE_CATEGORY[nodeType] ?? 'process'
  return TOKENS[cat]
}
