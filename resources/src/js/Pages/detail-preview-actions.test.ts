import { afterEach, expect, it, vi } from 'vitest'
import { createApp, defineComponent, h, nextTick, reactive } from 'vue'
import AdminView from './admin/ViewPage.vue'
import UserView from './users/ViewPage.vue'
const fixture = vi.hoisted(() => ({ page: null as unknown as { props: Record<string, unknown> } }))
vi.mock('@inertiajs/vue3', () => ({
  usePage: () => fixture.page,
  Link: defineComponent({
    props: ['href'],
    setup:
      (props, { slots }) =>
      () =>
        h('a', { href: props.href }, slots.default?.()),
  }),
}))
vi.mock('@/components/layout/AppHead.vue', () => ({
  default: defineComponent({ render: () => null }),
}))
vi.mock('@/components/layout/PageActionBar.vue', () => ({
  default: defineComponent({ render: () => null }),
}))
let app: ReturnType<typeof createApp> | undefined
afterEach(() => {
  app?.unmount()
})
it.each([
  { component: AdminView, domain: 'admins' },
  { component: UserView, domain: 'users' },
])(
  'hides both $domain edit links for a viewer and restores only server-allowed actions',
  async ({ component, domain }) => {
    fixture.page = reactive({ props: {} as Record<string, unknown> })
    const root = document.createElement('div')
    const props = { data: { id: 7, name: 'Synthetic', deletedAt: { Valid: false } } }
    app = createApp(component, props)
    app.mount(root)
    const links = () => root.querySelectorAll(`a[href="/${domain}/7/edit"]`)
    expect(links()).toHaveLength(2)
    fixture.page.props.rolePreview = { active: true, actions: { [`${domain}:update`]: false } }
    await nextTick()
    expect(links()).toHaveLength(0)
    fixture.page.props.rolePreview = { active: true }
    await nextTick()
    expect(links()).toHaveLength(0)
    fixture.page.props.rolePreview = { active: true, actions: { [`${domain}:update`]: true } }
    await nextTick()
    expect(links()).toHaveLength(2)
    fixture.page.props.rolePreview = { active: false }
    await nextTick()
    expect(links()).toHaveLength(2)
  },
)
