import { afterEach, expect, it, vi } from 'vitest'
import { createApp, defineComponent, h, nextTick, reactive } from 'vue'
import Field from './Field.vue'
import Form from '@/Pages/crud/Form.vue'
import Show from '@/Pages/crud/Show.vue'
import { applyLocale } from '@/i18n'
import { crudRecordHref, crudReturnHref, registerCrudField, type CrudResource } from './index'
const mocks = vi.hoisted(() => ({
  post: vi.fn(),
  put: vi.fn(),
  remove: vi.fn(),
  cancel: vi.fn(),
  on: vi.fn(),
  page: { url: '/products/records/a/edit?returnTo=%2Fproducts%3Fpage%3D3', props: {} },
  form: undefined as unknown,
}))
vi.mock('@inertiajs/vue3', () => ({
  usePage: () => mocks.page,
  router: { on: mocks.on, get: vi.fn(), post: mocks.post, delete: mocks.remove },
  Link: defineComponent({
    props: ['href'],
    setup:
      (props, { slots }) =>
      () =>
        h('a', { href: props.href }, slots.default?.()),
  }),
  Head: defineComponent({ setup: () => () => null }),
  useForm: (values: Record<string, unknown>) => {
    let transform = (data: Record<string, unknown>) => data
    const form = reactive({
      ...values,
      processing: false,
      errors: {},
      isDirty: false,
      transform: (fn: typeof transform) => {
        transform = fn
        return form
      },
      put: (url: string, options: unknown) => mocks.put(url, transform(values), options),
      post: (url: string, options: unknown) => mocks.post(url, transform(values), options),
      cancel: mocks.cancel,
      defaults: vi.fn(),
      reset: vi.fn(),
      clearErrors: vi.fn(),
    })
    mocks.form = form
    return form
  },
}))
let app: ReturnType<typeof createApp> | undefined
let unregister: (() => void) | undefined
function mount(component: Parameters<typeof createApp>[0], props: Record<string, unknown>) {
  const root = document.createElement('div')
  document.body.append(root)
  app = createApp(component, props)
  app.mount(root)
  return root
}
afterEach(() => {
  app?.unmount()
  unregister?.()
  unregister = undefined
  document.body.innerHTML = ''
  applyLocale('ru')
  vi.clearAllMocks()
  vi.unstubAllGlobals()
})
const resource: CrudResource = {
  key: 'products',
  basePath: '/products',
  title: 'Products',
  actions: [],
  fields: [
    {
      key: 'name',
      label: 'Name',
      type: 'text',
      read: true,
      create: true,
      update: true,
      required: true,
    },
    { key: 'active', label: 'Active', type: 'boolean', read: true, create: true, update: true },
  ],
}
it.each([
  ['en', 'Delete', 'Yes', 'No'],
  ['ru', 'Удалить', 'Да', 'Нет'],
])(
  'preserves record data and nullable boolean values in %s detail views',
  (locale, option, yes, no) => {
    applyLocale(locale!)
    const fields: CrudResource['fields'] = [
      { key: 'name', label: 'Name', type: 'text', read: true, create: false, update: false },
      {
        key: 'description',
        label: 'Description',
        type: 'textarea',
        read: true,
        create: false,
        update: false,
      },
      {
        key: 'choice',
        label: 'Choice',
        type: 'select',
        options: [{ value: 'x', label: 'Удалить' }],
        read: true,
        create: false,
        update: false,
      },
      ...['unknown', 'enabled', 'disabled'].map((key) => ({
        key,
        label: key,
        type: 'boolean' as const,
        read: true,
        create: false,
        update: false,
      })),
    ]
    const root = mount(Show, {
      resource: { ...resource, fields },
      record: {
        id: 'a',
        revision: '1',
        data: {
          name: 'Удалить',
          description: 'Да',
          choice: 'x',
          unknown: null,
          enabled: true,
          disabled: false,
        },
      },
      permissions: { create: false, update: false, delete: false },
    })
    expect([...root.querySelectorAll('dd')].map((node) => node.textContent!.trim())).toEqual([
      'Удалить',
      'Да',
      option,
      '—',
      yes,
      no,
    ])
  },
)
it('preserves false and exact revisions and return conditions in the editing form', async () => {
  const root = mount(Form, {
    resource,
    mode: 'update',
    values: { name: 'A', active: false },
    record: { id: 'a', revision: '9007199254740993', data: {} },
  })
  expect(root.querySelector<HTMLInputElement>('[name=active]')!.checked).toBe(false)
  expect(root.querySelector('a')?.getAttribute('href')).toBe('/products?page=3')
  root.querySelector('form')!.dispatchEvent(new Event('submit', { cancelable: true }))
  expect(mocks.put).toHaveBeenCalledWith(
    '/products/records/a?returnTo=%2Fproducts%3Fpage%3D3',
    { name: 'A', active: false, _revision: '9007199254740993' },
    expect.anything(),
  )
  const form = mocks.form as { processing: boolean; errors: Record<string, string> }
  form.processing = true
  await nextTick()
  root.querySelector('form')!.dispatchEvent(new Event('submit', { cancelable: true }))
  expect(mocks.put).toHaveBeenCalledOnce()
  form.processing = false
  form.errors = { form: 'Conflict' }
  await nextTick()
  await nextTick()
  expect(root.querySelector<HTMLInputElement>('[name=name]')!.value).toBe('A')
  expect(root.querySelector('[role=alert]')!.textContent).toContain('Conflict')
})
it('keeps record identity outside create input and cancels work on unmount', () => {
  const root = mount(Form, {
    resource,
    mode: 'create',
    values: { name: '', active: false, secret: 'hidden' },
  })
  root.querySelector('form')!.dispatchEvent(new Event('submit', { cancelable: true }))
  expect(mocks.post).toHaveBeenCalledWith(
    '/products?returnTo=%2Fproducts%3Fpage%3D3',
    { name: '', active: false },
    expect.anything(),
  )
  app!.unmount()
  app = undefined
  expect(mocks.cancel).toHaveBeenCalledOnce()
})
it('binds native dates and select values with labels and accessible error references', async () => {
  const update = vi.fn()
  const root = mount(Field, {
    field: { key: 'available', label: 'Available', type: 'date' },
    modelValue: '2026-11-06',
    error: 'Invalid date',
    'onUpdate:modelValue': update,
  })
  const input = root.querySelector('input')!
  expect(input.type).toBe('date')
  expect(input.value).toBe('2026-11-06')
  expect(root.querySelector('label')!.htmlFor).toBe(input.id)
  expect(document.getElementById(input.getAttribute('aria-describedby')!)!.textContent).toContain(
    'Invalid date',
  )
  input.value = '2026-12-01'
  input.dispatchEvent(new Event('input'))
  await nextTick()
  expect(update).toHaveBeenCalledWith('2026-12-01')
})
it('uses registered custom editors and fences disabled updates', async () => {
  unregister = registerCrudField(
    'host.example/slug',
    defineComponent({
      props: ['modelValue'],
      emits: ['update:modelValue'],
      setup:
        (props, { emit }) =>
        () =>
          h(
            'button',
            { onClick: () => emit('update:modelValue', 'changed') },
            String(props.modelValue),
          ),
    }),
  )
  const update = vi.fn()
  const root = mount(Field, {
    field: { key: 'slug', label: 'Slug', type: 'custom', component: 'host.example/slug' },
    modelValue: 'initial',
    disabled: true,
    'onUpdate:modelValue': update,
  })
  root.querySelector('button')!.click()
  await nextTick()
  expect(update).not.toHaveBeenCalled()
  expect(() => registerCrudField('host.example/slug', {})).toThrow('duplicate')
})
it('fences list return targets and encodes opaque record identities', () => {
  expect(crudReturnHref('/products', '/products/create?returnTo=https://evil.invalid')).toBe(
    '/products',
  )
  expect(crudRecordHref('/products', 'a/b', true, '/products?search=x')).toBe(
    '/products/records/a%2Fb/edit?returnTo=%2Fproducts%3Fsearch%3Dx',
  )
})

it('isolates domain names from form methods and preserves explicit null', () => {
  const fields = ['processing', 'transform', 'defaults'].map((key) => ({
    key,
    label: key,
    type: 'text' as const,
    read: true,
    create: true,
    update: true,
  }))
  const root = mount(Form, {
    resource: { ...resource, fields },
    mode: 'update',
    values: { processing: 'domain', transform: 'value', defaults: null },
    record: { id: 'a', revision: '1', data: {} },
  })
  expect(root.querySelector<HTMLInputElement>('[name=processing]')!.value).toBe('domain')
  root.querySelector('form')!.dispatchEvent(new Event('submit', { cancelable: true }))
  expect(mocks.put).toHaveBeenCalledWith(
    expect.any(String),
    { processing: 'domain', transform: 'value', defaults: null, _revision: '1' },
    expect.anything(),
  )
})
it('does not adopt a new server revision for an old dirty draft after conflict', async () => {
  const state = reactive({
    resource,
    mode: 'update' as const,
    values: { name: 'Draft', active: false },
    record: { id: 'a', revision: '1', data: {} },
  })
  const root = document.createElement('div')
  document.body.append(root)
  app = createApp({ render: () => h(Form, state) })
  app.mount(root)
  state.record = { id: 'a', revision: '2', data: {} }
  state.values = { name: 'Other writer', active: false }
  await nextTick()
  root.querySelector('form')!.dispatchEvent(new Event('submit', { cancelable: true }))
  expect(mocks.put.mock.calls[0][1]).toEqual({ name: 'Draft', active: false, _revision: '1' })
})

it('dispatches a confirmed record command once until its request finishes', async () => {
  mocks.post.mockImplementationOnce((_url, _data, options) => options.onStart())
  vi.stubGlobal(
    'confirm',
    vi.fn(() => true),
  )
  const root = mount(Show, {
    resource: {
      ...resource,
      actions: [{ key: 'publish', label: 'Publish', confirmation: 'Publish?' }],
    },
    record: {
      id: 'a',
      revision: '1',
      data: { name: 'A', active: false },
      actions: ['publish', 'delete'],
    },
    permissions: { create: true, update: true, delete: true },
  })
  const button = [...root.querySelectorAll('button')].find(
    (button) => button.textContent?.trim() === 'Publish',
  )!
  button.click()
  button.click()
  await nextTick()
  expect(mocks.post).toHaveBeenCalledOnce()
  expect(button.disabled).toBe(true)
  const options = mocks.post.mock.calls[0][2]
  options.onFinish()
  await nextTick()
  button.click()
  expect(mocks.post).toHaveBeenCalledTimes(2)
})

it('keeps action controls usable when a global before hook cancels before request start', async () => {
  vi.stubGlobal(
    'confirm',
    vi.fn(() => true),
  )
  const root = mount(Show, {
    resource: {
      ...resource,
      actions: [{ key: 'publish', label: 'Publish', confirmation: 'Publish?' }],
    },
    record: { id: 'a', revision: '1', data: { name: 'A' }, actions: ['publish'] },
    permissions: { create: true, update: true, delete: true },
  })
  const button = [...root.querySelectorAll('button')].find(
    (button) => button.textContent?.trim() === 'Publish',
  )!
  button.click()
  await nextTick()
  expect(button.disabled).toBe(false)
  button.click()
  expect(mocks.post).toHaveBeenCalledTimes(2)
})
