<script setup lang="ts">
import { computed } from 'vue'
import FormField from '@/components/ui/FormField.vue'
import FormInput from '@/components/ui/FormInput.vue'
import FormSelect from '@/components/ui/FormSelect.vue'
import FormCheckbox from '@/components/ui/FormCheckbox.vue'
import NumberInput from '@/components/ui/NumberInput.vue'
import { useNodeProperty } from '@/composables/useNodeProperty'

const mode = useNodeProperty<string>('mode', 'watch')
const path = useNodeProperty<string>('path', '')
const recursive = useNodeProperty<boolean>('recursive', false)
const glob = useNodeProperty<string>('glob', '*')
const watchEvents = useNodeProperty<string[]>('watchEvents', ['create', 'write', 'remove', 'rename'])
const sendAs = useNodeProperty<string>('sendAs', 'individual')
const debounceMs = useNodeProperty<number>('debounceMs', 100)
const incremental = useNodeProperty<boolean>('incremental', false)
const fromStart = useNodeProperty<boolean>('fromStart', false)
const rootJail = useNodeProperty<string>('rootJail', '')

const modes = [
  { value: 'read',       label: 'Read (list folder, triggered by message)' },
  { value: 'watch',      label: 'Watch (event-driven)' },
  { value: 'read+watch', label: 'Read + Watch (always incremental)' },
]

const sendAsOptions = [
  { value: 'individual', label: 'Individual (one message per entry)' },
  { value: 'array',      label: 'Array (one message with all entries)' },
]

const isWatchMode = computed(() => mode.value !== 'read')
const isReadPlusWatch = computed(() => mode.value === 'read+watch')
// read+watch is always incremental — UI hides the toggle for that mode.
const showIncremental = computed(() => mode.value !== 'read+watch')

const eventNames = ['create', 'write', 'remove', 'rename'] as const
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
const createT = makeToggle('create')
const writeT = makeToggle('write')
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

    <FormField :label="mode === 'read' ? 'Path (optional)' : 'Path'">
      <FormInput
        v-model="path"
        placeholder="/data/incoming"
        mono
        :invalid="mode !== 'read' && !path"
      />
      <div v-if="mode !== 'read' && !path" class="text-[10px] text-status-error leading-tight">
        Absolute path required for Watch / Read+Watch modes
      </div>
      <div v-else-if="!path" class="text-[10px] text-text-muted leading-tight">
        Leave empty to list msg.path from the incoming message (Read mode only).
      </div>
      <div v-else class="text-[10px] text-text-muted leading-tight">
        File Watch never reads file contents — chain a File Read to consume the changed file.
        msg.path overrides this in Read mode.
      </div>
    </FormField>

    <FormField label="Filter (glob)">
      <FormInput v-model="glob" placeholder="*" mono />
    </FormField>

    <FormCheckbox v-model="recursive" label="Recursive (descend into subfolders, Read mode only)" />

    <FormField label="Send as">
      <FormSelect
        :model-value="sendAs"
        :options="sendAsOptions"
        @update:model-value="sendAs = String($event)"
      />
    </FormField>

    <template v-if="showIncremental">
      <FormCheckbox v-model="incremental" label="Incremental (only changed entries since last scan)" />
    </template>
    <div v-else class="text-[10px] text-text-muted leading-tight">
      Read + Watch mode is always incremental. Each event triggers a re-scan;
      only entries with advanced modTime are emitted.
    </div>

    <template v-if="incremental || isReadPlusWatch">
      <FormCheckbox v-model="fromStart" label="Emit all existing entries on first access" />
    </template>

    <template v-if="isWatchMode">
      <FormField label="Watch events">
        <div class="flex flex-col gap-1">
          <FormCheckbox v-model="createT" label="Create" />
          <FormCheckbox v-model="writeT"  label="Write" />
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
      </FormField>

      <div class="text-[10px] text-status-warning leading-tight">
        v1 limitation: watcher is non-recursive even when "recursive" is set.
        Recursive walks apply to mode=read only.
      </div>
    </template>

    <FormField label="Root jail (optional)">
      <FormInput
        v-model="rootJail"
        placeholder="/data"
        mono
      />
      <div class="text-[10px] text-text-muted leading-tight">
        Resolved paths must stay inside this directory. Empty disables the check.
      </div>
    </FormField>
  </div>
</template>
