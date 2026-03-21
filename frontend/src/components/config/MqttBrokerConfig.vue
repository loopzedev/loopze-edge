<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useFlowStore } from '@/stores/flowStore'
import { useUiStore } from '@/stores/uiStore'
import FormLabel from '@/components/ui/FormLabel.vue'
import FormInput from '@/components/ui/FormInput.vue'
import FormCheckbox from '@/components/ui/FormCheckbox.vue'

const props = defineProps<{
  configId?: string
}>()

const flowStore = useFlowStore()
const ui = useUiStore()

// Form state
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
    // Load existing config
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
    flowStore.updateConfig(props.configId, {
      name: name.value,
      config,
    })
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
  <div class="flex flex-col gap-3 p-3">
    <div class="flex items-center gap-2 mb-1">
      <span class="text-accent text-sm font-bold uppercase tracking-wider">
        {{ isEditing ? 'Edit' : 'New' }} MQTT Broker
      </span>
    </div>

    <!-- Name -->
    <div class="flex flex-col gap-1">
      <FormLabel>Name</FormLabel>
      <FormInput v-model="name" placeholder="e.g. Production Broker" />
    </div>

    <!-- Host + Port -->
    <div class="flex flex-col gap-1">
      <FormLabel>Server</FormLabel>
      <div class="flex items-center gap-1">
        <div class="flex-1 min-w-0">
          <FormInput v-model="host" placeholder="mqtt.example.com" />
        </div>
        <div class="w-16 shrink-0">
          <FormInput
            :model-value="String(port)"
            type="number"
            placeholder="1883"
            @update:model-value="port = Number($event)"
          />
        </div>
      </div>
    </div>

    <!-- Client ID -->
    <div class="flex flex-col gap-1">
      <FormLabel>Client ID</FormLabel>
      <FormInput v-model="clientId" placeholder="auto-generated if empty" />
    </div>

    <!-- Credentials -->
    <div class="flex flex-col gap-1">
      <FormLabel>Username</FormLabel>
      <FormInput v-model="username" placeholder="optional" />
    </div>
    <div class="flex flex-col gap-1">
      <FormLabel>Password</FormLabel>
      <FormInput v-model="password" type="password" placeholder="optional" />
    </div>

    <!-- Keep-Alive -->
    <div class="flex flex-col gap-1">
      <FormLabel>Keep-Alive (seconds)</FormLabel>
      <FormInput
        :model-value="String(keepalive)"
        type="number"
        @update:model-value="keepalive = Number($event)"
      />
    </div>

    <!-- Toggles -->
    <FormCheckbox v-model="cleanSession" label="Clean Session" />
    <FormCheckbox v-model="useTLS" label="Use TLS" />

    <!-- Actions -->
    <div class="flex items-center gap-2 mt-2">
      <button
        class="px-3 py-1 text-[10px] uppercase tracking-wider font-bold
               bg-accent/10 text-accent border border-accent/30
               hover:bg-accent/20 transition-colors"
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
