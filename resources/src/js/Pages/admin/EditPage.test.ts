import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, defineComponent, reactive } from 'vue'
import EditPage from './EditPage.vue'
import CreatePage from './CreatePage.vue'
import { setAdminCapabilities } from '@/composables/useAdminCapabilities'

vi.mock('@inertiajs/vue3', () => ({
  usePage: () => ({ props: {} }),
  useForm: (data: object) =>
    reactive({ ...data, errors: {}, put: vi.fn(), post: vi.fn(), processing: false }),
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

describe('admin email capability', () => {
  it.each([false, true])('allows existing email editing only with authmail=%s', (enabled) => {
    setAdminCapabilities({ props: { adminCapabilities: enabled ? { authmail: true } : {} } })
    const root = document.createElement('div')
    const editProps = { data: { id: 1, name: 'Admin', email: 'admin@example.test' } }
    const app = createApp(EditPage, editProps)
    apps.push(app)
    app.mount(root)
    const email = root.querySelector<HTMLInputElement>('input[name="email"]')
    expect(email?.readOnly).toBe(!enabled)
    expect(email?.value).toBe('admin@example.test')
    expect(root.querySelector<HTMLInputElement>('input[name="name"]')?.readOnly).toBe(false)
  })

  it('allows the initial email when creating an administrator without authmail', () => {
    setAdminCapabilities({ props: { adminCapabilities: {} } })
    const root = document.createElement('div')
    const app = createApp(CreatePage)
    apps.push(app)
    app.mount(root)
    const email = root.querySelector<HTMLInputElement>('input[name="email"]')
    expect(email?.readOnly).toBe(false)
    expect(email?.required).toBe(true)
  })
})
