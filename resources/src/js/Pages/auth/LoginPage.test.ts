import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, defineComponent, h, reactive } from 'vue'
import LoginPage from './LoginPage.vue'
import { setAdminCapabilities } from '@/composables/useAdminCapabilities'

vi.mock('@inertiajs/vue3', () => ({
  Link: defineComponent({
    setup:
      (_, { attrs, slots }) =>
      () =>
        h('a', attrs, slots.default?.()),
  }),
  usePage: () => ({ props: {} }),
  useForm: (data: object) => reactive({ ...data, errors: {}, post: vi.fn(), processing: false }),
  router: { on: () => () => {} },
}))
vi.mock('@/components/layout/AuthLayout.vue', () => ({
  default: defineComponent({
    setup:
      (_, { slots }) =>
      () =>
        h('div', [slots.default?.(), slots.footer?.()]),
  }),
}))
vi.mock('@/components/layout/AppHead.vue', () => ({
  default: defineComponent({ render: () => null }),
}))

const apps: ReturnType<typeof createApp>[] = []
afterEach(() => {
  apps.splice(0).forEach((app) => app.unmount())
  setAdminCapabilities({ props: {} })
})

describe('login authmail capability', () => {
  it.each([false, true])('shows password recovery only with authmail=%s', (enabled) => {
    setAdminCapabilities({ props: { adminCapabilities: enabled ? { authmail: true } : {} } })
    const root = document.createElement('div')
    const app = createApp(LoginPage, { canRegisterFirstAdmin: true })
    apps.push(app)
    app.mount(root)
    expect(Boolean(root.querySelector('a[href="/auth/forgot-password"]'))).toBe(enabled)
    expect(root.querySelector('form')).not.toBeNull()
    expect(root.querySelector('a[href="/auth/register"]')).not.toBeNull()
  })
})

describe('login auth-providers slot', () => {
  it('keeps the default login form without a provider wrapper or spacing', () => {
    const root = document.createElement('div')
    const app = createApp(LoginPage)
    apps.push(app)
    app.mount(root)

    const form = root.querySelector('form')
    expect(form).not.toBeNull()
    expect(root.querySelectorAll('form')).toHaveLength(1)
    expect(form?.parentElement?.children).toHaveLength(1)
    expect(root.querySelector('input[name="email"]')).not.toBeNull()
    expect(root.querySelector('input[name="password"]')).not.toBeNull()
  })

  it('renders provider content before the unchanged local form in the same layout', () => {
    const root = document.createElement('div')
    const app = createApp({
      render: () =>
        h(LoginPage, null, {
          'auth-providers': () =>
            h('div', { 'data-testid': 'providers', class: 'mb-6' }, [
              h('a', { href: '/auth/sso/login' }, 'Continue with SSO'),
            ]),
        }),
    })
    apps.push(app)
    app.mount(root)

    const providers = root.querySelector('[data-testid="providers"]')
    const form = root.querySelector('form')
    expect(providers).not.toBeNull()
    expect(form).not.toBeNull()
    expect(providers?.parentElement).toBe(form?.parentElement)
    expect(providers?.nextElementSibling).toBe(form)
    expect(form?.contains(providers)).toBe(false)
    expect(root.querySelectorAll('form')).toHaveLength(1)
    expect(root.querySelector('a[href="/auth/sso/login"]')?.textContent).toBe('Continue with SSO')
    expect(form?.querySelector('input[name="email"]')).not.toBeNull()
    expect(form?.querySelector('input[name="password"]')).not.toBeNull()
    expect(form?.querySelector('button[type="submit"]')?.textContent).toContain('Войти')
  })
})
