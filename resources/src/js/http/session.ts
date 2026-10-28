/** Public authority metadata contains deadlines and fences, never credentials. */
export interface SessionMaintenance {
  version: 1
  serverTime: string
  nextCheckAfterMs: number
  externalValidUntil?: string
  csrfExpiresAt: string
  hardExpiresAt: string
  sessionEpoch: string
  revision: number
  renewed: boolean
}

export type SessionFailureCode =
  | 'authentication_unavailable'
  | 'reauthentication_required'
  | 'access_denied'
  | 'session_changed'
  | 'cancelled'

export class SessionReadinessError extends Error {
  constructor(readonly code: SessionFailureCode) {
    super(
      code === 'authentication_unavailable'
        ? 'Сервис авторизации временно недоступен. Данные формы сохранены. Повторите попытку.'
        : code === 'session_changed'
          ? 'Сеанс изменился. Данные формы сохранены. Войдите снова перед отправкой.'
          : code === 'cancelled'
            ? 'Отправка отменена.'
            : 'Сеанс завершён или доступ закрыт. Данные формы сохранены. Войдите снова.',
    )
    this.name = 'SessionReadinessError'
  }
}

export type SessionState =
  | 'disabled'
  | 'ready'
  | 'checking'
  | 'csrf_needs_sync'
  | SessionFailureCode
export interface SessionReadinessOptions {
  signal?: AbortSignal
  force?: boolean
}
export interface SessionCoordinatorOptions {
  request?: typeof fetch
  now?: () => number
  onState?: (state: SessionState) => void
  onReady?: (maintenance: SessionMaintenance) => void
}

export const sessionMaintenancePath = '/auth/session/maintain'
const FOREGROUND_FRESH_MS = 30_000
const FOREGROUND_BUDGET_MS = 10_000
const isRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === 'object' && value !== null && !Array.isArray(value)
const date = (value: unknown): number =>
  typeof value === 'string' &&
  /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/.test(value)
    ? Date.parse(value)
    : NaN

export function parseSessionMaintenance(value: unknown): SessionMaintenance | null {
  if (!isRecord(value)) return null
  if (
    value.version !== 1 ||
    !Number.isFinite(date(value.serverTime)) ||
    !Number.isFinite(date(value.csrfExpiresAt)) ||
    !Number.isFinite(date(value.hardExpiresAt)) ||
    (value.externalValidUntil !== undefined && !Number.isFinite(date(value.externalValidUntil))) ||
    typeof value.nextCheckAfterMs !== 'number' ||
    !Number.isFinite(value.nextCheckAfterMs) ||
    value.nextCheckAfterMs <= 0 ||
    typeof value.sessionEpoch !== 'string' ||
    !value.sessionEpoch ||
    !Number.isSafeInteger(value.revision) ||
    (value.revision as number) < 0 ||
    typeof value.renewed !== 'boolean'
  )
    return null
  return value as unknown as SessionMaintenance
}

/** One coordinator per document; callers retain ownership of unsent commands. */
export function createSessionCoordinator(options: SessionCoordinatorOptions = {}) {
  const request = options.request ?? ((input, init) => fetch(input, init))
  const now = options.now ?? Date.now
  let enabled = false
  let state: SessionState = 'disabled'
  let metadata: SessionMaintenance | null = null
  let documentEpoch: string | undefined
  let checkedAt: number | null = null
  let observedAt: number | null = null
  let consecutiveFailures = 0
  let retryDeadline = 0
  let hardDeadline = Infinity
  let generation = 0
  let maintenanceGeneration = 0
  let commandController = new AbortController()
  let flight: Promise<void> | null = null
  let controller: AbortController | null = null
  let timer: ReturnType<typeof setTimeout> | undefined
  let disposeEvents: (() => void) | undefined
  const setState = (next: SessionState) => {
    state = next
    options.onState?.(next)
  }
  const terminal = () =>
    state === 'reauthentication_required' ||
    state === 'access_denied' ||
    state === 'session_changed'
  const clearTimer = () => {
    clearTimeout(timer)
    timer = undefined
  }
  const fail = (code: SessionFailureCode) => {
    setState(code)
    if (terminal()) clearTimer()
    throw new SessionReadinessError(code)
  }
  const checkHardExpiry = () => {
    if (now() >= hardDeadline) fail('reauthentication_required')
  }
  const proofDeadlines = () => {
    if (!metadata || observedAt === null) return []
    return [metadata.csrfExpiresAt, metadata.externalValidUntil]
      .filter((value): value is string => Boolean(value))
      .map((value) => observedAt! + date(value) - date(metadata!.serverTime) - 1000)
  }
  const readyDeadline = () =>
    metadata && observedAt !== null
      ? Math.min(observedAt + metadata.nextCheckAfterMs, ...proofDeadlines(), hardDeadline)
      : 0
  const schedule = (retry = false) => {
    clearTimer()
    if (!enabled || !documentEpoch || terminal()) return
    const cap = typeof document !== 'undefined' && document.hidden ? 120_000 : 60_000
    let deadline = retry ? retryDeadline : Math.min(readyDeadline(), (observedAt ?? now()) + cap)
    // An already expired proof cannot be used for commands. Preserve outage
    // backoff after expiry rather than hammering the authority once per second.
    for (const proof of proofDeadlines()) {
      if (!retry || proof > now()) deadline = Math.min(deadline, proof)
    }
    deadline = Math.min(deadline, hardDeadline)
    timer = setTimeout(
      () => {
        void ensure({ force: true }).catch(() => {})
      },
      Math.max(1, deadline - now()),
    )
  }
  const unavailable = (retryAfter?: unknown) => {
    consecutiveFailures++
    const backoff = [1000, 2000, 5000, 10000, 30000]
    const hinted =
      typeof retryAfter === 'string' && /^\d{1,9}$/.test(retryAfter)
        ? Math.max(1, Math.min(120, Number(retryAfter))) * 1000
        : undefined
    retryDeadline =
      now() + (hinted ?? backoff[Math.min(consecutiveFailures - 1, backoff.length - 1)]!)
    setState('authentication_unavailable')
    schedule(true)
  }
  const check = (): Promise<void> => {
    if (flight) return flight
    const currentGeneration = maintenanceGeneration
    const currentController = new AbortController()
    controller = currentController
    setState('checking')
    const operation = async () => {
      let retryAfter: unknown
      try {
        for (let step = 0; step < 2; step++) {
          const sentAt = now()
          const response = await request(sessionMaintenancePath, {
            method: 'GET',
            credentials: 'same-origin',
            mode: 'same-origin',
            cache: 'no-store',
            redirect: 'error',
            signal: currentController.signal,
            headers: { Accept: 'application/json', 'X-Goadmin-Auth-Recovery': '1' },
          })
          if (currentController.signal.aborted || maintenanceGeneration !== currentGeneration)
            throw new SessionReadinessError('cancelled')
          if (
            response.redirected ||
            response.headers.get('x-goadmin-auth-recovery') !== '1' ||
            response.headers.get('content-type')?.split(';')[0]?.trim().toLowerCase() !==
              'application/json'
          ) {
            throw new SessionReadinessError('authentication_unavailable')
          }
          if (response.status === 503) retryAfter = response.headers.get('retry-after')
          const body: unknown = await response.json()
          if (currentController.signal.aborted || maintenanceGeneration !== currentGeneration)
            throw new SessionReadinessError('cancelled')
          if (!isRecord(body) || body.status !== response.status)
            throw new SessionReadinessError('authentication_unavailable')
          if (
            response.status === 409 &&
            body.code === 'authentication_recovery_continue' &&
            step === 0
          )
            continue
          if (response.status === 401 && body.code === 'reauthentication_required')
            fail('reauthentication_required')
          if (response.status === 403 && body.code === 'access_denied') fail('access_denied')
          if (
            response.status !== 200 ||
            body.code !== 'authentication_recovered' ||
            body.csrfReady !== true
          ) {
            throw new SessionReadinessError('authentication_unavailable')
          }
          const next = parseSessionMaintenance(body.maintenance)
          if (!next) throw new SessionReadinessError('authentication_unavailable')
          if (metadata?.sessionEpoch === next.sessionEpoch && next.revision < metadata.revision) {
            throw new SessionReadinessError('authentication_unavailable')
          }
          const epochChanged = documentEpoch !== next.sessionEpoch
          metadata = next
          checkedAt = now()
          // Conservatively charge the complete round trip against server deadlines.
          observedAt = Math.min(sentAt, checkedAt)
          hardDeadline = observedAt + date(next.hardExpiresAt) - date(next.serverTime)
          checkHardExpiry()
          if (epochChanged) fail('session_changed')
          if (now() >= readyDeadline())
            throw new SessionReadinessError('authentication_unavailable')
          consecutiveFailures = 0
          retryDeadline = 0
          setState('ready')
          options.onReady?.(next)
          schedule()
          return
        }
      } catch (error) {
        if (maintenanceGeneration !== currentGeneration || currentController.signal.aborted)
          throw new SessionReadinessError('cancelled')
        if (error instanceof SessionReadinessError && error.code === 'session_changed') throw error
        if (!terminal()) unavailable(retryAfter)
        throw error instanceof SessionReadinessError
          ? error
          : new SessionReadinessError('authentication_unavailable')
      }
    }
    // Abort and reject even if a transport ignores its AbortSignal.
    let timeout: ReturnType<typeof setTimeout>
    const budget = new Promise<never>((_, reject) => {
      timeout = setTimeout(() => {
        if (maintenanceGeneration === currentGeneration) {
          maintenanceGeneration++
          reject(new SessionReadinessError('authentication_unavailable'))
          currentController.abort()
          unavailable()
        }
        reject(new SessionReadinessError('authentication_unavailable'))
      }, FOREGROUND_BUDGET_MS)
    })
    const cancelled = new Promise<never>((_, reject) => {
      currentController.signal.addEventListener(
        'abort',
        () => reject(new SessionReadinessError('cancelled')),
        { once: true },
      )
    })
    flight = Promise.race([operation(), budget, cancelled]).finally(() => {
      clearTimeout(timeout)
      if (controller === currentController) {
        controller = null
        flight = null
      }
    })
    return flight
  }
  const ensure = async ({ signal, force = false }: SessionReadinessOptions = {}): Promise<void> => {
    if (signal?.aborted) throw new SessionReadinessError('cancelled')
    if (!enabled) return
    if (terminal()) throw new SessionReadinessError(state as SessionFailureCode)
    if (!documentEpoch) throw new SessionReadinessError('authentication_unavailable')
    checkHardExpiry()
    if (!force && state === 'authentication_unavailable' && now() < retryDeadline) {
      throw new SessionReadinessError('authentication_unavailable')
    }
    if (
      !force &&
      state === 'ready' &&
      checkedAt !== null &&
      now() - checkedAt < FOREGROUND_FRESH_MS &&
      now() < readyDeadline()
    )
      return
    const commandSignal = commandController.signal
    const pending = check()
    await new Promise<void>((resolve, reject) => {
      const abort = () => reject(new SessionReadinessError('cancelled'))
      commandSignal.addEventListener('abort', abort, { once: true })
      signal?.addEventListener('abort', abort, { once: true })
      pending.then(resolve, reject).finally(() => {
        commandSignal.removeEventListener('abort', abort)
        signal?.removeEventListener('abort', abort)
      })
      if (signal?.aborted || commandSignal.aborted) abort()
    })
    signal?.throwIfAborted()
  }
  // Navigation/dismissal owns command continuations, not the watchdog lifecycle.
  const cancel = () => {
    generation++
    commandController.abort()
    commandController = new AbortController()
  }
  const stopMaintenance = () => {
    maintenanceGeneration++
    controller?.abort()
    controller = null
    flight = null
  }
  const configure = (page: unknown) => {
    const props = isRecord(page) && isRecord(page.props) ? page.props : null
    const supported =
      props &&
      isRecord(props.adminSessionMaintenance) &&
      props.adminSessionMaintenance.version === 1
    if (Boolean(supported) === enabled) return
    cancel()
    stopMaintenance()
    enabled = Boolean(supported)
    const anchor = supported ? (props?.adminSessionMaintenance as Record<string, unknown>) : null
    documentEpoch =
      typeof anchor?.sessionEpoch === 'string' && anchor.sessionEpoch.trim()
        ? anchor.sessionEpoch
        : undefined
    metadata = null
    checkedAt = null
    observedAt = null
    consecutiveFailures = 0
    retryDeadline = 0
    hardDeadline = Infinity
    clearTimer()
    setState(enabled ? (documentEpoch ? 'checking' : 'authentication_unavailable') : 'disabled')
    if (enabled && documentEpoch) void ensure({ force: true }).catch(() => {})
  }
  const start = () => {
    if (typeof window === 'undefined' || disposeEvents) return
    const wake = () => {
      if (enabled) void ensure().catch(() => {})
      schedule(state === 'authentication_unavailable')
    }
    const hide = () => cancel()
    for (const event of ['focus', 'pageshow', 'online']) window.addEventListener(event, wake)
    window.addEventListener('pagehide', hide)
    document.addEventListener('visibilitychange', wake)
    disposeEvents = () => {
      for (const event of ['focus', 'pageshow', 'online']) window.removeEventListener(event, wake)
      window.removeEventListener('pagehide', hide)
      document.removeEventListener('visibilitychange', wake)
    }
  }
  return {
    ensure,
    repair: () => {
      if (!enabled) return Promise.resolve()
      if (terminal()) return Promise.reject(new SessionReadinessError(state as SessionFailureCode))
      setState('csrf_needs_sync')
      return ensure({ force: true })
    },
    configure,
    start,
    cancel,
    isEnabled: () => enabled,
    getState: () => state,
    getMaintenance: () => metadata,
    commandFence: () => ({ generation, epoch: metadata?.sessionEpoch ?? documentEpoch }),
    assertCommandFence: (fence: { generation: number; epoch?: string }) => {
      if (generation !== fence.generation) throw new SessionReadinessError('cancelled')
      if (fence.epoch && (metadata?.sessionEpoch ?? documentEpoch) !== fence.epoch)
        throw new SessionReadinessError('session_changed')
      if (enabled) {
        if (terminal()) throw new SessionReadinessError(state as SessionFailureCode)
        checkHardExpiry()
        if (now() >= readyDeadline()) throw new SessionReadinessError('authentication_unavailable')
      }
    },
    reportFailure: (
      code: 'authentication_unavailable' | 'reauthentication_required' | 'access_denied',
      retryAfter?: unknown,
    ) => {
      if (!enabled || terminal()) return
      cancel()
      stopMaintenance()
      if (code === 'authentication_unavailable') unavailable(retryAfter)
      else {
        setState(code)
        clearTimer()
      }
    },
    dispose: () => {
      cancel()
      stopMaintenance()
      clearTimer()
      disposeEvents?.()
      disposeEvents = undefined
    },
  }
}
