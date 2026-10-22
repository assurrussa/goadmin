import {
  createUploadTestApp as createApp,
  uploadTestOwner,
} from '@/composables/__tests__/uploadTaskTrackerTestHarness'
import { setLocale } from '@/i18n'
import { afterEach, describe, expect, it, vi, beforeEach } from 'vitest'
import { h, nextTick, reactive } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import { useUploadQueueStore } from '@/stores/uploadQueue'
import { useAuthUserStore } from '@/stores/authUser'
import ImageUploader from './ImageUploader.vue'
import { setAdminCapabilities } from '@/composables/useAdminCapabilities'

import { useUploadCompletion } from '@/composables/useUploadCompletion'

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

let nextSession = 0
const apps: ReturnType<typeof createApp>[] = []
function mountUploader(onUpdate = vi.fn(), extra = {}) {
  const root = document.createElement('div')
  const app = createApp(ImageUploader, {
    objectType: 'admin',
    objectId: 7,
    'onUpdate:modelValue': onUpdate,
    ...extra,
  }).use(createPinia())
  apps.push(app)
  app.mount(root)
  return root
}

afterEach(async () => {
  apps.splice(0).forEach((app) => app.unmount())
  setAdminCapabilities({ props: {} })
  await new Promise((resolve) => setTimeout(resolve, 0))
  setActivePinia(createPinia())
  useAuthUserStore().setAuthUser(uploadTestOwner)
  for (const id of [7, 8]) {
    for (const field of [undefined, 'avatar-lifecycle']) {
      const recovery = useUploadCompletion(
        () => 'admin',
        () => id,
        () => 'image-uploader',
        undefined,
        () => field,
      )
      if (recovery.pending.value) recovery.release(recovery.pending.value.completion)
    }
  }
  sessionStorage.clear()
  vi.clearAllMocks()
})

describe('image upload capabilities', () => {
  it.each(['websocket', 'poll'])(
    'assigns once when %s completes before the other channel',
    async (first) => {
      setAdminCapabilities({ props: { adminCapabilities: { uploads: true, realtime: true } } })
      mocks.on.mockReturnValue(vi.fn())
      mocks.upload.mockResolvedValue({ status: 'queued', tasks: [{ id: 42, status: 'queued' }] })
      let finish!: (result: unknown) => void
      mocks.poll.mockImplementation(
        () =>
          new Promise((resolve) => {
            finish = resolve
          }),
      )
      const update = vi.fn()
      const root = mountUploader(update)
      const input = root.querySelector<HTMLInputElement>('input[type=file]')!
      Object.defineProperty(input, 'files', {
        value: [new File(['image'], 'avatar.png', { type: 'image/png' })],
      })
      input.dispatchEvent(new Event('change'))
      await vi.waitFor(() => expect(mocks.poll).toHaveBeenCalled())
      const listener = mocks.on.mock.calls.at(-1)![1]
      const event = {
        eventType: 'files.upload.status',
        taskId: 42,
        status: 'completed',
        file: { id: 42, fileName: 'avatar.png', url: '/avatar.png' },
      }
      const pollResult = {
        status: 'completed',
        task: { id: 42 },
        file: { id: 42, filename: 'avatar.png', url: '/avatar.png' },
      }
      if (first === 'websocket') listener(event)
      else finish(pollResult)
      await vi.waitFor(() => expect(update).toHaveBeenCalledTimes(1))
      if (first === 'websocket') finish(pollResult)
      else listener(event)
      await nextTick()
      expect(update).toHaveBeenCalledTimes(1)
      expect(mocks.upload).toHaveBeenCalledTimes(1)
    },
  )

  it('ignores an old polling error after a realtime field changes entity', async () => {
    setAdminCapabilities({ props: { adminCapabilities: { uploads: true, realtime: true } } })
    mocks.on.mockReturnValue(vi.fn())
    mocks.upload.mockResolvedValue({ status: 'queued', tasks: [{ id: 42, status: 'queued' }] })
    let reject!: (reason: Error) => void
    mocks.poll.mockImplementation(
      () =>
        new Promise((_resolve, fail) => {
          reject = fail
        }),
    )
    const onError = vi.fn()
    const props = reactive({ objectType: 'admin', objectId: 7, onError })
    const root = document.createElement('div')
    const app = createApp({ render: () => h(ImageUploader, props) }).use(createPinia())
    apps.push(app)
    app.mount(root)
    const input = root.querySelector<HTMLInputElement>('input[type=file]')!
    Object.defineProperty(input, 'files', {
      value: [new File(['image'], 'avatar.png', { type: 'image/png' })],
    })
    input.dispatchEvent(new Event('change'))
    await vi.waitFor(() => expect(mocks.poll).toHaveBeenCalled())
    props.objectId = 8
    await nextTick()
    reject(new Error('stale polling error'))
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(onError).not.toHaveBeenCalled()
    expect(root.textContent).not.toContain('stale polling error')
  })
  it('reconciles a missed realtime terminal event through the same task status', async () => {
    setAdminCapabilities({ props: { adminCapabilities: { uploads: true, realtime: true } } })
    const unsubscribe = vi.fn()
    mocks.on.mockReturnValue(unsubscribe)
    mocks.upload.mockResolvedValue({ status: 'queued', tasks: [{ id: 42, status: 'queued' }] })
    mocks.poll.mockResolvedValue({
      status: 'completed',
      task: { id: 42, status: 'completed' },
      file: { id: 42, filename: 'avatar.png', url: '/avatar.png' },
    })
    const update = vi.fn()
    const root = mountUploader(update)
    const input = root.querySelector<HTMLInputElement>('input[type=file]')!
    Object.defineProperty(input, 'files', {
      value: [new File(['image'], 'avatar.png', { type: 'image/png' })],
    })
    input.dispatchEvent(new Event('change'))
    await vi.waitFor(() => expect(update).toHaveBeenCalledTimes(1))
    expect(mocks.upload).toHaveBeenCalledTimes(1)
    expect(mocks.poll).toHaveBeenCalledWith(42, {
      signal: expect.any(AbortSignal),
      maxAttempts: 120,
      intervalMs: 1000,
    })
    expect(unsubscribe).not.toHaveBeenCalled()
    expect(input.disabled).toBe(false)
  })

  it('keeps realtime recovery alive after status polling fails', async () => {
    setAdminCapabilities({ props: { adminCapabilities: { uploads: true, realtime: true } } })
    const unsubscribe = vi.fn()
    mocks.on.mockReturnValue(unsubscribe)
    mocks.upload.mockResolvedValue({ status: 'queued', tasks: [{ id: 42, status: 'queued' }] })
    mocks.poll.mockRejectedValue(new Error('status temporarily unavailable'))
    const update = vi.fn()
    const root = mountUploader(update)
    const input = root.querySelector<HTMLInputElement>('input[type=file]')!
    Object.defineProperty(input, 'files', {
      value: [new File(['image'], 'avatar.png', { type: 'image/png' })],
    })
    input.dispatchEvent(new Event('change'))
    await vi.waitFor(() => expect(root.textContent).toContain('status temporarily unavailable'))
    expect(unsubscribe).not.toHaveBeenCalled()
    const listener = mocks.on.mock.calls.at(-1)![1]
    const event = {
      eventType: 'files.upload.status',
      taskId: 42,
      status: 'completed',
      file: { id: 42, fileName: 'avatar.png', url: '/avatar.png' },
    }
    listener(event)
    listener(event)
    await vi.waitFor(() => expect(update).toHaveBeenCalledTimes(1))
    expect(unsubscribe).not.toHaveBeenCalled()
    expect(root.textContent).not.toContain('status temporarily unavailable')
  })
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
    expect(mocks.poll).toHaveBeenCalledWith(42, {
      signal: expect.any(AbortSignal),
      maxAttempts: 120,
      intervalMs: 1000,
    })
    expect(mocks.connect).not.toHaveBeenCalled()
    expect(mocks.on).toHaveBeenCalledTimes(1)
  })

  it('retains an unconfirmed session and blocks fresh uploads until an explicit recheck succeeds', async () => {
    setAdminCapabilities({ props: { adminCapabilities: { uploads: true } } })
    const completion = {
      location: `/tus/original-${++nextSession}`,
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
    expect(mocks.reconcile).toHaveBeenNthCalledWith(1, completion, {
      signal: expect.any(AbortSignal),
    })
    expect(mocks.reconcile).toHaveBeenNthCalledWith(2, completion, {
      signal: expect.any(AbortSignal),
    })
    expect(mocks.upload).toHaveBeenCalledTimes(1)
    expect(mocks.upload.mock.calls[0][0].file).toBe(file)
    expect(root.textContent).not.toContain('Check upload')
  })

  it('accepts a completed same-session replay without waiting for another realtime event', async () => {
    setAdminCapabilities({ props: { adminCapabilities: { uploads: true, realtime: true } } })
    const completion = {
      location: `/tus/original-${++nextSession}`,
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
    expect(mocks.reconcile).toHaveBeenCalledWith(completion, { signal: expect.any(AbortSignal) })
    expect(mocks.upload).toHaveBeenCalledTimes(1)
    expect(mocks.on).toHaveBeenCalledTimes(1)
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
        location: `/tus/original-${++nextSession}`,
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
      expect(mocks.reconcile).toHaveBeenCalledWith(completion, { signal: expect.any(AbortSignal) })
      expect(mocks.upload).toHaveBeenCalledTimes(1)
      input.dispatchEvent(new Event('change'))
      await vi.waitFor(() => expect(mocks.upload).toHaveBeenCalledTimes(2))
    },
  )
  it.each(['unknown', 'dispatch'])(
    'restores the %s fence after navigation and releases a definite replay rejection',
    async (stage) => {
      setAdminCapabilities({ props: { adminCapabilities: { uploads: true } } })
      const completion = {
        location: `/files/tus/original-${++nextSession}`,
        filename: 'avatar.png',
        entityType: 'admin',
        entityId: 7,
      }
      mocks.upload.mockImplementation((request) => {
        request.onCompletionSession(completion)
        return stage === 'dispatch'
          ? new Promise(() => {})
          : Promise.resolve({
              status: 'completion_unknown',
              tasks: [],
              uncertainCompletion: completion,
            })
      })
      const root = mountUploader()
      const input = root.querySelector<HTMLInputElement>('input[type=file]')!
      Object.defineProperty(input, 'files', {
        value: [new File(['image'], 'avatar.png', { type: 'image/png' })],
      })
      input.dispatchEvent(new Event('change'))
      await vi.waitFor(() => expect(mocks.upload).toHaveBeenCalledTimes(1))
      apps.pop()!.unmount()
      expect(mocks.upload.mock.calls[0][0].signal.aborted).toBe(true)
      const remounted = mountUploader()
      const newInput = remounted.querySelector<HTMLInputElement>('input[type=file]')!
      expect(newInput.disabled).toBe(true)
      expect(remounted.textContent).toContain('Check upload')
      newInput.dispatchEvent(new Event('change'))
      expect(mocks.upload).toHaveBeenCalledTimes(1)
      mocks.reconcile.mockResolvedValue({ status: 'error', tasks: [], error: 'Session expired' })
      Array.from(remounted.querySelectorAll('button'))
        .find((button) => button.textContent?.includes('Check upload'))!
        .click()
      await vi.waitFor(() => expect(newInput.disabled).toBe(false))
      expect(remounted.textContent).toContain('Session expired')
      expect(mocks.reconcile).toHaveBeenCalledWith(completion, { signal: expect.any(AbortSignal) })
      expect(mocks.upload).toHaveBeenCalledTimes(1)
    },
  )
  it('does not apply an old task result after the entity changes', async () => {
    setAdminCapabilities({ props: { adminCapabilities: { uploads: true } } })
    mocks.upload.mockResolvedValue({ status: 'queued', tasks: [{ id: 42, status: 'queued' }] })
    let finish!: (value: unknown) => void
    mocks.poll.mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = resolve
        }),
    )
    const update = vi.fn()
    const props = reactive({ objectType: 'admin', objectId: 7, 'onUpdate:modelValue': update })
    const root = document.createElement('div')
    const app = createApp({ render: () => h(ImageUploader, props) }).use(createPinia())
    apps.push(app)
    app.mount(root)
    const input = root.querySelector<HTMLInputElement>('input[type=file]')!
    Object.defineProperty(input, 'files', {
      value: [new File(['image'], 'avatar.png', { type: 'image/png' })],
    })
    input.dispatchEvent(new Event('change'))
    await vi.waitFor(() => expect(mocks.poll).toHaveBeenCalled())
    props.objectId = 8
    await nextTick()
    finish({
      status: 'completed',
      task: { id: 42 },
      file: { id: 42, filename: 'old.png', url: '/old.png' },
    })
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(update).not.toHaveBeenCalled()
  })
  it.each([undefined, 'avatar-lifecycle'])(
    'requires explicit assignment after legacy recovery remount with key %s',
    async (uploadRecoveryKey) => {
      setAdminCapabilities({ props: { adminCapabilities: { uploads: true } } })
      const completion = {
        location: `/files/tus/unkeyed-${++nextSession}`,
        filename: 'avatar.png',
        context: 'image-uploader',
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
            tempUrl: '/foreign-temp.png',
            file: {
              id: 42,
              filename: 'avatar.png',
              originalName: 'avatar.png',
              url: '/avatar.png',
            },
          },
        ],
      })
      const first = mountUploader()
      const input = first.querySelector<HTMLInputElement>('input[type=file]')!
      Object.defineProperty(input, 'files', {
        value: [new File(['image'], 'avatar.png', { type: 'image/png' })],
      })
      input.dispatchEvent(new Event('change'))
      await vi.waitFor(() => expect(first.textContent).toContain('Check upload'))
      apps.pop()!.unmount()
      const update = vi.fn()
      let restored = mountUploader(update, {
        modelValue: { id: 6, url: '/kept.png' },
        uploadRecoveryKey,
      })
      Array.from(restored.querySelectorAll('button'))
        .find((button) => button.textContent?.includes('Check upload'))!
        .click()
      await vi.waitFor(() => expect(restored.textContent).toContain('Use image in this field'))
      expect(update).not.toHaveBeenCalled()
      expect(restored.querySelector('img')?.getAttribute('src')).toBe('/kept.png')
      apps.pop()!.unmount()
      restored = mountUploader(update, {
        modelValue: { id: 6, url: '/kept.png' },
        uploadRecoveryKey,
      })
      expect(restored.querySelector<HTMLInputElement>('input[type=file]')!.disabled).toBe(true)
      Array.from(restored.querySelectorAll('button'))
        .find((button) => button.textContent?.includes('Check upload'))!
        .click()
      await vi.waitFor(() => expect(restored.textContent).toContain('Use image in this field'))
      expect(update).not.toHaveBeenCalled()

      Array.from(restored.querySelectorAll('button'))
        .find((button) => button.textContent?.includes('Use image in this field'))!
        .click()
      expect(update).toHaveBeenCalledWith(expect.objectContaining({ id: 42 }))
      expect(mocks.upload).toHaveBeenCalledTimes(1)
    },
  )
  it.each(
    ['fresh', 'replay'].flatMap((source) =>
      ['queued', 'processing'].flatMap((status) =>
        ['unmount', 'rebind', 'poll error'].map((interruption) => ({
          source,
          status,
          interruption,
        })),
      ),
    ),
  )(
    'retains $source $status recovery through $interruption until assignment',
    async ({ source, status, interruption }) => {
      setAdminCapabilities({ props: { adminCapabilities: { uploads: true } } })
      const completion = {
        location: `/files/tus/lifecycle-${++nextSession}`,
        filename: 'avatar.png',
        context: 'image-uploader',
        entityType: 'admin',
        entityId: 7,
      }
      const accepted = {
        status,
        tasks: [
          {
            id: 42,
            status,
            tempUrl: '/preview.png',
            file: { id: 42, filename: 'avatar.png', url: '/not-ready.png' },
          },
        ],
      }
      const completed = {
        status: 'completed',
        tasks: [
          {
            id: 42,
            status: 'completed',
            file: {
              id: 42,
              filename: 'avatar.png',
              originalName: 'avatar.png',
              url: '/avatar.png',
            },
          },
        ],
      }
      mocks.upload.mockImplementation((request) => {
        request.onCompletionSession(completion)
        return Promise.resolve(
          source === 'fresh'
            ? accepted
            : { status: 'completion_unknown', tasks: [], uncertainCompletion: completion },
        )
      })
      mocks.reconcile.mockResolvedValue(accepted)
      let finish!: (value: unknown) => void
      let fail!: (reason: Error) => void
      mocks.poll.mockImplementation(
        () =>
          new Promise((resolve, reject) => {
            finish = resolve
            fail = reject
          }),
      )
      const update = vi.fn()
      const props = reactive({
        objectType: 'admin',
        objectId: 7,
        uploadRecoveryKey: 'avatar-lifecycle',
        'onUpdate:modelValue': update,
      })
      const mount = () => {
        const root = document.createElement('div')
        const app = createApp({ render: () => h(ImageUploader, props) }).use(createPinia())
        apps.push(app)
        app.mount(root)
        return root
      }
      let root = mount()
      const input = root.querySelector<HTMLInputElement>('input[type=file]')!
      Object.defineProperty(input, 'files', {
        value: [new File(['image'], 'avatar.png', { type: 'image/png' })],
      })
      input.dispatchEvent(new Event('change'))
      const check = () =>
        Array.from(root.querySelectorAll('button')).find((button) =>
          button.textContent?.includes('Check upload'),
        )!
      if (source === 'replay') {
        await vi.waitFor(() => expect(check()?.disabled).toBe(false))
        check().click()
      }
      await vi.waitFor(() => expect(mocks.poll).toHaveBeenCalledTimes(1))
      expect(update).not.toHaveBeenCalled()
      expect(input.disabled).toBe(true)
      const oldSignal = mocks.poll.mock.calls[0][1].signal
      if (interruption === 'unmount') {
        apps.pop()!.unmount()
        expect(oldSignal.aborted).toBe(true)
      } else if (interruption === 'rebind') {
        props.objectId = 8
        await nextTick()
      }
      if (interruption === 'poll error') fail(new Error('Offline'))
      else
        finish({
          status: 'completed',
          task: { id: 42, status: 'completed' },
          file: completed.tasks[0].file,
        })
      await new Promise((resolve) => setTimeout(resolve, 0))
      expect(update).not.toHaveBeenCalled()
      if (interruption === 'unmount') root = mount()
      else {
        props.objectId = 7
        await nextTick()
      }
      expect(root.querySelector<HTMLInputElement>('input[type=file]')!.disabled).toBe(true)
      mocks.reconcile.mockResolvedValue(completed)
      check().click()
      await vi.waitFor(() =>
        expect(update).toHaveBeenCalledExactlyOnceWith(
          expect.objectContaining({ id: 42, url: '/avatar.png' }),
        ),
      )
      expect(mocks.upload).toHaveBeenCalledTimes(1)
      expect(mocks.reconcile).toHaveBeenLastCalledWith(completion, {
        signal: expect.any(AbortSignal),
      })
      expect(root.querySelector<HTMLInputElement>('input[type=file]')!.disabled).toBe(false)
    },
  )
  it('assigns a fresh completed response and releases only after the model update', async () => {
    setAdminCapabilities({ props: { adminCapabilities: { uploads: true } } })
    const completion = {
      location: `/files/tus/fresh-done-${++nextSession}`,
      filename: 'avatar.png',
      context: 'image-uploader',
      entityType: 'admin',
      entityId: 7,
    }
    mocks.upload.mockImplementation((request) => {
      request.onCompletionSession(completion)
      return Promise.resolve({
        status: 'completed',
        tasks: [
          {
            id: 42,
            status: 'completed',
            file: { id: 42, filename: 'avatar.png', url: '/avatar.png' },
          },
        ],
      })
    })
    const onUpdate = vi.fn(() => {
      const recovery = useUploadCompletion(
        () => 'admin',
        () => 7,
        () => 'image-uploader',
        undefined,
        () => 'avatar-lifecycle',
      )
      expect(recovery.pending.value?.completion.location).toBe(completion.location)
    })
    const root = mountUploader(onUpdate, { uploadRecoveryKey: 'avatar-lifecycle' })
    const input = root.querySelector<HTMLInputElement>('input[type=file]')!
    Object.defineProperty(input, 'files', {
      value: [new File(['image'], 'avatar.png', { type: 'image/png' })],
    })
    input.dispatchEvent(new Event('change'))
    await vi.waitFor(() =>
      expect(onUpdate).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({ id: 42 })),
    )
    expect(mocks.poll).not.toHaveBeenCalled()
    expect(root.textContent).not.toContain('Check upload')
  })
  it.each(
    ['fresh', 'replay'].flatMap((source) =>
      ['entity', 'field', 'endpoint', 'owner', 'unmount'].map((change) => ({ source, change })),
    ),
  )(
    'retains $source completed image when progress changes $change before assignment',
    async ({ source, change }) => {
      setAdminCapabilities({ props: { adminCapabilities: { uploads: true } } })
      const completion = {
        location: `/files/tus/progress-${++nextSession}`,
        filename: 'avatar.png',
        context: 'image-uploader',
        entityType: 'admin',
        entityId: 7,
      }
      const completed = {
        status: 'completed',
        tasks: [
          {
            id: 42,
            status: 'completed',
            file: { id: 42, filename: 'avatar.png', url: '/avatar.png' },
          },
        ],
      }
      mocks.upload.mockImplementation((request) => {
        request.onCompletionSession(completion)
        return Promise.resolve(
          source === 'fresh'
            ? completed
            : { status: 'completion_unknown', tasks: [], uncertainCompletion: completion },
        )
      })
      mocks.reconcile.mockResolvedValue(completed)
      const update = vi.fn()
      const props = reactive({
        objectType: 'admin',
        objectId: 7,
        uploadRecoveryKey: 'avatar-lifecycle',
        uploadUrl: '/files',
      })
      const pinia = createPinia()
      let changed = false
      const root = document.createElement('div')
      const app = createApp({
        render: () =>
          h(ImageUploader, {
            ...props,
            'onUpdate:modelValue': update,
            onProgress: (progress: number) => {
              if (progress !== 100 || (source === 'replay' && !mocks.reconcile.mock.calls.length))
                return
              if (change === 'entity') props.objectId = 8
              if (change === 'field') props.uploadRecoveryKey = 'other'
              if (change === 'endpoint') props.uploadUrl = '/other/files'
              if (change === 'owner')
                useAuthUserStore(pinia).setAuthUser({
                  id: 'other',
                  name: '',
                  lastName: '',
                  email: '',
                  roles: [],
                  avatarUrl: '',
                })
              if (change === 'unmount') apps.pop()!.unmount()
              changed = true
            },
          }),
      }).use(pinia)
      apps.push(app)
      app.mount(root)
      const input = root.querySelector<HTMLInputElement>('input[type=file]')!
      Object.defineProperty(input, 'files', {
        value: [new File(['image'], 'avatar.png', { type: 'image/png' })],
      })
      input.dispatchEvent(new Event('change'))
      if (source === 'replay') {
        const check = () =>
          Array.from(root.querySelectorAll('button')).find((button) =>
            button.textContent?.includes('Check upload'),
          )!
        await vi.waitFor(() => expect(check()?.disabled).toBe(false))
        check().click()
      }
      await vi.waitFor(() => expect(changed).toBe(true))
      await nextTick()
      expect(update).not.toHaveBeenCalled()
      useAuthUserStore(pinia).setAuthUser(uploadTestOwner)
      const original = useUploadCompletion(
        () => 'admin',
        () => 7,
        () => 'image-uploader',
        undefined,
        () => 'avatar-lifecycle',
      )
      expect(original.pending.value?.completion.location).toBe(completion.location)
      original.release(completion)
    },
  )
})

// Existing English control selectors run under the explicit English UI locale.
beforeEach(() => setLocale('en'))
afterEach(() => setLocale('ru'))

it('keeps observing an accepted server task after the field unmounts on navigation', async () => {
  setAdminCapabilities({ props: { adminCapabilities: { uploads: true, realtime: false } } })
  mocks.upload.mockResolvedValue({ status: 'queued', tasks: [{ id: 942, status: 'queued' }] })
  let finish!: (result: unknown) => void
  mocks.poll.mockImplementation(
    () =>
      new Promise((resolve) => {
        finish = resolve
      }),
  )
  const visible = reactive({ field: true })
  const update = vi.fn()
  const pinia = createPinia()
  const app = createApp({
    render: () =>
      visible.field
        ? h(ImageUploader, {
            objectType: 'admin',
            objectId: 7,
            'onUpdate:modelValue': update,
          })
        : h('main', 'Next page'),
  }).use(pinia)
  apps.push(app)
  const root = document.createElement('div')
  app.mount(root)
  const input = root.querySelector<HTMLInputElement>('input[type=file]')!
  Object.defineProperty(input, 'files', {
    value: [new File(['image'], 'avatar.png', { type: 'image/png' })],
  })
  input.dispatchEvent(new Event('change'))
  await vi.waitFor(() => expect(mocks.poll).toHaveBeenCalled())
  visible.field = false
  await nextTick()
  finish({
    status: 'completed',
    task: { id: 942 },
    file: { id: 942, filename: 'avatar.png', url: '/avatar.png' },
  })
  await vi.waitFor(() => expect(useUploadQueueStore(pinia).uploads[0]?.status).toBe('completed'))
  expect(update).not.toHaveBeenCalled()
})

it.each(['completed', 'failed', 'queued'])(
  'tracks late accepted %s after the field unmounts before the response arrives',
  async (status) => {
    setAdminCapabilities({ props: { adminCapabilities: { uploads: true, realtime: false } } })
    let accept!: (value: unknown) => void
    mocks.upload.mockImplementation(
      () =>
        new Promise((resolve) => {
          accept = resolve
        }),
    )
    if (status === 'queued')
      mocks.poll.mockResolvedValue({
        status: 'completed',
        task: { id: 943, status: 'completed' },
        file: { id: 943, filename: 'avatar.png', url: '/avatar.png' },
      })
    const visible = reactive({ field: true })
    const update = vi.fn()
    const pinia = createPinia()
    const app = createApp({
      render: () =>
        visible.field
          ? h(ImageUploader, {
              objectType: 'admin',
              objectId: 7,
              'onUpdate:modelValue': update,
            })
          : h('main', 'Next page'),
    }).use(pinia)
    apps.push(app)
    const root = document.createElement('div')
    app.mount(root)
    const input = root.querySelector<HTMLInputElement>('input[type=file]')!
    Object.defineProperty(input, 'files', {
      value: [new File(['image'], 'avatar.png', { type: 'image/png' })],
    })
    input.dispatchEvent(new Event('change'))
    await vi.waitFor(() => expect(mocks.upload).toHaveBeenCalled())
    visible.field = false
    await nextTick()
    accept({
      status,
      tasks: [
        {
          id: 943,
          status,
          file:
            status === 'completed'
              ? { id: 943, filename: 'avatar.png', url: '/avatar.png' }
              : undefined,
          error: 'Server rejection',
        },
      ],
    })
    await vi.waitFor(() =>
      expect(useUploadQueueStore(pinia).uploads[0]?.status).toBe(
        status === 'failed' ? 'error' : 'completed',
      ),
    )
    expect(update).not.toHaveBeenCalled()
    if (status === 'queued') expect(mocks.poll).toHaveBeenCalledTimes(1)
    else expect(mocks.poll).not.toHaveBeenCalled()
  },
)

it.each(['reject', 'uncertain-response'])(
  'removes an unobserved TUS queue row after navigation cancellation (%s)',
  async (result) => {
    const completion = {
      location: `/tus/navigation-${++nextSession}`,
      filename: 'avatar.png',
      entityType: 'admin',
      entityId: 7,
    }
    let finish!: (value: unknown) => void
    mocks.upload.mockImplementation(
      ({ signal, onCompletionSession }) =>
        new Promise((resolve, reject) => {
          onCompletionSession(completion)
          finish = resolve
          signal.addEventListener(
            'abort',
            () => {
              if (result === 'reject') reject(new DOMException('Cancelled', 'AbortError'))
              else
                resolve({
                  status: 'completion_unknown',
                  tasks: [],
                  uncertainCompletion: completion,
                })
            },
            { once: true },
          )
        }),
    )
    const visible = reactive({ field: true })
    const pinia = createPinia()
    const app = createApp({
      render: () =>
        visible.field ? h(ImageUploader, { objectType: 'admin', objectId: 7 }) : h('main'),
    }).use(pinia)
    apps.push(app)
    const root = document.createElement('div')
    app.mount(root)
    const input = root.querySelector<HTMLInputElement>('input[type=file]')!
    Object.defineProperty(input, 'files', {
      value: [new File(['image'], 'avatar.png', { type: 'image/png' })],
    })
    input.dispatchEvent(new Event('change'))
    await vi.waitFor(() => expect(mocks.upload).toHaveBeenCalled())
    expect(useUploadQueueStore(pinia).uploads).toHaveLength(1)
    visible.field = false
    await nextTick()
    await vi.waitFor(() => expect(useUploadQueueStore(pinia).uploads).toEqual([]))
    finish({ status: 'error', tasks: [] })
    setActivePinia(pinia)
    const recovery = useUploadCompletion(
      () => 'admin',
      () => 7,
      () => 'image-uploader',
    )
    expect(recovery.pending.value?.completion.location).toBe(completion.location)
    expect(mocks.poll).not.toHaveBeenCalled()
  },
)
