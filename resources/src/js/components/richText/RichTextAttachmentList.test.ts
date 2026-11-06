import { afterEach, expect, it, vi } from 'vitest'
import { createApp, defineComponent, h, nextTick, reactive, type App } from 'vue'
import AttachmentList from './RichTextAttachmentList.vue'

const mocks = vi.hoisted(() => ({ confirm: vi.fn(), delete: vi.fn() }))
vi.mock('@/composables/useConfirmation', () => ({
  useConfirmation: () => async (options: unknown, onConfirm: () => void | Promise<void>) => {
    const accepted = await mocks.confirm(options)
    if (accepted) await onConfirm()
    return accepted
  },
}))
vi.mock('@/services/fileUploadService', () => ({ deleteUploadedFile: mocks.delete }))
vi.mock('@/components/ui/AppButton.vue', () => ({
  default: defineComponent({
    setup:
      (_, { attrs, slots }) =>
      () =>
        h('button', attrs, slots.default?.()),
  }),
}))
const apps: App[] = []
afterEach(() => {
  apps.splice(0).forEach((app) => app.unmount())
  vi.resetAllMocks()
})
function mount() {
  const file = reactive({ id: 7, objectType: 'article', filename: 'photo.jpg', isPrimary: false })
  const props = reactive({
    files: [file],
    attachedIds: [] as number[],
    allowDeleteUnattached: true,
  })
  const removed = vi.fn()
  const root = document.createElement('div')
  const app = createApp(
    defineComponent({ setup: () => () => h(AttachmentList, { ...props, onRemoved: removed }) }),
  )
  apps.push(app)
  app.mount(root)
  return { root, props, removed, file }
}
it.each([true, false])('awaits the file deletion decision accepted=%s', async (accepted) => {
  let resolve!: (accepted: boolean) => void
  mocks.confirm.mockReturnValue(
    new Promise<boolean>((done) => {
      resolve = done
    }),
  )
  mocks.delete.mockResolvedValue(undefined)
  const { root, file, removed } = mount()
  root.querySelector('button')!.click()
  expect(mocks.delete).not.toHaveBeenCalled()
  expect(mocks.confirm).toHaveBeenCalledWith({
    message: 'Удалить файл?',
    confirmLabel: 'Удалить',
    variant: 'danger',
  })
  file.id = 99
  resolve(accepted)
  await nextTick()
  await nextTick()
  if (accepted) {
    expect(mocks.delete).toHaveBeenCalledExactlyOnceWith(7)
    expect(removed).toHaveBeenCalledExactlyOnceWith(7)
  } else {
    expect(mocks.delete).not.toHaveBeenCalled()
    expect(removed).not.toHaveBeenCalled()
  }
})
it.each(['attached', 'disabled'] as const)(
  'rechecks %s guard after an asynchronous decision',
  async (guard) => {
    let resolve!: (accepted: boolean) => void
    mocks.confirm.mockReturnValue(
      new Promise<boolean>((done) => {
        resolve = done
      }),
    )
    const { root, props } = mount()
    root.querySelector('button')!.click()
    if (guard === 'attached') props.attachedIds = [7]
    else props.allowDeleteUnattached = false
    await nextTick()
    resolve(true)
    await nextTick()
    expect(mocks.delete).not.toHaveBeenCalled()
  },
)
it('does not dispatch a second delete while the first request is running', async () => {
  mocks.confirm.mockResolvedValue(true)
  mocks.delete.mockReturnValue(new Promise(() => {}))
  const { root } = mount()
  root.querySelector('button')!.click()
  root.querySelector('button')!.click()
  await nextTick()
  expect(mocks.delete).toHaveBeenCalledExactlyOnceWith(7)
})
