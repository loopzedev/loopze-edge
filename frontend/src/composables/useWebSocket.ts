import { ref, onUnmounted } from 'vue'
import type {
  WebSocketEventType,
  WebSocketEnvelope,
  WebSocketPayloadMap,
  DebugMessage,
  StatusEvent,
  DeployEvent,
  NotificationEvent,
  LogEntry,
} from '@/types/events'

export type ConnectionStatus = 'connecting' | 'connected' | 'disconnected' | 'reconnecting'

type MessageCallback<T extends WebSocketEventType> = (payload: WebSocketPayloadMap[T]) => void

interface UseWebSocketOptions {
  url?: string
  autoConnect?: boolean
  maxReconnectAttempts?: number
  baseReconnectDelay?: number
  maxReconnectDelay?: number
}

const DEFAULT_OPTIONS: Required<UseWebSocketOptions> = {
  url: `${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}/ws`,
  autoConnect: true,
  maxReconnectAttempts: Infinity,
  baseReconnectDelay: 1000,
  maxReconnectDelay: 30000,
}

export function useWebSocket(options: UseWebSocketOptions = {}) {
  const opts = { ...DEFAULT_OPTIONS, ...options }

  const status = ref<ConnectionStatus>('disconnected')
  const lastMessage = ref<WebSocketEnvelope | null>(null)
  const reconnectAttempts = ref(0)

  let socket: WebSocket | null = null
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null
  let isManualClose = false

  const listeners: {
    debug: Set<MessageCallback<'debug'>>
    status: Set<MessageCallback<'status'>>
    deploy: Set<MessageCallback<'deploy'>>
    notification: Set<MessageCallback<'notification'>>
    log: Set<MessageCallback<'log'>>
  } = {
    debug: new Set(),
    status: new Set(),
    deploy: new Set(),
    notification: new Set(),
    log: new Set(),
  }

  function getReconnectDelay(): number {
    const attempt = reconnectAttempts.value
    const delay = Math.min(
      opts.baseReconnectDelay * Math.pow(2, attempt),
      opts.maxReconnectDelay
    )
    // Add jitter (±25%)
    const jitter = delay * 0.25 * (Math.random() * 2 - 1)
    return Math.round(delay + jitter)
  }

  function connect(): void {
    if (socket && (socket.readyState === WebSocket.OPEN || socket.readyState === WebSocket.CONNECTING)) {
      return
    }

    isManualClose = false
    status.value = reconnectAttempts.value > 0 ? 'reconnecting' : 'connecting'

    try {
      socket = new WebSocket(opts.url)
    } catch (err) {
      console.error('[LOOPZE WS] Failed to create WebSocket:', err)
      scheduleReconnect()
      return
    }

    socket.onopen = () => {
      status.value = 'connected'
      reconnectAttempts.value = 0
      console.info('[LOOPZE WS] Connected')
    }

    socket.onclose = (event) => {
      status.value = 'disconnected'
      console.info(`[LOOPZE WS] Closed (code=${event.code}, reason=${event.reason || 'none'})`)

      if (!isManualClose) {
        scheduleReconnect()
      }
    }

    socket.onerror = (event) => {
      console.error('[LOOPZE WS] Error:', event)
    }

    socket.onmessage = (event) => {
      try {
        const envelope = JSON.parse(event.data) as WebSocketEnvelope
        lastMessage.value = envelope
        dispatch(envelope)
      } catch (err) {
        console.warn('[LOOPZE WS] Failed to parse message:', event.data, err)
      }
    }
  }

  function disconnect(): void {
    isManualClose = true
    clearReconnectTimer()

    if (socket) {
      socket.close(1000, 'Client disconnect')
      socket = null
    }

    status.value = 'disconnected'
    reconnectAttempts.value = 0
  }

  function scheduleReconnect(): void {
    if (isManualClose) return
    if (reconnectAttempts.value >= opts.maxReconnectAttempts) {
      console.warn('[LOOPZE WS] Max reconnect attempts reached')
      return
    }

    clearReconnectTimer()

    const delay = getReconnectDelay()
    reconnectAttempts.value++
    status.value = 'reconnecting'

    console.info(`[LOOPZE WS] Reconnecting in ${delay}ms (attempt ${reconnectAttempts.value})`)

    reconnectTimer = setTimeout(() => {
      connect()
    }, delay)
  }

  function clearReconnectTimer(): void {
    if (reconnectTimer !== null) {
      clearTimeout(reconnectTimer)
      reconnectTimer = null
    }
  }

  function dispatch(envelope: WebSocketEnvelope): void {
    const { type, payload } = envelope

    switch (type) {
      case 'debug':
        listeners.debug.forEach((cb) => cb(payload as DebugMessage))
        break
      case 'status':
        listeners.status.forEach((cb) => cb(payload as StatusEvent))
        break
      case 'deploy':
        listeners.deploy.forEach((cb) => cb(payload as DeployEvent))
        break
      case 'notification':
        listeners.notification.forEach((cb) => cb(payload as NotificationEvent))
        break
      case 'log':
        listeners.log.forEach((cb) => cb(payload as LogEntry))
        break
      default:
        console.warn(`[LOOPZE WS] Unknown message type: ${type}`)
    }
  }

  function onDebug(callback: MessageCallback<'debug'>): () => void {
    listeners.debug.add(callback)
    return () => listeners.debug.delete(callback)
  }

  function onStatus(callback: MessageCallback<'status'>): () => void {
    listeners.status.add(callback)
    return () => listeners.status.delete(callback)
  }

  function onDeploy(callback: MessageCallback<'deploy'>): () => void {
    listeners.deploy.add(callback)
    return () => listeners.deploy.delete(callback)
  }

  function onNotification(callback: MessageCallback<'notification'>): () => void {
    listeners.notification.add(callback)
    return () => listeners.notification.delete(callback)
  }

  function onLog(callback: MessageCallback<'log'>): () => void {
    listeners.log.add(callback)
    return () => listeners.log.delete(callback)
  }

  function send(data: unknown): void {
    if (!socket || socket.readyState !== WebSocket.OPEN) {
      console.warn('[LOOPZE WS] Cannot send, socket not open')
      return
    }

    const message = typeof data === 'string' ? data : JSON.stringify(data)
    socket.send(message)
  }

  // Auto-connect if enabled
  if (opts.autoConnect) {
    connect()
  }

  // Cleanup on component unmount
  onUnmounted(() => {
    disconnect()
  })

  return {
    status,
    lastMessage,
    reconnectAttempts,
    connect,
    disconnect,
    send,
    onDebug,
    onStatus,
    onDeploy,
    onNotification,
    onLog,
  }
}
