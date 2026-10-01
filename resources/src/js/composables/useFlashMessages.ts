import { watch, nextTick, onUnmounted } from 'vue'
import { usePage, router } from '@inertiajs/vue3'
import { useNotifications } from './useNotifications'

// Глобальное состояние для предотвращения дублирования
const globalFlashState = {
  lastProcessedFlash: '',
  processing: false,
  initialized: false,
  stop: null as null | (() => void),
  routerBound: false,
}

/**
 * Composable для автоматической обработки flash сообщений от Inertia
 * Автоматически отображает уведомления на основе flash prop
 */
export function useFlashMessages() {
  const page = usePage()
  const { success, error, info, warning } = useNotifications()

  // Функция для обработки flash сообщений
  const processFlashMessages = async (flash: Record<string, string> | undefined) => {
    if (globalFlashState.processing) return
    globalFlashState.processing = true

    await nextTick()

    if (!flash || Object.keys(flash).length === 0) {
      globalFlashState.processing = false
      return
    }

    // Создаем уникальный хеш для flash сообщений
    const flashHash = JSON.stringify(flash)

    // Проверяем, не обрабатывали ли мы уже такие же сообщения недавно
    if (globalFlashState.lastProcessedFlash === flashHash) {
      globalFlashState.processing = false
      return
    }

    globalFlashState.lastProcessedFlash = flashHash

    if (flash.success) {
      success(flash.success)
    }

    if (flash.error) {
      error(flash.error)
    }

    if (flash.info) {
      info(flash.info)
    }

    if (flash.warning) {
      warning(flash.warning)
    }

    setTimeout(() => {
      globalFlashState.lastProcessedFlash = ''
    }, 1000)

    globalFlashState.processing = false
  }

  if (!globalFlashState.initialized) {
    globalFlashState.initialized = true

    // Сохраняем стоп-функцию, чтобы корректно пересоздавать watcher при смене layout'ов
    globalFlashState.stop = watch(
      () => page.props.flash,
      async (newFlash) => {
        await processFlashMessages(newFlash as Record<string, string> | undefined)
      },
      { immediate: true, deep: true },
    )
  }

  // Дополнительно подписываемся на события навигации Inertia, чтобы гарантированно
  // обработать flash после любых AJAX-переходов (в т.ч. редиректов 303/409)
  if (!globalFlashState.routerBound) {
    router.on('finish', async () => {
      // Берем актуальный flash из props и обрабатываем
      await processFlashMessages(page.props.flash as Record<string, string> | undefined)
    })
    globalFlashState.routerBound = true
  }

  // Если компонент, вызвавший useFlashMessages, размонтируется (смена layout при Inertia-навигации),
  // снимем watcher, чтобы следующий layout мог корректно его создать заново
  onUnmounted(() => {
    if (globalFlashState.stop) {
      globalFlashState.stop()
      globalFlashState.stop = null
    }
    globalFlashState.initialized = false
  })
}

/**
 * Composable для ручного управления flash сообщениями
 * Позволяет вручную отображать уведомления из flash props
 */
export function useFlashMessagesManual() {
  const page = usePage()
  const { success, error, info, warning } = useNotifications()

  const flashHandlers = {
    success,
    error,
    info,
    warning,
  } as const

  const showFlash = (type: keyof typeof flashHandlers, message?: string) => {
    const flash = page.props.flash as Record<string, string> | undefined
    const flashMessage = message || flash?.[type as string]

    if (!flashMessage) return

    flashHandlers[type]?.(flashMessage)
  }

  const getFlashMessage = (type: string): string | undefined => {
    const flash = page.props.flash as Record<string, string> | undefined
    return flash?.[type]
  }

  const hasFlash = (type?: string): boolean => {
    const flash = page.props.flash as Record<string, string> | undefined
    if (!flash) return false

    if (type) {
      return !!flash[type]
    }

    return Object.keys(flash).length > 0
  }

  return {
    showFlash,
    getFlashMessage,
    hasFlash,
  }
}
