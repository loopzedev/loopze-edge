<script setup lang="ts">
import FormField from '@/components/ui/FormField.vue'
import FormInput from '@/components/ui/FormInput.vue'
import { useNodeProperty } from '@/composables/useNodeProperty'

// Phase 1: skeleton — only the path field. Mode, encoding, watch options,
// incremental cursor controls land in Phases 3–5.
const path = useNodeProperty<string>('path', '')
</script>

<template>
  <div class="flex flex-col gap-3 flex-1 min-h-0">
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
    </FormField>

    <div class="text-[10px] text-text-muted leading-tight">
      File In is in skeleton state — full configuration UI lands in upcoming phases.
    </div>
  </div>
</template>
