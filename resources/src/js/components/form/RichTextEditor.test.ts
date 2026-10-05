import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, reactive } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import RichTextEditor from './RichTextEditor.vue'

import { useUploadCompletion } from '@/composables/useUploadCompletion'

const mocks = vi.hoisted(() => ({ upload: vi.fn(), reconcile: vi.fn() }))
vi.mock('@/services/fileUploadService', () => ({
  uploadFiles: mocks.upload,
  reconcileUploadCompletion: mocks.reconcile,
  fetchUploadTask: vi.fn(),
}))
vi.mock('@/composables/useAdminWebSocket', () => ({
  useAdminWebSocket: () => ({ ensureConnected: vi.fn(), on: vi.fn() }),
}))
let nextSession = 0
const apps: ReturnType<typeof createApp>[] = []
afterEach(async () => {
  apps.splice(0).forEach((app) => app.unmount())
  await new Promise((resolve) => setTimeout(resolve, 0))
  setActivePinia(createPinia())
  for (const id of [7, 8]) {
    const recovery = useUploadCompletion(
      () => 'rich-text',
      () => id,
      () => 'rich-text',
    )
    if (recovery.pending.value) recovery.release(recovery.pending.value.completion)
  }
  sessionStorage.clear()
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
})
