import { afterEach, expect, it } from 'vitest'
import { createApp, defineComponent, h, nextTick } from 'vue'
import { createInertiaApp } from '@inertiajs/vue3'
import { createPinia } from 'pinia'
import { useNotificationsStore } from '@/stores/notifications'
import { setAdminCapabilities } from '@/composables/useAdminCapabilities'
import AdminRuntimeFrame from './AdminRuntimeFrame.vue'

let app: ReturnType<typeof createApp> | undefined
afterEach(() => {
  app?.unmount()
  app = undefined
  document.body.innerHTML = ''
  setAdminCapabilities(null)
})

it('starts its outer frame before real Inertia initializes page props and still delivers initial flash once', async () => {
  const root = document.createElement('div')
  root.id = 'presentation-startup'
  document.body.append(root)
  const pinia = createPinia()
  const errors: unknown[] = []
  setAdminCapabilities({ props: { adminCapabilities: {} } })
  await createInertiaApp({
    id: root.id,
    progress: false,
    page: {
      component: 'startup',
      props: { errors: {}, flash: { success: 'Initial flash' } },
      url: '/',
      version: null,
      clearHistory: false,
      encryptHistory: false,
      flash: {},
      rememberedState: {},
    },
    resolve: () => defineComponent({ render: () => h('main', 'Page mounted') }),
    setup({ el, App, props, plugin }) {
      app = createApp({
        render: () => h(AdminRuntimeFrame, null, { default: () => h(App, props) }),
      })
      app.use(plugin).use(pinia)
      app.config.errorHandler = (error) => errors.push(error)
      app.mount(el)
    },
  })
  await nextTick()
  await nextTick()
  expect(errors).toEqual([])
  expect(root.querySelector('main')?.textContent).toBe('Page mounted')
  expect(useNotificationsStore(pinia).notifications.map((item) => item.message)).toEqual([
    'Initial flash',
  ])
})
