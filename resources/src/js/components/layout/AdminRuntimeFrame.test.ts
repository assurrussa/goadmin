import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, reactive, ref } from 'vue'
import { createPinia } from 'pinia'
import AdminRuntimeFrame from './AdminRuntimeFrame.vue'

const mocks = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  info: vi.fn(),
  warning: vi.fn(),
  stop: vi.fn(),
  on: vi.fn(),
  page: { props: {} as Record<string, unknown> },
}))
vi.mock('@inertiajs/vue3', () => ({ usePage: () => mocks.page, router: { on: mocks.on } }))
vi.mock('@/composables/useNotifications', () => ({ useNotifications: () => mocks }))
vi.mock('@/components/NotificationsContainer.vue', () => ({
  default: { render: () => h('div', { 'data-notifications': '' }) },
}))
vi.mock('@/components/UploadQueueContainer.vue', () => ({
  default: { render: () => h('div', { 'data-uploads': '' }) },
}))
vi.mock('@/composables/useAdminCapabilities', () => ({
  useAdminCapabilities: () => ({ uploads: ref(true), realtime: ref(false) }),
}))
let app: ReturnType<typeof createApp> | undefined
beforeEach(() => {
  vi.useFakeTimers()
  vi.clearAllMocks()
  mocks.page.props = reactive({ flash: { success: 'Saved' } })
  mocks.on.mockReturnValue(mocks.stop)
})
afterEach(() => {
  app?.unmount()
  app = undefined
  vi.useRealTimers()
})

it('keeps one service owner through auth/admin page changes and cleans subscriptions', async () => {
  const pageName = ref('auth')
  app = createApp({
    render: () => h(AdminRuntimeFrame, null, { default: () => h('main', pageName.value) }),
  })
  const root = document.createElement('div')
  app.use(createPinia()).mount(root)
  await nextTick()
  await nextTick()
  expect(mocks.success).toHaveBeenCalledExactlyOnceWith('Saved')
  expect(mocks.on).toHaveBeenCalledTimes(1)
  const finish = mocks.on.mock.calls[0]![1] as () => void
  finish()
  pageName.value = 'admin'
  await nextTick()
  await nextTick()
  expect(root.querySelector('main')?.textContent).toBe('admin')
  expect(root.querySelectorAll('[data-notifications]')).toHaveLength(1)
  expect(root.querySelectorAll('[data-uploads]')).toHaveLength(1)
  expect(mocks.success).toHaveBeenCalledTimes(1)
  mocks.page.props.flash = { error: 'Failed' }
  await nextTick()
  await nextTick()
  expect(mocks.error).toHaveBeenCalledExactlyOnceWith('Failed')
  app.unmount()
  app = undefined
  expect(mocks.stop).toHaveBeenCalledTimes(1)
  mocks.page.props.flash = { success: 'Too late' }
  finish()
  await nextTick()
  await nextTick()
  expect(mocks.success).toHaveBeenCalledTimes(1)
  expect(vi.getTimerCount()).toBe(0)
})

it('ignores pending flash work after its frame unmounts', async () => {
  app = createApp(AdminRuntimeFrame)
  app.use(createPinia()).mount(document.createElement('div'))
  app.unmount()
  app = undefined
  await nextTick()
  expect(mocks.success).not.toHaveBeenCalled()
  expect(mocks.stop).toHaveBeenCalledTimes(1)
})
