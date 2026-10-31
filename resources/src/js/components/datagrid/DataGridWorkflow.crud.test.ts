import { afterEach, expect, it, vi } from 'vitest'
import { createApp, defineComponent, h, nextTick, type App } from 'vue'
import DataGridWorkflow from './DataGridWorkflow.vue'
import { useDataGrid } from '@/composables/useDataGrid'
import { useDataGridWorkflow } from '@/composables/useDataGridWorkflow'

let app: App | undefined
const settle = async () => {
  for (let index = 0; index < 20; index++) await nextTick()
}
afterEach(() => {
  app?.unmount()
  vi.unstubAllGlobals()
})
it('creates, edits, explicitly saves, defaults and deletes a named view through the actual editor', async () => {
  const fetchMock = vi.fn<typeof fetch>().mockImplementation(
    async (_url, init) =>
      ({
        ok: true,
        json: async () =>
          init?.method === 'PUT'
            ? { owner: 'owner', version: 2 }
            : {
                owner: 'owner',
                version: 1,
                payload: { schemaVersion: 1, columns: ['id'], views: [] },
              },
      }) as Response,
  )
  vi.stubGlobal('fetch', fetchMock)
  const root = document.createElement('div')
  let model!: ReturnType<typeof useDataGridWorkflow>
  let grid!: ReturnType<typeof useDataGrid>
  app = createApp(
    defineComponent({
      setup() {
        grid = useDataGrid({
          apiUrl: '/users',
          initialData: {
            data: [],
            config: {
              columns: [{ key: 'id', label: 'ID', title: 'ID' }],
              workflow: {
                key: 'users',
                preferencesUrl: '/users/preferences',
                exportUrl: '/users/export',
              },
            },
          },
        })
        model = useDataGridWorkflow(grid, false)
        return () => h(DataGridWorkflow, { model, selectedCount: 0, loading: false })
      },
    }),
  )
  app.mount(root)
  await settle()
  const input = root.querySelector('input')!
  const fill = async (value: string) => {
    input.value = value
    input.dispatchEvent(new Event('input', { bubbles: true }))
    await settle()
  }
  const click = async (label: string) => {
    const button = Array.from(root.querySelectorAll('button')).find(
      (button) => button.textContent?.trim() === label,
    )!
    expect(button.disabled).toBe(false)
    button.click()
    await settle()
  }
  await fill('Mine')
  await click('Сохранить как новое')
  const id = model.activeViewId.value
  expect(root.querySelector('[data-part="view-dirty"]')).toBeNull()
  expect(root.textContent).toContain('Настройки сохранены.')
  await fill('Renamed')
  expect(root.querySelector('[data-part="view-dirty"]')?.textContent).toContain(
    'Представление изменено',
  )
  expect(root.textContent).not.toContain('Настройки сохранены.')
  expect(model.preferences.value.views[0].name).toBe('Mine')
  await click('Сохранить / переименовать')
  expect(model.preferences.value.views[0].name).toBe('Renamed')
  expect(model.preferences.value.views[0].id).toBe(id)
  expect(root.querySelector('[data-part="view-dirty"]')).toBeNull()
  grid.searchQuery.value = 'unsaved conditions'
  await settle()
  expect(root.querySelector('[data-part="view-dirty"]')).not.toBeNull()
  await click('Сохранить / переименовать')
  expect(model.preferences.value.views[0].state.search).toBe('unsaved conditions')
  await click('По умолчанию')
  expect(model.preferences.value.defaultViewId).toBe(id)
  await click('Удалить представление')
  expect(model.preferences.value.views).toEqual([])
  expect(model.preferences.value.defaultViewId).toBeUndefined()
  expect(input.value).toBe('')
  expect(root.querySelector('[data-part="view-dirty"]')).toBeNull()
})
