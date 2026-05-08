<script setup lang="ts">
import { computed } from 'vue'
import FormField from '@/components/ui/FormField.vue'
import FormInput from '@/components/ui/FormInput.vue'
import FormSelect from '@/components/ui/FormSelect.vue'
import NumberInput from '@/components/ui/NumberInput.vue'
import { useNodeProperty } from '@/composables/useNodeProperty'

const host = useNodeProperty<string>('host', '0.0.0.0')
const port = useNodeProperty<number>('port', 5000)
const payloadEncoding = useNodeProperty<string>('payloadEncoding', 'buffer')
const maxDatagramBytes = useNodeProperty<number>('maxDatagramBytes', 65507)
const multicastGroups = useNodeProperty<string[]>('multicastGroups', [])
const allowedRemotes = useNodeProperty<string[]>('allowedRemotes', [])

const multicastCSV = computed({
  get: () => (multicastGroups.value ?? []).join(', '),
  set: (s: string) => {
    multicastGroups.value = s.split(',').map(x => x.trim()).filter(Boolean)
  },
})
const allowedCSV = computed({
  get: () => (allowedRemotes.value ?? []).join(', '),
  set: (s: string) => {
    allowedRemotes.value = s.split(',').map(x => x.trim()).filter(Boolean)
  },
})

const payloadEncodings = [
  { value: 'buffer', label: 'Buffer (number array)' },
  { value: 'string', label: 'String (UTF-8)' },
  { value: 'base64', label: 'Base64' },
]
</script>

<template>
  <div class="flex flex-col gap-3">
    <div class="grid grid-cols-2 gap-3">
      <FormField label="Bind host">
        <FormInput v-model="host" placeholder="0.0.0.0" mono />
      </FormField>
      <FormField label="Port">
        <NumberInput
          :model-value="port"
          :min="0"
          :max="65535"
          @update:model-value="port = Number($event)"
        />
      </FormField>
    </div>

    <FormField label="Payload as">
      <FormSelect
        :model-value="payloadEncoding"
        :options="payloadEncodings"
        @update:model-value="payloadEncoding = String($event)"
      />
    </FormField>

    <FormField label="Max datagram bytes">
      <NumberInput
        :model-value="maxDatagramBytes"
        :min="1"
        :max="65535"
        @update:model-value="maxDatagramBytes = Number($event)"
      />
    </FormField>

    <FormField label="Multicast groups (comma-separated, IPv4)">
      <FormInput
        v-model="multicastCSV"
        placeholder="224.0.0.251 or 239.255.42.99%eth0"
        mono
      />
      <div class="text-[10px] text-terminal-text-dim leading-tight">
        Append <code>%iface</code> to pin a specific interface (e.g. <code>%eth0</code>).
      </div>
    </FormField>

    <FormField label="Allowed remotes (CIDR, comma-separated)">
      <FormInput
        v-model="allowedCSV"
        placeholder="10.0.0.0/8, 192.168.1.0/24"
        mono
      />
      <div class="text-[10px] text-terminal-text-dim leading-tight">
        Datagrams from senders outside the allowlist are dropped silently.
      </div>
    </FormField>
  </div>
</template>
