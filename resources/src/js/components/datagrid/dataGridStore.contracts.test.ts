import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useDataGridStore } from '../../stores/dataGrid'
import type { ApiResponse } from '../../composables/useDataGrid'

// This store is not wired into DataGrid.vue; these are store-helper contracts,
// not evidence that the grid exports files or performs bulk actions itself.
const fixture = (): ApiResponse => ({
  data: [
    {
      item: { id: 1, name: 'Raw', count: 0, enabled: false },
      values: { name: 'Computed' },
      actions: [],
    },
  ],
  config: {
    columns: [
      { key: 'name', title: 'Name', label: 'Name' },
      { key: 'count', title: 'Count', label: 'Count' },
      { key: 'enabled', title: 'Enabled', label: 'Enabled' },
    ],
  },
})
beforeEach(() => {
  setActivePinia(createPinia())
  vi.useFakeTimers()
  vi.setSystemTime(new Date('2026-01-01T00:00:00Z'))
})
afterEach(() => vi.useRealTimers())

describe('dataGrid store passing helper contracts', () => {
  it('returns cache miss and no selection/export for an unknown key', () => {
    const store = useDataGridStore()
    expect(store.getCachedData('missing')).toBeNull()
    expect(store.isCacheValid('missing')).toBe(false)
    expect(store.getSelectedItems('missing')).toEqual([])
    expect(store.exportData('missing')).toBeNull()
  })
  it('caches independently per grid and expires at the strict five-minute boundary', () => {
    const store = useDataGridStore()
    store.cacheData('a', fixture())
    store.cacheData('b', { data: [] })
    expect(store.getCachedData('a')?.data).toHaveLength(1)
    expect(store.getCachedData('b')?.data).toEqual([])
    vi.advanceTimersByTime(299999)
    expect(store.isCacheValid('a')).toBe(true)
    vi.advanceTimersByTime(1)
    expect(store.isCacheValid('a')).toBe(false)
    expect(store.getCachedData('a')).not.toBeNull()
  })
  it('accepts custom maximum age and refreshes the timestamp on cache write', () => {
    const store = useDataGridStore()
    store.cacheData('a', fixture())
    vi.advanceTimersByTime(1000)
    expect(store.isCacheValid('a', 1000)).toBe(false)
    store.cacheData('a', fixture())
    expect(store.isCacheValid('a', 1000)).toBe(true)
  })
  it('clears data/timestamp/selection for only the requested grid', () => {
    const store = useDataGridStore()
    for (const key of ['a', 'b']) {
      store.cacheData(key, fixture())
      store.setSelectedItems(key, [0, 'uuid'])
    }
    store.clearCache('a')
    expect(store.getCachedData('a')).toBeNull()
    expect(store.getSelectedItems('a')).toEqual([])
    expect(store.lastUpdated.has('a')).toBe(false)
    expect(store.getCachedData('b')).not.toBeNull()
    expect(store.getSelectedItems('b')).toEqual([0, 'uuid'])
  })
  it('clears every map without a key', () => {
    const store = useDataGridStore()
    store.cacheData('a', fixture())
    store.setSelectedItems('a', [1])
    store.clearCache()
    expect(store.cache.size).toBe(0)
    expect(store.lastUpdated.size).toBe(0)
    expect(store.selectedItems.size).toBe(0)
  })
  it('JSON export serializes cached DataItem envelopes including values and actions', () => {
    const store = useDataGridStore(),
      data = fixture()
    store.cacheData('a', data)
    expect(JSON.parse(store.exportData('a')!)).toEqual(data.data)
  })
})

describe('dataGrid store CSV contracts: helper not wired to header export', () => {
  it('exports raw values rather than computed values and preserves zero/false', () => {
    const store = useDataGridStore()
    store.cacheData('a', fixture())
    expect(store.exportData('a', 'csv')).toBe('Name,Count,Enabled\n"Raw",0,false')
  })
  it('uses CSV doubled-quote escaping', () => {
    const store = useDataGridStore(),
      data = fixture()
    data.data[0].item.name = 'A "quote"'
    store.cacheData('a', data)
    expect(store.exportData('a', 'csv')).toBe('Name,Count,Enabled\n"A ""quote""",0,false')
  })
  it('escapes commas in column titles', () => {
    const store = useDataGridStore(),
      data = fixture()
    data.config!.columns![0].title = 'Last, First'
    store.cacheData('a', data)
    expect(store.exportData('a', 'csv')?.split('\n')[0]).toBe('"Last, First",Count,Enabled')
  })
  it('does not neutralize formula-like string contents for spreadsheet import', () => {
    const store = useDataGridStore(),
      data = fixture()
    data.data[0].item.name = '=1+1'
    store.cacheData('a', data)
    expect(store.exportData('a', 'csv')?.split('\n')[1]).toBe('"=1+1",0,false')
  })
})

describe('dataGrid store corrected CSV regression guards', () => {
  it('preserves zero and false when serializing CSV', () => {
    const store = useDataGridStore()
    store.cacheData('a', fixture())
    expect(store.exportData('a', 'csv')).toBe('Name,Count,Enabled\n"Raw",0,false')
  })
  it('uses standard doubled quotes for CSV strings', () => {
    const store = useDataGridStore(),
      data = fixture()
    data.data[0].item.name = 'A "quote"'
    data.config!.columns = [data.config!.columns![0]]
    store.cacheData('a', data)
    expect(store.exportData('a', 'csv')?.split('\n')[1]).toBe('"A ""quote"""')
  })
})

// Formula-like values remain literal source data; this unused helper does not
// claim to sanitize content for automatic execution by spreadsheet applications.
it('quotes multiline values, JSON objects and null cells without dropping data', () => {
  const store = useDataGridStore()
  const data = fixture()
  data.data[0].item = { name: 'one,\ntwo "quoted"', count: { nested: 'value' }, enabled: null }
  data.config!.columns![0].title = 'Line\n"Heading"'
  store.cacheData('a', data)
  expect(store.exportData('a', 'csv')).toBe(
    '"Line\n""Heading""",Count,Enabled\n"one,\ntwo ""quoted""","{""nested"":""value""}",""',
  )
})
