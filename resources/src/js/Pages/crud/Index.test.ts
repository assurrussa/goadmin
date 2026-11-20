import { afterEach, expect, it, vi } from 'vitest'
import { createApp, defineComponent, nextTick } from 'vue'
import Index from './Index.vue'
import type { ApiResponse } from '@/composables/useDataGrid'
import { applyLocale } from '@/i18n'
import { router } from '@inertiajs/vue3'

vi.mock('@inertiajs/vue3', () => ({
  usePage: () => ({ props: {} }),
  router: { on: () => () => {}, get: vi.fn() },
  Head: defineComponent({ render: () => null }),
}))
vi.mock('@/composables/useConfirmation', () => ({ useConfirmation: () => vi.fn() }))
let app: ReturnType<typeof createApp> | undefined
afterEach(() => {
  app?.unmount()
  app = undefined
  document.body.replaceChildren()
  window.history.replaceState({}, '', '/')
  applyLocale('ru')
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
  root.querySelector<HTMLButtonElement>('button[aria-label="Просмотр"]')!.click()
  const href = new URL(vi.mocked(router.get).mock.lastCall![0] as string, window.location.origin)
  expect(href.pathname).toBe(basePath + '/records/' + id)
  expect(href.searchParams.get('returnTo')).toBe(window.location.pathname + window.location.search)
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
