import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, defineComponent, h, nextTick } from 'vue'
import SSOLinkPage from './SSOLinkPage.vue'
import SSOErrorPage from './SSOErrorPage.vue'
import { configureSessionMaintenance, sessionCoordinator } from '@/http'
import SSOButton from '@/components/auth/SSOButton.vue'
import {
  localReauthenticationDestination,
  requestLocalReauthentication,
} from '@/auth/localReauthentication'
vi.mock('@inertiajs/vue3', () => ({
  usePage: () => ({ props: {}, url: '/auth/login?redirectback=%2Fsites%2F7' }),
}))
vi.mock('@/i18n', () => ({ t: (s: string) => s }))
vi.mock('@/components/layout/AuthLayout.vue', () => ({
  default: defineComponent({
    setup:
      (_, { slots }) =>
      () =>
        h('main', [slots.title?.(), slots.default?.()]),
  }),
}))
vi.mock('@/components/layout/AppHead.vue', () => ({
  default: defineComponent({ render: () => null }),
}))
vi.mock('@/components/form/FormButton.vue', () => ({
  default: defineComponent({
    setup:
      (_, { attrs, slots }) =>
      () =>
        h('button', attrs, slots.default?.()),
  }),
}))
const apps: ReturnType<typeof createApp>[] = []
afterEach(() => {
  apps.splice(0).forEach((a) => a.unmount())
  configureSessionMaintenance({ props: {} }, {})
  sessionCoordinator.dispose()
  document.cookie = 'csrf_token=; Max-Age=0; path=/'
  vi.unstubAllGlobals()
})
function mount(component: Parameters<typeof createApp>[0], props = {}) {
  const root = document.createElement('div')
  const app = createApp(component, props)
  apps.push(app)
  app.mount(root)
  return root
}
describe('shared SSO pages', () => {
  it('uses a full document link and carries the original location', () => {
    const root = mount(SSOButton)
    expect(root.querySelector('a')?.getAttribute('href')).toBe('/sso?redirectback=%2Fsites%2F7')
  })
  it('escapes the verified identity instead of inserting executable HTML', () => {
    const subject = '<script>alert(1)</script>'
    const root = mount(SSOLinkPage, {
      issuer: 'https://issuer.test',
      subject,
      intent: 'intent',
      csrf: 'synthetic',
    })
    expect(root.textContent).toContain(subject)
    expect(root.querySelector('script')).toBeNull()
    expect(root.innerHTML).toContain('&lt;script&gt;')
  })
  it('submits explicit intent with native CSRF and never repeats an uncertain mutation', async () => {
    const fetch = vi.fn().mockRejectedValue(new Error('network'))
    vi.stubGlobal('fetch', fetch)
    const root = mount(SSOLinkPage, {
      issuer: 'https://issuer.test',
      subject: 'subject',
      intent: 'intent',
      csrf: 'synthetic',
    })
    const form = root.querySelector('form')!
    form.dispatchEvent(new Event('submit', { cancelable: true }))
    await new Promise((resolve) => setTimeout(resolve, 0))
    await nextTick()
    expect(fetch).toHaveBeenCalledTimes(1)
    const [url, options] = fetch.mock.calls[0]
    expect(url).toBe('/sso-link')
    expect(options.headers['X-CSRF-Token']).toBe('synthetic')
    expect(options.body.get('confirm')).toBe('link-this-identity')
    expect(root.textContent).toContain('Результат запроса неизвестен')
    expect(root.querySelector('a[href="/sso-link"]')).not.toBeNull()
    form.dispatchEvent(new Event('submit', { cancelable: true }))
    expect(fetch).toHaveBeenCalledTimes(1)
  })
  it('waits once on double submit and builds the explicit native proof body from the refreshed cookie', async () => {
    let resolve!: (response: Response) => void
    const fetch = vi
      .fn<typeof globalThis.fetch>()
      .mockImplementationOnce(
        () =>
          new Promise((done) => {
            resolve = done
          }),
      )
      .mockRejectedValueOnce(new Error('response lost'))
    vi.stubGlobal('fetch', fetch)
    configureSessionMaintenance({
      props: { adminSessionMaintenance: { version: 1, sessionEpoch: 'one' } },
    })
    const root = mount(SSOLinkPage, {
      issuer: 'issuer',
      subject: 'subject',
      intent: 'explicit-intent',
      csrf: 'expired-page-proof',
    })
    const form = root.querySelector('form')!
    form.dispatchEvent(new Event('submit', { cancelable: true }))
    form.dispatchEvent(new Event('submit', { cancelable: true }))
    expect(fetch).toHaveBeenCalledTimes(1)
    document.cookie = 'csrf_token=fresh-native-proof; path=/'
    const now = Date.now()
    resolve(
      new Response(
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
      ),
    )
    await new Promise((done) => setTimeout(done, 0))
    await nextTick()
    expect(fetch).toHaveBeenCalledTimes(2)
    expect(fetch.mock.calls[1]?.[1]?.body).toBeInstanceOf(URLSearchParams)
    const body = fetch.mock.calls[1]?.[1]?.body as URLSearchParams
    expect(body.get('csrf')).toBe('fresh-native-proof')
    expect(body.get('intent')).toBe('explicit-intent')
    expect(new Headers(fetch.mock.calls[1]?.[1]?.headers).get('X-CSRF-Token')).toBe(
      'fresh-native-proof',
    )
    expect(root.textContent).toContain('Результат запроса неизвестен')
    form.dispatchEvent(new Event('submit', { cancelable: true }))
    expect(fetch).toHaveBeenCalledTimes(2)
  })

  it('constructs local reauthentication proof from the current cookie while retaining its transition bypass', async () => {
    const now = Date.now()
    const fetch = vi
      .fn<typeof globalThis.fetch>()
      .mockResolvedValueOnce(
        new Response(
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
        ),
      )
      .mockResolvedValueOnce(
        new Response('{}', { status: 403, headers: { 'Content-Type': 'application/json' } }),
      )
    vi.stubGlobal('fetch', fetch)
    configureSessionMaintenance({
      props: { adminSessionMaintenance: { version: 1, sessionEpoch: 'one' } },
    })
    document.cookie = 'csrf_token=refreshed-reauth-proof; path=/'
    expect(await requestLocalReauthentication('expired-page-proof')).toBe(false)
    const body = fetch.mock.calls[1]?.[1]?.body as URLSearchParams
    expect(body.get('csrf')).toBe('refreshed-reauth-proof')
    expect(body.get('confirm')).toBe('reauthenticate-local')
    expect(fetch.mock.calls[1]?.[0]).toBe('/sso-link/reauth')
    expect(fetch).toHaveBeenCalledTimes(2)
  })

  it('sends both stale reauthentication buttons through the common CSRF header contract', async () => {
    const fetch = vi.fn().mockResolvedValue({
      status: 403,
      headers: new Headers({ 'content-type': 'application/json' }),
      json: async () => ({ code: 'local_auth_stale' }),
    })
    vi.stubGlobal('fetch', fetch)
    const errorPage = mount(SSOErrorPage, {
      title: 'Local session expired',
      message: 'Sign in again',
      code: 'local_auth_stale',
      reauthCSRF: 'error-token',
    })
    const errorForm = errorPage.querySelector('form[action="/sso-link/reauth"]')!
    errorForm.dispatchEvent(new Event('submit', { cancelable: true }))
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(fetch).toHaveBeenCalledTimes(1)
    expect(fetch.mock.calls[0][0]).toBe('/sso-link/reauth')
    expect(fetch.mock.calls[0][1].credentials).toBe('same-origin')
    expect(fetch.mock.calls[0][1].headers['X-CSRF-Token']).toBe('error-token')
    expect(fetch.mock.calls[0][1].body.get('csrf')).toBe('error-token')
    expect(fetch.mock.calls[0][1].body.get('confirm')).toBe('reauthenticate-local')
    expect(errorPage.querySelector('button')?.hasAttribute('disabled')).toBe(true)
    errorForm.dispatchEvent(new Event('submit', { cancelable: true }))
    expect(fetch).toHaveBeenCalledTimes(1)

    const linkPage = mount(SSOLinkPage, {
      issuer: 'https://issuer.test',
      subject: 'subject',
      intent: 'intent',
      csrf: 'link-token',
    })
    linkPage
      .querySelector('form[action="/sso-link"]')!
      .dispatchEvent(new Event('submit', { cancelable: true }))
    await new Promise((resolve) => setTimeout(resolve, 0))
    await nextTick()
    const linkReauth = linkPage.querySelector('form[action="/sso-link/reauth"]')!
    linkReauth.dispatchEvent(new Event('submit', { cancelable: true }))
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(fetch).toHaveBeenCalledTimes(3)
    expect(fetch.mock.calls[2][1].headers['X-CSRF-Token']).toBe('link-token')
    expect(fetch.mock.calls[2][1].body.get('confirm')).toBe('reauthenticate-local')
    expect(
      linkPage.querySelector('form[action="/sso-link/reauth"] button')?.hasAttribute('disabled'),
    ).toBe(true)
  })
  it('accepts only a specific JSON success and never trusts redirects or arbitrary destinations', async () => {
    const fetch = vi.fn()
    vi.stubGlobal('fetch', fetch)
    for (const response of [
      {
        status: 403,
        headers: new Headers({ 'content-type': 'application/json' }),
        json: async () => ({ next: localReauthenticationDestination }),
      },
      {
        status: 200,
        headers: new Headers({ 'content-type': 'text/html' }),
        json: async () => ({ next: localReauthenticationDestination }),
      },
      {
        status: 200,
        headers: new Headers({ 'content-type': 'application/json' }),
        json: async () => ({ next: 'https://evil.invalid' }),
      },
    ]) {
      fetch.mockResolvedValueOnce(response)
      expect(await requestLocalReauthentication('token')).toBe(false)
    }
    fetch.mockResolvedValueOnce({
      status: 200,
      headers: new Headers({ 'content-type': 'application/json' }),
      json: async () => ({ next: localReauthenticationDestination }),
    })
    expect(await requestLocalReauthentication('token')).toBe(true)
  })
  it('renders a diagnostic and does not offer linking when local login is disabled', () => {
    const root = mount(SSOErrorPage, {
      title: 'Нет доступа',
      message: 'Нужна помощь администратора',
      code: 'identity_not_linked',
      linkAvailable: false,
    })
    expect(root.textContent).toContain('identity_not_linked')
    expect(root.querySelector('a[href*="redirectback"]')).toBeNull()
  })
  it('guides a verified unlinked identity through local login to a new confirmation form', () => {
    const root = mount(SSOErrorPage, {
      title: 'Привязка нужна',
      message: 'Войдите локально и подтвердите привязку',
      code: 'identity_not_linked',
      linkAvailable: true,
      returnTarget: '/auth/profile/settings',
    })
    expect(root.querySelector('a[href="/sso-link"]')).not.toBeNull()
    expect(root.querySelector('form[action="/sso-link"]')).toBeNull()
  })
  it('does not loop through linking for an external native session', () => {
    const root = mount(SSOErrorPage, {
      title: 'Локальный вход нужен',
      message: 'Завершите текущий сеанс и войдите локально',
      code: 'local_auth_required',
      linkAvailable: false,
      startAvailable: false,
    })
    expect(root.querySelector('a[href="/sso-link"]')).toBeNull()
    expect(root.querySelector('a[href="/auth/profile/settings"]')).not.toBeNull()
    expect(root.querySelector('a[href="/auth/login?local=1"]')).toBeNull()
  })
})
