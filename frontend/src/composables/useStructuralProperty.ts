import { computed, nextTick, type WritableComputedRef } from 'vue'
import { useVueFlow } from '@vue-flow/core'
import { useFlowStore } from '@/stores/flowStore'

interface Options<T> {
  port: 'inputs' | 'outputs'
  derive?: (value: T) => number
  clamp?: { min: number; max: number }
  enabled?: () => boolean
}

/**
 * Two-way bound config property whose value also drives a structural
 * change on the node — a port count on `node.data.inputs` /
 * `node.data.outputs`. Writing the property updates the config, mirrors
 * the derived port count, and triggers `updateNodeInternals` so the
 * Vue Flow handles re-render and now-orphaned edges get pruned by
 * `flowStore.updateNodeData`.
 *
 * Two shapes:
 *   - numeric property: pass `clamp` and the value itself becomes the
 *     port count (e.g. function-node `outputs`).
 *   - derived: pass `derive` to map the property value to a port count
 *     (e.g. mqtt-in `mode='dynamic'` → 1 input port).
 *
 * The optional `enabled` gate skips the structural mirror — useful for
 * shared configs where the same property exists on multiple node types
 * but only some of them should change port counts.
 */
export function useStructuralProperty<T>(
  key: string,
  defaultValue: T,
  options: Options<T>,
): WritableComputedRef<T> {
  const flowStore = useFlowStore()
  const { updateNodeInternals } = useVueFlow('loopze-flow-editor')

  return computed<T>({
    get: () => {
      const node = flowStore.selectedNode
      const cfg = (node?.data?.config ?? {}) as Record<string, unknown>
      const v = cfg[key]
      return (v === undefined ? defaultValue : v) as T
    },
    set: (raw: T) => {
      const node = flowStore.selectedNode
      if (!node) return

      let value = raw
      if (options.clamp && typeof value === 'number') {
        const clamped = Math.max(options.clamp.min, Math.min(options.clamp.max, value))
        value = clamped as T
      }

      const cfg = (node.data?.config ?? {}) as Record<string, unknown>

      if (!options.enabled || options.enabled()) {
        const desired = options.derive ? options.derive(value) : Number(value)
        const current = (node.data?.[options.port] as number | undefined) ?? 0
        const update: Record<string, unknown> = {
          config: { ...cfg, [key]: value },
        }
        if (Number.isFinite(desired) && desired !== current) {
          update[options.port] = desired
        }
        flowStore.updateNodeData(node.id, update)
        if (Number.isFinite(desired) && desired !== current) {
          nextTick(() => updateNodeInternals([node.id]))
        }
      } else {
        flowStore.updateNodeData(node.id, {
          config: { ...cfg, [key]: value },
        })
      }
    },
  })
}
