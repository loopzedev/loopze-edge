<script setup lang="ts">
import FormField from '@/components/ui/FormField.vue'
import FormInput from '@/components/ui/FormInput.vue'
import FormSelect from '@/components/ui/FormSelect.vue'
import FormCheckbox from '@/components/ui/FormCheckbox.vue'
import NumberInput from '@/components/ui/NumberInput.vue'
import { useNodeProperty } from '@/composables/useNodeProperty'

const path = useNodeProperty<string>('path', '')
const encoding = useNodeProperty<string>('encoding', 'auto')
const rootJail = useNodeProperty<string>('rootJail', '')
const incremental = useNodeProperty<boolean>('incremental', false)
const fromStart = useNodeProperty<boolean>('fromStart', false)
const delimiter = useNodeProperty<string>('delimiter', '\n')
const maxLineBytes = useNodeProperty<number>('maxLineBytes', 1048576)

const encodings = [
  { value: 'auto',         label: 'Auto (by file extension)' },
  { value: 'utf-8',        label: 'UTF-8' },
  { value: 'utf-16le',     label: 'UTF-16 LE (Windows Unicode / Excel)' },
  { value: 'utf-16be',     label: 'UTF-16 BE' },
  { value: 'utf-16',       label: 'UTF-16 (BOM detection)' },
  { value: 'latin1',       label: 'Latin-1 / ISO-8859-1' },
  { value: 'windows-1252', label: 'Windows-1252' },
  { value: 'binary',       label: 'Binary ([]int)' },
]

const delimiters = [
  { value: '\n',   label: 'LF (\\n)' },
  { value: '\r\n', label: 'CRLF (\\r\\n)' },
  { value: 'auto', label: 'Auto (LF or CRLF)' },
  { value: 'none', label: 'None (raw bytes)' },
]

// Workaround for Vue's Mustache-style interpolation: rendering the literal
// string "{{mustache}}" inline would close the outer {{ }} prematurely.
const MUSTACHE_TOKEN = '{{mustache}}'
</script>

<template>
  <div class="flex flex-col gap-3 flex-1 min-h-0">
    <FormField label="Path (optional)">
      <FormInput
        v-model="path"
        placeholder="/var/log/app.log"
        mono
      />
      <div v-if="!path" class="text-[10px] text-text-muted leading-tight">
        Leave empty to read msg.filename from the incoming message
        (e.g. wired after a File Watch node).
      </div>
      <div v-else class="text-[10px] text-text-muted leading-tight">
        {{ MUSTACHE_TOKEN }} supported; msg.filename overrides this path
      </div>
    </FormField>

    <FormField label="Encoding">
      <FormSelect
        :model-value="encoding"
        :options="encodings"
        @update:model-value="encoding = String($event)"
      />
    </FormField>

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
