import { afterEach, expect, it, vi } from 'vitest'
import { createApp, defineComponent, h, nextTick, reactive } from 'vue'
import RecordEditForm from './RecordEditForm.vue'
import { setLocale, t } from '@/i18n'

const mocks = vi.hoisted(() => ({ before: vi.fn() }))
vi.mock('@inertiajs/vue3', () => ({
  usePage: () => ({ props: {} }),
  router: { on: mocks.before },
  Link: defineComponent({
    props: ['href'],
    setup:
      (props, { slots }) =>
      () =>
        h('a', { href: props.href }, slots.default?.()),
  }),
}))
let app: ReturnType<typeof createApp> | undefined
afterEach(() => {
  setLocale('ru')
  app?.unmount()
  document.body.innerHTML = ''
  vi.unstubAllGlobals()
  mocks.before.mockReset()
})

it('shares explicit list links, guards dirty navigation and disables actions during submission', async () => {
  mocks.before.mockReturnValue(() => {})
  const confirm = vi.fn(() => false)
  vi.stubGlobal('confirm', confirm)
  const form = reactive({ errors: {}, processing: false, isDirty: true })
  const submit = vi.fn()
  const root = document.createElement('div')
  document.body.append(root)
  app = createApp({
    render: () =>
      h(
        RecordEditForm,
        { form, fields: ['name'], returnHref: '/users?page=3', label: 'Edit', onSubmit: submit },
        { default: () => h('input', { name: 'name' }) },
      ),
  })
  app.mount(root)
  expect([...root.querySelectorAll('a')].map((a) => a.getAttribute('href'))).toEqual([
    '/users?page=3',
    '/users?page=3',
  ])
  const guard = mocks.before.mock.calls[0][1]
  expect(guard({ detail: { visit: { method: 'get' } } })).toBe(false)
  expect(confirm).toHaveBeenCalledOnce()
  root.querySelector('form')!.dispatchEvent(new Event('submit', { cancelable: true }))
  expect(submit).toHaveBeenCalledOnce()
  form.processing = true
  await nextTick()
  root.querySelector('form')!.dispatchEvent(new Event('submit', { cancelable: true }))
  expect(submit).toHaveBeenCalledOnce()
  expect(root.querySelector('fieldset')!.disabled).toBe(true)
  expect(root.querySelectorAll('[inert]')).toHaveLength(2)
  form.processing = false
  form.isDirty = false
  await nextTick()
  expect(guard({ detail: { visit: { method: 'get' } } })).toBeUndefined()
  expect(confirm).toHaveBeenCalledOnce()
})

it('shows a non-field server error without its technical key and focuses it', async () => {
  mocks.before.mockReturnValue(() => {})
  const form = reactive({ errors: {} as Record<string, string>, processing: true, isDirty: true })
  const root = document.createElement('div')
  document.body.append(root)
  app = createApp(RecordEditForm, { form, fields: ['name'], returnHref: '/admins', label: 'Edit' })
  app.mount(root)
  form.errors.form = 'Результат операции неизвестен.'
  form.processing = false
  await nextTick()
  await nextTick()
  const alert = root.querySelector('[role="alert"]')!
  expect(alert.textContent).toContain('Результат операции неизвестен.')
  expect(alert.textContent).not.toContain('form:')
  expect(document.activeElement).toBe(alert)
  expect(form.isDirty).toBe(true)
})

it('updates list navigation and page label when locale changes without remounting', async () => {
  mocks.before.mockReturnValue(() => {})
  setLocale('en')
  const root = document.createElement('div')
  app = createApp({
    render: () =>
      h(RecordEditForm, {
        form: { errors: {}, processing: false },
        fields: [],
        returnHref: '/users',
        label: t('Редактирование'),
      }),
  })
  app.mount(root)
  expect(root.querySelector('a')?.textContent).toContain('Back to list')
  setLocale('ru')
  await nextTick()
  expect(root.querySelector('a')?.textContent).toContain('К списку')
  expect(root.textContent).toContain('Редактирование')
  expect(root.textContent).not.toContain('Back to list')
})
