<script setup lang="ts">
import { computed, onMounted } from 'vue'

import FormSelect from '@/components/ui/FormSelect.vue'
import { useCertsStore } from '@/stores/certsStore'
import type { CertType } from '@/types/cert'

const props = defineProps<{
  modelValue: string
  type: CertType
  placeholder?: string
}>()

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

const store = useCertsStore()

onMounted(() => {
  void store.ensureLoaded()
})

// radix-vue's SelectItem refuses an empty-string value, so we use a
// non-empty sentinel internally and translate at the v-model boundary.
// Without this the entire dropdown silently breaks on the "— none —"
// row and clicks become no-ops.
const NONE_VALUE = '__none__'

const options = computed(() => {
  const base = [{ value: NONE_VALUE, label: '— none —' }]
  const filtered = store.byType(props.type).map((c) => ({
    value: c.id,
    label: c.name ? `${c.name} (${c.id})` : c.id,
  }))
  return [...base, ...filtered]
})

const selected = computed({
  get: () => (props.modelValue ? props.modelValue : NONE_VALUE),
  set: (v: string) => emit('update:modelValue', v === NONE_VALUE ? '' : v),
})

const selectedEntry = computed(() =>
  store.certs.find((c) => c.id === props.modelValue),
)

const noEntriesAvailable = computed(
  () => store.loaded && store.byType(props.type).length === 0,
)

function formatExpiry(iso: string | undefined): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  return d.toISOString().slice(0, 10)
}
</script>

<template>
  <div class="flex flex-col gap-1">
    <FormSelect
      :model-value="selected"
      :options="options"
      :placeholder="placeholder ?? 'Select stored certificate'"
      @update:model-value="(v) => (selected = String(v))"
    />
    <div
      v-if="selectedEntry"
      class="text-[10px] text-terminal-text-dim leading-tight font-mono"
    >
      <div v-if="selectedEntry.subject">{{ selectedEntry.subject }}</div>
      <div v-if="selectedEntry.notAfter">
        Expires {{ formatExpiry(selectedEntry.notAfter) }}
      </div>
      <div v-if="selectedEntry.fingerprint" class="truncate">
        sha256:{{ selectedEntry.fingerprint.slice(0, 16) }}…
      </div>
    </div>
    <div
      v-else-if="noEntriesAvailable"
      class="text-[10px] text-terminal-text-dim leading-tight"
    >
      No {{ props.type }} entries in the cert store yet —
      <router-link to="/certs" class="text-accent hover:underline">add one</router-link>
      .
    </div>
    <div v-if="store.error" class="text-[10px] text-status-error leading-tight">
      {{ store.error }}
    </div>
  </div>
</template>
