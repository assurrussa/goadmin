export const localReauthenticationDestination = '/auth/login?local=1&redirectback=%2Fsso-link'

// A redirect followed by fetch must not be mistaken for successful logout.
export async function requestLocalReauthentication(csrf: string): Promise<boolean> {
  const response = await fetch('/sso-link/reauth', {
    method: 'POST',
    credentials: 'same-origin',
    redirect: 'manual',
    headers: {
      'Content-Type': 'application/x-www-form-urlencoded',
      'X-CSRF-Token': csrf,
      Accept: 'application/json',
    },
    body: new URLSearchParams({ csrf, confirm: 'reauthenticate-local' }),
  })
  if (
    response.status !== 200 ||
    !response.headers.get('content-type')?.includes('application/json')
  ) {
    return false
  }
  const body: unknown = await response.json()
  return (
    typeof body === 'object' &&
    body !== null &&
    'next' in body &&
    body.next === localReauthenticationDestination
  )
}
