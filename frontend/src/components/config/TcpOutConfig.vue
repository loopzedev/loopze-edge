<script setup lang="ts">
import FormField from '@/components/ui/FormField.vue'
import FormInput from '@/components/ui/FormInput.vue'
import FormSelect from '@/components/ui/FormSelect.vue'
import NumberInput from '@/components/ui/NumberInput.vue'
import FormCheckbox from '@/components/ui/FormCheckbox.vue'
import TlsConfigSection from '@/components/config/TlsConfigSection.vue'
import { useNodeProperty } from '@/composables/useNodeProperty'

const mode = useNodeProperty<string>('mode', 'reply')
const host = useNodeProperty<string>('host', '')
const port = useNodeProperty<number>('port', 0)
const targetTcpIn = useNodeProperty<string>('targetTcpIn', '')
const keepConnection = useNodeProperty<boolean>('keepConnection', true)
const appendDelimiter = useNodeProperty<string>('appendDelimiter', '')
const closeAfterSend = useNodeProperty<boolean>('closeAfterSend', false)
const dialTimeout = useNodeProperty<number>('dialTimeout', 10)
const writeTimeout = useNodeProperty<number>('writeTimeout', 10)
const outboundQueueSize = useNodeProperty<number>('outboundQueueSize', 64)

const modes = [
  { value: 'reply',            label: 'Reply (use msg.session)' },
  { value: 'server-broadcast', label: 'Server broadcast' },
  { value: 'client',           label: 'Client (dial host:port)' },
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

    <template v-if="mode === 'reply'">
      <FormField>
        <FormCheckbox
          :model-value="closeAfterSend"
          label="Close session after send"
          @update:model-value="closeAfterSend = Boolean($event)"
        />
      </FormField>
    </template>

    <template v-if="mode === 'server-broadcast'">
      <FormField label="Target TCP-In node ID">
        <FormInput
          v-model="targetTcpIn"
          placeholder="tcp-in-1"
          mono
        />
        <div class="text-[10px] text-terminal-text-dim leading-tight">
          Broadcast is sent to every active session whose owner is this node.
        </div>
      </FormField>
    </template>

    <template v-if="mode === 'client'">
      <div class="grid grid-cols-2 gap-3">
        <FormField label="Host">
          <FormInput v-model="host" placeholder="device.local" mono />
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

      <FormField>
        <FormCheckbox
          :model-value="keepConnection"
          label="Keep connection across messages"
          @update:model-value="keepConnection = Boolean($event)"
        />
      </FormField>

      <div class="grid grid-cols-2 gap-3">
        <FormField label="Dial timeout">
          <NumberInput
            :model-value="dialTimeout"
            :min="1"
            unit="s"
            @update:model-value="dialTimeout = Number($event)"
          />
        </FormField>
        <FormField label="Outbound queue size">
          <NumberInput
            :model-value="outboundQueueSize"
            :min="1"
            @update:model-value="outboundQueueSize = Number($event)"
          />
        </FormField>
      </div>

      <FormField>
        <FormCheckbox
          :model-value="closeAfterSend"
          label="Close after send"
          @update:model-value="closeAfterSend = Boolean($event)"
        />
      </FormField>
    </template>

    <FormField label="Append after payload (delimiter)">
      <FormInput
        v-model="appendDelimiter"
        placeholder="(none) — JS escapes: \n, \r\n"
        mono
      />
    </FormField>

    <FormField label="Write timeout">
      <NumberInput
        :model-value="writeTimeout"
        :min="1"
        unit="s"
        @update:model-value="writeTimeout = Number($event)"
      />
    </FormField>

    <TlsConfigSection v-if="mode === 'client'" />
  </div>
</template>
