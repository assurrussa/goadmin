import { afterEach, expect, it, vi } from 'vitest'
import { createApp, defineComponent, h } from 'vue'
import SidebarLink from './SidebarLink.vue'

vi.mock('@inertiajs/vue3', () => ({
  Link: defineComponent({
    setup:
      (_, { attrs, slots }) =>
      () =>
        h('a', attrs, slots.default?.()),
  }),
}))

const apps: ReturnType<typeof createApp>[] = []
afterEach(() => apps.splice(0).forEach((app) => app.unmount()))

it.each([true, false])('keeps readable navigation and icon colors with active=%s', (active) => {
  const root = document.createElement('div')
  const app = createApp(SidebarLink, {
    href: '/students',
    active,
    icon: defineComponent({ render: () => h('svg') }),
  })
  apps.push(app)
  app.mount(root)
  const link = root.querySelector('a')!
  const icon = root.querySelector('svg')!
  expect(link.getAttribute('href')).toBe('/students')
  if (active) {
    for (const element of [link, icon]) {
      expect(element.classList.contains('text-primary-dark')).toBe(true)
      expect(element.classList.contains('dark:text-primary-light')).toBe(true)
      expect(element.classList.contains('text-primary')).toBe(false)
    }
  } else {
    expect(link.classList.contains('text-text-secondary')).toBe(true)
    expect(icon.classList.contains('text-text-tertiary')).toBe(true)
  }
  // Browser acceptance checks actual composited colors in both selected themes.
})
