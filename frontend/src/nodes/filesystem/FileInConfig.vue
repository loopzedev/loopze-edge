<script setup lang="ts">
import FormField from '@/components/ui/FormField.vue'
import FormInput from '@/components/ui/FormInput.vue'
import FormSelect from '@/components/ui/FormSelect.vue'
import { useNodeProperty } from '@/composables/useNodeProperty'

const mode = useNodeProperty<string>('mode', 'read')
const path = useNodeProperty<string>('path', '')
const encoding = useNodeProperty<string>('encoding', 'auto')
const rootJail = useNodeProperty<string>('rootJail', '')

// Phase 3: only read mode is functional. Watch / Read+Watch land in
// phase 4; incremental controls land in phase 5. The dropdown still lists
// the future modes so workspaces created today survive the upgrade.
const modes = [
  { value: 'read',       label: 'Read (triggered by message)' },
  { value: 'watch',      label: 'Watch (coming in phase 4)' },
  { value: 'read+watch', label: 'Read + Watch (coming in phase 4)' },
]

const encodings = [
  { value: 'auto',   label: 'Auto (by extension)' },
  { value: 'utf-8',  label: 'UTF-8 (text)' },
  { value: 'binary', label: 'Binary (number array)' },
]
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

    <div v-if="mode !== 'read'" class="text-[10px] text-status-warning leading-tight">
      Watch mode lands in phase 4. Until then the node only reacts to incoming messages in Read mode.
    </div>
  </div>
</template>
