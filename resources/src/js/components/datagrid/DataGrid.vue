<template>
  <div class="data-grid space-y-6">
    <DataGridHeader
      :meta="metaInfo"
      :config="config"
      :search-query="searchQuery"
      @search="handleSearch"
      @create="handleCreate"
      @refresh="handleRefresh"
      @export="handleExport"
    />

    <DataGridFilters
      :filterable-columns="filterableColumns"
      :filters="filters"
      @filter-change="handleFilterChange"
    />

    <div
      v-if="loadError"
      role="alert"
      class="rounded-md border border-error/30 bg-error/10 p-4 text-error"
    >
      <p>{{ loadError }}</p>
      <p v-if="items.length" class="mt-1 text-sm">Показаны ранее загруженные данные.</p>
      <AppButton type="button" variant="outline" class="mt-3" @click="retryLoad()"
        >Повторить</AppButton
      >
    </div>

    <DataGridTable
      v-if="items.length > 0"
      :config="config"
      :items="items"
      :sort-by="sortBy"
      :sort-order="sortOrder"
      :selected-items="selectedItems"
      :all-selected="allSelected"
      :show-inline-create="config.behaviour?.inlineCreatable && items.length > 0"
      @sort="handleSort"
      @toggle-select-all="toggleSelectAll"
      @toggle-select-item="toggleSelectItem"
      @action="handleActionRow"
      @inline-create="handleCreate"
    />

    <DataGridPagination
      v-if="items.length > 0"
      :pagination="pagination"
      @page-change="handlePageChange"
    />

    <!-- Пустое состояние -->
    <div
      v-if="!loading && !loadError && hasLoaded && items.length === 0"
      class="text-center py-16"
      role="status"
    >
      <div class="mx-auto h-24 w-24 text-text-tertiary mb-4">
        <Inbox class="h-24 w-24" />
      </div>
      <h3 class="text-lg font-medium text-text-primary mb-2">
        {{
          searchQuery || Object.values(filters).some(Boolean) ? 'Ничего не найдено' : 'Нет данных'
        }}
      </h3>
      <p class="text-text-secondary mb-6">
        {{
          searchQuery || Object.values(filters).some(Boolean)
            ? 'Измените поиск или фильтры и попробуйте снова.'
            : config.ui?.emptyMessage || 'Нет данных для отображения'
        }}
      </p>
      <AppButton
        v-if="config.behaviour?.creatable && !searchQuery && !Object.values(filters).some(Boolean)"
        @click="handleCreate"
      >
        <Plus class="-ml-1 mr-2 h-4 w-4" />
        {{ config.ui?.createButtonText || 'Создать' }}
      </AppButton>
    </div>

    <!-- Загрузка -->
    <div v-if="loading" class="text-center py-16" role="status" aria-live="polite">
      <div class="inline-flex items-center">
        <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary mr-3"></div>
        <span class="text-text-secondary text-lg">Загрузка...</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { nextTick, onMounted, watch } from 'vue'
import { type ApiResponse, type DataItemValue, useDataGrid } from '~/composables/useDataGrid.ts'
import { Inbox, Plus } from 'lucide-vue-next'
import AppButton from '@/components/ui/AppButton.vue'
import DataGridHeader from './DataGridHeader.vue'
import DataGridFilters from './DataGridFilters.vue'
import DataGridTable from './DataGridTable.vue'
import DataGridPagination from './DataGridPagination.vue'

// Props interface
interface Props {
  apiUrl: string
  initialData?: ApiResponse | null
  syncWithUrl?: boolean
}

// Define props with defaults
const props = withDefaults(defineProps<Props>(), {
  initialData: null,
  syncWithUrl: true,
})

// Define emits для передачи событий наружу
const emit = defineEmits<{
  'action-create': []
  'action-refresh': []
  'action-export': []
  'action-table': [actionKey: string, val: string | number | null]
  'action-row': [actionKey: string, item: DataItemValue]
  'page-change': [urlOrPage: string | number | null]
}>()

const {
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
  allSelected,

  // Actions
  handleSearch,
  handleFilterChange,
  handleSort,
  goToPage,
  goToPageByUrl,
  toggleSelectAll,
  toggleSelectItem,
  loadData,
  retryLoad,
} = useDataGrid({
  apiUrl: props.apiUrl,
  initialData: props.initialData,
})

// Флаг для предотвращения циклической синхронизации
let isUpdatingFromURL = false

// Функция для обновления URL
const updateURL = (): void => {
  if (!props.syncWithUrl) return
  if (isUpdatingFromURL) return
  if (typeof window === 'undefined') return

  const params = new URLSearchParams()

  // Добавляем параметры только если они отличаются от значений по умолчанию
  if (pagination.value?.currentPage && pagination.value.currentPage > 1) {
    params.set('page', String(pagination.value.currentPage))
  }

  if (sortBy.value && sortBy.value !== 'createdAt') {
    params.set('sortBy', sortBy.value)
  }

  if (sortOrder.value && sortOrder.value !== 'desc') {
    params.set('sortOrder', sortOrder.value)
  }

  if (searchQuery.value) {
    params.set('search', searchQuery.value)
  }

  // Добавляем фильтры
  Object.entries(filters.value).forEach(([key, value]) => {
    if (value !== null && value !== undefined && value !== '') {
      params.set(key, String(value))
    }
  })

  // Обновляем URL без перезагрузки страницы
  const newUrl = params.toString()
    ? `${window.location.pathname}?${params}`
    : window.location.pathname
  window.history.replaceState({}, '', newUrl)
}

// Функция для инициализации состояния из URL
const initFromURL = (): void => {
  if (!props.syncWithUrl) return
  if (typeof window === 'undefined') return

  const urlParams = new URLSearchParams(window.location.search)

  // Извлекаем параметры из URL и загружаем данные если они есть
  const page = urlParams.get('page')
  const search = urlParams.get('search')
  const sortByParam = urlParams.get('sortBy')
  const sortOrderParam = urlParams.get('sortOrder')

  // Извлекаем фильтры (все параметры кроме стандартных)
  const urlFilters: Record<string, unknown> = {}
  urlParams.forEach((value, key) => {
    if (!['page', 'limit', 'sortBy', 'sortOrder', 'search'].includes(key)) {
      urlFilters[key] = value
    }
  })

  // Если есть параметры в URL, загружаем данные с ними
  if (page || search || sortByParam || sortOrderParam || Object.keys(urlFilters).length > 0) {
    const loadParams: {
      page?: number
      search?: string
      sortBy?: string
      sortOrder?: 'asc' | 'desc'
      filters?: Record<string, unknown>
    } = {}

    if (page) loadParams.page = parseInt(page, 10)
    if (search) loadParams.search = search
    if (sortByParam) loadParams.sortBy = sortByParam
    if (sortOrderParam) loadParams.sortOrder = sortOrderParam as 'asc' | 'desc'
    if (Object.keys(urlFilters).length > 0) loadParams.filters = urlFilters

    isUpdatingFromURL = true
    // Используем loadData из текущего экземпляра useDataGrid
    loadData(loadParams).finally(() => {
      isUpdatingFromURL = false
    })
  }
}

// Watchers для синхронизации с URL
watch(
  [pagination, sortBy, sortOrder, searchQuery, filters],
  () => {
    nextTick(() => updateURL())
  },
  { deep: true },
)

const handleCreate = (): void => {
  emit('action-create')
}

const handleRefresh = (): void => {
  emit('action-table', 'refresh', null)
}

const handleExport = (): void => {
  emit('action-table', 'export', null)
}

const handlePageChange = (urlOrPage: string | number | null): void => {
  if (typeof urlOrPage === 'string') {
    goToPageByUrl(urlOrPage)
  } else if (typeof urlOrPage === 'number') {
    goToPage(urlOrPage)
  }

  // Передаем событие наружу
  emit('page-change', urlOrPage)
}

const handleActionRow = (actionKey: string, item: DataItemValue): void => {
  emit('action-row', actionKey, item)
}

// Метод для обновления данных извне (для использования через ref)
const refreshData = (): void => {
  console.debug('Refreshing DataGrid data...')
  loadData()
}

// Expose метод для использования через ref
defineExpose({
  refreshData,
})

// Инициализация из URL при монтировании
onMounted(() => {
  initFromURL()
})
</script>
