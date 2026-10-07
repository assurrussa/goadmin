import {
  computed,
  onMounted,
  onUnmounted,
  ref,
  toValue,
  watch,
  type MaybeRefOrGetter,
  type Ref,
} from 'vue'

// Type definitions
interface DataItem {
  item: DataItemValue
  actions: Array<DataItemAction>
  values?: Record<string, unknown>
}

type DataItemValue = Record<string, unknown>

interface DataItemAction {
  key: string
  label: string
  icon?: string
  variant?: 'primary' | 'danger' | 'success' | 'warning'
}

interface Column {
  key: string
  label: string
  title: string
  sortable?: boolean
  filterable?: boolean
  type?: 'text' | 'number' | 'select' | 'date' | 'boolean' | 'image' | 'badge' | 'richtext'
  format?: string
  placeholder?: string
  options?: Array<{ value: string | number; label: string }>
  badges?: Record<string | number, { label: string; variant: string }>
}

interface Config {
  routePath?: string
  columns?: Column[]
  ui?: {
    title?: string
    description?: string
    createButtonText?: string
    emptyMessage?: string
    idKey?: string
  }
  behaviour?: {
    searchable?: boolean
    creatable?: boolean
    inlineCreatable?: boolean
    selectable?: boolean
    refreshable?: boolean
    exportable?: boolean
  }
}

interface PaginationLink {
  url: string | null // URL может быть null для неактивных ссылок
  label: string
  active: boolean
}

interface Pagination {
  currentPage: number
  perPage: number
  total: number
  totalPages: number
  // Laravel-style поля
  from: number
  to: number
  firstPageUrl?: string
  lastPageUrl?: string
  nextPageUrl?: string
  prevPageUrl?: string
  path?: string
  links: PaginationLink[] // Массив ссылок как в Laravel
}

interface ApiResponse {
  data: DataItem[]
  pagination?: Pagination
  meta?: Meta
  config?: Config
}

interface Meta {
  // Query that produced these rows; effective defaults remain in pagination/sorting/filters.
  requestQuery?: string
  title: string
  description: string
  pagination?: Pagination
  sorting?: {
    sortBy: string
    sortOrder: 'asc' | 'desc'
  }
  filters?: Record<string, unknown>
}

interface UseDataGridParams {
  apiUrl: MaybeRefOrGetter<string>
  initialData?: MaybeRefOrGetter<ApiResponse | null | undefined>
  initialParams?: LoadDataParams
  initialQuery?: MaybeRefOrGetter<string | undefined>
}

interface LoadDataParams {
  page?: number
  limit?: number
  sortBy?: string
  sortOrder?: 'asc' | 'desc'
  search?: string
  filters?: Record<string, unknown>
}

declare global {
  interface Window {
    AdminDataGrid?: string | ApiResponse | { dataResponse: string }
  }
}

const reservedQueryKeys = new Set(['page', 'limit', 'sortBy', 'sortOrder', 'search', '_search'])

export const isDataGridFilterKey = (key: string): boolean => !reservedQueryKeys.has(key)

const positiveInteger = (value: unknown, fallback: number): number => {
  const number = Number(value)
  return Number.isSafeInteger(number) && number > 0 ? number : fallback
}

export function parseDataGridQuery(
  params: URLSearchParams,
  defaultLimit?: number,
  preserveMissing = false,
): LoadDataParams {
  const parsed: LoadDataParams = {
    page: positiveInteger(params.get('page'), 1),
    limit: params.has('limit')
      ? positiveInteger(params.get('limit'), defaultLimit || 10)
      : defaultLimit,
    sortBy: params.get('sortBy') || '',
    sortOrder: params.get('sortOrder') === 'asc' ? 'asc' : 'desc',
    search: params.get('search') || '',
    filters: Object.fromEntries([...params].filter(([key]) => isDataGridFilterKey(key))),
  }
  if (preserveMissing) {
    for (const key of ['page', 'sortBy', 'sortOrder', 'search'] as const) {
      if (!params.has(key)) delete parsed[key]
    }
    if (Object.keys(parsed.filters || {}).length === 0) delete parsed.filters
  }
  return parsed
}

const ordinaryFilters = (values: Record<string, unknown>): Record<string, unknown> =>
  Object.fromEntries(Object.entries(values).filter(([key]) => isDataGridFilterKey(key)))

const canonicalQuery = (query: string): string => {
  const params = new URLSearchParams(query)
  params.sort()
  return params.toString()
}

// Response provenance is explicit: effective defaults alone cannot prove which URL was loaded.
export function matchesDataGridQuery(data: ApiResponse, query: string): boolean {
  return (
    typeof data.meta?.requestQuery === 'string' &&
    canonicalQuery(data.meta.requestQuery) === canonicalQuery(query)
  )
}

export function useDataGrid({
  apiUrl,
  initialData,
  initialParams,
  initialQuery,
}: UseDataGridParams) {
  // Reactive state
  const loading: Ref<boolean> = ref(false)
  const loadError = ref<string | null>(null)
  const hasLoaded = ref(false)
  let requestSequence = 0
  const navigationSuspended = ref(false)
  let activeController: AbortController | null = null
  const abortActiveRequest = (): void => {
    activeController?.abort()
    activeController = null
  }
  let lastRequest: LoadDataParams = {}
  let lastRequestUsesServerDefaults = false
  const metaInfo: Ref<Meta | null> = ref(null)
  const items: Ref<DataItem[]> = ref([])
  const pagination: Ref<Pagination | null> = ref(null)
  const config: Ref<Config> = ref({})
  const filters: Ref<Record<string, unknown>> = ref({})
  const searchQuery: Ref<string> = ref('')
  const sortBy: Ref<string> = ref('')
  const sortOrder: Ref<'asc' | 'desc'> = ref('desc')
  const selectedItems: Ref<(string | number)[]> = ref([])

  // Search and filter edits describe one query, so only the newest timer may run.
  let queryTimeout: ReturnType<typeof setTimeout> | null = null
  const cancelPendingQuery = (): void => {
    if (queryTimeout !== null) clearTimeout(queryTimeout)
    queryTimeout = null
  }

  // Changing endpoints affects subsequent requests; old endpoint work must never hydrate the new grid.
  watch(
    () => toValue(apiUrl),
    () => {
      requestSequence++
      abortActiveRequest()
      cancelPendingQuery()
      loading.value = navigationSuspended.value
      loadError.value = null
    },
    { flush: 'sync' },
  )

  // Computed values
  const filterableColumns = computed((): Column[] => {
    return config.value.columns?.filter((col) => col.filterable) || []
  })

  const hasFilters = computed((): boolean => filterableColumns.value.length > 0)

  const allSelected = computed((): boolean => {
    const selected = new Set(selectedItems.value)
    return (
      items.value.length > 0 &&
      items.value.every(({ item }) => {
        const id = item[config.value.ui?.idKey || 'id']
        return (typeof id === 'string' || typeof id === 'number') && selected.has(id)
      })
    )
  })

  const hydrateResponse = (result: ApiResponse): void => {
    items.value = result.data || []
    selectedItems.value = []
    metaInfo.value = result.meta || null
    pagination.value = result.meta?.pagination || result.pagination || null
    config.value = result.config || {}
    if (result.meta?.sorting) {
      sortBy.value = result.meta.sorting.sortBy
      sortOrder.value = result.meta.sorting.sortOrder === 'asc' ? 'asc' : 'desc'
    }
    if (result.meta?.filters) {
      filters.value = ordinaryFilters(result.meta.filters)
      searchQuery.value = String(result.meta.filters._search ?? '')
    }
    hasLoaded.value = true
  }

  // A direct request supersedes every pending debounce and snapshots the full intent for retry.
  const requestData = async (
    params: LoadDataParams = {},
    useServerDefaults = false,
  ): Promise<void> => {
    if (navigationSuspended.value) return
    cancelPendingQuery()
    const request = ++requestSequence
    abortActiveRequest()
    const controller = new AbortController()
    activeController = controller
    loading.value = true
    loadError.value = null
    if (params.sortBy !== undefined) sortBy.value = params.sortBy
    if (params.sortOrder !== undefined) sortOrder.value = params.sortOrder
    if (params.search !== undefined) searchQuery.value = params.search
    if (params.filters !== undefined) filters.value = ordinaryFilters(params.filters)
    const page = positiveInteger(
      params.page,
      pagination.value?.currentPage || lastRequest.page || 1,
    )
    const limit = positiveInteger(
      params.limit,
      pagination.value?.perPage || lastRequest.limit || 10,
    )
    lastRequest = {
      page,
      limit,
      sortBy: sortBy.value,
      sortOrder: sortOrder.value,
      search: searchQuery.value,
      filters: { ...filters.value },
    }
    lastRequestUsesServerDefaults = useServerDefaults
    // URL navigation must not mistake a previous response's effective settings for host defaults.
    if (useServerDefaults) {
      for (const key of ['limit', 'sortBy', 'sortOrder'] as const) {
        if (params[key] === undefined) delete lastRequest[key]
      }
    }
    if (pagination.value) {
      pagination.value = { ...pagination.value, currentPage: page, perPage: limit }
    }
    try {
      const searchParams = new URLSearchParams({
        page: String(page),
        limit: String(limit),
        sortBy: sortBy.value,
        sortOrder: sortOrder.value,
        search: searchQuery.value,
      })
      if (useServerDefaults) {
        for (const key of ['limit', 'sortBy', 'sortOrder'] as const) {
          if (params[key] === undefined) searchParams.delete(key)
        }
      }
      Object.entries(filters.value).forEach(([key, value]) => {
        if (isDataGridFilterKey(key) && value !== null && value !== undefined && value !== '') {
          searchParams.set(key, String(value))
        }
      })

      const response = await fetch(`${toValue(apiUrl)}/data?${searchParams}`, {
        signal: controller.signal,
      })
      // Avoid status handling and JSON decoding even when the transport ignores cancellation.
      if (request !== requestSequence || controller.signal.aborted) return
      if (!response.ok) {
        if (response.status === 401)
          throw new Error('Сеанс истёк. Войдите снова и повторите попытку.')
        if (response.status === 403) throw new Error('У вас нет доступа к этим данным.')
        throw new Error('Не удалось загрузить данные. Проверьте соединение и повторите попытку.')
      }

      const result: ApiResponse = await response.json()
      if (request !== requestSequence || controller.signal.aborted) return

      hydrateResponse(result)
    } catch (error) {
      if (
        request === requestSequence &&
        !controller.signal.aborted &&
        !(error && typeof error === 'object' && 'name' in error && error.name === 'AbortError')
      ) {
        loadError.value = error instanceof Error ? error.message : 'Не удалось загрузить данные.'
      }
    } finally {
      if (activeController === controller) activeController = null
      if (request === requestSequence) loading.value = false
    }
  }
  const loadData = (params: LoadDataParams = {}): Promise<void> => requestData(params)
  const retryLoad = (): Promise<void> => requestData(lastRequest, lastRequestUsesServerDefaults)
  const loadDataFromUrl = (url: string): Promise<void> => {
    const params = parseDataGridQuery(
      new URL(url, window.location.origin).searchParams,
      undefined,
      true,
    )
    return requestData(
      {
        ...params,
        page: params.page ?? 1,
        search: params.search ?? '',
        filters: params.filters ?? {},
      },
      true,
    )
  }

  const readInitialData = (allowGlobal: boolean): ApiResponse | null => {
    const provided = toValue(initialData)
    if (provided && Array.isArray(provided.data)) return provided
    if (allowGlobal && typeof window !== 'undefined' && window.AdminDataGrid) {
      try {
        const globalData = window.AdminDataGrid
        const data =
          typeof globalData === 'string'
            ? JSON.parse(globalData)
            : 'dataResponse' in globalData
              ? JSON.parse(globalData.dataResponse)
              : globalData
        if (data && Array.isArray(data.data)) return data
      } catch (error) {
        console.error('Error parsing AdminDataGrid:', error)
      }
    }
    return null
  }

  const acceptProvidedData = (data: ApiResponse): void => {
    navigationSuspended.value = false
    requestSequence++
    abortActiveRequest()
    cancelPendingQuery()
    loading.value = false
    loadError.value = null
    searchQuery.value = ''
    filters.value = {}
    sortBy.value = ''
    sortOrder.value = 'desc'
    lastRequest = {}
    lastRequestUsesServerDefaults = false
    hydrateResponse(data)
  }

  // Initialize synchronously for Vue SSR/hydration. Network work starts only on the client.
  const bootstrapData = readInitialData(true)
  if (bootstrapData) acceptProvidedData(bootstrapData)

  const initializeData = (allowGlobal: boolean): void => {
    const data = readInitialData(false) ?? (allowGlobal ? bootstrapData : null)
    if (data) acceptProvidedData(data)
    const query = toValue(initialQuery)
    const params =
      query !== undefined
        ? parseDataGridQuery(new URLSearchParams(query), undefined, true)
        : initialParams
    const routeMatches =
      !data?.config?.routePath ||
      data.config.routePath.replace(/\/$/, '') === toValue(apiUrl).replace(/\/$/, '')
    if (
      data &&
      routeMatches &&
      (params === undefined ||
        (query === '' && data.meta?.requestQuery === undefined) ||
        (query !== undefined && matchesDataGridQuery(data, query)))
    )
      return
    if (params !== undefined) {
      void requestData(
        {
          ...params,
          page: params.page ?? 1,
          search:
            params.search ?? (data?.meta?.requestQuery === undefined ? searchQuery.value : ''),
          filters: params.filters ?? (data?.meta?.requestQuery === undefined ? filters.value : {}),
        },
        true,
      )
    } else void loadData()
  }

  let mounted = false
  watch(
    () => toValue(initialData),
    () => {
      if (mounted) initializeData(false)
    },
  )

  // History restoration is asynchronous in Inertia. Invalidate immediately and keep
  // URL synchronization paused until authoritative props arrive or this grid unmounts.
  const suspendForNavigation = (): void => {
    navigationSuspended.value = true
    requestSequence++
    abortActiveRequest()
    cancelPendingQuery()
    loading.value = true
    loadError.value = null
  }

  const scheduleQuery = (delay: number): void => {
    if (navigationSuspended.value) return
    cancelPendingQuery()
    requestSequence++
    abortActiveRequest()
    loading.value = true
    loadError.value = null
    queryTimeout = setTimeout(() => {
      queryTimeout = null
      void loadData({ page: 1 })
    }, delay)
  }

  const handleSearch = (query: string): void => {
    searchQuery.value = query
    scheduleQuery(500)
  }

  const handleFilterChange = (newFilters: Record<string, unknown>): void => {
    filters.value = ordinaryFilters({ ...filters.value, ...newFilters })
    scheduleQuery(300)
  }

  // Sort handler
  const handleSort = (columnKey: string): void => {
    const column = config.value.columns?.find((col) => col.key === columnKey)
    if (!column?.sortable) return

    const newSortBy = columnKey
    let newSortOrder: 'asc' | 'desc' = 'asc'

    if (sortBy.value === columnKey) {
      newSortOrder = sortOrder.value === 'asc' ? 'desc' : 'asc'
    }

    sortBy.value = newSortBy
    sortOrder.value = newSortOrder

    loadData({
      sortBy: newSortBy,
      sortOrder: newSortOrder,
      search: searchQuery.value,
      filters: filters.value,
      page: pagination.value?.currentPage || 1,
    })
  }

  // Page handler
  const goToPage = (page: number): void => {
    if (!Number.isSafeInteger(page) || page < 1 || page > (pagination.value?.totalPages || 1))
      return

    if (pagination.value) {
      pagination.value = { ...pagination.value, currentPage: page }
    }
    loadData({
      page,
      search: searchQuery.value,
      filters: filters.value,
      sortBy: sortBy.value,
      sortOrder: sortOrder.value,
    })
  }

  // URL-based page handler для Laravel-style links
  const goToPageByUrl = (url: string): void => {
    if (!url) return

    try {
      const urlObj = new URL(url, window.location.origin)
      void loadData(parseDataGridQuery(urlObj.searchParams, pagination.value?.perPage || 10))
    } catch (error) {
      console.error('Error parsing pagination URL:', error)
    }
  }

  // Selection handlers
  const toggleSelectAll = (): void => {
    if (allSelected.value) {
      selectedItems.value = []
    } else {
      selectedItems.value = [
        ...new Set(
          items.value
            .map((item) => item.item[config.value.ui?.idKey || 'id'])
            .filter(
              (id): id is string | number => typeof id === 'string' || typeof id === 'number',
            ),
        ),
      ]
    }
  }

  const toggleSelectItem = (id: string | number): void => {
    const index = selectedItems.value.indexOf(id)
    if (index > -1) {
      selectedItems.value.splice(index, 1)
    } else {
      selectedItems.value.push(id)
    }
  }

  // Lifecycle
  onMounted(() => {
    mounted = true
    initializeData(true)
  })

  onUnmounted(() => {
    mounted = false
    requestSequence++
    abortActiveRequest()
    cancelPendingQuery()
  })

  return {
    // State
    metaInfo,
    loading,
    loadError,
    hasLoaded,
    navigationSuspended,
    items,
    pagination,
    config,
    filters,
    searchQuery,
    sortBy,
    sortOrder,
    selectedItems,

    // Computed
    filterableColumns,
    hasFilters,
    allSelected,

    // Actions
    loadData,
    loadDataFromUrl,
    retryLoad,
    suspendForNavigation,
    handleSearch,
    handleFilterChange,
    handleSort,
    goToPage,
    goToPageByUrl,
    toggleSelectAll,
    toggleSelectItem,
  }
}

// Export all types and functions
export type {
  Meta,
  Column,
  Config,
  Pagination,
  PaginationLink,
  ApiResponse,
  UseDataGridParams,
  LoadDataParams,
  DataItem,
  DataItemValue,
  DataItemAction,
}
