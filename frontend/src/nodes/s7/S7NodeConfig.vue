<script setup lang="ts">
import { computed, nextTick } from 'vue'
import { useVueFlow } from '@vue-flow/core'
import { useFlowStore } from '@/stores/flowStore'
import { useConfigSelector } from '@/composables/useConfigSelector'
import { useNodeProperty } from '@/composables/useNodeProperty'
import { useStructuralProperty } from '@/composables/useStructuralProperty'
import FormField from '@/components/ui/FormField.vue'
import FormInput from '@/components/ui/FormInput.vue'
import FormSelect from '@/components/ui/FormSelect.vue'
import FormCheckbox from '@/components/ui/FormCheckbox.vue'
import NumberInput from '@/components/ui/NumberInput.vue'
import SectionHeader from '@/components/ui/SectionHeader.vue'
import ToggleGroup from '@/components/ui/ToggleGroup.vue'
import {
  S7_DATA_TYPES,
  S7_BLOCK_AREAS,
  S7_WRITE_BLOCK_AREAS,
  S7_OUTPUT_SHAPES,
  S7_VALUE_SOURCES,
  validateS7Address,
} from './enums'

const flowStore = useFlowStore()
const { updateNodeInternals } = useVueFlow('loopze-flow-editor')
const { options: plcOptions, openNewConfig, openEditConfig } = useConfigSelector('s7-plc')

const node = computed(() => flowStore.selectedNode)
const isWrite = computed(() => (node.value?.data?.nodeType ?? node.value?.type) === 's7-write')

const plc = useNodeProperty<string>('plc', '')
const variables = useNodeProperty<S7Variable[]>('variables', [])
const block = useNodeProperty<S7BlockConfig>('block', defaultBlock())
const outputShape = useNodeProperty<string>('outputShape', '')

// Mode is the structural driver for the read node's input port count.
// `useStructuralProperty` keeps the port in sync via the derive function,
// which consults the (current) block.triggerOnInput when mode='block'. The
// composable's enabled/derive contract is the *correct* place for this —
// the previous implementation used a free-standing watch on
// flowStore.selectedNode and leaked writes onto whatever node the user
// happened to click next.
//
// Write-side mode never affects the input port (writes always have 1 input),
// so the composable's enabled gate skips the structural mirror entirely.
const mode = useStructuralProperty<string>('mode', 'static', {
  port: 'inputs',
  derive: deriveReadInputs,
  enabled: () => !isWrite.value,
})

// Read-side polling + suppression flags.
const pollInterval = useNodeProperty<number>('pollInterval', 1000)
const emitOnChange = useNodeProperty<boolean>('emitOnChange', false)
const emitOnError = useNodeProperty<boolean>('emitOnError', false)

// Write-side output flags. `useStructuralProperty` keeps the port count in
// sync with the toggle — the spec requires the two flags to be mutually
// exclusive, which the backend's Init() also enforces.
const emitAck = useStructuralProperty<boolean>('emitAck', false, {
  port: 'outputs',
  derive: (v) => (v ? 1 : 0),
  enabled: () => isWrite.value,
})
const passthrough = useStructuralProperty<boolean>('passthrough', false, {
  port: 'outputs',
  derive: (v) => (v ? 1 : 0),
  enabled: () => isWrite.value,
})

interface S7Variable {
  name?: string
  address: string
  dataType: string
  scale?: number
  offset?: number
  // Write-only fields
  valueSource?: 'static' | 'msg'
  value?: unknown
  valuePath?: string
}

interface S7BlockConfig {
  area: string
  db?: number
  start: number
  length?: number
  triggerOnInput?: boolean
  inputProperty?: string
}

function defaultBlock(): S7BlockConfig {
  return { area: 'DB', db: 1, start: 0, length: 0, triggerOnInput: false, inputProperty: 'payload' }
}

const isBlock = computed(() => mode.value === 'block')
const isDynamic = computed(() => mode.value === 'dynamic')

// Read-mode toggle. Writes don't have block right now in PR-9 either — wait,
// they do! Both nodes support all three modes. The write node uses `block`
// instead of `variables` differently (no variables-list entries; instead
// `msg.payload` carries the bytes). So the toggle applies to both.
const modePresets = [
  { label: 'Static (poll)',  value: 'static' },
  { label: 'Dynamic (msg)',  value: 'dynamic' },
  { label: 'Block (raw bytes)', value: 'block' },
]

const blockAreas = computed(() => isWrite.value ? S7_WRITE_BLOCK_AREAS : S7_BLOCK_AREAS)

// deriveReadInputs computes the input-port count for an s7-read node from
// the current `mode` and `block.triggerOnInput`. Used by both the mode
// structural property and the manual block.triggerOnInput sync below.
//
// Reads the *current* block.triggerOnInput from the selected node's config,
// because mode and triggerOnInput are independent inputs to the same port
// count: changing mode shouldn't reset the trigger preference, and toggling
// the trigger inside block mode shouldn't move the user out of block mode.
function deriveReadInputs(m: string): number {
  if (m === 'dynamic') return 1
  if (m === 'block') {
    const cfg = (flowStore.selectedNode?.data?.config ?? {}) as Record<string, unknown>
    const blk = (cfg.block ?? {}) as Record<string, unknown>
    return blk.triggerOnInput ? 1 : 0
  }
  return 0 // static
}

// syncBlockTriggerInputPort updates the input-port count when the user
// toggles `block.triggerOnInput`. Called explicitly from the toggle handler
// — never from a reactive effect — so we don't leak writes onto another
// node when the panel is being torn down.
function syncBlockTriggerInputPort(trigger: boolean) {
  if (isWrite.value) return // write nodes always have 1 input
  if (mode.value !== 'block') return // only block mode is trigger-driven
  const n = flowStore.selectedNode
  if (!n) return
  const desired = trigger ? 1 : 0
  if ((n.data?.inputs as number | undefined) !== desired) {
    flowStore.updateNodeData(n.id, { inputs: desired })
    nextTick(() => updateNodeInternals([n.id]))
  }
}

// Mutually-exclusive: turning on emitAck clears passthrough and vice versa.
function onEmitAckChange(v: boolean) {
  emitAck.value = v
  if (v && passthrough.value) passthrough.value = false
}
function onPassthroughChange(v: boolean) {
  passthrough.value = v
  if (v && emitAck.value) emitAck.value = false
}

// ── Variables-list editing ────────────────────────────────────────────────

function addVariable() {
  const blank: S7Variable = isWrite.value
    ? { address: '', dataType: 'real', valueSource: 'msg', valuePath: 'payload' }
    : { name: '', address: '', dataType: 'real' }
  variables.value = [...(variables.value ?? []), blank]
}

function removeVariable(idx: number) {
  const next = [...(variables.value ?? [])]
  next.splice(idx, 1)
  variables.value = next
}

function updateVariableField(idx: number, patch: Partial<S7Variable>) {
  const next = [...(variables.value ?? [])]
  next[idx] = { ...next[idx], ...patch }
  variables.value = next
}

// ── Block-config editing ──────────────────────────────────────────────────

function updateBlockField(patch: Partial<S7BlockConfig>) {
  const cur = block.value ?? defaultBlock()
  block.value = { ...cur, ...patch }
  // The trigger flag also drives the input port count when the node is in
  // block mode — sync it explicitly so the canvas reflects the change
  // without waiting for a structural-property recompute on `mode`.
  if (patch.triggerOnInput !== undefined) {
    syncBlockTriggerInputPort(Boolean(patch.triggerOnInput))
  }
}

// s7IsNonScalable mirrors the Go-side helper of the same name (s7_parser.go).
// Scaling is meaningless on these — they emit/accept strings, byte slices, or
// counter/timer ticks, none of which combine with scale × offset arithmetic.
const S7_NON_SCALABLE = new Set([
  'bool',
  'string', 'wstring', 'raw',
  'date', 'dt', 'ldt', 'dtl', 'wchar',
  'counter', 'timer',
])
function s7IsNonScalable(typ: string): boolean {
  return S7_NON_SCALABLE.has(typ)
}

// Per-row address validation. Used by the table editor to flag bad addresses
// before save (the backend's parser is authoritative; this is just UX).
function validateRow(v: S7Variable): { ok: boolean; msg?: string } {
  if (!v.address) return { ok: false, msg: 'address required' }
  const result = validateS7Address(v.address, v.dataType)
  if (!result.valid) return { ok: false, msg: result.error }
  return { ok: true }
}
</script>

<template>
  <div class="flex flex-col gap-3">
    <FormField label="PLC">
      <template #action>
        <button
          v-if="plc"
          class="text-[9px] uppercase tracking-wider text-terminal-text-dim hover:text-accent transition-colors"
          @click="openEditConfig(plc)"
        >Edit</button>
        <button
          class="text-[9px] uppercase tracking-wider text-terminal-text-dim hover:text-accent transition-colors"
          @click="openNewConfig()"
        >+ New</button>
      </template>
      <FormSelect v-model="plc" :options="plcOptions" placeholder="Select PLC..." />
    </FormField>

    <FormField label="Mode">
      <ToggleGroup v-model="mode" :options="modePresets" />
    </FormField>

    <!-- Variables list editor — shown for:
         - read static / dynamic (dynamic uses these as fallback)
         - write static
         The write+dynamic path ignores the sidebar list entirely (the runtime
         only consults `msg.variables` / convenience fields), so we hide the
         editor and show a hint instead — see the bottom of this template. -->
    <SectionHeader v-if="!isBlock && !(isWrite && isDynamic)" :title="isWrite ? 'Variables (write)' : 'Variables (read)'">
      <div class="flex flex-col gap-2">
        <div
          v-if="(variables ?? []).length === 0"
          class="text-[10px] text-terminal-text-dim italic"
        >
          {{ isDynamic
            ? 'No variables configured. The msg.variables array overrides this list per message.'
            : 'No variables yet — add one below.' }}
        </div>

        <div
          v-for="(v, i) in (variables ?? [])"
          :key="i"
          class="border border-terminal-border bg-terminal-bg p-2 flex flex-col gap-1.5 rounded"
        >
          <!-- Row 1: address (prominent) + delete -->
          <div class="flex items-stretch gap-1">
            <div class="flex-1 min-w-0">
              <FormInput
                :model-value="v.address"
                placeholder="DB10.DBD0 / M0.0 / IB1 …"
                mono
                :invalid="!validateRow(v).ok"
                @update:model-value="(val) => updateVariableField(i, { address: String(val) })"
              />
            </div>
            <button
              class="px-1.5 text-[10px] text-terminal-text-dim border border-terminal-border
                     hover:text-red-400 hover:border-red-700/40 transition-colors"
              @click="removeVariable(i)"
            >×</button>
          </div>
          <!-- Row 2: name (read only) + dataType -->
          <div class="flex items-stretch gap-1">
            <div v-if="!isWrite" class="flex-1 min-w-0">
              <FormInput
                :model-value="v.name ?? ''"
                placeholder="Name (defaults to address)"
                @update:model-value="(val) => updateVariableField(i, { name: String(val) })"
              />
            </div>
            <FormSelect
              :model-value="v.dataType"
              :options="S7_DATA_TYPES"
              :width="isWrite ? '100%' : '220px'"
              @update:model-value="(val) => updateVariableField(i, { dataType: String(val) })"
            />
          </div>

          <div
            v-if="!validateRow(v).ok && v.address"
            class="text-[10px] text-red-400/80 leading-tight"
          >
            {{ validateRow(v).msg }}
          </div>

          <!-- Read-only: per-variable scaling — value = raw * scale + offset -->
          <div
            v-if="!isWrite && !s7IsNonScalable(v.dataType)"
            class="flex items-stretch gap-2 text-[10px] text-terminal-text-dim"
          >
            <div class="flex items-center gap-1 flex-1 min-w-0">
              <span class="shrink-0">scale</span>
              <div class="flex-1 min-w-0">
                <NumberInput
                  :model-value="v.scale ?? 1"
                  :step="0.01"
                  title="Multiplicative scale: value = raw × scale + offset"
                  @update:model-value="(val) => updateVariableField(i, { scale: Number(val) })"
                />
              </div>
            </div>
            <div class="flex items-center gap-1 flex-1 min-w-0">
              <span class="shrink-0">offset</span>
              <div class="flex-1 min-w-0">
                <NumberInput
                  :model-value="v.offset ?? 0"
                  :step="0.01"
                  title="Additive offset, applied after scaling"
                  @update:model-value="(val) => updateVariableField(i, { offset: Number(val) })"
                />
              </div>
            </div>
          </div>

          <!-- Write-only: value source per variable -->
          <div v-if="isWrite" class="flex flex-col gap-1">
            <FormSelect
              :model-value="v.valueSource ?? 'msg'"
              :options="S7_VALUE_SOURCES"
              @update:model-value="(val) => updateVariableField(i, { valueSource: val as 'static' | 'msg' })"
            />
            <FormInput
              v-if="(v.valueSource ?? 'msg') === 'msg'"
              :model-value="v.valuePath ?? 'payload'"
              placeholder="payload"
              mono
              @update:model-value="(val) => updateVariableField(i, { valuePath: String(val) })"
            />
            <FormInput
              v-else
              :model-value="String(v.value ?? '')"
              placeholder="Static value (typed against dataType)"
              mono
              @update:model-value="(val) => updateVariableField(i, { value: val })"
            />
          </div>
        </div>

        <button
          class="self-start px-2 py-1 text-[10px] uppercase tracking-wider
                 text-accent border border-accent/30 hover:bg-accent/10 transition-colors"
          @click="addVariable"
        >+ Add Variable</button>
      </div>
    </SectionHeader>

    <!-- Block-mode editor ─────────────────────────────────────────────── -->
    <SectionHeader v-if="isBlock" title="Block">
      <div class="flex flex-col gap-2">
        <div class="flex items-stretch gap-2">
          <div class="flex-1">
            <FormField label="Area">
              <FormSelect
                :model-value="block?.area ?? 'DB'"
                :options="blockAreas"
                @update:model-value="(val) => updateBlockField({ area: String(val) })"
              />
            </FormField>
          </div>
          <div v-if="(block?.area ?? 'DB') === 'DB'" class="w-24 shrink-0">
            <FormField label="DB">
              <NumberInput
                :model-value="block?.db ?? 1"
                :min="1"
                @update:model-value="(val) => updateBlockField({ db: Number(val) })"
              />
            </FormField>
          </div>
        </div>
        <div class="flex items-stretch gap-2">
          <div class="flex-1">
            <FormField label="Start">
              <NumberInput
                :model-value="block?.start ?? 0"
                :min="0"
                @update:model-value="(val) => updateBlockField({ start: Number(val) })"
              />
            </FormField>
          </div>
          <div class="flex-1">
            <FormField
              :label="isWrite ? 'Length (0 = use input length)' : 'Length'"
              :hint="isWrite ? 'Optional; if set, msg.payload length must match' : 'Required'"
            >
              <NumberInput
                :model-value="block?.length ?? 0"
                :min="0"
                unit="bytes"
                @update:model-value="(val) => updateBlockField({ length: Number(val) })"
              />
            </FormField>
          </div>
        </div>
        <FormCheckbox
          v-if="!isWrite"
          :model-value="block?.triggerOnInput ?? false"
          label="Trigger on input (additive to poll)"
          @update:model-value="(val) => updateBlockField({ triggerOnInput: Boolean(val) })"
        />
        <FormField
          v-if="isWrite"
          label="Input property"
          hint="msg field carrying the byte slice (default: payload)"
        >
          <FormInput
            :model-value="block?.inputProperty ?? 'payload'"
            mono
            @update:model-value="(val) => updateBlockField({ inputProperty: String(val) })"
          />
        </FormField>

        <div class="text-[10px] text-terminal-text-dim leading-relaxed border border-terminal-border bg-terminal-bg p-2 rounded">
          <span v-if="!isWrite">
            Output is raw <span class="font-mono text-accent">msg.payload = []byte</span>.
            Pipe into an <span class="font-mono text-accent">s7-parser</span> to decode.
            Block size is capped by the negotiated PDU; oversized blocks are auto-split.
          </span>
          <span v-else>
            Input is raw bytes from <span class="font-mono text-accent">msg.{{ block?.inputProperty || 'payload' }}</span>.
            Sent in one AGWriteArea call (auto-split on PDU). Override
            <span class="font-mono text-accent">msg.s7.area</span> /
            <span class="font-mono text-accent">db</span> /
            <span class="font-mono text-accent">start</span> per message.
          </span>
        </div>
      </div>
    </SectionHeader>

    <!-- Read-only: output shape ─────────────────────────────────────── -->
    <FormField v-if="!isWrite && !isBlock" label="Output shape" hint="Defaults: single (1 var) / object (>1 var)">
      <FormSelect
        v-model="outputShape"
        :options="[{ value: '', label: '(auto)' }, ...S7_OUTPUT_SHAPES]"
      />
    </FormField>

    <!-- Read-only: polling settings ─────────────────────────────────── -->
    <SectionHeader v-if="!isWrite && !isDynamic" title="Polling">
      <div class="flex flex-col gap-2">
        <FormField label="Interval">
          <NumberInput v-model="pollInterval" :min="50" unit="ms" />
        </FormField>
        <FormCheckbox v-model="emitOnChange" label="Emit only on change" />
        <FormCheckbox v-model="emitOnError" label="Emit error message on read failure" />
      </div>
    </SectionHeader>

    <!-- Read-only: dynamic-mode hint ────────────────────────────────── -->
    <div
      v-if="!isWrite && isDynamic"
      class="text-[10px] text-terminal-text-dim leading-relaxed border border-terminal-border bg-terminal-bg p-2 rounded"
    >
      Send any message to trigger a read. Override the variables list via
      <span class="font-mono text-accent">msg.variables=[{address,dataType,name}]</span>
      or use the convenience form
      <span class="font-mono text-accent">msg.address</span> +
      <span class="font-mono text-accent">msg.dataType</span> for a single read.
    </div>

    <!-- Write-only: output flags ────────────────────────────────────── -->
    <SectionHeader v-if="isWrite" title="Output">
      <div class="flex flex-col gap-2">
        <FormCheckbox
          :model-value="emitAck"
          label="Emit ACK message after successful write"
          @update:model-value="onEmitAckChange"
        />
        <FormCheckbox
          :model-value="passthrough"
          label="Pass input message through with msg.s7Write enriched"
          @update:model-value="onPassthroughChange"
        />
      </div>
    </SectionHeader>

    <div
      v-if="isWrite && isDynamic"
      class="text-[10px] text-terminal-text-dim leading-relaxed border border-terminal-border bg-terminal-bg p-2 rounded flex flex-col gap-1.5"
    >
      <p>
        Dynamic mode takes the variable list <em>only</em> from the incoming
        message — the sidebar list is not used. Send either:
      </p>
      <ul class="list-disc list-inside ml-1 space-y-0.5">
        <li><span class="font-mono text-accent">msg.variables = [{ address, dataType, value }]</span> — full form</li>
        <li><span class="font-mono text-accent">msg.address</span> + <span class="font-mono text-accent">msg.dataType</span> + <span class="font-mono text-accent">msg.payload</span> — single write</li>
      </ul>
      <p>
        For a fixed address with values from incoming messages, use
        <span class="font-mono text-accent">Static</span> mode with
        <span class="font-mono text-accent">valueSource = msg</span> instead.
      </p>
    </div>
  </div>
</template>
