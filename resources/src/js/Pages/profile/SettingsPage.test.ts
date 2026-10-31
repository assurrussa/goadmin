import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, defineComponent, h, nextTick, reactive } from 'vue'
import SettingsPage from './SettingsPage.vue'
import { setAdminCapabilities } from '@/composables/useAdminCapabilities'
import { appearance } from '@/appearance/state'

interface MockForm {
  [key: string]: unknown
  errors: Record<string, string>
  processing: boolean
  reset: (...keys: string[]) => void
}
const requests = vi.hoisted(() => ({ post: vi.fn(), forms: [] as MockForm[] }))
const pageState = vi.hoisted(() => ({ props: {} as Record<string, unknown> }))
vi.mock('@inertiajs/vue3', () => ({
  usePage: () => pageState,
  router: { on: () => () => {} },
  useForm: (data: Record<string, string>) => {
    let defaults = { ...data }
    const form = reactive(
      Object.assign({ ...data }, { errors: {} as Record<string, string>, processing: false }),
    )
    const mockedForm = Object.assign(form, {
      defaults: (fields: string | Record<string, string>, value?: string) => {
        Object.assign(defaults, typeof fields === 'string' ? { [fields]: value } : fields)
      },
      reset: (...keys: string[]) => keys.forEach((key) => (form[key] = defaults[key])),
      post: (path: string, options: { onSuccess?: () => void; onFinish?: () => void }) =>
        requests.post(
          path,
          { ...form },
          {
            ...options,
            onSuccess: () => {
              options.onSuccess?.()
              // Inertia snapshots successful form data as the next reset defaults before onFinish.
              defaults = Object.fromEntries(Object.keys(data).map((key) => [key, form[key]]))
            },
          },
        ),
    })
    requests.forms.push(mockedForm)
    return mockedForm
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
  pageState.props = {}
  appearance.brandingRead = false
  window.history.replaceState(null, '', '/auth/profile/settings')
  vi.unstubAllGlobals()
})

describe('settings section navigation', () => {
  function enableSections() {
    pageState.props = { adminSSOEnabled: true }
    appearance.brandingRead = true
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({ version: 0, payload: {}, linked: false }),
      }),
    )
  }

  it.each([
    ['sso-settings-title', 'account', 'sso-settings-title'],
    ['profile-settings-title', 'account', 'profile-settings-title'],
    ['security-settings-title', 'security', 'security-settings-title'],
    ['interface', 'interface', 'appearance-title'],
    ['account', 'account', 'profile-settings-title'],
    ['security', 'security', 'security-settings-title'],
    ['branding', 'branding', 'branding-title'],
    ['unknown', 'interface', 'appearance-title'],
  ])('opens #%s in the visible %s section', async (hash, section, heading) => {
    enableSections()
    window.history.replaceState(null, '', '/auth/profile/settings#' + hash)
    const { root } = mountSettings()
    await nextTick()
    expect(root.querySelector('nav a[aria-current="page"]')?.getAttribute('href')).toBe(
      '#' + section,
    )
    const content = root.querySelector<HTMLElement>('section[aria-labelledby="' + heading + '"]')!
    expect(content).not.toBeNull()
    expect(content.style.display).not.toBe('none')
  })

  it('restores legacy and new sections through real Back/Forward hash navigation', async () => {
    enableSections()
    window.history.replaceState(null, '', '/auth/profile/settings#sso-settings-title')
    const { root } = mountSettings()
    await nextTick()
    async function navigate(action: () => void) {
      const changed = new Promise<void>((resolve) =>
        window.addEventListener('hashchange', () => resolve(), { once: true }),
      )
      action()
      await changed
      await nextTick()
    }
    const selected = () => root.querySelector('nav a[aria-current="page"]')?.getAttribute('href')
    expect(selected()).toBe('#account')
    await navigate(() => {
      window.location.hash = '#security'
    })
    expect(selected()).toBe('#security')
    await navigate(() => window.history.back())
    expect(selected()).toBe('#account')
    expect(
      root.querySelector<HTMLElement>('section[aria-labelledby="sso-settings-title"]')!.style
        .display,
    ).not.toBe('none')
    await navigate(() => window.history.forward())
    expect(selected()).toBe('#security')
  })
})

describe('optional SSO settings', () => {
  it('does not load or show SSO when the adapter is absent', async () => {
    const fetch = vi.fn()
    vi.stubGlobal('fetch', fetch)
    const { root } = mountSettings()
    await nextTick()
    expect(root.textContent).not.toContain('AuthHub SSO')
    expect(fetch).not.toHaveBeenCalled()
  })

  it.each([
    [
      { linked: true, pending: false, localAuth: 'not_local' },
      'Учётная запись AuthHub привязана.',
      false,
    ],
    [
      { linked: false, pending: false, localAuth: 'fresh' },
      'Учётная запись AuthHub не привязана.',
      true,
    ],
    [{ linked: false, pending: true, localAuth: 'stale' }, 'Завершите привязку', true],
    [{ linked: false, pending: true, localAuth: 'not_local' }, 'войдите локально', false],
  ] as const)('shows canonical account status %#', async (status, message, showLink) => {
    pageState.props = { adminSSOEnabled: true }
    const fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => status })
    vi.stubGlobal('fetch', fetch)
    const { root } = mountSettings()
    await new Promise((resolve) => setTimeout(resolve, 0))
    await nextTick()
    expect(fetch).toHaveBeenCalledWith(
      '/sso-account',
      expect.objectContaining({ credentials: 'same-origin' }),
    )
    expect(root.textContent).toContain(message)
    expect(
      root.querySelector('a[href="/sso-link"]') !== null ||
        root.querySelector('a[href="/sso?redirectback=%2Fauth%2Fprofile%2Fsettings"]') !== null,
    ).toBe(showLink)
    if (status.linked) expect(root.textContent).not.toContain('Завершите привязку')
  })
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
      expectEmptySecretDefaults(requests.forms[0], ['currentPassword'])
    },
  )
})

function expectEmptySecretDefaults(form: MockForm, keys: string[]) {
  for (const key of keys) form[key] = 'another value'
  form.reset(...keys)
  for (const key of keys) expect(form[key]).toBe('')
}

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

  it.each(['success', 'failure'])('clears a confirmation code after %s', async (outcome) => {
    const { root } = mountSettings({
      newEmail: 'destination@example.test',
      expiresAt: '2099-01-01T00:00:00Z',
    })
    requests.forms[1].code = '123456'
    submit(root, 'Подтверждение email')
    expect(requests.post.mock.calls[0][0]).toBe('/auth/profile/settings/email/confirm')
    expect(requests.post.mock.calls[0][1].code).toBe('123456')
    const options = requests.post.mock.calls[0][2]
    if (outcome === 'success') options.onSuccess()
    options.onFinish()
    await nextTick()
    expect(requests.forms[1].code).toBe('')
    expectEmptySecretDefaults(requests.forms[1], ['code'])
  })

  it.each(['success', 'failure'])(
    'submits password independently and clears secrets after %s',
    async (outcome) => {
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
      if (outcome === 'success') options.onSuccess()
      options.onFinish()
      await nextTick()
      expect(requests.forms[2].currentPassword).toBe('')
      expect(requests.forms[2].password).toBe('')
      expect(requests.forms[2].passwordConfirmation).toBe('')
      expectEmptySecretDefaults(requests.forms[2], [
        'currentPassword',
        'password',
        'passwordConfirmation',
      ])
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
