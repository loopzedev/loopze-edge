<script setup lang="ts">
import { onMounted, ref } from 'vue'
import FormField from '@/components/ui/FormField.vue'
import FormInput from '@/components/ui/FormInput.vue'
import FormSelect from '@/components/ui/FormSelect.vue'
import FormCheckbox from '@/components/ui/FormCheckbox.vue'
import { useFlowStore } from '@/stores/flowStore'
import { useUiStore } from '@/stores/uiStore'

const props = defineProps<{ configId?: string }>()
const flowStore = useFlowStore()
const ui = useUiStore()

const name = ref('LOOPZE Dashboard')
const theme = ref('dark')
const accentColor = ref('#58a6ff')
const auth = ref('session')
const showNav = ref(true)
const navStyle = ref('tabs')
const density = ref('default')

const isEditing = ref(false)

const themes = [
  { value: 'dark',  label: 'Dark' },
  { value: 'light', label: 'Light' },
]
const auths = [
  { value: 'session', label: 'Session (logged-in users only)' },
  { value: 'none',    label: 'None (open kiosk — no auth)' },
]
const densities = [
  { value: 'compact',     label: 'Compact' },
  { value: 'default',     label: 'Default' },
  { value: 'comfortable', label: 'Comfortable' },
]
const navStyles = [
  { value: 'tabs',    label: 'Tabs (top bar)' },
  { value: 'sidebar', label: 'Sidebar (collapsible left rail)' },
]

onMounted(() => {
  if (!props.configId) return
  const existing = flowStore.configs.find((c) => c.id === props.configId)
  if (!existing) return
  isEditing.value = true
  name.value = existing.name || 'LOOPZE Dashboard'
  const cfg = (existing.config ?? {}) as Record<string, unknown>
  if (typeof cfg.theme === 'string') theme.value = cfg.theme
  if (typeof cfg.accentColor === 'string') accentColor.value = cfg.accentColor
  if (typeof cfg.auth === 'string') auth.value = cfg.auth
  if (typeof cfg.showNav === 'boolean') showNav.value = cfg.showNav
  if (typeof cfg.navStyle === 'string') navStyle.value = cfg.navStyle
  if (typeof cfg.density === 'string') density.value = cfg.density
})

function save() {
  const config = {
    name: name.value,
    path: '/dashboard',
    theme: theme.value,
    accentColor: accentColor.value,
    auth: auth.value,
    showNav: showNav.value,
    navStyle: navStyle.value,
    density: density.value,
  }
  if (isEditing.value && props.configId) {
    flowStore.updateConfig(props.configId, { name: name.value, config })
  } else {
    flowStore.addConfig({
      id: crypto.randomUUID(),
      type: 'ui-base',
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
        {{ isEditing ? 'Edit' : 'New' }} Dashboard
      </span>

      <FormField label="Name" :error="!name ? 'Name required' : ''">
        <FormInput v-model="name" placeholder="e.g. Plant Overview" :invalid="!name" />
      </FormField>

      <FormField label="Theme">
        <FormSelect v-model="theme" :options="themes" />
      </FormField>

      <FormField label="Accent color">
        <FormInput v-model="accentColor" mono placeholder="#58a6ff" />
      </FormField>

      <FormField label="Auth">
        <FormSelect v-model="auth" :options="auths" />
        <div v-if="auth === 'none'" class="text-[10px] text-status-warn leading-tight">
          ⚠ Dashboard is reachable without login. Use only for kiosks on a
          trusted network.
        </div>
      </FormField>

      <FormCheckbox
        v-model="showNav"
        label="Show page navigation (hide when only one page is needed)"
      />

      <FormField v-if="showNav" label="Navigation style">
        <FormSelect v-model="navStyle" :options="navStyles" />
      </FormField>

      <FormField label="Density">
        <FormSelect v-model="density" :options="densities" />
      </FormField>

      <div class="text-[10px] text-terminal-text-dim leading-tight">
        Phase 1: mount path is fixed at <code>/dashboard</code>. Custom paths
        come in a later release.
      </div>
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
