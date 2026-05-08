<script setup lang="ts">
import { computed } from 'vue'
import FormField from '@/components/ui/FormField.vue'
import FormInput from '@/components/ui/FormInput.vue'
import FormCheckbox from '@/components/ui/FormCheckbox.vue'
import SectionHeader from '@/components/ui/SectionHeader.vue'
import { useNodeProperty } from '@/composables/useNodeProperty'

interface TLSBlock {
  enabled?: boolean
  serverName?: string
  caBundle?: string
  clientCert?: string
  clientKey?: string
  insecureSkipVerify?: boolean
}

const tls = useNodeProperty<TLSBlock>('tls', {})

const enabled = computed({
  get: () => !!tls.value?.enabled,
  set: (v: boolean) => { tls.value = { ...(tls.value ?? {}), enabled: v } },
})
const serverName = computed({
  get: () => tls.value?.serverName ?? '',
  set: (v: string) => { tls.value = { ...(tls.value ?? {}), serverName: v } },
})
const caBundle = computed({
  get: () => tls.value?.caBundle ?? '',
  set: (v: string) => { tls.value = { ...(tls.value ?? {}), caBundle: v } },
})
const clientCert = computed({
  get: () => tls.value?.clientCert ?? '',
  set: (v: string) => { tls.value = { ...(tls.value ?? {}), clientCert: v } },
})
const clientKey = computed({
  get: () => tls.value?.clientKey ?? '',
  set: (v: string) => { tls.value = { ...(tls.value ?? {}), clientKey: v } },
})
const insecureSkipVerify = computed({
  get: () => !!tls.value?.insecureSkipVerify,
  set: (v: boolean) => { tls.value = { ...(tls.value ?? {}), insecureSkipVerify: v } },
})
</script>

<template>
  <SectionHeader title="TLS">
    <FormField>
      <FormCheckbox
        :model-value="enabled"
        label="Enable TLS"
        @update:model-value="enabled = Boolean($event)"
      />
    </FormField>

    <template v-if="enabled">
      <FormField label="Server name (SNI)">
        <FormInput
          v-model="serverName"
          placeholder="device.example.com"
          mono
        />
        <div class="text-[10px] text-terminal-text-dim leading-tight">
          Falls back to the dial host when empty.
        </div>
      </FormField>

      <FormField label="CA bundle (PEM, optional)">
        <textarea
          v-model="caBundle"
          rows="4"
          class="w-full font-mono text-[11px] bg-terminal-input-bg border border-terminal-border rounded px-2 py-1 resize-y"
          placeholder="-----BEGIN CERTIFICATE-----&#10;…&#10;-----END CERTIFICATE-----"
        />
        <div class="text-[10px] text-terminal-text-dim leading-tight">
          Empty = system trust roots.
        </div>
      </FormField>

      <FormField label="Client certificate (PEM, optional)">
        <textarea
          v-model="clientCert"
          rows="4"
          class="w-full font-mono text-[11px] bg-terminal-input-bg border border-terminal-border rounded px-2 py-1 resize-y"
          placeholder="-----BEGIN CERTIFICATE-----&#10;…&#10;-----END CERTIFICATE-----"
        />
      </FormField>
      <FormField label="Client private key (PEM, optional)">
        <textarea
          v-model="clientKey"
          rows="4"
          class="w-full font-mono text-[11px] bg-terminal-input-bg border border-terminal-border rounded px-2 py-1 resize-y"
          placeholder="-----BEGIN PRIVATE KEY-----&#10;…&#10;-----END PRIVATE KEY-----"
        />
        <div class="text-[10px] text-terminal-text-dim leading-tight">
          Set both certificate and key for mTLS, or leave both empty.
        </div>
      </FormField>

      <FormField>
        <FormCheckbox
          :model-value="insecureSkipVerify"
          label="Skip TLS verification (insecure)"
          @update:model-value="insecureSkipVerify = Boolean($event)"
        />
        <div v-if="insecureSkipVerify" class="text-[10px] text-status-warn leading-tight">
          ⚠ Disables certificate verification — only use in development.
        </div>
      </FormField>
    </template>
  </SectionHeader>
</template>
