import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { downloadUserExport } from './userExport'

const fetchMock = vi.fn()
const createURL = vi.fn(() => 'blob:export')
const revokeURL = vi.fn()
const clickedLinks: HTMLAnchorElement[] = []
const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (
  this: HTMLAnchorElement,
) {
  clickedLinks.push(this)
})

beforeEach(() => {
  vi.useFakeTimers()
  clickedLinks.length = 0
  vi.stubGlobal('fetch', fetchMock)
  Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: createURL })
  Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: revokeURL })
})
afterEach(() => {
  vi.runOnlyPendingTimers()
  vi.useRealTimers()
  vi.clearAllMocks()
  vi.unstubAllGlobals()
})

function response(truncated = false) {
  return {
    ok: true,
    headers: new Headers({
      'content-type': 'text/csv; charset=utf-8',
      'content-disposition': 'attachment; filename="users-20261002.csv"',
      'x-goadmin-export-truncated': String(truncated),
    }),
    blob: async () => new Blob(['id,name\n1,User']),
  }
}

describe('users CSV transfer', () => {
  it.each([false, true])(
    'preserves matching query and reports truncation=%s',
    async (truncated) => {
      fetchMock.mockResolvedValue(response(truncated))
      const signal = new AbortController().signal
      const result = await downloadUserExport(
        '/users',
        'https://admin.test/users?page=4&limit=20&search=Alice&status=active&sortBy=createdAt&sortOrder=asc#section',
        signal,
      )
      const [url, options] = fetchMock.mock.calls[0]
      const target = new URL(url)
      expect(target.pathname).toBe('/users/export')
      expect(Object.fromEntries(target.searchParams)).toEqual({
        search: 'Alice',
        status: 'active',
        sortBy: 'createdAt',
        sortOrder: 'asc',
      })
      expect(target.hash).toBe('')
      expect(options).toEqual({
        credentials: 'same-origin',
        headers: { Accept: 'text/csv' },
        signal,
      })
      expect(click).toHaveBeenCalledOnce()
      expect(clickedLinks[0].download).toBe('users-20261002.csv')
      expect(document.querySelector('a[download]')).toBeNull()
      expect(result).toBe(truncated)
      vi.runOnlyPendingTimers()
      expect(revokeURL).toHaveBeenCalledWith('blob:export')
    },
  )

  it.each([
    { ok: false, headers: new Headers({ 'content-type': 'text/csv' }) },
    { ok: true, headers: new Headers({ 'content-type': 'text/html' }) },
  ])('never downloads an authorization/error page', async (invalid) => {
    fetchMock.mockResolvedValue(invalid)
    await expect(
      downloadUserExport('/users', 'https://admin.test/users', new AbortController().signal),
    ).rejects.toThrow('Не удалось')
    expect(click).not.toHaveBeenCalled()
    expect(createURL).not.toHaveBeenCalled()
  })

  it('discards a completed response after navigation cancels the export', async () => {
    const controller = new AbortController()
    fetchMock.mockResolvedValue({
      ...response(),
      blob: async () => {
        controller.abort()
        return new Blob()
      },
    })
    await expect(
      downloadUserExport('/users', 'https://admin.test/users', controller.signal),
    ).rejects.toThrow()
    expect(click).not.toHaveBeenCalled()
    expect(createURL).not.toHaveBeenCalled()
  })

  it('rejects external targets before sending credentials', async () => {
    await expect(
      downloadUserExport(
        'https://other.test/users',
        'https://admin.test/users',
        new AbortController().signal,
      ),
    ).rejects.toThrow('Недопустимый')
    expect(fetchMock).not.toHaveBeenCalled()
  })
})
