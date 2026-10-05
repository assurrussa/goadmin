import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, reactive } from 'vue'
import { createPinia } from 'pinia'
import RichTextEditor from './RichTextEditor.vue'

const mocks = vi.hoisted(() => ({ upload: vi.fn(), reconcile: vi.fn() }))
vi.mock('@/services/fileUploadService', () => ({
  uploadFiles: mocks.upload,
  reconcileUploadCompletion: mocks.reconcile,
  fetchUploadTask: vi.fn(),
}))
vi.mock('@/composables/useAdminWebSocket', () => ({
  useAdminWebSocket: () => ({ ensureConnected: vi.fn(), on: vi.fn() }),
}))
const apps: ReturnType<typeof createApp>[] = []
afterEach(() => {
  apps.splice(0).forEach((app) => app.unmount())
  vi.clearAllMocks()
})

describe('unconfirmed rich text uploads', () => {
  it('stops a batch, blocks new uploads and explicitly checks the original session before inserting', async () => {
    const completion = {
      location: '/tus/original',
      filename: 'clip.mp4',
      entityType: 'rich-text',
      entityId: 7,
    }
    mocks.upload.mockResolvedValue({
      status: 'completion_unknown',
      tasks: [],
      uncertainCompletion: completion,
      error: 'Upload completion is unconfirmed.',
    })
    mocks.reconcile.mockResolvedValue({
      status: 'queued',
      tasks: [
        {
          id: 42,
          status: 'completed',
          file: {
            id: 42,
            filename: 'clip.mp4',
            originalName: 'clip.mp4',
            url: '/clip.mp4',
            mimeType: 'video/mp4',
          },
        },
      ],
    })
    const uploaded = vi.fn()
    const root = document.createElement('div')
    const app = createApp(RichTextEditor, { entityId: 7, 'onFile-upload': uploaded }).use(
      createPinia(),
    )
    apps.push(app)
    app.mount(root)
    const input = root.querySelector<HTMLInputElement>('input[type=file]')!
    const original = new File(['video'], 'clip.mp4', { type: 'video/mp4' })
    Object.defineProperty(input, 'files', {
      value: [original, new File(['second'], 'second.mp4', { type: 'video/mp4' })],
    })
    input.dispatchEvent(new Event('change'))
    await vi.waitFor(() => expect(root.textContent).toContain('Check upload'))
    expect(mocks.upload).toHaveBeenCalledTimes(1)
    expect(mocks.reconcile).not.toHaveBeenCalled()
    expect(input.disabled).toBe(true)
    input.dispatchEvent(new Event('change'))
    expect(mocks.upload).toHaveBeenCalledTimes(1)
    Array.from(root.querySelectorAll('button'))
      .find((button) => button.textContent?.includes('Check upload'))!
      .click()
    await vi.waitFor(() => expect(uploaded).toHaveBeenCalledWith(original))
    expect(mocks.reconcile).toHaveBeenCalledWith(completion)
    expect(mocks.upload).toHaveBeenCalledTimes(1)
    expect(root.textContent).not.toContain('Check upload')
  })
  it('does not reconcile or insert into a different entity', async () => {
    const completion = {
      location: '/tus/original',
      filename: 'clip.mp4',
      entityType: 'rich-text',
      entityId: 7,
    }
    mocks.upload.mockResolvedValue({
      status: 'completion_unknown',
      tasks: [],
      uncertainCompletion: completion,
    })
    const props = reactive({ entityId: 7 })
    const root = document.createElement('div')
    const app = createApp({ render: () => h(RichTextEditor, props) }).use(createPinia())
    apps.push(app)
    app.mount(root)
    const input = root.querySelector<HTMLInputElement>('input[type=file]')!
    Object.defineProperty(input, 'files', {
      value: [new File(['video'], 'clip.mp4', { type: 'video/mp4' })],
    })
    input.dispatchEvent(new Event('change'))
    await vi.waitFor(() => expect(root.textContent).toContain('Check upload'))
    props.entityId = 8
    await nextTick()
    const check = Array.from(root.querySelectorAll('button')).find((button) =>
      button.textContent?.includes('Check upload'),
    )!
    expect(check.disabled).toBe(true)
    check.click()
    input.dispatchEvent(new Event('change'))
    expect(mocks.reconcile).not.toHaveBeenCalled()
    expect(mocks.upload).toHaveBeenCalledTimes(1)
  })
  it.each(['failed', 'error'])(
    'unlocks fresh uploads after definitive %s completion replay',
    async (status) => {
      const completion = {
        location: '/tus/original',
        filename: 'clip.mp4',
        entityType: 'rich-text',
        entityId: 7,
      }
      mocks.upload.mockResolvedValue({
        status: 'completion_unknown',
        tasks: [],
        uncertainCompletion: completion,
      })
      mocks.reconcile.mockResolvedValue({
        status,
        tasks: [{ id: 42, status, error: 'Processing rejected' }],
      })
      const root = document.createElement('div')
      const app = createApp(RichTextEditor, { entityId: 7 }).use(createPinia())
      apps.push(app)
      app.mount(root)
      const input = root.querySelector<HTMLInputElement>('input[type=file]')!
      Object.defineProperty(input, 'files', {
        value: [new File(['media'], 'clip.mp4', { type: 'video/mp4' })],
      })
      input.dispatchEvent(new Event('change'))
      await vi.waitFor(() => expect(root.textContent).toContain('Check upload'))
      Array.from(root.querySelectorAll('button'))
        .find((button) => button.textContent?.includes('Check upload'))!
        .click()
      await vi.waitFor(() => expect(input.disabled).toBe(false))
      expect(root.textContent).not.toContain('Check upload')
      expect(mocks.reconcile).toHaveBeenCalledWith(completion)
      expect(mocks.upload).toHaveBeenCalledTimes(1)
      input.dispatchEvent(new Event('change'))
      await vi.waitFor(() => expect(mocks.upload).toHaveBeenCalledTimes(2))
    },
  )
})
