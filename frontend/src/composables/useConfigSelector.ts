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
      label: c.name || configFallbackLabel(c.config) || c.id.slice(0, 12),
    })),
  )

  /** Build a fallback label from host:port if no name is set. */
  function configFallbackLabel(config: Record<string, unknown>): string {
    const host = config.host as string
    if (!host) return ''
    const port = config.port as number
    return port ? `${host}:${port}` : host
  }

  function openNewConfig() {
    uiStore.openConfigEditor(configType)
  }

  function openEditConfig(configId: string) {
    uiStore.openConfigEditor(configType, configId)
  }

  return { availableConfigs, options, openNewConfig, openEditConfig }
}
