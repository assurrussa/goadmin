import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { createApp, defineComponent, h, nextTick, type App } from 'vue'
import Active from './ActivePage.vue'
import Failed from './FailedPage.vue'

const state = vi.hoisted(() => ({ delete: vi.fn(), post: vi.fn() }))
vi.mock('@inertiajs/vue3', () => ({
  usePage: () => ({ url: '/queues/jobs', props: {} }),
  router: { delete: state.delete, post: state.post },
  Link: defineComponent({
    setup:
      (_, { slots }) =>
      () =>
        h('a', slots.default?.()),
  }),
}))
vi.mock('@/components/layout/AppHead.vue', () => ({
  default: defineComponent({ render: () => null }),
}))
vi.mock('@/components/datagrid/DataGrid.vue', () => ({
  default: defineComponent({
    emits: ['bulk-complete', 'action-row', 'action-table'],
    setup(_, { emit, expose }) {
      expose({ refreshData: vi.fn() })
      return () =>
        h('div', [
          h(
            'button',
            { onClick: () => emit('bulk-complete', [{ id: 'job', status: 'succeeded' }]) },
            'Complete bulk',
          ),
          h('button', { onClick: () => emit('action-row', 'delete', { id: 'job' }) }, 'Delete one'),
          h('button', { onClick: () => emit('action-row', 'retry', { id: 'job' }) }, 'Retry one'),
        ])
    },
  }),
}))
const apps: App[] = []
const fetchMock = vi.fn<typeof fetch>()
const settle = async () => {
  for (let index = 0; index < 20; index++) await nextTick()
}
function mount(component: typeof Active | typeof Failed) {
  const root = document.createElement('div')
  const app = createApp(component, { jobsCount: 5, failedCount: 3, statsApiUrl: '/queues/stats' })
  app.mount(root)
  apps.push(app)
  return root
}
const click = (root: Element, label: string) =>
  Array.from(root.querySelectorAll('button'))
    .find((button) => button.textContent === label)!
    .click()
beforeEach(() => {
  fetchMock
    .mockReset()
    .mockResolvedValue({ ok: true, json: async () => ({ jobs: 42, failed: 17 }) } as Response)
  vi.stubGlobal('fetch', fetchMock)
  vi.stubGlobal('confirm', () => true)
  state.delete.mockReset()
  state.post.mockReset()
})
afterEach(() => {
  apps.splice(0).forEach((app) => app.unmount())
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})
for (const [name, component] of [
  ['active', Active],
  ['failed', Failed],
] as const) {
  it(`${name} reloads authoritative counts after a confirmed bulk response`, async () => {
    const root = mount(component)
    click(root, 'Complete bulk')
    await settle()
    expect(fetchMock).toHaveBeenCalledWith(
      '/queues/stats',
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    )
    expect(root.querySelector('[data-queue-count="jobs"]')?.textContent).toBe('42')
    expect(root.querySelector('[data-queue-count="failed"]')?.textContent).toBe('17')
  })
  it(`${name} refresh failure labels retained counts as stale`, async () => {
    fetchMock.mockResolvedValueOnce({ ok: false, status: 503 } as Response)
    const root = mount(component)
    click(root, 'Complete bulk')
    await settle()
    expect(root.querySelector('[role="alert"]')?.textContent).toContain(
      'Не удалось обновить счётчики',
    )
    expect(root.querySelector('[data-queue-count="jobs"]')?.textContent).toBe('5')
  })
  it(`${name} single-delete success refreshes server counts rather than subtracting locally`, async () => {
    const root = mount(component)
    click(root, 'Delete one')
    expect(state.delete).toHaveBeenCalledOnce()
    const callbacks = state.delete.mock.calls[0][1]
    expect(callbacks.onSuccess).toBeTypeOf('function')
    callbacks.onSuccess()
    await settle()
    expect(root.querySelector('[data-queue-count="jobs"]')?.textContent).toBe('42')
    expect(root.querySelector('[data-queue-count="failed"]')?.textContent).toBe('17')
  })
}
it('failed single-retry success also refreshes both server counts', async () => {
  const root = mount(Failed)
  click(root, 'Retry one')
  const callbacks = state.post.mock.calls[0][2]
  expect(callbacks.onSuccess).toBeTypeOf('function')
  callbacks.onSuccess()
  await settle()
  expect(root.querySelector('[data-queue-count="jobs"]')?.textContent).toBe('42')
  expect(root.querySelector('[data-queue-count="failed"]')?.textContent).toBe('17')
})
