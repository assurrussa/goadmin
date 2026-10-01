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
