import { afterEach, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, reactive } from 'vue'
import { createPinia } from 'pinia'
import { useNotificationsStore } from '@/stores/notifications'
import { useFlashMessages } from './useFlashMessages'

const mocks = vi.hoisted(() => ({
  page: {} as object,
  finish: undefined as (() => void) | undefined,
}))
vi.mock('@inertiajs/vue3', () => ({
  usePage: () => mocks.page,
  router: {
    on: (_event: string, callback: () => void) => {
      mocks.finish = callback
      return vi.fn()
    },
  },
}))
let app: ReturnType<typeof createApp> | undefined
afterEach(() => {
  app?.unmount()
  document.body.innerHTML = ''
  mocks.finish = undefined
})

it('shows one toast per response and permits an identical message from a new response', async () => {
  const page = reactive({ props: { flash: { error: 'Record changed; reload before saving' } } })
  mocks.page = page
  const pinia = createPinia()
  app = createApp({
    setup() {
      useFlashMessages()
      return () => h('div')
    },
  })
  app.use(pinia).mount(document.createElement('div'))
  const store = useNotificationsStore(pinia)
  await nextTick()
  await nextTick()
  mocks.finish?.()
  await nextTick()
  await nextTick()
  expect(store.notifications).toHaveLength(1)
  store.clearAll()
  page.props = { flash: { error: 'Record changed; reload before saving' } }
  await nextTick()
  await nextTick()
  mocks.finish?.()
  await nextTick()
  await nextTick()
  expect(store.notifications).toHaveLength(1)
})
