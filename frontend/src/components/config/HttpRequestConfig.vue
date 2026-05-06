<script setup lang="ts">
import { computed } from 'vue'
import FormField from '@/components/ui/FormField.vue'
import FormInput from '@/components/ui/FormInput.vue'
import FormSelect from '@/components/ui/FormSelect.vue'
import NumberInput from '@/components/ui/NumberInput.vue'
import FormCheckbox from '@/components/ui/FormCheckbox.vue'
import SectionHeader from '@/components/ui/SectionHeader.vue'
import IconButton from '@/components/ui/IconButton.vue'
import { useNodeProperty } from '@/composables/useNodeProperty'

const method = useNodeProperty<string>('method', 'GET')
const url = useNodeProperty<string>('url', '')
const responseFormat = useNodeProperty<string>('responseFormat', 'auto')
const bodyEncoding = useNodeProperty<string>('bodyEncoding', 'auto')
const timeout = useNodeProperty<number>('timeout', 30)
const followRedirects = useNodeProperty<boolean>('followRedirects', true)
const tlsInsecure = useNodeProperty<boolean>('tlsInsecure', false)
const errorMode = useNodeProperty<string>('errorMode', 'passthrough')
const headers = useNodeProperty<Record<string, string>>('headers', {})

interface AuthConfig {
  type?: string
  username?: string
  password?: string
  token?: string
}
const auth = useNodeProperty<AuthConfig>('auth', { type: 'none' })

const authType = computed({
  get: () => auth.value?.type ?? 'none',
  set: (v: string) => { auth.value = { ...(auth.value ?? {}), type: v } },
})
const authUsername = computed({
  get: () => auth.value?.username ?? '',
  set: (v: string) => { auth.value = { ...(auth.value ?? {}), username: v } },
})
const authPassword = computed({
  get: () => auth.value?.password ?? '',
  set: (v: string) => { auth.value = { ...(auth.value ?? {}), password: v } },
})
const authToken = computed({
  get: () => auth.value?.token ?? '',
  set: (v: string) => { auth.value = { ...(auth.value ?? {}), token: v } },
})

const methods = [
  { value: 'GET',     label: 'GET' },
  { value: 'POST',    label: 'POST' },
  { value: 'PUT',     label: 'PUT' },
  { value: 'PATCH',   label: 'PATCH' },
  { value: 'DELETE',  label: 'DELETE' },
  { value: 'HEAD',    label: 'HEAD' },
  { value: 'OPTIONS', label: 'OPTIONS' },
]

const responseFormats = [
  { value: 'auto',   label: 'Auto (by Content-Type)' },
  { value: 'string', label: 'String' },
  { value: 'json',   label: 'JSON' },
  { value: 'buffer', label: 'Buffer (number array)' },
]

const bodyEncodings = [
  { value: 'auto', label: 'Auto (by payload type)' },
  { value: 'json', label: 'JSON' },
  { value: 'form', label: 'Form (urlencoded)' },
  { value: 'text', label: 'Text' },
  { value: 'none', label: 'None (no body)' },
]

const authTypes = [
  { value: 'none',   label: 'None' },
  { value: 'basic',  label: 'Basic' },
  { value: 'bearer', label: 'Bearer Token' },
]

const errorModes = [
  { value: 'passthrough', label: 'Pass through' },
  { value: 'error',       label: 'Treat non-2xx as error' },
]

const headerEntries = computed<{ key: string; value: string }[]>({
  get: () => Object.entries(headers.value ?? {}).map(([key, value]) => ({ key, value })),
  set: () => {},
})

function addHeader() { headers.value = { ...(headers.value ?? {}), '': '' } }

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
    <FormField label="Method">
      <FormSelect
        :model-value="method"
        :options="methods"
        @update:model-value="method = String($event)"
      />
    </FormField>

    <FormField label="URL">
      <FormInput
        v-model="url"
        placeholder="https://api.example.com/p/{{payload.id}}"
        mono
      />
      <div class="text-[10px] text-terminal-text-dim leading-tight">
        Mustache substitution over msg fields (e.g. <code>{{ '{' + '{payload.id}' + '}' }}</code>). Empty → use msg.url.
      </div>
    </FormField>

    <FormField label="Return">
      <FormSelect
        :model-value="responseFormat"
        :options="responseFormats"
        @update:model-value="responseFormat = String($event)"
      />
    </FormField>

    <FormField label="Body encoding">
      <FormSelect
        :model-value="bodyEncoding"
        :options="bodyEncodings"
        @update:model-value="bodyEncoding = String($event)"
      />
    </FormField>

    <div class="grid grid-cols-2 gap-3">
      <FormField label="Timeout">
        <NumberInput
          :model-value="timeout"
          :min="1"
          unit="s"
          @update:model-value="timeout = Number($event)"
        />
      </FormField>
      <FormField label="Follow redirects">
        <FormCheckbox
          :model-value="followRedirects"
          label="Follow"
          @update:model-value="followRedirects = Boolean($event)"
        />
      </FormField>
    </div>

    <FormField label="TLS verification">
      <FormCheckbox
        :model-value="tlsInsecure"
        label="Skip TLS verify (insecure)"
        @update:model-value="tlsInsecure = Boolean($event)"
      />
      <div v-if="tlsInsecure" class="text-[10px] text-status-warn leading-tight">
        ⚠ Disables certificate verification — only use in development.
      </div>
    </FormField>

    <SectionHeader title="Authentication">
      <FormField label="Type">
        <FormSelect
          :model-value="authType"
          :options="authTypes"
          @update:model-value="authType = String($event)"
        />
      </FormField>
      <template v-if="authType === 'basic'">
        <FormField label="Username">
          <FormInput v-model="authUsername" mono />
        </FormField>
        <FormField label="Password">
          <FormInput v-model="authPassword" type="password" mono />
        </FormField>
      </template>
      <template v-if="authType === 'bearer'">
        <FormField label="Token">
          <FormInput v-model="authToken" mono />
        </FormField>
      </template>
    </SectionHeader>

    <SectionHeader title="Headers">
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
    </SectionHeader>

    <FormField label="Non-2xx response">
      <FormSelect
        :model-value="errorMode"
        :options="errorModes"
        @update:model-value="errorMode = String($event)"
      />
    </FormField>
  </div>
</template>
