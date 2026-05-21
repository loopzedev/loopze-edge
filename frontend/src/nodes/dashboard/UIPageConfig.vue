<script setup lang="ts">
import { onMounted, ref } from 'vue'
import FormField from '@/components/ui/FormField.vue'
import FormInput from '@/components/ui/FormInput.vue'
import FormSelect from '@/components/ui/FormSelect.vue'
import NumberInput from '@/components/ui/NumberInput.vue'
import { useFlowStore } from '@/stores/flowStore'
import { useUiStore } from '@/stores/uiStore'

const props = defineProps<{ configId?: string }>()
const flowStore = useFlowStore()
const ui = useUiStore()

const name = ref('Page 1')
const path = ref('')
const icon = ref('')
const layout = ref('grid')
const order = ref(0)

const isEditing = ref(false)

const layouts = [
  { value: 'grid', label: 'Grid (responsive 12-col)' },
  { value: 'flex', label: 'Flex (free)' },
  { value: 'tabs', label: 'Tabs' },
]

onMounted(() => {
  if (!props.configId) return
  const existing = flowStore.configs.find((c) => c.id === props.configId)
  if (!existing) return
  isEditing.value = true
  name.value = existing.name || 'Page 1'
  const cfg = (existing.config ?? {}) as Record<string, unknown>
  if (typeof cfg.path === 'string') path.value = cfg.path
  if (typeof cfg.icon === 'string') icon.value = cfg.icon
  if (typeof cfg.layout === 'string') layout.value = cfg.layout
  if (typeof cfg.order === 'number') order.value = cfg.order
})

function save() {
  const config = {
    name: name.value,
    path: path.value,
    icon: icon.value,
    layout: layout.value,
    order: order.value,
  }
  if (isEditing.value && props.configId) {
    flowStore.updateConfig(props.configId, { name: name.value, config })
  } else {
    flowStore.addConfig({
      id: crypto.randomUUID(),
      type: 'ui-page',
      name: name.value,
      config,
    })
  }
  ui.clearConfigEditor()
}

function cancel() {
  ui.clearConfigEditor()
}
</script>

<template>
  <div class="flex flex-col h-full">
    <div class="flex-1 overflow-y-auto px-4 py-3 flex flex-col gap-3">
      <span class="text-accent text-sm font-bold uppercase tracking-wider">
        {{ isEditing ? 'Edit' : 'New' }} Dashboard Page
      </span>

      <FormField label="Name" :error="!name ? 'Name required' : ''">
        <FormInput v-model="name" placeholder="e.g. Overview" :invalid="!name" />
      </FormField>

      <FormField label="URL path">
        <FormInput v-model="path" mono placeholder="derived from name if empty" />
      </FormField>

      <FormField label="Icon">
        <FormInput v-model="icon" mono placeholder="lucide name, optional" />
      </FormField>

      <FormField label="Layout">
        <FormSelect v-model="layout" :options="layouts" />
      </FormField>

      <FormField label="Sort order">
        <NumberInput v-model="order" :min="0" />
      </FormField>
    </div>
    <div class="border-t border-terminal-border px-4 py-2 flex gap-2 justify-end">
      <button class="px-3 py-1 text-xs" @click="cancel">Cancel</button>
      <button
        class="px-3 py-1 text-xs bg-accent text-terminal-bg font-semibold rounded"
        :disabled="!name"
        @click="save"
      >
        {{ isEditing ? 'Save' : 'Create' }}
      </button>
    </div>
  </div>
</template>
