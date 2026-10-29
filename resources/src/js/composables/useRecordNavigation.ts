import { computed } from 'vue'
import { usePage } from '@inertiajs/vue3'
import { isDataGridFilterKey } from './useDataGrid'

const origin = 'https://goadmin.invalid'
const listKeys = new Set(['page', 'limit', 'sortBy', 'sortOrder', 'search'])

/** A return target is always a list in this resource, never a redirect destination. */
export function recordListHref(basePath: string, target: string | null): string {
  if (!target?.startsWith('/') || target.startsWith('//') || /[\\\u0000-\u0020]/.test(target))
    return basePath
  try {
    const url = new URL(target, origin)
    if (url.origin !== origin || url.pathname !== basePath) return basePath
    const params = new URLSearchParams()
    for (const [key, value] of url.searchParams) {
      if (key !== 'returnTo' && (listKeys.has(key) || isDataGridFilterKey(key)))
        params.set(key, value)
    }
    const query = params.toString()
    return basePath + (query ? `?${query}` : '')
  } catch {
    return basePath
  }
}

export function useRecordNavigation(basePath: string) {
  const page = usePage()
  const listHref = computed(() => {
    const url = new URL(page.url || basePath, origin)
    return recordListHref(basePath, url.searchParams.get('returnTo'))
  })
  function recordHref(id: unknown, edit = false): string {
    // DataGrid synchronizes its latest state with replaceState, independently of page.url.
    const source =
      typeof window !== 'undefined' && window.location.pathname === basePath
        ? recordListHref(basePath, window.location.pathname + window.location.search)
        : listHref.value
    const params = new URLSearchParams({ returnTo: source })
    return `${basePath}/${encodeURIComponent(String(id))}${edit ? '/edit' : ''}${source === basePath ? '' : `?${params}`}`
  }
  return { listHref, recordHref }
}
