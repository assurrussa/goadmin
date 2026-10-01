import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { ApiResponse } from '@/composables/useDataGrid'

export const useDataGridStore = defineStore('dataGrid', () => {
  // State
  const cache = ref<Map<string, ApiResponse>>(new Map())
  const selectedItems = ref<Map<string, (string | number)[]>>(new Map())
  const lastUpdated = ref<Map<string, Date>>(new Map())

  // Actions
  const cacheData = (key: string, data: ApiResponse) => {
    cache.value.set(key, data)
    lastUpdated.value.set(key, new Date())
  }

  const getCachedData = (key: string): ApiResponse | null => {
    return cache.value.get(key) || null
  }

  const isCacheValid = (key: string, maxAge: number = 5 * 60 * 1000): boolean => {
    const lastUpdate = lastUpdated.value.get(key)
    if (!lastUpdate) return false

    return Date.now() - lastUpdate.getTime() < maxAge
  }

  const setSelectedItems = (gridKey: string, items: (string | number)[]) => {
    selectedItems.value.set(gridKey, items)
  }

  const getSelectedItems = (gridKey: string): (string | number)[] => {
    return selectedItems.value.get(gridKey) || []
  }

  const clearCache = (key?: string) => {
    if (key) {
      cache.value.delete(key)
      lastUpdated.value.delete(key)
      selectedItems.value.delete(key)
    } else {
      cache.value.clear()
      lastUpdated.value.clear()
      selectedItems.value.clear()
    }
  }

  const exportData = (gridKey: string, format: 'json' | 'csv' = 'json') => {
    const data = getCachedData(gridKey)
    if (!data) return null

    if (format === 'json') {
      return JSON.stringify(data.data, null, 2)
    }

    if (format === 'csv') {
      const headers = data.config?.columns?.map((col) => col.title).join(',') || ''
      const rows = data.data
        .map((item) =>
          data.config?.columns?.map((col) => JSON.stringify(item.item[col.key] || '')).join(','),
        )
        .join('\n')

      return `${headers}\n${rows}`
    }

    return null
  }

  return {
    // State
    cache,
    selectedItems,
    lastUpdated,

    // Actions
    cacheData,
    getCachedData,
    isCacheValid,
    setSelectedItems,
    getSelectedItems,
    clearCache,
    exportData,
  }
})
