<script setup lang="ts">
import { computed, defineComponent, h, ref } from 'vue'
import type { PropType, VNode } from 'vue'
import NodeIcon from '@/components/nodes/NodeIcon.vue'
import { getTokens, STATUS_COLORS } from '@/components/nodes/tokens'
import type { NodeTokens } from '@/components/nodes/tokens'

interface SampleNode {
  id: string
  nodeType: string
  typeLabel: string
  label: string
  inputs: number
  outputs: number
  status: { fill: 'red' | 'green' | 'yellow' | 'blue' | 'grey'; text: string }
  actionButton?: { label: string }
  toggleButton?: { state: boolean }
}

const samples: SampleNode[] = [
  { id: 'a', nodeType: 'mqtt-in',       typeLabel: 'MQTT Subscribe', label: 'sensors/temp/+', inputs: 0, outputs: 1, status: { fill: 'green',  text: 'connected' } },
  { id: 'b', nodeType: 'function',      typeLabel: 'Function',       label: 'normalize',      inputs: 1, outputs: 2, status: { fill: 'blue',   text: 'idle' } },
  { id: 'c', nodeType: 'context-watch', typeLabel: 'Context Watch',  label: 'cache.users',    inputs: 0, outputs: 1, status: { fill: 'yellow', text: 'watching' } },
  { id: 'd', nodeType: 'debug',         typeLabel: 'Debug',          label: 'msg.payload',    inputs: 1, outputs: 0, status: { fill: 'red',    text: 'error: null' } },
  { id: 'e', nodeType: 'inject',        typeLabel: 'Inject',         label: 'every 5s',       inputs: 0, outputs: 1, status: { fill: 'green',  text: 'armed' },     actionButton: { label: 'TRIG' } },
  { id: 'f', nodeType: 'debug',         typeLabel: 'Debug',          label: 'msg.payload',    inputs: 1, outputs: 0, status: { fill: 'blue',   text: '12 msgs' },   toggleButton: { state: true } },
]

const variants = [
  { id: 1,  name: 'Minimal Flat',        desc: 'Borderless, einheitliche Surface, 2 px Akzent oben.' },
  { id: 2,  name: 'Soft Card',           desc: 'Soft-Shadow Card, Icon in akzentfarbener Tile.' },
  { id: 3,  name: 'Bottom Accent',       desc: 'Akzent-Linie unterhalb des Bodies — wie ein Footer-Strich.' },
  { id: 4,  name: 'Left Accent Hairline',desc: 'Sehr dünner 2 px Akzent-Strich links, sonst minimal.' },
  { id: 5,  name: 'Icon Side Tile',      desc: 'Icon-Tile in Akzentton füllt die linke Spalte komplett.' },
  { id: 6,  name: 'Outlined',            desc: 'Akzent-Border statt Schatten — strukturierter, ohne Tiefe.' },
  { id: 7,  name: 'Tinted Surface',      desc: 'Sehr dezenter Akzent-Tint im Hintergrund, kein Border.' },
  { id: 8,  name: 'Header Tint',         desc: 'Header bekommt subtilen Akzent-Background, Body neutral.' },
  { id: 9,  name: 'Floating',            desc: 'Reine Schatten-Card, kein Border. Maximale Ruhe.' },
  { id: 10, name: 'Underline Type',      desc: 'Type-Label mit Akzent-Underline — typografische Hierarchie.' },
  { id: 11, name: 'Quiet Mono',          desc: 'Fast farblos, Akzent nur als Mikro-Marker. Sehr leise.' },
  { id: 12, name: 'Stacked Center',      desc: 'Type oben, Icon mittig groß, Label unten — vertikale Symmetrie.' },
]

const selectedId = ref<number | null>(null)
const showAll = computed(() => selectedId.value === null)
function pick(id: number) {
  selectedId.value = selectedId.value === id ? null : id
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────
function port(side: 'left' | 'right', i: number, total: number, color: string) {
  const top = ((i + 1) / (total + 1)) * 100
  return h('span', {
    class: 'absolute w-2.5 h-2.5 rounded-full border-2',
    style: {
      [side]: '-5px',
      top: `${top}%`,
      transform: 'translateY(-50%)',
      borderColor: color,
      background: '#0d1117',
    },
  })
}
function ports(node: SampleNode, color: string) {
  const ins  = Array.from({ length: node.inputs  }, (_, i) => port('left',  i, node.inputs,  color))
  const outs = Array.from({ length: node.outputs }, (_, i) => port('right', i, node.outputs, color))
  return [...ins, ...outs]
}

const nodeProp = { node: { type: Object as PropType<SampleNode>, required: true as const } }

// ─────────────────────────────────────────────────────────────────────────────
// Status pill — uniform under every node
// ─────────────────────────────────────────────────────────────────────────────
function statusPill(node: SampleNode): VNode {
  const s = node.status
  const c = STATUS_COLORS[s.fill]
  return h('div', {
    class: 'inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-[10px] ml-2',
    style: {
      background: c + '22',
      color: c,
      border: `1px solid ${c}55`,
      whiteSpace: 'nowrap',
    },
  }, [
    h('span', {
      class: 'w-1.5 h-1.5 rounded-full',
      style: { background: c, boxShadow: `0 0 4px ${c}` },
    }),
    h('span', s.text),
  ])
}

// ─────────────────────────────────────────────────────────────────────────────
// Button styles — staying inside the minimal/soft family
// ─────────────────────────────────────────────────────────────────────────────
type ActionFn = (t: NodeTokens, label: string) => VNode
type ToggleFn = (t: NodeTokens, state: boolean) => VNode
interface BtnSet { action: ActionFn; toggle: ToggleFn }

function vText(text: string, style: Record<string, string>) {
  return h('span', {
    class: 'text-[9px] font-bold tracking-widest',
    style: { writingMode: 'vertical-lr', textOrientation: 'mixed', ...style },
  }, text)
}

// rounded — 6 px, soft surface, accent on toggle. Default for most variants.
const rounded: BtnSet = {
  action: (t, label) => h('button', {
    class: 'shrink-0 w-9 flex items-center justify-center',
    style: {
      background: '#161b22', border: `1px solid ${t.border}`, borderRight: 'none',
      borderRadius: '6px 0 0 6px',
    },
  }, [vText(label, { color: t.textSub })]),
  toggle: (t, state) => h('button', {
    class: 'shrink-0 w-9 flex items-center justify-center',
    style: {
      background: state ? t.accent + '22' : '#161b22',
      border: `1px solid ${state ? t.accent : t.border}`, borderLeft: 'none',
      borderRadius: '0 6px 6px 0',
    },
  }, [vText(state ? 'ON' : 'OFF', { color: state ? t.accent : t.textSub })]),
}

// ghost — borderless, sits beside the node with a small gap. For Quiet/Floating.
function ghostBtn(side: 'left' | 'right', t: NodeTokens, label: string, active: boolean): VNode {
  return h('button', {
    class: 'shrink-0 w-9 self-stretch flex items-center justify-center rounded-md',
    style: {
      background: active ? t.accent + '18' : 'transparent',
      color: active ? t.accent : t.textSub,
      [side === 'left' ? 'marginRight' : 'marginLeft']: '6px',
    },
  }, [h('span', { class: 'text-[9px] font-bold tracking-widest', style: { writingMode: 'vertical-lr', textOrientation: 'mixed' } }, label)])
}
const ghost: BtnSet = {
  action: (t, label) => ghostBtn('left', t, label, false),
  toggle: (t, state) => ghostBtn('right', t, state ? 'ON' : 'OFF', state),
}

// pill — soft circular, for Stacked Center
function pillBtn(side: 'left' | 'right', t: NodeTokens, label: string, active: boolean): VNode {
  return h('button', {
    class: 'shrink-0 w-10 h-10 rounded-full flex items-center justify-center self-center text-[9px] font-bold',
    style: {
      background: active ? t.accent + '22' : '#161b22',
      border: `1px solid ${active ? t.accent : t.border}`,
      color: active ? t.accent : t.textSub,
      [side === 'left' ? 'marginRight' : 'marginLeft']: '8px',
    },
  }, label)
}
const pill: BtnSet = {
  action: (t, label) => pillBtn('left', t, label, false),
  toggle: (t, state) => pillBtn('right', t, state ? 'ON' : 'OFF', state),
}

// outline — matches the Outlined variant
const outline: BtnSet = {
  action: (t, label) => h('button', {
    class: 'shrink-0 w-9 flex items-center justify-center',
    style: {
      background: 'transparent', border: `1px solid ${t.accent}55`, borderRight: 'none',
      borderRadius: '6px 0 0 6px',
    },
  }, [vText(label, { color: t.accent })]),
  toggle: (t, state) => h('button', {
    class: 'shrink-0 w-9 flex items-center justify-center',
    style: {
      background: state ? t.accent + '18' : 'transparent',
      border: `1px solid ${t.accent}${state ? '' : '55'}`, borderLeft: 'none',
      borderRadius: '0 6px 6px 0',
    },
  }, [vText(state ? 'ON' : 'OFF', { color: t.accent })]),
}

// ─────────────────────────────────────────────────────────────────────────────
// Variant id → button-style mapping
// ─────────────────────────────────────────────────────────────────────────────
const VARIANT_BTN: Record<number, BtnSet> = {
  1: rounded, 2: rounded, 3: rounded, 4: rounded,
  5: rounded, 6: outline, 7: rounded, 8: rounded,
  9: ghost,   10: rounded, 11: ghost, 12: pill,
}

// ─────────────────────────────────────────────────────────────────────────────
// NodeShell
// ─────────────────────────────────────────────────────────────────────────────
const NodeShell = defineComponent({
  name: 'NodeShell',
  props: {
    node: nodeProp.node,
    variantId: { type: Number, required: true },
  },
  setup(p, { slots }) {
    return () => {
      const t = getTokens(p.node.nodeType)
      const set = VARIANT_BTN[p.variantId] ?? rounded
      const a = p.node.actionButton
      const tg = p.node.toggleButton

      return h('div', { class: 'flex flex-col items-start gap-1.5' }, [
        h('div', { class: 'flex items-stretch' }, [
          a && set.action(t, a.label),
          h('div', { class: 'relative flex' }, slots.default?.()),
          tg && set.toggle(t, tg.state),
        ]),
        statusPill(p.node),
      ])
    }
  },
})

// ─────────────────────────────────────────────────────────────────────────────
// Variants — all in the minimal / soft family
// ─────────────────────────────────────────────────────────────────────────────

// V1 — Minimal Flat
const V1 = defineComponent({
  props: nodeProp,
  setup(p) {
    return () => {
      const t = getTokens(p.node.nodeType)
      return h('div', {
        class: 'relative flex flex-col rounded-md',
        style: { minWidth: '200px', background: '#161b22' },
      }, [
        ...ports(p.node, t.accent),
        h('div', { class: 'h-[2px] rounded-t-md', style: { background: t.accent } }),
        h('div', { class: 'flex items-center gap-2 px-3 pt-2.5' }, [
          h('span', { style: { color: t.accent } }, [h(NodeIcon, { type: p.node.nodeType })]),
          h('span', { class: 'text-[11px] font-semibold tracking-wide' }, p.node.typeLabel),
        ]),
        h('div', { class: 'px-3 py-1 pb-2.5 text-[10px] font-mono text-terminal-text-dim' }, p.node.label),
      ])
    }
  },
})

// V2 — Soft Card
const V2 = defineComponent({
  props: nodeProp,
  setup(p) {
    return () => {
      const t = getTokens(p.node.nodeType)
      return h('div', {
        class: 'relative flex flex-col rounded-lg overflow-hidden',
        style: { minWidth: '230px', background: '#161b22', boxShadow: `0 6px 16px rgba(0,0,0,0.4), 0 0 0 1px ${t.border}` },
      }, [
        ...ports(p.node, t.accent),
        h('div', { class: 'flex items-center gap-3 px-4 py-3.5' }, [
          h('div', {
            class: 'w-9 h-9 rounded-md flex items-center justify-center shrink-0',
            style: { background: t.accent + '18', color: t.accent },
          }, [h(NodeIcon, { type: p.node.nodeType })]),
          h('div', { class: 'flex-1' }, [
            h('div', { class: 'text-[11px] font-semibold tracking-wide text-terminal-text' }, p.node.typeLabel),
            h('div', { class: 'text-[10px] font-mono text-terminal-text-dim mt-0.5' }, p.node.label),
          ]),
        ]),
      ])
    }
  },
})

// V3 — Bottom Accent
const V3 = defineComponent({
  props: nodeProp,
  setup(p) {
    return () => {
      const t = getTokens(p.node.nodeType)
      return h('div', {
        class: 'relative flex flex-col rounded-md',
        style: { minWidth: '210px', background: '#161b22' },
      }, [
        ...ports(p.node, t.accent),
        h('div', { class: 'flex items-center gap-2 px-3 pt-2.5' }, [
          h('span', { style: { color: t.accent } }, [h(NodeIcon, { type: p.node.nodeType })]),
          h('span', { class: 'text-[11px] font-semibold tracking-wide' }, p.node.typeLabel),
        ]),
        h('div', { class: 'px-3 py-1 pb-2.5 text-[10px] font-mono text-terminal-text-dim' }, p.node.label),
        h('div', { class: 'h-[2px] rounded-b-md', style: { background: t.accent } }),
      ])
    }
  },
})

// V4 — Left Accent Hairline
const V4 = defineComponent({
  props: nodeProp,
  setup(p) {
    return () => {
      const t = getTokens(p.node.nodeType)
      return h('div', {
        class: 'relative flex rounded-md overflow-hidden',
        style: { minWidth: '210px', background: '#161b22' },
      }, [
        ...ports(p.node, t.accent),
        h('div', { class: 'shrink-0 w-[2px]', style: { background: t.accent } }),
        h('div', { class: 'flex-1 flex flex-col px-3 py-2.5' }, [
          h('div', { class: 'flex items-center gap-2' }, [
            h('span', { style: { color: t.accent } }, [h(NodeIcon, { type: p.node.nodeType })]),
            h('span', { class: 'text-[11px] font-semibold tracking-wide' }, p.node.typeLabel),
          ]),
          h('div', { class: 'text-[10px] font-mono text-terminal-text-dim mt-0.5' }, p.node.label),
        ]),
      ])
    }
  },
})

// V5 — Icon Side Tile
const V5 = defineComponent({
  props: nodeProp,
  setup(p) {
    return () => {
      const t = getTokens(p.node.nodeType)
      return h('div', {
        class: 'relative flex rounded-lg overflow-hidden',
        style: { minWidth: '230px', background: '#161b22', boxShadow: `0 0 0 1px ${t.border}` },
      }, [
        ...ports(p.node, t.accent),
        h('div', {
          class: 'shrink-0 w-12 flex items-center justify-center',
          style: { background: t.accent + '18', color: t.accent },
        }, [h(NodeIcon, { type: p.node.nodeType })]),
        h('div', { class: 'flex-1 flex flex-col px-3 py-2.5' }, [
          h('div', { class: 'text-[11px] font-semibold tracking-wide text-terminal-text' }, p.node.typeLabel),
          h('div', { class: 'text-[10px] font-mono text-terminal-text-dim mt-0.5' }, p.node.label),
        ]),
      ])
    }
  },
})

// V6 — Outlined
const V6 = defineComponent({
  props: nodeProp,
  setup(p) {
    return () => {
      const t = getTokens(p.node.nodeType)
      return h('div', {
        class: 'relative flex flex-col rounded-lg overflow-hidden',
        style: {
          minWidth: '220px',
          background: '#0d1117',
          border: `1px solid ${t.accent}55`,
        },
      }, [
        ...ports(p.node, t.accent),
        h('div', { class: 'flex items-center gap-3 px-4 py-3.5' }, [
          h('div', {
            class: 'w-9 h-9 rounded-md flex items-center justify-center shrink-0',
            style: { color: t.accent, border: `1px solid ${t.accent}55` },
          }, [h(NodeIcon, { type: p.node.nodeType })]),
          h('div', { class: 'flex-1' }, [
            h('div', { class: 'text-[11px] font-semibold tracking-wide', style: { color: t.accent } }, p.node.typeLabel),
            h('div', { class: 'text-[10px] font-mono text-terminal-text-dim mt-0.5' }, p.node.label),
          ]),
        ]),
      ])
    }
  },
})

// V7 — Tinted Surface
const V7 = defineComponent({
  props: nodeProp,
  setup(p) {
    return () => {
      const t = getTokens(p.node.nodeType)
      return h('div', {
        class: 'relative flex flex-col rounded-lg',
        style: {
          minWidth: '220px',
          background: `linear-gradient(180deg, ${t.accent}10, ${t.accent}04 60%, transparent)`,
        },
      }, [
        ...ports(p.node, t.accent),
        h('div', { class: 'flex items-center gap-3 px-4 pt-3.5' }, [
          h('div', {
            class: 'w-8 h-8 rounded-md flex items-center justify-center shrink-0',
            style: { background: t.accent + '22', color: t.accent },
          }, [h(NodeIcon, { type: p.node.nodeType })]),
          h('div', { class: 'flex-1' }, [
            h('div', { class: 'text-[11px] font-semibold tracking-wide', style: { color: t.accent } }, p.node.typeLabel),
          ]),
        ]),
        h('div', { class: 'text-[10px] font-mono text-terminal-text-dim px-4 pt-1 pb-3' }, p.node.label),
      ])
    }
  },
})

// V8 — Header Tint
const V8 = defineComponent({
  props: nodeProp,
  setup(p) {
    return () => {
      const t = getTokens(p.node.nodeType)
      return h('div', {
        class: 'relative flex flex-col rounded-lg overflow-hidden',
        style: { minWidth: '220px', background: '#161b22', boxShadow: `0 0 0 1px ${t.border}` },
      }, [
        ...ports(p.node, t.accent),
        h('div', {
          class: 'flex items-center gap-2 px-3 py-2',
          style: { background: t.accent + '14' },
        }, [
          h('span', { style: { color: t.accent } }, [h(NodeIcon, { type: p.node.nodeType })]),
          h('span', { class: 'text-[11px] font-semibold tracking-wide', style: { color: t.accent } }, p.node.typeLabel),
        ]),
        h('div', { class: 'text-[10px] font-mono text-terminal-text-dim px-3 py-2' }, p.node.label),
      ])
    }
  },
})

// V9 — Floating
const V9 = defineComponent({
  props: nodeProp,
  setup(p) {
    return () => {
      const t = getTokens(p.node.nodeType)
      return h('div', {
        class: 'relative flex flex-col rounded-xl',
        style: {
          minWidth: '230px',
          background: '#1c2128',
          boxShadow: '0 12px 24px rgba(0,0,0,0.45), 0 2px 4px rgba(0,0,0,0.3)',
        },
      }, [
        ...ports(p.node, t.accent),
        h('div', { class: 'flex items-center gap-3 px-4 py-3.5' }, [
          h('div', {
            class: 'w-9 h-9 rounded-lg flex items-center justify-center shrink-0',
            style: { background: t.accent + '20', color: t.accent },
          }, [h(NodeIcon, { type: p.node.nodeType })]),
          h('div', { class: 'flex-1' }, [
            h('div', { class: 'text-[11px] font-semibold tracking-wide text-terminal-text' }, p.node.typeLabel),
            h('div', { class: 'text-[10px] font-mono text-terminal-text-dim mt-0.5' }, p.node.label),
          ]),
        ]),
      ])
    }
  },
})

// V10 — Underline Type
const V10 = defineComponent({
  props: nodeProp,
  setup(p) {
    return () => {
      const t = getTokens(p.node.nodeType)
      return h('div', {
        class: 'relative flex flex-col rounded-md',
        style: { minWidth: '220px', background: '#161b22', boxShadow: `0 0 0 1px ${t.border}` },
      }, [
        ...ports(p.node, t.accent),
        h('div', { class: 'flex items-center gap-2 px-3 pt-3' }, [
          h('span', { style: { color: t.accent } }, [h(NodeIcon, { type: p.node.nodeType })]),
          h('span', {
            class: 'text-[11px] font-semibold tracking-wide pb-1',
            style: { borderBottom: `2px solid ${t.accent}`, color: t.accent },
          }, p.node.typeLabel),
        ]),
        h('div', { class: 'text-[10px] font-mono text-terminal-text-dim px-3 py-2' }, p.node.label),
      ])
    }
  },
})

// V11 — Quiet Mono
const V11 = defineComponent({
  props: nodeProp,
  setup(p) {
    return () => {
      const t = getTokens(p.node.nodeType)
      return h('div', {
        class: 'relative flex flex-col rounded-md',
        style: { minWidth: '220px', background: '#161b22', boxShadow: '0 0 0 1px #2a3441' },
      }, [
        ...ports(p.node, t.accent),
        h('div', { class: 'flex items-center gap-2 px-3 pt-2.5' }, [
          h('span', { style: { color: '#7d8590' } }, [h(NodeIcon, { type: p.node.nodeType })]),
          h('span', { class: 'text-[11px] font-medium tracking-wide text-terminal-text' }, p.node.typeLabel),
          h('span', { class: 'w-1 h-1 rounded-full ml-auto', style: { background: t.accent } }),
        ]),
        h('div', { class: 'text-[10px] font-mono text-terminal-text-dim px-3 py-1 pb-2.5' }, p.node.label),
      ])
    }
  },
})

// V12 — Stacked Center
const V12 = defineComponent({
  props: nodeProp,
  setup(p) {
    return () => {
      const t = getTokens(p.node.nodeType)
      return h('div', {
        class: 'relative flex flex-col items-center px-4 py-3 rounded-lg',
        style: { minWidth: '210px', background: '#161b22', boxShadow: `0 0 0 1px ${t.border}` },
      }, [
        ...ports(p.node, t.accent),
        h('div', { class: 'text-[9px] font-semibold uppercase tracking-[0.2em]', style: { color: t.accent } }, p.node.typeLabel),
        h('div', {
          class: 'w-12 h-12 rounded-lg flex items-center justify-center my-2',
          style: { background: t.accent + '18', color: t.accent },
        }, [h(NodeIcon, { type: p.node.nodeType })]),
        h('div', { class: 'text-[10px] font-mono text-terminal-text-dim' }, p.node.label),
      ])
    }
  },
})

const VARIANT_COMPS = [V1, V2, V3, V4, V5, V6, V7, V8, V9, V10, V11, V12]
function variantComp(id: number) { return VARIANT_COMPS[id - 1] ?? V1 }
</script>

<template>
  <div class="h-full overflow-y-auto bg-terminal-bg text-terminal-text">
    <header class="sticky top-0 z-20 bg-terminal-surface border-b border-terminal-border px-6 py-4 flex items-center justify-between">
      <div>
        <h1 class="text-lg font-bold tracking-wide">Node Design Preview</h1>
        <p class="text-xs text-terminal-text-dim mt-0.5">12 Varianten in der Minimal/Soft-Familie. Klick auf eine Karte für Solo-Ansicht.</p>
      </div>
      <div class="flex items-center gap-2">
        <button
          v-if="!showAll"
          class="px-3 py-1.5 text-xs rounded border border-terminal-border hover:bg-terminal-surface-alt transition-colors"
          @click="selectedId = null"
        >Zurück zur Übersicht</button>
        <router-link
          to="/"
          class="px-3 py-1.5 text-xs rounded border border-terminal-border hover:bg-terminal-surface-alt transition-colors"
        >Zum Editor</router-link>
      </div>
    </header>

    <main class="p-6">
      <div
        v-if="showAll"
        class="grid gap-6"
        style="grid-template-columns: repeat(auto-fill, minmax(560px, 1fr))"
      >
        <section
          v-for="v in variants"
          :key="v.id"
          class="bg-terminal-surface border border-terminal-border rounded-lg overflow-hidden cursor-pointer hover:border-accent transition-colors"
          @click="pick(v.id)"
        >
          <div class="px-4 py-3 border-b border-terminal-border flex items-baseline justify-between">
            <div>
              <div class="text-[10px] text-terminal-text-dim font-mono">VARIANT {{ String(v.id).padStart(2, '0') }}</div>
              <h2 class="text-sm font-semibold tracking-wide">{{ v.name }}</h2>
            </div>
            <span class="text-[10px] text-terminal-text-dim">→ Solo</span>
          </div>
          <p class="px-4 pt-3 text-xs text-terminal-text-dim leading-relaxed">{{ v.desc }}</p>
          <div class="p-4 flex flex-wrap gap-4 items-start">
            <NodeShell v-for="s in samples" :key="s.id" :node="s" :variant-id="v.id">
              <component :is="variantComp(v.id)" :node="s" />
            </NodeShell>
          </div>
        </section>
      </div>

      <div v-else class="max-w-5xl mx-auto">
        <div
          v-for="v in variants.filter(x => x.id === selectedId)"
          :key="v.id"
          class="bg-terminal-surface border border-terminal-border rounded-lg overflow-hidden"
        >
          <div class="px-6 py-4 border-b border-terminal-border">
            <div class="text-[10px] text-terminal-text-dim font-mono">VARIANT {{ String(v.id).padStart(2, '0') }}</div>
            <h2 class="text-xl font-semibold tracking-wide">{{ v.name }}</h2>
            <p class="text-sm text-terminal-text-dim mt-1">{{ v.desc }}</p>
          </div>
          <div class="p-8 bg-terminal-bg flex flex-wrap gap-6 items-start">
            <NodeShell v-for="s in samples" :key="s.id" :node="s" :variant-id="v.id">
              <component :is="variantComp(v.id)" :node="s" />
            </NodeShell>
          </div>
        </div>
      </div>
    </main>
  </div>
</template>
