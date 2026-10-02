import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, defineComponent, reactive } from 'vue'
import EditPage from './EditPage.vue'
import { setAdminCapabilities } from '@/composables/useAdminCapabilities'

vi.mock('@inertiajs/vue3', () => ({
  usePage: () => ({ props: {} }),
  useForm: (data: object) => reactive({ ...data, errors: {}, put: vi.fn(), processing: false }),
  router: { on: () => () => {} },
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
})

describe('users email capability', () => {
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
