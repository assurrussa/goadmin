import type { App, Plugin } from 'vue'
import { useAdminWebSocket } from '@/composables/useAdminWebSocket'
import { hasAdminCapability } from '@/composables/useAdminCapabilities'
import { registerWebsocketHandlers } from '@/websocket/registerHandlers'
import type { AdminWebSocketHandler } from '@/websocket/types'

export interface AdminWebSocketPluginOptions {
  /**
   * Дополнительные обработчики событий, подключаемые поверх дефолтных.
   */
  handlers?: AdminWebSocketHandler[]
  /**
   * Если true — сокет подключится сразу при установке плагина.
   * Используйте, если доступ к сокету не зависит от авторизации.
   */
  autoConnect?: boolean
}

const INSTALL_FLAG = '__adminWebSocketPluginInstalled__'
const HANDLER_REGISTRY_FLAG = '__adminWebSocketRegisteredHandlers__'
const DEFAULTS_FLAG = '__adminWebSocketDefaultsRegistered__'

export function createAdminWebSocketPlugin(options: AdminWebSocketPluginOptions = {}): Plugin {
  return {
    install(app: App) {
      if (typeof window === 'undefined' || !hasAdminCapability('realtime')) {
        return
      }

      const { handlers = [], autoConnect = false } = options
      const globalFlags = app.config.globalProperties as Record<string, unknown>
      const alreadyInstalled = Boolean(globalFlags[INSTALL_FLAG])
      if (!alreadyInstalled) {
        globalFlags[INSTALL_FLAG] = true
      }

      let handlerRegistry = globalFlags[HANDLER_REGISTRY_FLAG] as
        | WeakSet<AdminWebSocketHandler>
        | undefined
      if (!handlerRegistry) {
        handlerRegistry = new WeakSet<AdminWebSocketHandler>()
        globalFlags[HANDLER_REGISTRY_FLAG] = handlerRegistry
      }

      const registerHandlersOnce = (items: AdminWebSocketHandler[]) => {
        return items.filter((handler) => {
          if (handlerRegistry?.has(handler)) {
            return false
          }
          handlerRegistry?.add(handler)
          return true
        })
      }

      const websocket = useAdminWebSocket()
      const uniqueHandlers = registerHandlersOnce(handlers)
      const defaultsRegistered = Boolean(globalFlags[DEFAULTS_FLAG])
      const shouldIncludeDefaults = !defaultsRegistered && !alreadyInstalled

      if (shouldIncludeDefaults || uniqueHandlers.length) {
        registerWebsocketHandlers(websocket, {
          handlers: uniqueHandlers,
          includeDefaults: shouldIncludeDefaults,
        })
        if (shouldIncludeDefaults) {
          globalFlags[DEFAULTS_FLAG] = true
        }
      }

      if (autoConnect) {
        websocket.ensureConnected()
      }
    },
  }
}
