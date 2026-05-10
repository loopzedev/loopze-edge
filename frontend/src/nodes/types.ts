import type { Component } from 'vue'

/**
 * NodeGroupManifest is the contract every node group exports from its
 * frontend folder (`frontend/src/nodes/<group>/index.ts`).
 *
 * The aggregator at `frontend/src/nodes/index.ts` walks every group's
 * manifest and builds the global registries (flow-node editors, config-node
 * editors, palette categories) — so adding a new group needs only the new
 * folder plus a single import line in the aggregator.
 *
 * The `name` field MUST match the corresponding backend group name passed
 * to `nodes.RegisterGroup` in `internal/nodes/<group>/init.go`. This is the
 * single string that ties the two trees together.
 */
export interface NodeGroupManifest {
  /** Group identifier — must match the backend group name. */
  name: string

  /** Default palette category applied to every node type in this group. */
  category?: string

  /**
   * Per-node-type category override. Used by groups that span several
   * sub-protocols (e.g. "network" covers HTTP, TCP and UDP, each with its
   * own palette colour). Takes precedence over `category` when both are set.
   */
  categories?: Record<string, string>

  /** Lazy editor loaders for flow node types (rendered in the property panel). */
  flowEditors: Record<string, () => Promise<Component>>

  /** Lazy editor loaders for config node types (e.g. mqtt-broker, s7-plc). */
  configEditors?: Record<string, () => Promise<Component>>

  /**
   * Flow node types whose editor needs the full panel height (code editors,
   * canvas-like layouts). The PropertyPanel skips its collapsible wrapper
   * for these so the editor can use the available vertical space.
   */
  fullHeightEditors?: string[]
}
