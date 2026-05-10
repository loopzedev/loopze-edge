<script setup lang="ts">
import { computed } from 'vue'
import FormField from '@/components/ui/FormField.vue'
import FormInput from '@/components/ui/FormInput.vue'
import NumberInput from '@/components/ui/NumberInput.vue'
import IconButton from '@/components/ui/IconButton.vue'
import { useNodeProperty } from '@/composables/useNodeProperty'

const statusCode = useNodeProperty<number>('statusCode', 200)
const headers = useNodeProperty<Record<string, string>>('headers', {})

const headerEntries = computed<{ key: string; value: string }[]>({
  get: () => Object.entries(headers.value ?? {}).map(([key, value]) => ({ key, value })),
  set: () => { /* set is handled via mutators below */ },
})

function addHeader() {
  headers.value = { ...(headers.value ?? {}), '': '' }
}

function updateHeaderKey(oldKey: string, newKey: string) {
  if (oldKey === newKey) return
  const next: Record<string, string> = {}
  for (const [k, v] of Object.entries(headers.value ?? {})) {
    next[k === oldKey ? newKey : k] = v
  }
  headers.value = next
}

function updateHeaderValue(key: string, value: string) {
  headers.value = { ...(headers.value ?? {}), [key]: value }
}

function removeHeader(key: string) {
  const next = { ...(headers.value ?? {}) }
  delete next[key]
  headers.value = next
}
</script>

<template>
  <div class="flex flex-col gap-3">
    <FormField label="Status code">
      <NumberInput
        :model-value="statusCode"
        :min="100"
        :max="599"
        @update:model-value="statusCode = Number($event)"
      />
      <div class="text-[10px] text-terminal-text-dim leading-tight">
        Overridden by msg.statusCode
      </div>
    </FormField>

    <FormField label="Headers">
      <div class="flex flex-col gap-1.5">
        <div
          v-for="entry in headerEntries"
          :key="entry.key"
          class="flex gap-1.5 items-center"
        >
          <FormInput
            :model-value="entry.key"
            placeholder="header name"
            mono
            class="flex-1"
            @update:model-value="updateHeaderKey(entry.key, String($event))"
          />
          <FormInput
            :model-value="entry.value"
            placeholder="value"
            mono
            class="flex-1"
            @update:model-value="updateHeaderValue(entry.key, String($event))"
          />
          <IconButton title="Remove" @click="removeHeader(entry.key)">×</IconButton>
        </div>
        <button
          type="button"
          class="text-[10px] uppercase tracking-wider text-terminal-text-dim hover:text-accent transition-colors text-left"
          @click="addHeader"
        >
          + Add header
        </button>
      </div>
    </FormField>

    <p class="text-[10px] text-terminal-text-dim leading-tight">
      msg.headers and msg.cookies override or extend these per message.
    </p>
  </div>
</template>
