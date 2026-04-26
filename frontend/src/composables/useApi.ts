import type {
  Flow,
  FlowsResponse,
  DeployPayload,
  DeployResponse,
  NodeCatalogEntry,
} from '@/types/flow'
import type { LogEntry } from '@/types/events'

export interface ApiError {
  status: number
  message: string
  details?: unknown
}

export type ContextScope = 'global' | 'flow'
export type ContextStorage = 'memory' | 'persistent'

export interface ContextEntry {
  key: string
  value: unknown
}

export interface ContextStoreResponse {
  scope: ContextScope
  storage: ContextStorage
  flowId: string
  entries: ContextEntry[]
}

const BASE_URL = '/api/v1'

async function request<T>(
  path: string,
  options: RequestInit = {}
): Promise<T> {
  const url = `${BASE_URL}${path}`

  const headers: Record<string, string> = {
    'Accept': 'application/json',
    ...(options.headers as Record<string, string> || {}),
  }

  if (options.body && typeof options.body === 'string') {
    headers['Content-Type'] = 'application/json'
  }

  const response = await fetch(url, {
    ...options,
    headers,
  })

  if (!response.ok) {
    let message = `HTTP ${response.status}: ${response.statusText}`
    let details: unknown = undefined

    try {
      const errorBody = await response.json()
      if (errorBody.message) {
        message = errorBody.message
      }
      details = errorBody
    } catch {
      // response body is not JSON, use default message
    }

    const error: ApiError = {
      status: response.status,
      message,
      details,
    }

    throw error
  }

  // Handle 204 No Content
  if (response.status === 204) {
    return undefined as T
  }

  return response.json() as Promise<T>
}

export function useApi() {
  /**
   * Fetch all flows from the backend.
   */
  async function getFlows(): Promise<FlowsResponse> {
    return request<FlowsResponse>('/flows')
  }

  /**
   * Deploy flows to the backend runtime.
   */
  async function deployFlows(payload: DeployPayload): Promise<DeployResponse> {
    return request<DeployResponse>('/flows', {
      method: 'POST',
      body: JSON.stringify(payload),
    })
  }

  /**
   * Fetch a single flow by ID.
   */
  async function getFlow(flowId: string): Promise<Flow> {
    return request<Flow>(`/flows/${flowId}`)
  }

  /**
   * Update a single flow by ID.
   */
  async function updateFlow(flowId: string, flow: Flow): Promise<Flow> {
    return request<Flow>(`/flows/${flowId}`, {
      method: 'PUT',
      body: JSON.stringify(flow),
    })
  }

  /**
   * Delete a flow by ID.
   */
  async function deleteFlow(flowId: string): Promise<void> {
    return request<void>(`/flows/${flowId}`, {
      method: 'DELETE',
    })
  }

  /**
   * Fetch available node types from the catalog.
   */
  async function getNodes(): Promise<NodeCatalogEntry[]> {
    const res = await request<{ nodes: NodeCatalogEntry[] }>('/nodes')
    return res.nodes ?? []
  }

  /**
   * Fetch registered config node types.
   */
  async function getConfigTypes(): Promise<{ type: string; label: string; description: string; defaults: Record<string, unknown> }[]> {
    const res = await request<{ types: any[] }>('/configs/types')
    return res.types ?? []
  }

  /**
   * Fetch application settings.
   */
  async function getSettings(): Promise<Record<string, unknown>> {
    return request<Record<string, unknown>>('/settings')
  }

  /**
   * Update application settings.
   */
  async function updateSettings(
    settings: Record<string, unknown>
  ): Promise<Record<string, unknown>> {
    return request<Record<string, unknown>>('/settings', {
      method: 'PUT',
      body: JSON.stringify(settings),
    })
  }

  /**
   * Fetch last known status for all running nodes.
   */
  async function getNodeStatuses(): Promise<Record<string, { nodeId: string; flowId: string; status: { fill: string; text: string } }>> {
    const res = await request<{ statuses: Record<string, any> }>('/status/nodes')
    return res.statuses ?? {}
  }

  /**
   * Trigger an inject node to fire its payload.
   */
  async function triggerInject(nodeId: string): Promise<void> {
    return request<void>(`/inject/${nodeId}`, {
      method: 'POST',
    })
  }

  /**
   * Fetch the most recent application log entries from the in-memory ring
   * buffer that backs the Terminal Log panel. Returns oldest-first; the
   * server clamps limit to [1, 1000].
   */
  async function getLogs(limit: number): Promise<LogEntry[]> {
    return request<LogEntry[]>(`/logs?limit=${limit}`)
  }

  // ── Context store endpoints ──────────────────────────────────────────────

  function ctxBasePath(scope: ContextScope, storage: ContextStorage, flowId?: string | null): string {
    if (scope === 'flow') {
      if (!flowId) throw new Error('flowId required for flow-scoped context')
      return `/context/flow/${encodeURIComponent(flowId)}/${storage}`
    }
    return `/context/global/${storage}`
  }

  async function getContext(
    scope: ContextScope,
    storage: ContextStorage,
    flowId?: string | null,
  ): Promise<ContextStoreResponse> {
    return request<ContextStoreResponse>(ctxBasePath(scope, storage, flowId))
  }

  async function getContextKey(
    scope: ContextScope,
    storage: ContextStorage,
    key: string,
    flowId?: string | null,
  ): Promise<ContextEntry> {
    return request<ContextEntry>(`${ctxBasePath(scope, storage, flowId)}/${encodeURIComponent(key)}`)
  }

  async function deleteContextKey(
    scope: ContextScope,
    storage: ContextStorage,
    key: string,
    flowId?: string | null,
  ): Promise<void> {
    return request<void>(`${ctxBasePath(scope, storage, flowId)}/${encodeURIComponent(key)}`, {
      method: 'DELETE',
    })
  }

  async function clearContext(
    scope: ContextScope,
    storage: ContextStorage,
    flowId?: string | null,
  ): Promise<{ deleted: number }> {
    return request<{ deleted: number }>(ctxBasePath(scope, storage, flowId), {
      method: 'DELETE',
    })
  }

  /**
   * Check if an error is an ApiError.
   */
  function isApiError(error: unknown): error is ApiError {
    return (
      typeof error === 'object' &&
      error !== null &&
      'status' in error &&
      'message' in error
    )
  }

  return {
    getFlows,
    deployFlows,
    getFlow,
    updateFlow,
    deleteFlow,
    getNodes,
    getConfigTypes,
    getSettings,
    updateSettings,
    getNodeStatuses,
    triggerInject,
    getLogs,
    getContext,
    getContextKey,
    deleteContextKey,
    clearContext,
    isApiError,
  }
}
