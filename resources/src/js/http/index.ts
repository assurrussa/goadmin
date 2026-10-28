import axios, {
  type AxiosInstance,
  type AxiosResponse,
  type InternalAxiosRequestConfig,
} from 'axios'
import {
  createSessionCoordinator,
  SessionReadinessError,
  type SessionCoordinatorOptions,
  type SessionReadinessOptions,
} from './session'

export {
  createSessionCoordinator,
  parseSessionMaintenance,
  SessionReadinessError,
  sessionMaintenancePath,
} from './session'
export type {
  SessionMaintenance,
  SessionState,
  SessionFailureCode,
  SessionReadinessOptions,
  SessionCoordinatorOptions,
} from './session'

let effects: SessionCoordinatorOptions = {}
export const sessionCoordinator = createSessionCoordinator({
  onState: (state) => effects.onState?.(state),
  onReady: (metadata) => effects.onReady?.(metadata),
})

/** Bootstrap once with the server's versioned page capability. Absent means legacy behavior. */
export function configureSessionMaintenance(page: unknown, callbacks?: SessionCoordinatorOptions) {
  if (callbacks) effects = callbacks
  sessionCoordinator.start()
  sessionCoordinator.configure(page)
}

export const ensureSessionReady = (options?: SessionReadinessOptions) =>
  sessionCoordinator.ensure(options)
export const cancelPendingSessionCommands = () => sessionCoordinator.cancel()

type ResponseFence = ReturnType<typeof sessionCoordinator.commandFence>
function currentResponse(fence: ResponseFence | undefined): boolean {
  if (!fence) return false
  const current = sessionCoordinator.commandFence()
  return fence.generation === current.generation && fence.epoch === current.epoch
}

function sameOrigin(value: string, base?: string): boolean {
  if (typeof window === 'undefined') return false
  try {
    const target = new URL(value, base ? new URL(base, window.location.href) : window.location.href)
    return target.origin === window.location.origin && !target.username && !target.password
  } catch {
    return false
  }
}
function transition(value: string): boolean {
  try {
    const path = new URL(value, window.location.href).pathname
    return (
      /^\/auth\/(?:login|logout|register)(?:\/|$)/.test(path) ||
      /^\/auth\/session\/(?:maintain|recover)$/.test(path) ||
      /^\/sso(?:\/(?:open|login|logout|callback))?\/?$/.test(path) ||
      ['/login', '/logout', '/callback', '/sso-link/reauth'].includes(path)
    )
  } catch {
    return false
  }
}
function logout(value: string): boolean {
  try {
    return ['/auth/logout', '/logout', '/sso/logout', '/sso-link/reauth'].includes(
      new URL(value, window.location.href).pathname,
    )
  } catch {
    return false
  }
}
/** Read immediately before constructing a native CSRF proof body; never cache it. */
export function getCurrentCSRFToken(): string | undefined {
  const name = import.meta.env.VITE_ADMIN_CSRF_COOKIE_NAME?.trim() || 'csrf_token'
  const cookie = document.cookie
    .split(';')
    .map((part) => part.trim())
    .find((part) => part.startsWith(name + '='))
  if (!cookie) return undefined
  try {
    return decodeURIComponent(cookie.slice(name.length + 1))
  } catch {
    return undefined
  }
}
const unsafe = (method = 'GET') => !['GET', 'HEAD', 'OPTIONS'].includes(method.toUpperCase())

export function reportSessionAuthFailure(
  status: number,
  headers: { get(name: string): unknown },
  data: unknown,
  applyEffects = true,
): boolean {
  if (
    !sessionCoordinator.isEnabled() ||
    headers.get('x-goadmin-auth-error') !== '1' ||
    String(headers.get('content-type')).split(';')[0]?.trim().toLowerCase() !== 'application/json'
  )
    return false
  if (typeof data === 'string') {
    try {
      data = JSON.parse(data)
    } catch {
      return false
    }
  }
  if (
    !data ||
    typeof data !== 'object' ||
    !('status' in data) ||
    data.status !== status ||
    !('code' in data)
  )
    return false
  if (status === 419 && data.code === 'csrf_needs_sync') {
    if (applyEffects) void sessionCoordinator.repair().catch(() => {})
  } else if (status === 401 && data.code === 'reauthentication_required') {
    if (applyEffects) sessionCoordinator.reportFailure('reauthentication_required')
  } else if (status === 403 && data.code === 'access_denied') {
    if (applyEffects) sessionCoordinator.reportFailure('access_denied')
  } else if (
    status === 503 &&
    ['authentication_unavailable', 'authentication_retry_required'].includes(String(data.code))
  ) {
    if (applyEffects)
      sessionCoordinator.reportFailure('authentication_unavailable', headers.get('retry-after'))
  } else return false
  return true
}

export interface AdminFetchOptions extends RequestInit {
  /** Caller asserts that GET/HEAD is safe to issue once more after a transient auth failure. */
  retrySafeRead?: boolean
}

/** Gates the original send. Mutations are never retained or replayed. */
export function adminFetch(
  input: string | URL,
  options: AdminFetchOptions = {},
): Promise<Response> {
  const { retrySafeRead = false, ...init } = options
  const target = String(input)
  if (!sessionCoordinator.isEnabled() || !sameOrigin(target)) return fetch(input, init)
  return sendAdminFetch(input, init, retrySafeRead)
}

async function sendAdminFetch(
  input: string | URL,
  init: RequestInit,
  retrySafeRead: boolean,
): Promise<Response> {
  const target = String(input)
  if (logout(target)) sessionCoordinator.reportFailure('reauthentication_required')
  const gate = !transition(target)
  const send = async () => {
    const fence = sessionCoordinator.commandFence()
    if (gate) {
      await ensureSessionReady({ signal: init.signal ?? undefined })
      sessionCoordinator.assertCommandFence(fence)
    }
    init.signal?.throwIfAborted()
    const headers = new Headers(init.headers)
    headers.set('X-Goadmin-Session-Maintenance', '1')
    if (unsafe(init.method)) {
      const csrf = getCurrentCSRFToken()
      if (csrf) headers.set('X-CSRF-Token', csrf)
    }
    const responseFence = sessionCoordinator.commandFence()
    try {
      const response = await fetch(input, {
        ...init,
        credentials: 'same-origin',
        mode: 'same-origin',
        headers,
      })
      init.signal?.throwIfAborted()
      return { response, fence: responseFence }
    } catch (error) {
      if (!init.signal?.aborted && currentResponse(responseFence))
        sessionCoordinator.reportFailure('authentication_unavailable')
      throw error
    }
  }
  let { response, fence } = await send()
  if (response.headers.get('x-goadmin-auth-error') === '1') {
    const body: unknown = await response
      .clone()
      .json()
      .catch(() => null)
    const handled =
      currentResponse(fence) && reportSessionAuthFailure(response.status, response.headers, body)
    if (
      handled &&
      response.status === 503 &&
      retrySafeRead &&
      !unsafe(init.method) &&
      ['GET', 'HEAD'].includes((init.method ?? 'GET').toUpperCase())
    ) {
      await ensureSessionReady({ signal: init.signal ?? undefined, force: true })
      const retried = await send()
      response = retried.response
      fence = retried.fence
      if (response.headers.get('x-goadmin-auth-error') === '1') {
        reportSessionAuthFailure(
          response.status,
          response.headers,
          await response
            .clone()
            .json()
            .catch(() => null),
          currentResponse(fence),
        )
      }
    }
  }
  return response
}

function observeAxios(response: AxiosResponse, fence: ResponseFence | undefined) {
  // Inertia applies a successful page before its success event. Fence negotiated
  // page responses here so stale props/history cannot replace the mounted draft.
  const inertiaPage =
    response.status >= 200 &&
    response.status < 300 &&
    response.config.headers.get('X-Inertia') === 'true' &&
    response.config.headers.get('X-Goadmin-Session-Maintenance') === '1' &&
    (typeof response.headers.get === 'function'
      ? response.headers.get('x-inertia')
      : response.headers['x-inertia']) === 'true'
  const terminal = ['reauthentication_required', 'access_denied', 'session_changed'].includes(
    sessionCoordinator.getState(),
  )
  if (
    inertiaPage &&
    (!currentResponse(fence) || (!transition(response.config.url ?? '') && terminal))
  ) {
    throw new SessionReadinessError(
      sessionCoordinator.getState() === 'session_changed' ? 'session_changed' : 'cancelled',
    )
  }
  if (
    currentResponse(fence) &&
    !response.config.signal?.aborted &&
    sameOrigin(response.config.url ?? '', response.config.baseURL)
  ) {
    reportSessionAuthFailure(
      response.status,
      {
        get: (name) =>
          typeof response.headers.get === 'function'
            ? response.headers.get(name)
            : response.headers[name],
      },
      response.data,
    )
  }
  return response
}

/** The shared Axios instance also transports Inertia visits in the pinned client. */
export function installSessionAxiosInterceptor(client: AxiosInstance = axios): () => void {
  const responseFences = new WeakMap<object, ResponseFence>()
  const requestID = client.interceptors.request.use(async (config: InternalAxiosRequestConfig) => {
    const target = config.url ?? ''
    if (!sessionCoordinator.isEnabled() || !sameOrigin(target, config.baseURL)) return config
    if (logout(target)) sessionCoordinator.reportFailure('reauthentication_required')
    if (!transition(target)) {
      const fence = sessionCoordinator.commandFence()
      await ensureSessionReady({ signal: config.signal as AbortSignal | undefined })
      sessionCoordinator.assertCommandFence(fence)
    }
    if (config.signal?.aborted) throw new SessionReadinessError('cancelled')
    config.cancelToken?.throwIfRequested()
    config.headers.set('X-Goadmin-Session-Maintenance', '1', true)
    if (unsafe(config.method)) {
      const csrf = getCurrentCSRFToken()
      if (csrf) config.headers.set('X-CSRF-Token', csrf)
    }
    responseFences.set(config, sessionCoordinator.commandFence())
    return config
  })
  const responseID = client.interceptors.response.use(
    (response) => observeAxios(response, responseFences.get(response.config)),
    (error: unknown) => {
      if (axios.isAxiosError(error) && error.response)
        observeAxios(error.response, responseFences.get(error.response.config))
      else if (
        axios.isAxiosError(error) &&
        !axios.isCancel(error) &&
        error.config &&
        !error.config.signal?.aborted &&
        sessionCoordinator.isEnabled() &&
        currentResponse(responseFences.get(error.config)) &&
        sameOrigin(error.config.url ?? '', error.config.baseURL)
      ) {
        sessionCoordinator.reportFailure('authentication_unavailable')
      }
      return Promise.reject(error)
    },
  )
  return () => {
    client.interceptors.request.eject(requestID)
    client.interceptors.response.eject(responseID)
  }
}

/** Create an isolated host client with the same pre-send contract. */
export function createAdminAxios(config?: Parameters<typeof axios.create>[0]): AxiosInstance {
  const client = axios.create(config)
  installSessionAxiosInterceptor(client)
  return client
}
