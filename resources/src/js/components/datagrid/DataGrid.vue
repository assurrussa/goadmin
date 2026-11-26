<template>
  <div data-ui="datagrid" data-part="root" class="data-grid space-y-6">
    <DataGridHeader
      ref="header"
      :meta="metaInfo"
      :config="headerConfig"
      :search-query="searchQuery"
      :export-disabled="loading || !!loadError"
      @search="handleSearch"
      @create="handleCreate"
      @refresh="handleRefresh"
      @export="handleExport"
    >
      <DataGridFilters
        :filterable-columns="filterableColumns"
        :filters="filters"
        @filter-change="handleFilterChange"
      />
    </DataGridHeader>

    <DataGridWorkflow
      v-if="config.workflow"
      :model="workflowModel"
      :selected-count="selectedItems.length"
      :loading="loading || !!loadError"
      @clear-selection="selectedItems = []"
    />

    <div
      v-if="loadError"
      role="alert"
      class="rounded-md border border-error/30 bg-error/10 p-4 text-error"
    >
      <p>{{ t(loadError) }}</p>
      <p v-if="items.length" class="mt-1 text-sm">{{ t('Показаны ранее загруженные данные.') }}</p>
      <AppButton type="button" variant="outline" class="mt-3" @click="retryLoad()">{{
        t('Повторить')
      }}</AppButton>
    </div>

    <DataGridTable
      v-if="items.length > 0"
      :config="workflowModel.tableConfig.value"
      :items="items"
      :sort-by="sortBy"
      :sort-order="sortOrder"
      :selected-items="selectedItems"
      :all-selected="allSelected"
      :selection-disabled="selectionDisabled"
      :format-cell="formatCell"
      :row-action-href="rowActionHref"
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
    <div v-if="!loading && !loadError && hasLoaded && items.length === 0" class="text-center py-16">
      <div class="mx-auto h-24 w-24 text-text-tertiary mb-4">
        <Inbox class="h-24 w-24" />
      </div>
      <div role="status">
        <h3 class="text-lg font-medium text-text-primary mb-2">
          {{ queryActive ? t('Ничего не найдено') : t('Нет данных') }}
        </h3>
        <p class="text-text-secondary mb-6">
          {{
            queryActive
              ? t('Измените поиск или фильтры и попробуйте снова.')
              : t(config.ui?.emptyMessage || 'Нет данных для отображения')
          }}
        </p>
      </div>
      <AppButton v-if="queryActive" variant="outline" @click="resetEmptyQuery">
        {{ t('Сбросить поиск и фильтры') }}
      </AppButton>
      <AppButton v-else-if="config.behaviour?.creatable" @click="handleCreate">
        <Plus class="-ml-1 mr-2 h-4 w-4" />
        {{ t(config.ui?.createButtonText || 'Создать') }}
      </AppButton>
    </div>

    <!-- Загрузка -->
    <div v-if="loading" class="text-center py-16" role="status" aria-live="polite">
      <div class="inline-flex items-center">
        <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary mr-3"></div>
        <span class="text-text-secondary text-lg">{{ t('Загрузка...') }}</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { t } from '@/i18n'

import { useAuthUserStore } from '@/stores/authUser'
import { useDataGridWorkflow, type BulkResult } from '@/composables/useDataGridWorkflow'
import DataGridWorkflow from './DataGridWorkflow.vue'
import { isGridHistoryRestoration } from './historyRestoration'
import {
  computed,
  getCurrentInstance,
  nextTick,
  onMounted,
  onUnmounted,
  ref,
  shallowRef,
  watch,
} from 'vue'
import {
  type ApiResponse,
  type DataItemValue,
  isActiveDataGridFilter,
  isDataGridFilterKey,
  parseDataGridQuery,
  useDataGrid,
} from '~/composables/useDataGrid.ts'
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
  navigationMode?: 'browser' | 'inertia'
  formatCell?: (key: string, value: unknown) => string | undefined
  resolveRowActionHref?: (key: string, item: DataItemValue, source: string) => string | undefined
}

// Define props with defaults
const props = withDefaults(defineProps<Props>(), {
  initialData: null,
  syncWithUrl: true,
  navigationMode: 'browser',
})

// Define emits для передачи событий наружу
const emit = defineEmits<{
  'action-create': []
  'action-refresh': []
  'action-export': []
  'action-table': [actionKey: string, val: string | number | null]
  'action-row': [actionKey: string, item: DataItemValue]
  'page-change': [urlOrPage: string | number | null]
  'bulk-complete': [results: BulkResult[]]
}>()

const grid = useDataGrid({
  apiUrl: () => props.apiUrl,
  initialData: () => props.initialData,
  initialQuery: () =>
    props.syncWithUrl && typeof window !== 'undefined' ? window.location.search : undefined,
  initialParams:
    props.syncWithUrl && typeof window !== 'undefined' && window.location.search
      ? parseDataGridQuery(new URLSearchParams(window.location.search), undefined, true)
      : undefined,
})
const workflowModel = useDataGridWorkflow(
  grid,
  props.syncWithUrl &&
    typeof window !== 'undefined' &&
    !window.location.search &&
    !isGridHistoryRestoration(),
  (results) => emit('bulk-complete', results),
)
const headerConfig = computed(() =>
  config.value.workflow
    ? { ...config.value, behaviour: { ...config.value.behaviour, exportable: false } }
    : config.value,
)
const pinia = getCurrentInstance()?.appContext.config.globalProperties.$pinia
if (pinia) {
  const auth = useAuthUserStore(pinia)
  watch(
    () => auth.id,
    () => workflowModel.invalidate(),
    { flush: 'sync' },
  )
}
const {
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
  allSelected,
  selectionDisabled,

  // Actions
  handleSearch,
  handleFilterChange,
  handleSort,
  goToPage,
  goToPageByUrl,
  toggleSelectAll,
  toggleSelectItem,
  loadData,
  loadDataFromUrl,
  retryLoad,
  suspendForNavigation,
} = grid

const queryActive = computed(
  () => !!searchQuery.value || Object.values(filters.value).some(isActiveDataGridFilter),
)

const header = ref<InstanceType<typeof DataGridHeader> | null>(null)
async function resetEmptyQuery(): Promise<void> {
  workflowModel.resetQuery()
  await nextTick()
  header.value?.focusControls()
}

let synchronizedLocation =
  typeof window === 'undefined' ? '' : window.location.pathname + window.location.search
// The URL in the address bar may belong to a pending history restoration.
const listSnapshot = shallowRef<{ rows: typeof items.value; href: string }>()
const listLocation = computed(() => {
  if (listSnapshot.value?.rows === items.value) return listSnapshot.value.href
  const query = metaInfo.value?.requestQuery
  return props.apiUrl + (query ? `?${query}` : '')
})

const rowActionHref = (key: string, item: DataItemValue) =>
  props.resolveRowActionHref?.(key, item, listLocation.value)

const updateURL = (): void => {
  if (
    !props.syncWithUrl ||
    navigationSuspended.value ||
    loading.value ||
    loadError.value ||
    typeof window === 'undefined'
  )
    return

  const params = new URLSearchParams()
  if (pagination.value?.currentPage && pagination.value.currentPage > 1) {
    params.set('page', String(pagination.value.currentPage))
  }
  if (pagination.value?.perPage) {
    params.set('limit', String(pagination.value.perPage))
  }
  if (sortBy.value) params.set('sortBy', sortBy.value)
  params.set('sortOrder', sortOrder.value)
  if (searchQuery.value) params.set('search', searchQuery.value)
  Object.entries(filters.value).forEach(([key, value]) => {
    if (isDataGridFilterKey(key) && isActiveDataGridFilter(value)) {
      params.set(key, String(value))
    }
  })

  const query = params.toString()
  const newUrl = `${window.location.pathname}${query ? `?${query}` : ''}${window.location.hash}`
  window.history.replaceState(window.history.state, '', newUrl)
  synchronizedLocation = window.location.pathname + window.location.search
  listSnapshot.value = { rows: items.value, href: synchronizedLocation }
}

const handlePopState = (event: PopStateEvent): void => {
  if (!props.syncWithUrl) return
  if (props.navigationMode === 'inertia') {
    // A null-state hash-only event does not restore an Inertia page.
    if (
      event.state !== null ||
      window.location.pathname + window.location.search !== synchronizedLocation
    )
      suspendForNavigation()
    return
  }
  void loadDataFromUrl(window.location.href)
}

watch(
  [pagination, sortBy, sortOrder, searchQuery, filters, loading, navigationSuspended],
  updateURL,
  { deep: true },
)

const handleCreate = (): void => {
  emit('action-create')
}

const handleRefresh = (): void => {
  emit('action-table', 'refresh', null)
}

const handleExport = (): void => {
  // URL state belongs to the last successful response until query synchronization finishes.
  if (loading.value || loadError.value) return
  if (config.value.workflow) void workflowModel.exportCSV('filtered')
  else emit('action-table', 'export', null)
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
  listLocation,
})

onMounted(() => {
  window.addEventListener('popstate', handlePopState)
})
onUnmounted(() => window.removeEventListener('popstate', handlePopState))
</script>
