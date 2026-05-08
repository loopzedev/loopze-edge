import type {
  Flow,
  FlowsResponse,
  DeployPayload,
  DeployResponse,
  NodeCatalogEntry,
} from '@/types/flow'
import type { LogEntry } from '@/types/events'
import type { Role, User } from '@/types/auth'
import { basePathNoSlash, readCookie } from '@/runtime'

export interface ApiError {
  status: number
  message: string
  details?: unknown
}

// Hooks the auth store registers with setApiHooks(). request() invokes
// them on the corresponding HTTP status codes so the store can react to
// "session lost" / "first-run setup needed" without every caller doing
// it manually.
interface ApiHooks {
  onUnauthorized?: () => void
  onSetupRequired?: () => void
}
let hooks: ApiHooks = {}

export function setApiHooks(next: ApiHooks): void {
  hooks = { ...hooks, ...next }
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

export interface StateMachineListItem {
  nodeID: string
  label: string
  currentState: string
}

export interface StateMachineListEntry {
  flowID: string
  flowLabel: string
  nodeID: string
  label: string
  currentState: string
}

export interface StateMachineListResponse {
  flowID: string
  machines: StateMachineListItem[] | null
}

export interface StateMachineAllListResponse {
  machines: StateMachineListEntry[] | null
}

export interface StateMachineTransitionEntry {
  ts: string
  from: string
  to: string
  event: string
}

export interface StateMachineSnapshot {
  machineId: string
  currentState: string
  states: string[]
  initial: string
  context: Record<string, unknown>
  availableEvents: string[]
  history: StateMachineTransitionEntry[]
}

export interface StateMachineSnapshotResponse {
  flowID: string
  nodeID: string
  snapshot: StateMachineSnapshot
}

export interface OpcuaDataTypeInfo {
  nodeId: string
  name?: string
  isStructure: boolean
}

export interface OpcuaBrowseChild {
  nodeId: string
  browseName: string
  displayName: string
  nodeClass: string
  hasChildren: boolean
  dataType?: OpcuaDataTypeInfo
  valueRank?: number
  accessLevel?: string
  description?: string
  /** True for tree rows that don't exist on the server but represent
   *  struct fields surfaced from a DataTypeDefinition. Not selectable. */
  synthetic?: boolean
}

export interface OpcuaBrowseResult {
  parent?: OpcuaBrowseChild
  children: OpcuaBrowseChild[]
  continuationPoint?: string
}

export interface OpcuaBrowseResponse {
  ok: boolean
  error?: string
  result?: OpcuaBrowseResult
}

export interface OpcuaReadResult {
  nodeId: string
  statusCode: string
  statusCodeRaw: number
  value: unknown
  dataType?: string
  sourceTimestamp?: string
  serverTimestamp?: string
}

export interface OpcuaReadResponse {
  ok: boolean
  error?: string
  result?: OpcuaReadResult
}

const BASE_URL = `${basePathNoSlash}/api/v1`

const CSRF_COOKIE = 'loopze_csrf'
const CSRF_HEADER = 'X-CSRF-Token'
const MUTATING_METHODS = new Set(['POST', 'PUT', 'PATCH', 'DELETE'])

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

  // Double-submit-cookie CSRF: echo the loopze_csrf cookie back as a
  // header on state-changing requests. The cookie is issued by the
  // backend on every API response, so by the time the SPA does its
  // first mutation it has already been set during /auth/status.
  const method = (options.method ?? 'GET').toUpperCase()
  if (MUTATING_METHODS.has(method)) {
    const token = readCookie(CSRF_COOKIE)
    if (token) {
      headers[CSRF_HEADER] = token
    }
  }

  const response = await fetch(url, {
    credentials: 'same-origin',
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

    if (response.status === 401) {
      hooks.onUnauthorized?.()
    } else if (response.status === 503) {
      hooks.onSetupRequired?.()
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

  // ── State Machine inspector ──────────────────────────────────────────────

  async function listStateMachines(flowId: string): Promise<StateMachineListResponse> {
    return request<StateMachineListResponse>(
      `/state-machines/flow/${encodeURIComponent(flowId)}`,
    )
  }

  async function listAllStateMachines(): Promise<StateMachineAllListResponse> {
    return request<StateMachineAllListResponse>('/state-machines')
  }

  async function getStateMachineSnapshot(
    flowId: string,
    nodeId: string,
  ): Promise<StateMachineSnapshotResponse> {
    return request<StateMachineSnapshotResponse>(
      `/state-machines/flow/${encodeURIComponent(flowId)}/${encodeURIComponent(nodeId)}`,
    )
  }

  // ── Auth endpoints ───────────────────────────────────────────────────────

  async function setupAdmin(username: string, password: string): Promise<User> {
    const res = await request<{ user: User }>('/setup', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    })
    return res.user
  }

  async function login(username: string, password: string): Promise<User> {
    const res = await request<{ user: User }>('/auth/login', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    })
    return res.user
  }

  async function logout(): Promise<void> {
    return request<void>('/auth/logout', { method: 'POST' })
  }

  async function getMe(): Promise<User> {
    const res = await request<{ user: User }>('/auth/me')
    return res.user
  }

  /** Returns the joint setup + auth state in a single call. Used at app
   *  start so the SPA can pick the initial UI (Setup / Login / Editor)
   *  without ambiguity between "no admin yet" and "logged out". */
  async function getAuthStatus(): Promise<{
    needsSetup: boolean
    authenticated: boolean
    user?: User
  }> {
    return request<{ needsSetup: boolean; authenticated: boolean; user?: User }>(
      '/auth/status',
    )
  }

  // ── User management endpoints (admin only) ───────────────────────────────

  async function listUsers(): Promise<User[]> {
    const res = await request<{ users: User[] }>('/users')
    return res.users ?? []
  }

  async function createUser(username: string, password: string, role: Role): Promise<User> {
    const res = await request<{ user: User }>('/users', {
      method: 'POST',
      body: JSON.stringify({ username, password, role }),
    })
    return res.user
  }

  async function updateUser(id: string, patch: { role?: Role; disabled?: boolean }): Promise<User> {
    const res = await request<{ user: User }>(`/users/${encodeURIComponent(id)}`, {
      method: 'PATCH',
      body: JSON.stringify(patch),
    })
    return res.user
  }

  async function setUserPassword(id: string, password: string): Promise<void> {
    return request<void>(`/users/${encodeURIComponent(id)}/password`, {
      method: 'POST',
      body: JSON.stringify({ password }),
    })
  }

  // ── OPC UA endpoints ─────────────────────────────────────────────────────

  /**
   * Probe an OPC UA server config without persisting it. Used by the
   * "Test Connection" button in the OPC UA Server config dialog.
   */
  async function testOpcuaConnection(payload: {
    id?: string
    name?: string
    config: Record<string, unknown>
  }): Promise<{ ok: boolean; error?: string; serverInfo?: { endpointUrl: string; serverTime?: string } }> {
    return request<{ ok: boolean; error?: string; serverInfo?: { endpointUrl: string; serverTime?: string } }>(
      '/opcua/test-connection',
      {
        method: 'POST',
        body: JSON.stringify(payload),
      },
    )
  }

  /**
   * Browse one level of the OPC UA address space. Either serverId (riding
   * along on a deployed session) or config (transient ad-hoc session) must
   * be supplied. nodeId defaults to the Objects folder when empty.
   */
  async function browseOpcua(payload: {
    serverId?: string
    config?: Record<string, unknown>
    nodeId?: string
  }): Promise<OpcuaBrowseResponse> {
    return request<OpcuaBrowseResponse>('/opcua/browse', {
      method: 'POST',
      body: JSON.stringify(payload),
    })
  }

  /**
   * One-shot Read of a single OPC UA NodeID. Used by the "Read now" button
   * in the address-space browser to inspect the current value before
   * committing to a subscribe/read configuration.
   */
  async function readOpcua(payload: {
    serverId?: string
    config?: Record<string, unknown>
    nodeId: string
  }): Promise<OpcuaReadResponse> {
    return request<OpcuaReadResponse>('/opcua/read', {
      method: 'POST',
      body: JSON.stringify(payload),
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
    listStateMachines,
    listAllStateMachines,
    getStateMachineSnapshot,
    setupAdmin,
    login,
    logout,
    getMe,
    getAuthStatus,
    listUsers,
    createUser,
    updateUser,
    setUserPassword,
    testOpcuaConnection,
    browseOpcua,
    readOpcua,
    isApiError,
  }
}
