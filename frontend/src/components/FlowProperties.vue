<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useFlowStore } from '@/stores/flowStore'
import { useUiStore } from '@/stores/uiStore'
import { useAuthStore } from '@/stores/authStore'
import SectionHeader from '@/components/ui/SectionHeader.vue'
import FormLabel from '@/components/ui/FormLabel.vue'
import FormInput from '@/components/ui/FormInput.vue'
import AppSwitch from '@/components/ui/AppSwitch.vue'

const flowStore = useFlowStore()
const ui = useUiStore()
const auth = useAuthStore()

const ctx = computed(() => ui.propertiesContext)
const isCreate = computed(() => ctx.value?.type === 'flow-create')
const isEdit = computed(() => ctx.value?.type === 'flow-edit')

// ── Create mode ──────────────────────────────────────────────────
const newFlowName = ref('')

function handleCreate() {
  const name = newFlowName.value.trim()
  if (!name) return
  flowStore.addFlow(name)
  newFlowName.value = ''
  ui.clearFlowProperties()
}

function handleCancel() {
  newFlowName.value = ''
  ui.clearFlowProperties()
}

// ── Edit mode ────────────────────────────────────────────────────
const editFlow = computed(() => {
  if (ctx.value?.type !== 'flow-edit') return null
  const c = ctx.value
  return flowStore.flows.find((f) => f.id === c.flowId) ?? null
})

const editName = ref('')
const confirmingDelete = ref(false)

watch(editFlow, (flow) => {
  if (flow) {
    editName.value = flow.label
    confirmingDelete.value = false
  }
}, { immediate: true })

function handleNameChange(value: string) {
  editName.value = value
  if (editFlow.value && value.trim()) {
    flowStore.updateFlowLabel(editFlow.value.id, value.trim())
  }
}

function handleToggleDisabled() {
  if (editFlow.value) {
    flowStore.toggleFlowDisabled(editFlow.value.id)
  }
}

function handleDeleteClick() {
  confirmingDelete.value = true
}

function handleDeleteConfirm() {
  if (editFlow.value) {
    flowStore.removeFlow(editFlow.value.id)
    confirmingDelete.value = false
    ui.clearFlowProperties()
  }
}

function handleDeleteCancel() {
  confirmingDelete.value = false
}

const canDelete = computed(() => flowStore.flows.length > 1)
</script>

<template>
  <!-- Create Flow -->
  <div v-if="isCreate" class="flex flex-col flex-1 min-h-0">
    <div class="px-3 py-3 border-b border-terminal-border">
      <div class="flex items-center gap-2 mb-2">
        <span class="w-3 h-3 bg-accent flex-shrink-0"></span>
        <span class="text-accent text-sm font-bold uppercase tracking-wider">New Flow</span>
      </div>
    </div>

    <div class="px-3 py-3">
      <SectionHeader title="Settings">
        <div class="flex flex-col gap-3">
          <div class="flex flex-col gap-1">
            <FormLabel>Name</FormLabel>
            <FormInput
              v-model="newFlowName"
              placeholder="Flow name"
              maxlength="50"
              @keyup.enter="handleCreate"
            />
          </div>
          <div class="flex gap-2">
            <button
              class="terminal-btn-primary text-xs uppercase tracking-wider flex-1"
              :disabled="!newFlowName.trim()"
              :class="{ 'opacity-50 cursor-not-allowed': !newFlowName.trim() }"
              @click="handleCreate"
            >
              Create
            </button>
            <button
              class="terminal-btn text-xs uppercase tracking-wider flex-1"
              @click="handleCancel"
            >
              Cancel
            </button>
          </div>
        </div>
      </SectionHeader>
    </div>
  </div>

  <!-- Edit Flow -->
  <div v-else-if="isEdit && editFlow" class="flex flex-col flex-1 min-h-0">
    <div class="px-3 py-3 border-b border-terminal-border">
      <div class="flex items-center gap-2 mb-2">
        <span class="w-3 h-3 bg-accent flex-shrink-0"></span>
        <span class="text-accent text-sm font-bold uppercase tracking-wider truncate">
          {{ editFlow.label }}
        </span>
      </div>
      <div class="flex items-center gap-2 text-[10px] text-terminal-text-dim">
        <span class="terminal-badge">flow</span>
        <span>{{ editFlow.id.slice(0, 12) }}&hellip;</span>
      </div>
    </div>

    <div class="px-3 py-3 border-b border-terminal-border">
      <SectionHeader title="Settings">
        <div class="flex flex-col gap-3">
          <div class="flex flex-col gap-1">
            <FormLabel>Name</FormLabel>
            <FormInput
              :model-value="editName"
              placeholder="Flow name"
              maxlength="50"
              @update:model-value="handleNameChange"
            />
          </div>
          <div class="flex flex-col gap-1">
            <FormLabel>Status</FormLabel>
            <AppSwitch
              :model-value="!editFlow.disabled"
              label="Enabled"
              @update:model-value="handleToggleDisabled"
            />
          </div>
        </div>
      </SectionHeader>
    </div>

    <div v-if="auth.can('deploy')" class="px-3 py-3">
      <div v-if="!confirmingDelete">
        <button
          class="text-xs text-red-400 hover:text-red-300 transition-colors duration-100 disabled:opacity-30 disabled:cursor-not-allowed"
          :disabled="!canDelete"
          :title="canDelete ? 'Delete flow' : 'The last flow cannot be deleted'"
          @click="handleDeleteClick"
        >
          Delete Flow
        </button>
      </div>
      <div v-else class="flex flex-col gap-2">
        <p class="text-[10px] text-terminal-text-dim">
          Really delete flow &laquo;{{ editFlow.label }}&raquo;?
        </p>
        <div class="flex gap-2">
          <button
            class="terminal-btn text-xs uppercase tracking-wider flex-1 !border-red-500 !text-red-400 hover:!bg-red-500/10"
            @click="handleDeleteConfirm"
          >
            Delete
          </button>
          <button
            class="terminal-btn text-xs uppercase tracking-wider flex-1"
            @click="handleDeleteCancel"
          >
            Cancel
          </button>
        </div>
      </div>
    </div>
  </div>
</template>
