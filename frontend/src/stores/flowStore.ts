import { defineStore } from "pinia";
import { computed, ref } from "vue";
import { useUiStore } from "./uiStore";
import type {
  Node as FlintNode,
  Flow,
  ConfigNode,
  DeployPayload,
  DeployResponse,
  DeployModeType,
} from "@/types/flow";

interface FlowNode {
  id: string;
  type: string;
  position: { x: number; y: number };
  data: Record<string, any>;
  [key: string]: any;
}

interface FlowEdge {
  id: string;
  source: string;
  target: string;
  sourceHandle?: string;
  targetHandle?: string;
  [key: string]: any;
}

export interface FlowStoreState {
  flows: Flow[];
  activeFlowId: string | null;
  nodes: FlowNode[];
  edges: FlowEdge[];
  selectedNodeId: string | null;
  dirty: boolean;
  revision: string | null;
  deploying: boolean;
}

export const useFlowStore = defineStore("flow", () => {
  // --------------- State ---------------

  const flows = ref<Flow[]>([]);
  const activeFlowId = ref<string | null>(null);
  const nodes = ref<FlowNode[]>([]);
  const edges = ref<FlowEdge[]>([]);
  const selectedNodeId = ref<string | null>(null);
  const selectedNodeIds = ref<string[]>([]);
  const clipboard = ref<{ nodes: FlowNode[]; edges: FlowEdge[] } | null>(null);
  const configs = ref<ConfigNode[]>([]);
  const dirty = ref(false);
  const dirtyNodeIds = ref(new Set<string>());
  const dirtyFlowIds = ref(new Set<string>());
  const revision = ref<string | null>(null);
  const deploying = ref(false);

  // Deploy mode: persisted in localStorage
  const savedMode = localStorage.getItem('flint-deploy-mode');
  const deployMode = ref<DeployModeType>(
    savedMode && ['nodes', 'flows', 'full', 'restart'].includes(savedMode)
      ? (savedMode as DeployModeType)
      : 'nodes',
  );

  // --------------- Getters ---------------

  const activeFlow = computed<Flow | undefined>(() =>
    flows.value.find((f) => f.id === activeFlowId.value),
  );

  const activeNodes = computed<FlowNode[]>(() => nodes.value);

  const activeEdges = computed<FlowEdge[]>(() => edges.value);

  const selectedNode = computed<FlowNode | undefined>(() =>
    nodes.value.find((n) => n.id === selectedNodeId.value),
  );

  const hasUnsavedChanges = computed(() => dirty.value);

  // --------------- Helpers ---------------

  function generateId(): string {
    return (
      crypto.randomUUID?.() ??
      `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`
    );
  }

  function markDirty(flowId?: string): void {
    dirty.value = true;
    const id = flowId ?? activeFlowId.value;
    if (id) {
      dirtyFlowIds.value = new Set([...dirtyFlowIds.value, id]);
    }
  }

  function markNodeDirty(nodeId: string): void {
    if (!dirtyNodeIds.value.has(nodeId)) {
      dirtyNodeIds.value = new Set([...dirtyNodeIds.value, nodeId]);
    }
    markDirty();
  }

  function isNodeDirty(nodeId: string): boolean {
    return dirtyNodeIds.value.has(nodeId);
  }

  function isFlowDirty(flowId: string): boolean {
    return dirtyFlowIds.value.has(flowId);
  }

  // --------------- Actions ---------------

  function setActiveFlow(flowId: string): void {
    activeFlowId.value = flowId;

    const flow = flows.value.find((f) => f.id === flowId);
    if (flow) {
      nodes.value = flow.nodes.map(flintNodeToVueFlowNode);
      edges.value = buildEdgesFromFlow(flow);
    } else {
      nodes.value = [];
      edges.value = [];
    }

    selectedNodeId.value = null;
  }

  function addFlow(label?: string): Flow {
    const flow: Flow = {
      id: generateId(),
      type: "tab",
      label: label ?? `Flow ${flows.value.length + 1}`,
      nodes: [],
      wires: [],
    };
    flows.value.push(flow);

    // Always sync current canvas before switching, then activate new flow
    if (activeFlowId.value) {
      syncCanvasToActiveFlow();
    }
    setActiveFlow(flow.id);

    markDirty();
    return flow;
  }

  function removeFlow(flowId: string): void {
    if (flows.value.length <= 1) return;

    const idx = flows.value.findIndex((f) => f.id === flowId);
    flows.value = flows.value.filter((f) => f.id !== flowId);

    if (activeFlowId.value === flowId) {
      // Switch to the next flow, or the previous one if we removed the last tab
      const nextIdx = Math.min(idx, flows.value.length - 1);
      setActiveFlow(flows.value[nextIdx].id);
    }
    markDirty();
  }

  function reorderFlows(fromIndex: number, toIndex: number): void {
    if (fromIndex === toIndex) return;
    const moved = flows.value.splice(fromIndex, 1)[0];
    flows.value.splice(toIndex, 0, moved);
    markDirty();
  }

  function updateFlowLabel(flowId: string, label: string): void {
    const flow = flows.value.find((f) => f.id === flowId);
    if (flow) {
      flow.label = label;
      markDirty(flowId);
    }
  }

  function toggleFlowDisabled(flowId: string): void {
    const flow = flows.value.find((f) => f.id === flowId);
    if (flow) {
      flow.disabled = !flow.disabled;
      markDirty(flowId);
    }
  }

  function addNode(
    type: string,
    position: { x: number; y: number },
    data?: Record<string, unknown>,
  ): FlowNode {
    const nodeId = generateId();

    const vfNode: FlowNode = {
      id: nodeId,
      type,
      position,
      data: {
        label: data?.label ?? '',
        nodeType: type,
        config: {},
        ...data,
      },
    };

    nodes.value.push(vfNode);
    markNodeDirty(nodeId);
    return vfNode;
  }

  function removeNode(nodeId: string): void {
    nodes.value = nodes.value.filter((n) => n.id !== nodeId);
    edges.value = edges.value.filter(
      (e) => e.source !== nodeId && e.target !== nodeId,
    );
    if (selectedNodeId.value === nodeId) {
      selectedNodeId.value = null;
    }
    markDirty();
  }

  function updateNodeData(nodeId: string, data: Record<string, unknown>): void {
    const node = nodes.value.find((n) => n.id === nodeId);
    if (node) {
      node.data = { ...node.data, ...data };
      markNodeDirty(nodeId);

      // Remove edges connected to ports that no longer exist
      if (typeof data.outputs === "number") {
        const outputCount = data.outputs as number;
        edges.value = edges.value.filter(
          (e) =>
            e.source !== nodeId ||
            !e.sourceHandle ||
            parseInt(e.sourceHandle.replace("output-", ""), 10) < outputCount,
        );
      }
      if (typeof data.inputs === "number") {
        const inputCount = data.inputs as number;
        edges.value = edges.value.filter(
          (e) =>
            e.target !== nodeId ||
            !e.targetHandle ||
            parseInt(e.targetHandle.replace("input-", ""), 10) < inputCount,
        );
      }
    }
  }

  function updateNodeStatus(nodeId: string, status: { fill: string; text: string }): void {
    // Search active flow nodes first
    let node = nodes.value.find((n) => n.id === nodeId);

    // If not in active flow, search across all flows and update the raw Flint node
    if (!node) {
      for (const flow of flows.value) {
        const flintNode = flow.nodes.find((n) => n.id === nodeId);
        if (flintNode) {
          flintNode.status = { fill: status.fill as any, shape: 'dot', text: status.text };
          return;
        }
      }
      return;
    }

    // Mutate existing data object to preserve Vue reactivity
    if (!node.data) node.data = {};
    node.data.status = { fill: status.fill, shape: 'dot' as const, text: status.text };
  }

  function updateNodePosition(
    nodeId: string,
    position: { x: number; y: number },
  ): void {
    const node = nodes.value.find((n) => n.id === nodeId);
    if (node) {
      node.position = { ...position };
      markNodeDirty(nodeId);
    }
  }

  function connectNodes(params: {
    source: string;
    target: string;
    sourceHandle?: string | null;
    targetHandle?: string | null;
  }): FlowEdge | null {
    const edgeId = `e-${params.source}-${params.target}-${Date.now()}`;

    const exists = edges.value.some(
      (e) =>
        e.source === params.source &&
        e.target === params.target &&
        e.sourceHandle === params.sourceHandle &&
        e.targetHandle === params.targetHandle,
    );
    if (exists) return null;

    const edge: FlowEdge = {
      id: edgeId,
      source: params.source,
      target: params.target,
      sourceHandle: params.sourceHandle ?? undefined,
      targetHandle: params.targetHandle ?? undefined,
    };

    edges.value.push(edge);
    markDirty();
    return edge;
  }

  function removeEdge(edgeId: string): void {
    edges.value = edges.value.filter((e) => e.id !== edgeId);
    markDirty();
  }

  function selectNode(nodeId: string | null): void {
    selectedNodeId.value = nodeId;
    selectedNodeIds.value = nodeId ? [nodeId] : [];
  }

  function setSelectedNodeIds(ids: string[]): void {
    selectedNodeIds.value = ids;
    if (ids.length === 1) {
      selectedNodeId.value = ids[0];
    } else if (ids.length === 0) {
      selectedNodeId.value = null;
    }
  }

  function copySelectedNodes(): void {
    const ids = selectedNodeIds.value;
    if (ids.length === 0) return;

    const idSet = new Set(ids);
    const copiedNodes = nodes.value
      .filter((n) => idSet.has(n.id))
      .map((n) => JSON.parse(JSON.stringify(n)));

    const copiedEdges = edges.value
      .filter((e) => idSet.has(e.source) && idSet.has(e.target))
      .map((e) => JSON.parse(JSON.stringify(e)));

    clipboard.value = { nodes: copiedNodes, edges: copiedEdges };
  }

  function pasteNodes(targetPosition?: { x: number; y: number }): FlowNode[] {
    if (!clipboard.value || clipboard.value.nodes.length === 0) return [];

    // Calculate offset: either relative to mouse position or a fixed offset
    let offsetX = 32;
    let offsetY = 32;

    if (targetPosition) {
      // Find the bounding box origin of copied nodes
      const minX = Math.min(...clipboard.value.nodes.map((n) => n.position.x));
      const minY = Math.min(...clipboard.value.nodes.map((n) => n.position.y));
      offsetX = targetPosition.x - minX;
      offsetY = targetPosition.y - minY;
    }

    const idMap = new Map<string, string>();
    const newNodes: FlowNode[] = [];

    for (const original of clipboard.value.nodes) {
      const newId = generateId();
      idMap.set(original.id, newId);

      const newNode: FlowNode = {
        ...JSON.parse(JSON.stringify(original)),
        id: newId,
        position: {
          x: original.position.x + offsetX,
          y: original.position.y + offsetY,
        },
      };

      nodes.value.push(newNode);
      newNodes.push(newNode);
      markNodeDirty(newId);
    }

    for (const original of clipboard.value.edges) {
      const newSource = idMap.get(original.source);
      const newTarget = idMap.get(original.target);
      if (!newSource || !newTarget) continue;

      edges.value.push({
        ...JSON.parse(JSON.stringify(original)),
        id: `e-${newSource}-${newTarget}-${Date.now()}`,
        source: newSource,
        target: newTarget,
      });
    }

    // Deselect all nodes, then select only pasted ones
    const newIdSet = new Set(idMap.values());
    for (const node of nodes.value) {
      node.selected = newIdSet.has(node.id);
    }
    setSelectedNodeIds([...idMap.values()]);

    markDirty();
    return newNodes;
  }

  function cutSelectedNodes(): void {
    copySelectedNodes();
    const ids = [...selectedNodeIds.value];
    for (const id of ids) {
      removeNode(id);
    }
  }

  function duplicateSelectedNodes(): FlowNode[] {
    copySelectedNodes();
    return pasteNodes();
  }

  function loadFlows(loadedFlows: Flow[], rev?: string, loadedConfigs?: ConfigNode[]): void {
    flows.value = loadedFlows;
    configs.value = loadedConfigs ?? [];
    revision.value = rev ?? null;

    if (loadedFlows.length > 0) {
      setActiveFlow(loadedFlows[0].id);
    } else {
      activeFlowId.value = null;
      nodes.value = [];
      edges.value = [];
    }

    dirty.value = false;
    dirtyNodeIds.value.clear();
    dirtyFlowIds.value = new Set();
  }

  async function deploy(): Promise<DeployResponse | null> {
    if (deploying.value) return null;

    const ui = useUiStore();
    deploying.value = true;
    ui.setDeployStatus('deploying');

    try {
      // Sync current canvas state back into the active flow before deploying
      syncCanvasToActiveFlow();

      const payload: DeployPayload = {
        flows: flows.value,
        configs: configs.value.length > 0 ? configs.value : undefined,
        rev: revision.value ?? undefined,
        deployMode: deployMode.value,
      };

      const response = await fetch("/api/v1/flows", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      });

      if (!response.ok) {
        const errorBody = await response.text();
        throw new Error(`Deploy failed: ${response.status} — ${errorBody}`);
      }

      const result: DeployResponse = await response.json();
      revision.value = result.rev;
      dirty.value = false;
      dirtyNodeIds.value = new Set();
      dirtyFlowIds.value = new Set();
      ui.setDeployStatus('deployed');

      return result;
    } catch (err) {
      console.error("[FlowStore] Deploy error:", err);
      ui.setDeployStatus('failed');
      return null;
    } finally {
      deploying.value = false;
    }
  }

  // --------------- Internal Converters ---------------

  function flintNodeToVueFlowNode(flintNode: FlintNode): FlowNode {
    return {
      id: flintNode.id,
      type: flintNode.type,
      position: { x: flintNode.x, y: flintNode.y },
      data: {
        label: flintNode.name ?? flintNode.label ?? '',
        nodeType: flintNode.type,
        config: flintNode.config ?? {},
        status: flintNode.status ?? null,
        inputs: flintNode.inputs,
        outputs: flintNode.outputs,
        disabled: flintNode.disabled ?? false,
      },
    };
  }

  function buildEdgesFromFlow(flow: Flow): FlowEdge[] {
    const result: FlowEdge[] = [];

    for (const node of flow.nodes) {
      if (!node.wires) continue;

      for (let outputIdx = 0; outputIdx < node.wires.length; outputIdx++) {
        const targets = node.wires[outputIdx];
        if (!targets) continue;

        for (const targetId of targets) {
          result.push({
            id: `e-${node.id}-${outputIdx}-${targetId}`,
            source: node.id,
            target: targetId,
            sourceHandle: `output-${outputIdx}`,
          });
        }
      }
    }

    // Also use wire entries if available
    for (const wire of flow.wires ?? []) {
      const exists = result.some(
        (e) => e.source === wire.sourceNode && e.target === wire.targetNode,
      );
      if (!exists) {
        result.push({
          id: wire.id,
          source: wire.sourceNode,
          target: wire.targetNode,
          sourceHandle: `output-${wire.sourcePort}`,
          targetHandle: `input-${wire.targetPort}`,
        });
      }
    }

    return result;
  }

  function syncCanvasToActiveFlow(): void {
    const flow = flows.value.find((f) => f.id === activeFlowId.value);
    if (!flow) return;

    flow.nodes = nodes.value.map((vfNode) => ({
      id: vfNode.id,
      type: vfNode.data?.nodeType ?? vfNode.type ?? "unknown",
      name: vfNode.data?.label ?? "",
      x: vfNode.position.x,
      y: vfNode.position.y,
      z: flow.id,
      inputs: vfNode.data?.inputs ?? 0,
      outputs: vfNode.data?.outputs ?? 1,
      wires: buildWiresForNode(vfNode.id),
      config: vfNode.data?.config ?? {},
      disabled: vfNode.data?.disabled ?? false,
    }));

    flow.wires = edges.value.map((edge) => ({
      id: edge.id,
      sourceNode: edge.source,
      sourcePort: parsePortIndex(edge.sourceHandle, "output"),
      targetNode: edge.target,
      targetPort: parsePortIndex(edge.targetHandle, "input"),
    }));
  }

  function buildWiresForNode(nodeId: string): string[][] {
    const outputMap = new Map<number, string[]>();

    for (const edge of edges.value) {
      if (edge.source !== nodeId) continue;
      const portIdx = parsePortIndex(edge.sourceHandle, "output");
      const targets = outputMap.get(portIdx) ?? [];
      targets.push(edge.target);
      outputMap.set(portIdx, targets);
    }

    const maxPort = outputMap.size > 0 ? Math.max(...outputMap.keys()) : -1;
    const wires: string[][] = [];
    for (let i = 0; i <= maxPort; i++) {
      wires.push(outputMap.get(i) ?? []);
    }

    return wires;
  }

  function parsePortIndex(
    handle: string | undefined | null,
    prefix: string,
  ): number {
    if (!handle) return 0;
    const match = handle.match(new RegExp(`^${prefix}-(\\d+)$`));
    return match ? parseInt(match[1], 10) : 0;
  }

  // --------------- Config Node CRUD ---------------

  function addConfig(config: ConfigNode): void {
    configs.value.push(config);
    dirty.value = true;
  }

  function updateConfig(id: string, updates: Partial<ConfigNode>): void {
    const idx = configs.value.findIndex((c) => c.id === id);
    if (idx !== -1) {
      configs.value[idx] = { ...configs.value[idx], ...updates };
      dirty.value = true;
    }
  }

  function removeConfig(id: string): void {
    configs.value = configs.value.filter((c) => c.id !== id);
    dirty.value = true;
  }

  function getConfigsByType(type: string): ConfigNode[] {
    return configs.value.filter((c) => c.type === type);
  }

  function setDeployMode(mode: DeployModeType): void {
    deployMode.value = mode;
    localStorage.setItem('flint-deploy-mode', mode);
  }

  // --------------- Return ---------------

  return {
    // State
    flows,
    configs,
    activeFlowId,
    nodes,
    edges,
    selectedNodeId,
    selectedNodeIds,
    clipboard,
    dirty,
    dirtyNodeIds,
    dirtyFlowIds,
    revision,
    deploying,
    deployMode,

    // Getters
    activeFlow,
    activeNodes,
    activeEdges,
    selectedNode,
    hasUnsavedChanges,

    // Actions
    setActiveFlow,
    addFlow,
    removeFlow,
    reorderFlows,
    updateFlowLabel,
    toggleFlowDisabled,
    addNode,
    removeNode,
    updateNodeData,
    updateNodeStatus,
    updateNodePosition,
    isNodeDirty,
    isFlowDirty,
    markNodeDirty,
    connectNodes,
    removeEdge,
    selectNode,
    setSelectedNodeIds,
    copySelectedNodes,
    pasteNodes,
    cutSelectedNodes,
    duplicateSelectedNodes,
    loadFlows,
    deploy,
    setDeployMode,
    syncCanvasToActiveFlow,
    addConfig,
    updateConfig,
    removeConfig,
    getConfigsByType,
  };
});
