import { computed, onMounted, onUnmounted, ref, type Ref } from 'vue'

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
  apiUrl: string
  initialData?: ApiResponse | null
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

export function useDataGrid({ apiUrl, initialData }: UseDataGridParams) {
  // Reactive state
  const loading: Ref<boolean> = ref(false)
  const loadError = ref<string | null>(null)
  const hasLoaded = ref(false)
  let requestSequence = 0
  let lastRequest: LoadDataParams = {}
  const metaInfo: Ref<Meta | null> = ref(null)
  const items: Ref<DataItem[]> = ref([])
  const pagination: Ref<Pagination | null> = ref(null)
  const config: Ref<Config> = ref({})
  const filters: Ref<Record<string, unknown>> = ref({})
  const searchQuery: Ref<string> = ref('')
  const sortBy: Ref<string> = ref('')
  const sortOrder: Ref<'asc' | 'desc'> = ref('desc')
  const selectedItems: Ref<(string | number)[]> = ref([])

  // Refs для debounce
  let searchTimeout: ReturnType<typeof setTimeout> | null = null
  let filterTimeout: ReturnType<typeof setTimeout> | null = null

  // Computed values
  const filterableColumns = computed((): Column[] => {
    return config.value.columns?.filter((col) => col.filterable) || []
  })

  const hasFilters = computed((): boolean => filterableColumns.value.length > 0)

  const allSelected = computed((): boolean => {
    return selectedItems.value.length === items.value.length && items.value.length > 0
  })

  // Основная функция загрузки данных
  const loadData = async (params: LoadDataParams = {}): Promise<void> => {
    lastRequest = { ...params, filters: params.filters ? { ...params.filters } : undefined }
    const request = ++requestSequence
    loading.value = true
    loadError.value = null
    if (params.sortBy !== undefined) sortBy.value = params.sortBy
    if (params.sortOrder !== undefined) sortOrder.value = params.sortOrder
    if (params.search !== undefined) searchQuery.value = params.search
    if (params.filters !== undefined) filters.value = params.filters
    try {
      const searchParams = new URLSearchParams({
        page: String(params.page || pagination.value?.currentPage || 1),
        limit: String(params.limit || pagination.value?.perPage || 10),
        sortBy: params.sortBy || sortBy.value || '',
        sortOrder: params.sortOrder || sortOrder.value || 'desc',
        search: params.search !== undefined ? params.search : searchQuery.value || '',
      })

      // Добавляем фильтры
      const currentFilters = params.filters || filters.value
      Object.entries(currentFilters).forEach(([key, value]) => {
        if (value !== null && value !== undefined && value !== '') {
          searchParams.set(key, String(value))
        }
      })

      const response = await fetch(`${apiUrl}/data?${searchParams}`)
      if (!response.ok) {
        if (response.status === 401)
          throw new Error('Сеанс истёк. Войдите снова и повторите попытку.')
        if (response.status === 403) throw new Error('У вас нет доступа к этим данным.')
        throw new Error('Не удалось загрузить данные. Проверьте соединение и повторите попытку.')
      }

      const result: ApiResponse = await response.json()
      if (request !== requestSequence) return

      items.value = result.data || []
      selectedItems.value = []
      metaInfo.value = result.meta || null
      pagination.value = result.meta?.pagination || result.pagination || null
      config.value = result.config || {}
      hasLoaded.value = true
    } catch (error) {
      if (request === requestSequence) {
        loadError.value = error instanceof Error ? error.message : 'Не удалось загрузить данные.'
      }
    } finally {
      if (request === requestSequence) loading.value = false
    }
  }
  const retryLoad = (): Promise<void> => loadData(lastRequest)

  // Initialize data
  const initializeData = (): void => {
    let data: ApiResponse | null = null
    if (initialData && typeof initialData === 'object' && initialData.data) {
      data = initialData
    } else if (window.AdminDataGrid) {
      try {
        data =
          typeof window.AdminDataGrid === 'string'
            ? JSON.parse(window.AdminDataGrid)
            : window.AdminDataGrid
      } catch (e) {
        console.error('Error parsing AdminDataGrid:', e)
      }
    }

    if (data && data.data) {
      items.value = data.data
      hasLoaded.value = true
      metaInfo.value = data.meta || null
      pagination.value = data.meta?.pagination || null
      config.value = data.config || {}
      sortBy.value = data.meta?.sorting?.sortBy || ''
      sortOrder.value = data.meta?.sorting?.sortOrder || 'desc'
      filters.value = { ...(data.meta?.filters || {}) }
      searchQuery.value = ''
    } else {
      loadData()
    }
  }

  // Search handler с debounce
  const handleSearch = (query: string): void => {
    requestSequence++
    loading.value = true
    searchQuery.value = query

    // Очищаем предыдущий таймер
    if (searchTimeout) {
      clearTimeout(searchTimeout)
    }

    // Устанавливаем новый таймер
    searchTimeout = setTimeout(() => {
      loadData({
        search: query,
        page: 1,
        filters: filters.value,
        sortBy: sortBy.value,
        sortOrder: sortOrder.value,
      })
      if (pagination.value) {
        pagination.value = { ...pagination.value, currentPage: 1 }
      }
    }, 500)
  }

  // Filter handler с debounce
  const handleFilterChange = (newFilters: Record<string, unknown>): void => {
    requestSequence++
    loading.value = true
    const updatedFilters = { ...filters.value, ...newFilters }
    filters.value = updatedFilters

    // Очищаем предыдущий таймер
    if (filterTimeout) {
      clearTimeout(filterTimeout)
    }

    // Устанавливаем новый таймер
    filterTimeout = setTimeout(() => {
      loadData({
        filters: updatedFilters,
        page: 1,
        search: searchQuery.value,
        sortBy: sortBy.value,
        sortOrder: sortOrder.value,
      })
      if (pagination.value) {
        pagination.value = { ...pagination.value, currentPage: 1 }
      }
    }, 300)
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
    if (page < 1 || page > (pagination.value?.totalPages || 1)) return

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
      const page = parseInt(urlObj.searchParams.get('page') || '1', 10)
      const limit = parseInt(urlObj.searchParams.get('limit') || '10', 10)

      // Извлекаем все параметры из URL
      const params: LoadDataParams = {
        page,
        limit,
        sortBy: urlObj.searchParams.get('sortBy') || '',
        sortOrder: (urlObj.searchParams.get('sortOrder') as 'asc' | 'desc') || 'desc',
        search: urlObj.searchParams.get('search') || '',
      }

      // Извлекаем фильтры
      const currentFilters: Record<string, unknown> = {}
      urlObj.searchParams.forEach((value, key) => {
        if (!['page', 'limit', 'sortBy', 'sortOrder', 'search'].includes(key)) {
          currentFilters[key] = value
        }
      })
      params.filters = currentFilters

      loadData(params)
    } catch (error) {
      console.error('Error parsing pagination URL:', error)
    }
  }

  // Selection handlers
  const toggleSelectAll = (): void => {
    if (allSelected.value) {
      selectedItems.value = []
    } else {
      selectedItems.value = items.value
        .map((item) => item.item[config.value.ui?.idKey || 'id'])
        .filter((id): id is string | number => typeof id === 'string' || typeof id === 'number')
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
    initializeData()
  })

  onUnmounted(() => {
    requestSequence++
    if (searchTimeout) {
      clearTimeout(searchTimeout)
    }
    if (filterTimeout) {
      clearTimeout(filterTimeout)
    }
  })

  return {
    // State
    metaInfo,
    loading,
    loadError,
    hasLoaded,
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
    retryLoad,
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
