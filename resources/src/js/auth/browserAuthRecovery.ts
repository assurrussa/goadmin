// The host explicitly opts in with the versioned recovery response marker.
// No response supplies a URL, and no rejected command is retained or replayed.
export const authRecoveryPath = '/auth/session/recover'

type RecoveryFetch = (input: string, init: RequestInit) => Promise<Response>

export async function recoverBrowserAuthentication(
  signal: AbortSignal,
  request: RecoveryFetch = (input, init) => fetch(input, init),
): Promise<boolean> {
  const controller = new AbortController()
  const abort = () => controller.abort()
  if (signal.aborted) {
    return false
  }
  signal.addEventListener('abort', abort, { once: true })
  const timeout = setTimeout(abort, 30_000)
  try {
    // Native admission may rotate the cookie. Only the next request can verify
    // that cookie and mint/check its matching CSRF proof. Never follow redirects.
    for (let step = 0; step < 2; step++) {
      const response = await request(authRecoveryPath, {
        method: 'GET',
        credentials: 'same-origin',
        mode: 'same-origin',
        cache: 'no-store',
        redirect: 'error',
        headers: {
          Accept: 'application/json',
          'X-Goadmin-Auth-Recovery': '1',
        },
        signal: controller.signal,
      })
      if (
        controller.signal.aborted ||
        response.redirected ||
        response.headers.get('x-goadmin-auth-recovery') !== '1' ||
        response.headers.get('content-type')?.split(';')[0]?.trim().toLowerCase() !==
          'application/json'
      ) {
        return false
      }
      const body: unknown = await response.json()
      if (
        controller.signal.aborted ||
        typeof body !== 'object' ||
        body === null ||
        Array.isArray(body)
      ) {
        return false
      }
      const result = body as Record<string, unknown>
      if (result.status !== response.status) {
        return false
      }
      if (
        response.status === 200 &&
        result.code === 'authentication_recovered' &&
        result.csrfReady === true
      ) {
        return true
      }
      if (response.status !== 409 || result.code !== 'authentication_recovery_continue') {
        return false
      }
    }
    return false
  } catch {
    return false
  } finally {
    clearTimeout(timeout)
    signal.removeEventListener('abort', abort)
  }
}
