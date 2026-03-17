/**
 * WebSocket event types for Flint real-time communication.
 */

export type WebSocketEventType = 'debug' | 'status' | 'deploy' | 'notification'

export interface WebSocketEnvelope<T extends WebSocketEventType = WebSocketEventType> {
  type: T
  timestamp: string
  payload: WebSocketPayloadMap[T]
}

export interface DebugMessage {
  id: string
  nodeId: string
  nodeName: string
  flowId: string
  timestamp: string
  status: 'debug' | 'warn' | 'error'
  payload: unknown
  format: 'string' | 'number' | 'boolean' | 'object' | 'array' | 'buffer' | 'undefined' | 'null'
  property: string
}

export interface StatusEvent {
  nodeId: string
  flowId: string
  status: NodeStatus
}

export interface NodeStatus {
  fill: 'red' | 'green' | 'yellow' | 'blue' | 'grey'
  shape: 'ring' | 'dot'
  text: string
}

export interface DeployEvent {
  action: 'deploying' | 'deployed' | 'failed'
  revision: string
  message?: string
}

export type NotificationLevel = 'info' | 'success' | 'warning' | 'error'

export interface NotificationEvent {
  id: string
  level: NotificationLevel
  title: string
  message: string
  timeout?: number
}

export interface WebSocketPayloadMap {
  debug: DebugMessage
  status: StatusEvent
  deploy: DeployEvent
  notification: NotificationEvent
}

export type WebSocketMessageHandler<T extends WebSocketEventType> = (
  payload: WebSocketPayloadMap[T]
) => void

export interface WebSocketHandlers {
  onDebug?: WebSocketMessageHandler<'debug'>
  onStatus?: WebSocketMessageHandler<'status'>
  onDeploy?: WebSocketMessageHandler<'deploy'>
  onNotification?: WebSocketMessageHandler<'notification'>
}
