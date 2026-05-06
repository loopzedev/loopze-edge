<script setup lang="ts">
import { computed } from 'vue'
import FormField from '@/components/ui/FormField.vue'
import FormInput from '@/components/ui/FormInput.vue'
import FormSelect from '@/components/ui/FormSelect.vue'
import NumberInput from '@/components/ui/NumberInput.vue'
import SectionHeader from '@/components/ui/SectionHeader.vue'
import FormCheckbox from '@/components/ui/FormCheckbox.vue'
import { useNodeProperty } from '@/composables/useNodeProperty'

const method = useNodeProperty<string>('method', 'GET')
const path = useNodeProperty<string>('path', '/endpoint')
const bodyParse = useNodeProperty<string>('bodyParse', 'auto')
const maxBodyBytes = useNodeProperty<number>('maxBodyBytes', 1048576)
const responseTimeout = useNodeProperty<number>('responseTimeout', 30)

interface CORSConfig {
  origins?: string[]
  headers?: string[]
  methods?: string[]
  credentials?: boolean
  maxAge?: number
}
const cors = useNodeProperty<CORSConfig | null>('cors', null)

const corsEnabled = computed({
  get: () => cors.value !== null && cors.value !== undefined,
  set: (v: boolean) => {
    cors.value = v ? { origins: ['*'], maxAge: 600 } : null
  },
})

const corsOriginsCSV = computed({
  get: () => (cors.value?.origins ?? []).join(', '),
  set: (s: string) => {
    cors.value = {
      ...(cors.value ?? {}),
      origins: s.split(',').map(x => x.trim()).filter(Boolean),
    }
  },
})

const corsHeadersCSV = computed({
  get: () => (cors.value?.headers ?? []).join(', '),
  set: (s: string) => {
    cors.value = {
      ...(cors.value ?? {}),
      headers: s.split(',').map(x => x.trim()).filter(Boolean),
    }
  },
})

const corsCredentials = computed({
  get: () => !!cors.value?.credentials,
  set: (v: boolean) => {
    cors.value = { ...(cors.value ?? {}), credentials: v }
  },
})

const corsMaxAge = computed({
  get: () => cors.value?.maxAge ?? 600,
  set: (n: number) => {
    cors.value = { ...(cors.value ?? {}), maxAge: n }
  },
})

const methods = [
  { value: 'GET',     label: 'GET' },
  { value: 'POST',    label: 'POST' },
  { value: 'PUT',     label: 'PUT' },
  { value: 'PATCH',   label: 'PATCH' },
  { value: 'DELETE',  label: 'DELETE' },
  { value: 'HEAD',    label: 'HEAD' },
  { value: 'OPTIONS', label: 'OPTIONS' },
  { value: '*',       label: 'Any' },
]

const bodyParseModes = [
  { value: 'auto',   label: 'Auto (by Content-Type)' },
  { value: 'string', label: 'String (UTF-8)' },
  { value: 'json',   label: 'JSON' },
  { value: 'buffer', label: 'Buffer (number array)' },
  { value: 'none',   label: 'None (skip body)' },
]
</script>

<template>
  <div class="flex flex-col gap-3">
    <FormField label="Method">
      <FormSelect
        :model-value="method"
        :options="methods"
        @update:model-value="method = String($event)"
      />
    </FormField>

    <FormField label="Path">
      <FormInput
        v-model="path"
        placeholder="/webhook"
        mono
        :invalid="!path?.startsWith('/')"
      >
        <template #prefix>/endpoint</template>
      </FormInput>
      <div v-if="!path?.startsWith('/')" class="text-[10px] text-status-error leading-tight">
        Path must start with /
      </div>
    </FormField>

    <FormField label="Parse body as">
      <FormSelect
        :model-value="bodyParse"
        :options="bodyParseModes"
        @update:model-value="bodyParse = String($event)"
      />
    </FormField>

    <div class="grid grid-cols-2 gap-3">
      <FormField label="Max body bytes">
        <NumberInput
          :model-value="maxBodyBytes"
          :min="0"
          @update:model-value="maxBodyBytes = Number($event)"
        />
      </FormField>
      <FormField label="Response timeout">
        <NumberInput
          :model-value="responseTimeout"
          :min="0"
          unit="s"
          @update:model-value="responseTimeout = Number($event)"
        />
      </FormField>
    </div>

    <SectionHeader title="CORS">
      <FormCheckbox
        :model-value="corsEnabled"
        label="Enable CORS"
        @update:model-value="corsEnabled = Boolean($event)"
      />
      <template v-if="corsEnabled">
        <FormField label="Allowed origins (comma-separated, * for all)">
          <FormInput v-model="corsOriginsCSV" placeholder="* or https://app.example.com" mono />
        </FormField>
        <FormField label="Allowed headers (comma-separated)">
          <FormInput v-model="corsHeadersCSV" placeholder="content-type, authorization" mono />
        </FormField>
        <div class="grid grid-cols-2 gap-3">
          <FormField label="Allow credentials">
            <FormCheckbox
              :model-value="corsCredentials"
              label="Allow credentials"
              @update:model-value="corsCredentials = Boolean($event)"
            />
          </FormField>
          <FormField label="Max-Age">
            <NumberInput
              :model-value="corsMaxAge"
              :min="0"
              unit="s"
              @update:model-value="corsMaxAge = Number($event)"
            />
          </FormField>
        </div>
      </template>
    </SectionHeader>
  </div>
</template>
