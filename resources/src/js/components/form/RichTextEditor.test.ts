import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, reactive } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import { Editor as CoreEditor } from '@tiptap/core'
import { useAuthUserStore } from '@/stores/authUser'
import RichTextEditor from './RichTextEditor.vue'

import { useUploadCompletion } from '@/composables/useUploadCompletion'

const mocks = vi.hoisted(() => ({ upload: vi.fn(), reconcile: vi.fn(), task: vi.fn() }))
vi.mock('@/services/fileUploadService', () => ({
  uploadFiles: mocks.upload,
  reconcileUploadCompletion: mocks.reconcile,
  fetchUploadTask: mocks.task,
}))
vi.mock('@/composables/useAdminWebSocket', () => ({
  useAdminWebSocket: () => ({ ensureConnected: vi.fn(), on: vi.fn() }),
}))
vi.mock('../imageCropper', async () => {
  const { defineComponent, h } = await import('vue')
  return {
    ImageCropper: defineComponent({
      emits: ['crop', 'close'],
      setup(_, { emit }) {
        return () =>
          h(
            'button',
            {
              'data-test': 'confirm-crop',
              onClick: () =>
                emit('crop', new File(['cropped'], 'cropped.png', { type: 'image/png' })),
            },
            'Confirm crop fixture',
          )
      },
    }),
  }
})
let nextSession = 0
const apps: ReturnType<typeof createApp>[] = []
afterEach(async () => {
  apps.splice(0).forEach((app) => app.unmount())
  await new Promise((resolve) => setTimeout(resolve, 0))
  setActivePinia(createPinia())
  for (const id of [7, 8]) {
    for (const field of [undefined, 'body', 'summary']) {
      const recovery = useUploadCompletion(
        () => 'rich-text',
        () => id,
        () => 'rich-text',
        undefined,
        () => field,
      )
      if (recovery.pending.value) recovery.release(recovery.pending.value.completion)
    }
  }
  sessionStorage.clear()
  vi.restoreAllMocks()
  vi.clearAllMocks()
})

describe('unconfirmed rich text uploads', () => {
  it('stops a batch, blocks new uploads and explicitly checks the original session before inserting', async () => {
    const completion = {
      location: `/tus/original-${++nextSession}`,
      filename: 'clip.mp4',
      fileCategory: 'video',
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
    expect(mocks.reconcile).toHaveBeenCalledWith(completion, { signal: expect.any(AbortSignal) })
    expect(mocks.upload).toHaveBeenCalledTimes(1)
    expect(root.textContent).not.toContain('Check upload')
  })
  it('does not reconcile or insert into a different entity', async () => {
    const completion = {
      location: `/tus/original-${++nextSession}`,
      filename: 'clip.mp4',
      fileCategory: 'video',
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
    expect(root.textContent).not.toContain('Check upload')
    expect(input.disabled).toBe(false)
    expect(mocks.reconcile).not.toHaveBeenCalled()
    props.entityId = 7
    await nextTick()
    expect(root.textContent).toContain('Check upload')
    expect(input.disabled).toBe(true)
    expect(mocks.upload).toHaveBeenCalledTimes(1)
  })
  it.each(['failed', 'error'])(
    'unlocks fresh uploads after definitive %s completion replay',
    async (status) => {
      const completion = {
        location: `/tus/original-${++nextSession}`,
        filename: 'clip.mp4',
        fileCategory: 'video',
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
      expect(mocks.reconcile).toHaveBeenCalledWith(completion, { signal: expect.any(AbortSignal) })
      expect(mocks.upload).toHaveBeenCalledTimes(1)
      input.dispatchEvent(new Event('change'))
      await vi.waitFor(() => expect(mocks.upload).toHaveBeenCalledTimes(2))
    },
  )
  it.each(['unknown', 'dispatch'])(
    'restores the %s fence after navigation and releases a definite replay rejection',
    async (stage) => {
      const completion = {
        location: `/files/tus/original-${++nextSession}`,
        filename: 'clip.mp4',
        fileCategory: 'video',
        context: 'rich-text',
        entityType: 'rich-text',
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
      const mount = () => {
        const root = document.createElement('div')
        const app = createApp(RichTextEditor, { entityId: 7 }).use(createPinia())
        apps.push(app)
        app.mount(root)
        return root
      }
      const root = mount()
      const input = root.querySelector<HTMLInputElement>('input[type=file]')!
      Object.defineProperty(input, 'files', {
        value: [new File(['video'], 'clip.mp4', { type: 'video/mp4' })],
      })
      input.dispatchEvent(new Event('change'))
      await vi.waitFor(() => expect(mocks.upload).toHaveBeenCalledTimes(1))
      apps.pop()!.unmount()
      expect(mocks.upload.mock.calls[0][0].signal.aborted).toBe(true)
      const remounted = mount()
      const newInput = remounted.querySelector<HTMLInputElement>('input[type=file]')!
      expect(newInput.disabled).toBe(true)
      expect(remounted.textContent).toContain('Check upload')
      newInput.dispatchEvent(new Event('change'))
      expect(mocks.upload).toHaveBeenCalledTimes(1)
      mocks.reconcile.mockResolvedValue({ status: 'error', tasks: [], error: 'Media rejected' })
      Array.from(remounted.querySelectorAll('button'))
        .find((button) => button.textContent?.includes('Check upload'))!
        .click()
      await vi.waitFor(() => expect(newInput.disabled).toBe(false))
      expect(mocks.reconcile).toHaveBeenCalledWith(completion, { signal: expect.any(AbortSignal) })
      expect(mocks.upload).toHaveBeenCalledTimes(1)
    },
  )
  it.each(['unmount', 'entity'])(
    'does not continue a batch after %s changes during an unfinished upload',
    async (change) => {
      let finish!: (value: unknown) => void
      mocks.upload.mockImplementation(
        () =>
          new Promise((resolve) => {
            finish = resolve
          }),
      )
      const props = reactive({ entityId: 7 })
      const root = document.createElement('div')
      const app = createApp({ render: () => h(RichTextEditor, props) }).use(createPinia())
      apps.push(app)
      app.mount(root)
      const input = root.querySelector<HTMLInputElement>('input[type=file]')!
      Object.defineProperty(input, 'files', {
        value: [
          new File(['one'], 'one.mp4', { type: 'video/mp4' }),
          new File(['two'], 'two.mp4', { type: 'video/mp4' }),
        ],
      })
      input.dispatchEvent(new Event('change'))
      await vi.waitFor(() => expect(mocks.upload).toHaveBeenCalledTimes(1))
      if (change === 'unmount') apps.pop()!.unmount()
      else {
        props.entityId = 8
        await nextTick()
      }
      finish({ status: 'error', tasks: [], error: 'Upload cancelled' })
      await new Promise((resolve) => setTimeout(resolve, 0))
      expect(mocks.upload).toHaveBeenCalledTimes(1)
    },
  )
  it('restores only the stable rich-text field after reordered remounts', async () => {
    const completion = {
      location: `/files/tus/field-${++nextSession}`,
      filename: 'clip.mp4',
      fileCategory: 'video',
      context: 'rich-text',
      entityType: 'rich-text',
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
    const bodyInserted = vi.fn(),
      summaryInserted = vi.fn()
    const mountFields = (order: string[]) => {
      const root = document.createElement('div')
      const app = createApp({
        render: () =>
          h(
            'div',
            order.map((name) =>
              h('div', { 'data-field': name }, [
                h(RichTextEditor, {
                  entityId: 7,
                  uploadRecoveryKey: name,
                  'onFiles-uploaded': name === 'body' ? bodyInserted : summaryInserted,
                }),
              ]),
            ),
          ),
      }).use(createPinia())
      apps.push(app)
      app.mount(root)
      return root
    }
    const first = mountFields(['body', 'summary'])
    const input = first.querySelector<HTMLInputElement>('[data-field=body] input[type=file]')!
    Object.defineProperty(input, 'files', {
      value: [new File(['video'], 'clip.mp4', { type: 'video/mp4' })],
    })
    input.dispatchEvent(new Event('change'))
    await vi.waitFor(() =>
      expect(first.querySelector('[data-field=body]')!.textContent).toContain('Check upload'),
    )
    expect(first.querySelector('[data-field=summary]')!.textContent).not.toContain('Check upload')
    expect(
      first.querySelector<HTMLInputElement>('[data-field=summary] input[type=file]')!.disabled,
    ).toBe(false)
    apps.pop()!.unmount()
    const restored = mountFields(['summary', 'body'])
    expect(restored.querySelector('[data-field=summary]')!.textContent).not.toContain(
      'Check upload',
    )
    Array.from(restored.querySelectorAll('[data-field=body] button'))
      .find((button) => button.textContent?.includes('Check upload'))!
      .dispatchEvent(new MouseEvent('click'))
    await vi.waitFor(() => expect(bodyInserted).toHaveBeenCalledWith([42]))
    expect(summaryInserted).not.toHaveBeenCalled()
    expect(mocks.upload).toHaveBeenCalledTimes(1)
  })

  it('requires explicit field selection for legacy unkeyed recovery in a neighboring editor', async () => {
    const completion = {
      location: `/files/tus/unkeyed-${++nextSession}`,
      filename: 'clip.mp4',
      fileCategory: 'video',
      context: 'rich-text',
      entityType: 'rich-text',
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
    const inserted = vi.fn()
    const root = document.createElement('div')
    const app = createApp({
      render: () =>
        h('div', [
          h('div', { 'data-field': 'first' }, [h(RichTextEditor, { entityId: 7 })]),
          h('div', { 'data-field': 'second' }, [
            h(RichTextEditor, { entityId: 7, 'onFiles-uploaded': inserted }),
          ]),
        ]),
    }).use(createPinia())
    apps.push(app)
    app.mount(root)
    const input = root.querySelector<HTMLInputElement>('[data-field=first] input[type=file]')!
    Object.defineProperty(input, 'files', {
      value: [new File(['video'], 'clip.mp4', { type: 'video/mp4' })],
    })
    input.dispatchEvent(new Event('change'))
    await vi.waitFor(() => expect(root.textContent).toContain('Check upload'))
    Array.from(root.querySelectorAll('[data-field=second] button'))
      .find((button) => button.textContent?.includes('Check upload'))!
      .dispatchEvent(new MouseEvent('click'))
    await vi.waitFor(() =>
      expect(root.querySelector('[data-field=second]')!.textContent).toContain(
        'Insert into this field',
      ),
    )
    expect(inserted).not.toHaveBeenCalled()
    const recovery = useUploadCompletion(
      () => 'rich-text',
      () => 7,
      () => 'rich-text',
    )
    expect(recovery.pending.value?.completion.location).toBe(completion.location)

    expect(root.querySelector('[data-field=second] video')).toBeNull()
    Array.from(root.querySelectorAll('[data-field=second] button'))
      .find((button) => button.textContent?.includes('Insert into this field'))!
      .dispatchEvent(new MouseEvent('click'))
    await vi.waitFor(() => expect(inserted).toHaveBeenCalledWith([42]))
    expect(mocks.upload).toHaveBeenCalledTimes(1)
    expect(recovery.pending.value).toBeNull()
  })
  it('does not insert or stage a replay after a progress callback rebinds the entity', async () => {
    const completion = {
      location: `/files/tus/rebind-${++nextSession}`,
      filename: 'clip.mp4',
      fileCategory: 'video',
      context: 'rich-text',
      entityType: 'rich-text',
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
    const inserted = vi.fn()
    const props = reactive({ entityId: 7 })
    const root = document.createElement('div')
    const app = createApp({
      render: () =>
        h(RichTextEditor, {
          ...props,
          uploadRecoveryKey: 'body',
          'onFiles-uploaded': inserted,
          'onUpload-progress': (progress: number) => {
            if (progress === 100) props.entityId = 8
          },
        }),
    }).use(createPinia())
    apps.push(app)
    app.mount(root)
    const input = root.querySelector<HTMLInputElement>('input[type=file]')!
    Object.defineProperty(input, 'files', {
      value: [new File(['video'], 'clip.mp4', { type: 'video/mp4' })],
    })
    input.dispatchEvent(new Event('change'))
    await vi.waitFor(() => expect(root.textContent).toContain('Check upload'))
    Array.from(root.querySelectorAll('button'))
      .find((button) => button.textContent?.includes('Check upload'))!
      .click()
    await vi.waitFor(() => expect(props.entityId).toBe(8))
    await nextTick()
    expect(inserted).not.toHaveBeenCalled()
    expect(root.querySelector('video')).toBeNull()
    expect(root.textContent).not.toContain('Insert into this field')
    const original = useUploadCompletion(
      () => 'rich-text',
      () => 7,
      () => 'rich-text',
      undefined,
      () => 'body',
    )
    expect(original.pending.value?.completion.location).toBe(completion.location)
    original.release(completion)
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
    'retains $source $status recovery through $interruption until insertion',
    async ({ source, status, interruption }) => {
      const completion = {
        location: `/files/tus/lifecycle-${++nextSession}`,
        filename: 'clip.mp4',
        fileCategory: 'video',
        context: 'rich-text',
        entityType: 'rich-text',
        entityId: 7,
      }
      const accepted = { status: 'queued', tasks: [{ id: 42, status }] }
      const completedFile = {
        id: 42,
        filename: 'clip.mp4',
        originalName: 'clip.mp4',
        url: '/clip.mp4',
        mimeType: 'video/mp4',
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
      mocks.task.mockImplementation(
        () =>
          new Promise((resolve, reject) => {
            finish = resolve
            fail = reject
          }),
      )
      const inserted = vi.fn()
      const props = reactive({
        entityId: 7,
        uploadRecoveryKey: 'body',
        'onFiles-uploaded': inserted,
      })
      const mount = () => {
        const root = document.createElement('div')
        const app = createApp({ render: () => h(RichTextEditor, props) }).use(createPinia())
        apps.push(app)
        app.mount(root)
        return root
      }
      let root = mount()
      const input = root.querySelector<HTMLInputElement>('input[type=file]')!
      Object.defineProperty(input, 'files', {
        value: [new File(['video'], 'clip.mp4', { type: 'video/mp4' })],
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
      await vi.waitFor(() => expect(mocks.task).toHaveBeenCalledTimes(1))
      const oldSignal = mocks.task.mock.calls[0][1] as AbortSignal
      expect(inserted).not.toHaveBeenCalled()
      expect(input.disabled).toBe(true)
      if (interruption === 'unmount') {
        apps.pop()!.unmount()
        expect(oldSignal.aborted).toBe(true)
      } else if (interruption === 'rebind') {
        props.entityId = 8
        await nextTick()
      }
      if (interruption === 'poll error') fail(new Error('Offline'))
      else
        finish({ status: 'completed', task: { id: 42, status: 'completed' }, file: completedFile })
      await new Promise((resolve) => setTimeout(resolve, 0))
      expect(inserted).not.toHaveBeenCalled()
      if (interruption === 'unmount') root = mount()
      else {
        props.entityId = 7
        await nextTick()
      }
      expect(root.querySelector<HTMLInputElement>('input[type=file]')!.disabled).toBe(true)
      mocks.reconcile.mockResolvedValue({
        status: 'completed',
        tasks: [{ id: 42, status: 'completed', file: completedFile }],
      })
      await vi.waitFor(() => expect(check()?.disabled).toBe(false))
      check().click()
      await vi.waitFor(() => expect(inserted).toHaveBeenCalledExactlyOnceWith([42]))
      expect(mocks.upload).toHaveBeenCalledTimes(1)
      expect(mocks.reconcile).toHaveBeenLastCalledWith(completion, {
        signal: expect.any(AbortSignal),
      })
      expect(root.querySelector<HTMLInputElement>('input[type=file]')!.disabled).toBe(false)
    },
  )

  it.each(['entity', 'field', 'endpoint', 'owner', 'unmount'])(
    'keeps fresh completed recovery when progress changes %s before insertion',
    async (change) => {
      const completion = {
        location: `/files/tus/fresh-rebind-${++nextSession}`,
        filename: 'clip.mp4',
        fileCategory: 'video',
        context: 'rich-text',
        entityType: 'rich-text',
        entityId: 7,
      }
      mocks.upload.mockImplementation((request) => {
        request.onCompletionSession(completion)
        return Promise.resolve({
          status: 'queued',
          tasks: [
            {
              id: 42,
              status: 'completed',
              file: { id: 42, filename: 'clip.mp4', url: '/clip.mp4', mimeType: 'video/mp4' },
            },
          ],
        })
      })
      const inserted = vi.fn()
      const props = reactive({ entityId: 7, uploadRecoveryKey: 'body', uploadUrl: '/files' })
      const pinia = createPinia()
      let finished = false
      const root = document.createElement('div')
      const app = createApp({
        render: () =>
          h(RichTextEditor, {
            ...props,
            'onFiles-uploaded': inserted,
            'onUpload-progress': (progress: number) => {
              if (progress !== 100) return
              if (change === 'entity') props.entityId = 8
              if (change === 'field') props.uploadRecoveryKey = 'summary'
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
              finished = true
            },
          }),
      }).use(pinia)
      apps.push(app)
      app.mount(root)
      const input = root.querySelector<HTMLInputElement>('input[type=file]')!
      Object.defineProperty(input, 'files', {
        value: [new File(['video'], 'clip.mp4', { type: 'video/mp4' })],
      })
      input.dispatchEvent(new Event('change'))
      await vi.waitFor(() => expect(finished).toBe(true))
      await nextTick()
      expect(inserted).not.toHaveBeenCalled()
      expect(root.querySelector('video')).toBeNull()
      useAuthUserStore(pinia).setAuthUser(null)
      const original = useUploadCompletion(
        () => 'rich-text',
        () => 7,
        () => 'rich-text',
        undefined,
        () => 'body',
      )
      expect(original.pending.value?.completion.location).toBe(completion.location)
      original.release(completion)
    },
  )
  it.each(['false', 'throw'])(
    'keeps a completed replay recoverable when editor insertion returns %s',
    async (failure) => {
      const completion = {
        location: `/files/tus/insertion-${++nextSession}`,
        filename: 'clip.mp4',
        fileCategory: 'video',
        context: 'rich-text',
        entityType: 'rich-text',
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
            file: { id: 42, filename: 'clip.mp4', url: '/clip.mp4', mimeType: 'video/mp4' },
          },
        ],
      })
      const inserted = vi.fn()
      const root = document.createElement('div')
      const app = createApp(RichTextEditor, {
        entityId: 7,
        uploadRecoveryKey: 'body',
        'onFiles-uploaded': inserted,
      }).use(createPinia())
      apps.push(app)
      app.mount(root)
      const input = root.querySelector<HTMLInputElement>('input[type=file]')!
      Object.defineProperty(input, 'files', {
        value: [new File(['video'], 'clip.mp4', { type: 'video/mp4' })],
      })
      input.dispatchEvent(new Event('change'))
      await vi.waitFor(() => expect(root.textContent).toContain('Check upload'))
      const originalChain = CoreEditor.prototype.chain
      const chainSpy = vi.spyOn(CoreEditor.prototype, 'chain').mockImplementation(function (
        this: CoreEditor,
      ) {
        const chain = originalChain.call(this)
        chain.run = () => {
          if (failure === 'throw') throw new Error('Insertion unavailable')
          return false
        }
        return chain
      })
      const check = () =>
        Array.from(root.querySelectorAll('button')).find((button) =>
          button.textContent?.includes('Check upload'),
        )!
      check().click()
      await vi.waitFor(() => expect(mocks.reconcile).toHaveBeenCalledTimes(1))
      await vi.waitFor(() => expect(check()?.disabled).toBe(false))
      expect(inserted).not.toHaveBeenCalled()
      expect(root.querySelector('video')).toBeNull()
      const recovery = useUploadCompletion(
        () => 'rich-text',
        () => 7,
        () => 'rich-text',
        undefined,
        () => 'body',
      )
      expect(recovery.pending.value?.completion.location).toBe(completion.location)
      chainSpy.mockRestore()
      check().click()
      await vi.waitFor(() => expect(inserted).toHaveBeenCalledExactlyOnceWith([42]))
      expect(recovery.pending.value).toBeNull()
      expect(mocks.upload).toHaveBeenCalledTimes(1)
    },
  )
  it.each(
    ['drop', 'cropper', 'replay'].flatMap((flow) =>
      ['assign', 'rebind'].map((outcome) => ({ flow, outcome })),
    ),
  )(
    'preserves the image $flow completion until $outcome at the real insertion boundary',
    async ({ flow, outcome }) => {
      const completion = {
        location: `/files/tus/image-${++nextSession}`,
        filename: flow === 'cropper' ? 'cropped.png' : 'photo.png',
        fileCategory: 'image',
        context: 'rich-text',
        entityType: 'rich-text',
        entityId: 7,
      }
      const completed = {
        status: 'completed',
        tasks: [
          {
            id: 42,
            status: 'completed',
            file: { id: 42, filename: 'photo.png', url: '/photo.png', mimeType: 'image/png' },
          },
        ],
      }
      mocks.upload.mockImplementation((request) => {
        request.onCompletionSession(completion)
        return Promise.resolve(
          flow === 'replay'
            ? { status: 'completion_unknown', tasks: [], uncertainCompletion: completion }
            : completed,
        )
      })
      mocks.reconcile.mockResolvedValue(completed)
      const props = reactive({ entityId: 7 })
      const inserted = vi.fn(),
        modelAssigned = vi.fn()
      const root = document.createElement('div')
      const app = createApp({
        render: () =>
          h(RichTextEditor, {
            ...props,
            uploadRecoveryKey: 'body',
            'onFiles-uploaded': inserted,
            'onUpload-progress': (progress: number) => {
              if (progress === 100 && outcome === 'rebind') props.entityId = 8
            },
            'onUpdate:modelValue': (value: unknown) => {
              if (!JSON.stringify(value).includes('/photo.png')) return
              const recovery = useUploadCompletion(
                () => 'rich-text',
                () => 7,
                () => 'rich-text',
                undefined,
                () => 'body',
              )
              modelAssigned(recovery.pending.value?.completion.location)
            },
          }),
      }).use(createPinia())
      apps.push(app)
      app.mount(root)
      const file = new File(['image'], 'photo.png', { type: 'image/png' })
      if (flow === 'cropper') {
        const input = root.querySelector<HTMLInputElement>('input[type=file]')!
        Object.defineProperty(input, 'files', { value: [file] })
        input.dispatchEvent(new Event('change'))
        await vi.waitFor(() =>
          expect(root.querySelector('[data-test=confirm-crop]')).not.toBeNull(),
        )
        root.querySelector<HTMLButtonElement>('[data-test=confirm-crop]')!.click()
      } else {
        const drag = new Event('dragenter')
        Object.defineProperty(drag, 'dataTransfer', { value: { types: ['Files'] } })
        document.dispatchEvent(drag)
        await nextTick()
        const drop = new Event('drop')
        Object.defineProperty(drop, 'dataTransfer', { value: { files: [file], types: ['Files'] } })
        root.querySelector('.upload-overlay')!.dispatchEvent(drop)
      }
      if (flow === 'replay') {
        await vi.waitFor(() => expect(root.textContent).toContain('Check upload'))
        Array.from(root.querySelectorAll('button'))
          .find((button) => button.textContent?.includes('Check upload'))!
          .click()
      }
      const recovery = useUploadCompletion(
        () => 'rich-text',
        () => 7,
        () => 'rich-text',
        undefined,
        () => 'body',
      )
      if (outcome === 'assign') {
        await vi.waitFor(() => expect(inserted).toHaveBeenCalledExactlyOnceWith([42]))
        expect(modelAssigned).toHaveBeenCalledWith(completion.location)
        expect(root.querySelector('img')?.getAttribute('src')).toBe('/photo.png')
        expect(recovery.pending.value).toBeNull()
      } else {
        await vi.waitFor(() => expect(props.entityId).toBe(8))
        await nextTick()
        expect(inserted).not.toHaveBeenCalled()
        expect(modelAssigned).not.toHaveBeenCalled()
        expect(recovery.pending.value?.completion.location).toBe(completion.location)
        recovery.release(completion)
      }
      expect(mocks.upload).toHaveBeenCalledTimes(1)
    },
  )
})
