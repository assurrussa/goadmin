import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, defineComponent, nextTick, reactive } from 'vue'
import EditPage from './EditPage.vue'
import { setAdminCapabilities } from '@/composables/useAdminCapabilities'

const page = vi.hoisted(() => ({ props: {} as Record<string, unknown> }))
vi.mock('@inertiajs/vue3', () => ({
  usePage: () => page,
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
  page.props = {}
})

describe('users email capability', () => {
  it('ignores rejected old email when mail is disabled but preserves other edits', () => {
    setAdminCapabilities({ props: { adminCapabilities: {} } })
    page.props = { old: { email: 'rejected@example.test', name: 'Changed' } }
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
    page.props = { old: { email: 'pending@example.test', name: 'Changed' } }
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
