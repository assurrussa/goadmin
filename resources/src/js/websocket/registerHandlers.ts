import type { AdminWebSocketBus } from '@/composables/useAdminWebSocket'
import type { AdminWebSocketHandler } from './types'
import { defaultAdminWebSocketHandlers } from './handlers'

export interface RegisterWebsocketHandlersOptions {
  /**
   * Пользовательские обработчики, регистрируемые дополнительно.
   */
  handlers?: AdminWebSocketHandler[]
  /**
   * Управляет подключением дефолтных обработчиков.
   * @defaultValue true
   */
  includeDefaults?: boolean
}

export function registerWebsocketHandlers(
  bus: AdminWebSocketBus,
  options: RegisterWebsocketHandlersOptions = {},
) {
  const { handlers = [], includeDefaults = true } = options

  const handlersToRegister = includeDefaults
    ? [...defaultAdminWebSocketHandlers, ...handlers]
    : [...handlers]

  if (!handlersToRegister.length) {
    return () => {}
  }

  const disposers = handlersToRegister.map((handler) => handler({ bus }))

  return () => {
    disposers.forEach((dispose) => dispose())
  }
}
