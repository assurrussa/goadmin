import type { AdminTranslations } from '@/i18n'
import type { App } from 'vue'
import type { AdminWebSocketHandler } from '@/websocket/types'

export type AdminExtension = {
  translations?: AdminTranslations
  install?: (app: App) => void
  webSocketHandlers?: AdminWebSocketHandler[]
}
