<script setup lang="ts">
import FormField from '@/components/ui/FormField.vue'
import FormInput from '@/components/ui/FormInput.vue'
import FormSelect from '@/components/ui/FormSelect.vue'
import NumberInput from '@/components/ui/NumberInput.vue'
import FormCheckbox from '@/components/ui/FormCheckbox.vue'
import { useNodeProperty } from '@/composables/useNodeProperty'

const host = useNodeProperty<string>('host', '')
const port = useNodeProperty<number>('port', 0)
const bindHost = useNodeProperty<string>('bindHost', '')
const mode = useNodeProperty<string>('mode', 'unicast')
const multicastTTL = useNodeProperty<number>('multicastTTL', 1)
const multicastLoopback = useNodeProperty<boolean>('multicastLoopback', true)
const reuseSocket = useNodeProperty<boolean>('reuseSocket', true)

const modes = [
  { value: 'unicast',   label: 'Unicast' },
  { value: 'broadcast', label: 'Broadcast (sets SO_BROADCAST)' },
  { value: 'multicast', label: 'Multicast (IPv4)' },
]
</script>

<template>
  <div class="flex flex-col gap-3">
    <FormField label="Mode">
      <FormSelect
        :model-value="mode"
        :options="modes"
        @update:model-value="mode = String($event)"
      />
    </FormField>

    <div class="grid grid-cols-2 gap-3">
      <FormField label="Host">
        <FormInput
          v-model="host"
          :placeholder="mode === 'multicast' ? '239.255.42.99' : (mode === 'broadcast' ? '255.255.255.255' : 'destination.example.com')"
          mono
        />
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
    <div class="text-[10px] text-terminal-text-dim leading-tight -mt-2">
      msg.host / msg.port override these per message.
    </div>

    <template v-if="mode === 'multicast'">
      <FormField label="Outbound interface (name, optional)">
        <FormInput v-model="bindHost" placeholder="eth0" mono />
        <div class="text-[10px] text-terminal-text-dim leading-tight">
          Pins the multicast egress to a specific interface. Empty = OS default.
        </div>
      </FormField>
      <div class="grid grid-cols-2 gap-3">
        <FormField label="TTL">
          <NumberInput
            :model-value="multicastTTL"
            :min="0"
            :max="255"
            @update:model-value="multicastTTL = Number($event)"
          />
        </FormField>
        <FormField>
          <FormCheckbox
            :model-value="multicastLoopback"
            label="Loopback"
            @update:model-value="multicastLoopback = Boolean($event)"
          />
        </FormField>
      </div>
    </template>

    <FormField>
      <FormCheckbox
        :model-value="reuseSocket"
        label="Keep one outbound socket across messages"
        @update:model-value="reuseSocket = Boolean($event)"
      />
    </FormField>
  </div>
</template>
