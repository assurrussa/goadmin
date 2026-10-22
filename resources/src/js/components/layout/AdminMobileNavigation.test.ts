import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, ref } from 'vue'
import AdminMobileNavigation from './AdminMobileNavigation.vue'

const mocks = vi.hoisted(() => ({ on: vi.fn(), stop: vi.fn(), add: vi.fn(), remove: vi.fn() }))
vi.mock('@inertiajs/vue3', () => ({ router: { on: mocks.on } }))
vi.mock('./AdminSidebar.vue', () => ({
  default: { render: () => h('nav', [h('button', 'First'), h('a', { href: '/users' }, 'Last')]) },
}))
let app: ReturnType<typeof createApp> | undefined
beforeEach(() => {
  vi.clearAllMocks()
  mocks.on.mockReturnValue(mocks.stop)
  vi.stubGlobal('matchMedia', () => ({
    matches: true,
    addEventListener: mocks.add,
    removeEventListener: mocks.remove,
  }))
  vi.spyOn(HTMLElement.prototype, 'getClientRects').mockReturnValue([
    new DOMRect(),
  ] as unknown as DOMRectList)
})
afterEach(() => {
  app?.unmount()
  document.body.innerHTML = ''
  document.body.style.overflow = ''
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})
const setup = () => {
  const open = ref(false)
  const trigger = document.createElement('button')
  const root = document.createElement('div')
  document.body.append(trigger, root)
  trigger.focus()
  app = createApp({
    render: () =>
      h(AdminMobileNavigation, {
        open: open.value,
        onClose: () => {
          open.value = false
        },
      }),
  })
  app.mount(root)
  return { open, trigger }
}
const settle = async () => {
  await nextTick()
  await nextTick()
}
it('owns inert, scroll lock, focus trap, Escape and focus restoration for every shell', async () => {
  const { open, trigger } = setup()
  const panel = document.getElementById('admin-mobile-navigation')!
  expect(panel.hasAttribute('inert')).toBe(true)
  document.body.style.overflow = 'auto'
  open.value = true
  await settle()
  const first = panel.querySelector('button')!,
    last = panel.querySelector('a')!
  expect(panel.getAttribute('role')).toBe('dialog')
  expect(panel.hasAttribute('inert')).toBe(false)
  expect(document.body.style.overflow).toBe('hidden')
  expect(document.activeElement).toBe(first)
  first.dispatchEvent(
    new KeyboardEvent('keydown', { key: 'Tab', shiftKey: true, bubbles: true, cancelable: true }),
  )
  expect(document.activeElement).toBe(last)
  last.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true }))
  expect(document.activeElement).toBe(first)
  first.dispatchEvent(
    new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }),
  )
  await settle()
  expect(open.value).toBe(false)
  expect(panel.hasAttribute('inert')).toBe(true)
  expect(document.body.style.overflow).toBe('auto')
  expect(document.activeElement).toBe(trigger)
})
it('closes on navigation and breakpoint changes, releasing listeners and locks on unmount', async () => {
  const { open } = setup()
  open.value = true
  await settle()
  const navigate = mocks.on.mock.calls[0]![1] as () => void
  navigate()
  await settle()
  expect(open.value).toBe(false)
  open.value = true
  await settle()
  const viewportChange = mocks.add.mock.calls[0]![1] as () => void
  viewportChange()
  await settle()
  expect(open.value).toBe(false)
  open.value = true
  await settle()
  app!.unmount()
  app = undefined
  expect(document.body.style.overflow).toBe('')
  expect(mocks.stop).toHaveBeenCalledTimes(1)
  expect(mocks.remove).toHaveBeenCalledWith('change', viewportChange)
})
