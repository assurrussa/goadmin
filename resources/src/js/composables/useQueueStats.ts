import {
  getCurrentInstance,
  onMounted,
  onUnmounted,
  ref,
  toValue,
  watch,
  type MaybeRefOrGetter,
} from 'vue'
import { adminFetch, sessionCoordinator } from '@/http'
import { useAuthUserStore } from '@/stores/authUser'

export interface QueueStats {
  jobs: number
  failed: number
}
const valid = (value: QueueStats) =>
  Number.isSafeInteger(value.jobs) &&
  value.jobs >= 0 &&
  Number.isSafeInteger(value.failed) &&
  value.failed >= 0

/** Counts are server snapshots, never arithmetic derived from mutation callbacks. */
export function useQueueStats(
  apiUrl: MaybeRefOrGetter<string>,
  initial: MaybeRefOrGetter<QueueStats>,
) {
  const counters = ref<QueueStats | null>({ ...toValue(initial) })
  const loading = ref(false)
  const error = ref('')
  const stale = ref(false)
  let sequence = 0
  let controller: AbortController | undefined
  let disposed = false
  let blocked = false
  let unsubscribe = () => {}
  const invalidate = () => {
    sequence++
    controller?.abort()
    counters.value = null
    loading.value = false
    error.value = ''
    stale.value = true
    blocked = true
  }
  const refreshStats = async () => {
    if (disposed || blocked) return
    const request = ++sequence
    controller?.abort()
    const active = new AbortController()
    controller = active
    const fence = sessionCoordinator.commandFence()
    const current = () => {
      const now = sessionCoordinator.commandFence()
      return (
        !disposed &&
        !blocked &&
        request === sequence &&
        !active.signal.aborted &&
        fence.generation === now.generation &&
        fence.epoch === now.epoch
      )
    }
    loading.value = true
    stale.value = true
    error.value = ''
    try {
      const response = await adminFetch(toValue(apiUrl), { signal: active.signal })
      if (!response.ok) throw new Error()
      const data = await response.json()
      if (!current()) return
      if (!data || !valid(data)) throw new Error()
      counters.value = { jobs: data.jobs, failed: data.failed }
      stale.value = false
    } catch {
      if (current())
        error.value = 'Не удалось обновить счётчики. Показаны ранее загруженные данные.'
    } finally {
      if (request === sequence && !disposed && !blocked && !active.signal.aborted) {
        loading.value = false
        if (!current())
          error.value = 'Не удалось обновить счётчики. Показаны ранее загруженные данные.'
      }
    }
  }
  watch(
    () => [toValue(initial).jobs, toValue(initial).failed],
    () => {
      if (blocked || disposed) return
      sequence++
      controller?.abort()
      counters.value = { ...toValue(initial) }
      stale.value = false
      loading.value = false
      error.value = ''
    },
  )
  watch(
    () => toValue(apiUrl),
    () => {
      sequence++
      controller?.abort()
      counters.value = null
      stale.value = true
      void refreshStats()
    },
    { flush: 'sync' },
  )
  const pinia = getCurrentInstance()?.appContext.config.globalProperties.$pinia
  if (pinia) {
    const auth = useAuthUserStore(pinia)
    watch(() => auth.id, invalidate, { flush: 'sync' })
  }
  const sessionState = (state: string) => {
    if (['session_changed', 'reauthentication_required', 'access_denied'].includes(state))
      invalidate()
  }
  onMounted(() => {
    unsubscribe = sessionCoordinator.subscribe(sessionState)
    sessionState(sessionCoordinator.getState())
  })
  onUnmounted(() => {
    disposed = true
    invalidate()
    unsubscribe()
  })
  return { counters, loading, error, stale, refreshStats }
}
