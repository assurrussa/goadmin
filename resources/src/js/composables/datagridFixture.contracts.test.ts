import { describe, expect, it, vi, afterEach } from 'vitest'
import { MockGridHost, makeRows, rowActions } from '../../../audit/datagrid/fixture'
afterEach(() => vi.useRealTimers())
describe('audit fixture only (not production API)', () => {
  it.each([100, 1000])('builds deterministic unique %i-row responses', (count) => {
    const host = new MockGridHost()
    host.rows = makeRows(count)
    host.pageSize = count
    const result = host.response()
    expect(result.data).toHaveLength(count)
    expect(new Set(result.data.map((row) => row.item.id)).size).toBe(count)
    expect(result.meta?.pagination?.total).toBe(count)
    expect(result.config?.behaviour?.inlineCreatable).toBe(true)
  })
  it('combines sorting, filtering, search and pagination while preserving query state', () => {
    const result = new MockGridHost().response(
      new URLSearchParams('page=2&limit=10&sortBy=id&sortOrder=asc&search=00&status=active'),
    )
    expect(result.data).toHaveLength(10)
    expect(
      result.data.every(
        (row) => row.item.status === 'active' && String(row.item.name).includes('00'),
      ),
    ).toBe(true)
    expect(result.data.map((row) => row.item.id)).toEqual(
      [...result.data.map((row) => row.item.id)].sort((a, b) => Number(a) - Number(b)),
    )
    expect(result.meta?.pagination?.nextPageUrl).toContain('status=active')
    expect(result.meta?.filters?._search).toBe('00')
  })
  it('records request and response order and consumes one failure', async () => {
    vi.useFakeTimers()
    const record = vi.fn()
    const host = new MockGridHost(record)
    host.nextStatus = 403
    const pending = host.fetch('/audit/data?page=1&limit=100')
    expect(record.mock.calls.filter((call) => call[0] === 'api.request')).toHaveLength(1)
    await vi.runAllTimersAsync()
    expect((await pending).status).toBe(403)
    expect(record.mock.calls.at(-1)?.[0]).toBe('api.response')
    expect(host.nextStatus).toBe(200)
  })
  it('models permissions by omitted row actions, not a new grid authorization layer', () => {
    expect(rowActions({ id: 5 }, 'mixed')).toEqual([])
    expect(rowActions({ id: 1 }, 'readonly').map((a) => a.key)).toEqual(['view'])
    expect(rowActions({ id: 1 }, 'full').map((a) => a.key)).toEqual([
      'view',
      'show',
      'edit',
      'delete',
      'archive',
      'duplicate',
    ])
  })
  it('validates host forms before modifying fixture and traces exact callback order', () => {
    const record = vi.fn()
    const host = new MockGridHost(record)
    expect(host.submit({ name: '' }, 'create')).toEqual({ name: 'Название обязательно' })
    expect(host.rows).toHaveLength(1000)
    expect(host.submit({ name: 'new', amount: 0 }, 'create')).toBeNull()
    expect(host.rows).toHaveLength(1001)
    expect(host.rows.at(-1)?.amount).toBe(0)
    expect(record.mock.calls.map((call) => call[0])).toEqual([
      'host.form.submit',
      'host.form.invalid',
      'host.form.submit',
      'host.form.saved',
    ])
  })
})
