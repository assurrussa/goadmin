import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, defineComponent, nextTick, type App } from 'vue'
import { useDataGrid } from './useDataGrid'

function mountGrid() {
  let grid!: ReturnType<typeof useDataGrid>
  const app: App = createApp(
    defineComponent({
      setup() {
        grid = useDataGrid({ apiUrl: '/admins', initialData: { data: [], config: {} } })
        return () => null
      },
    }),
  )
  const element = document.createElement('div')
  app.mount(element)
  return { grid, app }
}

function response(id: number): Response {
  return {
    ok: true,
    json: async () => ({ data: [{ item: { id }, actions: [] }], config: {} }),
  } as Response
}

afterEach(() => vi.unstubAllGlobals())

describe('useDataGrid loading', () => {
  it('ignores a late response from an older request', async () => {
    const pending: ((response: Response) => void)[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn(() => new Promise<Response>((resolve) => pending.push(resolve))),
    )
    const { grid, app } = mountGrid()
    const first = grid.loadData({ search: 'old' })
    const second = grid.loadData({ search: 'new' })
    pending[1](response(2))
    await second
    pending[0](response(1))
    await first
    await nextTick()
    expect(grid.items.value[0].item.id).toBe(2)
    expect(grid.searchQuery.value).toBe('new')
    expect(grid.loading.value).toBe(false)
    app.unmount()
  })

  it('keeps loaded rows and exposes a useful authorization error', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(response(1))
      .mockResolvedValueOnce({ ok: false, status: 403 })
      .mockResolvedValueOnce(response(2))
    vi.stubGlobal('fetch', fetchMock)
    const { grid, app } = mountGrid()
    await grid.loadData()
    await grid.loadData({ search: 'restricted' })
    expect(grid.items.value[0].item.id).toBe(1)
    expect(grid.loadError.value).toContain('нет доступа')
    await grid.retryLoad()
    expect(String(fetchMock.mock.calls[2][0])).toContain('search=restricted')
    expect(grid.items.value[0].item.id).toBe(2)
    expect(grid.loadError.value).toBeNull()
    app.unmount()
  })
})
