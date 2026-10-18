import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, defineComponent, h, nextTick } from 'vue'
import WaitingPage from '../auth/AdmissionWaitingPage.vue'
import IndexPage from './IndexPage.vue'
import { setLocale } from '@/i18n'

const post = vi.hoisted(() => vi.fn())
vi.mock('@inertiajs/vue3', () => ({
  router: { post, on: () => () => {} },
}))
vi.mock('@/components/layout/AuthLayout.vue', () => ({
  default: defineComponent({
    setup:
      (_, { slots }) =>
      () =>
        h('div', slots.default?.()),
  }),
}))
vi.mock('@/components/layout/AppHead.vue', () => ({
  default: defineComponent({ render: () => null }),
}))
vi.mock('@/components/layout/PageActionBar.vue', () => ({
  default: defineComponent({ render: () => null }),
}))
vi.mock('@/components/ui/AppButton.vue', () => ({
  default: defineComponent({
    setup:
      (_, { attrs, slots }) =>
      () =>
        h('button', attrs, slots.default?.()),
  }),
}))
vi.mock('@/components/form/FormInput.vue', () => ({
  default: defineComponent({
    props: ['modelValue', 'name', 'label'],
    emits: ['update:modelValue'],
    setup:
      (props, { emit }) =>
      () =>
        h('label', [
          props.label,
          h('input', {
            name: props.name,
            value: props.modelValue,
            onInput: (event: Event) =>
              emit('update:modelValue', (event.target as HTMLInputElement).value),
          }),
        ]),
  }),
}))
const apps: ReturnType<typeof createApp>[] = []
afterEach(() => {
  apps.splice(0).forEach((app) => app.unmount())
  post.mockReset()
  setLocale('ru')
})
describe('SSO waiting isolation', () => {
  it.each(['pending', 'approved', 'active', 'rejected'])(
    'renders %s without admin controls',
    (status) => {
      const root = document.createElement('div')
      const app = createApp(WaitingPage, { admissionStatus: status, signInPath: '/sso/login' })
      apps.push(app)
      app.mount(root)
      expect(root.querySelector('[role="status"]')).not.toBeNull()
      expect(root.querySelector('a')?.getAttribute('href')).toBe('/sso/login')
      expect(root.querySelector('form, input, select')).toBeNull()
      expect(post).not.toHaveBeenCalled()
    },
  )
  it.each(['https://foreign.example', '//foreign.example', '/\\foreign.example'])(
    'rejects unsafe retry %s',
    (path) => {
      const root = document.createElement('div')
      const app = createApp(WaitingPage, { admissionStatus: 'pending', signInPath: path })
      apps.push(app)
      app.mount(root)
      expect(root.querySelector('a')?.getAttribute('href')).toBe('/auth/login')
    },
  )
  it('updates RU/EN without issuing a request', async () => {
    const root = document.createElement('div')
    const app = createApp(WaitingPage, { admissionStatus: 'pending', signInPath: '/sso/login' })
    apps.push(app)
    app.mount(root)
    expect(root.textContent).toContain('Ожидайте')
    setLocale('en')
    await nextTick()
    expect(root.textContent).toContain('Wait for an administrator')
    expect(post).not.toHaveBeenCalled()
  })
})
describe('SSO moderation UI', () => {
  it('has no enabled mutation for a read-only view', () => {
    const root = document.createElement('div')
    const app = createApp(IndexPage, {
      records: [],
      invitations: [],
      inviteRoles: [],
      approveRoles: [],
      canInvite: false,
      canReject: false,
    })
    apps.push(app)
    app.mount(root)
    for (const button of root.querySelectorAll('button')) expect(button.disabled).toBe(true)
    expect(post).not.toHaveBeenCalled()
  })
})
