import { ref, shallowRef, getCurrentInstance, onUnmounted, watch } from 'vue'
import { hasAdminCapability, useAdminCapabilities } from './useAdminCapabilities'

export const FILE_UPLOAD_STATUS_EVENT = 'files.upload.status' as const
export const FILE_DELETED_EVENT = 'file.deleted' as const
export const FILE_AFTER_PROCESS_EVENT = 'file.after.process' as const

// Define the structure of the WebSocket messages based on backend events
interface FileUploadEventFile {
  id: number
  fileName: string
  originalName: string
  url: string
  size: number
  mimeType: string
  width?: number
  height?: number
}

export interface FileUploadStatusEvent {
  eventId: string
  eventType: typeof FILE_UPLOAD_STATUS_EVENT
  taskId: number
  status: string
  file?: FileUploadEventFile
  error?: string
  tempFileUrl?: string
  metadata?: Record<string, unknown>
}

export interface FileDeletedEvent {
  id: string
  eventType: typeof FILE_DELETED_EVENT
  fileId: number
  filePath: string
  status: string
  error?: string
}

export interface FileAfterProcessEvent {
  id: string
  eventType: typeof FILE_AFTER_PROCESS_EVENT
  fileId: number
  filePath: string
  status: string
  eventTrigger: string
  error?: string
}

export type WebSocketEventName =
  | typeof FILE_UPLOAD_STATUS_EVENT
  | typeof FILE_DELETED_EVENT
  | typeof FILE_AFTER_PROCESS_EVENT

export interface WebSocketEventMap {
  [FILE_UPLOAD_STATUS_EVENT]: FileUploadStatusEvent
  [FILE_DELETED_EVENT]: FileDeletedEvent
  [FILE_AFTER_PROCESS_EVENT]: FileAfterProcessEvent
}

type WebSocketEvent = WebSocketEventMap[WebSocketEventName]

export type MessageHandler<E extends WebSocketEventName = WebSocketEventName> = (
  event: WebSocketEventMap[E],
) => void

type GenericMessageHandler = (event: WebSocketEvent) => void

const socket = shallowRef<WebSocket | null>(null)
const handlers = ref(new Map<WebSocketEventName, GenericMessageHandler[]>())
const isConnecting = ref(false)
const isConnected = ref(false)

const HEARTBEAT_INTERVAL = 30_000
const RECONNECT_DELAY = 5_000

let heartbeatTimer: number | null = null
let reconnectTimer: number | null = null
let manualDisconnect = false

const startHeartbeat = () => {
  if (heartbeatTimer || typeof window === 'undefined') {
    return
  }

  heartbeatTimer = window.setInterval(() => {
    const ws = socket.value
    if (!ws || ws.readyState !== WebSocket.OPEN) {
      isConnected.value = false
      if (!isConnecting.value && !manualDisconnect) {
        scheduleReconnect()
      }
    }
  }, HEARTBEAT_INTERVAL)
}

const stopHeartbeat = () => {
  if (heartbeatTimer) {
    clearInterval(heartbeatTimer)
    heartbeatTimer = null
  }
}

const scheduleReconnect = () => {
  if (typeof window === 'undefined' || !hasAdminCapability('realtime')) {
    return
  }
  if (reconnectTimer || isConnecting.value || manualDisconnect) {
    return
  }

  reconnectTimer = window.setTimeout(() => {
    reconnectTimer = null
    if (!manualDisconnect) {
      connect()
    }
  }, RECONNECT_DELAY)
}

const clearReconnect = () => {
  if (reconnectTimer) {
    clearTimeout(reconnectTimer)
    reconnectTimer = null
  }
}

const dispatchEvent = (message: WebSocketEvent) => {
  const callbacks = handlers.value.get(message.eventType)
  if (!callbacks?.length) {
    return
  }

  callbacks.forEach((handler) => {
    try {
      handler(message)
    } catch (error) {
      console.error('WebSocket handler error:', error)
    }
  })
}

const connect = () => {
  if (typeof window === 'undefined' || !hasAdminCapability('realtime')) {
    return
  }
  if ((socket.value && isConnected.value) || isConnecting.value) {
    return
  }

  isConnecting.value = true
  const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const wsUrl = `${wsProtocol}//${window.location.host}/ws`

  console.debug('[ws] connecting to', wsUrl)

  try {
    const ws = new WebSocket(wsUrl, 'tgmulti-service-protocol')
    socket.value = ws

    ws.onopen = () => {
      if (socket.value !== ws || !hasAdminCapability('realtime')) {
        ws.close()
        return
      }
      console.debug('[ws] connected')
      socket.value = ws
      isConnecting.value = false
      isConnected.value = true
      manualDisconnect = false
      stopHeartbeat()
      startHeartbeat()
      clearReconnect()
    }

    ws.onmessage = (event) => {
      try {
        const rawMessage = JSON.parse(event.data) as Record<string, unknown>
        const eventType = rawMessage?.eventType

        if (eventType === FILE_UPLOAD_STATUS_EVENT) {
          dispatchEvent(rawMessage as unknown as FileUploadStatusEvent)
        } else if (eventType === FILE_DELETED_EVENT) {
          dispatchEvent(rawMessage as unknown as FileDeletedEvent)
        } else if (eventType === FILE_AFTER_PROCESS_EVENT) {
          dispatchEvent(rawMessage as unknown as FileAfterProcessEvent)
        }
      } catch (error) {
        console.error('[ws] message parse error:', error)
      }
    }

    ws.onclose = () => {
      if (socket.value !== ws) {
        return
      }
      console.debug('[ws] closed')
      socket.value = null
      isConnecting.value = false
      isConnected.value = false
      stopHeartbeat()
      if (!manualDisconnect) {
        scheduleReconnect()
      } else {
        manualDisconnect = false
      }
    }

    ws.onerror = (error) => {
      if (socket.value !== ws) {
        return
      }
      console.error('[ws] error:', error)
      isConnecting.value = false
      isConnected.value = false
      if (!manualDisconnect) {
        scheduleReconnect()
      }
    }
  } catch (error) {
    console.error('[ws] connection failed:', error)
    isConnecting.value = false
    if (!manualDisconnect) {
      scheduleReconnect()
    }
  }
}

const off = <E extends WebSocketEventName>(eventName: E, handler: MessageHandler<E>) => {
  if (!handlers.value.has(eventName)) {
    return
  }
  const eventHandlers = handlers.value.get(eventName)
  const handlerIndex = eventHandlers?.indexOf(handler as GenericMessageHandler)
  if (handlerIndex !== undefined && handlerIndex > -1) {
    eventHandlers?.splice(handlerIndex, 1)
  }
}

const on = <E extends WebSocketEventName>(eventName: E, handler: MessageHandler<E>) => {
  if (!handlers.value.has(eventName)) {
    handlers.value.set(eventName, [])
  }

  handlers.value.get(eventName)?.push(handler as GenericMessageHandler)

  return () => off(eventName, handler)
}

const disconnect = () => {
  manualDisconnect = true
  stopHeartbeat()
  clearReconnect()
  if (socket.value) {
    socket.value.close()
    socket.value = null
  }
  isConnected.value = false
  isConnecting.value = false
}

watch(useAdminCapabilities().realtime, (enabled) => {
  if (!enabled) disconnect()
})

export function useAdminWebSocket() {
  const instance = getCurrentInstance()
  const localDisposers: Array<() => void> = []

  const subscribe = <E extends WebSocketEventName>(eventName: E, handler: MessageHandler<E>) => {
    const dispose = on(eventName, handler)
    if (instance) {
      localDisposers.push(dispose)
    }
    return dispose
  }

  if (instance) {
    onUnmounted(() => {
      localDisposers.forEach((dispose) => dispose())
    })
  }

  return {
    connect,
    ensureConnected: connect,
    disconnect,
    on: subscribe,
    off,
    isConnected,
  }
}

export type AdminWebSocketOn = <E extends WebSocketEventName>(
  eventName: E,
  handler: MessageHandler<E>,
) => () => void

export interface AdminWebSocketBus {
  on: AdminWebSocketOn
}
