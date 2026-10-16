import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, defineComponent, h, reactive } from 'vue'
import { createPinia } from 'pinia'
import ProfileEdit from './EditPage.vue'
import AdminEdit from '../admin/EditPage.vue'
import ProfileView from './ViewPage.vue'
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
