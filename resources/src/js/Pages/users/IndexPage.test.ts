import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, defineComponent, h, nextTick } from 'vue'
import IndexPage from './IndexPage.vue'

const mocks = vi.hoisted(() => ({
  download: vi.fn(),
  warning: vi.fn(),
  error: vi.fn(),
  get: vi.fn(),
}))
vi.mock('@/services/userExport', () => ({ downloadUserExport: mocks.download }))
vi.mock('@/composables/useNotifications', () => ({
  useNotifications: () => ({ warning: mocks.warning, error: mocks.error }),
}))
vi.mock('@inertiajs/vue3', () => ({ router: { get: mocks.get } }))
vi.mock('@/components/layout/AppHead.vue', () => ({
  default: defineComponent({ render: () => null }),
}))
vi.mock('@/components/datagrid/DataGrid.vue', () => ({
  default: defineComponent({
    emits: ['action-table'],
    setup(_, { emit }) {
      return () => h('button', { onClick: () => emit('action-table', 'export', null) }, 'Export')
    },
  }),
}))

const apps: ReturnType<typeof createApp>[] = []
afterEach(() => {
  apps.splice(0).forEach((app) => app.unmount())
  vi.clearAllMocks()
})

function mount() {
  const root = document.createElement('div')
  const pageProps = { data: { data: [], config: { routePath: '/users' } } }
  const app = createApp(IndexPage, pageProps)
  apps.push(app)
  app.mount(root)
  return { app, button: root.querySelector('button')! }
}

describe('users export action', () => {
  it('uses one download for repeated clicks and displays truncation', async () => {
    let resolve!: (value: boolean) => void
    mocks.download.mockReturnValue(
      new Promise<boolean>((done) => {
        resolve = done
      }),
    )
    const { button } = mount()
    button.click()
    button.click()
    expect(mocks.download).toHaveBeenCalledOnce()
    expect(mocks.download.mock.calls[0].slice(0, 2)).toEqual(['/users', window.location.href])
    expect(mocks.get).not.toHaveBeenCalled()
    resolve(true)
    await nextTick()
    expect(mocks.warning).toHaveBeenCalledWith(expect.stringContaining('10 000'))
  })

  it('cancels on navigation and ignores its late result', async () => {
    let resolve!: (value: boolean) => void
    mocks.download.mockReturnValue(
      new Promise<boolean>((done) => {
        resolve = done
      }),
    )
    const { app, button } = mount()
    button.click()
    const signal = mocks.download.mock.calls[0][2] as AbortSignal
    app.unmount()
    apps.pop()
    expect(signal.aborted).toBe(true)
    resolve(true)
    await nextTick()
    expect(mocks.warning).not.toHaveBeenCalled()
    expect(mocks.error).not.toHaveBeenCalled()
  })
})
