import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  createApp,
  defineComponent,
  h,
  nextTick,
  reactive,
  shallowRef,
  type App,
  type Component,
} from 'vue'
import DataGrid from './DataGrid.vue'
import DataGridHeader from './DataGridHeader.vue'
import DataGridFilters from './DataGridFilters.vue'
import DataGridTable from './DataGridTable.vue'
import DataGridPagination from './DataGridPagination.vue'
import type {
  ApiResponse,
  Column,
  Config,
  DataItem,
  Pagination,
} from '../../composables/useDataGrid'

// All DataGrid and UI components are real, including the selection primitive.
// Network is the sole API boundary mock.
const apps: App[] = []
function mount(component: Component, initialProps: Record<string, unknown>) {
  const props = reactive(initialProps)
  const componentRef = shallowRef<{ refreshData?: () => void } | null>(null)
  const warnings: string[] = []
  const app = createApp(
    defineComponent({ setup: () => () => h(component, { ...props, ref: componentRef }) }),
  )
  app.config.warnHandler = (message) => warnings.push(message)
  const element = document.createElement('div')
  document.body.append(element)
  app.mount(element)
  apps.push(app)
  return { app, element, props, warnings, componentRef }
}
const column = (key: string, overrides: Partial<Column> = {}): Column => ({
  key,
  title: key,
  label: key,
  ...overrides,
})
const page = (overrides: Partial<Pagination> = {}): Pagination => ({
  currentPage: 1,
  perPage: 10,
  total: 30,
  totalPages: 3,
  from: 1,
  to: 10,
  nextPageUrl: '/audit?page=2',
  links: [
    { url: null, label: 'Previous', active: false },
    { url: '/audit?page=1', label: '1', active: true },
    { url: '/audit?page=2', label: '2', active: false },
    { url: null, label: '...', active: false },
    { url: '/audit?page=2', label: 'Next', active: false },
  ],
  ...overrides,
})
const config = (overrides: Partial<Config> = {}): Config => ({
  columns: [column('name', { sortable: true }), column('plain')],
  behaviour: { searchable: true, creatable: true, refreshable: true, exportable: true },
  ui: { createButtonText: 'Add record' },
  ...overrides,
})
const row = (id: string | number = 'uuid-1', overrides: Partial<DataItem> = {}): DataItem => ({
  item: { id, name: 'Record one', plain: 'Plain' },
  actions: [{ key: 'edit', label: 'Edit row' }],
  ...overrides,
})
const fixture = (overrides: Partial<ApiResponse> = {}): ApiResponse => ({
  data: [row()],
  config: config(),
  meta: { title: 'Audit title', description: 'Audit description', pagination: page() },
  ...overrides,
})
function grid(data: ApiResponse = fixture(), props: Record<string, unknown> = {}) {
  return mount(DataGrid, { apiUrl: '/audit', initialData: data, syncWithUrl: false, ...props })
}
function table(items = [row()], options: Record<string, unknown> = {}) {
  return mount(DataGridTable, {
    config: config(),
    items,
    sortBy: '',
    sortOrder: 'desc',
    selectedItems: [],
    allSelected: false,
    ...options,
  })
}
const buttons = (element: ParentNode) =>
  Array.from(element.querySelectorAll<HTMLButtonElement>('button'))
const button = (element: ParentNode, label: string) => {
  const match = buttons(element).find(
    (candidate) =>
      candidate.textContent?.trim() === label || candidate.getAttribute('aria-label') === label,
  )
  if (!match) throw new Error(`Button ${label} not found`)
  return match
}
const text = (element: Element) => element.textContent?.replace(/\s+/g, ' ').trim()
const settle = async () => {
  for (let i = 0; i < 8; i++) await nextTick()
}
const fetchMock = vi.fn<typeof fetch>()
const response = (data: ApiResponse = fixture()): Response =>
  ({ ok: true, json: async () => data }) as Response
const query = (index = 0) =>
  new URL(String(fetchMock.mock.calls[index][0]), window.location.origin).searchParams
const input = async (element: HTMLInputElement, value: string) => {
  element.value = value
  element.dispatchEvent(new Event('input', { bubbles: true }))
  await settle()
}
const rich = (content: unknown[]) => JSON.stringify({ type: 'doc', content })
const richText = (value: string) =>
  table([row('r', { item: { id: 'r', body: value }, actions: [] })], {
    config: config({ columns: [column('body', { type: 'richtext' })] }),
  })

beforeEach(() => {
  delete window.AdminDataGrid
  window.history.replaceState({}, '', '/')
  fetchMock.mockReset().mockResolvedValue(response())
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

describe('DataGrid passing contracts: exact public action callbacks', () => {
  it('emits refresh/export once through action-table with null payload, without fetching', async () => {
    const onActionTable = vi.fn(),
      onActionRefresh = vi.fn(),
      onActionExport = vi.fn()
    const { element } = grid(fixture(), { onActionTable, onActionRefresh, onActionExport })
    await settle()
    button(element, 'Обновить').click()
    button(element, 'Экспорт').click()
    expect(onActionTable.mock.calls).toEqual([
      ['refresh', null],
      ['export', null],
    ])
    expect(onActionRefresh).not.toHaveBeenCalled()
    expect(onActionExport).not.toHaveBeenCalled()
    expect(fetchMock).not.toHaveBeenCalled()
  })
  it('emits create exactly once with no arguments', async () => {
    const onActionCreate = vi.fn()
    const { element } = grid(fixture(), { onActionCreate })
    await settle()
    button(element, 'Add record').click()
    expect(onActionCreate.mock.calls).toEqual([[]])
    expect(fetchMock).not.toHaveBeenCalled()
  })
  it.each(['edit', 'show', 'view', 'delete'])(
    'emits %s once with the original row object, not computed values',
    async (key) => {
      const item = { id: 'stable', name: 'Raw name' }
      const onActionRow = vi.fn()
      const { element } = grid(
        fixture({
          data: [
            row('stable', {
              item,
              values: { name: 'Computed name' },
              actions: [{ key, label: `${key} row` }],
            }),
          ],
        }),
        { onActionRow },
      )
      await settle()
      button(element, `${key} row`).click()
      expect(onActionRow).toHaveBeenCalledTimes(1)
      expect(onActionRow).toHaveBeenCalledWith(key, item)
      expect(onActionRow.mock.calls[0][1].name).toBe('Raw name')
      expect(text(element)).toContain('Computed name')
    },
  )
  it('exposes refreshData as a single fetch of current grid state', async () => {
    vi.spyOn(console, 'debug').mockImplementation(() => {})
    const { componentRef } = grid()
    await settle()
    expect(componentRef.value?.refreshData).toBeTypeOf('function')
    componentRef.value!.refreshData!()
    expect(fetchMock).toHaveBeenCalledOnce()
    expect(query().get('page')).toBe('1')
  })
  it('hides all header controls when their behavior flags are false', async () => {
    const { element } = grid(fixture({ config: config({ behaviour: {} }) }))
    await settle()
    expect(element.querySelector('input')).toBeNull()
    expect(
      buttons(element).some((candidate) =>
        ['Обновить', 'Экспорт', 'Add record'].includes(text(candidate) || ''),
      ),
    ).toBe(false)
  })
})

describe('DataGridHeader passing contracts', () => {
  it('uses meta title/description and configured create label', () => {
    const { element } = mount(DataGridHeader, {
      meta: { title: 'Title', description: 'Description' },
      config: config(),
      searchQuery: '',
    })
    expect(element.querySelector('h1')?.textContent).toBe('Title')
    expect(text(element)).toContain('Description')
    expect(button(element, 'Add record')).toBeDefined()
  })
  it('falls back to default title and does not use config.ui.title/description', () => {
    const { element } = mount(DataGridHeader, {
      meta: null,
      config: { ui: { title: 'Ignored title', description: 'Ignored description' } },
      searchQuery: '',
    })
    expect(text(element)).toBe('Данные')
  })
  it('emits one exact search argument per real input update', async () => {
    const onSearch = vi.fn()
    const { element } = mount(DataGridHeader, {
      meta: null,
      config: config(),
      searchQuery: '',
      onSearch,
    })
    await input(element.querySelector('input')!, 'Alice & Bob')
    expect(onSearch.mock.calls).toEqual([['Alice & Bob']])
  })
})

describe('DataGridPagination passing contracts: enabled versus no-op controls', () => {
  it.each([null, page({ totalPages: 1 }), page({ totalPages: 0 })])(
    'renders nothing without multiple pages (%j)',
    (pagination) => {
      expect(mount(DataGridPagination, { pagination }).element.querySelector('button')).toBeNull()
    },
  )
  it('emits exact URL once for each enabled numbered and mobile-next click', () => {
    const onPageChange = vi.fn()
    const { element } = mount(DataGridPagination, { pagination: page(), onPageChange })
    button(element, '2').click()
    button(element, 'Вперед').click()
    expect(onPageChange.mock.calls).toEqual([['/audit?page=2'], ['/audit?page=2']])
  })
  it('disables absent previous and active page; ellipsis is text and none emit on click', () => {
    const onPageChange = vi.fn()
    const { element } = mount(DataGridPagination, { pagination: page(), onPageChange })
    for (const label of ['Назад', '‹', '1']) {
      expect(button(element, label).disabled).toBe(true)
      button(element, label).click()
    }
    const ellipsis = Array.from(element.querySelectorAll('span')).find(
      (span) => text(span) === '...',
    )!
    ellipsis.click()
    expect(onPageChange).not.toHaveBeenCalled()
  })
  it('formats Previous/Next labels and shows server supplied from/to/total', () => {
    const { element } = mount(DataGridPagination, { pagination: page() })
    expect(button(element, '‹')).toBeDefined()
    expect(button(element, '›')).toBeDefined()
    expect(text(element)).toContain('Показано 1 - 10 из 30 записей')
  })
  it('integrates one enabled page click into one fetch and one public page-change callback', async () => {
    const onPageChange = vi.fn()
    const { element } = grid(fixture(), { onPageChange })
    await settle()
    button(element, '2').click()
    expect(fetchMock).toHaveBeenCalledOnce()
    expect(query().get('page')).toBe('2')
    expect(onPageChange.mock.calls).toEqual([['/audit?page=2']])
  })
})

describe('DataGridFilters passing contracts', () => {
  it('renders nothing when there are no filterable columns', () => {
    expect(text(mount(DataGridFilters, { filterableColumns: [], filters: {} }).element)).toBe('')
  })
  it('renders text and numeric inputs with labels and emits one-key patches', async () => {
    const onFilterChange = vi.fn()
    const { element } = mount(DataGridFilters, {
      filterableColumns: [column('name'), column('count', { type: 'number' })],
      filters: {},
      onFilterChange,
    })
    expect(element.querySelector('label[for="filter-name"]')).not.toBeNull()
    const count = element.querySelector<HTMLInputElement>('#filter-count')!
    expect(count.type).toBe('number')
    expect(count.getAttribute('inputmode')).toBe('numeric')
    await input(element.querySelector('#filter-name')!, 'Ada')
    await input(count, '0')
    expect(onFilterChange.mock.calls).toEqual([[{ name: 'Ada' }], [{ count: 0 }]])
  })
  it('displays 0 and false as active filters and clears configured columns with empty strings', () => {
    const onFilterChange = vi.fn()
    const { element } = mount(DataGridFilters, {
      filterableColumns: [column('count'), column('flag')],
      filters: { count: 0, flag: false },
      onFilterChange,
    })
    expect(element.querySelector<HTMLInputElement>('#filter-count')?.value).toBe('0')
    expect(element.querySelector<HTMLInputElement>('#filter-flag')?.value).toBe('false')
    button(element, 'Очистить все').click()
    expect(onFilterChange.mock.calls).toEqual([[{ count: '', flag: '' }]])
  })
  it('hides clear-all for only blank/null/undefined filters', () => {
    const { element } = mount(DataGridFilters, {
      filterableColumns: [column('name')],
      filters: { name: '', a: null, b: undefined },
    })
    expect(buttons(element)).toHaveLength(0)
  })
  it('renders select with label, placeholder and configured selected option', async () => {
    const { element } = mount(DataGridFilters, {
      filterableColumns: [
        column('status', {
          type: 'select',
          options: [{ value: 'a', label: 'Active' }],
          placeholder: 'Choose status',
        }),
      ],
      filters: { status: 'a' },
    })
    await settle()
    expect(element.querySelector('[role="combobox"]')).not.toBeNull()
    expect(text(element)).toContain('Active')
    expect(element.querySelector('label')?.getAttribute('for')).toBe('filter-status')
  })
})

describe('DataGridTable passing contracts: rendering, sort and identity', () => {
  it('emits sorting only from sortable header, once with column key', () => {
    const onSort = vi.fn()
    const { element } = table([row()], { onSort })
    const heads = element.querySelectorAll<HTMLTableCellElement>('th')
    heads[0].querySelector('button')!.click()
    heads[1].click()
    expect(onSort.mock.calls).toEqual([['name']])
  })
  it('renders no action header or cells when every row has no actions', () => {
    const { element } = table([row('x', { actions: [] })])
    expect(element.querySelectorAll('th')).toHaveLength(2)
    expect(element.querySelectorAll('td')).toHaveLength(2)
    expect(element.querySelector('tbody button')).toBeNull()
  })
  it('prefers own computed values even when null or empty, but falls back when absent', () => {
    const { element } = table([row('x', { values: { name: null } })])
    const cells = element.querySelectorAll('td')
    expect(text(cells[0])).toBe('')
    expect(text(cells[1])).toBe('Plain')
  })
  it('does not read inherited computed values', () => {
    const values = Object.create({ name: 'Inherited' }) as Record<string, unknown>
    const { element } = table([row('x', { values })])
    expect(text(element.querySelector('td')!)).toBe('Record one')
  })
  it('renders boolean true/false distinctly', () => {
    const { element } = table([row('x', { item: { id: 'x', yes: true, no: false } })], {
      config: config({
        columns: [column('yes', { type: 'boolean' }), column('no', { type: 'boolean' })],
      }),
    })
    const cells = element.querySelectorAll('td')
    expect(text(cells[0])).toBe('Да')
    expect(text(cells[1])).toBe('Нет')
  })
  it('renders select badge labels including a numeric zero key and falls back for unknown values', () => {
    const { element } = table([row('x', { item: { id: 'x', status: 0, unknown: 'other' } })], {
      config: config({
        columns: [
          column('status', {
            type: 'select',
            badges: { 0: { label: 'Zero status', variant: 'success' } },
          }),
          column('unknown', { type: 'select', badges: {} }),
        ],
      }),
    })
    expect(text(element.querySelectorAll('td')[0])).toBe('Zero status')
    expect(text(element.querySelectorAll('td')[1])).toBe('other')
  })
  it('renders plain text HTML characters literally without introducing DOM elements', () => {
    const { element } = table([
      row('x', { item: { id: 'x', name: '<b data-audit-plain>literal</b>' } }),
    ])
    expect(element.querySelector('[data-audit-plain]')).toBeNull()
    expect(text(element)).toContain('<b data-audit-plain>literal</b>')
  })
  it('preserves actual row DOM identity when unique configured IDs reorder', async () => {
    const rows = ['a', 'b'].map((uuid) => row(uuid, { item: { uuid, name: uuid }, actions: [] }))
    const { element, props } = table(rows, { config: config({ ui: { idKey: 'uuid' } }) })
    const before = Array.from(element.querySelectorAll('tbody tr'))
    props.items = [rows[1], rows[0]]
    await settle()
    const after = Array.from(element.querySelectorAll('tbody tr'))
    expect(after[0]).toBe(before[1])
    expect(after[1]).toBe(before[0])
  })
  it('preserves row DOM identity when values change but the ID stays stable', async () => {
    const { element, props } = table([row('stable')])
    const before = element.querySelector('tbody tr')
    props.items = [row('stable', { item: { id: 'stable', name: 'Updated' } })]
    await settle()
    expect(element.querySelector('tbody tr')).toBe(before)
    expect(text(before!)).toContain('Updated')
  })
  it('retains safe richtext formatting and omits unsupported image nodes', () => {
    const { element } = richText(
      rich([
        { type: 'heading', attrs: { level: 2 }, content: [{ type: 'text', text: 'Heading' }] },
        {
          type: 'paragraph',
          content: [
            { type: 'text', text: 'Bold', marks: [{ type: 'bold' }] },
            { type: 'hardBreak' },
            { type: 'text', text: 'Italic', marks: [{ type: 'italic' }] },
          ],
        },
        { type: 'image', attrs: { src: 'https://example.invalid/never-requested.png' } },
      ]),
    )
    expect(element.querySelector('h2')?.textContent).toBe('Heading')
    expect(element.querySelector('strong')?.textContent).toBe('Bold')
    expect(element.querySelector('em')?.textContent).toBe('Italic')
    expect(element.querySelector('br')).not.toBeNull()
    expect(element.querySelector('img')).toBeNull()
  })
  it('renders malformed/non-doc richtext as escaped text', () => {
    const { element } = richText('<b data-audit-fallback>not JSON</b>')
    expect(element.querySelector('[data-audit-fallback]')).toBeNull()
    expect(text(element)).toContain('<b data-audit-fallback>not JSON</b>')
  })
})

describe('DataGrid passing contracts: empty, loading, error and URL state', () => {
  it('shows configured empty text and one callback per empty-state create click', async () => {
    const onActionCreate = vi.fn()
    const { element } = grid(
      fixture({
        data: [],
        config: config({ ui: { emptyMessage: 'Nothing here', createButtonText: 'Add record' } }),
      }),
      { onActionCreate },
    )
    await settle()
    expect(text(element.querySelector('[role="status"]')!)).toContain('Nothing here')
    const create = buttons(element).filter((candidate) => text(candidate) === 'Add record')
    expect(create).toHaveLength(2)
    create[1].click()
    expect(onActionCreate.mock.calls).toEqual([[]])
  })
  it('shows loading instead of empty state before first result', async () => {
    fetchMock.mockReturnValue(new Promise<Response>(() => {}))
    const { element } = mount(DataGrid, { apiUrl: '/audit', syncWithUrl: false })
    await settle()
    expect(text(element)).toContain('Загрузка...')
    expect(text(element)).not.toContain('Нет данных')
  })
  it('keeps rows visible on failure, displays stale-data warning, and retries once', async () => {
    vi.spyOn(console, 'debug').mockImplementation(() => {})
    fetchMock
      .mockResolvedValueOnce({ ok: false, status: 403 } as Response)
      .mockResolvedValueOnce(response())
    const { element, componentRef } = grid()
    await settle()
    componentRef.value!.refreshData!()
    await settle()
    expect(text(element.querySelector('[role="alert"]')!)).toContain(
      'Показаны ранее загруженные данные.',
    )
    expect(element.querySelectorAll('tbody tr')).toHaveLength(1)
    button(element, 'Повторить').click()
    await settle()
    expect(fetchMock).toHaveBeenCalledTimes(2)
    expect(element.querySelector('[role="alert"]')).toBeNull()
  })
  it('debounces a real header input into one fetch and shows search-specific empty state', async () => {
    vi.useFakeTimers()
    fetchMock.mockResolvedValue(response(fixture({ data: [] })))
    const { element } = grid()
    await settle()
    await input(element.querySelector('input')!, 'missing')
    expect(fetchMock).not.toHaveBeenCalled()
    expect(text(element)).toContain('Загрузка...')
    await vi.advanceTimersByTimeAsync(500)
    await settle()
    expect(fetchMock).toHaveBeenCalledOnce()
    expect(query().get('search')).toBe('missing')
    const status = element.querySelector('[role="status"]')!
    expect(text(status)).toContain('Ничего не найдено')
    expect(status.querySelector('button')).toBeNull()
  })
  it('with syncWithUrl false neither reads nor rewrites the page query', async () => {
    window.history.replaceState({}, '', '/admin?search=preserve#anchor')
    grid()
    await settle()
    expect(fetchMock).not.toHaveBeenCalled()
    expect(window.location.search).toBe('?search=preserve')
    expect(window.location.hash).toBe('#anchor')
  })
  it('with syncWithUrl true loads URL search/page/sort/filters once over initial data', async () => {
    window.history.replaceState(
      {},
      '',
      '/admin?page=2&search=needle&sortBy=name&sortOrder=asc&status=active',
    )
    grid(fixture(), { syncWithUrl: true })
    await settle()
    expect(fetchMock).toHaveBeenCalledOnce()
    expect(Object.fromEntries(query())).toEqual({
      page: '2',
      search: 'needle',
      sortBy: 'name',
      sortOrder: 'asc',
      status: 'active',
    })
  })
})

describe('DataGrid regression fixes and remaining behavior boundaries', () => {
  it('fetches only the URL intent on mount without initialData', async () => {
    window.history.replaceState({}, '', '/admin?search=from-url')
    mount(DataGrid, { apiUrl: '/audit', syncWithUrl: true })
    await settle()
    expect(fetchMock).toHaveBeenCalledOnce()
    expect(query().get('search')).toBe('from-url')
  })
  it('loads browser popstate exactly once and rehydrates the search input', async () => {
    const { element } = grid(fixture(), { syncWithUrl: true })
    await settle()
    window.history.replaceState({}, '', '/admin?search=later-navigation')
    window.dispatchEvent(new PopStateEvent('popstate'))
    await settle()
    expect(fetchMock).toHaveBeenCalledOnce()
    expect(query().get('search')).toBe('later-navigation')
    expect(element.querySelector<HTMLInputElement>('input')?.value).toBe('later-navigation')
  })
  it('preserves browser history state and hash while synchronizing grid state', async () => {
    window.history.replaceState({ hostState: 'keep-me' }, '', '/admin#section')
    grid(fixture(), { syncWithUrl: true })
    await settle()
    expect(window.location.hash).toBe('#section')
    expect(window.history.state).toEqual({ hostState: 'keep-me' })
  })
  it('refreshData uses the latest apiUrl after apiUrl prop changes', async () => {
    vi.spyOn(console, 'debug').mockImplementation(() => {})
    const { props, componentRef } = grid()
    await settle()
    props.apiUrl = '/other'
    await settle()
    componentRef.value!.refreshData!()
    expect(String(fetchMock.mock.calls[0][0])).toMatch(/^\/other\/data\?/)
  })

  it('has no inline-create control even when inlineCreatable/show-inline-create are enabled', async () => {
    const onActionCreate = vi.fn()
    const { element } = grid(
      fixture({ config: config({ behaviour: { inlineCreatable: true } }) }),
      { onActionCreate },
    )
    await settle()
    expect(element.querySelectorAll('tbody tr')).toHaveLength(1)
    expect(buttons(element).filter((candidate) => text(candidate) === 'Создать')).toHaveLength(0)
    expect(onActionCreate).not.toHaveBeenCalled()
    // Capability boundary only, not a promised feature regression.
  })
  it('clear-all only clears configured filter keys and leaves extra active keys untouched', () => {
    const onFilterChange = vi.fn()
    const { element } = mount(DataGridFilters, {
      filterableColumns: [column('name')],
      filters: { hidden: 'retained' },
      onFilterChange,
    })
    button(element, 'Очистить все').click()
    expect(onFilterChange.mock.calls).toEqual([[{ name: '' }]])
  })
  it('treats a zero-valued filter as a filtered empty-state condition', async () => {
    const { element } = grid(
      fixture({ data: [], meta: { title: '', description: '', filters: { count: 0 } } }),
    )
    await settle()
    expect(text(element.querySelector('[role="status"]')!)).toContain('Ничего не найдено')
    expect(element.querySelector('[role="status"] button')).toBeNull()
  })
  it('loads URL limit when it is the only query parameter', async () => {
    window.history.replaceState({}, '', '/admin?limit=50')
    grid(fixture(), { syncWithUrl: true })
    await settle()
    expect(fetchMock).toHaveBeenCalledOnce()
    expect(query().get('limit')).toBe('50')
  })
  it('does not refetch when initialData or apiUrl props change after mount', async () => {
    const { props, element } = grid()
    await settle()
    props.initialData = fixture({
      data: [row('new', { item: { id: 'new', name: 'Changed prop' } })],
    })
    props.apiUrl = '/other'
    await settle()
    expect(text(element)).toContain('Record one')
    expect(text(element)).not.toContain('Changed prop')
    expect(fetchMock).not.toHaveBeenCalled()
  })
})

describe('DataGrid passing contracts: real dropdown and repeated lifecycle', () => {
  it('opens a select filter and emits the chosen value once as a one-key patch', async () => {
    const onFilterChange = vi.fn()
    const { element } = mount(DataGridFilters, {
      filterableColumns: [
        column('status', {
          type: 'select',
          options: [
            { value: 'a', label: 'Active' },
            { value: 'p', label: 'Paused' },
          ],
        }),
      ],
      filters: {},
      onFilterChange,
    })
    const trigger = element.querySelector<HTMLElement>('[role="combobox"]')!
    trigger.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
    await settle()
    const option = Array.from(document.querySelectorAll<HTMLElement>('[role="option"]')).find(
      (item) => text(item) === 'Paused',
    )!
    expect(option).not.toBeUndefined()
    option.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
    await settle()
    expect(onFilterChange.mock.calls).toEqual([[{ status: 'p' }]])
  })

  it('opens a custom row action and emits its exact key/raw row once', async () => {
    const onAction = vi.fn()
    const original = { id: 'custom', name: 'Raw custom' }
    const { element } = table(
      [row('custom', { item: original, actions: [{ key: 'archive', label: 'Archive record' }] })],
      { onAction },
    )
    const trigger = button(element, 'Еще')
    trigger.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
    await settle()
    const action = document.querySelector<HTMLElement>('[role="menuitem"]')!
    expect(action).not.toBeNull()
    expect(text(action)).toBe('Archive record')
    action.click()
    await settle()
    expect(onAction.mock.calls).toEqual([['archive', original]])
  })
  it('mounts/unmounts grids containing custom dropdowns repeatedly without detached menus', async () => {
    for (let run = 0; run < 3; run++) {
      const { app, element } = grid(
        fixture({
          data: Array.from({ length: 10 }, (_, id) =>
            row(id, {
              actions: [
                { key: 'view', label: `View ${id}` },
                { key: 'archive', label: `Archive ${id}` },
              ],
            }),
          ),
        }),
      )
      await settle()
      expect(element.querySelectorAll('tbody tr')).toHaveLength(10)
      app.unmount()
      apps.splice(apps.indexOf(app), 1)
      element.remove()
      await settle()
      expect(document.querySelector('[role="menu"]')).toBeNull()
    }
  })
})

describe('DataGrid repaired baseline defects: real table UI and safe rendering', () => {
  it('updates real checkbox state through the full grid without fetching', async () => {
    const { element } = grid(
      fixture({
        data: [row(0), row('b')],
        config: config({ behaviour: { selectable: true } }),
      }),
    )
    await settle()
    const controls = Array.from(element.querySelectorAll<HTMLButtonElement>('[role="checkbox"]'))
    expect(controls).toHaveLength(3)
    controls[1].click()
    await settle()
    expect(controls.map((control) => control.getAttribute('aria-checked'))).toEqual([
      'false',
      'true',
      'false',
    ])
    controls[0].click()
    await settle()
    expect(controls.map((control) => control.getAttribute('aria-checked'))).toEqual([
      'true',
      'true',
      'true',
    ])
    controls[0].click()
    await settle()
    expect(controls.map((control) => control.getAttribute('aria-checked'))).toEqual([
      'false',
      'false',
      'false',
    ])
    expect(fetchMock).not.toHaveBeenCalled()
  })
  it('fetches exactly once per sortable button activation through the full grid', async () => {
    const { element } = grid()
    await settle()
    const sortButton = element.querySelector<HTMLButtonElement>('th button')!
    sortButton.click()
    await settle()
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(query(0).get('sortBy')).toBe('name')
    expect(query(0).get('sortOrder')).toBe('asc')
    expect(element.querySelector('th[aria-sort="ascending"]')).not.toBeNull()
    sortButton.click()
    await settle()
    expect(fetchMock).toHaveBeenCalledTimes(2)
    expect(query(1).get('sortOrder')).toBe('desc')
    expect(element.querySelector('th[aria-sort="descending"]')).not.toBeNull()
  })
  it('selectable mode resolves the real Checkbox primitive without component warnings', () => {
    const { element, warnings } = table([row()], {
      config: config({ behaviour: { selectable: true } }),
    })
    expect(
      warnings.some((message) => message.includes('Failed to resolve component: Checkbox')),
    ).toBe(false)
    expect(element.querySelector('checkbox')).toBeNull()
    expect(element.querySelectorAll('button[role="checkbox"]')).toHaveLength(2)
  })
  it('clicking the real row Checkbox emits exactly one selection callback', () => {
    const onToggleSelectItem = vi.fn()
    const { element } = table([row()], {
      config: config({ behaviour: { selectable: true } }),
      onToggleSelectItem,
    })
    element.querySelectorAll<HTMLElement>('[role="checkbox"]')[1].click()
    expect(onToggleSelectItem.mock.calls).toEqual([['uuid-1']])
  })
  it('zero and false stay visible in standard value cells', () => {
    const { element } = table(
      [row('r', { item: { id: 'r', zero: 0, falseValue: false }, actions: [] })],
      { config: config({ columns: [column('zero', { type: 'number' }), column('falseValue')] }) },
    )
    expect(Array.from(element.querySelectorAll('td')).map(text)).toEqual(['0', 'false'])
  })
  it('renders harmless richtext HTML-shaped text literally without injecting an element', () => {
    const { element } = richText(
      rich([
        {
          type: 'paragraph',
          content: [{ type: 'text', text: '<b data-audit-injection="text">harmless marker</b>' }],
        },
      ]),
    )
    expect(element.querySelector('b[data-audit-injection="text"]')).toBeNull()
    expect(text(element)).toContain('<b data-audit-injection="text">harmless marker</b>')
  })
  it('rejects a javascript link URL and escapes harmless link-attribute breakout', () => {
    const { element } = richText(
      rich([
        {
          type: 'paragraph',
          content: [
            {
              type: 'text',
              text: 'Do not click',
              marks: [{ type: 'link', attrs: { href: 'javascript:void(0)' } }],
            },
            {
              type: 'text',
              text: 'Marker',
              marks: [{ type: 'link', attrs: { href: '#" data-audit-link="injected' } }],
            },
          ],
        },
      ]),
    )
    expect(element.querySelectorAll('a')).toHaveLength(1)
    expect(element.querySelector('a')?.getAttribute('href')).toBe('#" data-audit-link="injected')
    expect(element.querySelector('a[data-audit-link="injected"]')).toBeNull()
    expect(text(element)).toContain('Do not click')
    // No injected script or event handler executes; no link is clicked.
  })
  it('preserves an empty action cell when other rows have an action column', () => {
    const { element } = table([row('a'), row('b', { actions: [] })])
    expect(element.querySelectorAll('th')).toHaveLength(3)
    expect(
      Array.from(element.querySelectorAll('tbody tr')).map(
        (entry) => entry.querySelectorAll('td').length,
      ),
    ).toEqual([3, 3])
  })
})

describe('DataGrid repaired regression guards: normal passing assertions', () => {
  it('selectable mode should render an accessible, operable checkbox', () => {
    const { element } = table([row()], { config: config({ behaviour: { selectable: true } }) })
    expect(element.querySelectorAll('[role="checkbox"], input[type="checkbox"]')).toHaveLength(2)
  })
  it('numeric zero should remain visible in a number cell', () => {
    const { element } = table([row('r', { item: { id: 'r', count: 0 }, actions: [] })], {
      config: config({ columns: [column('count', { type: 'number' })] }),
    })
    expect(text(element.querySelector('td')!)).toBe('0')
  })
  it('richtext text-node content should be escaped rather than become HTML', () => {
    const { element } = richText(
      rich([
        {
          type: 'paragraph',
          content: [{ type: 'text', text: '<b data-audit-injection>marker</b>' }],
        },
      ]),
    )
    expect(element.querySelector('[data-audit-injection]')).toBeNull()
  })
  it('richtext links should reject javascript URLs', () => {
    const { element } = richText(
      rich([
        {
          type: 'paragraph',
          content: [
            {
              type: 'text',
              text: 'link',
              marks: [{ type: 'link', attrs: { href: 'javascript:void(0)' } }],
            },
          ],
        },
      ]),
    )
    expect(element.querySelector('a')?.getAttribute('href') || '').not.toMatch(/^javascript:/i)
  })
  it('rows without actions should preserve table column alignment with an empty action cell', () => {
    const { element } = table([row('a'), row('b', { actions: [] })])
    expect(element.querySelectorAll('tbody tr')[1].querySelectorAll('td')).toHaveLength(3)
  })
})
