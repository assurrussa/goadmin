import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, defineComponent, h, nextTick, reactive } from 'vue'
import SettingsPage from './SettingsPage.vue'
import { setAdminCapabilities } from '@/composables/useAdminCapabilities'

interface MockForm {
  [key: string]: unknown
  errors: Record<string, string>
  processing: boolean
}
const requests = vi.hoisted(() => ({ post: vi.fn(), forms: [] as MockForm[] }))
vi.mock('@inertiajs/vue3', () => ({
  usePage: () => ({ props: {} }),
  router: { on: () => () => {} },
  useForm: (data: Record<string, string>) => {
    const form = reactive(
      Object.assign({ ...data }, { errors: {} as Record<string, string>, processing: false }),
    )
    Object.assign(form, {
      errors: {},
      processing: false,
      reset: (...keys: string[]) => keys.forEach((key) => (form[key] = data[key])),
      post: (path: string, options: object) => requests.post(path, { ...form }, options),
    })
    requests.forms.push(form)
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
  requests.forms.length = 0
  vi.useRealTimers()
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
      expect(send.type).toBe('submit')
      send.closest('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
      expect(requests.post).toHaveBeenCalledOnce()
      const [path, payload, options] = requests.post.mock.calls[0]
      expect(path).toBe('/auth/profile/settings/email/request')
      expect(payload.currentPassword).toBe('password proof ')
      expect(payload.email).toBe('previous@example.test')
      if (outcome === 'success') options.onSuccess?.()
      options.onFinish()
      await nextTick()
      expect(password.value).toBe('')
    },
  )
})

function mountSettings(pendingEmailChange?: { newEmail: string; expiresAt: string }) {
  const root = document.createElement('div')
  const props = reactive({ data: { email: 'previous@example.test' }, pendingEmailChange })
  const app = createApp({ render: () => h(SettingsPage, props) })
  apps.push(app)
  app.mount(root)
  return { root, props }
}

function submit(root: HTMLElement, label: string) {
  root
    .querySelector(`form[aria-label="${label}"]`)!
    .dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
}

describe('independent settings forms', () => {
  it('shows confirmation only for a pending change and expires it while mounted', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-10-16T10:00:00Z'))
    const { root, props } = mountSettings()
    expect(root.querySelector('input[name="code"]')).toBeNull()
    props.pendingEmailChange = {
      newEmail: 'destination@example.test',
      expiresAt: '2026-10-16T10:00:01Z',
    }
    await nextTick()
    expect(root.textContent).toContain('destination@example.test')
    expect(root.textContent).toContain('Действует до')
    expect(root.querySelector('input[name="code"]')).not.toBeNull()
    requests.forms[1].code = '123456'
    await vi.advanceTimersByTimeAsync(1000)
    expect(root.querySelector('input[name="code"]')).toBeNull()
    expect(requests.forms[1].code).toBe('')
    expect(root.textContent).toContain('Запросите новый код')
    expect(root.querySelector('form[aria-label="Запрос смены email"]')).not.toBeNull()
  })

  it.each(['success', 'failure'])('clears a confirmation code after %s', async () => {
    const { root } = mountSettings({
      newEmail: 'destination@example.test',
      expiresAt: '2099-01-01T00:00:00Z',
    })
    requests.forms[1].code = '123456'
    submit(root, 'Подтверждение email')
    expect(requests.post.mock.calls[0][0]).toBe('/auth/profile/settings/email/confirm')
    expect(requests.post.mock.calls[0][1].code).toBe('123456')
    requests.post.mock.calls[0][2].onFinish()
    await nextTick()
    expect(requests.forms[1].code).toBe('')
  })

  it.each(['success', 'failure'])(
    'submits password independently and clears secrets after %s',
    async () => {
      const { root } = mountSettings()
      Object.assign(requests.forms[2], {
        currentPassword: 'proof',
        password: 'new password',
        passwordConfirmation: 'new password',
      })
      await nextTick()
      submit(root, 'Смена пароля')
      const [path, payload, options] = requests.post.mock.calls[0]
      expect(path).toBe('/auth/profile/settings/password')
      expect(payload.passwordConfirmation).toBe('new password')
      options.onFinish()
      await nextTick()
      expect(requests.forms[2].currentPassword).toBe('')
      expect(requests.forms[2].password).toBe('')
      expect(requests.forms[2].passwordConfirmation).toBe('')
      expect(root.textContent).toContain('потребуется войти заново')
    },
  )

  it('guards repeated native submissions and keeps request and confirmation mutually exclusive', async () => {
    const { root } = mountSettings({
      newEmail: 'destination@example.test',
      expiresAt: '2099-01-01T00:00:00Z',
    })
    requests.forms[0].processing = true
    submit(root, 'Запрос смены email')
    submit(root, 'Подтверждение email')
    requests.forms[0].processing = false
    requests.forms[1].processing = true
    submit(root, 'Запрос смены email')
    submit(root, 'Подтверждение email')
    Object.assign(requests.forms[2], {
      currentPassword: 'proof',
      password: 'new password',
      passwordConfirmation: 'new password',
      processing: true,
    })
    await nextTick()
    submit(root, 'Смена пароля')
    expect(requests.post).not.toHaveBeenCalled()
  })

  it('renders the canonical passwordConfirmation server error', async () => {
    const { root } = mountSettings()
    requests.forms[2].errors = { passwordConfirmation: 'Server mismatch' }
    await nextTick()
    expect(root.querySelector('input[name="passwordConfirmation"]')!.getAttribute('error')).toBe(
      'Server mismatch',
    )
  })

  it.each([
    [{}, true],
    [{ adminCapabilities: {} }, false],
    [{ adminCapabilities: { authmail: false } }, false],
    [{ adminCapabilities: { authmail: true } }, true],
  ])('gates email by capability while retaining password settings: %j', (props, emailAvailable) => {
    setAdminCapabilities({ props })
    const { root } = mountSettings()
    expect(Boolean(root.querySelector('form[aria-label="Запрос смены email"]'))).toBe(
      emailAvailable,
    )
    expect(root.querySelector('form[aria-label="Смена пароля"]')).not.toBeNull()
    expect(root.textContent).toContain('Безопасность')
  })
})

it('retains the existing MFA component inside Security with independent password controls', () => {
  setAdminCapabilities({ props: { adminCapabilities: { authmail: false } } })
  const root = document.createElement('div')
  const pageProps = {
    data: { email: 'sso@example.test' },
    mfaAvailable: true,
    mfaEnabled: true,
  }
  const app = createApp(SettingsPage, pageProps)
  apps.push(app)
  app.mount(root)
  const security = root.querySelector('section[aria-labelledby="security-settings-title"]')!
  expect(security.textContent).toContain('Двухфакторная защита')
  expect(security.textContent).toContain('Защита включена')
  expect(security.querySelector('input[name="mfaPassword"]')).not.toBeNull()
  expect(security.querySelector('form[aria-label="Смена пароля"]')).not.toBeNull()
})
