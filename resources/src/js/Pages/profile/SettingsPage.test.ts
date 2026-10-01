import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, defineComponent, h, nextTick, reactive } from 'vue'
import SettingsPage from './SettingsPage.vue'
import { setAdminCapabilities } from '@/composables/useAdminCapabilities'

const requests = vi.hoisted(() => ({ post: vi.fn() }))
vi.mock('@inertiajs/vue3', () => ({
  usePage: () => ({ props: {} }),
  router: { on: () => () => {} },
  useForm: (data: Record<string, string>) => {
    const form = reactive({ ...data })
    Object.assign(form, {
      errors: {},
      processing: false,
      reset: (...keys: string[]) => keys.forEach((key) => (form[key] = data[key])),
      post: (path: string, options: object) => requests.post(path, { ...form }, options),
    })
    return form
  },
}))
vi.mock('@/components/layout/AppHead.vue', () => ({
  default: defineComponent({ render: () => null }),
}))
vi.mock('@/components/layout/PageActionBar.vue', () => ({
  default: defineComponent({ render: () => null }),
}))
vi.mock('@/components/form/PasswordStrengthIndicator.vue', () => ({
  default: defineComponent({ render: () => null }),
}))
vi.mock('@/components/form/PasswordInput.vue', () => ({
  default: defineComponent({
    props: ['modelValue'],
    emits: ['update:modelValue'],
    setup:
      (props, { attrs, emit }) =>
      () =>
        h('input', {
          ...attrs,
          type: 'password',
          value: props.modelValue,
          onInput: (event: Event) =>
            emit('update:modelValue', (event.target as HTMLInputElement).value),
        }),
  }),
}))

const apps: ReturnType<typeof createApp>[] = []
afterEach(() => {
  apps.splice(0).forEach((app) => app.unmount())
  requests.post.mockReset()
  setAdminCapabilities({ props: {} })
})

describe('profile email reauthentication', () => {
  it.each(['success', 'failure'])(
    'sends current password and clears it after %s',
    async (outcome) => {
      setAdminCapabilities({ props: { adminCapabilities: { authmail: true } } })
      const root = document.createElement('div')
      const pageProps = { data: { email: 'previous@example.test' } }
      const app = createApp(SettingsPage, pageProps)
      apps.push(app)
      app.mount(root)
      const password = root.querySelector<HTMLInputElement>('input[name="currentPassword"]')!
      expect(password.type).toBe('password')
      expect(password.autocomplete).toBe('current-password')
      password.value = 'password proof '
      password.dispatchEvent(new Event('input', { bubbles: true }))
      await nextTick()
      const send = [...root.querySelectorAll('button')].find((button) =>
        button.textContent?.includes('Отправить код'),
      )!
      send.click()
      expect(requests.post).toHaveBeenCalledOnce()
      const [path, payload, options] = requests.post.mock.calls[0]
      expect(path).toBe('/auth/profile/settings/email/request')
      expect(payload.currentPassword).toBe('password proof ')
      expect(payload.email).toBe('previous@example.test')
      if (outcome === 'success') options.onSuccess()
      options.onFinish()
      await nextTick()
      expect(password.value).toBe('')
    },
  )
})
