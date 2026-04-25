import { computed, type WritableComputedRef } from 'vue'
import { useFlowStore } from '@/stores/flowStore'

/**
 * Two-way bound config property for the currently selected node.
 *
 * Reads from `selectedNode.data.config[key]`, falling back to `defaultValue`.
 * Writes back via `flowStore.updateNodeData`, preserving the rest of the config.
 *
 * Usage in a config panel:
 *   const once = useNodeProperty<boolean>('once', false)
 *   const interval = useNodeProperty<number>('interval', 0)
 */
export function useNodeProperty<T>(
  key: string,
  defaultValue: T,
): WritableComputedRef<T> {
  const flowStore = useFlowStore()

  return computed<T>({
    get: () => {
      const node = flowStore.selectedNode
      const cfg = (node?.data?.config ?? {}) as Record<string, unknown>
      const v = cfg[key]
      return (v === undefined ? defaultValue : v) as T
    },
    set: (value: T) => {
      const node = flowStore.selectedNode
      if (!node) return
      const cfg = (node.data?.config ?? {}) as Record<string, unknown>
      flowStore.updateNodeData(node.id, {
        config: { ...cfg, [key]: value },
      })
    },
  })
}
