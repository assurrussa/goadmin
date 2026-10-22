import type { AdminTranslations } from '@/i18n'
import type { App } from 'vue'
import type { AdminWebSocketHandler } from '@/websocket/types'
import type { AdminThemePack } from '@/presentation/types'

export type AdminExtension = {
  theme?: AdminThemePack
  translations?: AdminTranslations
  install?: (app: App) => void
  webSocketHandlers?: AdminWebSocketHandler[]
}
