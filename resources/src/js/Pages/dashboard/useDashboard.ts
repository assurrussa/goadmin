import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { adminFetch, sessionCoordinator } from '@/http'
import { useAuthUserStore } from '@/stores/authUser'
import type { AdminLocale } from '@/i18n'

export interface DashboardText {
  text: string
  ru?: string
  en?: string
}
export interface DashboardCard {
  key: string
  title: DashboardText
  links: { label: DashboardText; href: string }[]
  metrics: { key: string; label: DashboardText; value: string }[]
  status?: {
    text: DashboardText
    tone: 'neutral' | 'info' | 'success' | 'warning' | 'error'
  }
  state: 'loading' | 'ready' | 'unavailable'
  updatedAt?: string
}
export interface DashboardSnapshot {
  owner: string
  cards: DashboardCard[]
  updatedAt?: string
}
export interface DashboardDisplayCard extends DashboardCard {
  stale?: boolean
  hasSuccessfulData?: boolean
}
export const dashboardText = (value: DashboardText, locale: AdminLocale): string =>
  value[locale] ?? value.text

const preferenceKey = (owner: string) => `goadmin.dashboard.autoRefresh.${owner}`
const terminal = () =>
  ['reauthentication_required', 'access_denied', 'session_changed'].includes(
    sessionCoordinator.getState(),
  )

/** Page-owned snapshots must never survive an actor change or page disposal. */
export function useDashboard(initial?: DashboardSnapshot) {
  const auth = useAuthUserStore()
  // The server serializes the ID as a JSON number; the shared TS model currently says string.
  const owner = computed(() => (auth.authUser?.id == null ? '' : String(auth.authUser.id)))
  const cards = ref<DashboardDisplayCard[]>([])
  const updatedAt = ref<string>()
  const refreshing = ref(false)
  const autoRefreshEnabled = ref(false)
  const failed = ref(false)
  const stopped = ref(!owner.value)
  let mounted = false
  let generation = 0
  let controller: AbortController | undefined
  let flight: Promise<void> | undefined
  let timer: ReturnType<typeof setInterval> | undefined
  // Only an explicit control change persists intent, using the owner at that moment.
  // Hydration and lifecycle resets update the backing ref without writing a preference.
  const autoRefresh = computed({
    get: () => autoRefreshEnabled.value,
    set: (enabled: boolean) => {
      autoRefreshEnabled.value = enabled
      if (mounted && owner.value && !stopped.value) {
        try {
          localStorage.setItem(preferenceKey(owner.value), String(enabled))
        } catch {
          // Private browsing and blocked storage must not break refresh controls.
        }
      }
    },
  })

  function stopTimer() {
    clearInterval(timer)
    timer = undefined
  }
  function clear() {
    generation++
    controller?.abort()
    controller = undefined
    flight = undefined
    refreshing.value = false
    cards.value = []
    updatedAt.value = undefined
    failed.value = false
    autoRefreshEnabled.value = false
    stopTimer()
  }
  function stop() {
    clear()
    stopped.value = true
  }
  function schedule() {
    stopTimer()
    if (mounted && owner.value && !stopped.value && autoRefresh.value && !document.hidden) {
      timer = setInterval(() => void refresh(), 60_000)
    }
  }
  function apply(snapshot: DashboardSnapshot) {
    const previous = new Map(cards.value.map((card) => [card.key, card]))
    // This is a complete authorization-filtered membership, not a patch.
    cards.value = snapshot.cards.map((card) => {
      const old = previous.get(card.key)
      if (card.state === 'unavailable' && old?.hasSuccessfulData) {
        return {
          ...card,
          metrics: old.metrics,
          status: old.status,
          updatedAt: old.updatedAt,
          stale: true,
          hasSuccessfulData: true,
        }
      }
      return {
        ...card,
        stale: card.state === 'unavailable',
        hasSuccessfulData: card.state === 'ready',
      }
    })
    updatedAt.value = snapshot.updatedAt
    failed.value = false
  }
  function markUnavailable() {
    failed.value = true
    cards.value = cards.value.map((card) => ({ ...card, state: 'unavailable', stale: true }))
  }
  function refresh(): Promise<void> {
    if (!mounted || !owner.value || stopped.value) return Promise.resolve()
    if (terminal()) {
      stop()
      return Promise.resolve()
    }
    if (flight) return flight
    const actor = owner.value
    const requestGeneration = generation
    const abort = new AbortController()
    controller = abort
    refreshing.value = true
    const current = () =>
      mounted && !abort.signal.aborted && generation === requestGeneration && owner.value === actor
    const request = (async () => {
      try {
        const response = await Promise.resolve().then(() =>
          adminFetch('/dashboard/data', { signal: abort.signal }),
        )
        if (!current()) return
        if (response.status === 401 || response.status === 403 || terminal()) {
          stop()
          return
        }
        if (!response.ok) throw new Error('Dashboard unavailable')
        const snapshot: DashboardSnapshot = await response.json()
        if (!current()) return
        if (terminal()) {
          stop()
          return
        }
        if (snapshot.owner !== actor) {
          // Even a successful old-actor response cannot authorize these values.
          stop()
          return
        }
        if (!Array.isArray(snapshot.cards)) throw new Error('Invalid dashboard snapshot')
        apply(snapshot)
      } catch {
        if (current()) {
          if (terminal()) stop()
          else markUnavailable()
        }
      } finally {
        if (current()) {
          controller = undefined
          flight = undefined
          refreshing.value = false
        }
      }
    })()
    flight = request
    return request
  }
  function visibilityChanged() {
    schedule()
    if (!document.hidden && autoRefresh.value) void refresh()
  }
  watch(
    owner,
    () => {
      clear()
      // A new actor starts clean; no old snapshot or running preference is restored.
      stopped.value = !owner.value
      if (mounted && owner.value) void refresh()
    },
    { flush: 'sync' },
  )
  watch(autoRefresh, schedule)
  if (initial?.owner === owner.value && owner.value) apply(initial)
  onMounted(() => {
    mounted = true
    if (owner.value && !terminal()) {
      try {
        autoRefreshEnabled.value = localStorage.getItem(preferenceKey(owner.value)) === 'true'
      } catch {
        // Auto-refresh defaults off when storage is unavailable.
      }
    }
    document.addEventListener('visibilitychange', visibilityChanged)
    schedule()
    void refresh()
  })
  onBeforeUnmount(() => {
    mounted = false
    clear()
    document.removeEventListener('visibilitychange', visibilityChanged)
  })
  return { cards, updatedAt, refreshing, autoRefresh, failed, stopped, refresh }
}
