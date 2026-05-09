<script setup lang="ts">
import { computed, watch } from 'vue'
import FormField from '@/components/ui/FormField.vue'
import FormInput from '@/components/ui/FormInput.vue'
import FormCheckbox from '@/components/ui/FormCheckbox.vue'
import SectionHeader from '@/components/ui/SectionHeader.vue'
import ToggleGroup from '@/components/ui/ToggleGroup.vue'
import CertSelector from '@/components/config/CertSelector.vue'
import { useNodeProperty } from '@/composables/useNodeProperty'

interface TLSBlock {
  enabled?: boolean
  serverName?: string
  caBundle?: string
  clientCert?: string
  clientKey?: string
  caBundleRef?: string
  clientPairRef?: string
  insecureSkipVerify?: boolean
}

type SourceMode = 'inline' | 'ref'

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
const caBundleRef = computed({
  get: () => tls.value?.caBundleRef ?? '',
  set: (v: string) => { tls.value = { ...(tls.value ?? {}), caBundleRef: v } },
})
const clientPairRef = computed({
  get: () => tls.value?.clientPairRef ?? '',
  set: (v: string) => { tls.value = { ...(tls.value ?? {}), clientPairRef: v } },
})
const insecureSkipVerify = computed({
  get: () => !!tls.value?.insecureSkipVerify,
  set: (v: boolean) => { tls.value = { ...(tls.value ?? {}), insecureSkipVerify: v } },
})

// Source mode is implicit in which fields are set today; the user
// flips it explicitly via the toggle. Switching modes clears the
// fields belonging to the inactive mode so the backend never sees a
// conflicting block (the API rejects mixed inline + ref).
const mode = computed<SourceMode>(() => {
  if (caBundleRef.value || clientPairRef.value) return 'ref'
  return 'inline'
})

function setMode(next: SourceMode) {
  if (next === mode.value) return
  if (next === 'inline') {
    tls.value = {
      ...(tls.value ?? {}),
      caBundleRef: '',
      clientPairRef: '',
    }
  } else {
    tls.value = {
      ...(tls.value ?? {}),
      caBundle: '',
      clientCert: '',
      clientKey: '',
    }
  }
}

// If the user disables the block, drop everything that's not user-edited
// metadata so a re-enable doesn't silently retain stale fields.
watch(enabled, (next, prev) => {
  if (prev && !next) {
    tls.value = { enabled: false }
  }
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
      <FormField label="Certificate source">
        <ToggleGroup
          :model-value="mode"
          :options="[
            { value: 'inline', label: 'Inline PEM' },
            { value: 'ref',    label: 'Stored cert' },
          ]"
          @update:model-value="(v) => setMode(v as SourceMode)"
        />
        <div class="text-[10px] text-terminal-text-dim leading-tight">
          Inline keeps PEM in this flow's JSON (encrypted at rest).
          Stored references the central cert store so one entry can
          back many flows and rotate independently.
        </div>
      </FormField>

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

      <!-- Inline-PEM mode (legacy / per-flow) -->
      <template v-if="mode === 'inline'">
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
      </template>

      <!-- Stored-cert mode (centralised, rotated independently) -->
      <template v-else>
        <FormField label="CA bundle">
          <CertSelector
            v-model="caBundleRef"
            type="ca-bundle"
            placeholder="— system roots —"
          />
        </FormField>
        <FormField label="Client cert + key (mTLS)">
          <CertSelector
            v-model="clientPairRef"
            type="client-pair"
            placeholder="— no client auth —"
          />
        </FormField>
        <div class="text-[10px] text-terminal-text-dim leading-tight">
          Manage stored entries under
          <router-link to="/certs" class="text-accent hover:underline">
            Certificates
          </router-link>
          .
        </div>
      </template>

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
