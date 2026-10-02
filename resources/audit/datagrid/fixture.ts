import type {
  ApiResponse,
  Column,
  DataItem,
  DataItemValue,
  Pagination,
} from '../../src/js/composables/useDataGrid'

/** Test-only in-memory host. This is deliberately not a production endpoint. */
export interface Trace {
  sequence: number
  at: number
  kind: string
  args: unknown[]
}
export type Permission = 'full' | 'readonly' | 'mixed'
export const columns: Column[] = [
  { key: 'id', label: 'ID', title: 'ID', type: 'number', sortable: true, filterable: true },
  {
    key: 'name',
    label: 'Название',
    title: 'Название',
    type: 'text',
    sortable: true,
    filterable: true,
  },
  {
    key: 'amount',
    label: 'Сумма (включая 0)',
    title: 'Сумма',
    type: 'number',
    sortable: true,
    filterable: true,
  },
  {
    key: 'status',
    label: 'Статус',
    title: 'Статус',
    type: 'select',
    filterable: true,
    options: [
      { value: 'active', label: 'Активна' },
      { value: 'paused', label: 'Приостановлена' },
    ],
    badges: {
      active: { label: 'Активна', variant: 'success' },
      paused: { label: 'Приостановлена', variant: 'warning' },
    },
  },
  { key: 'enabled', label: 'Включена', title: 'Включена', type: 'boolean', filterable: true },
  {
    key: 'createdAt',
    label: 'Дата',
    title: 'Дата',
    type: 'date',
    sortable: true,
    filterable: true,
    format: 'date',
  },
  { key: 'summary', label: 'Rich text', title: 'Rich text', type: 'richtext' },
  { key: 'image', label: 'Image (текстовый fallback)', title: 'Image', type: 'image' },
  { key: 'badge', label: 'Badge (текстовый fallback)', title: 'Badge', type: 'badge' },
]

export function makeRows(count: number): DataItemValue[] {
  return Array.from({ length: count }, (_, i) => ({
    id: i + 1,
    name: `Запись ${String(i + 1).padStart(4, '0')}`,
    amount: i % 11 === 0 ? 0 : i * 7,
    status: i % 3 ? 'active' : 'paused',
    enabled: i % 2 === 0,
    createdAt: new Date(Date.UTC(2026, 0, 1 + (i % 200))).toISOString(),
    summary: JSON.stringify({
      type: 'doc',
      content: [
        {
          type: 'paragraph',
          content: [{ type: 'text', text: `Текст ${i + 1}`, marks: [{ type: 'bold' }] }],
        },
      ],
    }),
    image: '/fixture-image.svg',
    badge: i % 2 ? 'gold' : 'silver',
  }))
}

export function rowActions(row: DataItemValue, permission: Permission): DataItem['actions'] {
  if (permission === 'readonly') return [{ key: 'view', label: `Просмотр #${row.id}` }]
  if (permission === 'mixed' && Number(row.id) % 5 === 0) return []
  return [
    { key: 'view', label: `Просмотр #${row.id}` },
    { key: 'show', label: `Show #${row.id}` },
    { key: 'edit', label: `Изменить #${row.id}` },
    { key: 'delete', label: `Удалить #${row.id}`, variant: 'danger' },
    { key: 'archive', label: `Архивировать #${row.id}`, variant: 'warning' },
    { key: 'duplicate', label: `Дублировать #${row.id}`, variant: 'success' },
  ]
}

export class MockGridHost {
  rows = makeRows(1000)
  permission: Permission = 'mixed'
  pageSize = 100
  latency = 50
  nextStatus = 200
  computedValues = true
  sequence = 0
  requestSequence = 0
  constructor(readonly record: (kind: string, ...args: unknown[]) => void = () => {}) {}

  response(params = new URLSearchParams()): ApiResponse {
    const search = params.get('search') || ''
    const sortBy = params.get('sortBy') || 'createdAt'
    const sortOrder = params.get('sortOrder') || 'desc'
    const filters = Object.fromEntries(
      [...params].filter(([k]) => !['page', 'limit', 'sortBy', 'sortOrder', 'search'].includes(k)),
    )
    const filtered = this.rows.filter(
      (row) =>
        (!search || String(row.name).toLowerCase().includes(search.toLowerCase())) &&
        Object.entries(filters).every(
          ([key, val]) => val === '' || String(row[key]).toLowerCase().includes(val.toLowerCase()),
        ),
    )
    const sorted = [...filtered].sort((a, b) => {
      const av = a[sortBy]
      const bv = b[sortBy]
      const cmp =
        typeof av === 'number' && typeof bv === 'number'
          ? av - bv
          : String(av).localeCompare(String(bv))
      return (sortOrder === 'asc' ? 1 : -1) * cmp || Number(a.id) - Number(b.id)
    })
    // The fixture deliberately supports 1000 rendered rows. The real GET handler caps at 100.
    const limit = Math.max(1, Math.min(1000, Number(params.get('limit')) || this.pageSize))
    const totalPages = Math.max(1, Math.ceil(sorted.length / limit))
    const page = Math.max(1, Math.min(totalPages, Number(params.get('page')) || 1))
    const url = (n: number) => {
      const q = new URLSearchParams(params)
      q.set('page', String(n))
      q.set('limit', String(limit))
      return `/audit/data?${q}`
    }
    const pagination: Pagination = {
      currentPage: page,
      perPage: limit,
      total: sorted.length,
      totalPages,
      from: sorted.length ? (page - 1) * limit + 1 : 0,
      to: Math.min(page * limit, sorted.length),
      firstPageUrl: url(1),
      lastPageUrl: url(totalPages),
      nextPageUrl: page < totalPages ? url(page + 1) : undefined,
      prevPageUrl: page > 1 ? url(page - 1) : undefined,
      links: [
        { label: 'Previous', url: page > 1 ? url(page - 1) : null, active: false },
        ...Array.from({ length: Math.min(totalPages, 12) }, (_, i) => ({
          label: String(i + 1),
          url: url(i + 1),
          active: i + 1 === page,
        })),
        { label: 'Next', url: page < totalPages ? url(page + 1) : null, active: false },
      ],
    }
    return {
      data: sorted.slice((page - 1) * limit, page * limit).map((item) => {
        const actions = rowActions(item, this.permission)
        this.record('host.permission.resolve', {
          id: item.id,
          policy: this.permission,
          actions: actions.map((action) => action.key),
        })
        return {
          item,
          actions,
          values: this.computedValues ? { name: `${item.name} · вычислено` } : undefined,
        }
      }),
      meta: {
        title: 'DataGrid · лаборатория контрактов',
        description:
          'Реальный компонент + детерминированный mock API. Данные полностью синтетические.',
        pagination,
        sorting: { sortBy, sortOrder: sortOrder as 'asc' | 'desc' },
        filters: { ...filters, ...(search ? { _search: search } : {}) },
      },
      config: {
        columns,
        ui: {
          idKey: 'id',
          createButtonText: 'Создать запись',
          emptyMessage: 'Mock API вернул пустой набор',
        },
        behaviour: {
          searchable: true,
          selectable: true,
          creatable: this.permission !== 'readonly',
          refreshable: true,
          exportable: true,
          inlineCreatable: true,
        },
      },
    }
  }

  fetch = async (input: RequestInfo | URL): Promise<Response> => {
    const url = new URL(String(input), 'http://localhost')
    if (url.pathname !== '/audit/data')
      throw new Error(`Fixture refuses non-mock request: ${url.pathname}`)
    const id = ++this.requestSequence
    const status = this.nextStatus
    this.nextStatus = 200
    const delay = this.latency
    const response = this.response(url.searchParams)
    this.record('api.request', {
      id,
      method: 'GET',
      pathname: url.pathname,
      params: Object.fromEntries(url.searchParams),
      delay,
      status,
    })
    await new Promise((resolve) => setTimeout(resolve, delay))
    this.record('api.response', {
      id,
      status,
      rows: response.data.length,
      ids: response.data.map((row) => row.item.id),
      pagination: response.meta?.pagination,
    })
    return new Response(JSON.stringify(response), {
      status,
      headers: { 'Content-Type': 'application/json' },
    })
  }

  submit(row: DataItemValue, mode: 'create' | 'edit') {
    this.record('host.form.submit', { mode, row })
    if (!String(row.name || '').trim()) {
      this.record('host.form.invalid', { name: 'Название обязательно' })
      return { name: 'Название обязательно' }
    }
    if (mode === 'edit')
      this.rows = this.rows.map((old) => (old.id === row.id ? { ...old, ...row } : old))
    else
      this.rows = [
        ...this.rows,
        { ...makeRows(1)[0], ...row, id: Math.max(0, ...this.rows.map((r) => Number(r.id))) + 1 },
      ]
    this.record('host.form.saved', { mode, count: this.rows.length })
    return null
  }
}
