import { afterEach, expect, it, vi } from 'vitest'
import { createApp, nextTick } from 'vue'
import AppearanceSettings from './AppearanceSettings.vue'
import { appearance, syncAppearance, disposeAppearance } from '@/appearance/state'
const fetch = vi.hoisted(() => vi.fn())
vi.mock('@/http', () => ({ adminFetch: (...args: unknown[]) => fetch(...args) }))
let app: ReturnType<typeof createApp> | undefined
afterEach(() => {
  app?.unmount()
  disposeAppearance()
  fetch.mockReset()
})
it('autosaves native select changes through the actual appearance state', async () => {
  syncAppearance({
    props: {
      authuser: { id: 1, idExact: '1' },
      appearance: {
        owner: '1',
        version: 0,
        payload: { schemaVersion: 1, theme: 'light', locale: 'en', density: 'default' },
      },
      appearanceAccess: { readOnly: false },
    },
  })
  fetch.mockResolvedValue({
    ok: true,
    status: 200,
    json: async () => ({
      owner: '1',
      version: 1,
      payload: { schemaVersion: 1, theme: 'dark', locale: 'en', density: 'default' },
    }),
  })
  const root = document.createElement('div')
  app = createApp(AppearanceSettings)
  app.mount(root)
  const select = root.querySelector('select')!
  expect(select.labels?.[0]?.textContent?.trim()).toBe('Theme')
  select.value = 'dark'
  select.dispatchEvent(new Event('input', { bubbles: true }))
  select.dispatchEvent(new Event('change', { bubbles: true }))
  for (let i = 0; i < 8; i++) await Promise.resolve()
  await nextTick()
  expect(fetch).toHaveBeenCalledTimes(1)
  expect(appearance.draft.theme).toBe('dark')
})
