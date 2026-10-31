import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, nextTick, type App } from 'vue'
import DataGrid from './DataGrid.vue'
import type { ApiResponse } from '@/composables/useDataGrid'

const apps: App[] = []
const initial: ApiResponse = {
  data: [{ selectionId: '1', item: { id: 1, name: 'Initial' }, actions: [] }],
  config: {
    routePath: '/users',
    columns: [{ key: 'name', title: 'Name', label: 'Name' }],
    workflow: { key: 'users', preferencesUrl: '/users/preferences', exportUrl: '/users/export' },
  },
}
const preferences = {
  owner: '1',
  version: 1,
  payload: {
    schemaVersion: 1,
    columns: ['name'],
    defaultViewId: 'mine',
    views: [
      {
        id: 'mine',
        name: 'My default',
        state: {
          search: 'saved-query',
          filters: {},
          sortBy: '',
          sortOrder: 'asc',
          limit: 25,
          columns: ['name'],
        },
      },
    ],
  },
}
const settle = async () => {
  for (let index = 0; index < 20; index++) await nextTick()
}
let fetchMock: ReturnType<typeof vi.fn<typeof fetch>>
function mount() {
  const root = document.createElement('div')
  document.body.append(root)
  const app = createApp(DataGrid, {
    apiUrl: '/users',
    initialData: initial,
    navigationMode: 'inertia',
  })
  apps.push(app)
  app.mount(root)
  return root
}
beforeEach(() => {
  document.dispatchEvent(new CustomEvent('inertia:before'))
  window.history.replaceState(
    { page: { component: 'users/IndexPage', url: '/users', props: {} } },
    '',
    '/users',
  )
  fetchMock = vi.fn<typeof fetch>().mockImplementation(
    async (input) =>
      ({
        ok: true,
        json: async () => (String(input) === '/users/preferences' ? preferences : initial),
      }) as Response,
  )
  vi.stubGlobal('fetch', fetchMock)
})
afterEach(() => {
  apps.splice(0).forEach((app) => app.unmount())
  document.body.innerHTML = ''
  document.dispatchEvent(new CustomEvent('inertia:before'))
  vi.unstubAllGlobals()
})
describe('default view navigation provenance', () => {
  it('applies a default on a fresh bare Inertia visit even with history.state.page', async () => {
    mount()
    await settle()
    expect(fetchMock.mock.calls.some(([url]) => String(url).includes('search=saved-query'))).toBe(
      true,
    )
  })
  it('preserves intentionally empty conditions when Back/Forward restores a bare URL', async () => {
    window.dispatchEvent(new PopStateEvent('popstate', { state: window.history.state }))
    const root = mount()
    await settle()
    expect(fetchMock.mock.calls).toHaveLength(1)
    expect(fetchMock.mock.calls[0][0]).toBe('/users/preferences')
    expect(root.textContent).toContain('Initial')
    expect(window.location.search).not.toContain('saved-query')
  })
  it('a fresh SPA visit following history restoration becomes eligible again', async () => {
    window.dispatchEvent(new PopStateEvent('popstate', { state: window.history.state }))
    document.dispatchEvent(new CustomEvent('inertia:before'))
    mount()
    await settle()
    expect(fetchMock.mock.calls.some(([url]) => String(url).includes('search=saved-query'))).toBe(
      true,
    )
  })
})
