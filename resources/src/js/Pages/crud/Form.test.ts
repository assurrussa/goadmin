import { afterEach, expect, it, vi } from 'vitest'
import { createApp, defineComponent, h, nextTick, reactive } from 'vue'
import { router } from '@inertiajs/vue3'
import type { ActiveVisit, Page, VisitOptions } from '@inertiajs/core'
import Form from './Form.vue'
import type { CrudResource, CrudValue } from '@/crud'
import { exactWireNumber } from '@/crud/number'
import { applyLocale } from '@/i18n'

const page = reactive({
  component: 'crud/Form',
  url: '/products',
  props: { errors: {} } as Page['props'],
})
const mocks = vi.hoisted(() => ({
  form: undefined as
    | {
        setError: (errors: Record<string, string>) => void
        clearErrors: () => void
        values: Record<string, CrudValue>
      }
    | undefined,
}))

vi.mock('@inertiajs/vue3', async (importOriginal) => {
  const original = await importOriginal<typeof import('@inertiajs/vue3')>()
  return {
    ...original,
    usePage: () => page,
    useForm: (...args: unknown[]) => {
      const form = Reflect.apply(original.useForm, undefined, args)
      mocks.form = form
      return form
    },
    Head: defineComponent({ render: () => null }),
    Link: defineComponent({
      props: ['href'],
      setup:
        (props, { slots }) =>
        () =>
          h('a', { href: props.href }, slots.default?.()),
    }),
  }
})

let app: ReturnType<typeof createApp> | undefined
afterEach(() => {
  applyLocale('ru')
  app?.unmount()
  app = undefined
  mocks.form = undefined
  page.component = 'crud/Form'
  page.url = '/products'
  page.props = { errors: {} }
  vi.restoreAllMocks()
  document.body.replaceChildren()
})

it.each(['create', 'update'] as const)(
  'renders inherited object field names and their actual validation errors in %s forms',
  async (mode) => {
    const keys = ['toString', 'valueOf']
    const resource: CrudResource = {
      key: 'products',
      basePath: '/products',
      title: 'Products',
      actions: [],
      fields: keys.map((key) => ({
        key,
        label: key,
        type: 'text',
        read: true,
        create: true,
        update: true,
      })),
    }
    const root = document.createElement('div')
    document.body.append(root)
    app = createApp(Form, {
      resource,
      mode,
      values: Object.fromEntries(keys.map((key) => [key, `${key} value`])),
      record: mode === 'update' ? { id: 'a', revision: '1', data: {} } : undefined,
    })
    app.mount(root)
    await nextTick()

    const inputs = keys.map((key) => root.querySelector<HTMLInputElement>(`[name="${key}"]`)!)
    expect(inputs.map((input) => input.value)).toEqual(keys.map((key) => `${key} value`))
    expect(inputs.map((input) => input.getAttribute('aria-invalid'))).toEqual(['false', 'false'])

    const errors = Object.fromEntries(keys.map((key) => [key, `${key} is invalid`]))
    mocks.form!.setError(errors)
    await nextTick()
    for (const [index, key] of keys.entries()) {
      const input = inputs[index]!
      expect(input.getAttribute('aria-invalid')).toBe('true')
      expect(document.getElementById(input.getAttribute('aria-describedby')!)?.textContent).toBe(
        errors[key],
      )
    }

    mocks.form!.clearErrors()
    await nextTick()
    expect(inputs.map((input) => input.getAttribute('aria-invalid'))).toEqual(['false', 'false'])
    expect(inputs.map((input) => input.value)).toEqual(keys.map((key) => `${key} value`))
  },
)

const owner = { idExact: '9007199254740993' }
const listHref = '/products?page=2&limit=1&search=tools'
const resource: CrudResource = {
  key: 'products',
  basePath: '/products',
  title: 'Products',
  actions: [],
  fields: [
    { key: 'name', label: 'Name', type: 'text', read: true, create: true, update: true },
    { key: 'notes', label: 'Notes', type: 'text', read: true, create: true, update: true },
    { key: 'token', label: 'Token', type: 'text', read: false, create: true, update: true },
    {
      key: 'requiredToken',
      label: 'Required token',
      type: 'text',
      read: false,
      create: false,
      update: true,
      required: true,
    },
  ],
}
const visit = { cancelled: false, interrupted: false } as ActiveVisit
const attemptHeader = 'X-GoAdmin-Record-Attempt'
const defaultValues = { name: 'Original name', notes: 'Original notes' }
type Values = Record<string, CrudValue>

function inertiaPage(props: Page['props']): Page {
  return {
    ...page,
    props,
    version: null,
    clearHistory: false,
    encryptHistory: false,
    flash: {},
    rememberedState: {},
  }
}

function render(
  mode: 'create' | 'update' = 'update',
  requiredSecret: boolean | 'date' = false,
  returnTo = listHref,
  fields = resource.fields,
  values: Values = defaultValues,
) {
  const root = document.createElement('div')
  document.body.append(root)
  const data = reactive({
    resource: {
      ...resource,
      fields: fields
        .filter(({ required }) => requiredSecret || !required)
        .map((field) =>
          field.required && requiredSecret === 'date' ? { ...field, type: 'date' as const } : field,
        ),
    },
    mode,
    values: { ...values },
    record: mode === 'update' ? { id: 'a', revision: 'r1', data: {} } : undefined,
  })
  page.url = `/products/records/a/edit?${new URLSearchParams({ returnTo })}`
  page.props = { authuser: owner, errors: {}, ...data }
  app = createApp({ render: () => h(Form, { ...data }) })
  app.mount(root)
  const put = vi.spyOn(router, 'put').mockImplementation((_url, _values, options) => {
    options?.onBefore?.(visit)
    options?.onStart?.(visit)
  })
  const get = vi.spyOn(router, 'get').mockImplementation(() => {})
  function setResponse(revision: string, values: Values, flash: Record<string, unknown>) {
    data.record = { id: 'a', revision, data: {} }
    data.values = values
    page.props = { authuser: owner, errors: {}, ...data, flash }
    return inertiaPage(page.props)
  }
  async function conflict(
    revision: string,
    values: Values,
    attempt = options().headers?.[attemptHeader],
  ) {
    const callbacks = options()
    setResponse(revision, values, { crudEditConflict: '1', recordEditAttempt: attempt })
    await nextTick()
    callbacks.onError?.({ form: 'Record changed' })
    callbacks.onFinish?.(visit)
    await nextTick()
    await nextTick()
  }
  async function success(revision: string, values: Values, response?: Partial<Page['props']>) {
    const callbacks = options()
    const result = setResponse(revision, values, {
      success: 'Saved',
      recordEditAttempt: callbacks.headers?.[attemptHeader],
    })
    if (response) result.props = { ...result.props, ...response }
    await nextTick()
    await callbacks.onSuccess?.(result)
    callbacks.onFinish?.(visit)
    await nextTick()
    await nextTick()
  }
  function options() {
    return put.mock.lastCall?.[2] as VisitOptions
  }
  return { root, data, put, get, conflict, success, options, setResponse }
}

function input(root: HTMLElement, name: string, value?: string) {
  const element = root.querySelector<HTMLInputElement>(`input[name="${name}"]`)!
  if (value !== undefined) {
    element.value = value
    element.dispatchEvent(new Event('input', { bubbles: true }))
  }
  return element
}

function submit(root: HTMLElement, returnToList = false) {
  root.querySelector('form')!.dispatchEvent(
    new SubmitEvent('submit', {
      bubbles: true,
      cancelable: true,
      submitter: returnToList
        ? root.querySelector<HTMLButtonElement>('button[data-save-and-return]')
        : null,
    }),
  )
}

function button(root: HTMLElement, label: string) {
  return [...root.querySelectorAll('button')].find(
    (element) => element.textContent?.trim() === label,
  )!
}

function choice(root: HTMLElement, name: string, value: 'mine' | 'latest') {
  const options = root.querySelectorAll<HTMLInputElement>(`[name="crud-conflict:${name}"]`)
  return options[value === 'mine' ? 0 : 1]!
}

it.each([false, true])(
  'issues one PUT and returns to the confirmed list with returnTo filtering %s',
  async (returnToFilter) => {
    const returnTo = returnToFilter
      ? listHref + '&returnTo=https%3A%2F%2Fexample.invalid'
      : listHref
    const { root, data, put, get, options, success } = render('update', false, returnTo)
    data.resource.returnToFilter = returnToFilter
    input(root, 'name', 'Saved name')
    await nextTick()
    submit(root, true)
    submit(root, true)
    expect(put).toHaveBeenCalledExactlyOnceWith(
      `/products/records/a?${new URLSearchParams({ returnTo })}`,
      { ...defaultValues, name: 'Saved name', _revision: 'r1' },
      expect.any(Object),
    )
    expect(options().headers?.['X-GoAdmin-Record-ID']).toBe('a')
    expect(options().headers?.[attemptHeader]).toMatch(/^[\da-f]{8}(?:-[\da-f]{4}){3}-[\da-f]{12}$/)
    expect(get).not.toHaveBeenCalled()
    await success('r2', { ...defaultValues, name: 'Saved name' })
    expect(get).toHaveBeenCalledExactlyOnceWith(returnTo)
  },
)

it('uses the confirmed response URL when the server drops the requested return destination', async () => {
  const { root, put, get, success } = render('update', false, listHref)
  input(root, 'name', 'Saved name')
  submit(root, true)
  expect(put.mock.lastCall?.[0]).toBe(
    `/products/records/a?${new URLSearchParams({ returnTo: listHref })}`,
  )
  page.url = '/products/records/a/edit'
  await success('r2', { ...defaultValues, name: 'Saved name' })
  expect(get).toHaveBeenCalledExactlyOnceWith('/products')
})

it('returns after saving a required write-only date even though the cleared input requires a future value', async () => {
  const { root, put, get, success } = render('update', 'date')
  for (const [part, value] of Object.entries({ День: '04', Месяц: '07', Год: '2026' })) {
    const input = root.querySelector<HTMLInputElement>(
      `input[aria-label="Required token: ${part}"]`,
    )!
    input.value = value
    input.dispatchEvent(new Event('input', { bubbles: true }))
  }
  await nextTick()
  submit(root, true)
  expect(put.mock.lastCall?.[1]).toMatchObject({ requiredToken: '2026-07-04' })
  await success('r2', defaultValues)
  expect(root.querySelector<HTMLButtonElement>('button[data-save-and-return]')?.disabled).toBe(true)
  expect(get).toHaveBeenCalledExactlyOnceWith(listHref)
})

it('uses the latest submit intent when saving again after validation fails', async () => {
  const { root, get, options, success } = render()
  input(root, 'name', 'Rejected')
  submit(root, true)
  options().onError?.({ name: 'Invalid' })
  options().onFinish?.(visit)
  await nextTick()
  await nextTick()
  input(root, 'name', 'Accepted')
  submit(root)
  await success('r2', { ...defaultValues, name: 'Accepted' })
  expect(get).not.toHaveBeenCalled()
})

it('keeps Enter as Save and uses the canonical list for an invalid return destination', async () => {
  const { root, put, get, success } = render('update', false, 'https://evil.invalid/products')
  input(root, 'name', 'Saved with Enter')
  submit(root)
  expect(put.mock.lastCall?.[0]).toBe('/products/records/a')
  await success('r2', { ...defaultValues, name: 'Saved with Enter' })
  expect(get).not.toHaveBeenCalled()
  input(root, 'name', 'Saved with intent')
  submit(root, true)
  await success('r3', { ...defaultValues, name: 'Saved with intent' })
  expect(get).toHaveBeenCalledExactlyOnceWith('/products')
})

it('does not show Save and return in create mode', () => {
  expect(render('create').root.querySelector('button[data-save-and-return]')).toBeNull()
})

const constrainedFields: CrudResource['fields'] = [
  { ...resource.fields[0]!, maxLength: 200, required: true },
  {
    ...resource.fields[0]!,
    key: 'stock',
    label: 'Stock',
    type: 'number',
    minimum: 0,
    required: true,
  },
]

function descriptions(control: HTMLElement) {
  return control
    .getAttribute('aria-describedby')
    ?.split(' ')
    .map((id) => document.getElementById(id)?.textContent)
}

it.each(['create', 'update'] as const)(
  'keeps the %s draft and submits only after every bound is satisfied',
  async (mode) => {
    const overlong = '🙂'.repeat(201)
    const { root, put } = render(mode, true, listHref, constrainedFields, {
      name: overlong,
      stock: -1,
    })
    const post = vi.spyOn(router, 'post').mockImplementation(() => {})
    const request = mode === 'create' ? post : put
    const name = input(root, 'name')
    const stock = input(root, 'stock')
    expect(root.querySelector('form')!.noValidate).toBe(true)
    expect(name.hasAttribute('maxlength')).toBe(false)
    expect(stock.min).toBe('0')
    expect(descriptions(name)).toEqual(['До 200 символов', '201 из 200 символов'])
    expect(descriptions(stock)).toEqual(['Минимум — 0', 'Введите значение не меньше 0'])

    applyLocale('en')
    await nextTick()
    expect(descriptions(name)).toEqual(['Up to 200 characters', '201 of 200 characters'])
    expect(descriptions(stock)).toEqual(['Minimum: 0', 'Enter a value of at least 0'])
    expect(name.getAttribute('aria-invalid')).toBe('true')
    submit(root)
    if (mode === 'update') submit(root, true)
    expect(request).not.toHaveBeenCalled()
    expect(name.value).toBe(overlong)
    expect(stock.value).toBe('-1')

    input(root, 'name', '🙂'.repeat(200))
    await nextTick()
    expect(descriptions(name)).toEqual(['Up to 200 characters'])
    submit(root)
    expect(request).not.toHaveBeenCalled()
    input(root, 'stock', '')
    await nextTick()
    submit(root)
    expect(request).not.toHaveBeenCalled()
    input(root, 'stock', '0')
    await nextTick()
    submit(root, mode === 'update')
    expect(request.mock.lastCall?.[1]).toMatchObject({ name: '🙂'.repeat(200), stock: 0 })
  },
)

it('keeps an incomplete number draft dirty until the browser reports a complete value', async () => {
  const { root, put } = render('update', true, listHref, constrainedFields, {
    name: 'A',
    stock: 18,
  })
  const stock = input(root, 'stock')
  stock.value = ''
  // jsdom has no keyboard number editor. Model the native DOM for a lone minus sign.
  const validity = vi
    .spyOn(stock, 'validity', 'get')
    .mockReturnValue({ badInput: true } as ValidityState)
  stock.dispatchEvent(new Event('input', { bubbles: true }))
  await nextTick()
  expect(stock.value).toBe('')
  expect(mocks.form!.values.stock).toBe(18)
  expect(descriptions(stock)).toEqual(['Минимум — 0', 'Введите число'])
  expect(window.dispatchEvent(new Event('beforeunload', { cancelable: true }))).toBe(false)
  submit(root)
  expect(put).not.toHaveBeenCalled()
  validity.mockRestore()
  input(root, 'stock', '1.5')
  await nextTick()
  submit(root)
  expect(put.mock.lastCall?.[1]).toMatchObject({ stock: 1.5 })
})

it.each([
  [0, '-1e-999', '-0e-999'],
  [0.3, '0.29999999999999999', '3e-1'],
  [-0.3, '-30000000000000001e-17', '-.30'],
  [10, '9.99999999999999999', '10.00'],
] as const)(
  'retains a decimal literal just below %s even if Number rounds it to the minimum',
  async (minimum, rejected, accepted) => {
    const field = { ...constrainedFields[1]!, minimum }
    const { root, put } = render('update', true, listHref, [field], { stock: minimum })
    expect(Number(rejected) === minimum).toBe(true)
    const stock = input(root, 'stock', rejected)
    await nextTick()
    expect(stock.value).toBe(rejected)
    expect(descriptions(stock)).toContain(`Введите значение не меньше ${minimum}`)
    submit(root, true)
    expect(put).not.toHaveBeenCalled()
    input(root, 'stock', accepted)
    await nextTick()
    submit(root)
    expect(put).toHaveBeenCalledOnce()
    expect(put.mock.lastCall?.[1]).toMatchObject({ stock: Number(accepted) })
  },
)

it.each([constrainedFields[1]!, { ...constrainedFields[1]!, minimum: undefined }])(
  'keeps an inexact numeric draft for correction with minimum $minimum',
  async (field) => {
    const { root, put } = render('update', true, listHref, [field], { stock: 5 })
    for (const raw of ['9007199254740990.5', '1e-999', '0.1234567890123456789']) {
      const stock = input(root, 'stock', raw)
      await nextTick()
      expect(stock.value).toBe(raw)
      expect(descriptions(stock)).toContain('Это число нельзя передать без потери точности')
      submit(root)
      expect(put).not.toHaveBeenCalled()
    }
    for (const raw of ['.5', '1.500', '1e+2', '-0e-999', '1.25e-8']) {
      const stock = input(root, 'stock', raw)
      await nextTick()
      expect(stock.getAttribute('aria-invalid')).toBe('false')
      expect(mocks.form!.values.stock).toBe(Number(raw))
    }
    submit(root)
    expect(put.mock.lastCall?.[1]).toMatchObject({ stock: 1.25e-8 })
  },
)

it('rejects overflow and preserves signed values that round-trip through numeric JSON', () => {
  for (const raw of ['1e309', '-1e309', '-1e-999', '-9007199254740990.5'])
    expect(exactWireNumber(raw)).toBe(false)
  for (const raw of ['-0.50', '-5e+2', '0e+999', '1e308', '-1e-323'])
    expect(exactWireNumber(raw)).toBe(true)
})

it('supports constrained textareas and host fields named after native form members', async () => {
  const fields = [
    { ...resource.fields[0]!, key: 'elements', type: 'textarea' as const, maxLength: 2 },
    { ...resource.fields[0]!, key: 'reportValidity' },
  ]
  const { root, put } = render('update', false, listHref, fields, {
    elements: '🌱🌱🌱',
    reportValidity: 'kept',
  })
  const form = root.querySelector('form')!
  const textarea = root.querySelector('textarea')!
  // jsdom does not expose named controls as own properties like a browser.
  Object.defineProperty(form, 'reportValidity', { value: input(root, 'reportValidity') })
  Object.defineProperty(form, 'elements', { value: textarea })
  submit(root)
  expect(put).not.toHaveBeenCalled()
  expect(document.activeElement).toBe(textarea)
  expect(descriptions(textarea)).toEqual(['До 2 символов', '3 из 2 символов'])
  textarea.value = 'e\u0301'
  textarea.dispatchEvent(new Event('input', { bubbles: true }))
  await nextTick()
  submit(root)
  expect(put.mock.lastCall?.[1]).toMatchObject({ elements: 'e\u0301', reportValidity: 'kept' })
})

it('localizes server bound errors and clears only the corrected field', async () => {
  const { root } = render('update', true, listHref, constrainedFields, { name: 'Name', stock: 1 })
  mocks.form!.setError({ name: 'Text is too long', stock: 'Value is below the minimum' })
  await nextTick()
  expect(descriptions(input(root, 'name'))).toContain('Слишком длинный текст')
  expect(descriptions(input(root, 'stock'))).toContain('Значение меньше допустимого минимума')
  input(root, 'stock', '0')
  await nextTick()
  expect(descriptions(input(root, 'stock'))).toEqual(['Минимум — 0'])
  expect(descriptions(input(root, 'name'))).toContain('Слишком длинный текст')
})

it('lets an operator correct a selected legacy value after a conflict', async () => {
  const { root, put, conflict } = render('update', true, listHref, constrainedFields, {
    name: 'A',
    stock: 3,
  })
  input(root, 'stock', '2')
  submit(root)
  await conflict('r2', { name: 'A', stock: -1 })
  choice(root, 'stock', 'latest').click()
  await nextTick()
  expect(input(root, 'stock').value).toBe('-1')
  submit(root)
  expect(put).toHaveBeenCalledOnce()
  input(root, 'stock', '0')
  await nextTick()
  submit(root, true)
  expect(put.mock.lastCall?.[1]).toMatchObject({ stock: 0, _revision: 'r2' })
})

it('keeps the draft when a host projection omits a readable field during a conflict', async () => {
  const { root, put, get, conflict } = render()
  input(root, 'notes', 'My notes')
  submit(root, true)
  await conflict('r2', { name: 'New name' })
  expect(input(root, 'notes').value).toBe('My notes')
  expect(root.querySelector('[role="alert"]')?.textContent).toContain(
    'Не удалось сравнить все поля',
  )
  expect(button(root, 'Загрузить актуальную запись')).toBeDefined()
  submit(root, true)
  expect(put).toHaveBeenCalledOnce()
  expect(get).not.toHaveBeenCalled()
})

it('requires an explicit value for a readable field omitted by a host, including after reload', async () => {
  const field = { ...resource.fields[0]!, key: 'stock', type: 'number' as const }
  const { root, put } = render('update', false, listHref, [...resource.fields, field])
  expect(input(root, 'stock').value).toBe('')
  input(root, 'name', 'Changed')
  submit(root, true)
  expect(put).not.toHaveBeenCalled()
  input(root, 'stock', '0')
  await nextTick()
  submit(root, true)
  expect(put.mock.lastCall?.[1]).toMatchObject({ name: 'Changed', stock: 0 })
})

it('distinguishes null from false, zero and empty text in localized conflict choices', async () => {
  applyLocale('en')
  const fields = [
    { ...resource.fields[0]!, key: 'count', type: 'number' as const },
    { ...resource.fields[0]!, key: 'enabled', type: 'boolean' as const },
    { ...resource.fields[0]!, key: 'note' },
    {
      ...resource.fields[0]!,
      key: 'action',
      type: 'select' as const,
      options: [
        { value: 'a', label: 'Создать' },
        { value: 'b', label: 'Удалить' },
        { value: 'c', label: 'Отмена' },
      ],
    },
  ]
  const { root, put, conflict } = render('update', false, listHref, fields, {
    count: 5,
    enabled: null,
    note: 'Original',
    action: 'a',
  })
  input(root, 'count', '0')
  input(root, 'enabled').click()
  input(root, 'note', '')
  const select = root.querySelector<HTMLSelectElement>('select[name="action"]')!
  select.value = 'b'
  select.dispatchEvent(new Event('change', { bubbles: true }))
  submit(root)
  await conflict('r2', { count: null, enabled: false, note: null, action: 'c' })
  expect(choice(root, 'count', 'latest').labels?.[0]?.textContent).toContain('Not specified')
  for (const [key, side, label] of [
    ['count', 'mine', '0'],
    ['enabled', 'latest', 'No'],
    ['note', 'mine', 'Empty string'],
    ['action', 'mine', 'Delete'],
  ] as const) {
    const radio = choice(root, key, side)
    expect(radio.labels?.[0]?.textContent?.trim()).toMatch(new RegExp(': ' + label + '$'))
    radio.click()
  }
  await nextTick()
  submit(root)
  expect(put.mock.lastCall?.[1]).toEqual({
    count: 0,
    enabled: false,
    note: '',
    action: 'b',
    _revision: 'r2',
  })
})

it('automatically merges disjoint and identical changes and waits for the next manual save', async () => {
  const { root, put, get, conflict, options } = render()
  input(root, 'name', 'My name')
  submit(root, true)
  const firstAttempt = options().headers?.[attemptHeader]
  await conflict('r2', { name: 'Original name', notes: 'Latest notes' })
  expect(input(root, 'name').value).toBe('My name')
  expect(input(root, 'notes').value).toBe('Latest notes')
  expect(choice(root, 'name', 'mine')).toBeUndefined()
  expect(get).not.toHaveBeenCalled()
  expect(put).toHaveBeenCalledOnce()
  submit(root)
  expect(put.mock.lastCall?.[1]).toEqual({
    name: 'My name',
    notes: 'Latest notes',
    _revision: 'r2',
  })
  expect(options().headers?.[attemptHeader]).not.toBe(firstAttempt)
  await conflict('r3', { name: 'My name', notes: 'New notes' })
  expect(input(root, 'name').value).toBe('My name')
  expect(input(root, 'notes').value).toBe('New notes')
  expect(choice(root, 'name', 'mine')).toBeUndefined()
})

it('shows both overlapping values, blocks both submits and rebases again on a repeated conflict', async () => {
  const { root, put, get, conflict } = render()
  input(root, 'name', 'My name')
  submit(root, true)
  await conflict('r2', { ...defaultValues, name: 'Latest name' })
  expect(choice(root, 'name', 'mine').labels?.[0]?.textContent).toContain('Моё значение: My name')
  expect(choice(root, 'name', 'latest').labels?.[0]?.textContent).toContain(
    'Актуальное значение: Latest name',
  )
  expect(
    [...root.querySelectorAll<HTMLButtonElement>('button[type="submit"]')].map(
      ({ disabled }) => disabled,
    ),
  ).toEqual([true, true])
  submit(root)
  submit(root, true)
  expect(put).toHaveBeenCalledOnce()
  choice(root, 'name', 'mine').click()
  await nextTick()
  submit(root)
  expect(put.mock.lastCall?.[1]).toMatchObject({ name: 'My name', _revision: 'r2' })
  await conflict('r3', { ...defaultValues, name: 'Another name' })
  expect(choice(root, 'name', 'mine').checked).toBe(false)
  choice(root, 'name', 'latest').click()
  await nextTick()
  submit(root)
  expect(put.mock.lastCall?.[1]).toMatchObject({ name: 'Another name', _revision: 'r3' })
  expect(get).not.toHaveBeenCalled()
})

it('keeps write-only conflict values private and includes only explicitly reapplied values', async () => {
  const { root, put, conflict } = render('update', true)
  input(root, 'token', 'optional secret')
  input(root, 'requiredToken', 'required secret')
  submit(root)
  await conflict('r2', { ...defaultValues, notes: 'Latest notes' })
  expect(root.innerHTML).not.toMatch(/optional secret|required secret/)
  expect(input(root, 'token')).toBeNull()
  expect(input(root, 'requiredToken')).toBeNull()
  expect(root.querySelector<HTMLButtonElement>('button[data-save-and-return]')?.disabled).toBe(true)
  const [optional, required] = [...root.querySelectorAll('fieldset')].filter((node) =>
    node.querySelector(':scope > legend'),
  )
  button(optional!, 'Пропустить поле').click()
  expect(button(required!, 'Пропустить поле')).toBeUndefined()
  expect(button(required!, 'Ввести новое значение')).toBeDefined()
  button(required!, 'Повторно применить моё значение').click()
  await nextTick()
  expect(root.innerHTML).not.toMatch(/optional secret|required secret/)
  submit(root)
  expect(put.mock.lastCall?.[1]).toEqual({
    ...defaultValues,
    notes: 'Latest notes',
    requiredToken: 'required secret',
    _revision: 'r2',
  })
  await conflict('r3', defaultValues)
  expect(button(root, 'Повторно применить моё значение')).toBeDefined()
  expect(root.innerHTML).not.toContain('required secret')
})

it.each(['validation', 'unknown', 'wrong attempt', 'cancel', 'interrupt'] as const)(
  'retains the draft without returning or adopting an unconfirmed revision on %s',
  async (outcome) => {
    const { root, put, get, options, conflict, success } = render()
    input(root, 'name', 'My draft')
    submit(root, true)
    switch (outcome) {
      case 'validation':
        options().onError?.({ name: 'Required' })
        options().onFinish?.(visit)
        break
      case 'unknown':
        await success(
          'r2',
          { ...defaultValues, name: 'Unconfirmed' },
          { flash: { success: 'Saved' } },
        )
        break
      case 'wrong attempt':
        await conflict('r2', { ...defaultValues, name: 'Unrelated' }, 'another-attempt')
        break
      case 'cancel':
        options().onFinish?.({ ...visit, cancelled: true })
        break
      case 'interrupt':
        options().onFinish?.({ ...visit, interrupted: true })
        break
    }
    await nextTick()
    await nextTick()
    expect(input(root, 'name').value).toBe('My draft')
    expect(choice(root, 'name', 'mine')).toBeUndefined()
    expect(get).not.toHaveBeenCalled()
    submit(root)
    if (outcome === 'unknown' || outcome === 'wrong attempt') {
      expect(put).toHaveBeenCalledOnce()
      expect(root.querySelector('[role="alert"]')?.textContent).toContain(
        'Результат операции неизвестен',
      )
    } else {
      expect(put.mock.lastCall?.[1]).toMatchObject({ _revision: 'r1' })
    }
  },
)

it('retains a newer draft after successful save and uses the confirmed revision on retry', async () => {
  const { root, get, put, success } = render()
  input(root, 'name', 'Submitted')
  submit(root, true)
  mocks.form!.values.name = 'Newer draft'
  await success('r2', { ...defaultValues, name: 'Canonical submitted' })
  expect(get).not.toHaveBeenCalled()
  expect(input(root, 'name').value).toBe('Newer draft')
  submit(root)
  expect(put.mock.lastCall?.[1]).toMatchObject({ name: 'Newer draft', _revision: 'r2' })
})

it.each(['actor', 'record', 'resource'] as const)(
  'clears conflict and ignores late callbacks when the %s changes in a preserved edit page',
  async (changed) => {
    const { root, data, get, options, conflict } = render()
    input(root, 'name', 'Draft')
    input(root, 'token', 'secret')
    submit(root, true)
    await conflict('r2', { ...defaultValues, name: 'Latest name' })
    choice(root, 'name', 'mine').click()
    button(root, 'Повторно применить моё значение').click()
    await nextTick()
    submit(root, true)
    const pending = options()
    const lateResponse = inertiaPage({
      ...page.props,
      record: { id: 'a', revision: 'late', data: {} },
      flash: { success: 'Saved', recordEditAttempt: pending.headers?.[attemptHeader] },
    })
    data.values = { ...defaultValues, name: 'Different page' }
    if (changed === 'actor')
      page.props = { ...page.props, authuser: { idExact: '9007199254740994' } }
    if (changed === 'record') data.record = { id: 'b', revision: 'r7', data: {} }
    if (changed === 'resource')
      data.resource = { ...resource, key: 'widgets', basePath: '/widgets' }
    await nextTick()
    await pending.onSuccess?.(lateResponse)
    pending.onFinish?.(visit)
    await nextTick()
    expect(input(root, 'name').value).toBe('Different page')
    expect(input(root, 'token').value).toBe('')
    expect(button(root, 'Повторно применить моё значение')).toBeUndefined()
    expect(get).not.toHaveBeenCalled()
  },
)
