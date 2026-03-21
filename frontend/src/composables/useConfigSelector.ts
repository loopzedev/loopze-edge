import { computed } from 'vue'
import { useFlowStore } from '@/stores/flowStore'
import { useUiStore } from '@/stores/uiStore'

/**
 * Reusable composable for the "config dropdown + plus button" pattern.
 * Every connector node type (MQTT, HTTP, Modbus, ...) uses this to let
 * the user select or create a config node instance.
 *
 * Usage:
 *   const { options, openNewConfig, openEditConfig } = useConfigSelector('mqtt-broker')
 */
export function useConfigSelector(configType: string) {
  const flowStore = useFlowStore()
  const uiStore = useUiStore()

  const availableConfigs = computed(() =>
    flowStore.getConfigsByType(configType),
  )

  const options = computed(() =>
    availableConfigs.value.map((c) => ({
      value: c.id,
      label: c.name || c.id.slice(0, 12),
    })),
  )

  function openNewConfig() {
    uiStore.openConfigEditor(configType)
  }

  function openEditConfig(configId: string) {
    uiStore.openConfigEditor(configType, configId)
  }

  return { availableConfigs, options, openNewConfig, openEditConfig }
}
