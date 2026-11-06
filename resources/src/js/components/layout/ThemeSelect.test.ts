import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { createApp, nextTick, type Component } from 'vue'
import { createPinia } from 'pinia'
import ThemeSelect from './ThemeSelect.vue'
import AuthLayout from './AuthLayout.vue'
import AdminHeaderUtilities from './AdminHeaderUtilities.vue'
import { appearance, deviceKey, disposeAppearance, syncAppearance } from '@/appearance/state'

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
function key(element: Element, value: string) {
  element.dispatchEvent(
    new KeyboardEvent('keydown', { key: value, bubbles: true, cancelable: true }),
  )
}
async function open(root: HTMLElement) {
  const trigger = root.querySelector<HTMLButtonElement>('button[aria-label^="Выбрать тему"]')!
  key(trigger, 'Enter')
  await flush()
  expect(trigger.getAttribute('aria-haspopup')).toBe('menu')
  expect(trigger.getAttribute('aria-expanded')).toBe('true')
  return [...document.querySelectorAll<HTMLElement>('[role="menuitemradio"]')]
}
it.each([
  ['light', 'Светлая'],
  ['dark', 'Тёмная'],
  ['system', 'Системная'],
] as const)(
  'selects explicit %s by keyboard and checks the stored preference',
  async (mode, label) => {
    const root = await mount()
    const options = await open(root)
    expect(options.map((item) => item.textContent?.trim())).toEqual([
      'Системная',
      'Светлая',
      'Тёмная',
    ])
    const item = options.find((item) => item.textContent?.trim() === label)!
    item.focus()
    key(item, 'Enter')
    await flush()
    expect(appearance.draft.theme).toBe(mode)
    expect(JSON.parse(localStorage.getItem(deviceKey)!).theme).toBe(mode)
    const reopened = await open(root)
    expect(
      reopened
        .filter((item) => item.getAttribute('aria-checked') === 'true')
        .map((item) => item.textContent?.trim()),
    ).toEqual([label])
    key(document.querySelector('[role="menu"]')!, 'Escape')
    await flush()
    expect(root.querySelector('button')!.getAttribute('aria-expanded')).toBe('false')
  },
)
it('keeps System checked when live OS changes resolve to dark and light', async () => {
  const root = await mount()
  expect(root.querySelector('button')!.title).toBe('Системная (светлая)')
  media.matches = true
  media.addEventListener.mock.calls[0]![1]()
  await flush()
  expect(root.querySelector('button')!.title).toBe('Системная (тёмная)')
  const options = await open(root)
  expect(options[0]!.getAttribute('aria-checked')).toBe('true')
  expect(options[2]!.getAttribute('aria-checked')).toBe('false')
  media.matches = false
  media.addEventListener.mock.calls[0]![1]()
  await flush()
  expect(root.querySelector('button')!.title).toBe('Системная (светлая)')
  expect(appearance.draft.theme).toBe('system')
  expect(localStorage.getItem(deviceKey)).toBeNull()
})
it('moves radio focus with arrow keys and fences a menu after live permission changes', async () => {
  const root = await mount()
  const options = await open(root)
  options[0]!.focus()
  key(options[0]!, 'ArrowDown')
  await new Promise((resolve) => setTimeout(resolve, 0))
  await flush()
  expect(document.activeElement).toBe(options[1])
  appearance.readOnly = true
  await flush()
  expect(options.every((item) => item.getAttribute('aria-disabled') === 'true')).toBe(true)
  options[2]!.click()
  key(options[2]!, 'Enter')
  await flush()
  expect(appearance.draft.theme).toBe('system')
})
it.each(['readOnly', 'resolving'] as const)('fences theme selection while %s', async (flag) => {
  appearance[flag] = true
  const root = await mount()
  const trigger = root.querySelector<HTMLButtonElement>('button')!
  expect(trigger.disabled).toBe(true)
  key(trigger, 'Enter')
  trigger.click()
  await flush()
  expect(document.querySelector('[role="menu"]')).toBeNull()
  expect(appearance.draft.theme).toBe('system')
})
it.each([
  ['auth shell', AuthLayout],
  ['admin header', AdminHeaderUtilities],
] as const)('mounts the same three-mode menu in %s', async (_label, component) => {
  const root = await mount(component)
  expect(await open(root)).toHaveLength(3)
  expect(root.querySelector('button[title="Системная (светлая)"]')).not.toBeNull()
})
