import { afterEach, expect, it, vi } from 'vitest'
import { createApp, h } from 'vue'
import { createPinia } from 'pinia'
import AuthLayout from './AuthLayout.vue'
import SplitAuthShell from '../../../../examples/theme-topbar/src/AuthShell.vue'
import { installPresentation } from '@/presentation/context'

vi.mock('@/plugins/theme', () => ({
  themeManager: {
    init: vi.fn(),
    getCurrent: () => 'light',
    toggle: vi.fn(),
    setLight: vi.fn(),
    setDark: vi.fn(),
  },
}))
const apps: ReturnType<typeof createApp>[] = []
afterEach(() => apps.splice(0).forEach((app) => app.unmount()))

it.each([undefined, SplitAuthShell])(
  'preserves host root classes and existing auth slots with shell %s',
  (authShell) => {
    const root = document.createElement('div')
    const app = createApp({
      render: () =>
        h(
          AuthLayout,
          { class: 'host-auth-class' },
          {
            title: () => 'Host title',
            subtitle: () => 'Host subtitle',
            default: () => h('form', { 'data-host-form': '' }, 'Unchanged form'),
            footer: () => 'Host footer',
          },
        ),
    }).use(createPinia())
    installPresentation(app, { id: 'test.shell', apiVersion: 1, authShell })
    apps.push(app)
    app.mount(root)
    expect(
      root.querySelector('[data-ui="auth-shell"]')?.classList.contains('host-auth-class'),
    ).toBe(true)
    expect(root.querySelectorAll('[data-host-form]')).toHaveLength(1)
    for (const text of ['Host title', 'Host subtitle', 'Host footer', 'Unchanged form'])
      expect(root.textContent).toContain(text)
  },
)
