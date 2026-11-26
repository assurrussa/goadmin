import { afterEach, expect, it, vi } from 'vitest'
import { createApp, defineComponent, nextTick } from 'vue'
import Index from './Index.vue'
import Show from './Show.vue'
import type { CrudResource } from '@/crud'
import plainResource from './no-business-actions.test.json'
import type { ApiResponse } from '@/composables/useDataGrid'
import { applyLocale } from '@/i18n'

const mocks = vi.hoisted(() => ({ confirm: vi.fn(), post: vi.fn() }))
vi.mock('@inertiajs/vue3', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@inertiajs/vue3')>()),
  usePage: () => ({ props: {}, url: '/people/records/42' }),
  router: { on: () => () => {}, get: vi.fn(), post: mocks.post },
  Head: defineComponent({ render: () => null }),
}))
vi.mock('@/composables/useConfirmation', () => ({ useConfirmation: () => mocks.confirm }))
let app: ReturnType<typeof createApp> | undefined
afterEach(() => {
  app?.unmount()
  app = undefined
  document.body.replaceChildren()
  window.history.replaceState({}, '', '/')
  applyLocale('ru')
  vi.clearAllMocks()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})
const settle = async () => {
  for (let i = 0; i < 16; i++) await nextTick()
}

it('opens a journal event from its filtered list without selection or export controls', async () => {
  const basePath = '/product-events'
  const query = 'search=17'
  window.history.replaceState({}, '', basePath + '?' + query)
  const fetch = vi.fn().mockResolvedValue({
    ok: true,
    json: async () => ({
      owner: '7',
      version: 0,
      payload: { schemaVersion: 1, columns: ['name'], views: [] },
    }),
  })
  vi.stubGlobal('fetch', fetch)
  const id = '9007199254740993'
  const root = document.createElement('div')
  document.body.append(root)
  const props = {
    resource: { key: 'product_events', basePath, title: 'Events', fields: [], actions: [] },
    data: {
      data: [
        {
          item: { id, name: 'Product 17' },
          selectionId: id,
          actions: [{ key: 'view', label: 'Просмотр' }],
        },
      ],
      config: {
        routePath: basePath,
        columns: [{ key: 'name', label: 'Name', title: 'Name' }],
        workflow: { key: 'product_events', preferencesUrl: basePath + '/table-preferences' },
      },
      meta: { title: 'Events', description: '', requestQuery: query, filters: { _search: '17' } },
    },
  }
  app = createApp(Index, props)
  app.mount(root)
  await settle()
  expect(root.textContent).toContain('Product 17')
  expect(root.textContent).toContain('Сохранить столбцы')
  expect(root.textContent).not.toMatch(/Выбрано:|CSV|Снять выделение/)
  expect(root.querySelector('table [role="checkbox"], table input[type="checkbox"]')).toBeNull()
  const link = root.querySelector<HTMLAnchorElement>('a[aria-label="Просмотр"]')!
  const href = new URL(link.href)
  expect(href.pathname).toBe(basePath + '/records/' + id)
  expect(href.searchParams.get('returnTo')).toBe(window.location.pathname + window.location.search)

  const { router } = await vi.importActual<typeof import('@inertiajs/vue3')>('@inertiajs/vue3')
  const visit = vi.spyOn(router, 'visit').mockImplementation(() => {})
  const clicks: boolean[] = []
  root.addEventListener('click', (event) => {
    clicks.push(event.defaultPrevented)
    event.preventDefault() // jsdom does not implement the native tab navigation.
  })
  for (const gesture of [{ ctrlKey: true }, { metaKey: true }, { button: 1 }, {}]) {
    link.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true, ...gesture }))
  }
  expect(clicks).toEqual([false, false, false, true])
  expect(visit.mock.calls.map(([target]) => target)).toEqual([link.getAttribute('href')])
})

it('runs a row command named show without replacing it with a detail link', async () => {
  const basePath = '/requests'
  const query = 'sortOrder=desc&search=Alice'
  const list = `${basePath}?${query}`
  window.history.replaceState({}, '', list)
  const root = document.createElement('div')
  document.body.append(root)
  const command = { key: 'show', label: 'Show in directory' }
  const data: ApiResponse = {
    data: [
      {
        item: { id: '42', name: 'Alice', _revision: 'v1' },
        actions: [{ key: 'view', label: 'Просмотр' }, command],
      },
    ],
    config: { columns: [{ key: 'name', label: 'Name', title: 'Name' }] },
    meta: { title: '', description: '', requestQuery: query, filters: { _search: 'Alice' } },
  }
  app = createApp(Index, {
    resource: { key: 'requests', basePath, title: 'Requests', fields: [], actions: [command] },
    data,
  })
  app.mount(root)
  await settle()

  expect(root.querySelector('a[aria-label="Просмотр"]')).not.toBeNull()
  const button = root.querySelector<HTMLButtonElement>('button[aria-label="Show in directory"]')
  expect(button).not.toBeNull()
  mocks.confirm.mockImplementationOnce((_options, accept) => accept())
  button!.click()
  expect(mocks.confirm).toHaveBeenCalledWith(
    expect.objectContaining({ confirmLabel: command.label }),
    expect.any(Function),
  )
  expect(mocks.post).toHaveBeenCalledWith(
    `${basePath}/records/42/actions/show?${new URLSearchParams({ returnTo: list })}`,
    { _revision: 'v1' },
    expect.any(Object),
  )
})

it('updates row links only after the searched rows have loaded', async () => {
  vi.useFakeTimers()
  try {
    const basePath = '/requests'
    const query = 'search=previous'
    const id = 'exact/id'
    window.history.replaceState({}, '', basePath + '?' + query)
    const data = (search: string): ApiResponse => ({
      data: [
        {
          item: { id, name: search },
          actions: [
            { key: 'view', label: 'Просмотр' },
            { key: 'edit', label: 'Редактировать' },
            { key: 'delete', label: 'Удалить' },
          ],
        },
      ],
      config: {
        routePath: basePath,
        columns: [{ key: 'name', label: 'Name', title: 'Name' }],
        behaviour: { searchable: true },
      },
      meta: {
        title: '',
        description: '',
        requestQuery: 'search=' + search,
        filters: { _search: search },
      },
    })
    let finish!: (value: Response) => void
    const fetch = vi.fn().mockImplementation(
      () =>
        new Promise((done) => {
          finish = done
        }),
    )
    vi.stubGlobal('fetch', fetch)
    const root = document.createElement('div')
    document.body.append(root)
    const props = {
      resource: { key: 'requests', basePath, title: 'Requests', fields: [], actions: [] },
      data: data('previous'),
    }
    app = createApp(Index, props)
    app.mount(root)
    await settle()
    const viewLink = () => root.querySelector<HTMLAnchorElement>('a[aria-label="Просмотр"]')!
    const source = () => new URL(viewLink().href).searchParams.get('returnTo')
    const previous = source()
    expect(previous).toBe(window.location.pathname + window.location.search)
    expect(new URL(previous!, window.location.origin).searchParams.get('search')).toBe('previous')
    expect(new URL(viewLink().href).pathname).toBe('/requests/records/exact%2Fid')
    expect(root.querySelector('button[aria-label="Удалить"]')).not.toBeNull()
    expect(root.querySelector('a[aria-label="Удалить"]')).toBeNull()
    const search = root.querySelector<HTMLInputElement>('[data-grid-search]')!
    search.value = 'latest'
    search.dispatchEvent(new Event('input', { bubbles: true }))
    await vi.advanceTimersByTimeAsync(500)
    expect(fetch).toHaveBeenCalledOnce()
    expect(source()).toBe(previous)
    expect(root.textContent).toContain('previous')
    finish(Response.json(data('latest')))
    await settle()
    expect(source()).toBe(window.location.pathname + window.location.search)
    expect(new URL(source()!, window.location.origin).searchParams.get('search')).toBe('latest')
    expect(root.textContent).toContain('latest')
    const edit = root.querySelector<HTMLAnchorElement>('a[aria-label="Редактировать"]')!
    expect(new URL(edit.href).pathname).toBe('/requests/records/exact%2Fid/edit')
    expect(new URL(edit.href).searchParams.get('returnTo')).toBe(source())

    const latest = source()
    search.value = 'failed'
    search.dispatchEvent(new Event('input', { bubbles: true }))
    await vi.advanceTimersByTimeAsync(500)
    finish(new Response(null, { status: 500 }))
    await settle()
    expect(root.querySelector('[role="alert"]')).not.toBeNull()
    expect(source()).toBe(latest)
    window.history.replaceState({}, '', '/requests?search=restored')
    window.dispatchEvent(new PopStateEvent('popstate', { state: {} }))
    await settle()
    expect(source()).toBe(latest)
  } finally {
    vi.useRealTimers()
  }
})
it.each([
  ['ru', 'Обновить'],
  ['en', 'Refresh'],
])('refreshes CRUD rows without losing list conditions in %s', async (locale, label) => {
  applyLocale(locale!)
  const query = 'page=2&limit=25&sortBy=name&sortOrder=desc&search=needle&status=draft'
  window.history.replaceState({}, '', '/products?' + query)
  const data: ApiResponse = {
    data: [{ item: { id: 'a', name: 'Before refresh' }, actions: [] }],
    config: {
      routePath: '/products',
      columns: [{ key: 'name', label: 'Name', title: 'Name', sortable: true }],
      behaviour: { refreshable: true },
    },
    meta: {
      title: 'Products',
      description: '',
      requestQuery: query,
      pagination: {
        currentPage: 2,
        perPage: 25,
        total: 50,
        totalPages: 2,
        from: 26,
        to: 50,
        links: [],
      },
      sorting: { sortBy: 'name', sortOrder: 'desc' },
      filters: { _search: 'needle', status: 'draft' },
    },
  }
  const fetch = vi.fn().mockResolvedValue({
    ok: true,
    json: async () => ({
      ...data,
      data: [{ item: { id: 'a', name: 'After refresh' }, actions: [] }],
    }),
  })
  vi.stubGlobal('fetch', fetch)
  vi.spyOn(console, 'debug').mockImplementation(() => {})
  const root = document.createElement('div')
  document.body.append(root)
  app = createApp(Index, {
    resource: {
      key: 'products',
      basePath: '/products',
      title: 'Products',
      fields: [],
      actions: [],
    },
    data,
  })
  app.mount(root)
  await settle()
  expect(fetch).not.toHaveBeenCalled()
  expect(root.textContent).toContain('Before refresh')
  const refresh = [...root.querySelectorAll('button')].find(
    (button) => button.textContent?.trim() === label,
  )!
  expect(refresh).toBeDefined()
  refresh.click()
  await settle()
  expect(fetch).toHaveBeenCalledOnce()
  const requested = new URL(String(fetch.mock.calls[0]![0]), window.location.origin)
  expect(requested.pathname).toBe('/products/data')
  expect(Object.fromEntries(requested.searchParams)).toEqual(
    Object.fromEntries(new URLSearchParams(query)),
  )
  expect(root.textContent).toContain('After refresh')
  expect(root.textContent).not.toContain('Before refresh')
})

it.each(['index', 'show'])(
  'renders the Go page resource without business actions on %s',
  async (screen) => {
    const resource = plainResource as CrudResource
    const root = document.createElement('div')
    document.body.append(root)
    const data = {
      config: { routePath: '/people', columns: [{ key: 'name', label: 'Name', title: 'Name' }] },
      meta: { title: 'People', description: '', filters: {} },
      data: [{ item: { id: '42', name: 'Alice' }, actions: [] }],
    }
    app =
      screen === 'index'
        ? createApp(Index, { resource, data })
        : createApp(Show, {
            resource,
            record: {
              id: '42',
              revision: 'v1',
              data: { name: 'Alice' },
              actions: ['edit', 'delete'],
            },
            permissions: { create: true, update: true, delete: true },
          })
    app.mount(root)
    await settle()
    expect(root.textContent).toContain('Alice')
    expect(root.querySelector('[data-crud-action-dialog]')).toBeNull()
  },
)
