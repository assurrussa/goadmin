import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, defineComponent, h, reactive } from 'vue'
import { createPinia } from 'pinia'
import ProfileEdit from './EditPage.vue'
import AdminEdit from '../admin/EditPage.vue'
import ProfileView from './ViewPage.vue'
import ProfileSource from './SourcePage.vue'
import { setAdminCapabilities } from '@/composables/useAdminCapabilities'

const state = vi.hoisted(() => ({
  pageProps: {} as Record<string, unknown>,
  forms: [] as Array<Record<string, unknown>>,
}))
vi.mock('@inertiajs/vue3', () => ({
  usePage: () => ({ props: state.pageProps }),
  useForm: (data: object) => {
    const form = reactive({ ...data, errors: {}, post: vi.fn(), put: vi.fn(), processing: false })
    state.forms.push(form)
    return form
  },
  Link: defineComponent({
    props: ['href'],
    setup:
      (props, { slots }) =>
      () =>
        h('a', { href: props.href }, slots.default?.()),
  }),
  router: { on: () => () => {} },
}))
vi.mock('@/components/layout/AppHead.vue', () => ({
  default: defineComponent({ render: () => null }),
}))
vi.mock('@/components/form', () => ({
  FormImageUploader: defineComponent({ render: () => h('div', { 'data-avatar-uploader': true }) }),
}))

const apps: ReturnType<typeof createApp>[] = []
const data = {
  id: 7,
  name: 'Анна',
  lastName: 'Иванова',
  username: 'anna',
  email: 'anna@example.test',
  createdAt: '2026-10-01T12:00:00Z',
  updatedAt: '2026-10-01T12:00:00Z',
}
afterEach(() => {
  apps.splice(0).forEach((app) => app.unmount())
  state.pageProps = {}
  state.forms = []
  try {
    window.sessionStorage.clear()
  } catch {
    /* No state to clean in opaque test origins. */
  }
  setAdminCapabilities({ props: {} })
})

for (const [label, component] of [
  ['profile', ProfileEdit],
  ['administrator', AdminEdit],
] as const) {
  describe(`${label} editing`, () => {
    it('preserves cleared fields after validation, including an invalid required name', () => {
      state.pageProps = { old: { name: '', lastName: '', username: '' } }
      const root = document.createElement('div')
      const app = createApp(component, { data })
      apps.push(app)
      app.mount(root)
      for (const field of ['name', 'lastName', 'username']) {
        expect(root.querySelector<HTMLInputElement>(`input[name="${field}"]`)?.value).toBe('')
      }
      const name = root.querySelector<HTMLInputElement>('input[name="name"]')!
      expect(name.required).toBe(true)
      expect(name.checkValidity()).toBe(false)
      expect(root.querySelector('a[href]')?.getAttribute('href')).toBe(
        label === 'profile' ? '/auth/profile' : '/admins',
      )
      expect(root.textContent).toContain('Отмена')
    })

    it('falls back only for missing fields and preserves explicit null optional values', () => {
      state.pageProps = { old: { lastName: null } }
      const root = document.createElement('div')
      const app = createApp(component, { data })
      apps.push(app)
      app.mount(root)
      expect(state.forms[0].name).toBe(data.name)
      expect(state.forms[0].lastName).toBeNull()
      expect(state.forms[0].username).toBe(data.username)
    })
  })
}

it('shows canonical readonly administrator email after a rejected email change', () => {
  state.pageProps = { old: { email: 'changed@example.test' } }
  setAdminCapabilities({ props: { adminCapabilities: { authmail: true } } })
  const root = document.createElement('div')
  const editProps = { data }
  const app = createApp(AdminEdit, editProps)
  apps.push(app)
  app.mount(root)
  const email = root.querySelector<HTMLInputElement>('input[name="email"]')!
  expect(email.value).toBe(data.email)
  expect(email.readOnly).toBe(true)
  expect(state.forms[0].email).toBe(data.email)
})

it.each([undefined, {}, { uploads: true }])(
  'renders truthful overview with capabilities %j',
  (capabilities) => {
    setAdminCapabilities({
      props: capabilities === undefined ? {} : { adminCapabilities: capabilities },
    })
    const root = document.createElement('div')
    const viewProps = { data: { ...data, username: '' } }
    const app = createApp(ProfileView, viewProps)
    apps.push(app)
    app.use(createPinia()).mount(root)
    expect(root.textContent).not.toContain('Активен')
    expect(root.textContent).not.toContain('История изменений')
    expect(root.textContent).not.toContain('Последнее обновление')
    expect(root.querySelectorAll('a[href="/auth/profile/edit"]')).toHaveLength(1)
    expect(root.querySelector('a[href="/auth/profile/settings"]')?.textContent).toContain(
      'Настройки',
    )
    expect(root.textContent).toContain('Не указано')
    expect(
      [...root.querySelectorAll('span')].some((span) => span.textContent?.trim() === '@'),
    ).toBe(false)
    expect(root.querySelector('[data-avatar-uploader]') !== null).toBe(
      capabilities === undefined || capabilities.uploads === true,
    )
  },
)

it('remounts the self-profile form from canonical GET props after success', () => {
  const root = document.createElement('div')
  const app = createApp(ProfileEdit, { data })
  apps.push(app)
  app.mount(root)
  root.querySelector('form')!.dispatchEvent(new Event('submit', { cancelable: true }))
  expect(state.forms[0].post).toHaveBeenCalledWith('/auth/profile/edit', {
    preserveState: 'errors',
  })
})

it('keeps profile source local by default and refreshes only on an explicit click', () => {
  const root = document.createElement('div')
  const sourceProps = {
    presentation: {
      version: '9007199254740993',
      updatedAt: '',
      fields: { name: false, login: false, avatar: false },
    },
    avatarAvailable: false,
  }
  const app = createApp(ProfileSource, sourceProps)
  apps.push(app)
  app.mount(root)
  expect(root.querySelectorAll('input[type="checkbox"]')).toHaveLength(2)
  expect(
    [...root.querySelectorAll<HTMLInputElement>('input[type="checkbox"]')].every(
      (input) => !input.checked,
    ),
  ).toBe(true)
  expect(root.querySelector('img')).toBeNull()
  expect(root.textContent).toContain('Внешние аватары отключены')
  expect(root.textContent).toContain('Локальные значения сохраняются')
  expect(state.forms[0].version).toBe('9007199254740993')
  expect(state.forms[0].post).not.toHaveBeenCalled()
  const refresh = [...root.querySelectorAll('button')].find((button) =>
    button.textContent?.includes('Обновить из SSO'),
  )
  refresh?.click()
  expect(state.forms[0].post).toHaveBeenCalledWith('/profile-source/refresh', {
    preserveState: 'errors',
  })
})

it('overlays only selected display fields and does not edit the local source form', () => {
  setAdminCapabilities({ props: { adminCapabilities: { uploads: false } } })
  state.pageProps = {
    authuser: {
      profilePresentation: {
        enabled: true,
        available: true,
        name: 'SSO Name',
        login: 'sso-label',
        avatarUrl: 'https://images.example.test/avatar.png',
        fields: { name: true, login: true, avatar: true },
      },
    },
  }
  const root = document.createElement('div')
  const app = createApp(ProfileView, { data })
  apps.push(app)
  app.use(createPinia()).mount(root)
  expect(root.querySelector('h1')?.textContent).toContain('SSO Name')
  expect(root.textContent).toContain('@sso-label')
  expect(root.querySelector('img')?.getAttribute('src')).toBe(
    'https://images.example.test/avatar.png',
  )
  expect(root.querySelector('img')?.getAttribute('referrerpolicy')).toBe('no-referrer')
  expect(root.textContent).toContain(data.email)
  expect(root.querySelector('a[href="/profile-source"]')).not.toBeNull()
  const editRoot = document.createElement('div')
  const edit = createApp(ProfileEdit, { data })
  apps.push(edit)
  edit.mount(editRoot)
  expect(editRoot.querySelector<HTMLInputElement>('input[name="name"]')?.value).toBe(data.name)
  expect(editRoot.querySelector<HTMLInputElement>('input[name="username"]')?.value).toBe(
    data.username,
  )
})

it('falls back locally for unavailable or unselected source snapshots', () => {
  state.pageProps = {
    authuser: {
      profilePresentation: {
        enabled: true,
        available: false,
        name: 'Stale SSO Name',
        login: 'stale-label',
        avatarUrl: 'https://images.example.test/avatar.png',
        fields: { name: true, login: true, avatar: true },
      },
    },
  }
  const root = document.createElement('div')
  const app = createApp(ProfileView, { data })
  apps.push(app)
  app.use(createPinia()).mount(root)
  expect(root.querySelector('h1')?.textContent).toContain(data.name)
  expect(root.textContent).not.toContain('Stale SSO Name')
  expect(root.querySelector('img[src^="https://images.example.test"]')).toBeNull()
})

it('automatically completes only the armed native navigation once without secrets', () => {
  const replace = vi.spyOn(window.history, 'replaceState')
  const root = document.createElement('div')
  const app = createApp(ProfileSource, {
    presentation: {
      version: '2',
      updatedAt: '',
      fields: { name: false, login: false, avatar: false },
    },
    avatarAvailable: false,
    completionReady: true,
  })
  apps.push(app)
  app.mount(root)
  expect(replace).toHaveBeenCalledWith(window.history.state, '', '/profile-source')
  expect(state.forms[0].post).toHaveBeenCalledTimes(1)
  expect(state.forms[0].post).toHaveBeenCalledWith('/profile-source/complete', {
    preserveState: 'errors',
  })
  expect(Object.keys(state.forms[0])).not.toContain('claim')
  expect(Object.keys(state.forms[0])).not.toContain('selector')
  replace.mockRestore()
})

it('does not auto-retry completion after an interrupted attempt or history restoration', () => {
  window.sessionStorage.setItem('goadmin.profile.completion-attempted', '1')
  const root = document.createElement('div')
  const app = createApp(ProfileSource, {
    presentation: {
      version: '2',
      updatedAt: '',
      fields: { name: false, login: false, avatar: false },
    },
    avatarAvailable: false,
    completionReady: true,
  })
  apps.push(app)
  app.mount(root)
  expect(state.forms[0].post).not.toHaveBeenCalled()
})
