import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, defineComponent, h, nextTick, reactive, type App } from 'vue'
import UsersPage from './IndexPage.vue'
import AdminPage from '../admin/IndexPage.vue'
import RolesPage from '../roles/IndexPage.vue'
import PermissionsPage from '../permissions/IndexPage.vue'
import type { ApiResponse } from '@/composables/useDataGrid'
const router = vi.hoisted(() => ({ visit: vi.fn(), get: vi.fn(), delete: vi.fn(), post: vi.fn() }))
vi.mock('@inertiajs/vue3', () => ({ router }))
vi.mock('@/components/layout/AppHead.vue', () => ({
  default: defineComponent({ render: () => null }),
}))
vi.mock('@/composables/useNotifications', () => ({
  useNotifications: () => ({ warning: vi.fn(), error: vi.fn() }),
}))
let app: App | undefined
const settle = async () => {
  for (let i = 0; i < 12; i++) await nextTick()
}
const data = (routePath: string, name: string): ApiResponse => ({
  data: [{ item: { id: 1, name }, actions: [{ key: 'delete', label: 'Delete row' }] }],
  config: {
    routePath,
    columns: [{ key: 'name', title: 'Name', label: 'Name' }],
    behaviour: { refreshable: true, exportable: true },
  },
  meta: { title: 'Test', description: '', requestQuery: window.location.search, filters: {} },
})
afterEach(() => {
  app?.unmount()
  app = undefined
  document.body.innerHTML = ''
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
  vi.clearAllMocks()
  window.history.replaceState({}, '', '/')
})
describe('redirected page props own refresh/mutation results', () => {
  it.each([
    [UsersPage, '/users'],
    [AdminPage, '/admins'],
    [RolesPage, '/roles'],
    [PermissionsPage, '/permissions'],
  ] as const)('hydrates %s without a follow-up grid request', async (component, routePath) => {
    window.history.replaceState({}, '', routePath)
    const fetch = vi.fn()
    vi.stubGlobal('fetch', fetch)
    const props = reactive({ data: data(routePath, 'Before refresh') })
    app = createApp(defineComponent({ setup: () => () => h(component, props) }))
    const root = document.createElement('div')
    document.body.append(root)
    app.mount(root)
    await settle()
    // The transport boundary supplies new props as Inertia does after POST -> redirect -> GET.
    router.visit.mockImplementation(() => {
      props.data = data(routePath, 'Authoritative refreshed page')
    })
    const refresh = [...root.querySelectorAll<HTMLButtonElement>('button')].find(
      (b) => b.textContent?.trim() === 'Обновить',
    )!
    refresh.click()
    await settle()
    expect(router.visit).toHaveBeenCalledOnce()
    expect(root.textContent).toContain('Authoritative refreshed page')
    expect(fetch).not.toHaveBeenCalled()
    // Permissions has sync rather than a delete action; its refresh above covers redirected props.
    if (routePath === '/permissions') return
    vi.stubGlobal('confirm', () => true)
    router.delete.mockImplementation(() => {
      props.data = data(routePath, 'Authoritative mutation page')
    })
    root.querySelector<HTMLButtonElement>('[aria-label="Delete row"]')!.click()
    await settle()
    expect(router.delete).toHaveBeenCalledOnce()
    expect(root.textContent).toContain('Authoritative mutation page')
    expect(fetch).not.toHaveBeenCalled()
  })
})
