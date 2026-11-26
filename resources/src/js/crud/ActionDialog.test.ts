import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, reactive } from 'vue'
import { applyLocale } from '@/i18n'
import { appearance } from '@/appearance/state'
import { router } from '@inertiajs/vue3'
import {
  adminFetch,
  AdminFetchNotSentError,
  sessionCoordinator,
  SessionReadinessError,
} from '@/http'
import ActionDialog from './ActionDialog.vue'
import type { CrudAction, CrudResource, CrudRecord } from './index'

let page: { props: { authuser: { idExact: string } }; url: string; version: string }
vi.mock('@inertiajs/vue3', () => ({
  usePage: () => page,
  router: { get: vi.fn(), on: () => () => {} },
}))
vi.mock('@/http', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/http')>()),
  adminFetch: vi.fn(),
}))
const fetch = vi.mocked(adminFetch)
const action: CrudAction = {
  key: 'unpublish',
  label: 'Снять с публикации',
  confirmation: 'Снять этот товар с публикации?',
  description: 'This action changes the publication status.',
  params: [
    {
      key: 'reason',
      label: 'Причина',
      description: 'Explain the decision for the activity log.',
      type: 'textarea',
      required: true,
      maxLength: 500,
      trimSpace: true,
    },
  ],
}
const resource: CrudResource = {
  key: 'products',
  title: 'Products',
  basePath: '/products',
  actions: [action],
  fields: [
    { key: 'name', label: 'Name', type: 'text', read: true, create: false, update: false },
    { key: 'active', label: 'Active', type: 'boolean', read: true, create: false, update: false },
  ],
}
const record: CrudRecord = {
  id: '9',
  revision: '12',
  data: { name: 'First product' },
  actions: ['unpublish'],
}
const list = '/products?search=mat&sortBy=id&sortOrder=desc&page=2'
const back = '/products/records/9?' + new URLSearchParams({ returnTo: list })
let app: ReturnType<typeof createApp>
let view: InstanceType<typeof ActionDialog>
let dialogResource: CrudResource
const settle = async () => {
  for (let i = 0; i < 8; i++) await nextTick()
}
const dialog = () => document.querySelector<HTMLElement>('[data-crud-action-dialog]')
const input = () => dialog()!.querySelector<HTMLTextAreaElement>('[name="reason"]')!
const accept = () =>
  dialog()!.querySelector<HTMLButtonElement>('[data-confirmation-accept]')!.click()
const write = async (value: string) => {
  input().value = value
  input().dispatchEvent(new Event('input', { bubbles: true }))
  await nextTick()
}
beforeEach(async () => {
  vi.stubGlobal(
    'ResizeObserver',
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  )
  page = reactive({ props: { authuser: { idExact: '17' } }, url: list, version: 'asset-1' })
  applyLocale('en')
  appearance.owner = '17'
  appearance.readOnly = false
  appearance.resolving = false
  fetch.mockReset()
  vi.mocked(router.get).mockClear()
  dialogResource = reactive({ ...resource })
  const root = document.createElement('div')
  document.body.append(root)
  app = createApp({
    render: () =>
      h(ActionDialog, {
        resource: dialogResource,
        ref: (value: unknown) => {
          view = value as typeof view
        },
      }),
  })
  app.mount(root)
  view.open(action, record, list)
  await settle()
})
afterEach(async () => {
  app.unmount()
  await nextTick()
  document.body.replaceChildren()
  applyLocale('ru')
  vi.unstubAllGlobals()
})
it('keeps the raw Unicode draft, rejects invalid input locally and sends one normalized command', async () => {
  expect(dialog()!.textContent).toContain('First product')
  expect(dialog()!.textContent).toContain(action.description)
  const ids = input().getAttribute('aria-describedby')?.split(' ') ?? []
  expect(ids.map((id) => document.getElementById(id)?.textContent)).toContain(
    action.params![0]!.description,
  )
  expect(dialog()!.textContent).toContain('Up to 500 characters')
  for (const value of ['\u0085 \n', 'x\0x', '\uD800', '🙂'.repeat(501)]) {
    await write(value)
    accept()
    await settle()
    expect(input().value).toBe(value)
    expect(input().getAttribute('aria-invalid')).toBe('true')
  }
  expect(fetch).not.toHaveBeenCalled()
  const normalized = '🙂'.repeat(495) + ' x\n\uFEFF\uFFFD'
  const raw = '\u0085' + normalized + '  '
  await write(raw)
  let finish!: (response: Response) => void
  fetch.mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        finish = resolve
      }),
  )
  accept()
  accept()
  expect(fetch).toHaveBeenCalledTimes(1)
  const [url, options] = fetch.mock.calls[0]!
  expect(url).toBe('/products/records/9/actions/unpublish')
  expect(JSON.parse(String(options?.body))).toEqual({
    _revision: '12',
    params: { reason: normalized },
  })
  finish(Response.json({ errors: { reason: ['Unexpected parameter'] } }, { status: 422 }))
  await vi.waitFor(() => expect(dialog()!.textContent).toContain('Unexpected parameter'))
  expect(input().value).toBe(raw)
  fetch.mockResolvedValueOnce(new Response(null, { status: 204 }))
  accept()
  await vi.waitFor(() => expect(router.get).toHaveBeenCalledWith(back))
})
function reread(actions: string[], actor = '17') {
  return Response.json({
    component: 'crud/Show',
    props: {
      authuser: { idExact: actor },
      resource,
      record: {
        ...record,
        revision: '13',
        actions,
        data: { name: 'Updated product', active: false },
      },
    },
  })
}
it('returns focus to the rejected reason and its linked error after a server validation failure', async () => {
  await write('  rejected draft  ')
  const confirm = dialog()!.querySelector<HTMLButtonElement>('[data-confirmation-accept]')!
  confirm.focus()
  fetch.mockResolvedValueOnce(
    Response.json({ errors: { reason: ['Reason rejected'] } }, { status: 422 }),
  )
  confirm.click()
  await vi.waitFor(() => expect(input().getAttribute('aria-invalid')).toBe('true'))
  expect(document.activeElement).toBe(input())
  expect(input().value).toBe('  rejected draft  ')
  const linked = input()
    .getAttribute('aria-describedby')!
    .split(' ')
    .map((id) => document.getElementById(id)?.textContent)
  expect(linked).toContain('Reason rejected')
})
it('reads the new record after a conflict and waits for a second explicit confirmation', async () => {
  fetch
    .mockResolvedValueOnce(new Response(null, { status: 409 }))
    .mockResolvedValueOnce(reread(['unpublish']))
  await write(' same draft ')
  accept()
  await vi.waitFor(() => expect(dialog()!.textContent).toContain('Updated product'))
  const displayed = () => [...dialog()!.querySelectorAll('dd')].map((el) => el.textContent)
  expect(displayed()).toEqual(['Updated product', 'No'])
  applyLocale('ru')
  await nextTick()
  expect(displayed()).toEqual(['Updated product', 'Нет'])
  expect(fetch).toHaveBeenCalledTimes(2)
  expect(fetch.mock.calls[1]![0]).toBe(back)
  expect(fetch.mock.calls[1]![1]?.headers).toMatchObject({ 'X-Inertia-Version': page.version })
  expect(input().value).toBe(' same draft ')
  fetch.mockResolvedValueOnce(new Response(null, { status: 204 }))
  accept()
  await vi.waitFor(() => expect(router.get).toHaveBeenCalledWith(back))
  expect(JSON.parse(String(fetch.mock.calls[2]![1]?.body))).toEqual({
    _revision: '13',
    params: { reason: 'same draft' },
  })
})
it('opens the latest card when a full CRUD command is no longer available', async () => {
  fetch.mockResolvedValueOnce(new Response(null, { status: 409 })).mockResolvedValueOnce(reread([]))
  await write('Note')
  accept()
  await vi.waitFor(() => expect(router.get).toHaveBeenCalledWith(back))
  expect(dialog()).toBeNull()
  expect(fetch).toHaveBeenCalledTimes(2)
})
it('explains the conflict when the action is no longer available', async () => {
  dialogResource.serverValidateActionParams = true
  fetch.mockResolvedValueOnce(new Response(null, { status: 409 })).mockResolvedValueOnce(reread([]))
  await write('Note')
  accept()
  await vi.waitFor(() =>
    expect(dialog()!.textContent).toContain('This action is no longer available'),
  )
  expect(dialog()!.querySelector('[data-confirmation-accept]')).toBeNull()
  expect(router.get).not.toHaveBeenCalled()
  const open = [...dialog()!.querySelectorAll('button')].find(
    (button) => button.textContent === 'Open record',
  )!
  open.click()
  await vi.waitFor(() => expect(router.get).toHaveBeenCalledWith(back))
  expect(dialog()).toBeNull()
  expect(fetch).toHaveBeenCalledTimes(2)
})
it.each([409, 422])('lets the server resolve an empty reason with status %s', async (status) => {
  dialogResource.serverValidateActionParams = true
  fetch.mockResolvedValueOnce(
    Response.json({ errors: { reason: ['This field is required'] } }, { status }),
  )
  if (status === 409) fetch.mockResolvedValueOnce(reread([]))
  await nextTick()
  accept()
  await vi.waitFor(() =>
    expect(dialog()!.textContent).toContain(
      status === 409 ? 'This action is no longer available' : 'This field is required',
    ),
  )
  expect(JSON.parse(String(fetch.mock.calls[0]![1]?.body))).toEqual({
    _revision: '12',
    params: { reason: '' },
  })
  expect(fetch).toHaveBeenCalledTimes(status === 409 ? 2 : 1)
  expect(router.get).not.toHaveBeenCalled()
})
it('clears the draft on permission loss and isolates a late response from the next owner', async () => {
  fetch.mockResolvedValueOnce(new Response(null, { status: 403 }))
  await write('private reason')
  accept()
  await vi.waitFor(() => expect(dialog()).toBeNull())
  expect(document.body.textContent).toContain('Record access denied')
  view.open(action, record, list)
  await settle()
  expect(input().value).toBe('')
  let finish!: (response: Response) => void
  fetch.mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        finish = resolve
      }),
  )
  await write('old account')
  accept()
  page.props.authuser.idExact = '18'
  await settle()
  expect(dialog()).toBeNull()
  expect(fetch.mock.lastCall?.[1]?.signal?.aborted).toBe(true)
  finish(new Response(null, { status: 204 }))
  await settle()
  expect(router.get).not.toHaveBeenCalled()
})
it('retains an uncertain result without sending another command', async () => {
  fetch.mockRejectedValueOnce(new TypeError('connection lost'))
  await write('Reason')
  accept()
  await vi.waitFor(() => expect(dialog()!.textContent).toContain('The outcome is unknown'))
  expect(dialog()!.querySelector('[data-confirmation-accept]')).toBeNull()
  expect(fetch).toHaveBeenCalledTimes(1)
})
it('distinguishes an input key inherited by plain JS objects', async () => {
  dialog()!.querySelector<HTMLButtonElement>('[data-confirmation-cancel]')!.click()
  await settle()
  view.open({ ...action, params: [{ ...action.params![0]!, key: 'toString' }] }, record, list)
  await settle()
  const control = dialog()!.querySelector<HTMLTextAreaElement>('[name="toString"]')!
  expect(control.value).toBe('')
  control.value = ' value '
  control.dispatchEvent(new Event('input', { bubbles: true }))
  fetch.mockResolvedValueOnce(new Response(null, { status: 204 }))
  accept()
  await vi.waitFor(() => expect(router.get).toHaveBeenCalledWith(back))
  expect(JSON.parse(String(fetch.mock.calls[0]![1]?.body)).params).toEqual({ toString: 'value' })
})
it('retries only the authorized read when refreshing a conflict fails', async () => {
  fetch
    .mockResolvedValueOnce(new Response(null, { status: 409 }))
    .mockRejectedValueOnce(new Error('read lost'))
  await write('Raw draft  ')
  accept()
  await vi.waitFor(() =>
    expect(dialog()!.textContent).toContain('Its current values could not be loaded'),
  )
  expect(input().value).toBe('Raw draft  ')
  fetch.mockResolvedValueOnce(reread(['unpublish']))
  const retry = [...dialog()!.querySelectorAll('button')].find(
    (button) => button.textContent === 'Check again',
  )!
  retry.click()
  await vi.waitFor(() => expect(dialog()!.textContent).toContain('Updated product'))
  expect(fetch.mock.calls.map(([, opts]) => opts?.method || 'GET')).toEqual(['POST', 'GET', 'GET'])
  expect(input().value).toBe('Raw draft  ')
})
it('allows a manual retry after readiness proves the POST was never sent', async () => {
  fetch.mockRejectedValueOnce(
    new AdminFetchNotSentError(new SessionReadinessError('authentication_unavailable')),
  )
  await write('Raw reason')
  accept()
  await vi.waitFor(() => expect(dialog()!.querySelector('p[role="alert"]')).toBeTruthy())
  expect(input().value).toBe('Raw reason')
  fetch.mockResolvedValueOnce(new Response(null, { status: 204 }))
  accept()
  await vi.waitFor(() => expect(router.get).toHaveBeenCalledWith(back))
})
it('clears the previous owner’s reason when the re-read identifies a different actor', async () => {
  fetch
    .mockResolvedValueOnce(new Response(null, { status: 409 }))
    .mockResolvedValueOnce(reread(['unpublish'], '18'))
  await write('Private')
  accept()
  await vi.waitFor(() => expect(dialog()).toBeNull())
  expect(document.body.textContent).not.toContain('Private')
  expect(router.get).not.toHaveBeenCalled()
})

it('keeps the original draft for manual confirmation after a marked 419', async () => {
  const enabled = vi.spyOn(sessionCoordinator, 'isEnabled').mockReturnValue(true)
  try {
    fetch.mockResolvedValueOnce(
      Response.json(
        { status: 419, code: 'csrf_needs_sync' },
        { status: 419, headers: { 'x-goadmin-auth-error': '1' } },
      ),
    )
    await write('  original reason  ')
    accept()
    await vi.waitFor(() => expect(dialog()!.textContent).toContain('Confirm again'))
    expect(input().value).toBe('  original reason  ')
    expect(fetch).toHaveBeenCalledTimes(1)
    fetch.mockResolvedValueOnce(new Response(null, { status: 204 }))
    accept()
    await vi.waitFor(() => expect(router.get).toHaveBeenCalledWith(back))
    expect(fetch).toHaveBeenCalledTimes(2)
  } finally {
    enabled.mockRestore()
  }
})
it.each(['POST', 'GET'])(
  'bounds a stalled %s and releases the dialog without replay',
  async (method) => {
    const timeout = new AbortController()
    const deadline = vi.spyOn(AbortSignal, 'timeout').mockReturnValue(timeout.signal)
    try {
      if (method === 'GET') fetch.mockResolvedValueOnce(new Response(null, { status: 409 }))
      fetch.mockImplementationOnce(
        (_url, init) =>
          new Promise((_resolve, reject) => {
            init!.signal!.addEventListener('abort', () => reject(init!.signal!.reason))
          }),
      )
      await write('Reason')
      accept()
      await vi.waitFor(() => expect(fetch).toHaveBeenCalledTimes(method === 'POST' ? 1 : 2))
      expect(deadline).toHaveBeenCalledWith(15_000)
      timeout.abort()
      await vi.waitFor(() =>
        expect(dialog()!.textContent).toContain(
          method === 'POST' ? 'The outcome is unknown' : 'Its current values could not be loaded',
        ),
      )
      expect(
        dialog()!.querySelector<HTMLButtonElement>('[data-confirmation-cancel]')?.disabled,
      ).toBe(false)
      expect(fetch.mock.calls.filter(([, init]) => init?.method === 'POST')).toHaveLength(1)
    } finally {
      deadline.mockRestore()
    }
  },
)
