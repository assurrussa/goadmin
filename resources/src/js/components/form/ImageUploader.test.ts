import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, reactive } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
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
  it('requires explicit assignment after legacy unkeyed image recovery remount', async () => {
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
          file: { id: 42, filename: 'avatar.png', originalName: 'avatar.png', url: '/avatar.png' },
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
    let restored = mountUploader(update, { modelValue: { id: 6, url: '/kept.png' } })
    Array.from(restored.querySelectorAll('button'))
      .find((button) => button.textContent?.includes('Check upload'))!
      .click()
    await vi.waitFor(() => expect(restored.textContent).toContain('Use image in this field'))
    expect(update).not.toHaveBeenCalled()
    expect(restored.querySelector('img')?.getAttribute('src')).toBe('/kept.png')
    apps.pop()!.unmount()
    restored = mountUploader(update, { modelValue: { id: 6, url: '/kept.png' } })
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
  })
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
      useAuthUserStore(pinia).setAuthUser(null)
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
