import { computed } from 'vue'
import { usePage } from '@inertiajs/vue3'
import { isDataGridFilterKey } from './useDataGrid'

const origin = 'https://goadmin.invalid'
const listKeys = new Set(['page', 'limit', 'sortBy', 'sortOrder', 'search'])
const maxReturnToBytes = 4096 // Same limit as the CRUD redirect handler.

/** A return target is always a list in this resource, never a redirect destination. */
export function recordListHref(
  basePath: string,
  target: string | null,
  returnToFilter = false,
): string {
  if (!target?.startsWith('/') || target.startsWith('//') || /[\\\u0000-\u0020]/.test(target))
    return basePath
  try {
    // Match the original path before URL normalizes dot segments.
    if (target.split(/[?#]/, 1)[0] !== basePath) return basePath
    const url = new URL(target, origin)
    if (url.origin !== origin || url.pathname !== basePath) return basePath
    const params = new URLSearchParams()
    for (const [key, value] of url.searchParams) {
      if ((key !== 'returnTo' || returnToFilter) && (listKeys.has(key) || isDataGridFilterKey(key)))
        params.set(key, value)
    }
    const query = params.toString()
    // URLSearchParams serializes the query to ASCII, including non-ASCII search terms.
    const href = basePath + (query ? `?${query}` : '')
    return href.length <= maxReturnToBytes ? href : basePath
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
    return recordPath(basePath, id, edit, source)
  }
  return { listHref, recordHref }
}

export function recordActionHref(
  basePath: string,
  id: unknown,
  edit: boolean,
  returnTo: string,
): string | undefined {
  if ((typeof id !== 'string' && typeof id !== 'number') || id === '') return
  return recordPath(basePath, id, edit, recordListHref(basePath, returnTo))
}

function recordPath(basePath: string, id: unknown, edit: boolean, source: string): string {
  const params = new URLSearchParams({ returnTo: source })
  return `${basePath}/${encodeURIComponent(String(id))}${edit ? '/edit' : ''}${source === basePath ? '' : `?${params}`}`
}
