<script setup lang="ts">
import { computed } from 'vue'
import FormField from '@/components/ui/FormField.vue'
import FormInput from '@/components/ui/FormInput.vue'
import FormSelect from '@/components/ui/FormSelect.vue'
import NumberInput from '@/components/ui/NumberInput.vue'
import FormCheckbox from '@/components/ui/FormCheckbox.vue'
import TlsConfigSection from '@/components/config/TlsConfigSection.vue'
import { useNodeProperty } from '@/composables/useNodeProperty'


const host = useNodeProperty<string>('host', '')
const port = useNodeProperty<string>('port', '')
const terminator = useNodeProperty<string>('terminator', 'time')
const delimiter = useNodeProperty<string>('delimiter', '\\n')
const responseLength = useNodeProperty<number>('responseLength', 0)

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

const appendDelimiter = useNodeProperty<string>('appendDelimiter', '')
const responseEncoding = useNodeProperty<string>('responseEncoding', 'buffer')
const responseTimeout = useNodeProperty<number>('responseTimeout', 5000)
const dialTimeout = useNodeProperty<number>('dialTimeout', 10)
const keepConnection = useNodeProperty<boolean>('keepConnection', false)

const terminators = [
  { value: 'time',          label: 'Time (read until timeout)' },
  { value: 'delimiter',     label: 'Delimiter (read until match)' },
  { value: 'length',        label: 'Length (read N bytes)' },
  { value: 'length-prefix', label: 'Length prefix' },
  { value: 'close',         label: 'Close (read until peer FIN)' },
]

const responseEncodings = [
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
    <div class="grid grid-cols-2 gap-3">
      <FormField label="Host">
        <FormInput v-model="host" placeholder="device.local" mono />
      </FormField>
      <FormField label="Port">
        <FormInput v-model="port" placeholder="9100" mono />
      </FormField>
    </div>
    <div class="text-[10px] text-terminal-text-dim leading-tight -mt-2">
      Mustache templating supported (e.g. <code>{{ '{' + '{target.host}' + '}' }}</code>). msg.host / msg.port override.
    </div>

    <FormField label="Terminator">
      <FormSelect
        :model-value="terminator"
        :options="terminators"
        @update:model-value="terminator = String($event)"
      />
    </FormField>

    <FormField v-if="terminator === 'delimiter'" label="Delimiter (JS escapes)">
      <FormInput v-model="delimiter" placeholder="\n" mono />
    </FormField>

    <FormField v-if="terminator === 'length'" label="Response length (bytes)">
      <NumberInput
        :model-value="responseLength"
        :min="1"
        @update:model-value="responseLength = Number($event)"
      />
    </FormField>

    <template v-if="terminator === 'length-prefix'">
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

    <FormField label="Append after payload (delimiter)">
      <FormInput
        v-model="appendDelimiter"
        placeholder="(none) — JS escapes: \n, \r\n"
        mono
      />
    </FormField>

    <FormField label="Return">
      <FormSelect
        :model-value="responseEncoding"
        :options="responseEncodings"
        @update:model-value="responseEncoding = String($event)"
      />
    </FormField>

    <div class="grid grid-cols-2 gap-3">
      <FormField label="Response timeout">
        <NumberInput
          :model-value="responseTimeout"
          :min="1"
          unit="ms"
          @update:model-value="responseTimeout = Number($event)"
        />
      </FormField>
      <FormField label="Dial timeout">
        <NumberInput
          :model-value="dialTimeout"
          :min="1"
          unit="s"
          @update:model-value="dialTimeout = Number($event)"
        />
      </FormField>
    </div>

    <FormField>
      <FormCheckbox
        :model-value="keepConnection"
        :disabled="terminator === 'close'"
        label="Keep connection open across requests"
        @update:model-value="keepConnection = Boolean($event)"
      />
      <div class="text-[10px] text-terminal-text-dim leading-tight">
        Reuses one TCP connection for every message — fewer dials, lower latency.
        On a stale connection (peer dropped, NAT timeout) the node redials transparently once.
        <span v-if="terminator === 'close'" class="text-status-warn">
          Disabled for terminator=close (peer FIN ends the connection).
        </span>
      </div>
    </FormField>

    <TlsConfigSection />
  </div>
</template>
