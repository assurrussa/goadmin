import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import {
  appearance,
  initAppearance,
  syncAppearance,
  editAppearance,
  reloadAppearance,
  saveAppearance,
  disposeAppearance,
  deviceKey,
} from './state'
import { deriveBrandPalette, safeLogo, emptyBrand } from './brand'
import { setLocale } from '@/i18n'
const fetch = vi.hoisted(() => vi.fn())
vi.mock('@/http', () => ({ adminFetch: (...args: unknown[]) => fetch(...args) }))
const preferences = { schemaVersion: 1, theme: 'system', locale: 'ru', density: 'default' }
const doc = (owner = 'a', version = 1, payload = preferences) => ({ owner, version, payload })
const page = (owner = 'a', readOnly = false) => ({
  props: { authuser: { id: owner }, appearance: doc(owner), appearanceAccess: { readOnly } },
})
const result = (data: unknown, status = 200) => ({
  ok: status < 400,
  status,
  json: async () => data,
})
const flush = async () => {
  for (let i = 0; i < 8; i++) await Promise.resolve()
}
beforeEach(() => {
  fetch.mockReset()
  appearance.brand = { ...emptyBrand }
  localStorage.clear()
  syncAppearance(page())
})
afterEach(() => {
  disposeAppearance()
  localStorage.clear()
})
it('serializes requests and coalesces edits made in flight', async () => {
  let finish!: (value: unknown) => void
  fetch.mockReturnValueOnce(
    new Promise((resolve) => {
      finish = resolve
    }),
  )
  fetch.mockResolvedValueOnce(result(doc('a', 3, { ...preferences, theme: 'dark', locale: 'en' })))
  editAppearance({ theme: 'dark' })
  editAppearance({ locale: 'en' })
  expect(fetch).toHaveBeenCalledTimes(1)
  finish(result(doc('a', 2, { ...preferences, theme: 'dark' })))
  await flush()
  expect(fetch).toHaveBeenCalledTimes(2)
  expect(JSON.parse(fetch.mock.calls[1]![1].body)).toMatchObject({
    version: 2,
    payload: { theme: 'dark', locale: 'en' },
  })
  expect(appearance.status).toBe('saved')
})
it('retains an error draft and retries only explicitly', async () => {
  fetch.mockRejectedValueOnce(new Error('offline'))
  editAppearance({ theme: 'dark' })
  await flush()
  editAppearance({ density: 'compact' })
  expect(fetch).toHaveBeenCalledTimes(1)
  expect(appearance.status).toBe('error')
  fetch.mockResolvedValueOnce(
    result(doc('a', 2, { ...preferences, theme: 'dark', density: 'compact' })),
  )
  await saveAppearance()
  expect(appearance.status).toBe('saved')
})
it('pauses on conflict and merges only changed fields with fresh server fields', async () => {
  fetch.mockResolvedValueOnce(result({}, 409))
  editAppearance({ theme: 'dark' })
  await flush()
  editAppearance({ density: 'compact' })
  expect(fetch).toHaveBeenCalledTimes(1)
  fetch.mockResolvedValueOnce(result(doc('a', 5, { ...preferences, locale: 'en' })))
  fetch.mockResolvedValueOnce(
    result(doc('a', 6, { ...preferences, theme: 'dark', locale: 'en', density: 'compact' })),
  )
  await reloadAppearance(true)
  await flush()
  expect(JSON.parse(fetch.mock.calls[2]![1].body)).toMatchObject({
    version: 5,
    payload: { theme: 'dark', locale: 'en', density: 'compact' },
  })
})
it('can discard a conflict draft by explicitly loading server preferences', async () => {
  fetch.mockResolvedValueOnce(result({}, 409))
  editAppearance({ theme: 'dark' })
  await flush()
  fetch.mockResolvedValueOnce(result(doc('a', 3, { ...preferences, locale: 'en' })))
  await reloadAppearance(false)
  expect(appearance.draft).toMatchObject({ theme: 'system', locale: 'en' })
  expect(fetch).toHaveBeenCalledTimes(2)
})
it('aborts and ignores late saves after owner switch', async () => {
  let finish!: (value: unknown) => void
  fetch.mockReturnValueOnce(
    new Promise((resolve) => {
      finish = resolve
    }),
  )
  editAppearance({ theme: 'dark' })
  const signal = fetch.mock.calls[0]![1].signal
  syncAppearance(page('b'))
  expect(signal.aborted).toBe(true)
  finish(result(doc('a', 9, { ...preferences, theme: 'dark' })))
  await flush()
  expect(appearance.owner).toBe('b')
  expect(appearance.version).toBe(1)
  expect(appearance.draft.theme).toBe('system')
})
it('keeps device settings separate across login and logout and ignores legacy values', () => {
  localStorage.setItem('theme-mode', 'dark')
  localStorage.setItem('goadmin.locale', 'en')
  syncAppearance({ props: { appearance: null } })
  expect(appearance.draft.theme).toBe('system')
  editAppearance({ locale: 'en', density: 'comfortable' })
  syncAppearance(page())
  expect(appearance.draft.locale).toBe('ru')
  syncAppearance({ props: { appearance: null } })
  expect(appearance.draft.locale).toBe('en')
  expect(JSON.parse(localStorage.getItem(deviceKey)!).density).toBe('comfortable')
  expect(fetch).not.toHaveBeenCalled()
})
it('fences role preview and unavailable account preferences', () => {
  syncAppearance(page('a', true))
  editAppearance({ theme: 'dark' })
  setLocale('en')
  expect(appearance.draft.locale).toBe('ru')
  expect(fetch).not.toHaveBeenCalled()
  syncAppearance({ props: { appearance: null, appearanceUnavailable: true } })
  editAppearance({ theme: 'dark' })
  expect(appearance.readOnly).toBe(true)
  expect(fetch).not.toHaveBeenCalled()
})
it('header locale updates the same account preference document', async () => {
  fetch.mockResolvedValueOnce(result(doc('a', 2, { ...preferences, locale: 'en' })))
  setLocale('en')
  await flush()
  expect(appearance.draft.locale).toBe('en')
  expect(fetch).toHaveBeenCalledTimes(1)
  expect(localStorage.getItem('goadmin.locale')).toBeNull()
})
it('rejects external and script logos and leaves status colors outside the brand palette', () => {
  expect(safeLogo('javascript:alert(1)')).toBe('')
  expect(safeLogo('https://evil.test/public/branding/light')).toBe('')
  expect(safeLogo('/public/branding/light')).toContain('/public/branding/light')
  expect(deriveBrandPalette('#ffffff', false)['--color-primary']).not.toBe('rgb(255 255 255)')
  expect(
    Object.keys(deriveBrandPalette('#010101', true)).some((key) =>
      /error|success|warning/.test(key),
    ),
  ).toBe(false)
})

it('seeds private first paint from validated head attributes instead of device settings', () => {
  disposeAppearance()
  localStorage.setItem(
    deviceKey,
    JSON.stringify({ theme: 'light', locale: 'ru', density: 'compact' }),
  )
  const root = document.documentElement
  root.dataset.appearanceOwner = 'account-seed'
  root.dataset.themePreference = 'dark'
  root.dataset.density = 'comfortable'
  root.lang = 'en'
  // An anonymous-shaped initial page must not be used for this check: init only seeds paint.
  initAppearance()
  expect(root.classList.contains('dark')).toBe(true)
  expect(root.lang).toBe('en')
  expect(appearance.draft.density).toBe('comfortable')
  expect(appearance.owner).toBe('')
  delete root.dataset.appearanceOwner
  delete root.dataset.themePreference
})

it('ignores a stale private appearance document on an anonymous page', () => {
  syncAppearance({
    props: { appearance: doc('old', 8, { ...preferences, theme: 'dark' }), authuser: null },
  })
  expect(appearance.owner).toBe('')
  expect(appearance.draft.theme).toBe('system')
  editAppearance({ locale: 'en' })
  expect(fetch).not.toHaveBeenCalled()
  expect(JSON.parse(localStorage.getItem(deviceKey)!).locale).toBe('en')
})
it('uses exact decimal owner identity above the safe JavaScript integer range', async () => {
  const exact = '9007199254740993'
  syncAppearance({
    props: { appearance: doc(exact), authuser: { id: Number(exact), idExact: exact } },
  })
  expect(appearance.owner).toBe(exact)
  expect(appearance.readOnly).toBe(false)
  fetch.mockResolvedValueOnce(result(doc(exact, 2, { ...preferences, theme: 'dark' })))
  editAppearance({ theme: 'dark' })
  await flush()
  expect(appearance.status).toBe('saved')
})

it('keeps saved preferences after history navigation and uses the current version for the next write', async () => {
  fetch.mockResolvedValueOnce(result(doc('a', 2, { ...preferences, theme: 'dark' })))
  editAppearance({ theme: 'dark' })
  await flush()
  syncAppearance(page())
  expect(appearance.version).toBe(2)
  expect(appearance.draft.theme).toBe('dark')
  expect(document.documentElement.classList.contains('dark')).toBe(true)
  fetch.mockResolvedValueOnce(result(doc('a', 3, { ...preferences, theme: 'dark', locale: 'en' })))
  editAppearance({ locale: 'en' })
  await flush()
  expect(JSON.parse(fetch.mock.calls[1]![1].body).version).toBe(2)
})
it('accepts newer same-owner props but does not downgrade through a permission fence', () => {
  syncAppearance({
    props: { ...page().props, appearance: doc('a', 3, { ...preferences, locale: 'en' }) },
  })
  syncAppearance(page('a', true))
  expect(appearance.version).toBe(3)
  expect(appearance.draft.locale).toBe('en')
  expect(appearance.readOnly).toBe(true)
  syncAppearance(page())
  expect(appearance.version).toBe(3)
  syncAppearance(page('b'))
  expect(appearance.version).toBe(1)
  expect(appearance.draft.locale).toBe('ru')
})
it('keeps a conflict draft paused when navigating to older same-owner props', async () => {
  fetch.mockResolvedValueOnce(result({}, 409))
  editAppearance({ theme: 'dark' })
  await flush()
  syncAppearance(page())
  expect(appearance.status).toBe('conflict')
  expect(appearance.draft.theme).toBe('dark')
  expect(fetch).toHaveBeenCalledTimes(1)
})
it('accepts lower server versions after logout and a new login to the same account', () => {
  syncAppearance({
    props: { ...page().props, appearance: doc('a', 4, { ...preferences, locale: 'en' }) },
  })
  syncAppearance({ props: { authuser: null } })
  syncAppearance(page())
  expect(appearance.version).toBe(1)
  expect(appearance.draft.locale).toBe('ru')
})

it('keeps global branding monotonic across account switches and logout with stale props', () => {
  syncAppearance({ props: { ...page().props, branding: { version: 2, name: 'Published' } } })
  syncAppearance({
    props: { ...page('b').props, appearance: doc('b', 0), branding: { version: 1, name: 'Old' } },
  })
  expect(appearance.owner).toBe('b')
  expect(appearance.version).toBe(0)
  expect(appearance.brand).toMatchObject({ version: 2, name: 'Published' })
  syncAppearance({
    props: { authuser: null, branding: { version: 1, name: 'Old anonymous page' } },
  })
  expect(appearance.owner).toBe('')
  expect(appearance.brand).toMatchObject({ version: 2, name: 'Published' })
  syncAppearance({ props: { ...page().props, branding: { version: 3, name: 'Newest' } } })
  expect(appearance.brand).toMatchObject({ version: 3, name: 'Newest' })
})
