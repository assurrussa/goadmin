import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, defineComponent, ref, type App } from 'vue'
import { createPinia } from 'pinia'
import { useQueueStats } from './useQueueStats'
import { useAuthUserStore } from '@/stores/authUser'
import { sessionCoordinator } from '@/http'
import type { AdminAuthUser } from '@/types/models'

const apps: App[] = []
const owner = (id: string): AdminAuthUser => ({
  id,
  name: 'Owner',
  lastName: 'Fixture',
  email: 'owner@example.test',
  roles: [],
  avatarUrl: '',
})
const response = (jobs: number, failed: number) =>
  ({ ok: true, json: async () => ({ jobs, failed }) }) as Response
let fetchMock: ReturnType<typeof vi.fn<typeof fetch>>
function mount() {
  let stats!: ReturnType<typeof useQueueStats>
  const url = ref('/queues/stats')
  const initial = ref({ jobs: 5, failed: 3 })
  const pinia = createPinia()
  const auth = useAuthUserStore(pinia)
  auth.setAuthUser(owner('1'))
  const app = createApp(
    defineComponent({
      setup() {
        stats = useQueueStats(url, initial)
        return () => null
      },
    }),
  )
  app.use(pinia)
  app.mount(document.createElement('div'))
  apps.push(app)
  return { stats, app, auth, url, initial }
}
beforeEach(() => {
  fetchMock = vi.fn<typeof fetch>().mockResolvedValue(response(9, 4))
  vi.stubGlobal('fetch', fetchMock)
})
afterEach(() => {
  apps.splice(0).forEach((app) => app.unmount())
  sessionCoordinator.configure({})
  sessionCoordinator.dispose()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})
describe('queue stats response ownership', () => {
  it('keeps only the latest server snapshot when requests finish in reverse order', async () => {
    const pending: Array<(response: Response) => void> = []
    fetchMock.mockImplementation(() => new Promise((resolve) => pending.push(resolve)))
    const { stats } = mount()
    const first = stats.refreshStats()
    const second = stats.refreshStats()
    pending[1](response(42, 17))
    await second
    pending[0](response(1, 1))
    await first
    expect(stats.counters.value).toEqual({ jobs: 42, failed: 17 })
    expect(stats.stale.value).toBe(false)
  })
  it.each(['unmount', 'owner', 'session'] as const)(
    'rejects late snapshots after %s',
    async (cause) => {
      let finish!: (response: Response) => void
      fetchMock.mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            finish = resolve
          }),
      )
      const { stats, app, auth } = mount()
      const request = stats.refreshStats()
      if (cause === 'unmount') app.unmount()
      else if (cause === 'owner') auth.setAuthUser(owner('2'))
      else {
        sessionCoordinator.configure({
          props: { adminSessionMaintenance: { version: 1, sessionEpoch: 'A' } },
        })
        sessionCoordinator.reportFailure('session_changed')
      }
      finish(response(999, 999))
      await request
      expect(stats.counters.value).toBeNull()
      expect(stats.loading.value).toBe(false)
      expect(stats.stale.value).toBe(true)
    },
  )
  it('marks invalid or failed snapshots stale and an explicit retry replaces them from the server', async () => {
    const { stats } = mount()
    fetchMock.mockResolvedValueOnce(response(-1, 10))
    await stats.refreshStats()
    expect(stats.counters.value).toEqual({ jobs: 5, failed: 3 })
    expect(stats.error.value).toContain('Не удалось обновить счётчики')
    expect(stats.stale.value).toBe(true)
    await stats.refreshStats()
    expect(stats.counters.value).toEqual({ jobs: 9, failed: 4 })
    expect(stats.error.value).toBe('')
    expect(stats.stale.value).toBe(false)
  })
})
