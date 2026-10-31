import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { adminFetch, AdminFetchNotSentError, sessionCoordinator } from '@/http'
import type { useDataGrid } from './useDataGrid'

export interface GridViewState {
  search: string
  filters: Record<string, string>
  sortBy: string
  sortOrder: 'asc' | 'desc'
  limit: number
  columns: string[]
  knownColumns?: string[]
}
export interface GridView {
  id: string
  name: string
  state: GridViewState
}
export interface GridPreferences {
  schemaVersion: 1
  columns: string[]
  knownColumns?: string[]
  views: GridView[]
  defaultViewId?: string
}
export interface BulkResult {
  id: string
  status: 'succeeded' | 'skipped' | 'failed' | 'unknown' | 'not_processed'
  code?: string
  relatedId?: string
}
const emptyPreferences = (): GridPreferences => ({ schemaVersion: 1, columns: [], views: [] })
const clone = <T>(value: T): T => JSON.parse(JSON.stringify(value))

// Missing snapshots are legacy documents: baseline them without revealing hidden keys.
function reconcileColumns(value: { columns: string[]; knownColumns?: string[] }, keys: string[]) {
  const available = new Set(keys)
  const known = new Set(value.knownColumns ?? keys)
  const columns = [...new Set(value.columns)].filter((key) => available.has(key))
  for (const key of keys) if (!known.has(key) && !columns.includes(key)) columns.push(key)
  value.columns = columns.length ? columns : [...keys]
  value.knownColumns = [...keys]
}
const viewSignature = (value: GridViewState) =>
  JSON.stringify([
    value.search,
    Object.entries(value.filters)
      .filter(([, value]) => value !== '')
      .sort(([a], [b]) => a.localeCompare(b)),
    value.sortBy,
    value.sortOrder,
    value.limit,
    value.columns,
  ])

/** All asynchronous results belong to one mounted resource and document session. */
export function useDataGridWorkflow(
  grid: ReturnType<typeof useDataGrid>,
  allowDefault: boolean,
  onBulkComplete?: (results: BulkResult[]) => void,
) {
  const preferences = ref<GridPreferences>(emptyPreferences())
  const version = ref(0)
  const owner = ref('')
  const activeViewId = ref('')
  const viewName = ref('')
  const persistedViews = ref<GridView[]>([])
  const error = ref('')
  const notice = ref('')
  const ready = ref(false)
  const busy = ref(false)
  const conflict = ref(false)
  const results = ref<BulkResult[]>([])
  const pendingAction = ref('')
  const blockedIds = new Set<string>()
  let generation = 0
  let controller = new AbortController()
  let defaultEligible = allowDefault
  let mounted = false
  const workflow = computed(() => grid.config.value.workflow)
  const availableColumns = computed(() => grid.config.value.columns ?? [])
  const columns = computed(() => {
    const keys = preferences.value.columns.filter((key) =>
      availableColumns.value.some((col) => col.key === key),
    )
    return keys.length ? keys : availableColumns.value.map((col) => col.key)
  })
  const tableConfig = computed(() => ({
    ...grid.config.value,
    columns: columns.value.flatMap((key) =>
      availableColumns.value.filter((col) => col.key === key),
    ),
    behaviour: {
      ...grid.config.value.behaviour,
      selectable: workflow.value ? true : grid.config.value.behaviour?.selectable,
    },
  }))
  const reset = () => {
    generation++
    controller.abort()
    controller = new AbortController()
    preferences.value = emptyPreferences()
    persistedViews.value = []
    viewName.value = ''
    owner.value = ''
    version.value = 0
    ready.value = false
    busy.value = false
    conflict.value = false
    error.value = ''
    notice.value = ''
    activeViewId.value = ''
    results.value = []
    pendingAction.value = ''
    blockedIds.clear()
    grid.selectedItems.value = []
  }
  const invalidate = () => {
    reset()
    defaultEligible = false
    grid.clearSensitiveData()
  }
  let unsubscribe = () => {}
  const handleSessionState = (state: string) => {
    if (['session_changed', 'reauthentication_required', 'access_denied'].includes(state))
      invalidate()
  }
  const current = (token: number, fence: ReturnType<typeof sessionCoordinator.commandFence>) => {
    const now = sessionCoordinator.commandFence()
    return (
      token === generation &&
      fence.generation === now.generation &&
      fence.epoch === now.epoch &&
      !controller.signal.aborted
    )
  }
  const state = (): GridViewState => ({
    search: grid.searchQuery.value,
    filters: Object.fromEntries(
      Object.entries(grid.filters.value).map(([key, value]) => [key, String(value ?? '')]),
    ),
    sortBy: grid.sortBy.value,
    sortOrder: grid.sortOrder.value,
    limit: grid.pagination.value?.perPage ?? 10,
    columns: [...columns.value],
    knownColumns: availableColumns.value.map((column) => column.key),
  })
  const reconcilePreferences = () => {
    const keys = availableColumns.value.map((column) => column.key)
    reconcileColumns(preferences.value, keys)
    for (const view of preferences.value.views) reconcileColumns(view.state, keys)
    for (const view of persistedViews.value) reconcileColumns(view.state, keys)
  }
  watch(() => availableColumns.value.map((column) => column.key).join('|'), reconcilePreferences, {
    flush: 'sync',
  })
  watch(
    activeViewId,
    (id) => {
      viewName.value = preferences.value.views.find((view) => view.id === id)?.name ?? ''
    },
    { flush: 'sync' },
  )
  const viewDirty = computed(() => {
    if (!activeViewId.value) return false
    const saved = persistedViews.value.find((view) => view.id === activeViewId.value)
    return (
      !saved ||
      viewName.value !== saved.name ||
      viewSignature(state()) !== viewSignature(saved.state)
    )
  })
  watch(
    [() => viewSignature(state()), viewName],
    () => {
      notice.value = ''
    },
    { flush: 'sync' },
  )
  const applyView = (id: string) => {
    defaultEligible = false
    if (!id) {
      activeViewId.value = ''
      viewName.value = ''
      grid.selectedItems.value = []
      return
    }
    const view = preferences.value.views.find((view) => view.id === id)
    if (!view || busy.value || grid.navigationSuspended.value) return
    activeViewId.value = id
    viewName.value = view.name
    grid.selectedItems.value = []
    preferences.value.columns = [...view.state.columns]
    preferences.value.knownColumns = [
      ...(view.state.knownColumns ?? availableColumns.value.map((column) => column.key)),
    ]
    void grid.loadData({ ...view.state, page: 1 })
  }
  const loadPreferences = async () => {
    const url = workflow.value?.preferencesUrl
    if (!url) return
    const token = generation
    const fence = sessionCoordinator.commandFence()
    const initialQuery = JSON.stringify(state())
    busy.value = true
    error.value = ''
    try {
      const response = await adminFetch(url, { signal: controller.signal })
      if (!response.ok) throw new Error('Не удалось загрузить представления.')
      const data = await response.json()
      if (!current(token, fence)) return
      if (
        !data ||
        typeof data.owner !== 'string' ||
        !Number.isSafeInteger(data.version) ||
        data.payload?.schemaVersion !== 1 ||
        !Array.isArray(data.payload.views) ||
        !Array.isArray(data.payload.columns)
      )
        throw new Error('Не удалось загрузить представления.')
      if (owner.value && owner.value !== data.owner) {
        invalidate()
        return
      }
      owner.value = data.owner
      version.value = data.version
      preferences.value = clone(data.payload)
      reconcilePreferences()
      persistedViews.value = clone(preferences.value.views)
      if (activeViewId.value) {
        const selected = preferences.value.views.find((view) => view.id === activeViewId.value)
        if (!selected) activeViewId.value = ''
        viewName.value = selected?.name ?? ''
      }
      ready.value = true
      conflict.value = false
      busy.value = false
      // User edits, URL navigation and history always win over a late default response.
      if (
        defaultEligible &&
        initialQuery ===
          JSON.stringify({ ...state(), columns: JSON.parse(initialQuery).columns }) &&
        preferences.value.defaultViewId
      )
        applyView(preferences.value.defaultViewId)
      defaultEligible = false
    } catch {
      if (current(token, fence)) error.value = 'Не удалось загрузить представления.'
    } finally {
      if (current(token, fence)) busy.value = false
    }
  }
  watch(
    () => [workflow.value?.key, workflow.value?.preferencesUrl].join('|'),
    (_key, previous) => {
      if (previous !== undefined) defaultEligible = false
      reset()
      if (mounted && workflow.value) void loadPreferences()
    },
    { immediate: true, flush: 'sync' },
  )
  onMounted(() => {
    mounted = true
    unsubscribe = sessionCoordinator.subscribe(handleSessionState)
    handleSessionState(sessionCoordinator.getState())
    if (workflow.value) void loadPreferences()
  })
  watch(
    grid.navigationSuspended,
    (suspended) => {
      if (suspended) {
        generation++
        controller.abort()
        controller = new AbortController()
        busy.value = false
        notice.value = ''
        results.value = []
        defaultEligible = false
        grid.selectedItems.value = []
        pendingAction.value = ''
      }
    },
    { flush: 'sync' },
  )
  watch(
    () => grid.pagination.value?.currentPage,
    (page, previous) => {
      if (previous !== undefined && page !== previous) defaultEligible = false
    },
    { flush: 'sync' },
  )
  // Explicit conditions reset cross-page selection even when history applies them directly.
  watch(
    () =>
      JSON.stringify([
        grid.searchQuery.value,
        Object.entries(grid.filters.value)
          .map(([key, value]) => [key, String(value ?? '')])
          .sort(([a], [b]) => a!.localeCompare(b!)),
      ]),
    () => {
      grid.selectedItems.value = []
    },
    { flush: 'sync' },
  )

  const savePreferences = async () => {
    if (!ready.value || busy.value || conflict.value || !workflow.value) return
    reconcilePreferences()
    if (
      preferences.value.views.length > 20 ||
      new TextEncoder().encode(JSON.stringify(preferences.value)).length > 65536
    ) {
      error.value = 'Не более 20 представлений и 64 КиБ настроек.'
      return
    }
    const savedPayload = clone(preferences.value)
    const token = generation
    const fence = sessionCoordinator.commandFence()
    busy.value = true
    error.value = ''
    notice.value = ''
    try {
      const response = await adminFetch(workflow.value.preferencesUrl, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ version: version.value, payload: savedPayload }),
        signal: controller.signal,
      })
      if (!current(token, fence)) return
      if (response.status === 409) {
        conflict.value = true
        error.value =
          'Настройки изменены в другом окне. Изменения сохранены в редакторе. Загрузите серверную версию для продолжения.'
        return
      }
      if (!response.ok) throw new Error()
      const data = await response.json()
      if (!current(token, fence)) return
      if (data.owner !== owner.value) {
        invalidate()
        return
      }
      version.value = data.version
      persistedViews.value = clone(savedPayload.views)
      notice.value = viewDirty.value ? '' : 'Настройки сохранены.'
    } catch {
      if (current(token, fence))
        error.value = 'Не удалось сохранить настройки. Изменения остались в редакторе.'
    } finally {
      if (current(token, fence)) busy.value = false
    }
  }
  const saveView = (name: string, replace = false) => {
    if (!name.trim() || busy.value || !ready.value || conflict.value) return
    const existing = replace
      ? preferences.value.views.find((view) => view.id === activeViewId.value)
      : undefined
    if (existing) {
      existing.name = name.trim()
      existing.state = state()
    } else {
      if (preferences.value.views.length >= 20) {
        error.value = 'Не более 20 представлений и 64 КиБ настроек.'
        return
      }
      const id = crypto.randomUUID()
      preferences.value.views.push({ id, name: name.trim(), state: state() })
      activeViewId.value = id
    }
    viewName.value = name.trim()
    void savePreferences()
  }
  const deleteView = () => {
    if (!ready.value || busy.value || conflict.value) return
    preferences.value.views = preferences.value.views.filter(
      (view) => view.id !== activeViewId.value,
    )
    if (preferences.value.defaultViewId === activeViewId.value)
      delete preferences.value.defaultViewId
    activeViewId.value = ''
    void savePreferences()
  }
  const setDefault = () => {
    if (!ready.value || busy.value || conflict.value) return
    if (activeViewId.value) preferences.value.defaultViewId = activeViewId.value
    else delete preferences.value.defaultViewId
    void savePreferences()
  }
  const toggleColumn = (key: string) => {
    if (columns.value.includes(key)) {
      if (columns.value.length > 1)
        preferences.value.columns = columns.value.filter((value) => value !== key)
    } else preferences.value.columns = [...columns.value, key]
  }
  const moveColumn = (index: number, direction: number) => {
    const next = [...columns.value]
    const target = index + direction
    if (target < 0 || target >= next.length) return
    ;[next[index], next[target]] = [next[target]!, next[index]!]
    preferences.value.columns = next
  }
  const exportCSV = async (scope: 'selected' | 'filtered') => {
    if (
      !workflow.value ||
      busy.value ||
      grid.loading.value ||
      grid.loadError.value ||
      (scope === 'selected' && !grid.selectedItems.value.length)
    )
      return
    const token = generation
    const fence = sessionCoordinator.commandFence()
    const url = new URL(workflow.value.exportUrl, window.location.href)
    const query = state()
    if (scope === 'filtered') {
      for (const [key, value] of Object.entries(query.filters))
        if (value) url.searchParams.set(key, value)
      url.searchParams.set('search', query.search)
      url.searchParams.set('sortBy', query.sortBy)
      url.searchParams.set('sortOrder', query.sortOrder)
    } else url.searchParams.set('ids', grid.selectedItems.value.join(','))
    url.searchParams.set('scope', scope)
    url.searchParams.set('columns', columns.value.join(','))
    busy.value = true
    error.value = ''
    try {
      const response = await adminFetch(url.href, {
        headers: { Accept: 'text/csv' },
        signal: controller.signal,
      })
      if (
        !response.ok ||
        response.headers.get('content-type')?.split(';')[0]?.trim() !== 'text/csv'
      )
        throw new Error()
      const blob = await response.blob()
      if (!current(token, fence)) return
      const objectUrl = URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = objectUrl
      link.download = `${workflow.value?.key ?? 'data'}.csv`
      document.body.append(link)
      try {
        link.click()
      } finally {
        link.remove()
        setTimeout(() => URL.revokeObjectURL(objectUrl), 0)
      }
      if (response.headers.get('X-Goadmin-Export-Truncated') === 'true')
        notice.value = 'Экспорт ограничен 10 000 строками.'
    } catch {
      if (current(token, fence)) error.value = 'Не удалось экспортировать данные.'
    } finally {
      if (current(token, fence)) busy.value = false
    }
  }
  const executeBulk = async () => {
    const action = pendingAction.value
    const endpoint = workflow.value?.bulkUrl
    if (
      !endpoint ||
      !workflow.value?.bulkActions?.some((value) => value.key === action) ||
      busy.value ||
      !grid.selectedItems.value.length
    )
      return
    const ids = grid.selectedItems.value
      .map(String)
      .filter((id) => !blockedIds.has(id))
      .slice(0, 100)
    pendingAction.value = ''
    if (!ids.length) return
    const token = generation
    const fence = sessionCoordinator.commandFence()
    busy.value = true
    error.value = ''
    results.value = []
    // Take IDs out of the command surface before dispatch; uncertainty is never replayed.
    ids.forEach((id) => blockedIds.add(id))
    grid.selectedItems.value = []
    try {
      const response = await adminFetch(endpoint, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ action, ids }),
        signal: controller.signal,
      })
      if (!response.ok) throw new Error()
      const data = await response.json()
      if (!current(token, fence)) return
      results.value = ids.map((id) => {
        const result = Array.isArray(data.results)
          ? data.results.find((result: BulkResult) => result.id === id)
          : undefined
        return result &&
          ['succeeded', 'skipped', 'failed', 'unknown', 'not_processed'].includes(result.status)
          ? result
          : { id, status: 'unknown' }
      })
      // Only a complete, validated server reply can notify host counters.
      if (
        Array.isArray(data.results) &&
        ids.every((id) =>
          data.results.some(
            (result: BulkResult) =>
              result.id === id &&
              ['succeeded', 'skipped', 'failed', 'unknown', 'not_processed'].includes(
                result.status,
              ),
          ),
        )
      )
        onBulkComplete?.([...results.value])
    } catch (cause) {
      if (!current(token, fence)) return
      results.value = ids.map((id) => ({
        id,
        status: cause instanceof AdminFetchNotSentError ? 'not_processed' : 'unknown',
      }))
    } finally {
      if (current(token, fence)) {
        results.value
          .filter((result) => result.status !== 'unknown')
          .forEach((result) => blockedIds.delete(result.id))
        busy.value = false
        void grid.loadData()
      }
    }
  }
  onUnmounted(() => {
    reset()
    unsubscribe()
  })
  return {
    workflow,
    preferences,
    activeViewId,
    viewName,
    viewDirty,
    availableColumns,
    columns,
    tableConfig,
    error,
    notice,
    ready,
    busy,
    conflict,
    results,
    pendingAction,
    applyView,
    savePreferences,
    saveView,
    deleteView,
    setDefault,
    toggleColumn,
    moveColumn,
    exportCSV,
    executeBulk,
    loadPreferences,
    invalidate,
  }
}
