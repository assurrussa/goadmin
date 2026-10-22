import { t } from '@/i18n'
import { watch, nextTick, onUnmounted } from 'vue'
import { usePage, router } from '@inertiajs/vue3'
import { useNotifications } from './useNotifications'

/** One application-frame owner; subscriptions and delayed work end with that owner. */
export function useFlashMessages() {
  const page = usePage()
  const { success, error, info, warning } = useNotifications()
  let active = true
  let lastProcessedFlash = ''
  let resetTimer: ReturnType<typeof setTimeout> | undefined

  const processFlashMessages = async (flash: Record<string, string> | undefined) => {
    if (!active || !flash || Object.keys(flash).length === 0) return
    await nextTick()
    if (!active) return
    const flashHash = JSON.stringify(flash)
    if (lastProcessedFlash !== flashHash) {
      lastProcessedFlash = flashHash
      if (flash.success) success(t(flash.success))
      if (flash.error) error(t(flash.error))
      if (flash.info) info(t(flash.info))
      if (flash.warning) warning(t(flash.warning))
      clearTimeout(resetTimer)
      resetTimer = setTimeout(() => {
        lastProcessedFlash = ''
      }, 1000)
    }
  }
  const stopWatch = watch(
    () => page.props?.flash,
    (flash) => {
      void processFlashMessages(flash as Record<string, string> | undefined)
    },
    { immediate: true, deep: true },
  )
  const stopRouter = router.on('finish', () => {
    void processFlashMessages(page.props?.flash as Record<string, string> | undefined)
  })
  onUnmounted(() => {
    active = false
    stopWatch()
    stopRouter()
    clearTimeout(resetTimer)
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

    flashHandlers[type]?.(t(flashMessage))
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
