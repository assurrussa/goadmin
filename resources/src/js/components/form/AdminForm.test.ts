import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, reactive } from 'vue'
import AdminForm from './AdminForm.vue'

const mocks = vi.hoisted(() => ({
  on: vi.fn(),
  remove: vi.fn(),
}))

vi.mock('@inertiajs/vue3', () => ({
  usePage: () => ({ props: { errors: {} } }),
  router: { on: mocks.on },
}))

const apps: ReturnType<typeof createApp>[] = []
afterEach(() => {
  apps.splice(0).forEach((app) => app.unmount())
  document.body.innerHTML = ''
  vi.unstubAllGlobals()
  mocks.on.mockReset()
  mocks.remove.mockReset()
})

describe('AdminForm unsaved changes guard', () => {
  it('confirms only dirty GET navigation and removes the subscription on unmount', () => {
    mocks.on.mockReturnValue(mocks.remove)
    const confirm = vi.fn(() => false)
    vi.stubGlobal('confirm', confirm)
    const form = reactive({ errors: {}, processing: false, isDirty: true })
    const app = createApp(AdminForm, { form, warnUnsaved: true })
    app.mount(document.createElement('div'))

    expect(mocks.on).toHaveBeenCalledWith('before', expect.any(Function))
    const guard = mocks.on.mock.calls[0][1] as (event: {
      detail: { visit: { method: string } }
    }) => boolean | void
    expect(guard({ detail: { visit: { method: 'get' } } })).toBe(false)
    expect(confirm).toHaveBeenCalledTimes(1)
    expect(guard({ detail: { visit: { method: 'post' } } })).toBeUndefined()
    form.processing = true
    expect(guard({ detail: { visit: { method: 'get' } } })).toBeUndefined()
    expect(confirm).toHaveBeenCalledTimes(1)

    app.unmount()
    expect(mocks.remove).toHaveBeenCalledOnce()
  })
})

function mountFields(
  form: { errors: Record<string, string>; processing: boolean },
  focusOnError = true,
) {
  const root = document.createElement('div')
  document.body.append(root)
  const app = createApp({
    render: () =>
      h(
        AdminForm,
        { form, focusOnError },
        {
          default: () =>
            h('div', { name: 'name' }, [h('input', { name: 'name', 'aria-label': 'Name' })]),
        },
      ),
  })
  apps.push(app)
  app.mount(root)
  return { app, root, input: root.querySelector<HTMLInputElement>('input[name="name"]')! }
}

it('focuses the real named input after processing unlocks its fieldset', async () => {
  const form = reactive({ errors: {} as Record<string, string>, processing: true })
  const { root, input } = mountFields(form)
  form.errors = { name: 'Required' }
  await nextTick()
  await nextTick()
  expect(root.querySelector('fieldset')?.disabled).toBe(true)
  expect(document.activeElement).not.toBe(input)
  form.processing = false
  await nextTick()
  await nextTick()
  expect(document.activeElement).toBe(input)
})

it('scopes error focus to its own form when another form has the same field name', async () => {
  const first = mountFields(reactive({ errors: {}, processing: false }))
  const form = reactive({ errors: {} as Record<string, string>, processing: false })
  const second = mountFields(form)
  first.input.focus()
  form.errors = { name: 'Required' }
  await nextTick()
  await nextTick()
  expect(document.activeElement).toBe(second.input)
})

it('honors focusOnError=false when errors arrive before processing ends', async () => {
  const form = reactive({ errors: {} as Record<string, string>, processing: true })
  const { input } = mountFields(form, false)
  form.errors = { name: 'Required' }
  await nextTick()
  form.processing = false
  await nextTick()
  await nextTick()
  expect(document.activeElement).not.toBe(input)
})

it('does not focus without errors when processing ends', async () => {
  const form = reactive({ errors: {}, processing: true })
  const { input } = mountFields(form)
  form.processing = false
  await nextTick()
  await nextTick()
  expect(document.activeElement).not.toBe(input)
})

it('does not focus a different matching field after the form unmounts', async () => {
  const form = reactive({ errors: {} as Record<string, string>, processing: false })
  const { app } = mountFields(form)
  form.errors = { name: 'Required' }
  await nextTick()
  app.unmount()
  apps.splice(apps.indexOf(app), 1)
  const outside = document.createElement('input')
  outside.name = 'name'
  document.body.append(outside)
  await nextTick()
  expect(document.activeElement).not.toBe(outside)
})
