import { computed, isRef, onBeforeUnmount, onMounted, watch, type ComputedRef, type Ref } from 'vue'
import {
  useAdminWebSocket,
  type MessageHandler,
  type WebSocketEventName,
} from '@/composables/useAdminWebSocket'

type MaybeBooleanRef = boolean | Ref<boolean> | ComputedRef<boolean>

const resolveBooleanRef = (value: MaybeBooleanRef): ComputedRef<boolean> => {
  if (typeof value === 'boolean') {
    return computed(() => value)
  }

  if (isRef(value)) {
    return computed(() => value.value)
  }

  return computed(() => true)
}

export interface UseAdminWebSocketEventOptions {
  /**
   * Управляет подпиской на событие. Можно передать true/false или ссылку (ref).
   * Пока значение false — обработчик отключен.
   * @defaultValue true
   */
  enabled?: MaybeBooleanRef
  /**
   * Если true — при монтировании гарантируем, что сокет подключен.
   * @defaultValue true
   */
  immediate?: boolean
}

export function useAdminWebSocketEvent<E extends WebSocketEventName>(
  eventName: E,
  handler: MessageHandler<E>,
  options: UseAdminWebSocketEventOptions = {},
) {
  const { on, ensureConnected, isConnected } = useAdminWebSocket()

  const enabled = resolveBooleanRef(options.enabled ?? true)
  const shouldAutoConnect = options.immediate ?? true

  let dispose: (() => void) | null = null

  const unsubscribe = () => {
    if (dispose) {
      dispose()
      dispose = null
    }
  }

  const subscribe = () => {
    if (!enabled.value) {
      return
    }

    unsubscribe()
    dispose = on(eventName, handler)
  }

  onMounted(() => {
    if (shouldAutoConnect && enabled.value && !isConnected.value) {
      ensureConnected()
    }
    subscribe()
  })

  watch(
    enabled,
    (next) => {
      if (next) {
        if (shouldAutoConnect && !isConnected.value) {
          ensureConnected()
        }
        subscribe()
      } else {
        unsubscribe()
      }
    },
    { immediate: false },
  )

  onBeforeUnmount(unsubscribe)

  return {
    subscribe,
    unsubscribe,
  }
}
