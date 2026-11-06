const confirmation = vi.hoisted(() => vi.fn<(...args: unknown[]) => Promise<boolean>>())
vi.mock('@/composables/useConfirmation', () => ({
  useConfirmation: () => async (options: unknown, onConfirm: () => void | Promise<void>) => {
    const accepted = await confirmation(options)
    if (accepted) await onConfirm()
    return accepted
  },
}))
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, defineComponent, h, nextTick, reactive, type App } from 'vue'
import UsersPage from './IndexPage.vue'
import AdminPage from '../admin/IndexPage.vue'
import RolesPage from '../roles/IndexPage.vue'
import PermissionsPage from '../permissions/IndexPage.vue'
import type { ApiResponse } from '@/composables/useDataGrid'
const router = vi.hoisted(() => ({ visit: vi.fn(), get: vi.fn(), delete: vi.fn(), post: vi.fn() }))
vi.mock('@inertiajs/vue3', () => ({
  router,
  usePage: () => ({ url: window.location.pathname + window.location.search }),
}))
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
vi.mock('@/components/datagrid/DataGrid.vue', () => ({
  default: defineComponent({
    props: ['initialData'],
    emits: ['action-row'],
    setup(props, { emit }) {
      return () =>
        h(
          'button',
          {
            'aria-label': 'Run action',
            onClick: () =>
              emit(
                'action-row',
                props.initialData.data[0].actions[0].key,
                props.initialData.data[0].item,
              ),
          },
          'Run action',
        )
    },
  }),
}))
describe('asynchronous row confirmation', () => {
  it.each([
    [UsersPage, '/users', 'delete', 'Удалить', 'danger'],
    [UsersPage, '/users', 'restore', 'Восстановить', 'primary'],
    [AdminPage, '/admins', 'delete', 'Удалить', 'danger'],
    [RolesPage, '/roles', 'delete', 'Удалить', 'danger'],
    [PermissionsPage, '/permissions', 'sync', 'Синхронизировать', 'primary'],
  ] as const)(
    '%s %s %s waits before requesting and snapshots its target',
    async (component, routePath, action, label, variant) => {
      let resolve!: (accepted: boolean) => void
      confirmation.mockReturnValue(
        new Promise<boolean>((done) => {
          resolve = done
        }),
      )
      const initial = data(routePath, 'Target')
      initial.data[0]!.actions = [{ key: action, label: 'Run action' }]
      const props = reactive({ data: initial })
      app = createApp(defineComponent({ setup: () => () => h(component, props) }))
      const root = document.createElement('div')
      app.mount(root)
      await settle()
      root.querySelector<HTMLButtonElement>('[aria-label="Run action"]')!.click()
      expect(confirmation).toHaveBeenCalledWith(
        expect.objectContaining({ confirmLabel: label, variant }),
      )
      expect(router.delete).not.toHaveBeenCalled()
      expect(router.post).not.toHaveBeenCalled()
      expect(router.visit).not.toHaveBeenCalled()
      props.data.data[0]!.item.id = 99
      props.data.config!.routePath = '/changed'
      resolve(true)
      await settle()
      if (action === 'sync')
        expect(router.visit).toHaveBeenCalledWith(`${routePath}/sync`, { method: 'post' })
      else if (action === 'restore')
        expect(router.post).toHaveBeenCalledWith(`${routePath}/1/restore`, {})
      else expect(router.delete).toHaveBeenCalledWith(`${routePath}/1`)
    },
  )
  it.each([UsersPage, AdminPage, RolesPage, PermissionsPage])(
    'cancellation leaves %s unchanged',
    async (component) => {
      confirmation.mockResolvedValue(false)
      const initial = data('/records', 'Target')
      initial.data[0]!.actions = [
        { key: component === PermissionsPage ? 'sync' : 'delete', label: 'Run action' },
      ]
      app = createApp(component, { data: initial })
      const root = document.createElement('div')
      app.mount(root)
      await settle()
      root.querySelector<HTMLButtonElement>('[aria-label="Run action"]')!.click()
      await settle()
      expect(router.delete).not.toHaveBeenCalled()
      expect(router.post).not.toHaveBeenCalled()
      expect(router.visit).not.toHaveBeenCalled()
    },
  )
})
