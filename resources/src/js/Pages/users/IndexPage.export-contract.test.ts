import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, defineComponent, nextTick, type App } from 'vue'
import IndexPage from './IndexPage.vue'
import type { ApiResponse } from '@/composables/useDataGrid'

const notices = vi.hoisted(() => ({ warning: vi.fn(), error: vi.fn() }))
vi.mock('@/composables/useNotifications', () => ({ useNotifications: () => notices }))
vi.mock('@inertiajs/vue3', () => ({ router: { get: vi.fn(), visit: vi.fn() } }))
vi.mock('@/components/layout/AppHead.vue', () => ({
  default: defineComponent({ render: () => null }),
}))
let app: App | undefined
const settle = async () => {
  for (let i = 0; i < 12; i++) await nextTick()
}
type UsersResponse = ApiResponse & { config: ApiResponse['config'] & { routePath: string } }
const data = (): UsersResponse => ({
  data: [{ item: { id: 1, name: 'Alice' }, actions: [] }],
  config: {
    routePath: '/users',
    columns: [{ key: 'name', title: 'Name', label: 'Name', sortable: true }],
    behaviour: { searchable: true, exportable: true },
  },
  meta: {
    title: 'Users',
    description: '',
    pagination: { currentPage: 1, perPage: 20, total: 1, totalPages: 1, from: 1, to: 1, links: [] },
  },
})
afterEach(() => {
  app?.unmount()
  app = undefined
  document.body.innerHTML = ''
  vi.runOnlyPendingTimers()
  vi.useRealTimers()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  window.history.replaceState({}, '', '/')
})

describe('users export with the real DataGrid URL contract', () => {
  it('blocks export during debounce, in-flight queries and failed synchronization', async () => {
    vi.useFakeTimers()
    let resolveRequest!: (value: Response) => void
    let rejectRequest!: (error: Error) => void
    const downloads: URL[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL) => {
        const url = new URL(String(input), window.location.origin)
        if (url.pathname.endsWith('/export')) {
          downloads.push(url)
          return Promise.resolve({
            ok: true,
            headers: new Headers({ 'content-type': 'text/csv' }),
            blob: async () => new Blob(['id,name']),
          } as Response)
        }
        return new Promise<Response>((resolve, reject) => {
          resolveRequest = resolve
          rejectRequest = reject
        })
      }),
    )
    vi.stubGlobal('URL', URL)
    URL.createObjectURL = vi.fn(() => 'blob:users')
    URL.revokeObjectURL = vi.fn()
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    const root = document.createElement('div')
    document.body.append(root)
    const pageProps = { data: data() }
    app = createApp(IndexPage, pageProps)
    app.mount(root)
    await settle()
    const exportButton = [...root.querySelectorAll<HTMLButtonElement>('button')].find((button) =>
      button.textContent?.includes('Экспорт'),
    )!
    const search = root.querySelector<HTMLInputElement>('input')!
    search.value = 'Latest query'
    search.dispatchEvent(new Event('input', { bubbles: true }))
    await settle()
    exportButton.click()
    await settle()
    expect(downloads).toHaveLength(0)
    expect(exportButton.disabled).toBe(true)
    await vi.advanceTimersByTimeAsync(500)
    exportButton.click()
    expect(downloads).toHaveLength(0)
    resolveRequest({ ok: true, json: async () => data() } as Response)
    await settle()
    expect(exportButton.disabled).toBe(false)
    exportButton.click()
    await settle()
    expect(downloads[0].searchParams.get('search')).toBe('Latest query')
    search.value = 'Failed query'
    search.dispatchEvent(new Event('input', { bubbles: true }))
    await vi.advanceTimersByTimeAsync(500)
    rejectRequest(new Error('injected query failure'))
    await settle()
    expect(exportButton.disabled).toBe(true)
    exportButton.click()
    expect(downloads).toHaveLength(1)
  })

  it('downloads interactive search, filters and sort, then respects Back/Forward clearing', async () => {
    vi.useFakeTimers()
    window.history.replaceState(
      { host: 'kept' },
      '',
      '/users?status=active&sortBy=name&sortOrder=asc#users',
    )
    const downloads: URL[] = []
    const requests: URL[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL) => {
        const url = new URL(String(input), window.location.origin)
        if (url.pathname.endsWith('/export')) {
          downloads.push(url)
          return {
            ok: true,
            headers: new Headers({
              'content-type': 'text/csv',
              'x-goadmin-export-truncated': 'true',
            }),
            blob: async () => new Blob(['id,name\n1,Alice']),
          } as Response
        }
        requests.push(url)
        const result = data()
        result.meta!.sorting = {
          sortBy: url.searchParams.get('sortBy') || 'name',
          sortOrder: url.searchParams.get('sortOrder') === 'desc' ? 'desc' : 'asc',
        }
        return { ok: true, json: async () => result } as Response
      }),
    )
    vi.stubGlobal('URL', URL)
    URL.createObjectURL = vi.fn(() => 'blob:users')
    URL.revokeObjectURL = vi.fn()
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    const root = document.createElement('div')
    document.body.append(root)
    const pageProps = { data: data() }
    app = createApp(IndexPage, pageProps)
    app.mount(root)
    await settle()
    const search = root.querySelector<HTMLInputElement>('input')!
    search.value = 'Alice & Bob'
    search.dispatchEvent(new Event('input', { bubbles: true }))
    await vi.advanceTimersByTimeAsync(500)
    await settle()
    const sort = [...root.querySelectorAll<HTMLButtonElement>('th button')].find((button) =>
      button.textContent?.includes('Name'),
    )!
    expect(sort).toBeDefined()
    sort.click()
    await settle()
    const exportButton = [...root.querySelectorAll<HTMLButtonElement>('button')].find((button) =>
      button.textContent?.includes('Экспорт'),
    )!
    exportButton.click()
    await settle()
    expect(Object.fromEntries(downloads[0].searchParams)).toEqual({
      sortBy: 'name',
      sortOrder: 'desc',
      search: 'Alice & Bob',
      status: 'active',
    })
    expect(downloads[0].hash).toBe('')
    expect(click).toHaveBeenCalledOnce()
    expect(notices.warning).toHaveBeenCalledWith(expect.stringContaining('10 000'))
    expect(window.history.state).toEqual({ host: 'kept' })
    window.history.replaceState({ host: 'back' }, '', '/users')
    window.dispatchEvent(new PopStateEvent('popstate'))
    await settle()
    exportButton.click()
    await settle()
    expect(downloads).toHaveLength(2)
    expect(downloads[1].searchParams.has('status')).toBe(false)
    expect(downloads[1].searchParams.has('search')).toBe(false)
    expect(downloads[1].searchParams.has('limit')).toBe(false)
    expect(downloads[1].searchParams.has('page')).toBe(false)
    expect(requests.at(-1)!.searchParams.get('search')).toBe('')
    expect(notices.error).not.toHaveBeenCalled()
    expect(document.querySelector('a[download]')).toBeNull()
  })
})
