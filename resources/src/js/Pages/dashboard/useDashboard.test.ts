import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import { createPinia } from 'pinia'
import { useAuthUserStore } from '@/stores/authUser'
import type { AdminAuthUser } from '@/types/models'
import { useDashboard, type DashboardCard, type DashboardSnapshot } from './useDashboard'

const transport = vi.hoisted(() => ({ request: vi.fn(), state: 'ready' }))
vi.mock('@/http', () => ({
  adminFetch: transport.request,
  sessionCoordinator: { getState: () => transport.state },
}))
const stamp = '2026-10-29T08:00:00Z'
const card = (key = 'queues', overrides: Partial<DashboardCard> = {}): DashboardCard => ({
  key,
  title: { text: key },
  links: [{ label: { text: 'Open' }, href: '/queues' }],
  metrics: [{ key: 'pending', label: { text: 'Pending' }, value: '23' }],
  state: 'ready',
  updatedAt: stamp,
  ...overrides,
})
const snapshot = (cards = [card()], owner = '7'): DashboardSnapshot => ({
  owner,
  cards,
  updatedAt: stamp,
})
const reply = (value = snapshot(), status = 200) => new Response(JSON.stringify(value), { status })
const flush = async () => {
  for (let index = 0; index < 15; index++) await Promise.resolve()
  await nextTick()
}
function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((done) => {
    resolve = done
  })
  return { promise, resolve }
}
const apps: ReturnType<typeof createApp>[] = []
function mount(initial?: DashboardSnapshot, id: string | number | null = 7) {
  const pinia = createPinia()
  const auth = useAuthUserStore(pinia)
  const user = (value: string | number) => ({ id: value }) as unknown as AdminAuthUser
  auth.setAuthUser(id == null ? null : user(id))
  let dashboard!: ReturnType<typeof useDashboard>
  const root = document.createElement('div')
  const app = createApp({
    setup() {
      dashboard = useDashboard(initial)
      return () => h('div')
    },
  })
  app.use(pinia)
  app.mount(root)
  apps.push(app)
  return { dashboard, auth, app, user }
}
beforeEach(() => {
  vi.useFakeTimers()
  transport.state = 'ready'
  transport.request.mockReset().mockImplementation(async () => reply())
  localStorage.clear()
  vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
})
afterEach(() => {
  apps.splice(0).forEach((app) => app.unmount())
  vi.useRealTimers()
  vi.restoreAllMocks()
})

describe('dashboard authorization snapshots', () => {
  it('normalizes the numeric admin identity and replaces loading descriptors on mount', async () => {
    const { dashboard } = mount(
      snapshot([card('queues', { state: 'loading', metrics: [], updatedAt: undefined })]),
    )
    expect(dashboard.cards.value[0]?.state).toBe('loading')
    await flush()
    expect(transport.request).toHaveBeenCalledExactlyOnceWith('/dashboard/data', {
      signal: expect.any(AbortSignal),
    })
    expect(dashboard.cards.value[0]?.metrics[0]?.value).toBe('23')
    expect(dashboard.autoRefresh.value).toBe(false)
  })

  it('fetches even an empty initial set and handles an empty authorized response', async () => {
    transport.request.mockResolvedValue(reply(snapshot([])))
    const { dashboard } = mount(snapshot([]))
    await flush()
    expect(transport.request).toHaveBeenCalledTimes(1)
    expect(dashboard.cards.value).toEqual([])
    expect(dashboard.failed.value).toBe(false)
  })

  it('retains same-owner values and their timestamp only for unavailable cards', async () => {
    const { dashboard } = mount()
    await flush()
    transport.request.mockResolvedValue(
      reply({
        ...snapshot([
          card('queues', { state: 'unavailable', metrics: [], updatedAt: undefined, links: [] }),
          card('new', { state: 'unavailable', metrics: [], updatedAt: undefined }),
        ]),
        updatedAt: '2026-10-29T08:01:00Z',
      }),
    )
    await dashboard.refresh()
    expect(dashboard.cards.value[0]).toMatchObject({
      state: 'unavailable',
      stale: true,
      updatedAt: stamp,
      links: [],
      metrics: [{ value: '23' }],
    })
    expect(dashboard.cards.value[1]).toMatchObject({
      state: 'unavailable',
      metrics: [],
      hasSuccessfulData: false,
    })
    expect(dashboard.cards.value[1]?.updatedAt).toBeUndefined()
  })

  it('preserves successful empty metrics and their timestamp on later failure', async () => {
    transport.request.mockResolvedValueOnce(reply(snapshot([card('queues', { metrics: [] })])))
    const { dashboard } = mount()
    await flush()
    transport.request.mockResolvedValue(
      reply(
        snapshot([card('queues', { state: 'unavailable', metrics: [], updatedAt: undefined })]),
      ),
    )
    await dashboard.refresh()
    expect(dashboard.cards.value[0]).toMatchObject({
      updatedAt: stamp,
      hasSuccessfulData: true,
      stale: true,
    })
  })

  it.each(['provider', 'network'])(
    'retains a status-only snapshot across repeated %s failures',
    async (failure) => {
      const status: DashboardCard['status'] = { text: { text: 'Awaiting approval' }, tone: 'info' }
      transport.request.mockResolvedValueOnce(
        reply(snapshot([card('host.workflow', { metrics: [], status })])),
      )
      const { dashboard } = mount()
      await flush()
      for (let attempt = 0; attempt < 2; attempt++) {
        if (failure === 'provider') {
          transport.request.mockResolvedValueOnce(
            reply(
              snapshot([
                card('host.workflow', { state: 'unavailable', metrics: [], updatedAt: undefined }),
              ]),
            ),
          )
        } else {
          transport.request.mockRejectedValueOnce(new Error('offline'))
        }
        await dashboard.refresh()
        expect(dashboard.cards.value[0]).toMatchObject({
          status,
          metrics: [],
          updatedAt: stamp,
          stale: true,
        })
      }
      transport.request.mockResolvedValueOnce(
        reply(
          snapshot([card('host.workflow', { metrics: [], updatedAt: '2026-10-29T08:02:00Z' })]),
        ),
      )
      await dashboard.refresh()
      expect(dashboard.cards.value[0]?.status).toBeUndefined()
      expect(dashboard.cards.value[0]?.stale).toBe(false)
      expect(dashboard.cards.value[0]?.updatedAt).toBe('2026-10-29T08:02:00Z')
    },
  )

  it('replaces membership, dropping revoked cards and their old values', async () => {
    transport.request.mockResolvedValueOnce(
      reply(
        snapshot([
          card('queues', {
            status: { text: { text: 'Queue status' }, tone: 'warning' },
          }),
        ]),
      ),
    )
    const { dashboard } = mount()
    await flush()
    transport.request.mockResolvedValueOnce(reply(snapshot([card('notifications')])))
    await dashboard.refresh()
    expect(dashboard.cards.value.map((item) => item.key)).toEqual(['notifications'])
    transport.request.mockResolvedValueOnce(
      reply(
        snapshot([card('queues', { state: 'unavailable', metrics: [], updatedAt: undefined })]),
      ),
    )
    await dashboard.refresh()
    expect(dashboard.cards.value[0]?.metrics).toEqual([])
    expect(dashboard.cards.value[0]?.status).toBeUndefined()
    transport.request.mockResolvedValueOnce(reply(snapshot([])))
    await dashboard.refresh()
    expect(dashboard.cards.value).toEqual([])
  })

  it('marks all retained values stale on network failure and clears the error after recovery', async () => {
    const { dashboard } = mount()
    await flush()
    transport.request.mockRejectedValueOnce(new Error('offline'))
    await dashboard.refresh()
    expect(dashboard.failed.value).toBe(true)
    expect(dashboard.cards.value[0]).toMatchObject({
      state: 'unavailable',
      stale: true,
      updatedAt: stamp,
      metrics: [{ value: '23' }],
    })
    await dashboard.refresh()
    expect(dashboard.failed.value).toBe(false)
    expect(dashboard.cards.value[0]?.stale).toBe(false)
  })

  it('does not synthesize zero metrics for a first-load failure', async () => {
    transport.request.mockRejectedValue(new Error('offline'))
    const { dashboard } = mount(
      snapshot([card('queues', { state: 'loading', metrics: [], updatedAt: undefined })]),
    )
    await flush()
    expect(dashboard.cards.value[0]).toMatchObject({ state: 'unavailable', metrics: [] })
    expect(dashboard.failed.value).toBe(true)
    expect(dashboard.updatedAt.value).toBe(stamp)
  })

  it.each([401, 403])('clears and stops on HTTP %i', async (status) => {
    transport.request.mockResolvedValue(reply(snapshot(), status))
    const { dashboard } = mount(snapshot())
    dashboard.autoRefresh.value = true
    await flush()
    expect(dashboard.cards.value).toEqual([])
    expect(dashboard.updatedAt.value).toBeUndefined()
    expect(dashboard.stopped.value).toBe(true)
    expect(dashboard.autoRefresh.value).toBe(false)
    await dashboard.refresh()
    await vi.advanceTimersByTimeAsync(180_000)
    expect(transport.request).toHaveBeenCalledTimes(1)
  })

  it.each(['reauthentication_required', 'access_denied', 'session_changed'])(
    'clears data and aborts when app clears auth after %s',
    async () => {
      const pending = deferred<Response>()
      transport.request.mockReturnValue(pending.promise)
      const { dashboard, auth } = mount(snapshot())
      await flush()
      const signal = transport.request.mock.calls[0]?.[1].signal as AbortSignal
      dashboard.autoRefresh.value = true
      auth.setAuthUser(null)
      expect(signal.aborted).toBe(true)
      expect(dashboard.cards.value).toEqual([])
      expect(dashboard.stopped.value).toBe(true)
      pending.resolve(reply())
      await flush()
      expect(dashboard.cards.value).toEqual([])
    },
  )

  it('rejects a mismatched response owner without displaying data', async () => {
    transport.request.mockResolvedValue(reply(snapshot([card('secret')], '99')))
    const { dashboard } = mount(snapshot())
    await flush()
    expect(dashboard.cards.value).toEqual([])
    expect(dashboard.stopped.value).toBe(true)
  })

  it('aborts an old actor and ignores late responses after switching away and back', async () => {
    const pending = deferred<Response>()
    transport.request.mockReturnValueOnce(pending.promise)
    const { dashboard, auth, user } = mount(snapshot())
    await flush()
    dashboard.autoRefresh.value = true
    auth.setAuthUser(user('8'))
    expect(dashboard.cards.value).toEqual([])
    expect(dashboard.autoRefresh.value).toBe(false)
    transport.request.mockResolvedValueOnce(reply(snapshot([card('actor8')], '8')))
    await flush()
    expect(dashboard.cards.value[0]?.key).toBe('actor8')
    transport.request.mockResolvedValueOnce(reply(snapshot([card('fresh7')])))
    auth.setAuthUser(user(7))
    await flush()
    pending.resolve(reply(snapshot([card('old7')])))
    await flush()
    expect(dashboard.cards.value[0]?.key).toBe('fresh7')
  })

  it('ignores an initial snapshot belonging to another owner', async () => {
    transport.request.mockReturnValue(new Promise(() => {}))
    const { dashboard } = mount(snapshot([card('secret')], '99'))
    expect(dashboard.cards.value).toEqual([])
  })
})

describe('dashboard refresh lifecycle', () => {
  it('coalesces repeated manual requests with the mounted request', async () => {
    const pending = deferred<Response>()
    transport.request.mockReturnValue(pending.promise)
    const { dashboard } = mount()
    await flush()
    const first = dashboard.refresh()
    expect(dashboard.refresh()).toBe(first)
    expect(transport.request).toHaveBeenCalledTimes(1)
    pending.resolve(reply())
    await first
    transport.request.mockResolvedValue(reply())
    await dashboard.refresh()
    expect(transport.request).toHaveBeenCalledTimes(2)
  })

  it('persists opt-in per admin and refreshes every 60 seconds only while enabled', async () => {
    const { dashboard } = mount()
    await flush()
    await vi.advanceTimersByTimeAsync(60_000)
    expect(transport.request).toHaveBeenCalledTimes(1)
    dashboard.autoRefresh.value = true
    await flush()
    expect(localStorage.getItem('goadmin.dashboard.autoRefresh.7')).toBe('true')
    await vi.advanceTimersByTimeAsync(59_999)
    expect(transport.request).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(1)
    expect(transport.request).toHaveBeenCalledTimes(2)
    dashboard.autoRefresh.value = false
    await flush()
    await vi.advanceTimersByTimeAsync(120_000)
    expect(transport.request).toHaveBeenCalledTimes(2)
  })

  it('restores only the current owner preference on mount', async () => {
    localStorage.setItem('goadmin.dashboard.autoRefresh.7', 'true')
    localStorage.setItem('goadmin.dashboard.autoRefresh.8', 'false')
    const { dashboard } = mount()
    await flush()
    expect(dashboard.autoRefresh.value).toBe(true)
    await vi.advanceTimersByTimeAsync(60_000)
    expect(transport.request).toHaveBeenCalledTimes(2)
  })

  it.each([
    ['true', false],
    ['false', false],
    ['true', true],
    ['false', true],
  ] as const)(
    'keeps both owners preferences when the new owner saved %s and the toggle was flushed: %s',
    async (saved, flushed) => {
      localStorage.setItem('goadmin.dashboard.autoRefresh.8', saved)
      const { dashboard, auth, user } = mount()
      await flush()
      // Switch before the next tick: a queued persistence effect must not use owner 8.
      dashboard.autoRefresh.value = true
      if (flushed) await flush()
      transport.request.mockResolvedValue(reply(snapshot([], '8')))
      auth.setAuthUser(user(8))
      await flush()
      expect(localStorage.getItem('goadmin.dashboard.autoRefresh.7')).toBe('true')
      expect(localStorage.getItem('goadmin.dashboard.autoRefresh.8')).toBe(saved)
      expect(dashboard.autoRefresh.value).toBe(false)
      await vi.advanceTimersByTimeAsync(60_000)
      expect(transport.request).toHaveBeenCalledTimes(2)
    },
  )

  it.each(['terminal session', 'unmount'])(
    'preserves the saved opt-in after %s and restores it on remount',
    async (reason) => {
      localStorage.setItem('goadmin.dashboard.autoRefresh.7', 'true')
      const { dashboard, app } = mount()
      await flush()
      if (reason === 'terminal session') {
        transport.state = 'session_changed'
        await dashboard.refresh()
      }
      app.unmount()
      apps.splice(apps.indexOf(app), 1)
      expect(localStorage.getItem('goadmin.dashboard.autoRefresh.7')).toBe('true')
      transport.state = 'ready'
      const remounted = mount()
      await flush()
      expect(remounted.dashboard.autoRefresh.value).toBe(true)
    },
  )

  it('survives blocked storage reads and writes', async () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('blocked')
    })
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('blocked')
    })
    const { dashboard } = mount()
    await flush()
    expect(dashboard.autoRefresh.value).toBe(false)
    dashboard.autoRefresh.value = true
    await flush()
    await vi.advanceTimersByTimeAsync(60_000)
    expect(transport.request).toHaveBeenCalledTimes(2)
  })

  it('pauses while hidden and resumes with one coalesced refresh, without backlog', async () => {
    const { dashboard } = mount()
    await flush()
    dashboard.autoRefresh.value = true
    await flush()
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(true)
    document.dispatchEvent(new Event('visibilitychange'))
    await vi.advanceTimersByTimeAsync(300_000)
    expect(transport.request).toHaveBeenCalledTimes(1)
    const pending = deferred<Response>()
    transport.request.mockReturnValueOnce(pending.promise)
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
    document.dispatchEvent(new Event('visibilitychange'))
    await flush()
    await vi.advanceTimersByTimeAsync(120_000)
    void dashboard.refresh()
    expect(transport.request).toHaveBeenCalledTimes(2)
    pending.resolve(reply())
    await flush()
    await vi.advanceTimersByTimeAsync(60_000)
    expect(transport.request).toHaveBeenCalledTimes(3)
  })

  it('aborts pending requests and removes timers and visibility listeners on unmount', async () => {
    const pending = deferred<Response>()
    transport.request.mockReturnValue(pending.promise)
    const remove = vi.spyOn(document, 'removeEventListener')
    const { dashboard, app } = mount(snapshot())
    dashboard.autoRefresh.value = true
    await flush()
    const signal = transport.request.mock.calls[0]?.[1].signal as AbortSignal
    app.unmount()
    apps.splice(apps.indexOf(app), 1)
    expect(signal.aborted).toBe(true)
    expect(remove).toHaveBeenCalledWith('visibilitychange', expect.any(Function))
    pending.resolve(reply())
    document.dispatchEvent(new Event('visibilitychange'))
    await vi.advanceTimersByTimeAsync(180_000)
    expect(transport.request).toHaveBeenCalledTimes(1)
    expect(dashboard.cards.value).toEqual([])
  })
})
