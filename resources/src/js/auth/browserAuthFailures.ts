// Only the admin-owned, versioned HTTP error contract is handled here.
// Unrelated non-Inertia responses must keep Inertia's normal diagnostics.
export interface AuthFailureEvent {
  readonly defaultPrevented: boolean
  readonly detail: { response: unknown }
  preventDefault(): void
}

export interface AuthFailureEffects {
  origin: string
  clearAuth(): void
  disconnect(): void
  // The application supplies a fixed same-origin GET navigation, never a replay.
  navigateToLogin(): void
  showNotice(message: string): void
}

type AuthFailureCode =
  | 'reauthentication_required'
  | 'authentication_retry_required'
  | 'authentication_unavailable'
  | 'invalid_browser_credentials'

type AuthFailure = { code: AuthFailureCode; retryAfter: number | null }

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function header(headers: unknown, name: string): string | undefined {
  if (!isRecord(headers)) {
    return undefined
  }
  // Both AxiosHeaders and plain Axios response header objects are enumerable.
  const entry = Object.entries(headers).find(([key]) => key.toLowerCase() === name)
  return typeof entry?.[1] === 'string' ? entry[1] : undefined
}

function parseFailure(response: unknown, origin: string): AuthFailure | null {
  if (!isRecord(response) || !isRecord(response.data) || !isRecord(response.config)) {
    return null
  }
  if (typeof response.config.url !== 'string') {
    return null
  }
  try {
    const target = new URL(response.config.url, origin)
    if (target.origin !== origin || target.username !== '' || target.password !== '') {
      return null
    }
  } catch {
    return null
  }
  if (
    header(response.headers, 'x-goadmin-auth-error') !== '1' ||
    header(response.headers, 'content-type')?.split(';')[0]?.trim().toLowerCase() !==
      'application/json' ||
    header(response.headers, 'x-inertia') !== undefined
  ) {
    return null
  }
  const body = response.data
  if (
    body.status !== response.status ||
    typeof body.message !== 'string' ||
    typeof body.code !== 'string'
  ) {
    return null
  }
  let code: AuthFailureCode
  if (response.status === 401 && body.code === 'reauthentication_required') {
    code = body.code
  } else if (
    response.status === 503 &&
    (body.code === 'authentication_retry_required' || body.code === 'authentication_unavailable')
  ) {
    code = body.code
  } else if (response.status === 400 && body.code === 'invalid_browser_credentials') {
    code = body.code
  } else {
    return null
  }
  const rawDelay = header(response.headers, 'retry-after')
  const delay = rawDelay && /^\d{1,2}$/.test(rawDelay) ? Number(rawDelay) : 0
  return { code, retryAfter: delay >= 1 && delay <= 60 ? delay : null }
}

export function createAuthFailureHandler(effects: AuthFailureEffects) {
  let leavingForLogin = false
  return (event: AuthFailureEvent): void => {
    if (event.defaultPrevented) {
      return
    }
    const failure = parseFailure(event.detail.response, effects.origin)
    if (failure === null) {
      return
    }
    event.preventDefault()
    if (leavingForLogin) {
      return
    }
    if (failure.code === 'reauthentication_required') {
      leavingForLogin = true
      effects.clearAuth()
      effects.disconnect()
      effects.navigateToLogin()
      return
    }
    if (failure.code === 'invalid_browser_credentials') {
      effects.showNotice('Не удалось проверить данные входа. Обновите страницу и войдите снова.')
      return
    }
    const delay =
      failure.retryAfter === null ? 'через несколько секунд.' : `через ${failure.retryAfter} с.`
    // No router/form/retry callback is retained: a mutation can only be resubmitted
    // by the user. Updating this notice must not replace the current page or form.
    effects.showNotice(
      `Сервис авторизации временно недоступен. Данные формы сохранены на странице. Повторите отправку вручную ${delay}`,
    )
  }
}
