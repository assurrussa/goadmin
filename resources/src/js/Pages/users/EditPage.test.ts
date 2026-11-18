import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, defineComponent, nextTick, reactive, h, type Component } from 'vue'
import { router } from '@inertiajs/vue3'
import type { ActiveVisit, Page, VisitOptions } from '@inertiajs/core'
import EditPage from './EditPage.vue'
import AdminEditPage from '@/Pages/admin/EditPage.vue'
import { setAdminCapabilities } from '@/composables/useAdminCapabilities'

const page = reactive({
  component: 'users/EditPage',
  url: '/users/1/edit',
  props: {} as Page['props'],
})
vi.mock('@inertiajs/vue3', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@inertiajs/vue3')>()),
  usePage: () => page,
  Link: defineComponent({
    props: ['href'],
    setup:
      (props, { slots }) =>
      () =>
        h('a', { href: props.href }, slots.default?.()),
  }),
}))
vi.mock('@/components/layout/AppHead.vue', () => ({
  default: defineComponent({ render: () => null }),
}))
vi.mock('@/components/layout/PageActionBar.vue', () => ({
  default: defineComponent({ render: () => null }),
}))

const apps: ReturnType<typeof createApp>[] = []
afterEach(() => {
  apps.splice(0).forEach((app) => app.unmount())
  setAdminCapabilities({ props: {} })
  page.component = 'users/EditPage'
  page.url = '/users/1/edit'
  page.props = { errors: {} }
  vi.restoreAllMocks()
})

const record = { id: 1, name: 'Original', email: 'canonical@example.test' }
const owner = { idExact: '9007199254740993' }
const visit = { cancelled: false, interrupted: false } as ActiveVisit
const put = () =>
  vi.spyOn(router, 'put').mockImplementation((_url, _data, options) => {
    options?.onBefore?.(visit)
    options?.onStart?.(visit)
  })
const response = (options: VisitOptions, props?: Partial<Page['props']>): Page => ({
  ...page,
  props: {
    data: record,
    authuser: owner,
    errors: {},
    flash: { success: 'Saved', recordEditAttempt: options.headers?.['X-GoAdmin-Record-Attempt'] },
    ...props,
  },
  version: null,
  clearHistory: false,
  encryptHistory: false,
  flash: {},
  rememberedState: {},
})

function renderEdit(component: Component = EditPage) {
  const root = document.createElement('div')
  const data = reactive({ ...record })
  const app = createApp({ render: () => h(component, { data: { ...data } }) })
  apps.push(app)
  app.mount(root)
  return { root, data }
}

function submit(root: HTMLElement, saveAndReturn = false) {
  root.querySelector('form')!.dispatchEvent(
    new SubmitEvent('submit', {
      bubbles: true,
      cancelable: true,
      submitter: saveAndReturn
        ? root.querySelector<HTMLButtonElement>('button[data-save-and-return]')
        : null,
    }),
  )
}

function input(root: HTMLElement, name: string, value: string) {
  const field = root.querySelector<HTMLInputElement>(`input[name="${name}"]`)!
  field.value = value
  field.dispatchEvent(new Event('input', { bubbles: true }))
}

function latestOptions(mock: ReturnType<typeof put>): VisitOptions {
  return mock.mock.lastCall?.[2] ?? {}
}

describe('users email capability', () => {
  it('ignores rejected old email when mail is disabled but preserves other edits', () => {
    setAdminCapabilities({ props: { adminCapabilities: {} } })
    page.props = { errors: {}, old: { email: 'rejected@example.test', name: 'Changed' } }
    const root = document.createElement('div')
    const pageProps = { data: { id: 1, name: 'Original', email: 'canonical@example.test' } }
    const app = createApp(EditPage, pageProps)
    apps.push(app)
    app.mount(root)
    expect(root.querySelector<HTMLInputElement>('input[name="email"]')?.value).toBe(
      'canonical@example.test',
    )
    expect(root.querySelector<HTMLInputElement>('input[name="name"]')?.value).toBe('Changed')
  })
  it('recovers a preserved form when authmail becomes disabled', async () => {
    setAdminCapabilities({ props: { adminCapabilities: { authmail: true } } })
    page.props = { errors: {}, old: { email: 'pending@example.test', name: 'Changed' } }
    const root = document.createElement('div')
    const pageProps = { data: { id: 1, name: 'Original', email: 'canonical@example.test' } }
    const app = createApp(EditPage, pageProps)
    apps.push(app)
    app.mount(root)
    expect(root.querySelector<HTMLInputElement>('input[name="email"]')?.value).toBe(
      'pending@example.test',
    )
    setAdminCapabilities({ props: { adminCapabilities: {} } })
    await nextTick()
    expect(root.querySelector<HTMLInputElement>('input[name="email"]')?.value).toBe(
      'canonical@example.test',
    )
    expect(root.querySelector<HTMLInputElement>('input[name="name"]')?.value).toBe('Changed')
  })
  it('restores canonical email after success before a later profile-only save', async () => {
    setAdminCapabilities({ props: { adminCapabilities: { authmail: true } } })
    page.props = {
      errors: {},
      old: { email: 'pending@example.test', name: 'Changed' },
      authuser: owner,
    }
    const routerPut = put()
    const routerGet = vi.spyOn(router, 'get').mockImplementation(() => {})
    const { root } = renderEdit()
    submit(root)
    expect(routerPut).toHaveBeenCalledOnce()
    const options = latestOptions(routerPut)
    await options.onSuccess?.(response(options))
    options.onFinish?.(visit)
    await nextTick()
    expect(root.querySelector<HTMLInputElement>('input[name="email"]')?.value).toBe(
      'canonical@example.test',
    )
    expect(root.querySelector<HTMLInputElement>('input[name="name"]')?.value).toBe('Changed')
    expect(routerGet).not.toHaveBeenCalled()
    submit(root)
    expect(routerPut.mock.lastCall?.[1]).toMatchObject({ email: 'canonical@example.test' })
  })
  it.each([false, true])('allows email editing only with authmail=%s', (enabled) => {
    setAdminCapabilities({ props: { adminCapabilities: enabled ? { authmail: true } : {} } })
    const root = document.createElement('div')
    const pageProps = { data: { id: 1, name: 'User', email: 'user@example.test' } }
    const app = createApp(EditPage, pageProps)
    apps.push(app)
    app.mount(root)
    const email = root.querySelector<HTMLInputElement>('input[name="email"]')
    expect(email?.readOnly).toBe(!enabled)
    expect(email?.value).toBe('user@example.test')
    expect(root.querySelector<HTMLInputElement>('input[name="name"]')?.readOnly).toBe(false)
  })
})

describe('save and return', () => {
  it.each([
    ['users', 'users', EditPage],
    ['admins', 'admin', AdminEditPage],
  ])(
    'saves %s once and visits the captured list after clearing the dirty guard',
    async (resource, componentPath, component) => {
      const routerPut = put()
      const routerGet = vi.spyOn(router, 'get').mockImplementation(() => {})
      const before = vi.spyOn(router, 'on')
      const listHref = `/${resource}?page=2&limit=1&sortBy=name&sortOrder=asc&search=smoke&status=active`
      page.component = `${componentPath}/EditPage`
      page.url = `/${resource}/1/edit?${new URLSearchParams({ returnTo: listHref })}`
      page.props = { errors: {}, authuser: owner }
      const { root } = renderEdit(component)
      input(root, 'name', 'Saved name')
      await nextTick()
      submit(root, true)
      submit(root, true)
      expect(routerPut).toHaveBeenCalledExactlyOnceWith(
        `/${resource}/1`,
        expect.objectContaining({ name: 'Saved name' }),
        expect.any(Object),
      )
      expect(routerGet).not.toHaveBeenCalled()
      const options = latestOptions(routerPut)
      // The response can strip returnTo. Navigation uses the validated pre-submit target.
      page.url = `/${resource}/1/edit`
      await options.onSuccess?.(response(options))
      expect(routerGet).not.toHaveBeenCalled()
      const guard = before.mock.calls.find(([event]) => event === 'before')?.[1]
      expect(guard?.({ detail: { visit: { method: 'get' } } } as never)).toBeUndefined()
      options.onFinish?.(visit)
      await nextTick()
      expect(routerGet).toHaveBeenCalledExactlyOnceWith(listHref)
    },
  )

  it.each(['validation', 'unknown', 'uncertain response', 'cancelled', 'interrupted'] as const)(
    'keeps the edits on %s',
    async (outcome) => {
      const routerPut = put()
      const routerGet = vi.spyOn(router, 'get').mockImplementation(() => {})
      const before = vi.spyOn(router, 'on')
      const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
      page.props = { errors: {}, authuser: owner }
      const { root } = renderEdit()
      input(root, 'name', 'Unsaved')
      submit(root, true)
      const options = latestOptions(routerPut)
      if (outcome === 'validation') options.onError?.({ name: 'Required' })
      if (outcome === 'unknown') options.onError?.({ form: 'Result unknown' })
      if (outcome === 'uncertain response')
        await options.onSuccess?.(response(options, { flash: { warning: 'Unexpected response' } }))
      if (outcome === 'cancelled' || outcome === 'interrupted') options.onCancel?.()
      options.onFinish?.({
        ...visit,
        cancelled: outcome === 'cancelled',
        interrupted: outcome === 'interrupted',
      })
      await nextTick()
      const guard = before.mock.calls.find(([event]) => event === 'before')?.[1]
      expect(guard?.({ detail: { visit: { method: 'get' } } } as never)).toBe(false)
      expect(confirm).toHaveBeenCalledOnce()
      expect(root.querySelector<HTMLInputElement>('input[name="name"]')?.value).toBe('Unsaved')
      expect(routerGet).not.toHaveBeenCalled()
    },
  )

  it('retains the draft when a different save on the same record reports success', async () => {
    const routerPut = put()
    const routerGet = vi.spyOn(router, 'get').mockImplementation(() => {})
    page.props = { errors: {}, authuser: owner }
    const { root } = renderEdit()
    input(root, 'name', 'First draft')
    submit(root, true)
    const first = latestOptions(routerPut)
    first.onError?.({ name: 'Invalid' })
    first.onFinish?.(visit)
    await nextTick()

    input(root, 'name', 'Current draft')
    submit(root, true)
    const second = latestOptions(routerPut)
    const attemptHeader = 'X-GoAdmin-Record-Attempt'
    expect(first.headers?.[attemptHeader]).toEqual(expect.any(String))
    expect(second.headers?.[attemptHeader]).not.toBe(first.headers?.[attemptHeader])
    await second.onSuccess?.(response(first))
    second.onFinish?.(visit)
    await nextTick()
    expect(routerGet).not.toHaveBeenCalled()
    expect(root.querySelector<HTMLInputElement>('input[name="name"]')?.value).toBe('Current draft')
    expect(root.querySelector('[role="alert"]')?.textContent).toContain(
      'Результат операции неизвестен',
    )
  })

  it.each(['new draft', 'draft edited at finish', 'new record', 'new owner'] as const)(
    'ignores the return intent after a %s',
    async (change) => {
      const routerPut = put()
      const routerGet = vi.spyOn(router, 'get').mockImplementation(() => {})
      const before = vi.spyOn(router, 'on')
      const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
      page.props = { errors: {}, authuser: owner }
      const { root, data } = renderEdit()
      input(root, 'name', 'Submitted draft')
      submit(root, true)
      const options = latestOptions(routerPut)
      if (change === 'new record') data.id = 2
      if (change === 'new owner')
        page.props = { errors: {}, authuser: { idExact: '9007199254740995' } }
      await nextTick()
      if (change !== 'draft edited at finish') input(root, 'name', 'New draft')
      await options.onSuccess?.(response(options))
      if (change === 'draft edited at finish') input(root, 'name', 'New draft')
      options.onFinish?.(visit)
      await nextTick()
      const guard = before.mock.calls.find(([event]) => event === 'before')?.[1]
      expect(guard?.({ detail: { visit: { method: 'get' } } } as never)).toBe(false)
      expect(confirm).toHaveBeenCalledOnce()
      expect(root.querySelector<HTMLInputElement>('input[name="name"]')?.value).toBe('New draft')
      expect(routerGet).not.toHaveBeenCalled()
    },
  )
})
