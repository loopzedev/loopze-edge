<script setup lang="ts">
import { computed } from 'vue'
import FormField from '@/components/ui/FormField.vue'
import FormInput from '@/components/ui/FormInput.vue'
import FormSelect from '@/components/ui/FormSelect.vue'
import NumberInput from '@/components/ui/NumberInput.vue'
import FormCheckbox from '@/components/ui/FormCheckbox.vue'
import SectionHeader from '@/components/ui/SectionHeader.vue'
import TlsConfigSection from '@/components/config/TlsConfigSection.vue'
import { useNodeProperty } from '@/composables/useNodeProperty'

const mode = useNodeProperty<string>('mode', 'server')
const host = useNodeProperty<string>('host', '0.0.0.0')
const port = useNodeProperty<number>('port', 7000)

const framing = useNodeProperty<string>('framing', 'stream')
const delimiter = useNodeProperty<string>('delimiter', '\\n')
const fixedLength = useNodeProperty<number>('fixedLength', 4)

interface LengthPrefix {
  bytes?: number
  endianness?: string
  includesHeader?: boolean
}
const lengthPrefix = useNodeProperty<LengthPrefix>('lengthPrefix', { bytes: 4, endianness: 'big', includesHeader: false })

const lpBytes = computed({
  get: () => lengthPrefix.value?.bytes ?? 4,
  set: (n: number) => { lengthPrefix.value = { ...(lengthPrefix.value ?? {}), bytes: n } },
})
const lpEndian = computed({
  get: () => lengthPrefix.value?.endianness ?? 'big',
  set: (v: string) => { lengthPrefix.value = { ...(lengthPrefix.value ?? {}), endianness: v } },
})
const lpIncludesHeader = computed({
  get: () => !!lengthPrefix.value?.includesHeader,
  set: (v: boolean) => { lengthPrefix.value = { ...(lengthPrefix.value ?? {}), includesHeader: v } },
})

const payloadEncoding = useNodeProperty<string>('payloadEncoding', 'buffer')
const maxFrameBytes = useNodeProperty<number>('maxFrameBytes', 1048576)
const keepAlive = useNodeProperty<boolean>('keepAlive', true)
const keepAliveInterval = useNodeProperty<number>('keepAliveInterval', 30)
const nodelay = useNodeProperty<boolean>('nodelay', false)
const emitCloseEvent = useNodeProperty<boolean>('emitCloseEvent', false)

// Server-only.
const allowedRemotes = useNodeProperty<string[]>('allowedRemotes', [])
const allowedRemotesCSV = computed({
  get: () => (allowedRemotes.value ?? []).join(', '),
  set: (s: string) => {
    allowedRemotes.value = s.split(',').map(x => x.trim()).filter(Boolean)
  },
})
const maxConnections = useNodeProperty<number>('maxConnections', 0)

// Client-only.
const reconnect = useNodeProperty<boolean>('reconnect', true)
const reconnectInitialDelay = useNodeProperty<number>('reconnectInitialDelay', 1000)
const reconnectMaxDelay = useNodeProperty<number>('reconnectMaxDelay', 30000)
const dialTimeout = useNodeProperty<number>('dialTimeout', 10)

const modes = [
  { value: 'server', label: 'Server (listen on port)' },
  { value: 'client', label: 'Client (connect to remote)' },
]

const framingModes = [
  { value: 'stream',         label: 'Stream (raw chunks)' },
  { value: 'delimiter',      label: 'Delimiter' },
  { value: 'length-prefix',  label: 'Length prefix' },
  { value: 'fixed-length',   label: 'Fixed length' },
]

const payloadEncodings = [
  { value: 'buffer', label: 'Buffer (number array)' },
  { value: 'string', label: 'String (UTF-8)' },
  { value: 'base64', label: 'Base64' },
]

const lpByteSizes = [
  { value: 1, label: '1 byte' },
  { value: 2, label: '2 bytes' },
  { value: 4, label: '4 bytes' },
  { value: 8, label: '8 bytes' },
]

const endianOptions = [
  { value: 'big',    label: 'Big-endian (network)' },
  { value: 'little', label: 'Little-endian' },
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
        <FormInput v-model="host" placeholder="0.0.0.0" mono />
      </FormField>
      <FormField label="Port">
        <NumberInput
          :model-value="port"
          :min="1"
          :max="65535"
          @update:model-value="port = Number($event)"
        />
      </FormField>
    </div>

    <FormField label="Framing">
      <FormSelect
        :model-value="framing"
        :options="framingModes"
        @update:model-value="framing = String($event)"
      />
    </FormField>

    <FormField v-if="framing === 'delimiter'" label="Delimiter (JS escapes: \n, \r\n, \xFF)">
      <FormInput v-model="delimiter" placeholder="\n" mono />
    </FormField>

    <template v-if="framing === 'length-prefix'">
      <div class="grid grid-cols-2 gap-3">
        <FormField label="Header bytes">
          <FormSelect
            :model-value="lpBytes"
            :options="lpByteSizes"
            @update:model-value="lpBytes = Number($event)"
          />
        </FormField>
        <FormField label="Endianness">
          <FormSelect
            :model-value="lpEndian"
            :options="endianOptions"
            @update:model-value="lpEndian = String($event)"
          />
        </FormField>
      </div>
      <FormField>
        <FormCheckbox
          :model-value="lpIncludesHeader"
          label="Length includes header bytes"
          @update:model-value="lpIncludesHeader = Boolean($event)"
        />
      </FormField>
    </template>

    <FormField v-if="framing === 'fixed-length'" label="Fixed frame size (bytes)">
      <NumberInput
        :model-value="fixedLength"
        :min="1"
        @update:model-value="fixedLength = Number($event)"
      />
    </FormField>

    <FormField label="Payload as">
      <FormSelect
        :model-value="payloadEncoding"
        :options="payloadEncodings"
        @update:model-value="payloadEncoding = String($event)"
      />
    </FormField>

    <div class="grid grid-cols-2 gap-3">
      <FormField label="Max frame bytes">
        <NumberInput
          :model-value="maxFrameBytes"
          :min="0"
          @update:model-value="maxFrameBytes = Number($event)"
        />
      </FormField>
      <FormField label="Keep-alive interval">
        <NumberInput
          :model-value="keepAliveInterval"
          :min="1"
          unit="s"
          @update:model-value="keepAliveInterval = Number($event)"
        />
      </FormField>
    </div>

    <div class="grid grid-cols-2 gap-3">
      <FormField>
        <FormCheckbox
          :model-value="keepAlive"
          label="TCP keep-alive"
          @update:model-value="keepAlive = Boolean($event)"
        />
      </FormField>
      <FormField>
        <FormCheckbox
          :model-value="nodelay"
          label="Disable Nagle (TCP_NODELAY)"
          @update:model-value="nodelay = Boolean($event)"
        />
      </FormField>
    </div>

    <FormField>
      <FormCheckbox
        :model-value="emitCloseEvent"
        label="Emit close marker on disconnect"
        @update:model-value="emitCloseEvent = Boolean($event)"
      />
      <div class="text-[10px] text-terminal-text-dim leading-tight">
        When enabled, the output also emits a message with <code>payload=null</code> and <code>_event=&quot;close&quot;</code> when a peer disconnects.
      </div>
    </FormField>

    <SectionHeader v-if="mode === 'server'" title="Server options">
      <FormField label="Max connections (0 = unlimited)">
        <NumberInput
          :model-value="maxConnections"
          :min="0"
          @update:model-value="maxConnections = Number($event)"
        />
      </FormField>
      <FormField label="Allowed remotes (CIDR, comma-separated)">
        <FormInput
          v-model="allowedRemotesCSV"
          placeholder="10.0.0.0/8, 192.168.1.0/24"
          mono
        />
      </FormField>
    </SectionHeader>

    <SectionHeader v-if="mode === 'client'" title="Client options">
      <FormField>
        <FormCheckbox
          :model-value="reconnect"
          label="Auto-reconnect with exponential backoff"
          @update:model-value="reconnect = Boolean($event)"
        />
      </FormField>
      <div class="grid grid-cols-2 gap-3">
        <FormField label="Initial backoff">
          <NumberInput
            :model-value="reconnectInitialDelay"
            :min="50"
            unit="ms"
            @update:model-value="reconnectInitialDelay = Number($event)"
          />
        </FormField>
        <FormField label="Max backoff">
          <NumberInput
            :model-value="reconnectMaxDelay"
            :min="100"
            unit="ms"
            @update:model-value="reconnectMaxDelay = Number($event)"
          />
        </FormField>
      </div>
      <FormField label="Dial timeout">
        <NumberInput
          :model-value="dialTimeout"
          :min="1"
          unit="s"
          @update:model-value="dialTimeout = Number($event)"
        />
      </FormField>
    </SectionHeader>

    <TlsConfigSection v-if="mode === 'client'" />
  </div>
</template>
