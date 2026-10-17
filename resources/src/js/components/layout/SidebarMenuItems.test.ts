import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, type App } from 'vue'
import SidebarMenuItems from './SidebarMenuItems.vue'
import { setLocale } from '@/i18n'
vi.mock('@inertiajs/vue3', () => ({
  Link: {
    props: ['href'],
    setup:
      (props: { href: string }, { slots }: { slots: { default?: () => unknown } }) =>
      () =>
        h('a', { href: props.href }, slots.default?.() as never),
  },
}))
let app: App | undefined
let container: HTMLDivElement

afterEach(() => {
  app?.unmount()
  container?.remove()
  setLocale('ru')
})
function mount(collapsed: boolean) {
  container = document.createElement('div')
  document.body.append(container)
  app = createApp({
    render: () =>
      h(SidebarMenuItems, {
        items: [
          {
            name: 'Администраторы',
            href: '/admins',
            children: [{ name: 'Профиль', href: '/auth/profile' }],
          },
        ],
        depth: 0,
        collapsed,
        isActive: (href: string) => href === '/auth/profile',
        resolveIcon: () => undefined,
      }),
  })
  app.mount(container)
}

describe('sidebar navigation', () => {
  it.each([true, false])(
    'keeps nested active navigation reachable when collapsed=%s',
    async (collapsed) => {
      mount(collapsed)
      const parent = container.querySelector('a')!
      const toggle = container.querySelector('button')!
      expect(parent.contains(toggle)).toBe(false)
      expect(container.querySelector('a[href="/auth/profile"]')).not.toBeNull()
      expect(toggle.type).toBe('button')
      expect(toggle.getAttribute('aria-expanded')).toBe('true')
      expect(document.getElementById(toggle.getAttribute('aria-controls')!)).not.toBeNull()
      if (collapsed) expect(parent.title).toBe('Администраторы')
      toggle.click()
      await nextTick()
      expect(toggle.getAttribute('aria-expanded')).toBe('false')
      expect(container.querySelector('a[href="/auth/profile"]')).toBeNull()
      toggle.click()
      await nextTick()
      expect(container.querySelector('a[href="/auth/profile"]')).not.toBeNull()
      setLocale('en')
      await nextTick()
      expect(parent.getAttribute('aria-label')).toBe('Administrators')
      expect(container.querySelector('[aria-current="page"]')?.getAttribute('href')).toBe(
        '/auth/profile',
      )
    },
  )
})

it('shows a meaningful initial and accessible group toggle for collapsed groups without a destination', async () => {
  container = document.createElement('div')
  document.body.append(container)
  app = createApp({
    render: () =>
      h(SidebarMenuItems, {
        items: [
          {
            name: 'Пользователи',
            href: '',
            children: [{ name: 'Профиль', href: '/auth/profile' }],
          },
        ],
        depth: 0,
        collapsed: true,
        isActive: () => false,
        resolveIcon: () => undefined,
      }),
  })
  app.mount(container)
  const label = container.querySelector('[title="Пользователи"]')!
  expect(label.querySelector('[aria-hidden="true"]')?.textContent).toBe('П')
  const button = container.querySelector('button')!
  expect(button.getAttribute('aria-label')).toBe('Развернуть Пользователи')
  button.click()
  await nextTick()
  expect(container.querySelector('a[href="/auth/profile"]')).not.toBeNull()
})
