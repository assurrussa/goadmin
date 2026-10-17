import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, defineComponent, h, nextTick, reactive } from 'vue'
import RolePreviewBanner from './RolePreviewBanner.vue'

const mocks = vi.hoisted(() => ({
  post: vi.fn(),
  visit: vi.fn(),
  page: { props: {} as Record<string, unknown>, url: '/roles' },
}))
vi.mock('axios', () => ({ default: { post: mocks.post } }))
vi.mock('@inertiajs/vue3', () => ({ usePage: () => mocks.page, router: { visit: mocks.visit } }))
vi.mock('@/components/ui/dialog', () => {
  const part = defineComponent({
    setup:
      (_, { slots }) =>
      () =>
        h('div', slots.default?.()),
  })
  return {
    Dialog: defineComponent({
      props: ['open'],
      setup:
        (props, { slots }) =>
        () =>
          props.open ? h('div', { role: 'dialog' }, slots.default?.()) : null,
    }),
    DialogContent: part,
    DialogHeader: part,
    DialogTitle: part,
    DialogDescription: part,
    DialogFooter: part,
    DialogClose: part,
  }
})
vi.mock('@/components/ui/AppButton.vue', () => ({
  default: defineComponent({
    setup:
      (_, { attrs, slots }) =>
      () =>
        h('button', attrs, slots.default?.()),
  }),
}))
const apps: ReturnType<typeof createApp>[] = []
afterEach(() => {
  apps.splice(0).forEach((app) => app.unmount())
  mocks.post.mockReset()
  mocks.visit.mockReset()
  mocks.page.props = {}
  mocks.page.url = '/roles'
})
function mount(preview: unknown, open = false) {
  mocks.page.props = { rolePreview: preview }
  const root = document.createElement('div')
  const state = reactive({ open })
  const app = createApp({
    render: () =>
      h(RolePreviewBanner, {
        open: state.open,
        'onUpdate:open': (value: boolean) => {
          state.open = value
        },
      }),
  })
  apps.push(app)
  app.mount(root)
  return { root, state }
}
describe('compact role preview', () => {
  it('does not occupy page space or expose a form until the profile menu opens it', async () => {
    const { root, state } = mount({
      active: false,
      available: true,
      roles: [{ id: 7, name: 'Editor' }],
    })
    expect(root.querySelector('form')).toBeNull()
    expect(root.querySelector('aside')).toBeNull()
    expect(root.textContent).toBe('')
    state.open = true
    await nextTick()
    expect(root.querySelector('[role="dialog"]')).not.toBeNull()
    expect(root.querySelector('select')?.textContent).toContain('Editor')
    expect(mocks.post).not.toHaveBeenCalled()
  })
  it('does not render a control for an unavailable role preview', () => {
    const { root } = mount({ active: false, available: false, roles: [] }, true)
    expect(root.querySelector('button')).toBeNull()
    expect(root.querySelector('form')).toBeNull()
  })
  it('preserves the read-only indicator and explicit server exit operation', async () => {
    mocks.post.mockResolvedValue({})
    const { root, state } = mount({ active: true, available: false, roleName: 'Editor', roles: [] })
    expect(root.textContent).toContain('Editor')
    expect(root.querySelector('[role="dialog"]')).toBeNull()
    root.querySelector('button')!.click()
    await nextTick()
    expect(state.open).toBe(true)
    const exit = [...root.querySelectorAll('button')].find((button) =>
      button.textContent?.includes('Выйти из предпросмотра'),
    )!
    exit.click()
    await Promise.resolve()
    await nextTick()
    expect(mocks.post).toHaveBeenCalledWith('/auth/role-preview', { roleId: 0 })
    expect(mocks.visit).toHaveBeenCalledWith('/auth/profile', { replace: true })
  })
})
