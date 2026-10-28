import axios, {
  AxiosError,
  type AxiosAdapter,
  type AxiosResponse,
  type InternalAxiosRequestConfig,
} from 'axios'
import { router } from '@inertiajs/vue3'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  adminFetch,
  configureSessionMaintenance,
  ensureSessionReady,
  installSessionAxiosInterceptor,
  sessionCoordinator,
  reportSessionAuthFailure,
} from './index'
import { reconcileUploadCompletion, uploadFiles } from '@/services/fileUploadService'

const capability = { props: { adminSessionMaintenance: { version: 1, sessionEpoch: 'one' } } }
function ready(epoch = 'one', revision = 1) {
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
        sessionEpoch: epoch,
        revision,
        renewed: false,
      },
    }),
    { headers: { 'Content-Type': 'application/json', 'X-Goadmin-Auth-Recovery': '1' } },
  )
}
const response = (
  config: InternalAxiosRequestConfig,
  status = 200,
  data: unknown = {},
): AxiosResponse => ({
  status,
  statusText: '',
  config,
  data,
  headers: {},
})
const flush = async () => {
  for (let i = 0; i < 20; i++) await Promise.resolve()
}
let stopAxios: (() => void) | undefined
const adapter = axios.defaults.adapter
beforeEach(() => {
  vi.useFakeTimers()
  vi.setSystemTime(new Date('2026-10-28T10:00:00Z'))
})
afterEach(() => {
  router.cancelAll()
  stopAxios?.()
  stopAxios = undefined
  axios.defaults.adapter = adapter
  configureSessionMaintenance({ props: {} }, {})
  sessionCoordinator.dispose()
  document.cookie = 'csrf_token=; Max-Age=0; path=/'
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
  vi.useRealTimers()
})

describe('public admin transport', () => {
  it('preserves the legacy transport without capability opt-in', async () => {
    const fetch = vi.fn<typeof globalThis.fetch>().mockResolvedValue(new Response('ok'))
    vi.stubGlobal('fetch', fetch)
    await adminFetch('/legacy', { method: 'POST', body: 'draft' })
    expect(fetch).toHaveBeenCalledExactlyOnceWith('/legacy', { method: 'POST', body: 'draft' })
  })

  it('leaves the Axios maintenance negotiation header absent without capability opt-in', async () => {
    const sends = vi.fn<AxiosAdapter>().mockImplementation(async (config) => response(config))
    axios.defaults.adapter = sends
    stopAxios = installSessionAxiosInterceptor()
    await axios.post('/legacy', { draft: true })
    expect(sends).toHaveBeenCalledTimes(1)
    expect(sends.mock.calls[0]?.[0].headers.has('X-Goadmin-Session-Maintenance')).toBe(false)
  })

  it('gates one original mutation and reads the CSRF cookie only after readiness', async () => {
    let resolve!: (response: Response) => void
    const fetch = vi
      .fn<typeof globalThis.fetch>()
      .mockImplementationOnce(
        () =>
          new Promise((done) => {
            resolve = done
          }),
      )
      .mockResolvedValueOnce(new Response('saved'))
    vi.stubGlobal('fetch', fetch)
    configureSessionMaintenance(capability)
    document.cookie = 'csrf_token=old; path=/'
    const pending = adminFetch('/save', { method: 'POST', body: 'draft' })
    expect(fetch).toHaveBeenCalledTimes(1)
    document.cookie = 'csrf_token=current-proof; path=/'
    resolve(ready())
    await pending
    expect(fetch).toHaveBeenCalledTimes(2)
    expect(fetch.mock.calls[1]?.[0]).toBe('/save')
    expect(new Headers(fetch.mock.calls[1]?.[1]?.headers).get('X-CSRF-Token')).toBe('current-proof')
    expect(
      new Headers(fetch.mock.calls[1]?.[1]?.headers).get('X-Goadmin-Session-Maintenance'),
    ).toBe('1')
    expect(fetch.mock.calls[1]?.[1]?.body).toBe('draft')
  })

  it('never replays a rejected mutation and keeps transient authentication state retryable', async () => {
    const onState = vi.fn()
    const fetch = vi
      .fn<typeof globalThis.fetch>()
      .mockResolvedValueOnce(ready())
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            status: 503,
            code: 'authentication_unavailable',
          }),
          {
            status: 503,
            headers: { 'X-Goadmin-Auth-Error': '1', 'Content-Type': 'application/json' },
          },
        ),
      )
    vi.stubGlobal('fetch', fetch)
    configureSessionMaintenance(capability, { onState })
    await ensureSessionReady()
    expect((await adminFetch('/save', { method: 'POST', body: 'draft' })).status).toBe(503)
    expect(fetch).toHaveBeenCalledTimes(2)
    expect(sessionCoordinator.getState()).toBe('authentication_unavailable')
    expect(onState).not.toHaveBeenCalledWith('reauthentication_required')
  })

  it('allows one explicitly safe read retry and no further retry', async () => {
    const rejected = () =>
      new Response(JSON.stringify({ status: 503, code: 'authentication_unavailable' }), {
        status: 503,
        headers: { 'X-Goadmin-Auth-Error': '1', 'Content-Type': 'application/json' },
      })
    const fetch = vi
      .fn<typeof globalThis.fetch>()
      .mockResolvedValueOnce(ready())
      .mockResolvedValueOnce(rejected())
      .mockResolvedValueOnce(ready())
      .mockResolvedValueOnce(rejected())
    vi.stubGlobal('fetch', fetch)
    configureSessionMaintenance(capability)
    await ensureSessionReady()
    expect((await adminFetch('/rows', { retrySafeRead: true })).status).toBe(503)
    expect(fetch.mock.calls.map(([url]) => url)).toEqual([
      '/auth/session/maintain',
      '/rows',
      '/auth/session/maintain',
      '/rows',
    ])
  })

  it('discards an unsent command on user cancellation even if readiness replies later', async () => {
    let resolve!: (response: Response) => void
    const fetch = vi.fn<typeof globalThis.fetch>(
      () =>
        new Promise((done) => {
          resolve = done
        }),
    )
    vi.stubGlobal('fetch', fetch)
    configureSessionMaintenance(capability)
    const abort = new AbortController()
    const pending = expect(
      adminFetch('/save', { method: 'POST', body: 'draft', signal: abort.signal }),
    ).rejects.toMatchObject({ code: 'cancelled' })
    abort.abort()
    await pending
    resolve(ready())
    await flush()
    expect(fetch).toHaveBeenCalledTimes(1)
  })

  it('gates actual Inertia transport once, retaining the original form body', async () => {
    let resolve!: (response: Response) => void
    vi.stubGlobal(
      'fetch',
      vi.fn<typeof globalThis.fetch>(
        () =>
          new Promise((done) => {
            resolve = done
          }),
      ),
    )
    const sends = vi
      .fn<AxiosAdapter>()
      .mockImplementation(async (config) => response(config, 200, '{}'))
    axios.defaults.adapter = sends
    stopAxios = installSessionAxiosInterceptor()
    configureSessionMaintenance(capability)
    router.init({
      initialPage: {
        component: 'Fixture',
        url: '/edit',
        version: null,
        props: { errors: {} },
        clearHistory: false,
        encryptHistory: false,
        flash: {},
        rememberedState: {},
      },
      resolveComponent: async () => ({}),
      swapComponent: async () => {},
    })
    const stopInvalid = router.on('invalid', (event) => event.preventDefault())
    router.post('/save', { title: 'unsaved draft' })
    await flush()
    expect(sends).not.toHaveBeenCalled()
    document.cookie = 'csrf_token=proof-at-send; path=/'
    resolve(ready())
    await flush()
    expect(sends).toHaveBeenCalledTimes(1)
    expect(sends.mock.calls[0]?.[0].data).toBe('{"title":"unsaved draft"}')
    expect(sends.mock.calls[0]?.[0].headers.get('X-CSRF-Token')).toBe('proof-at-send')
    expect(sends.mock.calls[0]?.[0].headers.get('X-Goadmin-Session-Maintenance')).toBe('1')
    stopInvalid()
  })

  it('coalesces double-clicked Inertia submissions while its normal cancellation discards the first continuation', async () => {
    let resolve!: (response: Response) => void
    vi.stubGlobal(
      'fetch',
      vi.fn<typeof globalThis.fetch>(
        () =>
          new Promise((done) => {
            resolve = done
          }),
      ),
    )
    const sends = vi
      .fn<AxiosAdapter>()
      .mockImplementation(async (config) => response(config, 200, '{}'))
    axios.defaults.adapter = sends
    stopAxios = installSessionAxiosInterceptor()
    configureSessionMaintenance(capability)
    const stopInvalid = router.on('invalid', (event) => event.preventDefault())
    const stopException = router.on('exception', (event) => event.preventDefault())
    router.post('/save', { title: 'draft' })
    await flush()
    router.post('/save', { title: 'draft' })
    await flush()
    expect(sends).not.toHaveBeenCalled()
    resolve(ready())
    await flush()
    expect(sends).toHaveBeenCalledTimes(1)
    stopInvalid()
    stopException()
  })

  it('repairs CSRF after a fetch 419 while returning the original rejection without replay', async () => {
    const fetch = vi
      .fn<typeof globalThis.fetch>()
      .mockResolvedValueOnce(ready())
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            status: 419,
            code: 'csrf_needs_sync',
          }),
          {
            status: 419,
            headers: { 'X-Goadmin-Auth-Error': '1', 'Content-Type': 'application/json' },
          },
        ),
      )
      .mockResolvedValueOnce(ready())
    vi.stubGlobal('fetch', fetch)
    const onState = vi.fn()
    configureSessionMaintenance(capability, { onState })
    await ensureSessionReady()
    expect((await adminFetch('/save', { method: 'POST', body: 'draft' })).status).toBe(419)
    await ensureSessionReady()
    expect(fetch.mock.calls.map(([url]) => url)).toEqual([
      '/auth/session/maintain',
      '/save',
      '/auth/session/maintain',
    ])
    expect(onState).toHaveBeenCalledWith('csrf_needs_sync')
    expect(sessionCoordinator.getState()).toBe('ready')
  })

  it('shares a 419 repair across Axios and the Inertia invalid observer without replay', async () => {
    const fetch = vi.fn<typeof globalThis.fetch>().mockImplementation(async () => ready())
    vi.stubGlobal('fetch', fetch)
    const failedHeaders = { 'x-goadmin-auth-error': '1', 'content-type': 'application/json' }
    const failedData = { status: 419, code: 'csrf_needs_sync' }
    const sends = vi.fn<AxiosAdapter>().mockImplementation(async (config) => {
      const failed = response(config, 419, JSON.stringify(failedData))
      failed.headers = failedHeaders
      throw new AxiosError('CSRF', 'ERR_BAD_RESPONSE', config, undefined, failed)
    })
    axios.defaults.adapter = sends
    stopAxios = installSessionAxiosInterceptor()
    configureSessionMaintenance(capability)
    await ensureSessionReady()
    await expect(axios.post('/save', { draft: true })).rejects.toBeInstanceOf(AxiosError)
    expect(
      reportSessionAuthFailure(
        419,
        { get: (name) => failedHeaders[name as keyof typeof failedHeaders] },
        failedData,
        false,
      ),
    ).toBe(true)
    await ensureSessionReady()
    expect(sends).toHaveBeenCalledTimes(1)
    expect(fetch).toHaveBeenCalledTimes(2)
  })

  it.each([
    '/sso',
    '/login',
    '/callback',
    '/sso/open',
    '/sso/login',
    '/sso/callback',
    '/auth/session/maintain',
    '/auth/session/recover',
  ])('bypasses the readiness gate for explicit transition %s', async (target) => {
    const fetch = vi
      .fn<typeof globalThis.fetch>()
      .mockImplementationOnce(() => new Promise(() => {}))
      .mockResolvedValueOnce(new Response('transition'))
    vi.stubGlobal('fetch', fetch)
    configureSessionMaintenance(capability)
    expect((await adminFetch(target)).status).toBe(200)
    expect(fetch).toHaveBeenCalledTimes(2)
  })

  it('cancels waiting commands on explicit logout without waiting for readiness', async () => {
    let resolve!: (response: Response) => void
    const fetch = vi
      .fn<typeof globalThis.fetch>()
      .mockImplementationOnce(
        () =>
          new Promise((done) => {
            resolve = done
          }),
      )
      .mockResolvedValueOnce(new Response('signed out'))
    vi.stubGlobal('fetch', fetch)
    configureSessionMaintenance(capability)
    const command = expect(adminFetch('/save', { method: 'POST' })).rejects.toMatchObject({
      code: 'cancelled',
    })
    await adminFetch('/logout', { method: 'POST' })
    await command
    resolve(ready())
    await flush()
    expect(fetch.mock.calls.map(([url]) => url)).toEqual(['/auth/session/maintain', '/logout'])
    expect(sessionCoordinator.getState()).toBe('reauthentication_required')
    sessionCoordinator.reportFailure('authentication_unavailable')
    expect(sessionCoordinator.getState()).toBe('reauthentication_required')
  })

  it('marks a network failure retryable without clearing authentication or replaying', async () => {
    const fetch = vi
      .fn<typeof globalThis.fetch>()
      .mockResolvedValueOnce(ready())
      .mockRejectedValueOnce(new TypeError('network failed'))
    vi.stubGlobal('fetch', fetch)
    configureSessionMaintenance(capability)
    await ensureSessionReady()
    await expect(adminFetch('/save', { method: 'POST' })).rejects.toThrow('network failed')
    expect(sessionCoordinator.getState()).toBe('authentication_unavailable')
    expect(fetch).toHaveBeenCalledTimes(2)
  })

  it('Inertia cancellation during the gate never reaches the adapter', async () => {
    let resolve!: (response: Response) => void
    vi.stubGlobal(
      'fetch',
      vi.fn<typeof globalThis.fetch>(
        () =>
          new Promise((done) => {
            resolve = done
          }),
      ),
    )
    const sends = vi.fn<AxiosAdapter>()
    axios.defaults.adapter = sends
    stopAxios = installSessionAxiosInterceptor()
    configureSessionMaintenance(capability)
    const stopException = router.on('exception', (event) => event.preventDefault())
    router.post('/save', { title: 'draft' })
    await flush()
    router.cancelAll()
    await flush()
    resolve(ready())
    await flush()
    expect(sends).not.toHaveBeenCalled()
    stopException()
  })

  it('does not expose an upload completion session before its readiness gate dispatches', async () => {
    let resolve!: (response: Response) => void
    vi.stubGlobal(
      'fetch',
      vi.fn<typeof globalThis.fetch>(
        () =>
          new Promise((done) => {
            resolve = done
          }),
      ),
    )
    const sends = vi
      .fn<AxiosAdapter>()
      .mockImplementation(async (config) =>
        response(config, 200, { tasks: [{ id: 7, status: 'queued' }] }),
      )
    axios.defaults.adapter = sends
    stopAxios = installSessionAxiosInterceptor()
    configureSessionMaintenance(capability)
    const pending = reconcileUploadCompletion({
      location: '/files/tus/existing',
      filename: 'a.mp3',
      entityType: 'fixture',
      entityId: 1,
    })
    await flush()
    expect(sends).not.toHaveBeenCalled()
    resolve(ready())
    const result = await pending
    expect(result.status).not.toBe('completion_unknown')
    expect(sends).toHaveBeenCalledTimes(1)
    expect(sends.mock.calls[0]?.[0].url).toBe('/files/tus/existing/complete')
  })

  it('retains existing completion uncertainty on a pre-send 503 without sending a reconciliation', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn<typeof globalThis.fetch>().mockResolvedValue(
        new Response(JSON.stringify({ status: 503, code: 'authentication_unavailable' }), {
          status: 503,
          headers: { 'X-Goadmin-Auth-Recovery': '1', 'Content-Type': 'application/json' },
        }),
      ),
    )
    const sends = vi.fn<AxiosAdapter>()
    axios.defaults.adapter = sends
    stopAxios = installSessionAxiosInterceptor()
    configureSessionMaintenance(capability)
    const result = await reconcileUploadCompletion({
      location: '/files/tus/existing',
      filename: 'a.mp3',
      entityType: 'fixture',
      entityId: 1,
    })
    expect(result.status).toBe('completion_unknown')
    expect(result.uncertainCompletion?.location).toBe('/files/tus/existing')
    expect(sends).not.toHaveBeenCalled()
  })

  it('does not create a fresh TUS session or uncertainty when readiness fails before its first send', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn<typeof globalThis.fetch>().mockResolvedValue(
        new Response(JSON.stringify({ status: 503, code: 'authentication_unavailable' }), {
          status: 503,
          headers: { 'X-Goadmin-Auth-Recovery': '1', 'Content-Type': 'application/json' },
        }),
      ),
    )
    const sends = vi.fn<AxiosAdapter>()
    axios.defaults.adapter = sends
    stopAxios = installSessionAxiosInterceptor()
    configureSessionMaintenance(capability)
    vi.spyOn(console, 'error').mockImplementation(() => {})
    const result = await uploadFiles({
      file: new File(['a'], 'a.mp3'),
      entityType: 'fixture',
      entityId: 1,
    })
    expect(result.error).toContain('Сервис авторизации временно недоступен')
    expect(result.status).not.toBe('completion_unknown')
    expect(result).not.toHaveProperty('uncertainCompletion')
    expect(sends).not.toHaveBeenCalled()
  })

  it.each([
    ['epoch', 'auth'],
    ['epoch', 'network'],
    ['generation', 'auth'],
    ['generation', 'network'],
  ] as const)(
    'keeps fetch response effects fenced after a %s change for a late %s failure',
    async (change, failure) => {
      let epoch = 'one'
      let finish!: () => void
      const lateResponse = new Response(
        JSON.stringify({ status: 401, code: 'reauthentication_required' }),
        {
          status: 401,
          headers: { 'X-Goadmin-Auth-Error': '1', 'Content-Type': 'application/json' },
        },
      )
      const fetch = vi.fn<typeof globalThis.fetch>().mockImplementation(async (input) => {
        if (String(input) === '/auth/session/maintain') return ready(epoch)
        return new Promise<Response>((resolve, reject) => {
          finish = () =>
            failure === 'auth' ? resolve(lateResponse) : reject(new TypeError('network failed'))
        })
      })
      vi.stubGlobal('fetch', fetch)
      configureSessionMaintenance(capability)
      await ensureSessionReady()
      const request = adminFetch('/save', { method: 'POST', body: 'original command' })
      const result =
        failure === 'network' ? expect(request).rejects.toThrow('network failed') : request
      await flush()
      if (change === 'epoch') {
        epoch = 'two'
        await expect(ensureSessionReady({ force: true })).rejects.toMatchObject({
          code: 'session_changed',
        })
      } else sessionCoordinator.cancel()
      finish()
      if (failure === 'auth') expect(await result).toBe(lateResponse)
      else await result
      expect(sessionCoordinator.getState()).toBe(change === 'epoch' ? 'session_changed' : 'ready')
      expect(sessionCoordinator.getMaintenance()?.sessionEpoch).toBe(epoch)
      expect(fetch.mock.calls.filter(([url]) => url === '/save')).toHaveLength(1)
    },
  )

  it.each([
    ['epoch', 'auth'],
    ['epoch', 'network'],
    ['generation', 'auth'],
    ['generation', 'network'],
  ] as const)(
    'keeps Axios response effects fenced after a %s change for a late %s failure',
    async (change, failure) => {
      let epoch = 'one'
      let finish!: () => void
      vi.stubGlobal(
        'fetch',
        vi.fn<typeof globalThis.fetch>().mockImplementation(async () => ready(epoch)),
      )
      const sends = vi.fn<AxiosAdapter>().mockImplementation(
        (config) =>
          new Promise((_, reject) => {
            finish = () => {
              if (failure === 'network') {
                reject(new AxiosError('network failed', 'ERR_NETWORK', config))
                return
              }
              const failed = response(config, 401, {
                status: 401,
                code: 'reauthentication_required',
              })
              failed.headers = { 'x-goadmin-auth-error': '1', 'content-type': 'application/json' }
              reject(
                new AxiosError('original rejection', 'ERR_BAD_RESPONSE', config, undefined, failed),
              )
            }
          }),
      )
      axios.defaults.adapter = sends
      stopAxios = installSessionAxiosInterceptor()
      configureSessionMaintenance(capability)
      await ensureSessionReady()
      const request = axios.post('/save', { draft: 'original command' })
      const rejected =
        failure === 'auth'
          ? expect(request).rejects.toMatchObject({ response: { status: 401 } })
          : expect(request).rejects.toThrow('network failed')
      await flush()
      if (change === 'epoch') {
        epoch = 'two'
        await expect(ensureSessionReady({ force: true })).rejects.toMatchObject({
          code: 'session_changed',
        })
      } else sessionCoordinator.cancel()
      finish()
      await rejected
      expect(sessionCoordinator.getState()).toBe(change === 'epoch' ? 'session_changed' : 'ready')
      expect(sessionCoordinator.getMaintenance()?.sessionEpoch).toBe(epoch)
      expect(sends).toHaveBeenCalledTimes(1)
    },
  )

  it('never replays an Axios mutation that returned an auth error', async () => {
    vi.stubGlobal('fetch', vi.fn<typeof globalThis.fetch>().mockResolvedValue(ready()))
    const sends = vi.fn<AxiosAdapter>().mockImplementation(async (config) => {
      const failed = response(config, 503, { status: 503, code: 'authentication_unavailable' })
      failed.headers = { 'x-goadmin-auth-error': '1', 'content-type': 'application/json' }
      throw new AxiosError('unavailable', 'ERR_BAD_RESPONSE', config, undefined, failed)
    })
    axios.defaults.adapter = sends
    stopAxios = installSessionAxiosInterceptor()
    configureSessionMaintenance(capability)
    await ensureSessionReady()
    await expect(axios.post('/save', { draft: true })).rejects.toBeInstanceOf(AxiosError)
    expect(sends).toHaveBeenCalledTimes(1)
    expect(sessionCoordinator.getState()).toBe('authentication_unavailable')
  })
})
