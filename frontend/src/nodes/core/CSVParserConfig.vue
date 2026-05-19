<script setup lang="ts">
import { computed } from 'vue'
import FormField from '@/components/ui/FormField.vue'
import FormInput from '@/components/ui/FormInput.vue'
import FormSelect from '@/components/ui/FormSelect.vue'
import FormCheckbox from '@/components/ui/FormCheckbox.vue'
import { useNodeProperty } from '@/composables/useNodeProperty'

const property        = useNodeProperty<string>('property', 'payload')
const action          = useNodeProperty<string>('action', 'auto')
const header          = useNodeProperty<boolean>('header', true)
const headerOnce      = useNodeProperty<boolean>('headerOnce', false)
const columns         = useNodeProperty<string>('columns', '')
const delimiter       = useNodeProperty<string>('delimiter', ',')
const quoteChar       = useNodeProperty<string>('quoteChar', '"')
const comment         = useNodeProperty<string>('comment', '')
const trimSpaces      = useNodeProperty<boolean>('trimSpaces', false)
const skipEmptyLines  = useNodeProperty<boolean>('skipEmptyLines', true)
const forceQuote      = useNodeProperty<boolean>('forceQuote', false)
const newline         = useNodeProperty<string>('newline', '\n')
const output          = useNodeProperty<string>('output', 'rows')
const cast            = useNodeProperty<boolean>('cast', false)

const actions = [
  { value: 'auto',      label: 'auto (string ↔ object)' },
  { value: 'parse',     label: 'parse (string → object)' },
  { value: 'stringify', label: 'stringify (object → string)' },
]

const knownDelimiters = [',', ';', '\t', '|']

const delimiterPresets = [
  { value: ',',      label: ', (comma)' },
  { value: ';',      label: '; (semicolon)' },
  { value: '\t',     label: '\\t (tab)' },
  { value: '|',      label: '| (pipe)' },
  { value: 'custom', label: 'custom…' },
]

// Preset reflects the delimiter when it is one of the known shortcuts;
// otherwise the user is in custom mode and the raw delimiter is shown in
// the custom input below. Switching to 'custom' keeps the existing value
// untouched so the user can edit it.
const delimiterPreset = computed<string>({
  get: () => (knownDelimiters.includes(delimiter.value) ? delimiter.value : 'custom'),
  set: (v: string) => {
    if (v === 'custom') return // wait for the custom input to set delimiter
    delimiter.value = v
  },
})

const isCustomDelimiter = computed(() => delimiterPreset.value === 'custom')

const newlines = [
  { value: '\n',   label: '\\n (LF)' },
  { value: '\r\n', label: '\\r\\n (CRLF)' },
]

const outputs = [
  { value: 'rows',  label: 'rows (one message per row)' },
  { value: 'array', label: 'array (single message)' },
]

const showStringifyFields = computed(() => action.value !== 'parse')
const showParseFields     = computed(() => action.value !== 'stringify')
</script>

<template>
  <div class="flex flex-col gap-3 flex-1 min-h-0">
    <FormField label="Property">
      <FormInput
        v-model="property"
        placeholder="payload"
        mono
        :invalid="!property"
      >
        <template #prefix>msg.</template>
      </FormInput>
      <div v-if="!property" class="text-[10px] text-status-error leading-tight">
        Property name required
      </div>
    </FormField>

    <FormField label="Action">
      <FormSelect
        :model-value="action"
        :options="actions"
        @update:model-value="action = String($event)"
      />
    </FormField>

    <FormField>
      <FormCheckbox v-model="header" label="First row is header" />
    </FormField>

    <FormField v-if="showStringifyFields && header">
      <FormCheckbox v-model="headerOnce" label="Emit header only on first message (stringify)" />
      <div class="text-[10px] text-terminal-text-dim leading-tight">
        Suppresses the header on every call after the first — useful when
        appending CSV rows to a file that already has the header on disk.
      </div>
    </FormField>

    <FormField label="Columns">
      <FormInput
        v-model="columns"
        placeholder="a,b,c"
        mono
      />
      <div class="text-[10px] text-terminal-text-dim leading-tight">
        <template v-if="action !== 'stringify' && header">
          Ignored on parse — header row supplies the column names.
        </template>
        <template v-else>
          Comma-separated list. Defines column order on stringify; supplies
          keys when parsing without a header row.
        </template>
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
      <FormInput
        v-model="quoteChar"
        placeholder="&quot;"
        mono
      />
    </FormField>

    <FormField label="Comment character">
      <FormInput
        v-model="comment"
        placeholder="(none)"
        mono
      />
      <div class="text-[10px] text-terminal-text-dim leading-tight">
        Lines starting with this character are skipped on parse. Leave blank to disable.
      </div>
    </FormField>

    <FormField>
      <FormCheckbox v-model="trimSpaces" label="Trim whitespace around cells" />
    </FormField>

    <FormField>
      <FormCheckbox v-model="skipEmptyLines" label="Skip empty lines (parse)" />
    </FormField>

    <FormField v-if="showStringifyFields">
      <FormCheckbox v-model="forceQuote" label="Always quote every field (stringify)" />
    </FormField>

    <FormField v-if="showStringifyFields" label="Line terminator">
      <FormSelect
        :model-value="newline"
        :options="newlines"
        @update:model-value="newline = String($event)"
      />
    </FormField>

    <FormField v-if="showParseFields" label="Output mode">
      <FormSelect
        :model-value="output"
        :options="outputs"
        @update:model-value="output = String($event)"
      />
    </FormField>

    <FormField v-if="showParseFields">
      <FormCheckbox v-model="cast" label="Cast values to bool / number / null" />
    </FormField>
  </div>
</template>
