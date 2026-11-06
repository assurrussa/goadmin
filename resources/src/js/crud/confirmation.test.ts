import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { createApp, defineComponent, h, nextTick, reactive } from 'vue'
import Index from '@/Pages/crud/Index.vue'
import Show from '@/Pages/crud/Show.vue'
import ConfirmationDialog from '@/components/confirmation/ConfirmationDialog.vue'
import { provideConfirmation } from '@/components/confirmation/confirmation'
import { appearance } from '@/appearance/state'
import { applyLocale, t } from '@/i18n'
import type { CrudResource } from './index'

const mocks = vi.hoisted(() => ({
  post: vi.fn(),
  remove: vi.fn(),
  page: undefined as unknown as { props: Record<string, unknown>; url: string },
}))
vi.mock('@inertiajs/vue3', () => ({
  usePage: () => mocks.page,
  router: { on: () => () => {}, get: vi.fn(), post: mocks.post, delete: mocks.remove },
  Head: defineComponent({ render: () => null }),
  Link: defineComponent({
    props: ['href'],
    setup:
      (props, { slots }) =>
      () =>
        h('a', { href: props.href }, slots.default?.()),
  }),
}))
vi.mock('@/components/datagrid/DataGrid.vue', () => ({
  default: defineComponent({
    props: ['initialData', 'apiUrl', 'navigationMode'],
    emits: ['action-row'],
    setup:
      (_props, { emit }) =>
      () =>
        h(
          'div',
          ['publish', 'delete'].map((action) =>
            h(
              'button',
              {
                onClick: () => emit('action-row', action, { id: 'record-1', _revision: 'rev-1' }),
              },
              t(action === 'publish' ? 'Опубликовать' : 'Удалить'),
            ),
          ),
        ),
  }),
}))

let app: ReturnType<typeof createApp> | undefined
const settle = async () => {
  for (let i = 0; i < 12; i++) await nextTick()
}
beforeEach(() => {
  vi.stubGlobal(
    'ResizeObserver',
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  )
  mocks.post.mockClear()
  mocks.remove.mockClear()
  mocks.page = reactive({ props: { errors: {} }, url: '/products' })
  appearance.owner = 'test-actor'
  appearance.readOnly = false
  appearance.resolving = false
})
afterEach(async () => {
  app?.unmount()
  app = undefined
  await settle()
  document.body.innerHTML = ''
  applyLocale('ru')
  vi.unstubAllGlobals()
})

const cases = (['list', 'detail'] as const).flatMap((view) =>
  (['ru', 'en'] as const).flatMap((locale) =>
    (['publish', 'delete'] as const).map((action) => ({ view, locale, action })),
  ),
)
it.each(cases)(
  'uses the correct fallback confirmation in $view $locale $action',
  async ({ view, locale, action }) => {
    applyLocale(locale)
    const resource: CrudResource = {
      key: 'products',
      basePath: '/products',
      title: 'Products',
      fields: [],
      actions: [{ key: 'publish', label: 'Опубликовать' }],
    }
    const root = document.createElement('div')
    document.body.append(root)
    app = createApp({
      setup() {
        const controller = provideConfirmation()
        return () => [
          view === 'list'
            ? h(Index, { resource, data: { data: [] } })
            : h(Show, {
                resource,
                record: {
                  id: 'record-1',
                  revision: 'rev-1',
                  data: {},
                  actions: ['publish', 'delete'],
                },
                permissions: { create: false, update: false, delete: true },
              }),
          h(ConfirmationDialog, { controller }),
        ]
      },
    })
    app.mount(root)
    const label = t(action === 'publish' ? 'Опубликовать' : 'Удалить')
    const trigger = [...root.querySelectorAll('button')].find(
      (button) => button.textContent?.trim() === label,
    )!
    trigger.click()
    await settle()
    let dialog = document.querySelector('.goadmin-confirmation-dialog')!
    const message =
      action === 'delete'
        ? locale === 'en'
          ? 'Are you sure you want to delete the record?'
          : 'Вы уверены, что хотите удалить запись?'
        : locale === 'en'
          ? 'Are you sure you want to perform this action?'
          : 'Вы уверены, что хотите выполнить действие?'
    expect(dialog.textContent).toContain(message)
    expect(dialog.querySelector('[data-confirmation-accept]')?.textContent).toBe(label)
    dialog.querySelector<HTMLButtonElement>('[data-confirmation-cancel]')!.click()
    await settle()
    expect(mocks.post).not.toHaveBeenCalled()
    expect(mocks.remove).not.toHaveBeenCalled()
    trigger.click()
    await settle()
    dialog = document.querySelector('.goadmin-confirmation-dialog')!
    dialog.querySelector<HTMLButtonElement>('[data-confirmation-accept]')!.click()
    await settle()
    const method = action === 'delete' ? mocks.remove : mocks.post
    expect(method).toHaveBeenCalledOnce()
    const [url, payload] = method.mock.calls[0]!
    expect(url).toContain(
      '/products/records/record-1' + (action === 'publish' ? '/actions/publish' : ''),
    )
    expect(action === 'publish' ? payload : payload.data).toEqual({ _revision: 'rev-1' })
  },
)
