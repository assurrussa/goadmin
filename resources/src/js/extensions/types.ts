import type { App } from 'vue'
import type { AdminWebSocketHandler } from '@/websocket/types'

export type AdminExtension = {
  install?: (app: App) => void
  webSocketHandlers?: AdminWebSocketHandler[]
}
