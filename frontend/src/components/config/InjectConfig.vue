<script setup lang="ts">
import FormCheckbox from '@/components/ui/FormCheckbox.vue'
import ToggleGroup from '@/components/ui/ToggleGroup.vue'
import NumberInput from '@/components/ui/NumberInput.vue'
import FormField from '@/components/ui/FormField.vue'
import PropertyList from '@/components/ui/PropertyList.vue'
import PropertyListItem from '@/components/ui/PropertyListItem.vue'
import MsgFieldEditor, { type MsgField } from '@/components/config/MsgFieldEditor.vue'
import { useNodeProperty } from '@/composables/useNodeProperty'
import { INTERVAL_PRESETS } from '@/components/config/enums'
import { computed } from 'vue'

const once = useNodeProperty<boolean>('once', false)
const interval = useNodeProperty<number>('interval', 0)

const rawProps = useNodeProperty<MsgField[]>('props', [
  { p: 'payload', vt: 'date', v: 'rfc3339', vs: 'memory' },
  { p: 'topic', vt: 'str', v: '', vs: 'memory' },
])

// Normalise: ensure every entry has all four fields (older configs may miss vs).
const fields = computed<MsgField[]>({
  get: () => (rawProps.value ?? []).map((r) => ({
    p: r?.p ?? '',
    vt: r?.vt ?? 'str',
    v: r?.v ?? '',
    vs: r?.vs ?? 'memory',
  })),
  set: (val) => { rawProps.value = val },
})

function newField(): MsgField {
  return { p: '', vt: 'str', v: '', vs: 'memory' }
}

function updateField(idx: number, next: MsgField) {
  const updated = [...fields.value]
  updated[idx] = next
  fields.value = updated
}

// Inject has no input message → exclude "msg" as a value source.
const excludeTypes = ['msg']
</script>

<template>
  <div class="flex flex-col gap-3">
    <!-- Trigger: once -->
    <FormCheckbox v-model="once" label="Inject once at startup" />

    <!-- Trigger: repeat interval -->
    <FormField label="Repeat Interval">
      <ToggleGroup v-model="interval" :options="INTERVAL_PRESETS" />
      <NumberInput
        v-model="interval"
        :min="0"
        :step="100"
        unit="ms"
        placeholder="Custom interval"
      />
    </FormField>

    <!-- Message fields -->
    <FormField :label="`Message Fields · ${fields.length}`">
      <PropertyList
        v-model="fields"
        :new-item="newField"
        add-label="+ add field"
      >
        <template #default="{ item, index, isDragging, isDropTarget, remove, handleMousedown }">
          <PropertyListItem
            :is-dragging="isDragging"
            :is-drop-target="isDropTarget"
            :is-invalid="!item.p"
            @remove="remove"
            @handle-mousedown="handleMousedown"
          >
            <MsgFieldEditor
              :model-value="item"
              :exclude-types="excludeTypes"
              property-required
              @update:model-value="updateField(index, $event)"
            />
          </PropertyListItem>
        </template>
      </PropertyList>
    </FormField>
  </div>
</template>
