import { afterEach, expect, it, vi } from 'vitest'
import { createApp, defineComponent, h } from 'vue'
import Dashboard from './IndexPage.vue'
import Active from './ActivePage.vue'
import Failed from './FailedPage.vue'

const state = vi.hoisted(() => ({ page: { url: '/queues', props: {} } }))
vi.mock('@inertiajs/vue3', () => ({
  usePage: () => state.page,
  router: {},
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
vi.mock('@/components/datagrid/DataGrid.vue', () => ({
  default: defineComponent({ render: () => null }),
}))

const cleanups: Array<() => void> = []
afterEach(() => cleanups.splice(0).forEach((cleanup) => cleanup()))

for (const [name, component] of [
  ['dashboard', Dashboard],
  ['active', Active],
  ['failed', Failed],
] as const) {
  it.each([
    ['/queues?view=overview', '/queues'],
    ['/queues/jobs?page=2', '/queues/jobs'],
    ['/queues/jobs-failed?limit=25#rows', '/queues/jobs-failed'],
  ])(`${name} marks exactly the current queue route for %s`, (url, expected) => {
    state.page.url = url
    const root = document.createElement('div')
    const app = createApp(component)
    app.mount(root)
    cleanups.push(() => app.unmount())
    const current = root.querySelectorAll('a[aria-current="page"]')
    expect(current).toHaveLength(1)
    expect(current[0].getAttribute('href')).toBe(expected)
  })
}
