import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ImageUploader from './ImageUploader.vue'
import { setAdminCapabilities } from '@/composables/useAdminCapabilities'

const mocks = vi.hoisted(() => ({
  connect: vi.fn(),
  on: vi.fn(),
  upload: vi.fn(),
  reconcile: vi.fn(),
  poll: vi.fn(),
}))
vi.mock('@/composables/useAdminWebSocket', () => ({
  FILE_UPLOAD_STATUS_EVENT: 'files.upload.status',
  useAdminWebSocket: () => ({ ensureConnected: mocks.connect, on: mocks.on }),
}))
vi.mock('@/services/fileUploadService', () => ({
  uploadFiles: mocks.upload,
  reconcileUploadCompletion: mocks.reconcile,
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

  it('retains an unconfirmed session and blocks fresh uploads until an explicit recheck succeeds', async () => {
    setAdminCapabilities({ props: { adminCapabilities: { uploads: true } } })
    const completion = {
      location: '/tus/original',
      filename: 'avatar.png',
      entityType: 'admin',
      entityId: 7,
    }
    const unknown = {
      status: 'completion_unknown',
      tasks: [],
      error: 'Upload completion is unconfirmed.',
      uncertainCompletion: completion,
    }
    mocks.upload.mockResolvedValue(unknown)
    mocks.reconcile
      .mockResolvedValueOnce(unknown)
      .mockResolvedValueOnce({ status: 'queued', tasks: [{ id: 42, status: 'queued' }] })
    mocks.poll.mockResolvedValue({
      status: 'completed',
      task: { id: 42 },
      file: { id: 42, filename: 'avatar.png', url: '/avatar.png' },
    })
    const update = vi.fn()
    const root = mountUploader(update)
    const input = root.querySelector<HTMLInputElement>('input[type=file]')!
    const file = new File(['image'], 'avatar.png', { type: 'image/png' })
    Object.defineProperty(input, 'files', { value: [file] })
    input.dispatchEvent(new Event('change'))
    await vi.waitFor(() => expect(root.textContent).toContain('Check upload'))
    expect(input.disabled).toBe(true)
    expect(mocks.reconcile).not.toHaveBeenCalled()
    input.dispatchEvent(new Event('change'))
    expect(mocks.upload).toHaveBeenCalledTimes(1)
    expect(root.textContent).toContain('Upload completion is unconfirmed.')
    const check = () =>
      Array.from(root.querySelectorAll('button')).find((button) =>
        button.textContent?.includes('Check upload'),
      )!
    check().click()
    await vi.waitFor(() => expect(mocks.reconcile).toHaveBeenCalledTimes(1))
    await vi.waitFor(() => expect(check().disabled).toBe(false))
    expect(input.disabled).toBe(true)
    check().click()
    await vi.waitFor(() => expect(update).toHaveBeenCalledWith(expect.objectContaining({ id: 42 })))
    expect(mocks.reconcile).toHaveBeenNthCalledWith(1, completion)
    expect(mocks.reconcile).toHaveBeenNthCalledWith(2, completion)
    expect(mocks.upload).toHaveBeenCalledTimes(1)
    expect(mocks.upload.mock.calls[0][0].file).toBe(file)
    expect(root.textContent).not.toContain('Check upload')
  })

  it('accepts a completed same-session replay without waiting for another realtime event', async () => {
    setAdminCapabilities({ props: { adminCapabilities: { uploads: true, realtime: true } } })
    const completion = {
      location: '/tus/original',
      filename: 'avatar.png',
      entityType: 'admin',
      entityId: 7,
    }
    mocks.upload.mockResolvedValue({
      status: 'completion_unknown',
      tasks: [],
      uncertainCompletion: completion,
    })
    mocks.reconcile.mockResolvedValue({
      status: 'completed',
      tasks: [
        {
          id: 42,
          status: 'completed',
          file: { id: 42, filename: 'avatar.png', url: '/avatar.png' },
        },
      ],
    })
    const update = vi.fn()
    const root = mountUploader(update)
    const input = root.querySelector<HTMLInputElement>('input[type=file]')!
    Object.defineProperty(input, 'files', {
      value: [new File(['image'], 'avatar.png', { type: 'image/png' })],
    })
    input.dispatchEvent(new Event('change'))
    await vi.waitFor(() => expect(root.textContent).toContain('Check upload'))
    Array.from(root.querySelectorAll('button'))
      .find((button) => button.textContent?.includes('Check upload'))!
      .click()
    await vi.waitFor(() =>
      expect(update).toHaveBeenCalledWith(expect.objectContaining({ id: 42, url: '/avatar.png' })),
    )
    expect(mocks.reconcile).toHaveBeenCalledWith(completion)
    expect(mocks.upload).toHaveBeenCalledTimes(1)
    expect(mocks.on).not.toHaveBeenCalled()
    expect(mocks.poll).not.toHaveBeenCalled()
    expect(input.disabled).toBe(false)
    expect(root.textContent).not.toContain('Check upload')
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
  it.each(['failed', 'error'])(
    'unlocks fresh uploads after definitive %s completion replay',
    async (status) => {
      const completion = {
        location: '/tus/original',
        filename: 'avatar.png',
        entityType: 'admin',
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
      setAdminCapabilities({ props: { adminCapabilities: { uploads: true } } })
      const root = mountUploader()
      const input = root.querySelector<HTMLInputElement>('input[type=file]')!
      Object.defineProperty(input, 'files', {
        value: [new File(['media'], 'avatar.png', { type: 'image/png' })],
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
