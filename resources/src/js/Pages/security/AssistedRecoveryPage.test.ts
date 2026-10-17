import { setLocale } from '@/i18n'
import { afterEach, expect, it, vi } from 'vitest'
import { createApp, nextTick } from 'vue'
import AssistedRecoveryPage from './AssistedRecoveryPage.vue'
vi.mock('@inertiajs/vue3', () => ({ router: { get: vi.fn() } }))
vi.mock('@/components/layout/AppHead.vue', () => ({ default: { render: () => null } }))
let app: ReturnType<typeof createApp> | undefined
afterEach(() => {
  setLocale('ru')
  app?.unmount()
  vi.unstubAllGlobals()
})
it('selects the resolved canonical subject and fences uncertain issuance without replay', async () => {
  const fetch = vi.fn<(url: string, options: RequestInit) => Promise<Response>>(async () => {
    throw new Error('response lost')
  })
  vi.stubGlobal('fetch', fetch)
  const root = document.createElement('div')
  document.body.appendChild(root)
  app = createApp(AssistedRecoveryPage, {
    title: '',
    nativeCSRF: 'native-proof',
    search: 'existing@example.test',
    targets: [{ subject: 'canonical-subject', name: 'Synthetic', email: 'existing@example.test' }],
  })
  app.mount(root)
  const option = root.querySelectorAll('option')[1]
  expect(option.value).toBe('canonical-subject')
  expect(option.textContent).toContain('existing@example.test')
  const form = root.querySelectorAll('form')[1]
  form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
  await nextTick()
  await nextTick()
  expect(fetch).toHaveBeenCalledTimes(1)
  expect(fetch.mock.calls[0]?.[1]?.headers).toEqual({
    'Content-Type': 'application/x-www-form-urlencoded',
    'X-CSRF-Token': 'native-proof',
  })
  expect(root.textContent).toContain('Результат неизвестен')
  setLocale('en')
  await nextTick()
  expect(root.textContent).toContain('Outcome unknown')
  setLocale('ru')
  await nextTick()
  expect(root.textContent).toContain('Результат неизвестен')
  expect(form.querySelector('fieldset')?.disabled).toBe(true)
  form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
  expect(fetch).toHaveBeenCalledTimes(1)
  root.remove()
})
