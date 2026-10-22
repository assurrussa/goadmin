import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { createApp, defineComponent, h, nextTick } from 'vue'
import { createPinia } from 'pinia'
import { useAuthUserStore } from '@/stores/authUser'
import { useUploadQueueStore } from '@/stores/uploadQueue'
import { provideUploadTaskTracker } from './useUploadTaskTracker'
import { setAdminCapabilities } from './useAdminCapabilities'
import UploadQueueContainer from '@/components/UploadQueueContainer.vue'
import type { FileUploadStatusEvent } from './useAdminWebSocket'

const mocks = vi.hoisted(() => ({ poll: vi.fn(), on: vi.fn(), connect: vi.fn(), off: vi.fn() }))
vi.mock('@/services/pollUploadTask', () => ({ pollUploadTask: mocks.poll }))
vi.mock('./useAdminWebSocket', () => ({
  useAdminWebSocket: () => ({ on: mocks.on, ensureConnected: mocks.connect }),
}))
let app: ReturnType<typeof createApp>
let tracker: ReturnType<typeof provideUploadTaskTracker>
let queue: ReturnType<typeof useUploadQueueStore>
let auth: ReturnType<typeof useAuthUserStore>
let event: (event: FileUploadStatusEvent) => void
const file = {
  id: 42,
  filename: 'image.png',
  originalName: 'image.png',
  url: '/image.png',
  entityId: null,
  fileType: 'image',
  sortOrder: 0,
}
const terminalEvent = (status = 'completed'): FileUploadStatusEvent => ({
  eventId: 'event',
  eventType: 'files.upload.status',
  taskId: 42,
  status,
  file: { ...file, fileName: file.filename, mimeType: 'image/png', size: 5 },
  error: status === 'failed' ? 'Rejected' : undefined,
})
beforeEach(() => {
  vi.useFakeTimers()
  vi.clearAllMocks()
  mocks.poll.mockImplementation(() => new Promise(() => {}))
  mocks.on.mockImplementation((_name, callback) => {
    event = callback
    return mocks.off
  })
  setAdminCapabilities({ props: { adminCapabilities: { uploads: true, realtime: true } } })
  const pinia = createPinia()
  auth = useAuthUserStore(pinia)
  auth.setAuthUser({ id: '7' } as never)
  queue = useUploadQueueStore(pinia)
  app = createApp(
    defineComponent({
      setup() {
        tracker = provideUploadTaskTracker()
        return () => h(UploadQueueContainer)
      },
    }),
  ).use(pinia)
  app.mount(document.createElement('div'))
})
afterEach(() => {
  app.unmount()
  expect(vi.getTimerCount()).toBe(0)
  vi.useRealTimers()
  document.body.innerHTML = ''
  setAdminCapabilities(null)
})
const accept = (status = 'queued', policy?: { attempts: number; interval: number }) => {
  const id = queue.start('image.png')
  const observation = tracker.accept(
    tracker.capture(),
    { id: 42, status, file: status === 'completed' ? file : undefined },
    status,
    id,
    policy,
  )!
  return { id, observation }
}
it.each(['websocket', 'poll'])(
  'deduplicates terminal delivery with %s first and one monitor per task',
  async (first) => {
    let finish!: (value: unknown) => void
    mocks.poll.mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = resolve
        }),
    )
    const { observation } = accept()
    accept()
    expect(mocks.poll).toHaveBeenCalledTimes(1)
    const listener = vi.fn()
    observation.subscribe(listener)
    const result = { status: 'completed', task: { id: 42, status: 'completed' }, file }
    if (first === 'websocket') event(terminalEvent())
    else finish(result)
    await Promise.resolve()
    if (first === 'websocket') finish(result)
    else event(terminalEvent('failed'))
    await Promise.resolve()
    expect(queue.uploads.map((item) => item.status)).toEqual(['completed'])
    expect(listener.mock.calls.filter(([value]) => value.status === 'completed')).toHaveLength(1)
  },
)
it.each(['completed', 'failed'])('handles immediate %s without polling', (status) => {
  accept(status)
  expect(mocks.poll).not.toHaveBeenCalled()
  expect(queue.uploads[0]?.status).toBe(status === 'failed' ? 'error' : 'completed')
})
it('retains unknown state and rechecks through the queue with a read-only task request', async () => {
  mocks.poll.mockRejectedValueOnce(new Error('Network unavailable'))
  const { id } = accept()
  await vi.waitFor(() => expect(queue.uploads[0]?.status).toBe('unknown'))
  await nextTick()
  expect(document.body.querySelector('[data-upload-status="unknown"]')).not.toBeNull()
  expect(document.body.textContent).not.toContain('Upload failed')
  mocks.poll.mockResolvedValueOnce({
    status: 'completed',
    task: { id: 42, status: 'completed' },
    file,
  })
  document.body.querySelector<HTMLButtonElement>('[data-upload-recheck]')!.click()
  await vi.waitFor(() => expect(queue.uploads[0]?.status).toBe('completed'))
  expect(mocks.poll).toHaveBeenCalledTimes(2)
  expect(mocks.poll.mock.calls[1]![0]).toBe(42)
  expect(queue.uploads[0]?.id).toBe(id)
})
it('bounds a stalled request, retaining unknown until a later realtime terminal event', async () => {
  accept('queued', { attempts: 2, interval: 1000 })
  const signal = mocks.poll.mock.calls[0]![1].signal as AbortSignal
  await vi.advanceTimersByTimeAsync(17_000)
  expect(signal.aborted).toBe(true)
  expect(queue.uploads[0]?.status).toBe('unknown')
  event(terminalEvent())
  expect(queue.uploads[0]?.status).toBe('completed')
})
it('uses distinct video observation bounds', () => {
  accept('queued', { attempts: 180, interval: 1500 })
  expect(mocks.poll).toHaveBeenCalledWith(42, {
    maxAttempts: 180,
    intervalMs: 1500,
    signal: expect.any(AbortSignal),
  })
})
it('discards late acceptance/results on logout and same-owner relogin epochs', async () => {
  auth.setAuthUser({ id: '7' } as never)
  const stale = tracker.capture()
  accept()
  const signal = mocks.poll.mock.calls[0]![1].signal as AbortSignal
  auth.setAuthUser(null)
  auth.setAuthUser({ id: '7' } as never)
  expect(signal.aborted).toBe(true)
  expect(queue.uploads).toEqual([])
  expect(tracker.accept(stale, { id: 42, status: 'queued' }, 'queued', 'stale')).toBeUndefined()
  event(terminalEvent())
  expect(queue.uploads).toEqual([])
})
it('clears dismissal timers and monitors on frame disposal', () => {
  accept('completed')
  expect(vi.getTimerCount()).toBe(1)
  app.unmount()
  expect(queue.uploads).toEqual([])
  expect(mocks.off).toHaveBeenCalledTimes(1)
  // Prevent a second Vue unmount warning in teardown.
  app = { unmount() {} } as ReturnType<typeof createApp>
})

it('rejects anonymous acceptance and never initiates a websocket connection on auth pages', () => {
  auth.setAuthUser(null)
  expect(
    tracker.accept(tracker.capture(), { id: 42, status: 'queued' }, 'queued', 'anonymous'),
  ).toBeUndefined()
  expect(mocks.poll).not.toHaveBeenCalled()
  expect(mocks.connect).not.toHaveBeenCalled()
})
it('rejects acceptance when uploads are disabled', () => {
  setAdminCapabilities({ props: { adminCapabilities: { realtime: true } } })
  expect(
    tracker.accept(tracker.capture(), { id: 42, status: 'queued' }, 'queued', 'disabled'),
  ).toBeUndefined()
  expect(mocks.poll).not.toHaveBeenCalled()
})
it('retains an immediate terminal snapshot after queue dismissal without retaining listeners', async () => {
  const { observation } = accept('completed')
  const listener = vi.fn()
  observation.subscribe(listener)
  await vi.advanceTimersByTimeAsync(queue.dismissDelay)
  expect(queue.uploads).toEqual([])
  observation.subscribe(listener)
  expect(listener).toHaveBeenCalledTimes(2)
  event(terminalEvent('failed'))
  expect(listener).toHaveBeenCalledTimes(2)
})

it.each(['owner', 'dispose'])(
  'cancels local observers explicitly on %s invalidation without claiming server failure',
  (change) => {
    const { observation } = accept()
    const snapshot = vi.fn()
    const cancel = vi.fn()
    observation.subscribe(snapshot, cancel)
    if (change === 'owner') auth.setAuthUser(null)
    else {
      app.unmount()
      app = { unmount() {} } as ReturnType<typeof createApp>
    }
    expect(cancel).toHaveBeenCalledTimes(1)
    expect(snapshot.mock.calls.map(([value]) => value.status)).toEqual(['processing'])
    expect(queue.uploads).toEqual([])
    const stale = vi.fn()
    observation.subscribe(vi.fn(), stale)
    expect(stale).toHaveBeenCalledTimes(1)
  },
)
