import axios, { type AxiosAdapter } from 'axios'
import { router } from '@inertiajs/vue3'
import type { Page } from '@inertiajs/core'
import { computed, reactive, shallowRef } from 'vue'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import {
  adminFetch,
  configureSessionMaintenance,
  ensureSessionReady,
  installSessionAxiosInterceptor,
  sessionCoordinator,
} from './index'
import { installSessionNavigationCancellation } from './sessionNavigation'

const capability = { adminSessionMaintenance: { version: 1, sessionEpoch: 'A' } }
const initial: Page = {
  component: 'Editor',
  url: '/editor',
  version: null,
  props: { errors: {}, ...capability },
  clearHistory: false,
  encryptHistory: false,
  flash: {},
  rememberedState: {},
}
function ready() {
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
        hardExpiresAt: new Date(now + 3600000).toISOString(),
        sessionEpoch: 'A',
        revision: 1,
        renewed: false,
      },
    }),
    { headers: { 'content-type': 'application/json', 'x-goadmin-auth-recovery': '1' } },
  )
}
async function flush() {
  for (let i = 0; i < 40; i++) await Promise.resolve()
}
const originalAdapter = axios.defaults.adapter
const cleanup: Array<() => void> = []
function pageCancellation() {
  // Match the pinned Vue adapter: one shallow page ref, exposed by usePage.
  const page = shallowRef(initial)
  cleanup.push(
    installSessionNavigationCancellation(
      reactive({
        component: computed(() => page.value.component),
        url: computed(() => page.value.url),
      }),
    ),
  )
  return page
}
beforeEach(() => {
  vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) =>
    setTimeout(() => callback(0), 0),
  )
  vi.stubGlobal('cancelAnimationFrame', clearTimeout)
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
})
afterEach(async () => {
  router.cancelAll()
  await new Promise((resolve) => setTimeout(resolve, 0))
  for (const dispose of cleanup.splice(0)) dispose()
  axios.defaults.adapter = originalAdapter
  configureSessionMaintenance({ props: {} }, {})
  sessionCoordinator.dispose()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

it('fences commands started during lazy page resolution without cancelling destination reads', async () => {
  let finishMaintenance!: (response: Response) => void
  const request = vi.fn<typeof fetch>().mockImplementation(async () => ready())
  vi.stubGlobal('fetch', request)
  cleanup.push(installSessionAxiosInterceptor())
  const current = pageCancellation()
  configureSessionMaintenance({ props: capability })
  await ensureSessionReady()
  let destinationRead: Promise<Response> | undefined
  const swap = vi.fn(async ({ page }: { page: Page }) => {
    current.value = page
    if (page.component === 'Roles') {
      destinationRead = adminFetch('/roles/1/admins')
      void destinationRead.catch(() => {})
    }
  })
  let finishResolution!: (component: object) => void
  router.init({
    initialPage: initial,
    resolveComponent: async (name) =>
      name === 'Roles'
        ? new Promise((resolve) => {
            finishResolution = resolve
          })
        : {},
    swapComponent: swap,
  })
  await flush()
  let finishNavigation!: () => void
  axios.defaults.adapter = vi.fn<AxiosAdapter>().mockImplementation(
    (config) =>
      new Promise((resolve) => {
        finishNavigation = () =>
          resolve({
            status: 200,
            statusText: 'OK',
            config,
            headers: { 'x-inertia': 'true' },
            data: JSON.stringify({ ...initial, component: 'Roles', url: '/roles/1' }),
          })
      }),
  )
  const done = new Promise<void>((resolve) =>
    router.get('/roles/1', {}, { onFinish: () => resolve() }),
  )
  await flush()
  finishNavigation()
  await flush()
  expect(finishResolution).toBeDefined()
  request.mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        finishMaintenance = resolve
      }),
  )
  const maintenance = ensureSessionReady({ force: true }).catch(() => {})
  const oldCommand = expect(adminFetch('/old-command', { method: 'POST' })).rejects.toMatchObject({
    code: 'cancelled',
  })
  finishResolution({})
  await done
  expect(destinationRead).toBeDefined()
  request.mockResolvedValue(new Response('{}'))
  finishMaintenance(ready())
  await maintenance
  await oldCommand
  await expect(destinationRead).resolves.toBeInstanceOf(Response)
  expect(request.mock.calls.map(([url]) => url)).toEqual([
    '/auth/session/maintain',
    '/auth/session/maintain',
    '/roles/1/admins',
  ])
  expect(sessionCoordinator.getState()).toBe('ready')
})

it.each([{}, { name: 'Required' }])(
  'preserves page commands across same-page submission with errors %j',
  async (errors) => {
    vi.stubGlobal(
      'fetch',
      vi.fn<typeof fetch>().mockImplementation(async () => ready()),
    )
    cleanup.push(installSessionAxiosInterceptor())
    const current = pageCancellation()
    configureSessionMaintenance({ props: capability })
    await ensureSessionReady()
    router.init({
      initialPage: initial,
      resolveComponent: async () => ({}),
      swapComponent: async ({ page }) => {
        current.value = page
      },
    })
    await flush()
    const fence = sessionCoordinator.commandFence()
    axios.defaults.adapter = vi.fn<AxiosAdapter>().mockImplementation(async (config) => ({
      status: 200,
      statusText: 'OK',
      config,
      headers: { 'x-inertia': 'true' },
      data: JSON.stringify({ ...initial, props: { ...initial.props, errors } }),
    }))
    await new Promise<void>((resolve) =>
      router.post('/editor', { name: 'Draft' }, { onFinish: () => resolve() }),
    )
    expect(() => sessionCoordinator.assertCommandFence(fence)).not.toThrow()
  },
)

it('fences native history before restoring a page and allows its mounted reads', async () => {
  const request = vi
    .fn<typeof fetch>()
    .mockImplementation(async (url) =>
      url === '/auth/session/maintain' ? ready() : new Response('{}'),
    )
  vi.stubGlobal('fetch', request)
  const current = pageCancellation()
  configureSessionMaintenance({ props: capability })
  await ensureSessionReady()
  let restoring = false
  let destinationRead: Promise<Response> | undefined
  const swap = vi.fn(async ({ page }: { page: Page }) => {
    current.value = page
    if (restoring) destinationRead = adminFetch('/restored-page/read')
  })
  router.init({ initialPage: initial, resolveComponent: async () => ({}), swapComponent: swap })
  await flush()
  const state = window.history.state
  expect(state).not.toBeNull()
  current.value = { ...initial, component: 'Other', url: '/other' }
  const fence = sessionCoordinator.commandFence()
  restoring = true
  window.dispatchEvent(new PopStateEvent('popstate', { state }))
  await flush()
  expect(() => sessionCoordinator.assertCommandFence(fence)).toThrow('Отправка отменена.')
  expect(destinationRead).toBeDefined()
  await expect(destinationRead).resolves.toBeInstanceOf(Response)
  expect(request.mock.calls.map(([url]) => url)).toContain('/restored-page/read')
})

it('preserves commands for hash navigation and stops watching on teardown', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn<typeof fetch>().mockImplementation(async () => ready()),
  )
  const current = reactive({ component: initial.component, url: initial.url })
  const dispose = installSessionNavigationCancellation(current)
  cleanup.push(dispose)
  configureSessionMaintenance({ props: capability })
  await ensureSessionReady()
  const fence = sessionCoordinator.commandFence()
  current.url = '/editor#section'
  expect(() => sessionCoordinator.assertCommandFence(fence)).not.toThrow()
  dispose()
  current.url = '/another-page'
  expect(() => sessionCoordinator.assertCommandFence(fence)).not.toThrow()
})
