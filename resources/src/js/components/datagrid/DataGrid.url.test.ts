import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, defineComponent, h, nextTick, reactive, shallowRef, type App } from 'vue'
import DataGrid from './DataGrid.vue'
import type { ApiResponse, Pagination } from '../../composables/useDataGrid'

const pagination = (perPage: number, currentPage = 1): Pagination => ({
  currentPage,
  perPage,
  total: 100,
  totalPages: Math.ceil(100 / perPage),
  from: 1,
  to: perPage,
  links: [],
})
const data = (perPage = 25, id = 'row'): ApiResponse => ({
  data: [{ item: { id, name: id }, actions: [] }],
  config: {
    columns: [{ key: 'name', title: 'Name', label: 'Name', sortable: true }],
    behaviour: { searchable: true },
  },
  meta: { title: '', description: '', pagination: pagination(perPage) },
})
const response = (body: ApiResponse): Response => ({ ok: true, json: async () => body }) as Response
const apps: App[] = []
const fetchMock = vi.fn<typeof fetch>()
const settle = async () => {
  for (let i = 0; i < 8; i++) await nextTick()
}
const query = (index = 0) =>
  new URL(String(fetchMock.mock.calls[index][0]), window.location.origin).searchParams
function mount(initialData: ApiResponse | null = data(), syncWithUrl = true) {
  const props = reactive({ apiUrl: '/audit', initialData, syncWithUrl })
  const componentRef = shallowRef<{ refreshData: () => void } | null>(null)
  const app = createApp(
    defineComponent({ setup: () => () => h(DataGrid, { ...props, ref: componentRef }) }),
  )
  const element = document.createElement('div')
  document.body.append(element)
  app.mount(element)
  apps.push(app)
  return { app, props, element, componentRef }
}
beforeEach(() => {
  window.history.replaceState({}, '', '/admin')
  delete window.AdminDataGrid
  vi.spyOn(console, 'debug').mockImplementation(() => {})
  fetchMock.mockReset().mockImplementation(async (input) => {
    const params = new URL(String(input), window.location.origin).searchParams
    const result = data(Number(params.get('limit') || 25))
    result.meta!.sorting = {
      sortBy: params.get('sortBy') ?? 'host-default',
      sortOrder: params.get('sortOrder') === 'desc' ? 'desc' : 'asc',
    }
    result.meta!.pagination!.currentPage = Number(params.get('page'))
    return response(result)
  })
  vi.stubGlobal('fetch', fetchMock)
})
afterEach(() => {
  apps.splice(0).forEach((app) => app.unmount())
  document.body.innerHTML = ''
  vi.useRealTimers()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  delete window.AdminDataGrid
  window.history.replaceState({}, '', '/')
})

describe('DataGrid URL and endpoint regressions', () => {
  it('restores authoritative host defaults on bare popstate after mounting with explicit overrides', async () => {
    window.history.replaceState({}, '', '/admin?limit=10&sortBy=name&sortOrder=desc')
    const { element } = mount(data(10))
    await settle()
    expect(new URLSearchParams(window.location.search).get('limit')).toBe('10')
    window.history.replaceState({ host: 'bare' }, '', '/admin#bare')
    window.dispatchEvent(new PopStateEvent('popstate'))
    await settle()
    expect(Object.fromEntries(query(1))).toEqual({ page: '1', search: '' })
    expect(Object.fromEntries(new URLSearchParams(window.location.search))).toEqual({
      limit: '25',
      sortBy: 'host-default',
      sortOrder: 'asc',
    })
    expect(window.history.state).toEqual({ host: 'bare' })
    expect(element.textContent).not.toContain('Загрузка...')
  })

  it('lets the server resolve omitted initial URL controls while retaining explicit search and filters', async () => {
    const initial = data(25)
    initial.meta!.sorting = { sortBy: 'name', sortOrder: 'asc' }
    initial.meta!.filters = { active: 'false' }
    window.history.replaceState({}, '', '/admin?search=needle')
    mount(initial)
    await settle()
    expect(Object.fromEntries(query())).toEqual({
      page: '1',
      search: 'needle',
      active: 'false',
    })
  })
  it('retains explicit descending sort order in URLs with ascending host defaults', async () => {
    const initial = data(25)
    initial.meta!.sorting = { sortBy: 'name', sortOrder: 'asc' }
    window.history.replaceState({}, '', '/admin?sortOrder=desc')
    mount(initial)
    await settle()
    expect(query().has('sortBy')).toBe(false)
    expect(query().get('sortOrder')).toBe('desc')
    expect(new URLSearchParams(window.location.search).get('sortOrder')).toBe('desc')
  })

  it('hydrates the server page size for an initial URL with search but no limit', async () => {
    window.history.replaceState({ host: 'kept' }, '', '/admin?search=needle#section')
    mount(data(25))
    await settle()
    expect(fetchMock).toHaveBeenCalledOnce()
    expect(query().has('limit')).toBe(false)
    expect(new URLSearchParams(window.location.search).get('limit')).toBe('25')
    expect(window.location.hash).toBe('#section')
    expect(window.history.state).toEqual({ host: 'kept' })
  })
  it('omits unspecified URL limit even with globally wrapped initial data', async () => {
    window.AdminDataGrid = { dataResponse: JSON.stringify(data(25)) }
    window.history.replaceState({}, '', '/admin?search=needle')
    mount(null)
    await settle()
    expect(fetchMock).toHaveBeenCalledOnce()
    expect(query().has('limit')).toBe(false)
  })
  it('keeps explicit limit 10 in the URL even when the host initially uses 25', async () => {
    window.history.replaceState({}, '', '/admin?limit=10')
    const { componentRef } = mount(data(25))
    await settle()
    expect(new URLSearchParams(window.location.search).get('limit')).toBe('10')
    componentRef.value!.refreshData()
    await settle()
    expect(query(1).get('limit')).toBe('10')
  })
  it('preserves a nondefault limit on response synchronization and subsequent page navigation', async () => {
    window.history.replaceState({}, '', '/admin?limit=50')
    const { componentRef } = mount(data())
    await settle()
    expect(new URLSearchParams(window.location.search).get('limit')).toBe('50')
    window.history.replaceState({ host: 2 }, '', '/admin?page=2&limit=50#next')
    window.dispatchEvent(new PopStateEvent('popstate'))
    await settle()
    componentRef.value!.refreshData()
    await settle()
    expect(fetchMock).toHaveBeenCalledTimes(3)
    expect(query(1).get('page')).toBe('2')
    expect(query(2).get('limit')).toBe('50')
    expect(window.location.hash).toBe('#next')
    expect(window.history.state).toEqual({ host: 2 })
  })
  it('popstate clears omitted search, sort and filters and supersedes pending search', async () => {
    vi.useFakeTimers()
    window.history.replaceState(
      {},
      '',
      '/admin?search=old&status=on&sortBy=name&sortOrder=asc&limit=25',
    )
    const { element } = mount()
    await settle()
    const input = element.querySelector<HTMLInputElement>('input')!
    input.value = 'pending'
    input.dispatchEvent(new Event('input', { bubbles: true }))
    await settle()
    window.history.replaceState({}, '', '/admin')
    window.dispatchEvent(new PopStateEvent('popstate'))
    await settle()
    await vi.advanceTimersByTimeAsync(1000)
    expect(fetchMock).toHaveBeenCalledTimes(2)
    expect(Object.fromEntries(query(1))).toEqual({
      page: '1',
      search: '',
    })
    expect(input.value).toBe('')
  })
  it('ignores obsolete popstate responses while keeping the latest navigation and history', async () => {
    const resolvers: Array<(value: Response) => void> = []
    fetchMock.mockImplementation(() => new Promise((resolve) => resolvers.push(resolve)))
    const { element } = mount()
    await settle()
    for (const search of ['old', 'new']) {
      window.history.replaceState({ search }, '', `/admin?search=${search}&limit=25#${search}`)
      window.dispatchEvent(new PopStateEvent('popstate'))
    }
    resolvers[1](response(data(25, 'new-result')))
    await settle()
    resolvers[0](response(data(25, 'old-result')))
    await settle()
    expect(element.textContent).toContain('new-result')
    expect(element.textContent).not.toContain('old-result')
    expect(new URLSearchParams(window.location.search).get('search')).toBe('new')
    expect(window.location.hash).toBe('#new')
    expect(window.history.state).toEqual({ search: 'new' })
  })
  it('removes the popstate listener on unmount and ignores popstate with synchronization disabled', async () => {
    const first = mount()
    await settle()
    first.app.unmount()
    apps.splice(apps.indexOf(first.app), 1)
    mount(data(), false)
    window.history.replaceState({}, '', '/admin?search=ignored')
    window.dispatchEvent(new PopStateEvent('popstate'))
    await settle()
    expect(fetchMock).not.toHaveBeenCalled()
  })
  it('invalidates in-flight old endpoint data and uses the changed endpoint on refresh', async () => {
    let resolve!: (value: Response) => void
    fetchMock.mockReturnValueOnce(
      new Promise((done) => {
        resolve = done
      }),
    )
    const { props, element, componentRef } = mount(data(), false)
    await settle()
    componentRef.value!.refreshData()
    props.apiUrl = '/other'
    await settle()
    resolve(response(data(25, 'obsolete-endpoint')))
    await settle()
    expect(element.textContent).not.toContain('obsolete-endpoint')
    expect(element.textContent).not.toContain('Загрузка...')
    expect(fetchMock).toHaveBeenCalledOnce()
    componentRef.value!.refreshData()
    await settle()
    expect(String(fetchMock.mock.calls[1][0])).toMatch(/^\/other\/data\?/)
  })
  it('excludes reserved metadata filter names from URL synchronization and renders false as active', async () => {
    const initial = data(25)
    initial.data = []
    initial.meta!.filters = {
      page: '99',
      limit: '1',
      search: 'bad',
      _search: 'canonical',
      flag: false,
    }
    const { element } = mount(initial)
    await settle()
    expect(Object.fromEntries(new URLSearchParams(window.location.search))).toEqual({
      limit: '25',
      sortOrder: 'desc',
      search: 'canonical',
      flag: 'false',
    })
    expect(element.textContent).toContain('Ничего не найдено')
    expect(fetchMock).not.toHaveBeenCalled()
  })
})
