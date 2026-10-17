import { afterEach, expect, it, vi } from 'vitest'
import { createApp, defineComponent, h } from 'vue'
import { createPinia } from 'pinia'
import AdminHeader from './AdminHeader.vue'
import AdminSidebar from './AdminSidebar.vue'
vi.mock('@/stores/theme', () => ({ useThemeStore: () => ({ isDark: false, toggle: vi.fn() }) }))
const mocks = vi.hoisted(() => ({
  page: {
    props: {
      adminMenu: {
        sections: [
          {
            key: 'account',
            placement: 'account',
            items: [
              { name: 'Linked account', href: '/sso/account' },
              ...['//external.test', '/\\external.test', '/bad\npath', 'https://external.test'].map(
                (href) => ({ name: 'Unsafe', href }),
              ),
              { name: 'Group', children: [{ name: 'Child', href: '/child' }] },
            ],
          },
          { key: 'main', items: [{ name: 'Sidebar only', href: '/users' }] },
        ],
      },
    },
    url: '/users',
  },
}))
vi.mock('@inertiajs/vue3', () => ({
  usePage: () => mocks.page,
  router: {},
  Link: defineComponent({
    props: ['href'],
    setup:
      (props, { slots }) =>
      () =>
        h('a', { href: props.href }, slots.default?.()),
  }),
}))
vi.mock('@/components/ui/dropdown-menu', () => {
  const part = defineComponent({
    setup:
      (_, { slots }) =>
      () =>
        h('div', slots.default?.()),
  })
  return Object.fromEntries(
    [
      'DropdownMenu',
      'DropdownMenuTrigger',
      'DropdownMenuContent',
      'DropdownMenuItem',
      'DropdownMenuSeparator',
      'DropdownMenuRadioGroup',
      'DropdownMenuRadioItem',
    ].map((name) => [name, part]),
  )
})
const apps: ReturnType<typeof createApp>[] = []
afterEach(() => {
  apps.splice(0).forEach((app) => app.unmount())
})
it('places only safe flat account links in the profile and keeps them out of the sidebar', () => {
  function mount(component: typeof AdminHeader | typeof AdminSidebar) {
    const root = document.createElement('div')
    const app = createApp(component).use(createPinia())
    apps.push(app)
    app.mount(root)
    return root
  }
  const header = mount(AdminHeader)
  expect(header.querySelector('a[href="/sso/account"]')?.textContent).toContain('Linked account')
  expect(header.textContent).not.toContain('Unsafe')
  expect(header.textContent).not.toContain('Group')
  expect(header.querySelector('a[href="/child"]')).toBeNull()
  expect(header.querySelector('a[href="/users"]')).toBeNull()
  const sidebar = mount(AdminSidebar)
  expect(sidebar.querySelector('a[href="/sso/account"]')).toBeNull()
  expect(sidebar.querySelector('a[href="/users"]')?.textContent).toContain('Sidebar only')
})

it('includes the border within the 68px header height', () => {
  const root = document.createElement('div')
  const app = createApp(AdminHeader).use(createPinia())
  apps.push(app)
  app.mount(root)
  const header = root.querySelector('header')!
  expect(header.classList.contains('box-border')).toBe(true)
  expect(header.classList.contains('h-[68px]')).toBe(true)
  expect(header.firstElementChild?.classList.contains('h-full')).toBe(true)
  expect(header.firstElementChild?.classList.contains('min-h-[68px]')).toBe(false)
})
