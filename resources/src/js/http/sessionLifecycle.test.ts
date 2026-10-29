import axios, { type AxiosAdapter } from 'axios'
import { router } from '@inertiajs/vue3'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  adminFetch,
  configureSessionMaintenance,
  ensureSessionReady,
  installSessionAxiosInterceptor,
  sessionCoordinator,
} from './index'

// Keep the document singleton on the coordinator's injectable clock so the
// hard-expiry case can advance authority time without waiting for real timers.
vi.mock('./session', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./session')>()
  return {
    ...actual,
    createSessionCoordinator: (options: import('./session').SessionCoordinatorOptions = {}) =>
      actual.createSessionCoordinator({ ...options, now: options.now ?? (() => Date.now()) }),
  }
})

const capability = { props: { adminSessionMaintenance: { version: 1, sessionEpoch: 'A' } } }
function reply(epoch: string, hardLifetime = 3600000) {
  const now = Date.now()
  return new Response(
    JSON.stringify({
      status: 200,
      code: 'authentication_recovered',
      csrfReady: true,
      maintenance: {
        version: 1,
        serverTime: new Date(now).toISOString(),
        nextCheckAfterMs: 60000,
        csrfExpiresAt: new Date(now + 300000).toISOString(),
        hardExpiresAt: new Date(now + hardLifetime).toISOString(),
        sessionEpoch: epoch,
        revision: 1,
        renewed: false,
      },
    }),
    { headers: { 'content-type': 'application/json', 'x-goadmin-auth-recovery': '1' } },
  )
}
const flush = async () => {
  for (let i = 0; i < 30; i++) await Promise.resolve()
}
const originalAdapter = axios.defaults.adapter
const disposers: Array<() => void> = []
beforeEach(() => {
  vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) =>
    setTimeout(() => callback(0), 0),
  )
  vi.stubGlobal('cancelAnimationFrame', clearTimeout)
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
})
afterEach(() => {
  router.cancelAll()
  for (const dispose of disposers.splice(0)) dispose()
  axios.defaults.adapter = originalAdapter
  configureSessionMaintenance({ props: {} }, {})
  sessionCoordinator.dispose()
  document.body.innerHTML = ''
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

describe('document epoch and the pinned Inertia transport', () => {
  it.each([
    { status: 401, code: 'reauthentication_required' },
    { status: 403, code: 'access_denied' },
  ])(
    'keeps the mounted editor when a cached-readiness reload receives $status',
    async ({ status, code }) => {
      const request = vi.fn<typeof globalThis.fetch>().mockImplementation(async () => reply('A'))
      vi.stubGlobal('fetch', request)
      disposers.push(installSessionAxiosInterceptor())
      configureSessionMaintenance(capability)
      await ensureSessionReady()
      const root = document.createElement('main')
      document.body.append(root)
      const swap = vi.fn(async () => {
        root.textContent = 'Login replacement'
      })
      const initial = {
        component: 'Editor',
        url: '/editor',
        version: null,
        props: { errors: {}, ...capability.props },
        clearHistory: false,
        encryptHistory: false,
        flash: {},
        rememberedState: {},
      }
      router.init({ initialPage: initial, resolveComponent: async () => ({}), swapComponent: swap })
      await flush()
      swap.mockClear()
      const draft = document.createElement('input')
      draft.value = 'unsaved editor draft'
      root.replaceChildren(draft)
      const sends = vi.fn<AxiosAdapter>().mockImplementation(async (config) => {
        expect(config.method).toBe('get')
        expect(config.headers.get('X-Goadmin-Session-Maintenance')).toBe('1')
        return {
          status,
          statusText: 'Refused',
          config,
          headers: { 'content-type': 'application/json', 'x-goadmin-auth-error': '1' },
          data: JSON.stringify({ status, code }),
        }
      })
      axios.defaults.adapter = sends
      disposers.push(router.on('invalid', (event) => event.preventDefault()))
      const success = vi.fn()
      await new Promise<void>((resolve) =>
        router.reload({ onFinish: () => resolve(), onSuccess: success }),
      )
      expect(root.firstChild).toBe(draft)
      expect(draft.value).toBe('unsaved editor draft')
      expect(swap).not.toHaveBeenCalled()
      expect(success).not.toHaveBeenCalled()
      expect(sessionCoordinator.getState()).toBe(code)
      expect(request).toHaveBeenCalledTimes(1)
      expect(sends).toHaveBeenCalledTimes(1)
      await expect(
        adminFetch('/save', { method: 'POST', body: draft.value }),
      ).rejects.toMatchObject({ code })
      expect(sends).toHaveBeenCalledTimes(1)
      expect(request).toHaveBeenCalledTimes(1)
    },
  )

  it('keeps the mounted Inertia draft after the server refuses a cached-readiness epoch mismatch', async () => {
    const request = vi.fn<typeof globalThis.fetch>().mockImplementation(async () => reply('A'))
    vi.stubGlobal('fetch', request)
    disposers.push(installSessionAxiosInterceptor())
    configureSessionMaintenance(capability)
    await ensureSessionReady()
    const root = document.createElement('main')
    document.body.append(root)
    const swap = vi.fn(async () => {
      root.textContent = 'replacement'
    })
    const initial = {
      component: 'Editor',
      url: '/editor',
      version: null,
      props: { errors: {}, ...capability.props },
      clearHistory: false,
      encryptHistory: false,
      flash: {},
      rememberedState: {},
    }
    router.init({ initialPage: initial, resolveComponent: async () => ({}), swapComponent: swap })
    await flush()
    swap.mockClear()
    const draft = document.createElement('input')
    draft.value = 'A draft while cookies belong to B'
    root.replaceChildren(draft)
    document.cookie = 'csrf_token=B-proof; path=/'
    const sends = vi.fn<AxiosAdapter>().mockImplementation(async (config) => {
      expect(config.headers.get('X-Goadmin-Session-Epoch')).toBe('A')
      expect(config.headers.get('X-CSRF-Token')).toBe('B-proof')
      return {
        status: 409,
        statusText: 'Conflict',
        config,
        headers: { 'content-type': 'application/json', 'x-goadmin-auth-error': '1' },
        data: JSON.stringify({ status: 409, code: 'session_changed' }),
      }
    })
    axios.defaults.adapter = sends
    disposers.push(router.on('invalid', (event) => event.preventDefault()))
    const success = vi.fn()
    await new Promise<void>((resolve) =>
      router.post(
        '/save',
        { text: draft.value },
        {
          onFinish: () => resolve(),
          onSuccess: success,
        },
      ),
    )
    expect(root.firstChild).toBe(draft)
    expect(draft.value).toBe('A draft while cookies belong to B')
    expect(swap).not.toHaveBeenCalled()
    expect(success).not.toHaveBeenCalled()
    expect(sessionCoordinator.getState()).toBe('session_changed')
    expect(request).toHaveBeenCalledTimes(1)
    expect(sends).toHaveBeenCalledTimes(1)
    document.cookie = 'csrf_token=; Max-Age=0; path=/'
  })

  it('never dispatches an A draft when the very first maintenance response belongs to B', async () => {
    const fetch = vi.fn<typeof globalThis.fetch>().mockImplementation(async () => reply('B'))
    vi.stubGlobal('fetch', fetch)
    const sends = vi.fn<AxiosAdapter>()
    axios.defaults.adapter = sends
    disposers.push(installSessionAxiosInterceptor())
    const onReady = vi.fn()
    configureSessionMaintenance(capability, { onReady })
    await Promise.all([
      expect(axios.post('/save', { draftOwnedBy: 'A' })).rejects.toMatchObject({
        code: 'session_changed',
      }),
      expect(
        adminFetch('/save', { method: 'POST', body: 'draft owned by A' }),
      ).rejects.toMatchObject({ code: 'session_changed' }),
    ])
    expect(sessionCoordinator.getState()).toBe('session_changed')
    expect(onReady).not.toHaveBeenCalled()
    expect(sends).not.toHaveBeenCalled()
    expect(fetch).toHaveBeenCalledTimes(1)
    expect(fetch.mock.calls[0]?.[0]).toBe('/auth/session/maintain')
  })

  it('keeps a cancelled draft unsent while its independent maintenance check completes', async () => {
    let finish!: (response: Response) => void
    const fetch = vi.fn<typeof globalThis.fetch>().mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = resolve
        }),
    )
    vi.stubGlobal('fetch', fetch)
    const sends = vi.fn<AxiosAdapter>()
    axios.defaults.adapter = sends
    disposers.push(installSessionAxiosInterceptor())
    configureSessionMaintenance(capability)
    const draft = expect(axios.post('/save', { draftOwnedBy: 'A' })).rejects.toMatchObject({
      code: 'cancelled',
    })
    await flush()
    sessionCoordinator.cancel()
    configureSessionMaintenance(capability)
    await draft
    finish(reply('A'))
    await ensureSessionReady()
    expect(sessionCoordinator.getState()).toBe('ready')
    expect(sends).not.toHaveBeenCalled()
    expect(fetch).toHaveBeenCalledTimes(1)
  })

  it.each(['get', 'post'] as const)(
    'discards a late successful %s Inertia page before swapComponent and retains the mounted draft',
    async (method) => {
      let epoch = 'A'
      vi.stubGlobal(
        'fetch',
        vi.fn<typeof globalThis.fetch>().mockImplementation(async () => reply(epoch)),
      )
      disposers.push(installSessionAxiosInterceptor())
      configureSessionMaintenance(capability)
      await ensureSessionReady()
      const root = document.createElement('main')
      document.body.append(root)
      const swap = vi.fn(async () => {
        root.textContent = 'replacement page'
      })
      const initial = {
        component: 'Editor',
        url: '/editor',
        version: null,
        props: { errors: {}, authuser: { id: 'A' }, ...capability.props },
        clearHistory: false,
        encryptHistory: false,
        flash: {},
        rememberedState: {},
      }
      router.init({ initialPage: initial, resolveComponent: async () => ({}), swapComponent: swap })
      await flush()
      swap.mockClear()
      const draft = document.createElement('input')
      draft.value = 'unsaved A draft'
      root.replaceChildren(draft)
      let finish!: () => void
      const sends = vi.fn<AxiosAdapter>().mockImplementation(
        (config) =>
          new Promise((resolve) => {
            finish = () =>
              resolve({
                status: 200,
                statusText: 'OK',
                config,
                headers: { 'x-inertia': 'true' },
                data: JSON.stringify({ ...initial, component: 'Replacement', url: '/old-result' }),
              })
          }),
      )
      axios.defaults.adapter = sends
      const success = vi.fn()
      disposers.push(router.on('exception', (event) => event.preventDefault()))
      const done = new Promise<void>((resolve) => {
        const options = { onFinish: () => resolve(), onSuccess: success }
        if (method === 'get') router.get('/old-result', {}, options)
        else router.post('/save', { draftOwnedBy: 'A' }, options)
      })
      await flush()
      expect(sends).toHaveBeenCalledTimes(1)
      expect(sends.mock.calls[0]?.[0].headers.get('X-Goadmin-Session-Maintenance')).toBe('1')
      epoch = 'B'
      await expect(ensureSessionReady({ force: true })).rejects.toMatchObject({
        code: 'session_changed',
      })
      finish()
      await done
      expect(swap).not.toHaveBeenCalled()
      expect(success).not.toHaveBeenCalled()
      expect(root.firstChild).toBe(draft)
      expect(draft.value).toBe('unsaved A draft')
      expect(sessionCoordinator.getState()).toBe('session_changed')
      expect(sends).toHaveBeenCalledTimes(1)
    },
  )

  it.each(['maintenance401', 'maintenance403', 'hard-expiry'] as const)(
    'rejects a late negotiated Inertia page after %s while preserving a non-Inertia success',
    async (failure) => {
      let now = Date.now()
      if (failure === 'hard-expiry') vi.spyOn(Date, 'now').mockImplementation(() => now)
      const request = vi.fn<typeof globalThis.fetch>().mockImplementation(async () => reply('A'))
      vi.stubGlobal('fetch', request)
      disposers.push(installSessionAxiosInterceptor())
      configureSessionMaintenance(capability)
      await ensureSessionReady()
      const root = document.createElement('main')
      document.body.append(root)
      const swap = vi.fn(async () => {
        root.textContent = 'replacement page'
      })
      const initial = {
        component: 'Editor',
        url: '/editor',
        version: null,
        props: { errors: {}, authuser: { id: 'A' }, ...capability.props },
        clearHistory: false,
        encryptHistory: false,
        flash: {},
        rememberedState: {},
      }
      router.init({ initialPage: initial, resolveComponent: async () => ({}), swapComponent: swap })
      await flush()
      swap.mockClear()
      const draft = document.createElement('input')
      draft.value = 'unsaved draft'
      root.replaceChildren(draft)
      const finish: Array<() => void> = []
      const sends = vi.fn<AxiosAdapter>().mockImplementation(
        (config) =>
          new Promise((resolve) => {
            const inertia = config.headers.get('X-Inertia') === 'true'
            finish.push(() =>
              resolve({
                status: 200,
                statusText: 'OK',
                config,
                headers: inertia ? { 'x-inertia': 'true' } : {},
                data: inertia
                  ? JSON.stringify({ ...initial, component: 'Replacement', url: '/old-result' })
                  : { saved: true },
              }),
            )
          }),
      )
      axios.defaults.adapter = sends
      disposers.push(router.on('exception', (event) => event.preventDefault()))
      const done = new Promise<void>((resolve) =>
        router.get('/old-result', {}, { onFinish: () => resolve() }),
      )
      const nonInertia = axios.post('/save', { draftOwnedBy: 'A' })
      await flush()
      expect(sends).toHaveBeenCalledTimes(2)
      const code = failure === 'maintenance403' ? 'access_denied' : 'reauthentication_required'
      if (failure === 'hard-expiry') {
        // Advance the authority clock beyond the fixed deadline without racing
        // the real watchdog timer, whose maintenance could renew a 100ms fixture.
        now += 3_600_001
      } else {
        const status = failure === 'maintenance403' ? 403 : 401
        request.mockImplementation(
          async () =>
            new Response(JSON.stringify({ status, code }), {
              status,
              headers: { 'content-type': 'application/json', 'x-goadmin-auth-recovery': '1' },
            }),
        )
      }
      await expect(ensureSessionReady({ force: true })).rejects.toMatchObject({ code })
      for (const resolve of finish) resolve()
      await done
      expect((await nonInertia).data).toEqual({ saved: true })
      expect(swap).not.toHaveBeenCalled()
      expect(root.firstChild).toBe(draft)
      expect(draft.value).toBe('unsaved draft')
      expect(sessionCoordinator.getState()).toBe(code)
      expect(sends).toHaveBeenCalledTimes(2)
    },
  )

  it.each([
    { target: '/auth/login', stale: false },
    { target: '/auth/logout', stale: false },
    { target: '/auth/login', stale: true },
    { target: '/auth/logout', stale: true },
  ])(
    'fences the $target Inertia transition with stale=$stale after terminal state',
    async ({ target, stale }) => {
      vi.stubGlobal(
        'fetch',
        vi.fn<typeof globalThis.fetch>().mockImplementation(async () => reply('A')),
      )
      disposers.push(installSessionAxiosInterceptor())
      configureSessionMaintenance(capability)
      await ensureSessionReady()
      const swap = vi.fn(async () => {})
      const initial = {
        component: 'Editor',
        url: '/editor',
        version: null,
        props: { errors: {}, ...capability.props },
        clearHistory: false,
        encryptHistory: false,
        flash: {},
        rememberedState: {},
      }
      router.init({ initialPage: initial, resolveComponent: async () => ({}), swapComponent: swap })
      await flush()
      swap.mockClear()
      sessionCoordinator.reportFailure('reauthentication_required')
      let finish!: () => void
      const sends = vi.fn<AxiosAdapter>().mockImplementation(
        (config) =>
          new Promise((resolve) => {
            finish = () =>
              resolve({
                status: 200,
                statusText: 'OK',
                config,
                headers: { 'x-inertia': 'true' },
                data: JSON.stringify({
                  ...initial,
                  component: 'Login',
                  url: '/auth/login',
                  props: { errors: {} },
                }),
              })
          }),
      )
      axios.defaults.adapter = sends
      disposers.push(router.on('exception', (event) => event.preventDefault()))
      const done = new Promise<void>((resolve) => {
        if (target === '/auth/logout') router.delete(target, { onFinish: () => resolve() })
        else router.get(target, {}, { onFinish: () => resolve() })
      })
      await flush()
      if (stale) sessionCoordinator.cancel()
      finish()
      await done
      expect(swap).toHaveBeenCalledTimes(stale ? 0 : 1)
      expect(sends).toHaveBeenCalledTimes(1)
    },
  )

  it('accepts an ordinary same-epoch Inertia page through the same Axios boundary', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn<typeof globalThis.fetch>().mockImplementation(async () => reply('A')),
    )
    disposers.push(installSessionAxiosInterceptor())
    configureSessionMaintenance(capability)
    await ensureSessionReady()
    const swap = vi.fn(async () => {})
    const initial = {
      component: 'Editor',
      url: '/editor',
      version: null,
      props: { errors: {}, ...capability.props },
      clearHistory: false,
      encryptHistory: false,
      flash: {},
      rememberedState: {},
    }
    router.init({ initialPage: initial, resolveComponent: async () => ({}), swapComponent: swap })
    await flush()
    swap.mockClear()
    axios.defaults.adapter = async (config) => ({
      status: 200,
      statusText: 'OK',
      config,
      headers: { 'x-inertia': 'true' },
      data: JSON.stringify({ ...initial, component: 'Replacement', url: '/accepted' }),
    })
    await new Promise<void>((resolve) => router.get('/accepted', {}, { onFinish: () => resolve() }))
    expect(swap).toHaveBeenCalledTimes(1)
    expect(sessionCoordinator.getState()).toBe('ready')
  })

  it('preserves a successful non-Inertia Axios caller response after the epoch changes', async () => {
    let epoch = 'A'
    vi.stubGlobal(
      'fetch',
      vi.fn<typeof globalThis.fetch>().mockImplementation(async () => reply(epoch)),
    )
    disposers.push(installSessionAxiosInterceptor())
    configureSessionMaintenance(capability)
    await ensureSessionReady()
    let finish!: () => void
    axios.defaults.adapter = (config) =>
      new Promise((resolve) => {
        finish = () =>
          resolve({ status: 200, statusText: 'OK', config, headers: {}, data: { saved: true } })
      })
    const sent = axios.post('/save', { draftOwnedBy: 'A' })
    await flush()
    epoch = 'B'
    await expect(ensureSessionReady({ force: true })).rejects.toMatchObject({
      code: 'session_changed',
    })
    finish()
    expect((await sent).data).toEqual({ saved: true })
    expect(sessionCoordinator.getState()).toBe('session_changed')
  })
})
