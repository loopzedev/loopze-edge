<script setup lang="ts">
import { computed } from 'vue'
import FormField from '@/components/ui/FormField.vue'
import FormInput from '@/components/ui/FormInput.vue'
import FormSelect from '@/components/ui/FormSelect.vue'
import FormCheckbox from '@/components/ui/FormCheckbox.vue'
import { useNodeProperty } from '@/composables/useNodeProperty'

const path        = useNodeProperty<string>('path', '')
const mode        = useNodeProperty<string>('mode', 'append')
const property    = useNodeProperty<string>('property', 'payload')
const header      = useNodeProperty<boolean>('header', true)
const columns     = useNodeProperty<string>('columns', '')
const delimiter   = useNodeProperty<string>('delimiter', ',')
const quoteChar   = useNodeProperty<string>('quoteChar', '"')
const forceQuote  = useNodeProperty<boolean>('forceQuote', false)
const newline     = useNodeProperty<string>('newline', '\n')
const createDirs  = useNodeProperty<boolean>('createDirs', false)

const modes = [
  { value: 'append',    label: 'append (write header only on empty file)' },
  { value: 'overwrite', label: 'overwrite (replace file)' },
  { value: 'create',    label: 'create (fail if file exists)' },
]

const knownDelimiters = [',', ';', '\t', '|']
const delimiterPresets = [
  { value: ',',      label: ', (comma)' },
  { value: ';',      label: '; (semicolon)' },
  { value: '\t',     label: '\\t (tab)' },
  { value: '|',      label: '| (pipe)' },
  { value: 'custom', label: 'custom…' },
]

const delimiterPreset = computed<string>({
  get: () => (knownDelimiters.includes(delimiter.value) ? delimiter.value : 'custom'),
  set: (v: string) => {
    if (v === 'custom') return
    delimiter.value = v
  },
})

const isCustomDelimiter = computed(() => delimiterPreset.value === 'custom')

const newlines = [
  { value: '\n',   label: '\\n (LF)' },
  { value: '\r\n', label: '\\r\\n (CRLF)' },
]
</script>

<template>
  <div class="flex flex-col gap-3 flex-1 min-h-0">
    <FormField label="Path">
      <FormInput
        v-model="path"
        placeholder="/var/log/sensors.csv"
        mono
        :invalid="!path"
      />
      <div v-if="!path" class="text-[10px] text-status-error leading-tight">
        Path required
      </div>
      <div v-else class="text-[10px] text-terminal-text-dim leading-tight">
        Absolute path. Mustache supported. msg.filename wins.
      </div>
    </FormField>

    <FormField label="Mode">
      <FormSelect
        :model-value="mode"
        :options="modes"
        @update:model-value="mode = String($event)"
      />
    </FormField>

    <FormField>
      <FormCheckbox v-model="createDirs" label="Create parent directories if missing" />
    </FormField>

    <FormField label="Property">
      <FormInput v-model="property" placeholder="payload" mono :invalid="!property">
        <template #prefix>msg.</template>
      </FormInput>
    </FormField>

    <FormField>
      <FormCheckbox v-model="header" label="Write header row" />
      <div class="text-[10px] text-terminal-text-dim leading-tight">
        Append mode: written only when file is empty/missing.
        Overwrite mode: written on every message.
      </div>
    </FormField>

    <FormField label="Columns">
      <FormInput v-model="columns" placeholder="a,b,c" mono />
      <div class="text-[10px] text-terminal-text-dim leading-tight">
        Comma-separated. Pins column order. Required for [][]any input
        when header=true.
      </div>
    </FormField>

    <FormField label="Delimiter">
      <FormSelect
        :model-value="delimiterPreset"
        :options="delimiterPresets"
        @update:model-value="delimiterPreset = String($event)"
      />
    </FormField>

    <FormField v-if="isCustomDelimiter" label="Custom delimiter">
      <FormInput
        v-model="delimiter"
        placeholder=","
        mono
        :invalid="!delimiter"
      />
      <div v-if="!delimiter" class="text-[10px] text-status-error leading-tight">
        Delimiter required
      </div>
    </FormField>

    <FormField label="Quote character">
      <FormInput v-model="quoteChar" placeholder="&quot;" mono />
    </FormField>

    <FormField>
      <FormCheckbox v-model="forceQuote" label="Quote every field" />
    </FormField>

    <FormField label="Line terminator">
      <FormSelect
        :model-value="newline"
        :options="newlines"
        @update:model-value="newline = String($event)"
      />
    </FormField>
  </div>
</template>
