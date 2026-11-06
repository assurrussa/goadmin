import { afterEach, expect, it, vi } from 'vitest'
import { createApp, defineComponent, h, nextTick } from 'vue'
import Form from './Form.vue'
import type { CrudResource } from '@/crud'

const mocks = vi.hoisted(() => ({
  page: { url: '/products', props: {} },
  form: undefined as
    | {
        setError: (errors: Record<string, string>) => void
        clearErrors: () => void
      }
    | undefined,
}))

vi.mock('@inertiajs/vue3', async (importOriginal) => {
  const original = await importOriginal<typeof import('@inertiajs/vue3')>()
  return {
    ...original,
    usePage: () => mocks.page,
    useForm: (...args: unknown[]) => {
      const form = Reflect.apply(original.useForm, undefined, args)
      mocks.form = form
      return form
    },
    router: { on: () => () => {} },
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
  app?.unmount()
  app = undefined
  mocks.form = undefined
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
