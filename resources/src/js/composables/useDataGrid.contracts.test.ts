import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, defineComponent, type App } from 'vue'
import { useDataGrid, type ApiResponse, type Pagination } from './useDataGrid'

const page = (overrides: Partial<Pagination> = {}): Pagination => ({
  currentPage: 2,
  perPage: 20,
  total: 60,
  totalPages: 3,
  from: 21,
  to: 40,
  links: [],
  ...overrides,
})
const fixture = (overrides: Partial<ApiResponse> = {}): ApiResponse => ({
  data: [
    { item: { id: 0, name: 'Zero' }, actions: [] },
    { item: { id: 'uuid-2', name: 'Second' }, actions: [] },
  ],
  meta: {
    title: 'Records',
    description: 'Fixture',
    pagination: page(),
    sorting: { sortBy: 'name', sortOrder: 'asc' },
    filters: { status: 'active' },
  },
  config: {
    columns: [
      { key: 'name', label: 'Name', title: 'Name', sortable: true, filterable: true },
      { key: 'age', label: 'Age', title: 'Age', sortable: true },
      { key: 'static', label: 'Static', title: 'Static' },
    ],
    ui: { idKey: 'id' },
  },
  ...overrides,
})
const response = (
  data: ApiResponse = fixture({
    meta: { title: 'Records', description: 'Fixture', pagination: page() },
  }),
): Response => ({ ok: true, json: async () => data }) as Response
const deferred = <T>() => {
  let resolve!: (value: T) => void
  let reject!: (error: Error) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}
const apps: App[] = []
function mount(initialData: ApiResponse | null = fixture(), apiUrl = '/audit') {
  let grid!: ReturnType<typeof useDataGrid>
  const app = createApp(
    defineComponent({
      setup() {
        grid = useDataGrid({ apiUrl, initialData })
        return () => null
      },
    }),
  )
  app.mount(document.createElement('div'))
  apps.push(app)
  return { grid, app }
}
const fetchMock = vi.fn<typeof fetch>()
const url = (index = 0) => new URL(String(fetchMock.mock.calls[index][0]), window.location.origin)
const settle = async () => {
  for (let i = 0; i < 6; i++) await Promise.resolve()
}
beforeEach(() => {
  fetchMock.mockReset().mockResolvedValue(response())
  vi.stubGlobal('fetch', fetchMock)
  delete window.AdminDataGrid
})
afterEach(() => {
  apps.splice(0).forEach((app) => app.unmount())
  vi.useRealTimers()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  delete window.AdminDataGrid
})

describe('useDataGrid passing contracts: initialization and response', () => {
  it('hydrates explicit initial rows, meta pagination, config, sorting and filters without fetching', () => {
    const { grid } = mount()
    expect(fetchMock).not.toHaveBeenCalled()
    expect(grid.hasLoaded.value).toBe(true)
    expect(grid.items.value.map((entry) => entry.item.id)).toEqual([0, 'uuid-2'])
    expect(grid.pagination.value).toEqual(page())
    expect(grid.sortBy.value).toBe('name')
    expect(grid.sortOrder.value).toBe('asc')
    expect(grid.filters.value).toEqual({ status: 'active' })
    expect(grid.searchQuery.value).toBe('')
    expect(grid.filterableColumns.value.map((col) => col.key)).toEqual(['name'])
    expect(grid.hasFilters.value).toBe(true)
  })
  it('treats an empty initial array as successfully loaded and does not fetch', () => {
    const { grid } = mount({ data: [] })
    expect(grid.hasLoaded.value).toBe(true)
    expect(grid.items.value).toEqual([])
    expect(grid.pagination.value).toBeNull()
    expect(grid.config.value).toEqual({})
    expect(grid.sortOrder.value).toBe('desc')
    expect(grid.hasFilters.value).toBe(false)
    expect(fetchMock).not.toHaveBeenCalled()
  })
  it.each(['object', 'json'] as const)('hydrates window.AdminDataGrid in %s form', (kind) => {
    window.AdminDataGrid = kind === 'json' ? JSON.stringify(fixture()) : fixture()
    const { grid } = mount(null)
    expect(grid.items.value).toHaveLength(2)
    expect(fetchMock).not.toHaveBeenCalled()
  })
  it('prefers explicit initialData over window.AdminDataGrid', () => {
    window.AdminDataGrid = fixture({ data: [{ item: { id: 'global' }, actions: [] }] })
    expect(mount().grid.items.value[0].item.id).toBe(0)
  })
  it('falls back to one GET with default parameters for missing initial data', async () => {
    const { grid } = mount(null)
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(fetchMock.mock.calls[0]).toHaveLength(1)
    expect(url().pathname).toBe('/audit/data')
    expect(Object.fromEntries(url().searchParams)).toEqual({
      page: '1',
      limit: '10',
      sortBy: '',
      sortOrder: 'desc',
      search: '',
    })
    expect(grid.loading.value).toBe(true)
    await settle()
    expect(grid.hasLoaded.value).toBe(true)
    expect(grid.loading.value).toBe(false)
  })
  it('logs malformed global JSON and fetches instead', async () => {
    const log = vi.spyOn(console, 'error').mockImplementation(() => {})
    window.AdminDataGrid = '{invalid'
    const { grid } = mount(null)
    await settle()
    expect(log).toHaveBeenCalledOnce()
    expect(fetchMock).toHaveBeenCalledOnce()
    expect(grid.hasLoaded.value).toBe(true)
  })
  it('prefers meta.pagination over a conflicting top-level pagination on fetch', async () => {
    fetchMock.mockResolvedValue(response(fixture({ pagination: page({ currentPage: 1 }) })))
    const { grid } = mount()
    await grid.loadData()
    expect(grid.pagination.value?.currentPage).toBe(2)
  })
  it('preserves local intent when a fetched response has no canonical sorting or filters', async () => {
    const { grid } = mount()
    grid.toggleSelectItem(0)
    fetchMock.mockResolvedValue(response({ data: [] }))
    await grid.loadData({
      search: 'current',
      filters: { local: 0 },
      sortBy: 'age',
      sortOrder: 'desc',
    })
    expect(grid.items.value).toEqual([])
    expect(grid.selectedItems.value).toEqual([])
    expect(grid.metaInfo.value).toBeNull()
    expect(grid.config.value).toEqual({})
    expect(grid.pagination.value).toBeNull()
    expect(grid.searchQuery.value).toBe('current')
    expect(grid.filters.value).toEqual({ local: 0 })
    expect(grid.sortBy.value).toBe('age')
  })
  it('treats a response object missing data as an empty successful result', async () => {
    fetchMock.mockResolvedValue(response({} as ApiResponse))
    const { grid } = mount()
    await grid.loadData()
    expect(grid.items.value).toEqual([])
    expect(grid.hasLoaded.value).toBe(true)
    expect(grid.loadError.value).toBeNull()
  })
})

describe('useDataGrid passing contracts: requests, sorting and pagination', () => {
  it('serializes zero/false and URL-sensitive characters while omitting blank/null/undefined filters', async () => {
    const { grid } = mount()
    await grid.loadData({
      filters: { zero: 0, flag: false, empty: '', nil: null, missing: undefined, text: 'a & b' },
    })
    expect(Object.fromEntries(url().searchParams)).toEqual({
      page: '2',
      limit: '20',
      sortBy: 'name',
      sortOrder: 'asc',
      search: '',
      zero: '0',
      flag: 'false',
      text: 'a & b',
    })
  })
  it('allows explicit empty search, filters and sortBy to clear current state', async () => {
    const { grid } = mount()
    await grid.loadData({ search: 'old' })
    await grid.loadData({ search: '', filters: {}, sortBy: '' })
    expect(url(1).searchParams.get('search')).toBe('')
    expect(url(1).searchParams.get('sortBy')).toBe('')
    expect(url(1).searchParams.has('status')).toBe(false)
  })
  it('sorts a new column ascending then descending and retains page/search/filters', async () => {
    const { grid } = mount()
    grid.searchQuery.value = 'needle'
    grid.handleSort('age')
    expect(fetchMock).toHaveBeenCalledOnce()
    expect(Object.fromEntries(url().searchParams)).toEqual({
      page: '2',
      limit: '20',
      sortBy: 'age',
      sortOrder: 'asc',
      search: 'needle',
      status: 'active',
    })
    await settle()
    grid.handleSort('age')
    expect(fetchMock).toHaveBeenCalledTimes(2)
    expect(url(1).searchParams.get('sortOrder')).toBe('desc')
  })
  it.each(['static', 'missing'])('does not fetch for non-sortable or unknown column %s', (key) => {
    const { grid } = mount()
    grid.handleSort(key)
    expect(fetchMock).not.toHaveBeenCalled()
    expect(grid.sortBy.value).toBe('name')
  })
  it.each([0, -1, 4])(
    'rejects out-of-range numeric page %s without updating or fetching',
    (target) => {
      const { grid } = mount()
      grid.goToPage(target)
      expect(grid.pagination.value?.currentPage).toBe(2)
      expect(fetchMock).not.toHaveBeenCalled()
    },
  )
  it('loads a valid numeric page once and optimistically updates current page', () => {
    const { grid } = mount()
    grid.goToPage(3)
    expect(grid.pagination.value?.currentPage).toBe(3)
    expect(fetchMock).toHaveBeenCalledOnce()
    expect(url().searchParams.get('page')).toBe('3')
    expect(url().searchParams.get('status')).toBe('active')
  })
  it('parses URL parameters but fetches its own apiUrl rather than supplied host/path', () => {
    const { grid } = mount()
    grid.goToPageByUrl(
      'https://example.invalid/other?page=3&limit=5&sortBy=age&sortOrder=desc&search=x&status=off',
    )
    expect(fetchMock).toHaveBeenCalledOnce()
    expect(url().pathname).toBe('/audit/data')
    expect(Object.fromEntries(url().searchParams)).toEqual({
      page: '3',
      limit: '5',
      sortBy: 'age',
      sortOrder: 'desc',
      search: 'x',
      status: 'off',
    })
  })
  it('URL pagination clears unspecified search/sort/filters and retains the current page size', () => {
    const { grid } = mount()
    grid.goToPageByUrl('/audit?page=1')
    expect(Object.fromEntries(url().searchParams)).toEqual({
      page: '1',
      limit: '20',
      sortBy: '',
      sortOrder: 'desc',
      search: '',
    })
    expect(grid.filters.value).toEqual({})
  })
  it('ignores blank URL pagination and catches malformed URLs without fetching', () => {
    const log = vi.spyOn(console, 'error').mockImplementation(() => {})
    const { grid } = mount()
    grid.goToPageByUrl('')
    grid.goToPageByUrl('http://[')
    expect(fetchMock).not.toHaveBeenCalled()
    expect(log).toHaveBeenCalledOnce()
  })
})

describe('useDataGrid passing contracts: debounce, races and lifecycle', () => {
  beforeEach(() => vi.useFakeTimers())
  it('debounces search for 500ms, sends only latest query, and requests page 1', async () => {
    const { grid } = mount()
    grid.handleSearch('a')
    expect(grid.loading.value).toBe(true)
    await vi.advanceTimersByTimeAsync(250)
    grid.handleSearch('ab')
    await vi.advanceTimersByTimeAsync(499)
    expect(fetchMock).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(1)
    expect(fetchMock).toHaveBeenCalledOnce()
    expect(url().searchParams.get('search')).toBe('ab')
    expect(url().searchParams.get('page')).toBe('1')
  })
  it('merges filter patches immediately and debounces 300ms to newest merged object', async () => {
    const { grid } = mount()
    grid.handleFilterChange({ status: 'off' })
    await vi.advanceTimersByTimeAsync(200)
    grid.handleFilterChange({ zero: 0 })
    expect(grid.filters.value).toEqual({ status: 'off', zero: 0 })
    await vi.advanceTimersByTimeAsync(299)
    expect(fetchMock).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(1)
    expect(fetchMock).toHaveBeenCalledOnce()
    expect(url().searchParams.get('status')).toBe('off')
    expect(url().searchParams.get('zero')).toBe('0')
    expect(url().searchParams.get('page')).toBe('1')
  })
  it('invalidates an in-flight response as soon as newer debounced intent is entered', async () => {
    const pending = deferred<Response>()
    fetchMock.mockReturnValueOnce(pending.promise)
    const { grid } = mount()
    const loading = grid.loadData({ search: 'old' })
    grid.handleSearch('new')
    pending.resolve(response(fixture({ data: [{ item: { id: 'stale' }, actions: [] }] })))
    await loading
    expect(grid.items.value[0].item.id).toBe(0)
    expect(grid.loading.value).toBe(true)
    await vi.advanceTimersByTimeAsync(500)
    expect(grid.searchQuery.value).toBe('new')
    expect(grid.loading.value).toBe(false)
  })
  it('ignores older errors while latest request remains pending', async () => {
    const old = deferred<Response>(),
      current = deferred<Response>()
    fetchMock.mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise)
    const { grid } = mount()
    const first = grid.loadData({ search: 'old' }),
      second = grid.loadData({ search: 'current' })
    old.reject(new Error('stale failure'))
    await first
    expect(grid.loadError.value).toBeNull()
    expect(grid.loading.value).toBe(true)
    current.resolve(response())
    await second
    expect(grid.loading.value).toBe(false)
  })
  it('cancels both pending debounce timers on unmount', async () => {
    const { grid, app } = mount()
    grid.handleSearch('abandoned')
    grid.handleFilterChange({ status: 'abandoned' })
    app.unmount()
    apps.splice(apps.indexOf(app), 1)
    await vi.advanceTimersByTimeAsync(1000)
    expect(fetchMock).not.toHaveBeenCalled()
  })
  it('ignores in-flight success after unmount and does not attach an AbortSignal to fetch', async () => {
    const pending = deferred<Response>()
    fetchMock.mockReturnValueOnce(pending.promise)
    const { grid, app } = mount()
    const task = grid.loadData()
    app.unmount()
    apps.splice(apps.indexOf(app), 1)
    pending.resolve(response(fixture({ data: [{ item: { id: 'late' }, actions: [] }] })))
    await task
    expect(grid.items.value[0].item.id).toBe(0)
    expect(fetchMock.mock.calls[0]).toHaveLength(1)
  })
})

describe('useDataGrid passing contracts: errors and retry', () => {
  it.each([
    [401, 'Сеанс истёк'],
    [403, 'нет доступа'],
    [500, 'Не удалось загрузить данные'],
  ])('preserves prior rows/selection on HTTP %s', async (status, message) => {
    fetchMock.mockResolvedValue({ ok: false, status } as Response)
    const { grid } = mount()
    grid.toggleSelectItem(0)
    await grid.loadData()
    expect(grid.items.value).toHaveLength(2)
    expect(grid.selectedItems.value).toEqual([0])
    expect(grid.loadError.value).toContain(message)
    expect(grid.hasLoaded.value).toBe(true)
    expect(grid.loading.value).toBe(false)
  })
  it('keeps hasLoaded false after failed first load', async () => {
    fetchMock.mockRejectedValue(new Error('offline'))
    const { grid } = mount(null)
    await settle()
    expect(grid.hasLoaded.value).toBe(false)
    expect(grid.loadError.value).toBe('offline')
  })
  it('reports malformed JSON without replacing data', async () => {
    fetchMock.mockResolvedValue({
      ok: true,
      json: async () => {
        throw new SyntaxError('bad JSON')
      },
    } as unknown as Response)
    const { grid } = mount()
    await grid.loadData()
    expect(grid.loadError.value).toBe('bad JSON')
    expect(grid.items.value).toHaveLength(2)
  })
  it('retries exact explicit parameters with a copied filters snapshot', async () => {
    fetchMock.mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce(response())
    const { grid } = mount(),
      filters = { status: 'original' }
    await grid.loadData({
      page: 3,
      limit: 5,
      search: 'retry',
      sortBy: 'age',
      sortOrder: 'desc',
      filters,
    })
    filters.status = 'mutated'
    await grid.retryLoad()
    expect(fetchMock).toHaveBeenCalledTimes(2)
    expect(fetchMock.mock.calls[1][0]).toBe(fetchMock.mock.calls[0][0])
    expect(grid.loadError.value).toBeNull()
  })
  it('uses a generic message for non-Error rejection', async () => {
    fetchMock.mockRejectedValue('offline')
    const { grid } = mount()
    await grid.loadData()
    expect(grid.loadError.value).toBe('Не удалось загрузить данные.')
  })
})

describe('useDataGrid passing contracts: selection', () => {
  it('selects and clears all visible string/numeric IDs including zero', () => {
    const { grid } = mount()
    expect(grid.allSelected.value).toBe(false)
    grid.toggleSelectAll()
    expect(grid.selectedItems.value).toEqual([0, 'uuid-2'])
    expect(grid.allSelected.value).toBe(true)
    grid.toggleSelectAll()
    expect(grid.selectedItems.value).toEqual([])
  })
  it('toggles individual IDs with strict number/string identity', () => {
    const { grid } = mount()
    grid.toggleSelectItem(0)
    grid.toggleSelectItem('0')
    expect(grid.selectedItems.value).toEqual([0, '0'])
    grid.toggleSelectItem(0)
    expect(grid.selectedItems.value).toEqual(['0'])
  })
  it('uses idKey and excludes missing/null/object IDs from select-all', () => {
    const { grid } = mount(
      fixture({
        config: { ui: { idKey: 'uuid' } },
        data: ['a', 0, null, undefined, {}].map((uuid) => ({ item: { uuid }, actions: [] })),
      }),
    )
    grid.toggleSelectAll()
    expect(grid.selectedItems.value).toEqual(['a', 0])
    expect(grid.allSelected.value).toBe(false)
  })
  it('does not claim all selected for an empty data set', () => {
    const { grid } = mount({ data: [] })
    grid.toggleSelectAll()
    expect(grid.allSelected.value).toBe(false)
  })
})

describe('useDataGrid regression fixes and remaining behavior boundaries', () => {
  it('accepts identical top-level pagination during initialData and fetch', async () => {
    const data = fixture({ meta: undefined, pagination: page() })
    const { grid } = mount(data)
    expect(grid.pagination.value).toEqual(page())
    fetchMock.mockResolvedValue(response(data))
    await grid.loadData()
    expect(grid.pagination.value).toEqual(page())
  })
  it('unwraps the declared window dataResponse wrapper without a redundant fetch', () => {
    window.AdminDataGrid = { dataResponse: JSON.stringify(fixture()) }
    const { grid } = mount(null)
    expect(grid.items.value).toHaveLength(2)
    expect(fetchMock).not.toHaveBeenCalled()
  })
  it('reloads the already-current numeric page', () => {
    mount().grid.goToPage(2)
    expect(fetchMock).toHaveBeenCalledOnce()
  })
  it('leaves positive URL page bounds to the server and normalizes invalid sortOrder', () => {
    mount().grid.goToPageByUrl('/audit?page=999&sortOrder=sideways')
    expect(url().searchParams.get('page')).toBe('999')
    expect(url().searchParams.get('sortOrder')).toBe('desc')
  })
  it('prevents filter keys from overwriting reserved query parameters', async () => {
    await mount().grid.loadData({
      page: 1,
      search: 'intended',
      filters: {
        page: 99,
        limit: 1,
        sortBy: 'override',
        sortOrder: 'override',
        search: 'override',
        _search: 'override',
        status: 'active',
      },
    })
    expect(Object.fromEntries(url().searchParams)).toEqual({
      page: '1',
      limit: '20',
      sortBy: 'name',
      sortOrder: 'asc',
      search: 'intended',
      status: 'active',
    })
  })
  it('coalesces search and filter edits into one request using the latest combined state', async () => {
    vi.useFakeTimers()
    const { grid } = mount()
    grid.handleSearch('needle')
    grid.handleFilterChange({ status: 'off' })
    await vi.advanceTimersByTimeAsync(300)
    expect(fetchMock).toHaveBeenCalledOnce()
    expect(url().searchParams.get('search')).toBe('needle')
    expect(url().searchParams.get('status')).toBe('off')
    await vi.advanceTimersByTimeAsync(200)
    expect(fetchMock).toHaveBeenCalledOnce()
  })
  it('cancels pending search when newer page navigation requests the latest query', async () => {
    vi.useFakeTimers()
    const { grid } = mount()
    grid.handleSearch('pending')
    grid.goToPage(3)
    expect(url().searchParams.get('page')).toBe('3')
    await vi.advanceTimersByTimeAsync(500)
    expect(fetchMock).toHaveBeenCalledOnce()
    expect(url().searchParams.get('search')).toBe('pending')
  })
  it('does not claim allSelected for unrelated IDs with an equal count', () => {
    const { grid } = mount()
    grid.toggleSelectItem('not-visible-1')
    grid.toggleSelectItem('not-visible-2')
    expect(grid.allSelected.value).toBe(false)
  })
  it('deduplicates row identifiers in select-all', () => {
    const { grid } = mount(fixture({ data: [1, 1].map((id) => ({ item: { id }, actions: [] })) }))
    grid.toggleSelectAll()
    expect(grid.selectedItems.value).toEqual([1])
    expect(grid.allSelected.value).toBe(true)
  })
})

describe('useDataGrid fixed regression guards', () => {
  it('initialData should accept the same top-level pagination envelope as fetch', () => {
    expect(mount(fixture({ meta: undefined, pagination: page() })).grid.pagination.value).toEqual(
      page(),
    )
  })
  it('allSelected should require membership of every visible row ID', () => {
    const { grid } = mount()
    grid.toggleSelectItem('not-visible-1')
    grid.toggleSelectItem('not-visible-2')
    expect(grid.allSelected.value).toBe(false)
  })
})

describe('useDataGrid query intent and metadata regressions', () => {
  it('hydrates canonical string filters and global search identically from initial and fetched metadata', async () => {
    const canonical = fixture({
      meta: {
        title: 'Canonical',
        description: '',
        pagination: page({ perPage: 25 }),
        sorting: { sortBy: 'age', sortOrder: 'desc' },
        filters: { count: '0', active: 'false', date: '2026-10-02', _search: 'needle', page: '99' },
      },
    })
    const { grid } = mount(canonical)
    expect(grid.searchQuery.value).toBe('needle')
    expect(grid.filters.value).toEqual({ count: '0', active: 'false', date: '2026-10-02' })
    grid.searchQuery.value = 'changed'
    grid.filters.value = { local: 'pending' }
    fetchMock.mockResolvedValue(response(canonical))
    await grid.loadData()
    expect(grid.searchQuery.value).toBe('needle')
    expect(grid.filters.value).toEqual({ count: '0', active: 'false', date: '2026-10-02' })
    expect(grid.sortBy.value).toBe('age')
    expect(grid.pagination.value?.perPage).toBe(25)
    await grid.loadData()
    expect(url(1).searchParams.get('search')).toBe('needle')
    expect(url(1).searchParams.has('_search')).toBe(false)
    expect(url(1).searchParams.get('page')).toBe('2')
  })
  it('coalesces filter then search while honoring the latest search debounce', async () => {
    vi.useFakeTimers()
    const { grid } = mount()
    grid.handleFilterChange({ status: 'off' })
    await vi.advanceTimersByTimeAsync(200)
    grid.handleSearch('newest')
    await vi.advanceTimersByTimeAsync(499)
    expect(fetchMock).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(1)
    expect(fetchMock).toHaveBeenCalledOnce()
    expect(url().searchParams.get('search')).toBe('newest')
    expect(url().searchParams.get('status')).toBe('off')
  })
  it.each(['sort', 'url', 'refresh'] as const)(
    'cancels an older pending filter when a newer %s intent runs',
    async (intent) => {
      vi.useFakeTimers()
      const { grid } = mount()
      grid.handleFilterChange({ status: 'pending' })
      if (intent === 'sort') grid.handleSort('age')
      else if (intent === 'url') grid.goToPageByUrl('/audit?page=3&search=url&status=url')
      else void grid.loadData()
      expect(fetchMock).toHaveBeenCalledOnce()
      await vi.advanceTimersByTimeAsync(1000)
      expect(fetchMock).toHaveBeenCalledOnce()
      expect(url().searchParams.get('status')).toBe(intent === 'url' ? 'url' : 'pending')
      expect(url().searchParams.get('page')).toBe(intent === 'url' ? '3' : '2')
    },
  )
  it('retries an implicit request using the complete original state snapshot', async () => {
    fetchMock.mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce(response())
    const { grid } = mount()
    await grid.loadData()
    grid.searchQuery.value = 'unsent'
    grid.filters.value.status = 'unsent'
    grid.sortBy.value = 'age'
    await grid.retryLoad()
    expect(fetchMock.mock.calls[1][0]).toBe(fetchMock.mock.calls[0][0])
  })
  it.each(['0', '-1', '1.5', '12tail', 'Infinity', '9007199254740992'])(
    'normalizes invalid URL page and limit %s without forwarding it',
    (value) => {
      const { grid } = mount()
      grid.goToPageByUrl(`/audit?page=${value}&limit=${value}`)
      expect(url().searchParams.get('page')).toBe('1')
      expect(url().searchParams.get('limit')).toBe('20')
    },
  )
  it.each([NaN, Infinity, 1.5])('ignores invalid numeric navigation %s', (value) => {
    mount().grid.goToPage(value)
    expect(fetchMock).not.toHaveBeenCalled()
  })
  it('does not allow a late stale response to rehydrate canonical metadata over a newer query', async () => {
    const pending = deferred<Response>()
    fetchMock.mockReturnValueOnce(pending.promise)
    const { grid } = mount()
    const old = grid.loadData()
    await grid.loadData({ search: 'latest', filters: { count: 0 } })
    pending.resolve(
      response(
        fixture({ meta: { title: '', description: '', filters: { _search: 'old', count: '99' } } }),
      ),
    )
    await old
    expect(grid.searchQuery.value).toBe('latest')
    expect(grid.filters.value).toEqual({ count: 0 })
  })
})

describe('useDataGrid URL navigation default controls', () => {
  it('omits absent transport controls on URL navigation and repeats the same omissions on retry', async () => {
    fetchMock.mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce(response())
    const { grid } = mount()
    await grid.loadDataFromUrl('/admin')
    expect(Object.fromEntries(url().searchParams)).toEqual({ page: '1', search: '' })
    expect(grid.items.value).toHaveLength(2)
    await grid.retryLoad()
    expect(fetchMock.mock.calls[1][0]).toBe(fetchMock.mock.calls[0][0])
    await grid.loadData()
    expect(url(2).searchParams.get('limit')).toBe('20')
    expect(url(2).searchParams.get('sortBy')).toBe('name')
  })
})
