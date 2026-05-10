<script setup lang="ts">
import { computed } from 'vue'
import FormField from '@/components/ui/FormField.vue'
import FormInput from '@/components/ui/FormInput.vue'
import FormSelect from '@/components/ui/FormSelect.vue'
import FormCheckbox from '@/components/ui/FormCheckbox.vue'
import NumberInput from '@/components/ui/NumberInput.vue'
import { useNodeProperty } from '@/composables/useNodeProperty'

const mode = useNodeProperty<string>('mode', 'read')
const path = useNodeProperty<string>('path', '')
const encoding = useNodeProperty<string>('encoding', 'auto')
const rootJail = useNodeProperty<string>('rootJail', '')
const watchEvents = useNodeProperty<string[]>('watchEvents', ['write', 'create'])
const debounceMs = useNodeProperty<number>('debounceMs', 50)
const incremental = useNodeProperty<boolean>('incremental', false)
const fromStart = useNodeProperty<boolean>('fromStart', false)
const delimiter = useNodeProperty<string>('delimiter', '\n')
const maxLineBytes = useNodeProperty<number>('maxLineBytes', 1048576)

const delimiters = [
  { value: '\n',   label: 'LF (\\n)' },
  { value: '\r\n', label: 'CRLF (\\r\\n)' },
  { value: 'auto', label: 'Auto (LF or CRLF)' },
  { value: 'none', label: 'None (raw bytes)' },
]

const modes = [
  { value: 'read',       label: 'Read (triggered by message)' },
  { value: 'watch',      label: 'Watch (event metadata only)' },
  { value: 'read+watch', label: 'Read + Watch (content on change)' },
]

const encodings = [
  { value: 'auto',   label: 'Auto (by extension)' },
  { value: 'utf-8',  label: 'UTF-8 (text)' },
  { value: 'binary', label: 'Binary (number array)' },
]

const isWatchMode = computed(() => mode.value !== 'read')

// Per-event toggle factory wraps watchEvents[] for FormCheckbox bindings.
const eventNames = ['write', 'create', 'remove', 'rename'] as const
function makeToggle(name: typeof eventNames[number]) {
  return computed({
    get: () => (watchEvents.value ?? []).includes(name),
    set: (v: boolean) => {
      const current = new Set(watchEvents.value ?? [])
      if (v) current.add(name)
      else current.delete(name)
      watchEvents.value = eventNames.filter(n => current.has(n))
    },
  })
}
const writeT = makeToggle('write')
const createT = makeToggle('create')
const removeT = makeToggle('remove')
const renameT = makeToggle('rename')
</script>

<template>
  <div class="flex flex-col gap-3 flex-1 min-h-0">
    <FormField label="Mode">
      <FormSelect
        :model-value="mode"
        :options="modes"
        @update:model-value="mode = String($event)"
      />
    </FormField>

    <FormField label="Path">
      <FormInput
        v-model="path"
        placeholder="/var/run/status.json"
        mono
        :invalid="!path"
      />
      <div v-if="!path" class="text-[10px] text-status-error leading-tight">
        Absolute file path required
      </div>
      <div v-else class="text-[10px] text-text-muted leading-tight">
        {{ '{{mustache}}' }} supported in Read mode; msg.filename overrides this path
      </div>
    </FormField>

    <FormField label="Encoding">
      <FormSelect
        :model-value="encoding"
        :options="encodings"
        @update:model-value="encoding = String($event)"
      />
    </FormField>

    <template v-if="isWatchMode">
      <FormField label="Watch events">
        <div class="flex flex-col gap-1">
          <FormCheckbox v-model="writeT"  label="Write" />
          <FormCheckbox v-model="createT" label="Create" />
          <FormCheckbox v-model="removeT" label="Remove" />
          <FormCheckbox v-model="renameT" label="Rename" />
        </div>
      </FormField>

      <FormField label="Debounce">
        <NumberInput
          :model-value="debounceMs"
          :min="0"
          :max="10000"
          unit="ms"
          @update:model-value="debounceMs = Number($event)"
        />
        <div class="text-[10px] text-text-muted leading-tight">
          Collapses bursts (e.g. write → flush → close) into a single event.
        </div>
      </FormField>
    </template>

    <FormCheckbox v-model="incremental" label="Incremental (only new bytes since last read)" />

    <template v-if="incremental">
      <FormCheckbox v-model="fromStart" label="Start from beginning on first access" />

      <FormField label="Line delimiter">
        <FormSelect
          :model-value="delimiter"
          :options="delimiters"
          @update:model-value="delimiter = String($event)"
        />
        <div class="text-[10px] text-text-muted leading-tight">
          Partial trailing lines are buffered until the next read.
        </div>
      </FormField>

      <FormField label="Max line size">
        <NumberInput
          :model-value="maxLineBytes"
          :min="1024"
          :max="104857600"
          unit="bytes"
          @update:model-value="maxLineBytes = Number($event)"
        />
      </FormField>
    </template>

    <FormField label="Root jail (optional)">
      <FormInput
        v-model="rootJail"
        placeholder="/var/log"
        mono
      />
      <div class="text-[10px] text-text-muted leading-tight">
        Resolved paths must stay inside this directory. Empty disables the check.
      </div>
    </FormField>
  </div>
</template>
