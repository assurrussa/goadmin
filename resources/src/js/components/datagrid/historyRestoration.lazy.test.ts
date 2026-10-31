import { afterEach, expect, it, vi } from 'vitest'
import { createApp, nextTick, type App } from 'vue'
// This is the eager app dependency; the destination component is deliberately lazy.
import './historyRestoration'

let app: App | undefined
afterEach(() => {
  app?.unmount()
  app = undefined
  document.dispatchEvent(new CustomEvent('inertia:before'))
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})
it('preserves a bare URL restored before the lazy grid destination has loaded', async () => {
  document.dispatchEvent(new CustomEvent('inertia:before'))
  window.history.replaceState({ page: { component: 'dashboard/IndexPage' } }, '', '/')
  // The current document was reloaded on Dashboard; no DataGrid chunk exists yet.
  const originalPerformance = window.performance
  vi.spyOn(window, 'performance', 'get').mockReturnValue(
    new Proxy(originalPerformance, {
      get(target, key) {
        if (key === 'getEntriesByType')
          return () => [{ type: 'reload' } as PerformanceNavigationTiming]
        const value = Reflect.get(target, key, target)
        return typeof value === 'function' ? value.bind(target) : value
      },
    }),
  )
  window.history.replaceState({ page: { component: 'users/IndexPage' } }, '', '/users')
  window.dispatchEvent(new PopStateEvent('popstate', { state: window.history.state }))
  const { default: DataGrid } = await import('./DataGrid.vue')
  const initial = {
    data: [{ selectionId: '1', item: { id: 1, name: 'Restored' }, actions: [] }],
    config: {
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
          name: 'Default',
          state: {
            search: 'must-not-apply',
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
  const fetchMock = vi
    .fn<typeof fetch>()
    .mockResolvedValue({ ok: true, json: async () => preferences } as Response)
  vi.stubGlobal('fetch', fetchMock)
  app = createApp(DataGrid, { apiUrl: '/users', initialData: initial, navigationMode: 'inertia' })
  app.mount(document.createElement('div'))
  for (let index = 0; index < 20; index++) await nextTick()
  expect(fetchMock).toHaveBeenCalledTimes(1)
  expect(fetchMock.mock.calls[0][0]).toBe('/users/preferences')
  expect(window.location.search).not.toContain('must-not-apply')
})
