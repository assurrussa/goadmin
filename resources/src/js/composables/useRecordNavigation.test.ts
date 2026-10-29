import { afterEach, describe, expect, it, vi } from 'vitest'
import { reactive } from 'vue'
import { recordListHref, useRecordNavigation } from './useRecordNavigation'

const page = vi.hoisted(() => ({ url: '/users' }))
vi.mock('@inertiajs/vue3', () => ({ usePage: () => reactive(page) }))
afterEach(() => {
  window.history.replaceState(null, '', '/')
  page.url = '/users'
})

describe('resource list navigation', () => {
  it('preserves the synchronized grid query through view, edit and reload', () => {
    const source = '/users?page=3&limit=10&sortBy=name&sortOrder=desc&search=Jane&status=active'
    window.history.replaceState(null, '', source)
    const navigation = useRecordNavigation('/users')
    const view = navigation.recordHref(42)
    page.url = view
    window.history.replaceState(null, '', view)
    expect(useRecordNavigation('/users').listHref.value).toBe(source)
    page.url = navigation.recordHref(42, true)
    expect(page.url).toContain('/users/42/edit?returnTo=')
    expect(useRecordNavigation('/users').listHref.value).toBe(source)
  })
  it.each([
    'https://evil.invalid/users',
    '//evil.invalid/users',
    '/admins?page=2',
    '/users/42',
    '/users/../admins',
    '/users\\evil',
    '/ users',
    'javascript:alert(1)',
    '/%75sers',
  ])('rejects a foreign or non-list target %s', (target) => {
    expect(recordListHref('/users', target)).toBe('/users')
  })
  it('falls back for direct links and removes nested return targets and fragments', () => {
    page.url = '/users/42/edit'
    expect(useRecordNavigation('/users').listHref.value).toBe('/users')
    expect(recordListHref('/admins', '/admins?page=2&returnTo=%2Fusers#secret')).toBe(
      '/admins?page=2',
    )
  })
})
