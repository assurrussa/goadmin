import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { createApp, nextTick, type Component } from 'vue'
import { createPinia } from 'pinia'
import ThemeSelect from './ThemeSelect.vue'
import AuthLayout from './AuthLayout.vue'
import AdminHeaderUtilities from './AdminHeaderUtilities.vue'
import {
  appearance,
  deviceKey,
  disposeAppearance,
  editAppearance,
  syncAppearance,
} from '@/appearance/state'

vi.mock('@inertiajs/vue3', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@inertiajs/vue3')>()),
  usePage: () => ({ props: {} }),
  router: { on: () => () => {} },
}))
const apps: ReturnType<typeof createApp>[] = []
let media: {
  matches: boolean
  addEventListener: ReturnType<typeof vi.fn>
  removeEventListener: ReturnType<typeof vi.fn>
}
beforeEach(() => {
  media = { matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() }
  vi.stubGlobal(
    'matchMedia',
    vi.fn(() => media),
  )
  syncAppearance({ props: {} })
})
afterEach(() => {
  apps.splice(0).forEach((app) => app.unmount())
  disposeAppearance()
  localStorage.clear()
  document.body.replaceChildren()
  vi.unstubAllGlobals()
})
const flush = async () => {
  for (let i = 0; i < 8; i++) await nextTick()
}
async function mount(component: Component = ThemeSelect) {
  const root = document.createElement('div')
  document.body.append(root)
  const app = createApp(component)
  app.use(createPinia())
  apps.push(app)
  app.mount(root)
  await flush()
  return root
}
function themeButton(root: HTMLElement) {
  return root.querySelector<HTMLButtonElement>('button[aria-label^="Выбрать тему"]')!
}
it.each([false, true])('cycles System → Light → Dark → System with OS dark=%s', async (dark) => {
  media.matches = dark
  media.addEventListener.mock.calls[0]![1]()
  const root = await mount()
  const button = themeButton(root)
  expect(button.title).toBe(dark ? 'Системная (тёмная)' : 'Системная (светлая)')
  expect(button.querySelector('.lucide-monitor')).not.toBeNull()
  expect(button.hasAttribute('aria-haspopup')).toBe(false)
  for (const [mode, title, icon] of [
    ['light', 'Светлая', 'sun'],
    ['dark', 'Тёмная', 'moon'],
    ['system', dark ? 'Системная (тёмная)' : 'Системная (светлая)', 'monitor'],
  ] as const) {
    button.click()
    await flush()
    expect(appearance.draft.theme).toBe(mode)
    expect(JSON.parse(localStorage.getItem(deviceKey)!).theme).toBe(mode)
    expect(button.title).toBe(title)
    expect(button.getAttribute('aria-label')).toBe('Выбрать тему: ' + title)
    expect(button.querySelector('.lucide-' + icon)).not.toBeNull()
    expect(document.querySelector('[role="menu"]')).toBeNull()
  }
})
it('retains System preference and monitor icon as the OS changes live', async () => {
  const root = await mount()
  const button = themeButton(root)
  for (const dark of [true, false]) {
    media.matches = dark
    media.addEventListener.mock.calls[0]![1]()
    await flush()
    expect(button.title).toBe(dark ? 'Системная (тёмная)' : 'Системная (светлая)')
    expect(button.querySelector('.lucide-monitor')).not.toBeNull()
    expect(appearance.draft.theme).toBe('system')
    expect(document.documentElement.dataset.themeMode).toBe(dark ? 'dark' : 'light')
  }
  expect(localStorage.getItem(deviceKey)).toBeNull()
})
it('restores a saved preference and localizes the button label', async () => {
  const root = await mount()
  themeButton(root).click()
  await flush()
  apps.splice(0).forEach((app) => app.unmount())
  disposeAppearance()
  syncAppearance({ props: {} })
  const restored = await mount()
  expect(appearance.draft.theme).toBe('light')
  editAppearance({ locale: 'en' })
  await flush()
  const button = restored.querySelector<HTMLButtonElement>('button')!
  expect(button.title).toBe('Light')
  expect(button.getAttribute('aria-label')).toBe('Choose theme: Light')
  button.click()
  await flush()
  expect(button.title).toBe('Dark')
  expect(button.getAttribute('aria-label')).toBe('Choose theme: Dark')
})
it.each(['readOnly', 'resolving'] as const)(
  'fences theme cycling while %s changes live',
  async (flag) => {
    const root = await mount()
    const button = themeButton(root)
    appearance[flag] = true
    await flush()
    expect(button.disabled).toBe(true)
    button.click()
    button.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await flush()
    expect(appearance.draft.theme).toBe('system')
    expect(localStorage.getItem(deviceKey)).toBeNull()
    appearance[flag] = false
    await flush()
    button.click()
    await flush()
    expect(appearance.draft.theme).toBe('light')
  },
)
it.each([
  ['auth shell', AuthLayout],
  ['admin header', AdminHeaderUtilities],
] as const)('mounts the same cycling button in %s', async (_label, component) => {
  const root = await mount(component)
  const button = themeButton(root)
  expect(button.title).toBe('Системная (светлая)')
  button.click()
  await flush()
  expect(button.title).toBe('Светлая')
  expect(document.querySelector('[role="menu"]')).toBeNull()
})
