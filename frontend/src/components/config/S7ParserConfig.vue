<script setup lang="ts">
import { computed } from 'vue'
import FormInput from '@/components/ui/FormInput.vue'
import FormField from '@/components/ui/FormField.vue'
import FormCheckbox from '@/components/ui/FormCheckbox.vue'
import NumberInput from '@/components/ui/NumberInput.vue'
import ToggleGroup from '@/components/ui/ToggleGroup.vue'
import SectionHeader from '@/components/ui/SectionHeader.vue'
import S7ParserLayoutEditor from './S7ParserLayoutEditor.vue'
import { useNodeProperty } from '@/composables/useNodeProperty'

const action = useNodeProperty<string>('action', 'auto')
// Default parseFrom == "payload" matches the s7-read block-mode output, so
// the typical chain `[s7-read block] → [s7-parser] → [debug]` works without
// extra configuration. encodeFrom defaults to "payload" for the same reason
// on the write path.
const parseFrom = useNodeProperty<string>('parseFrom', 'payload')
const encodeFrom = useNodeProperty<string>('encodeFrom', 'payload')
const blockLength = useNodeProperty<number>('blockLength', 0)
const preserveBytes = useNodeProperty<boolean>('preserveBytes', false)

const ACTIONS = [
  { value: 'auto',   label: 'Auto'   },
  { value: 'parse',  label: 'Parse'  },
  { value: 'encode', label: 'Encode' },
]

const showParseFrom = computed(() => action.value !== 'encode')
const showEncodeFrom = computed(() => action.value !== 'parse')
</script>

<template>
  <div class="flex flex-col gap-3">
    <FormField label="Action" hint="Auto: map → encode, byte array → parse">
      <ToggleGroup v-model="action" :options="ACTIONS" />
    </FormField>

    <FormField v-if="showParseFrom" label="Parse from">
      <FormInput v-model="parseFrom" placeholder="payload" mono>
        <template #prefix>msg.</template>
      </FormInput>
    </FormField>

    <FormField v-if="showEncodeFrom" label="Encode from">
      <FormInput v-model="encodeFrom" placeholder="payload" mono>
        <template #prefix>msg.</template>
      </FormInput>
    </FormField>

    <FormField
      label="Block length"
      hint="0 = derive from layout. Set explicitly to validate input size on parse."
    >
      <NumberInput v-model="blockLength" :min="0" unit="bytes" />
    </FormField>

    <SectionHeader title="Parse / encode behaviour">
      <div class="flex flex-col gap-1.5">
        <FormCheckbox
          v-model="preserveBytes"
          label="Forward raw bytes as msg.bytes (parse only)"
        />
        <p class="text-[10px] text-terminal-text-dim leading-relaxed">
          Layout offsets are buffer-relative — field at offset 12 lives at
          <span class="font-mono">payload[12]</span>. Configure
          <span class="font-mono">start</span> on the downstream
          <span class="font-mono">s7-write</span> block to position the
          buffer in the PLC.
        </p>
      </div>
    </SectionHeader>

    <S7ParserLayoutEditor />
  </div>
</template>
