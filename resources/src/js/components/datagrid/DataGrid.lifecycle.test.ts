import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, createSSRApp, defineComponent, h, nextTick, reactive, type App } from 'vue'
import { renderToString } from '@vue/server-renderer'
import DataGrid from './DataGrid.vue'
import type { ApiResponse } from '@/composables/useDataGrid'

const apps: App[] = []
const fetchMock = vi.fn<typeof fetch>()
const settle = async () => {
  for (let i = 0; i < 12; i++) await nextTick()
}
const data = (name = 'Server row', requestQuery = ''): ApiResponse => ({
  data: [{ item: { id: name, name }, actions: [] }],
  config: {
    routePath: '/users',
    columns: [{ key: 'name', label: 'Name', title: 'Name', sortable: true }],
    behaviour: { searchable: true, selectable: true, exportable: true },
  },
  meta: {
    title: 'Users',
    description: '',
    requestQuery,
    pagination: {
      currentPage: 2,
      perPage: 25,
      total: 50,
      totalPages: 2,
      from: 26,
      to: 50,
      links: [],
    },
    sorting: { sortBy: 'name', sortOrder: 'asc' },
    filters: { _search: 'Alice' },
  },
})
const response = (value: ApiResponse): Response =>
  ({ ok: true, json: async () => value }) as Response
function mount(initialData: ApiResponse | null, navigationMode: 'browser' | 'inertia' = 'browser') {
  const props = reactive({ apiUrl: '/users', initialData, syncWithUrl: true, navigationMode })
  const root = document.createElement('div')
  document.body.append(root)
  const app = createApp(defineComponent({ setup: () => () => h(DataGrid, props) }))
  apps.push(app)
  app.mount(root)
  return { props, root, app }
}
beforeEach(() => {
  window.history.replaceState({}, '', '/users')
  delete window.AdminDataGrid
  fetchMock.mockReset().mockResolvedValue(response(data()))
  vi.stubGlobal('fetch', fetchMock)
})
afterEach(() => {
  apps.splice(0).forEach((app) => app.unmount())
  document.body.innerHTML = ''
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
  vi.useRealTimers()
})

describe('server response and URL provenance', () => {
  it('hydrates Vue SSR markup without re-fetching a matching filtered page', async () => {
    const query = 'search=Alice&page=2&limit=25&sortBy=name&sortOrder=asc'
    window.history.replaceState({}, '', '/users?' + query)
    const props = { apiUrl: '/users', initialData: data('SSR row', query), syncWithUrl: true }
    const html = await renderToString(createSSRApp(DataGrid, props))
    expect(html).toContain('SSR row')
    expect(fetchMock).not.toHaveBeenCalled()
    const root = document.createElement('div')
    root.innerHTML = html
    document.body.append(root)
    const warnings: string[] = []
    const app = createSSRApp(DataGrid, props)
    app.config.warnHandler = (msg) => warnings.push(msg)
    apps.push(app)
    app.mount(root)
    await settle()
    expect(warnings).toEqual([])
    expect(root.textContent).toContain('SSR row')
    expect(fetchMock).not.toHaveBeenCalled()
  })
  it.each(['page=2&search=Alice', 'limit=1000', 'sortBy=invalid', 'search=Alice+%26+Bob'])(
    'reuses a known server-applied %s including effective server defaults',
    async (query) => {
      window.history.replaceState({}, '', '/users?' + query)
      const result = data('First client mount', query)
      mount(result)
      await settle()
      expect(fetchMock).not.toHaveBeenCalled()
      expect(new URLSearchParams(window.location.search).get('limit')).toBe('25')
      expect(new URLSearchParams(window.location.search).get('sortBy')).toBe('name')
    },
  )
  it('accepts equivalent encoding and key order without losing filters', async () => {
    window.history.replaceState({}, '', '/users?name=Alice%20%26%20Bob&page=2')
    const result = data('Canonical match', 'page=2&name=Alice+%26+Bob')
    result.meta!.filters = { name: 'Alice & Bob' }
    mount(result)
    await settle()
    expect(fetchMock).not.toHaveBeenCalled()
    expect(new URLSearchParams(window.location.search).get('name')).toBe('Alice & Bob')
  })
  it.each(['mismatch', 'legacy', 'wrong-endpoint'])(
    'loads one authoritative request for %s initial data',
    async (mode) => {
      window.history.replaceState({}, '', '/users?search=Newest')
      const result = data('Old row', 'search=Old')
      if (mode === 'legacy') delete result.meta!.requestQuery
      if (mode === 'wrong-endpoint') {
        result.meta!.requestQuery = 'search=Newest'
        result.config!.routePath = '/other'
      }
      fetchMock.mockResolvedValue(response(data('Newest row', 'search=Newest')))
      const { root } = mount(result)
      await settle()
      expect(fetchMock).toHaveBeenCalledOnce()
      expect(root.textContent).toContain('Newest row')
    },
  )
  it('fetches once for CSR without structured or global initial data', async () => {
    window.history.replaceState({}, '', '/users?name=Newest')
    mount(null)
    await settle()
    expect(fetchMock).toHaveBeenCalledOnce()
    const query = new URL(String(fetchMock.mock.calls[0][0]), window.location.origin).searchParams
    expect(query.get('name')).toBe('Newest')
    expect(query.has('limit')).toBe(false)
  })
  it('hydrates replacement Inertia props and suppresses the competing native history fetch', async () => {
    const first = data('Old Inertia page', '')
    first.meta!.filters = {}
    const { props, root } = mount(first, 'inertia')
    await settle()
    window.history.replaceState({}, '', '/users?search=Alice&page=2')
    window.dispatchEvent(new PopStateEvent('popstate'))
    props.initialData = data('Restored Inertia page', 'search=Alice&page=2')
    await settle()
    expect(fetchMock).not.toHaveBeenCalled()
    expect(root.textContent).toContain('Restored Inertia page')
    expect(root.querySelector('input')?.value).toBe('Alice')
    // Forward clearing is authoritative too, including search/filter state.
    window.history.replaceState({}, '', '/users')
    window.dispatchEvent(new PopStateEvent('popstate'))
    const forward = data('Forward page', '')
    forward.meta!.filters = {}
    props.initialData = forward
    await settle()
    expect(fetchMock).not.toHaveBeenCalled()
    expect(root.querySelector('input')?.value).toBe('')
  })
  it('aborts a pending grid fetch when redirected mutation props replace it', async () => {
    const first = data('Old page', '')
    first.meta!.filters = {}
    const { props, root } = mount(first, 'inertia')
    await settle()
    let resolve!: (value: Response) => void
    fetchMock.mockImplementationOnce(() => new Promise((r) => (resolve = r)))
    const sort = [...root.querySelectorAll<HTMLButtonElement>('th button')].find((b) =>
      b.textContent?.includes('Name'),
    )!
    sort.click()
    await settle()
    const signal = fetchMock.mock.calls[0][1]!.signal!
    const redirected = data('After mutation', window.location.search)
    redirected.meta!.filters = {}
    props.initialData = redirected
    await settle()
    expect(signal.aborted).toBe(true)
    const json = vi.fn(async () => data('Obsolete page'))
    resolve({ ok: true, json } as unknown as Response)
    await settle()
    expect(json).not.toHaveBeenCalled()
    expect(root.textContent).toContain('After mutation')
    expect(root.textContent).not.toContain('Obsolete page')
    expect(fetchMock).toHaveBeenCalledOnce()
  })
  it.each([
    ['Back', '/users?search=Destination', 'request'],
    ['Forward', '/roles?search=Destination', 'request'],
    ['Back', '/users?search=Destination', 'debounce'],
    ['Forward', '/roles?search=Destination', 'debounce'],
  ])(
    'invalidates %s %s during delayed Inertia props with pending %s',
    async (_, destination, pending) => {
      vi.useFakeTimers()
      const initial = data('Old rows', '')
      initial.meta!.filters = {}
      const { props, root } = mount(initial, 'inertia')
      await settle()
      let resolve!: (value: Response) => void
      fetchMock.mockImplementationOnce(
        () =>
          new Promise((r) => {
            resolve = r
          }),
      )
      if (pending === 'request') {
        const sort = [...root.querySelectorAll<HTMLButtonElement>('th button')].find((b) =>
          b.textContent?.includes('Name'),
        )!
        sort.click()
        await settle()
        expect(fetchMock).toHaveBeenCalledOnce()
      } else {
        const input = root.querySelector<HTMLInputElement>('input')!
        input.value = 'Obsolete pending query'
        input.dispatchEvent(new Event('input', { bubbles: true }))
        await settle()
      }
      window.history.replaceState({ page: { delayed: true } }, '', destination + '#destination')
      window.dispatchEvent(new PopStateEvent('popstate', { state: window.history.state }))
      await settle()
      if (pending === 'request') {
        expect(fetchMock.mock.calls[0][1]?.signal?.aborted).toBe(true)
        const json = vi.fn(async () => data('Obsolete response'))
        resolve({ ok: true, json } as unknown as Response)
        await settle()
        expect(json).not.toHaveBeenCalled()
      }
      await vi.advanceTimersByTimeAsync(1000)
      expect(fetchMock).toHaveBeenCalledTimes(pending === 'request' ? 1 : 0)
      expect(window.location.pathname + window.location.search + window.location.hash).toBe(
        destination + '#destination',
      )
      expect(root.textContent).not.toContain('Obsolete response')
      const restored = data('Restored destination', 'search=Destination')
      restored.config!.routePath = destination.split('?')[0]
      restored.meta!.filters = { _search: 'Destination' }
      props.apiUrl = restored.config!.routePath!
      props.initialData = restored
      await settle()
      expect(root.textContent).toContain('Restored destination')
      expect(fetchMock).toHaveBeenCalledTimes(pending === 'request' ? 1 : 0)
      expect(window.location.pathname).toBe(destination.split('?')[0])
      expect(new URLSearchParams(window.location.search).get('search')).toBe('Destination')
      expect(window.location.hash).toBe('#destination')
    },
  )
  it('does not suspend an Inertia grid for a null-state hash-only history event', async () => {
    const initial = data('Existing page', '')
    initial.meta!.filters = {}
    const { root } = mount(initial, 'inertia')
    await settle()
    const search = window.location.search
    window.history.replaceState(null, '', '/users' + search + '#section')
    window.dispatchEvent(new PopStateEvent('popstate', { state: null }))
    await settle()
    const exportButton = [...root.querySelectorAll<HTMLButtonElement>('button')].find(
      (b) => b.textContent?.trim() === 'Экспорт',
    )!
    expect(exportButton.disabled).toBe(false)
    expect(fetchMock).not.toHaveBeenCalled()
    expect(root.textContent).toContain('Existing page')
    expect(window.location.search).toBe(search)
    expect(window.location.hash).toBe('#section')
  })
})
