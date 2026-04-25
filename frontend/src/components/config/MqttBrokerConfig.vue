<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useFlowStore } from '@/stores/flowStore'
import { useUiStore } from '@/stores/uiStore'
import FormInput from '@/components/ui/FormInput.vue'
import FormCheckbox from '@/components/ui/FormCheckbox.vue'
import FormField from '@/components/ui/FormField.vue'
import NumberInput from '@/components/ui/NumberInput.vue'

const props = defineProps<{
  configId?: string
}>()

const flowStore = useFlowStore()
const ui = useUiStore()

const name = ref('')
const host = ref('localhost')
const port = ref(1883)
const clientId = ref('')
const username = ref('')
const password = ref('')
const keepalive = ref(60)
const cleanSession = ref(true)
const useTLS = ref(false)

const isEditing = ref(false)

onMounted(() => {
  if (props.configId) {
    const existing = flowStore.configs.find((c) => c.id === props.configId)
    if (existing) {
      isEditing.value = true
      name.value = existing.name ?? ''
      const cfg = existing.config ?? {}
      host.value = (cfg.host as string) ?? 'localhost'
      port.value = (cfg.port as number) ?? 1883
      clientId.value = (cfg.clientId as string) ?? ''
      username.value = (cfg.username as string) ?? ''
      password.value = (cfg.password as string) ?? ''
      keepalive.value = (cfg.keepalive as number) ?? 60
      cleanSession.value = (cfg.cleanSession as boolean) ?? true
      useTLS.value = (cfg.useTLS as boolean) ?? false
    }
  }
})

function generateId(): string {
  return crypto.randomUUID()
}

function save() {
  const config = {
    host: host.value,
    port: port.value,
    clientId: clientId.value,
    username: username.value,
    password: password.value,
    keepalive: keepalive.value,
    cleanSession: cleanSession.value,
    useTLS: useTLS.value,
  }

  if (isEditing.value && props.configId) {
    flowStore.updateConfig(props.configId, { name: name.value, config })
  } else {
    flowStore.addConfig({
      id: generateId(),
      type: 'mqtt-broker',
      name: name.value,
      config,
    })
  }

  cancel()
}

function cancel() {
  ui.clearConfigEditor()
}
</script>

<template>
  <div class="flex flex-col h-full">
    <div class="flex-1 overflow-y-auto px-4 py-3 flex flex-col gap-3">
      <div class="flex items-center gap-2 mb-1">
        <span class="text-accent text-sm font-bold uppercase tracking-wider">
          {{ isEditing ? 'Edit' : 'New' }} MQTT Broker
        </span>
      </div>

      <FormField label="Name" :error="!name ? 'Name required' : ''">
        <FormInput v-model="name" placeholder="e.g. Production Broker" :invalid="!name" />
      </FormField>

      <FormField label="Server">
        <div class="flex items-stretch gap-1">
          <div class="flex-1 min-w-0">
            <FormInput v-model="host" placeholder="mqtt.example.com" mono />
          </div>
          <div class="w-24 shrink-0">
            <NumberInput v-model="port" :min="1" :max="65535" />
          </div>
        </div>
      </FormField>

      <FormField label="Client ID">
        <FormInput v-model="clientId" placeholder="auto-generated if empty" mono />
      </FormField>

      <FormField label="Username">
        <FormInput v-model="username" placeholder="optional" mono />
      </FormField>

      <FormField label="Password">
        <FormInput v-model="password" type="password" placeholder="optional" />
      </FormField>

      <FormField label="Keep-Alive">
        <NumberInput v-model="keepalive" :min="0" unit="sec" />
      </FormField>

      <FormCheckbox v-model="cleanSession" label="Clean Session" />
      <FormCheckbox v-model="useTLS" label="Use TLS" />
    </div>

    <!-- Sticky action footer -->
    <div class="shrink-0 flex items-center gap-2 px-4 py-2.5 border-t border-terminal-border bg-terminal-surface">
      <button
        class="px-3 py-1 text-[10px] uppercase tracking-wider font-bold
               bg-accent/10 text-accent border border-accent/30
               hover:bg-accent/20 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
        :disabled="!name"
        @click="save"
      >
        {{ isEditing ? 'Update' : 'Create' }}
      </button>
      <button
        class="px-3 py-1 text-[10px] uppercase tracking-wider
               text-terminal-text-dim border border-terminal-border
               hover:text-terminal-text hover:border-terminal-text-dim transition-colors"
        @click="cancel"
      >
        Cancel
      </button>
    </div>
  </div>
</template>
