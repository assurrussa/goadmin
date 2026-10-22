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

it('renders the adapter account links only when its section is supplied', () => {
  const original = mocks.page.props.adminMenu.sections
  const section = {
    key: 'goadmin-sso-account',
    placement: 'account',
    items: [
      { name: 'Связь с AuthHub', href: '/auth/profile/settings#sso-settings-title' },
      { name: 'Открыть AuthHub', href: '/sso-provider-account' },
    ],
  }
  try {
    mocks.page.props.adminMenu.sections = [section]
    const enabledRoot = document.createElement('div')
    const enabledApp = createApp(AdminHeader).use(createPinia())
    apps.push(enabledApp)
    enabledApp.mount(enabledRoot)
    expect(
      enabledRoot.querySelectorAll('a[href="/auth/profile/settings#sso-settings-title"]'),
    ).toHaveLength(1)
    expect(enabledRoot.querySelectorAll('a[href="/sso-provider-account"]')).toHaveLength(1)
    expect(enabledRoot.querySelector('a[href="/sso/account"]')).toBeNull()

    mocks.page.props.adminMenu.sections = []
    const disabledRoot = document.createElement('div')
    const disabledApp = createApp(AdminHeader).use(createPinia())
    apps.push(disabledApp)
    disabledApp.mount(disabledRoot)
    expect(disabledRoot.querySelector('a[href="/sso-provider-account"]')).toBeNull()
    expect(
      disabledRoot.querySelector('a[href="/auth/profile/settings#sso-settings-title"]'),
    ).toBeNull()
  } finally {
    mocks.page.props.adminMenu.sections = original
  }
})

it('shares shell height/content tokens while preserving the original defaults', () => {
  const root = document.createElement('div')
  const app = createApp(AdminHeader).use(createPinia())
  apps.push(app)
  app.mount(root)
  const header = root.querySelector('header')!
  expect(header.classList.contains('box-border')).toBe(true)
  const sidebarRoot = document.createElement('div')
  const sidebarApp = createApp(AdminSidebar).use(createPinia())
  apps.push(sidebarApp)
  sidebarApp.mount(sidebarRoot)
  expect(
    sidebarRoot.firstElementChild?.firstElementChild?.classList.contains(
      'h-[var(--goadmin-header-height,68px)]',
    ),
  ).toBe(true)
  expect(
    header.firstElementChild?.classList.contains('max-w-[var(--goadmin-content-max-width,80rem)]'),
  ).toBe(true)
  expect(
    header
      .querySelector('[data-profile-menu-trigger]')
      ?.classList.contains('rounded-[var(--goadmin-menu-radius,1rem)]'),
  ).toBe(true)
  expect(header.classList.contains('h-[var(--goadmin-header-height,68px)]')).toBe(true)
  expect(header.firstElementChild?.classList.contains('h-full')).toBe(true)
  expect(header.firstElementChild?.classList.contains('min-h-[68px]')).toBe(false)
})
