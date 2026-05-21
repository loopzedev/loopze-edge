// Dashboard WebSocket client with automatic reconnect.
//
// Reconnect policy: exponential backoff 250 ms → 8 s (capped). The caller
// gets a "connecting" / "open" / "closed" state ref it can render. After
// 2 s of disconnect a "reconnecting…" banner is appropriate in the UI.

import { ref, type Ref } from 'vue'
import type { ClientFrame, ServerFrame } from './types'

const MIN_DELAY_MS = 250
const MAX_DELAY_MS = 8000

export type WsState = 'connecting' | 'open' | 'closed'

export interface WsClient {
  state: Ref<WsState>
  /** Milliseconds since the connection was last lost. 0 while open. */
  downSince: Ref<number>
  send: (frame: ClientFrame) => void
  close: () => void
}

export function createWsClient(onFrame: (frame: ServerFrame) => void): WsClient {
  const state = ref<WsState>('connecting')
  const downSince = ref(0)

  let socket: WebSocket | null = null
  let reconnectDelay = MIN_DELAY_MS
  let reconnectTimer: number | undefined
  let closedByCaller = false
  let downSinceTs = 0
  let downTickTimer: number | undefined

  function url(): string {
    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
    return `${proto}//${location.host}/api/dashboard/ws`
  }

  function startDownTick() {
    stopDownTick()
    downSinceTs = Date.now()
    downSince.value = 0
    downTickTimer = window.setInterval(() => {
      downSince.value = Date.now() - downSinceTs
    }, 200)
  }

  function stopDownTick() {
    if (downTickTimer !== undefined) {
      window.clearInterval(downTickTimer)
      downTickTimer = undefined
    }
    downSince.value = 0
  }

  function connect() {
    state.value = 'connecting'
    const s = new WebSocket(url())
    socket = s

    s.onopen = () => {
      state.value = 'open'
      reconnectDelay = MIN_DELAY_MS
      stopDownTick()
      send({ type: 'hello' })
    }

    s.onmessage = (ev) => {
      let frame: ServerFrame
      try {
        frame = JSON.parse(ev.data) as ServerFrame
      } catch {
        return
      }
      onFrame(frame)
    }

    s.onclose = () => {
      socket = null
      state.value = 'closed'
      if (closedByCaller) return
      startDownTick()
      reconnectTimer = window.setTimeout(() => {
        reconnectDelay = Math.min(reconnectDelay * 2, MAX_DELAY_MS)
        connect()
      }, reconnectDelay)
    }

    s.onerror = () => {
      // The browser also fires onclose; let that path handle the retry.
    }
  }

  function send(frame: ClientFrame) {
    if (socket && socket.readyState === WebSocket.OPEN) {
      socket.send(JSON.stringify(frame))
    }
  }

  function close() {
    closedByCaller = true
    if (reconnectTimer !== undefined) {
      window.clearTimeout(reconnectTimer)
      reconnectTimer = undefined
    }
    stopDownTick()
    if (socket) {
      socket.close()
      socket = null
    }
  }

  connect()

  return { state, downSince, send, close }
}
