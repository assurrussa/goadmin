import type { AdminWebSocketBus } from '@/composables/useAdminWebSocket'

export interface AdminWebSocketHandlerContext {
  bus: AdminWebSocketBus
}

export type AdminWebSocketHandler = (context: AdminWebSocketHandlerContext) => () => void
