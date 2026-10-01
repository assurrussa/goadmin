import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ImageUploader from './ImageUploader.vue'
import { setAdminCapabilities } from '@/composables/useAdminCapabilities'

const mocks = vi.hoisted(() => ({
  connect: vi.fn(),
  on: vi.fn(),
  upload: vi.fn(),
  poll: vi.fn(),
}))
vi.mock('@/composables/useAdminWebSocket', () => ({
  FILE_UPLOAD_STATUS_EVENT: 'files.upload.status',
  useAdminWebSocket: () => ({ ensureConnected: mocks.connect, on: mocks.on }),
}))
vi.mock('@/services/fileUploadService', () => ({
  uploadFiles: mocks.upload,
  deleteUploadedFile: vi.fn(),
}))
vi.mock('@/services/pollUploadTask', () => ({ pollUploadTask: mocks.poll }))

const apps: ReturnType<typeof createApp>[] = []
function mountUploader(onUpdate = vi.fn()) {
  const root = document.createElement('div')
  const app = createApp(ImageUploader, {
    objectType: 'admin',
    objectId: 7,
    'onUpdate:modelValue': onUpdate,
  }).use(createPinia())
  apps.push(app)
  app.mount(root)
  return root
}

afterEach(() => {
  apps.splice(0).forEach((app) => app.unmount())
  setAdminCapabilities({ props: {} })
  vi.clearAllMocks()
})

describe('image upload capabilities', () => {
  it('disables upload controls and does not connect when uploads are absent', () => {
    setAdminCapabilities({ props: { adminCapabilities: {} } })
    const root = mountUploader()
    expect(root.querySelector<HTMLInputElement>('input[type=file]')?.disabled).toBe(true)
    expect(mocks.connect).not.toHaveBeenCalled()
    expect(mocks.upload).not.toHaveBeenCalled()
  })

  it('completes an upload by polling when realtime is absent', async () => {
    setAdminCapabilities({ props: { adminCapabilities: { uploads: true } } })
    mocks.upload.mockResolvedValue({ status: 'queued', tasks: [{ id: 42, status: 'queued' }] })
    mocks.poll.mockResolvedValue({
      status: 'completed',
      task: { id: 42, status: 'completed' },
      file: { id: 42, filename: 'avatar.png', originalName: 'avatar.png', url: '/avatar.png' },
    })
    const update = vi.fn()
    const root = mountUploader(update)
    const input = root.querySelector<HTMLInputElement>('input[type=file]')!
    Object.defineProperty(input, 'files', {
      value: [new File(['image'], 'avatar.png', { type: 'image/png' })],
    })
    input.dispatchEvent(new Event('change'))
    await vi.waitFor(() =>
      expect(update).toHaveBeenCalledWith(expect.objectContaining({ id: 42, url: '/avatar.png' })),
    )
    expect(mocks.poll).toHaveBeenCalledWith(42, { signal: expect.any(AbortSignal) })
    expect(mocks.connect).not.toHaveBeenCalled()
    expect(mocks.on).not.toHaveBeenCalled()
  })

  it('cancels pending status polling when the uploader unmounts', async () => {
    setAdminCapabilities({ props: { adminCapabilities: { uploads: true } } })
    mocks.upload.mockResolvedValue({ status: 'queued', tasks: [{ id: 42, status: 'queued' }] })
    mocks.poll.mockReturnValue(new Promise(() => {}))
    const root = mountUploader()
    const input = root.querySelector<HTMLInputElement>('input[type=file]')!
    Object.defineProperty(input, 'files', {
      value: [new File(['image'], 'avatar.png', { type: 'image/png' })],
    })
    input.dispatchEvent(new Event('change'))
    await vi.waitFor(() => expect(mocks.poll).toHaveBeenCalled())
    const signal = mocks.poll.mock.calls[0][1].signal as AbortSignal
    expect(signal.aborted).toBe(false)
    apps.pop()?.unmount()
    expect(signal.aborted).toBe(true)
  })
})
