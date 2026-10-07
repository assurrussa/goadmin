import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, defineComponent, nextTick, ref, type App } from 'vue'
import { useDataGrid } from './useDataGrid'

const apps: App[] = []
function mount() {
  const apiUrl = ref('/users')
  let grid!: ReturnType<typeof useDataGrid>
  const app = createApp(
    defineComponent({
      setup() {
        grid = useDataGrid({ apiUrl, initialData: { data: [] } })
        return () => null
      },
    }),
  )
  apps.push(app)
  app.mount(document.createElement('div'))
  return { grid, app, apiUrl }
}
const value = (id: number) => ({ data: [{ item: { id }, actions: [] }], config: {} })
const response = (id: number) => ({ ok: true, json: async () => value(id) }) as Response
const settle = async () => {
  for (let i = 0; i < 8; i++) await nextTick()
}
afterEach(() => {
  apps.splice(0).forEach((app) => app.unmount())
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

describe('DataGrid request lifetime', () => {
  it.each(['success', '500'])(
    'aborts superseded transport and skips outdated %s before JSON/status work',
    async (mode) => {
      const pending: ((r: Response) => void)[] = []
      const fetch = vi.fn<typeof window.fetch>(() => new Promise((r) => pending.push(r)))
      vi.stubGlobal('fetch', fetch)
      const { grid } = mount()
      const old = grid.loadData({ search: 'old' })
      const latest = grid.loadData({ search: 'new' })
      expect(fetch.mock.calls[0][1]!.signal!.aborted).toBe(true)
      pending[1](response(2))
      await latest
      const json = vi.fn(async () => value(1))
      pending[0]({ ok: mode === 'success', status: 500, json } as unknown as Response)
      await old
      expect(json).not.toHaveBeenCalled()
      expect(grid.items.value[0].item.id).toBe(2)
      expect(grid.loadError.value).toBeNull()
      expect(grid.loading.value).toBe(false)
    },
  )
  it('quietly settles an obsolete transport rejection after the newer request succeeds', async () => {
    let reject!: (error: Error) => void
    const fetch = vi
      .fn<typeof window.fetch>()
      .mockImplementationOnce(
        () =>
          new Promise((_resolve, failure) => {
            reject = failure
          }),
      )
      .mockResolvedValue(response(2))
    vi.stubGlobal('fetch', fetch)
    const { grid } = mount()
    const old = grid.loadData({ search: 'old' })
    await grid.loadData({ search: 'new' })
    reject(new DOMException('Aborted', 'AbortError'))
    await old
    expect(grid.items.value[0].item.id).toBe(2)
    expect(grid.loadError.value).toBeNull()
    expect(grid.loading.value).toBe(false)
  })
  it('keeps the sequence guard when superseded work is already decoding JSON', async () => {
    let decode!: (v: ReturnType<typeof value>) => void
    const fetch = vi
      .fn()
      .mockResolvedValueOnce({ ok: true, json: () => new Promise((r) => (decode = r)) })
      .mockResolvedValueOnce(response(2))
    vi.stubGlobal('fetch', fetch)
    const { grid } = mount()
    const old = grid.loadData()
    await settle()
    await grid.loadData({ search: 'new' })
    decode(value(1))
    await old
    expect(grid.items.value[0].item.id).toBe(2)
    expect(grid.loadError.value).toBeNull()
  })
  it.each(['unmount', 'endpoint'])('aborts and invalidates in-flight work on %s', async (mode) => {
    let resolve!: (r: Response) => void
    const fetch = vi.fn<typeof window.fetch>(() => new Promise((r) => (resolve = r)))
    vi.stubGlobal('fetch', fetch)
    const { grid, app, apiUrl } = mount()
    const pending = grid.loadData()
    if (mode === 'unmount') app.unmount()
    else apiUrl.value = '/other'
    expect(fetch.mock.calls[0][1]!.signal!.aborted).toBe(true)
    const json = vi.fn(async () => value(1))
    resolve({ ok: true, json } as unknown as Response)
    await pending
    expect(json).not.toHaveBeenCalled()
    expect(grid.items.value).toEqual([])
    expect(grid.loadError.value).toBeNull()
  })
  it('aborts as soon as a debounced edit supersedes the request and issues only the latest combined intent', async () => {
    vi.useFakeTimers()
    let reject!: (e: Error) => void
    const fetch = vi
      .fn<typeof window.fetch>()
      .mockImplementationOnce(
        (_url, options) =>
          new Promise((_r, j) => {
            reject = j
            options!.signal!.addEventListener('abort', () =>
              j(new DOMException('Aborted', 'AbortError')),
            )
          }),
      )
      .mockResolvedValue(response(2))
    vi.stubGlobal('fetch', fetch)
    const { grid } = mount()
    const pending = grid.loadData()
    grid.handleSearch('intermediate')
    grid.handleFilterChange({ status: 'active' })
    grid.handleSearch('latest')
    expect(fetch.mock.calls[0][1]!.signal!.aborted).toBe(true)
    await pending
    expect(grid.loadError.value).toBeNull()
    await vi.advanceTimersByTimeAsync(500)
    expect(fetch).toHaveBeenCalledTimes(2)
    const q = new URL(String(fetch.mock.calls[1][0]), window.location.origin).searchParams
    expect(q.get('search')).toBe('latest')
    expect(q.get('status')).toBe('active')
    expect(reject).toBeDefined()
    expect(grid.items.value[0].item.id).toBe(2)
  })
  it('does not expose an intentional AbortError as a load error', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new DOMException('Aborted', 'AbortError')))
    const { grid } = mount()
    await grid.loadData()
    expect(grid.loadError.value).toBeNull()
    expect(grid.loading.value).toBe(false)
  })
})
