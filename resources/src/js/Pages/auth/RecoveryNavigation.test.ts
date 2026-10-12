import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp } from 'vue'
import ForgotPasswordPage from './ForgotPasswordPage.vue'
import ResetPasswordPage from './ResetPasswordPage.vue'

vi.mock('@inertiajs/vue3', async () => {
  const { defineComponent, h, reactive } = await import('vue')
  return {
    Link: defineComponent({
      setup:
        (_, { attrs, slots }) =>
        () =>
          h('a', { ...attrs, 'data-inertia-link': 'true' }, slots.default?.()),
    }),
    Head: defineComponent({ setup: () => () => null }),
    usePage: () => ({ props: {} }),
    useForm: (data: Record<string, unknown>) =>
      reactive({ ...data, errors: {}, post: vi.fn(), processing: false }),
    router: { on: () => () => {} },
  }
})
vi.mock('@/components/layout/AuthLayout.vue', async () => {
  const { defineComponent, h } = await import('vue')
  return {
    default: defineComponent({
      setup:
        (_, { slots }) =>
        () =>
          h('div', [slots.title?.(), slots.default?.(), slots.footer?.()]),
    }),
  }
})
vi.mock('@/components/layout/AppHead.vue', () => ({ default: { setup: () => () => null } }))
vi.mock('@/components/form/FormInput.vue', async () => {
  const { defineComponent, h } = await import('vue')
  return { default: defineComponent({ setup: () => () => h('input') }) }
})
vi.mock('@/components/form/PasswordInput.vue', async () => {
  const { defineComponent, h } = await import('vue')
  return { default: defineComponent({ setup: () => () => h('input') }) }
})
vi.mock('@/components/form/PasswordStrengthIndicator.vue', () => ({
  default: { setup: () => () => null },
}))
vi.mock('@/components/form/FormButton.vue', async () => {
  const { defineComponent, h } = await import('vue')
  return {
    default: defineComponent({
      setup:
        (_, { slots }) =>
        () =>
          h('button', slots.default?.()),
    }),
  }
})
vi.mock('@/components/form/AdminForm.vue', async () => {
  const { defineComponent, h } = await import('vue')
  return {
    default: defineComponent({
      setup:
        (_, { slots }) =>
        () =>
          h('form', slots.default?.()),
    }),
  }
})

let app: ReturnType<typeof createApp> | undefined
afterEach(() => {
  app?.unmount()
  app = undefined
})

describe('cross-origin recovery login navigation', () => {
  for (const [name, component] of [
    ['forgot', ForgotPasswordPage],
    ['reset', ResetPasswordPage],
  ] as const) {
    it(name + ' uses an ordinary anchor for a different login origin', () => {
      const root = document.createElement('div')
      app = createApp(component, {
        token: 'synthetic-token',
        email: 'synthetic@example.test',
        formAction: '/auth/sso/reset-password',
        loginURL: 'https://sso.example.test/sso/account',
      })
      app.mount(root)
      const link = root.querySelector<HTMLAnchorElement>('a')
      expect(link?.href).toBe('https://sso.example.test/sso/account')
      expect(link?.hasAttribute('data-inertia-link')).toBe(false)
    })
    it(name + ' hides absent login destinations', () => {
      const root = document.createElement('div')
      app = createApp(component, { token: 'synthetic-token', email: '', loginURL: '' })
      app.mount(root)
      expect(root.querySelector('a')).toBeNull()
    })
  }
})
