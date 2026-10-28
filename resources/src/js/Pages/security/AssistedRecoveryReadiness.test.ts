import { afterEach, expect, it, vi } from 'vitest'
import { createApp, nextTick } from 'vue'
import { configureSessionMaintenance, sessionCoordinator } from '@/http'
import AssistedRecoveryPage from './AssistedRecoveryPage.vue'

vi.mock('@inertiajs/vue3', () => ({ router: { get: vi.fn() } }))
vi.mock('@/components/layout/AppHead.vue', () => ({ default: { render: () => null } }))

let app: ReturnType<typeof createApp> | undefined
afterEach(() => {
  app?.unmount()
  configureSessionMaintenance({ props: {} }, {})
  sessionCoordinator.dispose()
  vi.unstubAllGlobals()
  vi.useRealTimers()
})
const flush = async () => {
  for (let i = 0; i < 30; i++) await Promise.resolve()
  await nextTick()
}
function recovered() {
  const now = Date.now()
  return new Response(
    JSON.stringify({
      status: 200,
      code: 'authentication_recovered',
      csrfReady: true,
      maintenance: {
        version: 1,
        serverTime: new Date(now).toISOString(),
        nextCheckAfterMs: 60_000,
        csrfExpiresAt: new Date(now + 300_000).toISOString(),
        hardExpiresAt: new Date(now + 3_600_000).toISOString(),
        sessionEpoch: 'one',
        revision: 1,
        renewed: false,
      },
    }),
    { headers: { 'Content-Type': 'application/json', 'X-Goadmin-Auth-Recovery': '1' } },
  )
}
it.each(['503', 'timeout'])(
  'allows a manual retry after an unsent %s readiness failure',
  async (failure) => {
    vi.useFakeTimers()
    let ready = false
    const fetch = vi.fn<typeof globalThis.fetch>((input) => {
      if (String(input) === '/auth/session/maintain') {
        if (ready) return Promise.resolve(recovered())
        if (failure === 'timeout') return new Promise(() => {})
        return Promise.resolve(
          new Response(JSON.stringify({ status: 503, code: 'authentication_unavailable' }), {
            status: 503,
            headers: { 'Content-Type': 'application/json', 'X-Goadmin-Auth-Recovery': '1' },
          }),
        )
      }
      return Promise.resolve(new Response('', { status: 200 }))
    })
    vi.stubGlobal('fetch', fetch)
    configureSessionMaintenance({
      props: { adminSessionMaintenance: { version: 1, sessionEpoch: 'one' } },
    })
    const root = document.createElement('div')
    app = createApp(AssistedRecoveryPage, {
      title: '',
      nativeCSRF: 'synthetic',
      search: 'person@example.test',
      targets: [{ subject: 'one', name: 'Person', email: 'person@example.test' }],
    })
    app.mount(root)
    const form = root.querySelectorAll('form')[1]!
    const password = form.querySelector<HTMLInputElement>('[name=password]')!
    const code = form.querySelector<HTMLInputElement>('[name=code]')!
    password.value = 'secret'
    password.dispatchEvent(new Event('input'))
    code.value = '123456'
    code.dispatchEvent(new Event('input'))
    form.dispatchEvent(new Event('submit', { cancelable: true }))
    if (failure === 'timeout') await vi.advanceTimersByTimeAsync(10_000)
    await flush()
    expect(fetch.mock.calls.filter(([url]) => String(url) === '/account-recovery')).toHaveLength(0)
    expect(form.querySelector('fieldset')?.disabled).toBe(false)
    expect(password.value).toBe('')
    expect(code.value).toBe('')
    expect(root.textContent).toContain('Приглашение не отправлено')
    ready = true
    await sessionCoordinator.ensure({ force: true })
    form.dispatchEvent(new Event('submit', { cancelable: true }))
    await flush()
    expect(fetch.mock.calls.filter(([url]) => String(url) === '/account-recovery')).toHaveLength(1)
    expect(form.querySelector('fieldset')?.disabled).toBe(true)
  },
)

it('keeps an uncertain dispatch locked even if transport throws a readiness error', async () => {
  const { SessionReadinessError } = await import('@/http')
  const fetch = vi
    .fn<typeof globalThis.fetch>()
    .mockRejectedValue(new SessionReadinessError('authentication_unavailable'))
  vi.stubGlobal('fetch', fetch)
  const root = document.createElement('div')
  app = createApp(AssistedRecoveryPage, {
    title: '',
    nativeCSRF: 'synthetic',
    search: 'person@example.test',
    targets: [{ subject: 'one', name: 'Person', email: 'person@example.test' }],
  })
  app.mount(root)
  const form = root.querySelectorAll('form')[1]!
  form.dispatchEvent(new Event('submit', { cancelable: true }))
  await flush()
  expect(root.textContent).toContain('Результат неизвестен')
  expect(form.querySelector('fieldset')?.disabled).toBe(true)
  form.dispatchEvent(new Event('submit', { cancelable: true }))
  expect(fetch).toHaveBeenCalledTimes(1)
})
