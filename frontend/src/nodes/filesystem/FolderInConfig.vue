<script setup lang="ts">
import FormField from '@/components/ui/FormField.vue'
import FormInput from '@/components/ui/FormInput.vue'
import { useNodeProperty } from '@/composables/useNodeProperty'

// Phase 1: skeleton — only the path field. Mode, glob, recursive, sendAs,
// includeContent, watch options, incremental controls land in Phase 6.
const path = useNodeProperty<string>('path', '')
</script>

<template>
  <div class="flex flex-col gap-3 flex-1 min-h-0">
    <FormField label="Folder path">
      <FormInput
        v-model="path"
        placeholder="/data/incoming"
        mono
        :invalid="!path"
      />
      <div v-if="!path" class="text-[10px] text-status-error leading-tight">
        Absolute folder path required
      </div>
    </FormField>

    <div class="text-[10px] text-text-muted leading-tight">
      Folder In is in skeleton state — full configuration UI lands in upcoming phases.
    </div>
  </div>
</template>
