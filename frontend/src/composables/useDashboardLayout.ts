// Layout-View read model + write helpers.
//
// The dashboard layout data is already in the flow store:
//   - flowStore.configs   → ui-base / ui-page / ui-group
//   - flowStore.flows[].nodes → ui-button / ui-text / ui-led / ui-gauge / …
//
// This composable derives a structured `LayoutTree` from that data
// (with legacy `order` → `x/y` migration), and exposes mutations
// that write back through the existing flow-store API. Mutations
// run push-down collision resolution so an explicit drop into an
// occupied slot shoves the overlapping widget downward instead of
// stacking visually.

import { computed } from 'vue'
import { useFlowStore } from '@/stores/flowStore'
import type { ConfigNode, Node as LoopzeNode } from '@/types/flow'
import { getCategory } from '@/nodes'
import {
  effectiveHeight,
  effectiveWidth,
  effectiveX,
  effectiveY,
  hasExplicitY,
  migratePositions,
  resolveCollisions,
  type PositionedItem,
} from '@/nodes/dashboard/sizing'

export interface LayoutWidget {
  flowId: string
  node: LoopzeNode
  /** Resolved x (post-migration). */
  x: number
  /** Resolved y (post-migration). */
  y: number
  width: number
  height: number
}

export interface LayoutGroup {
  group: ConfigNode
  x: number
  y: number
  width: number
  height: number
  widgets: LayoutWidget[]
}

export interface LayoutPage {
  page: ConfigNode
  groups: LayoutGroup[]
}

export interface LayoutTree {
  base: ConfigNode | null
  pages: LayoutPage[]
  /** Widgets that reference a missing group — surfaced separately so
   *  the view can warn instead of silently dropping them. */
  orphans: LayoutWidget[]
}

const WIDGET_TYPES = new Set(['ui-button', 'ui-text', 'ui-led', 'ui-gauge'])

function isWidgetType(t: string): boolean {
  if (WIDGET_TYPES.has(t)) return true
  const cat = getCategory(t)
  return cat === 'dashboard-input' || cat === 'dashboard-display'
}

function intProp(cfg: Record<string, unknown> | undefined, key: string, fallback: number): number {
  const v = cfg?.[key]
  if (typeof v === 'number') return v
  return fallback
}

function stringProp(cfg: Record<string, unknown> | undefined, key: string): string {
  const v = cfg?.[key]
  return typeof v === 'string' ? v : ''
}

export function useDashboardLayout() {
  const flowStore = useFlowStore()

  const tree = computed<LayoutTree>(() => {
    let base: ConfigNode | null = null
    const pages: ConfigNode[] = []
    const groups: ConfigNode[] = []
    for (const cfg of flowStore.configs) {
      switch (cfg.type) {
        case 'ui-base':
          if (!base) base = cfg
          break
        case 'ui-page': pages.push(cfg); break
        case 'ui-group': groups.push(cfg); break
      }
    }

    // Collect raw widgets per group; orphans go to a side bucket.
    const rawByGroup = new Map<string, LayoutWidget[]>()
    const orphans: LayoutWidget[] = []
    const validGroupIds = new Set(groups.map((g) => g.id))

    for (const flow of flowStore.flows) {
      if (flow.disabled) continue
      for (const n of flow.nodes) {
        if (!isWidgetType(n.type)) continue
        const groupRef = stringProp(n.config, 'group')
        const w: LayoutWidget = {
          flowId: flow.id,
          node: n,
          x: effectiveX(n.config),
          y: effectiveY(n.config),
          width: effectiveWidth(n.type, n.config),
          height: effectiveHeight(n.type, n.config),
        }
        if (!groupRef || !validGroupIds.has(groupRef)) {
          orphans.push(w)
          continue
        }
        const list = rawByGroup.get(groupRef) ?? []
        list.push(w)
        rawByGroup.set(groupRef, list)
      }
    }

    // Migrate per-group: widgets without explicit y get stacked.
    const migratedByGroup = new Map<string, LayoutWidget[]>()
    for (const [gid, list] of rawByGroup) {
      const positioned: PositionedItem[] = list.map((w) => ({
        id: w.node.id,
        hasY: hasExplicitY(w.node.config),
        x: w.x,
        y: w.y,
        width: w.width,
        height: w.height,
        order: intProp(w.node.config, 'order', 0),
      }))
      const migrated = migratePositions(positioned)
      const byId = new Map(migrated.map((m) => [m.id, m]))
      migratedByGroup.set(
        gid,
        list.map((w) => ({ ...w, y: byId.get(w.node.id)!.y })),
      )
    }

    // Migrate groups within each page (no explicit-y reading yet —
    // group config doesn't expose y in the editor, so all groups
    // stack vertically by order).
    const groupsByPage = new Map<string, ConfigNode[]>()
    const validPageIds = new Set(pages.map((p) => p.id))
    for (const g of groups) {
      const pageRef = stringProp(g.config, 'page')
      if (!pageRef || !validPageIds.has(pageRef)) continue
      const list = groupsByPage.get(pageRef) ?? []
      list.push(g)
      groupsByPage.set(pageRef, list)
    }

    const orderedPages = [...pages].sort((a, b) => {
      const oa = intProp(a.config, 'order', 0)
      const ob = intProp(b.config, 'order', 0)
      if (oa !== ob) return oa - ob
      return (a.name || '').localeCompare(b.name || '')
    })

    const pagesOut: LayoutPage[] = orderedPages.map((page) => {
      const pageGroups = groupsByPage.get(page.id) ?? []
      const groupPositions: PositionedItem[] = pageGroups.map((g) => ({
        id: g.id,
        hasY: hasExplicitY(g.config),
        x: effectiveX(g.config),
        y: effectiveY(g.config),
        width: Math.max(1, Math.min(12, intProp(g.config, 'width', 12))),
        height: Math.max(1, Math.min(100, intProp(g.config, 'height', 6))),
        order: intProp(g.config, 'order', 0),
      }))
      const migratedGroups = migratePositions(groupPositions)
      const byId = new Map(migratedGroups.map((m) => [m.id, m]))
      const groupsOut: LayoutGroup[] = pageGroups.map((g) => {
        const m = byId.get(g.id)!
        return {
          group: g,
          x: m.x,
          y: m.y,
          width: m.width,
          height: m.height,
          widgets: migratedByGroup.get(g.id) ?? [],
        }
      })
      // Render order by migrated y then x (stable)
      groupsOut.sort((a, b) => (a.y - b.y) || (a.x - b.x))
      return { page, groups: groupsOut }
    })

    return { base, pages: pagesOut, orphans }
  })

  // ─── Mutations ────────────────────────────────────────────────────────────

  /** Move a widget to a new position (and possibly group). Push-down
   *  resolves collisions; the moved widget is the anchor. */
  function moveWidget(
    nodeId: string,
    targetGroupId: string,
    targetX: number,
    targetY: number,
  ): void {
    const widget = findWidget(tree.value, nodeId)
    if (!widget) return

    const targetGroup = tree.value.pages
      .flatMap((p) => p.groups)
      .find((g) => g.group.id === targetGroupId)
    if (!targetGroup) return

    // Build the post-move set of widgets in the target group.
    const w = Math.max(1, Math.min(12, widget.width))
    const x = Math.max(0, Math.min(12 - w, targetX))
    const y = Math.max(0, targetY)

    const sourceGroupId = stringProp(widget.node.config, 'group')
    const isMovingBetweenGroups = sourceGroupId !== targetGroupId

    const targetWidgets = targetGroup.widgets.filter((w) => w.node.id !== nodeId)
    const positioned: PositionedItem[] = [
      ...targetWidgets.map((tw) => ({
        id: tw.node.id,
        hasY: true,
        x: tw.x,
        y: tw.y,
        width: tw.width,
        height: tw.height,
        order: intProp(tw.node.config, 'order', 0),
      })),
      {
        id: nodeId,
        hasY: true,
        x,
        y,
        width: widget.width,
        height: widget.height,
        order: intProp(widget.node.config, 'order', 0),
      },
    ]

    const resolved = resolveCollisions(positioned, nodeId)

    // Persist each widget whose (x, y) changed.
    for (const r of resolved) {
      const patch: Record<string, unknown> = { x: r.x, y: r.y }
      if (r.id === nodeId && isMovingBetweenGroups) {
        patch.group = targetGroupId
      }
      flowStore.updateNodeDataAcrossFlows(r.id, patch)
    }

    // If we left a group, the source-group siblings keep their
    // positions — push-down only runs in the destination. The gap
    // left behind is fine for v1; the user can drag to close it.
  }

  /** Resize a widget. Push-down resolves any new overlap. */
  function resizeWidget(nodeId: string, width: number, height: number): void {
    const widget = findWidget(tree.value, nodeId)
    if (!widget) return
    const groupId = stringProp(widget.node.config, 'group')
    if (!groupId) return

    const group = tree.value.pages
      .flatMap((p) => p.groups)
      .find((g) => g.group.id === groupId)
    if (!group) return

    const w = Math.max(1, Math.min(12, Math.round(width)))
    const h = Math.max(1, Math.min(12, Math.round(height)))
    // Clamp x so the resized widget stays within the 12-col grid.
    const x = Math.max(0, Math.min(12 - w, widget.x))

    const others = group.widgets.filter((gw) => gw.node.id !== nodeId)
    const positioned: PositionedItem[] = [
      ...others.map((gw) => ({
        id: gw.node.id,
        hasY: true,
        x: gw.x,
        y: gw.y,
        width: gw.width,
        height: gw.height,
        order: intProp(gw.node.config, 'order', 0),
      })),
      {
        id: nodeId,
        hasY: true,
        x,
        y: widget.y,
        width: w,
        height: h,
        order: intProp(widget.node.config, 'order', 0),
      },
    ]

    const resolved = resolveCollisions(positioned, nodeId)

    for (const r of resolved) {
      if (r.id === nodeId) {
        flowStore.updateNodeDataAcrossFlows(nodeId, {
          x: r.x, y: r.y, width: r.width, height: r.height,
        })
      } else {
        flowStore.updateNodeDataAcrossFlows(r.id, { x: r.x, y: r.y })
      }
    }
  }

  /** Move a group to a new (x, y) within its page. Push-down on the
   *  page level — sibling groups get shoved down if they overlap. */
  function moveGroup(groupId: string, x: number, y: number): void {
    const page = tree.value.pages.find((p) =>
      p.groups.some((g) => g.group.id === groupId),
    )
    if (!page) return
    const g = page.groups.find((g) => g.group.id === groupId)
    if (!g) return

    const targetW = g.width
    const clampedX = Math.max(0, Math.min(12 - targetW, x))
    const clampedY = Math.max(0, y)

    const others = page.groups.filter((sib) => sib.group.id !== groupId)
    const positioned: PositionedItem[] = [
      ...others.map((sib) => ({
        id: sib.group.id,
        hasY: true,
        x: sib.x,
        y: sib.y,
        width: sib.width,
        height: sib.height,
        order: intProp(sib.group.config, 'order', 0),
      })),
      {
        id: groupId,
        hasY: true,
        x: clampedX,
        y: clampedY,
        width: g.width,
        height: g.height,
        order: intProp(g.group.config, 'order', 0),
      },
    ]
    const resolved = resolveCollisions(positioned, groupId)
    for (const r of resolved) {
      const cfg = flowStore.configs.find((c) => c.id === r.id)
      if (!cfg) continue
      flowStore.updateConfig(r.id, {
        config: { ...(cfg.config ?? {}), x: r.x, y: r.y },
      })
    }
  }

  /** Resize a group. Same push-down rules as moveGroup. */
  function resizeGroup(groupId: string, width: number, height: number): void {
    const page = tree.value.pages.find((p) =>
      p.groups.some((g) => g.group.id === groupId),
    )
    if (!page) return
    const g = page.groups.find((g) => g.group.id === groupId)
    if (!g) return

    const w = Math.max(1, Math.min(12, Math.round(width)))
    const h = Math.max(1, Math.min(100, Math.round(height)))
    const clampedX = Math.max(0, Math.min(12 - w, g.x))

    const others = page.groups.filter((sib) => sib.group.id !== groupId)
    const positioned: PositionedItem[] = [
      ...others.map((sib) => ({
        id: sib.group.id,
        hasY: true,
        x: sib.x,
        y: sib.y,
        width: sib.width,
        height: sib.height,
        order: intProp(sib.group.config, 'order', 0),
      })),
      {
        id: groupId,
        hasY: true,
        x: clampedX,
        y: g.y,
        width: w,
        height: h,
        order: intProp(g.group.config, 'order', 0),
      },
    ]
    const resolved = resolveCollisions(positioned, groupId)
    for (const r of resolved) {
      const cfg = flowStore.configs.find((c) => c.id === r.id)
      if (!cfg) continue
      const patch: Record<string, unknown> = { x: r.x, y: r.y }
      if (r.id === groupId) {
        patch.width = r.width
        patch.height = r.height
      }
      flowStore.updateConfig(r.id, { config: { ...(cfg.config ?? {}), ...patch } })
    }
  }

  return { tree, moveWidget, resizeWidget, moveGroup, resizeGroup }
}

function findWidget(tree: LayoutTree, nodeId: string): LayoutWidget | null {
  for (const page of tree.pages) {
    for (const group of page.groups) {
      const hit = group.widgets.find((w) => w.node.id === nodeId)
      if (hit) return hit
    }
  }
  for (const w of tree.orphans) {
    if (w.node.id === nodeId) return w
  }
  return null
}
