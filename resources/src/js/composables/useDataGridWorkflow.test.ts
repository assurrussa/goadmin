import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, createSSRApp, defineComponent, nextTick, ref, type App } from 'vue'
import { renderToString } from '@vue/server-renderer'
import { sessionCoordinator } from '@/http'
import { useDataGrid, type ApiResponse } from './useDataGrid'
import { useDataGridWorkflow, type GridPreferences, type BulkResult } from './useDataGridWorkflow'
import { createSessionCoordinator } from '@/http/session'

const apps: App[] = []
const workflow = {
  key: 'users',
  preferencesUrl: '/users/table-preferences',
  exportUrl: '/users/export',
  bulkUrl: '/users/bulk',
  bulkActions: [{ key: 'delete', label: 'Delete' }],
}
const page = (ids = ['9007199254740993'], currentPage = 1): ApiResponse => ({
  data: ids.map((id) => ({ item: { id: Number(id), name: id }, selectionId: id, actions: [] })),
  config: {
    workflow,
    columns: [
      { key: 'id', label: 'ID', title: 'ID' },
      { key: 'name', label: 'Name', title: 'Name' },
    ],
  },
  pagination: { currentPage, perPage: 10, total: 30, totalPages: 3, from: 1, to: 10, links: [] },
})
const payload = (): GridPreferences => ({ schemaVersion: 1, columns: [], views: [] })
const response = (data: unknown, status = 200) =>
  ({ ok: status < 400, status, json: async () => data }) as Response
const prefs = (value = payload()) => response({ owner: 'owner-a', version: 1, payload: value })
const settle = async () => {
  for (let index = 0; index < 15; index++) await nextTick()
}
let fetchMock: ReturnType<typeof vi.fn<typeof fetch>>
function mount(allowDefault = false, onBulkComplete?: (results: BulkResult[]) => void) {
  let grid!: ReturnType<typeof useDataGrid>
  let model!: ReturnType<typeof useDataGridWorkflow>
  const initialData = ref(page())
  const apiUrl = ref('/users')
  const app = createApp(
    defineComponent({
      setup() {
        grid = useDataGrid({ apiUrl, initialData })
        model = useDataGridWorkflow(grid, allowDefault, onBulkComplete)
        return () => null
      },
    }),
  )
  app.mount(document.createElement('div'))
  apps.push(app)
  return { grid, model, initialData, apiUrl, app }
}
beforeEach(() => {
  fetchMock = vi.fn<typeof fetch>().mockResolvedValue(prefs())
  vi.stubGlobal('fetch', fetchMock)
})
afterEach(() => {
  apps.splice(0).forEach((app) => app.unmount())
  sessionCoordinator.configure({})
  sessionCoordinator.dispose()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

describe('opt-in workflow lifecycle', () => {
  it('does not fetch preferences or retain session subscriptions during server rendering', async () => {
    const subscribe = vi.spyOn(sessionCoordinator, 'subscribe')
    const app = createSSRApp(
      defineComponent({
        setup() {
          const grid = useDataGrid({ apiUrl: '/users', initialData: page() })
          useDataGridWorkflow(grid, true)
          return () => null
        },
      }),
    )
    await renderToString(app)
    expect(fetchMock).not.toHaveBeenCalled()
    expect(subscribe).not.toHaveBeenCalled()
  })
  it('clears rows, selection, drafts and results immediately on a terminal session change', async () => {
    const { grid, model } = mount()
    await settle()
    sessionCoordinator.configure({
      props: { adminSessionMaintenance: { version: 1, sessionEpoch: 'A' } },
    })
    grid.toggleSelectAll()
    model.preferences.value.views.push({
      id: 'draft',
      name: 'Private',
      state: { search: '', filters: {}, sortBy: '', sortOrder: 'desc', limit: 10, columns: ['id'] },
    })
    model.results.value = [{ id: 'secret', status: 'unknown' }]
    sessionCoordinator.reportFailure('session_changed')
    expect(grid.items.value).toEqual([])
    expect(grid.selectedItems.value).toEqual([])
    expect(model.preferences.value.views).toEqual([])
    expect(model.results.value).toEqual([])
    sessionCoordinator.dispose()
  })
  it('preserves exact IDs across pages, page-unselect removes only page IDs, and caps at 100', async () => {
    const { grid } = mount()
    await settle()
    grid.filters.value = { status: '1' }
    grid.toggleSelectAll()
    expect(grid.selectedItems.value).toEqual(['9007199254740993'])
    fetchMock.mockResolvedValueOnce(
      response({
        ...page(['2', '3'], 2),
        meta: { title: '', description: '', filters: { status: 1, _search: '' } },
      }),
    )
    await grid.loadData({ page: 2 })
    grid.toggleSelectAll()
    expect(grid.selectedItems.value).toEqual(['9007199254740993', '2', '3'])
    grid.toggleSelectAll()
    expect(grid.selectedItems.value).toEqual(['9007199254740993'])
    fetchMock.mockResolvedValueOnce(
      response(
        page(
          Array.from({ length: 110 }, (_, i) => String(i + 10)),
          3,
        ),
      ),
    )
    await grid.loadData({ page: 3 })
    grid.toggleSelectAll()
    expect(grid.selectedItems.value).toHaveLength(100)
    grid.handleSearch('changed')
    expect(grid.selectedItems.value).toEqual([])
  })
  it.each(['search', 'filter'] as const)(
    'blocks stale-row selection immediately during the %s debounce',
    async (condition) => {
      const { grid } = mount()
      await settle()
      if (condition === 'search') grid.handleSearch('new query')
      else grid.handleFilterChange({ status: 'active' })
      expect(grid.loading.value).toBe(true)
      expect(grid.selectionDisabled.value).toBe(true)
      grid.toggleSelectAll()
      expect(grid.selectedItems.value).toEqual([])
      grid.toggleSelectItem('9007199254740993')
      expect(grid.selectedItems.value).toEqual([])
      expect(fetchMock).toHaveBeenCalledTimes(1)
    },
  )
  it('cannot select retained rows while a new filter is loading or failed', async () => {
    const { grid, model } = mount()
    await settle()
    let finish!: (value: Response) => void
    fetchMock.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finish = resolve
        }),
    )
    const request = grid.loadData({ filters: { status: 'active' } })
    grid.toggleSelectAll()
    expect(grid.selectedItems.value).toEqual([])
    grid.toggleSelectItem('9007199254740993')
    expect(grid.selectedItems.value).toEqual([])
    finish(response({}, 500))
    await request
    grid.toggleSelectAll()
    expect(grid.selectedItems.value).toEqual([])
    grid.toggleSelectItem('9007199254740993')
    expect(grid.selectedItems.value).toEqual([])
    model.pendingAction.value = 'delete'
    await model.executeBulk()
    expect(fetchMock.mock.calls.some(([, init]) => init?.method === 'POST')).toBe(false)
  })
  it('keeps selection blocked until the newest query succeeds, then uses its rows', async () => {
    const { grid, model } = mount()
    await settle()
    const pending: Array<(value: Response) => void> = []
    fetchMock.mockImplementationOnce(() => new Promise((resolve) => pending.push(resolve)))
    fetchMock.mockImplementationOnce(() => new Promise((resolve) => pending.push(resolve)))
    const older = grid.loadData({ filters: { status: 'older' } })
    const newer = grid.loadData({ filters: { status: 'newer' } })
    pending[0]!(response(page(['2'])))
    await older
    grid.toggleSelectAll()
    expect(grid.loading.value).toBe(true)
    expect(grid.selectedItems.value).toEqual([])
    const latest = {
      ...page(['3']),
      meta: { title: '', description: '', filters: { status: 'newer', _search: '' } },
    }
    pending[1]!(response(latest))
    await newer
    grid.toggleSelectAll()
    expect(grid.selectedItems.value).toEqual(['3'])
    fetchMock.mockResolvedValueOnce(response({ results: [{ id: '3', status: 'succeeded' }] }))
    fetchMock.mockResolvedValueOnce(response(latest))
    model.pendingAction.value = 'delete'
    await model.executeBulk()
    const command = fetchMock.mock.calls.find(([, init]) => init?.method === 'POST')
    expect(JSON.parse(String(command?.[1]?.body))).toEqual({ action: 'delete', ids: ['3'] })
  })
  it('never sends old-filter rows selected during loading after the new result arrives', async () => {
    const { grid, model } = mount()
    await settle()
    let finish!: (value: Response) => void
    fetchMock.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finish = resolve
        }),
    )
    const request = grid.loadData({ filters: { status: 'active' } })
    grid.toggleSelectAll()
    finish(
      response({
        ...page(['2']),
        meta: { title: '', description: '', filters: { status: 'active', _search: '' } },
      }),
    )
    await request
    model.pendingAction.value = 'delete'
    await model.executeBulk()
    expect(grid.items.value.map((row) => row.selectionId)).toEqual(['2'])
    expect(fetchMock.mock.calls.some(([, init]) => init?.method === 'POST')).toBe(false)
  })
  it.each(['loading', 'failed', 'navigation'] as const)(
    'does not dispatch bulk against a %s grid',
    async (state) => {
      const { grid, model } = mount()
      await settle()
      grid.toggleSelectAll()
      if (state === 'loading') grid.loading.value = true
      if (state === 'failed') grid.loadError.value = 'Load failed'
      if (state === 'navigation') grid.suspendForNavigation()
      // Even a caller retaining a stale selection cannot bypass the command guard.
      grid.selectedItems.value = ['9007199254740993']
      model.pendingAction.value = 'delete'
      await model.executeBulk()
      expect(fetchMock.mock.calls.some(([, init]) => init?.method === 'POST')).toBe(false)
      grid.toggleSelectAll()
      expect(grid.selectedItems.value).toEqual(['9007199254740993'])
    },
  )
  it('retains local edits on optimistic conflicts and prevents a second overwrite', async () => {
    const { model } = mount()
    await settle()
    model.toggleColumn('id')
    fetchMock.mockResolvedValueOnce(response({}, 409))
    await model.savePreferences()
    expect(model.conflict.value).toBe(true)
    expect(model.columns.value).toEqual(['name'])
    const calls = fetchMock.mock.calls.length
    await model.savePreferences()
    expect(fetchMock).toHaveBeenCalledTimes(calls)
  })
  it('does not let late preferences or commands hydrate a new owner/resource', async () => {
    let finish!: (value: Response) => void
    fetchMock.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finish = resolve
        }),
    )
    const { model, grid } = mount()
    model.invalidate()
    finish(prefs({ schemaVersion: 1, columns: ['name'], views: [] }))
    await settle()
    expect(model.ready.value).toBe(false)
    expect(grid.items.value).toEqual([])
    expect(grid.config.value).toEqual({})
    expect(model.preferences.value.views).toEqual([])
  })
  it('never replays an unknown bulk result, even if its ID is selected again', async () => {
    const { model, grid } = mount()
    await settle()
    grid.toggleSelectAll()
    fetchMock.mockRejectedValueOnce(new Error('connection lost'))
    fetchMock.mockResolvedValueOnce(response(page()))
    model.pendingAction.value = 'delete'
    await model.executeBulk()
    await settle()
    expect(model.results.value).toEqual([{ id: '9007199254740993', status: 'unknown' }])
    const command = fetchMock.mock.calls.find(([, init]) => init?.method === 'POST')
    expect(JSON.parse(String(command?.[1]?.body))).toEqual({
      action: 'delete',
      ids: ['9007199254740993'],
    })
    grid.toggleSelectAll()
    model.pendingAction.value = 'delete'
    await model.executeBulk()
    expect(fetchMock.mock.calls.filter(([, init]) => init?.method === 'POST')).toHaveLength(1)
  })
  it('applies a default only when eligible and user conditions remain unchanged', async () => {
    const value: GridPreferences = {
      schemaVersion: 1,
      columns: ['id'],
      defaultViewId: 'mine',
      views: [
        {
          id: 'mine',
          name: 'Mine',
          state: {
            search: 'saved',
            filters: {},
            sortBy: 'name',
            sortOrder: 'asc',
            limit: 25,
            columns: ['name'],
          },
        },
      ],
    }
    fetchMock.mockResolvedValueOnce(prefs(value)).mockResolvedValueOnce(response(page()))
    const { model } = mount(true)
    await settle()
    expect(model.activeViewId.value).toBe('mine')
    expect(String(fetchMock.mock.calls[1]?.[0])).toContain('search=saved')
    let finish!: (value: Response) => void
    fetchMock.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finish = resolve
        }),
    )
    const second = mount(true)
    second.grid.handleSearch('typing')
    finish(prefs(value))
    await settle()
    expect(second.model.activeViewId.value).toBe('')
    expect(second.grid.searchQuery.value).toBe('typing')
  })
  it('keeps visible column order in selected CSV, preserves exact IDs, and reports truncation', async () => {
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    vi.stubGlobal(
      'URL',
      Object.assign(URL, { createObjectURL: vi.fn(() => 'blob:test'), revokeObjectURL: vi.fn() }),
    )
    const { model, grid } = mount()
    await settle()
    model.preferences.value.columns = ['name', 'id']
    grid.toggleSelectAll()
    fetchMock.mockResolvedValueOnce({
      ok: true,
      headers: new Headers({ 'content-type': 'text/csv', 'X-Goadmin-Export-Truncated': 'true' }),
      blob: async () => new Blob(['Name,ID']),
    } as Response)
    await model.exportCSV('selected')
    const url = new URL(String(fetchMock.mock.calls.at(-1)?.[0]))
    expect(url.searchParams.get('ids')).toBe('9007199254740993')
    expect(url.searchParams.get('columns')).toBe('name,id')
    expect(model.notice.value).toContain('10 000')
    expect(click).toHaveBeenCalledOnce()
  })
  it('does not change legacy grids or load preferences without opt-in metadata', async () => {
    const { initialData, model } = mount()
    await settle()
    initialData.value = { data: [], config: {} }
    await settle()
    expect(model.workflow.value).toBeUndefined()
    expect(model.ready.value).toBe(false)
  })
})

describe('session observer', () => {
  it('subscribes and unsubscribes without changing coordinator state', () => {
    const coordinator = createSessionCoordinator()
    const listener = vi.fn()
    const stop = coordinator.subscribe(listener)
    coordinator.configure({ props: { adminSessionMaintenance: { version: 1, sessionEpoch: 'A' } } })
    const before = listener.mock.calls.length
    expect(before).toBeGreaterThan(0)
    stop()
    coordinator.reportFailure('session_changed')
    expect(listener).toHaveBeenCalledTimes(before)
    expect(coordinator.getState()).toBe('session_changed')
    coordinator.dispose()
  })
})

describe('named view drafts and schema evolution', () => {
  const named = (): GridPreferences => ({
    schemaVersion: 1,
    columns: ['id'],
    knownColumns: ['id', 'name'],
    views: [
      {
        id: 'mine',
        name: 'Saved',
        state: {
          search: '',
          filters: {},
          sortBy: '',
          sortOrder: 'desc',
          limit: 10,
          columns: ['id'],
          knownColumns: ['id', 'name'],
        },
      },
    ],
  })
  it('marks every named-view edit dirty until explicit save and clears stale saved feedback', async () => {
    fetchMock.mockResolvedValueOnce(prefs(named()))
    const { model, grid } = mount()
    await settle()
    vi.spyOn(grid, 'loadData').mockImplementation(async (params) => {
      grid.searchQuery.value = params?.search ?? ''
      grid.filters.value = params?.filters ?? {}
      grid.sortBy.value = params?.sortBy ?? ''
      grid.sortOrder.value = params?.sortOrder ?? 'desc'
      grid.pagination.value!.perPage = params?.limit ?? 10
    })
    model.applyView('mine')
    await settle()
    expect(model.viewDirty?.value).toBe(false)
    model.notice.value = 'Настройки сохранены.'
    grid.searchQuery.value = 'draft'
    expect(model.viewDirty.value).toBe(true)
    expect(model.notice.value).toBe('')
    grid.searchQuery.value = ''
    expect(model.viewDirty.value).toBe(false)
    for (const edit of [
      () => {
        grid.filters.value = { name: 'Draft' }
      },
      () => {
        grid.sortBy.value = 'name'
      },
      () => {
        grid.sortOrder.value = 'asc'
      },
      () => {
        grid.pagination.value!.perPage = 25
      },
      () => model.toggleColumn('name'),
      () => {
        model.viewName.value = 'Renamed'
      },
    ]) {
      edit()
      expect(model.viewDirty.value).toBe(true)
      model.applyView('mine')
      await settle()
      expect(model.viewDirty.value).toBe(false)
    }
  })
  it('CRUD saves full conditions and snapshots, renames explicitly, removes default on deletion', async () => {
    const { model, grid } = mount()
    await settle()
    grid.searchQuery.value = 'saved'
    grid.filters.value = { name: 'Alice' }
    fetchMock.mockResolvedValueOnce(response({ owner: 'owner-a', version: 2 }))
    model.saveView('Mine')
    await settle()
    const id = model.activeViewId.value
    expect(model.viewDirty.value).toBe(false)
    const body = JSON.parse(String(fetchMock.mock.calls.at(-1)?.[1]?.body))
    expect(body.payload.knownColumns).toEqual(['id', 'name'])
    expect(body.payload.views[0].state).toMatchObject({
      search: 'saved',
      filters: { name: 'Alice' },
      knownColumns: ['id', 'name'],
    })
    model.viewName.value = 'Renamed'
    expect(model.viewDirty.value).toBe(true)
    fetchMock.mockResolvedValueOnce(response({}, 409))
    model.saveView(model.viewName.value, true)
    await settle()
    expect(model.viewDirty.value).toBe(true)
    expect(model.viewName.value).toBe('Renamed')
    model.conflict.value = false
    fetchMock.mockResolvedValueOnce(response({ owner: 'owner-a', version: 3 }))
    model.saveView(model.viewName.value, true)
    await settle()
    expect(model.viewDirty.value).toBe(false)
    expect(model.preferences.value.views[0].id).toBe(id)
    fetchMock.mockResolvedValueOnce(response({ owner: 'owner-a', version: 4 }))
    model.setDefault()
    await settle()
    expect(model.preferences.value.defaultViewId).toBe(id)
    fetchMock.mockResolvedValueOnce(response({ owner: 'owner-a', version: 5 }))
    model.deleteView()
    await settle()
    expect(model.preferences.value.views).toEqual([])
    expect(model.preferences.value.defaultViewId).toBeUndefined()
    expect(model.viewName.value).toBe('')
    expect(model.viewDirty.value).toBe(false)
  })
  it('adds truly new columns for each independent view while retaining intentional hidden keys', async () => {
    const value = named()
    value.views.push({
      id: 'other',
      name: 'Other',
      state: { ...value.views[0].state, columns: ['name'], knownColumns: ['name'] },
    })
    fetchMock.mockResolvedValueOnce(prefs(value))
    const { grid, model } = mount()
    await settle()
    expect(model.preferences.value.views[1].state.columns).toEqual(['name', 'id'])
    grid.config.value = {
      ...grid.config.value,
      columns: [...grid.config.value.columns!, { key: 'added', label: 'New', title: 'New' }],
    }
    await settle()
    expect(model.columns.value).toEqual(['id', 'added'])
    expect(model.preferences.value.views[0].state.columns).toEqual(['id', 'added'])
    expect(model.preferences.value.views[1].state.columns).toEqual(['name', 'id', 'added'])
    model.applyView('other')
    expect(model.columns.value).toEqual(['name', 'id', 'added'])
    expect(model.preferences.value.knownColumns).toEqual(['id', 'name', 'added'])
  })
  it('baselines legacy snapshots without revealing hidden columns and removes disappeared keys', async () => {
    fetchMock.mockResolvedValueOnce(
      prefs({
        schemaVersion: 1,
        columns: ['id'],
        views: [
          {
            id: 'old',
            name: 'Old',
            state: {
              search: '',
              filters: {},
              sortBy: '',
              sortOrder: 'desc',
              limit: 10,
              columns: ['id'],
            },
          },
        ],
      }),
    )
    const { grid, model } = mount()
    await settle()
    expect(model.columns.value).toEqual(['id'])
    expect(model.preferences.value.knownColumns).toEqual(['id', 'name'])
    grid.config.value = {
      ...grid.config.value,
      columns: [
        { key: 'name', label: 'Name', title: 'Name' },
        { key: 'new', label: 'New', title: 'New' },
      ],
    }
    await settle()
    expect(model.columns.value).toEqual(['new'])
    expect(model.preferences.value.views[0].state.columns).toEqual(['new'])
  })
})

describe('late file and mutation responses', () => {
  const invalidate = (
    cause: 'session' | 'unmount' | 'navigation',
    mounted: ReturnType<typeof mount>,
  ) => {
    if (cause === 'unmount') mounted.app.unmount()
    else if (cause === 'navigation') mounted.grid.suspendForNavigation()
    else {
      sessionCoordinator.configure({
        props: { adminSessionMaintenance: { version: 1, sessionEpoch: 'A' } },
      })
      sessionCoordinator.reportFailure('session_changed')
    }
  }
  it.each(['session', 'unmount', 'navigation'] as const)(
    'never downloads a late CSV body after %s',
    async (cause) => {
      const createObjectURL = vi.fn(() => 'blob:test')
      const OriginalURL = URL
      vi.stubGlobal(
        'URL',
        class extends OriginalURL {
          static createObjectURL = createObjectURL
          static revokeObjectURL = vi.fn()
        },
      )
      const mounted = mount()
      await settle()
      let finish!: (blob: Blob) => void
      fetchMock.mockResolvedValueOnce({
        ok: true,
        headers: new Headers({ 'content-type': 'text/csv' }),
        blob: () =>
          new Promise((resolve) => {
            finish = resolve
          }),
      } as Response)
      const request = mounted.model.exportCSV('filtered')
      await settle()
      invalidate(cause, mounted)
      finish(new Blob(['secret']))
      await request
      expect(createObjectURL).not.toHaveBeenCalled()
      expect(mounted.model.notice.value).toBe('')
      expect(mounted.model.busy.value).toBe(false)
    },
  )
  it.each(['session', 'unmount', 'navigation'] as const)(
    'never reports or publishes a late bulk JSON body after %s',
    async (cause) => {
      const completed = vi.fn()
      const mounted = mount(false, completed)
      await settle()
      mounted.grid.toggleSelectAll()
      let finish!: (value: unknown) => void
      fetchMock.mockResolvedValueOnce({
        ok: true,
        json: () =>
          new Promise((resolve) => {
            finish = resolve
          }),
      } as Response)
      mounted.model.pendingAction.value = 'delete'
      const request = mounted.model.executeBulk()
      await settle()
      invalidate(cause, mounted)
      finish({ results: [{ id: '9007199254740993', status: 'succeeded' }] })
      await request
      expect(completed).not.toHaveBeenCalled()
      expect(mounted.model.results.value).toEqual([])
      expect(mounted.model.busy.value).toBe(false)
    },
  )
  it('notifies hosts only after a validated complete server response, preserving per-item outcomes', async () => {
    const completed = vi.fn()
    const { model, grid } = mount(false, completed)
    await settle()
    grid.toggleSelectAll()
    fetchMock.mockResolvedValueOnce(
      response({ results: [{ id: '9007199254740993', status: 'skipped' }] }),
    )
    fetchMock.mockResolvedValueOnce(response(page()))
    model.pendingAction.value = 'delete'
    await model.executeBulk()
    await settle()
    expect(completed).toHaveBeenCalledWith([{ id: '9007199254740993', status: 'skipped' }])
    completed.mockClear()
    grid.toggleSelectAll()
    fetchMock.mockRejectedValueOnce(new Error('lost response'))
    fetchMock.mockResolvedValueOnce(response(page()))
    model.pendingAction.value = 'delete'
    await model.executeBulk()
    expect(completed).not.toHaveBeenCalled()
  })
})
