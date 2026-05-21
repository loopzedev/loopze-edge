<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import FormField from '@/components/ui/FormField.vue'
import FormInput from '@/components/ui/FormInput.vue'
import FormSelect from '@/components/ui/FormSelect.vue'
import FormCheckbox from '@/components/ui/FormCheckbox.vue'
import NumberInput from '@/components/ui/NumberInput.vue'
import { useFlowStore } from '@/stores/flowStore'
import { useUiStore } from '@/stores/uiStore'
import { useConfigSelector } from '@/composables/useConfigSelector'

const props = defineProps<{ configId?: string }>()
const flowStore = useFlowStore()
const ui = useUiStore()

const { options: pageSelectOptions, openNewConfig: openNewPage, openEditConfig: openEditPage } =
  useConfigSelector('ui-page')

const name = ref('Group 1')
const page = ref('')
const x = ref(0)
const y = ref(0)
const width = ref(12)
const height = ref(6)
const collapsible = ref(false)
const order = ref(0)

const isEditing = ref(false)

const pageOptions = computed(() => [
  { value: '', label: '— select page —' },
  ...pageSelectOptions.value,
])

onMounted(() => {
  if (!props.configId) return
  const existing = flowStore.configs.find((c) => c.id === props.configId)
  if (!existing) return
  isEditing.value = true
  name.value = existing.name || 'Group 1'
  const cfg = (existing.config ?? {}) as Record<string, unknown>
  if (typeof cfg.page === 'string') page.value = cfg.page
  if (typeof cfg.x === 'number') x.value = cfg.x
  if (typeof cfg.y === 'number') y.value = cfg.y
  if (typeof cfg.width === 'number') width.value = cfg.width
  if (typeof cfg.height === 'number') height.value = cfg.height
  if (typeof cfg.collapsible === 'boolean') collapsible.value = cfg.collapsible
  if (typeof cfg.order === 'number') order.value = cfg.order
})

function save() {
  const config = {
    name: name.value,
    page: page.value,
    x: x.value,
    y: y.value,
    width: width.value,
    height: height.value,
    collapsible: collapsible.value,
    order: order.value,
  }
  if (isEditing.value && props.configId) {
    flowStore.updateConfig(props.configId, { name: name.value, config })
  } else {
    flowStore.addConfig({
      id: crypto.randomUUID(),
      type: 'ui-group',
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
        {{ isEditing ? 'Edit' : 'New' }} Dashboard Group
      </span>

      <FormField label="Name" :error="!name ? 'Name required' : ''">
        <FormInput v-model="name" placeholder="e.g. Controls" :invalid="!name" />
      </FormField>

      <FormField label="Page" :error="!page ? 'Page required' : ''">
        <template #action>
          <button
            v-if="page"
            type="button"
            class="text-[9px] uppercase tracking-wider text-terminal-text-dim hover:text-accent transition-colors"
            @click="openEditPage(page)"
          >Edit</button>
          <button
            type="button"
            class="text-[9px] uppercase tracking-wider text-terminal-text-dim hover:text-accent transition-colors"
            @click="openNewPage()"
          >+ New</button>
        </template>
        <FormSelect v-model="page" :options="pageOptions" />
      </FormField>

      <FormField label="Position (col × row)">
        <div class="flex items-stretch gap-1">
          <div class="flex-1 min-w-0">
            <NumberInput v-model="x" :min="0" :max="11" unit="X" />
          </div>
          <div class="flex-1 min-w-0">
            <NumberInput v-model="y" :min="0" :max="100" unit="Y" />
          </div>
        </div>
      </FormField>

      <FormField label="Size (W × H grid units)">
        <div class="flex items-stretch gap-1">
          <div class="flex-1 min-w-0">
            <NumberInput v-model="width" :min="1" :max="12" unit="W" />
          </div>
          <div class="flex-1 min-w-0">
            <NumberInput v-model="height" :min="1" :max="100" unit="H" />
          </div>
        </div>
      </FormField>

      <FormCheckbox v-model="collapsible" label="Collapsible" />
    </div>
    <div class="border-t border-terminal-border px-4 py-2 flex gap-2 justify-end">
      <button class="px-3 py-1 text-xs" @click="cancel">Cancel</button>
      <button
        class="px-3 py-1 text-xs bg-accent text-terminal-bg font-semibold rounded"
        :disabled="!name || !page"
        @click="save"
      >
        {{ isEditing ? 'Save' : 'Create' }}
      </button>
    </div>
  </div>
</template>
