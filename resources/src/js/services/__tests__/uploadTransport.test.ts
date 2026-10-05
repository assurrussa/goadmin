import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import axios from 'axios'
import { uploadFiles, uploadPendingWithTus, reconcileUploadCompletion } from '../fileUploadService'
vi.mock('axios', () => ({
  default: { post: vi.fn(), head: vi.fn(), patch: vi.fn(), isAxiosError: vi.fn(() => false) },
}))
const file = () => new File(['bytes'], 'track.mp3', { type: 'audio/mpeg' })
const request = () => ({
  file: file(),
  entityType: 'meditation',
  entityId: 1,
  requestTimeoutMs: 1000,
})
const completion = () => ({
  location: '/files/tus/original',
  filename: 'track.mp3',
  entityType: 'meditation',
  entityId: 1,
})
const tick = async () => {
  await vi.advanceTimersByTimeAsync(0)
}
beforeEach(() => {
  vi.useFakeTimers()
  vi.spyOn(console, 'error').mockImplementation(() => {})
  vi.mocked(axios.post)
    .mockResolvedValueOnce({ headers: { location: '/files/tus/session' } })
    .mockResolvedValue({ data: { tasks: [{ id: 9, status: 'queued' }] } })
  vi.mocked(axios.head).mockResolvedValue({ headers: { 'upload-offset': '0' } })
  vi.mocked(axios.patch).mockResolvedValue({ status: 204, headers: { 'upload-offset': '5' } })
})
afterEach(() => {
  vi.useRealTimers()
  vi.restoreAllMocks()
  vi.resetAllMocks()
})
describe('bounded upload requests', () => {
  it.each(['create', 'head', 'patch', 'complete'])(
    'aborts a stalled %s without an automatic retry',
    async (stage) => {
      const hang = () => new Promise<never>(() => {})
      if (stage === 'create') vi.mocked(axios.post).mockReset().mockImplementation(hang)
      if (stage === 'head') vi.mocked(axios.head).mockImplementation(hang)
      if (stage === 'patch') vi.mocked(axios.patch).mockImplementation(hang)
      if (stage === 'complete')
        vi.mocked(axios.post)
          .mockReset()
          .mockResolvedValueOnce({ headers: { location: '/files/tus/session' } })
          .mockImplementation(hang)
      const pending = uploadFiles(request())
      await tick()
      const config =
        stage === 'head'
          ? vi.mocked(axios.head).mock.calls.at(-1)![1]
          : stage === 'patch'
            ? vi.mocked(axios.patch).mock.calls.at(-1)![2]
            : vi.mocked(axios.post).mock.calls.at(-1)![2]
      await vi.advanceTimersByTimeAsync(1001)
      expect((await pending).error).toContain(
        stage === 'complete' ? 'completion is unconfirmed' : 'timed out',
      )
      expect(config!.signal!.aborted).toBe(true)
      expect(axios.post).toHaveBeenCalledTimes(stage === 'complete' ? 2 : 1)
      expect(vi.getTimerCount()).toBe(0)
    },
  )
  it('allows a slow active request to exceed its inactivity budget', async () => {
    let resolve!: (value: unknown) => void
    vi.mocked(axios.patch).mockImplementation(
      () =>
        new Promise((r) => {
          resolve = r
        }),
    )
    const pending = uploadFiles(request())
    await tick()
    const config = vi.mocked(axios.patch).mock.calls[0][2]!
    for (let i = 0; i < 4; i++) {
      await vi.advanceTimersByTimeAsync(900)
      config.onUploadProgress!({ loaded: i + 1 } as never)
    }
    expect(config.signal!.aborted).toBe(false)
    resolve({ status: 204, headers: { 'upload-offset': '5' } })
    expect((await pending).error).toBeUndefined()
    expect(vi.getTimerCount()).toBe(0)
  })
  it('cancels an in-flight request and ignores its late success', async () => {
    let resolve!: (value: unknown) => void
    vi.mocked(axios.patch).mockImplementation(
      () =>
        new Promise((r) => {
          resolve = r
        }),
    )
    const controller = new AbortController()
    const onProgress = vi.fn()
    const pending = uploadFiles({ ...request(), signal: controller.signal, onProgress })
    await tick()
    const config = vi.mocked(axios.patch).mock.calls[0][2]!
    controller.abort()
    expect((await pending).error).toBe('Upload cancelled')
    expect(config.signal!.aborted).toBe(true)
    resolve({ status: 204, headers: { 'upload-offset': '5' } })
    await tick()
    expect(onProgress.mock.calls).toEqual([[0]])
    expect(axios.post).toHaveBeenCalledTimes(1)
  })
  it('does not accept or resend a cancelled completion with a late response', async () => {
    let resolve!: (value: unknown) => void
    vi.mocked(axios.post)
      .mockReset()
      .mockResolvedValueOnce({ headers: { location: '/files/tus/session' } })
      .mockImplementation(
        () =>
          new Promise((r) => {
            resolve = r
          }),
      )
    const controller = new AbortController()
    const pending = uploadFiles({ ...request(), signal: controller.signal })
    await tick()
    expect(axios.post).toHaveBeenCalledTimes(2)
    controller.abort()
    expect(await pending).toMatchObject({
      status: 'completion_unknown',
      tasks: [],
      uncertainCompletion: {
        location: '/files/tus/session',
        filename: 'track.mp3',
        entityType: 'meditation',
        entityId: '1',
      },
    })
    resolve({ data: { tasks: [{ id: 9, status: 'completed' }] } })
    await tick()
    expect(axios.post).toHaveBeenCalledTimes(2)
    expect(vi.getTimerCount()).toBe(0)
  })
  it('reconciles a lost response after server commit using only the original completion session', async () => {
    const committed = { tasks: [{ id: 9, status: 'queued' }] }
    vi.mocked(axios.post)
      .mockReset()
      .mockResolvedValueOnce({ headers: { location: '/files/tus/session' } })
      .mockImplementationOnce(() => new Promise(() => {})) // Server committed; response lost.
      .mockResolvedValue({ data: committed })
    const onCompletionSession = vi.fn()
    const pending = uploadFiles({ ...request(), onCompletionSession })
    await tick()
    expect(onCompletionSession).toHaveBeenCalledWith(
      expect.objectContaining({ location: '/files/tus/session', entityId: '1' }),
    )
    await vi.advanceTimersByTimeAsync(1001)
    const uncertain = await pending
    expect(uncertain.status).toBe('completion_unknown')
    expect(uncertain.error).not.toContain('try again')
    const recovered = await reconcileUploadCompletion(uncertain.uncertainCompletion!)
    expect(recovered.tasks).toMatchObject([{ id: 9, status: 'queued' }])
    expect(recovered.uncertainCompletion).toBeUndefined()
    expect(vi.mocked(axios.post).mock.calls.map((call) => call[0])).toEqual([
      '/files/tus',
      '/files/tus/session/complete',
      '/files/tus/session/complete',
    ])
    expect(axios.head).toHaveBeenCalledTimes(1)
    expect(axios.patch).toHaveBeenCalledTimes(1)
  })
  it('retains the session when explicit completion reconciliation times out again', async () => {
    vi.mocked(axios.post)
      .mockReset()
      .mockImplementation(() => new Promise(() => {}))
    const completion = {
      location: '/files/tus/original',
      filename: 'track.mp3',
      entityType: 'meditation',
      entityId: 1,
    }
    const pending = reconcileUploadCompletion(completion, { requestTimeoutMs: 1000 })
    await tick()
    await vi.advanceTimersByTimeAsync(1001)
    expect(await pending).toMatchObject({
      status: 'completion_unknown',
      uncertainCompletion: completion,
      tasks: [],
    })
    expect(axios.post).toHaveBeenCalledTimes(1)
    expect(axios.head).not.toHaveBeenCalled()
    expect(axios.patch).not.toHaveBeenCalled()
  })
  it.each([
    null,
    {},
    { tasks: [] },
    { tasks: [{ id: 0, status: 'queued' }] },
    { tasks: [{ id: -1, status: 'queued' }] },
    { error: { message: 'Malformed error' } },
    { error: true },
  ])('retains completion uncertainty for malformed successful acknowledgment %j', async (data) => {
    vi.mocked(axios.post)
      .mockReset()
      .mockResolvedValueOnce({ headers: { location: '/files/tus/session' } })
      .mockResolvedValue({ data })
    const initial = await uploadFiles(request())
    expect(initial.status).toBe('completion_unknown')
    const recovered = await reconcileUploadCompletion(initial.uncertainCompletion!)
    expect(recovered).toMatchObject({
      status: 'completion_unknown',
      uncertainCompletion: initial.uncertainCompletion,
      tasks: [],
    })
    expect(axios.patch).toHaveBeenCalledTimes(1)
  })
  it('distinguishes a definite completion rejection from an unknown response', async () => {
    vi.mocked(axios.post)
      .mockReset()
      .mockResolvedValueOnce({ headers: { location: '/files/tus/session' } })
      .mockRejectedValue({ response: { status: 400, data: { error: 'Invalid media' } } })
    vi.mocked(axios.isAxiosError).mockReturnValue(true)
    const result = await uploadFiles(request())
    expect(result.error).toBe('Invalid media')
    expect(result.uncertainCompletion).toBeUndefined()
  })
  it.each([400, 410])(
    'releases completion uncertainty after a definitive HTTP %i rejection',
    async (status) => {
      vi.mocked(axios.post)
        .mockReset()
        .mockRejectedValue({
          response: {
            status,
            data: {
              status: 'error',
              error:
                status === 410 ? 'completed upload is no longer available' : 'Completion rejected',
            },
          },
        })
      vi.mocked(axios.isAxiosError).mockReturnValue(true)

      expect(await reconcileUploadCompletion(completion())).toEqual({
        status: 'error',
        tasks: [],
        error: status === 410 ? 'completed upload is no longer available' : 'Completion rejected',
      })
      expect(axios.post).toHaveBeenCalledExactlyOnceWith(
        '/files/tus/original/complete',
        {},
        expect.any(Object),
      )
      expect(axios.head).not.toHaveBeenCalled()
      expect(axios.patch).not.toHaveBeenCalled()
      expect(vi.getTimerCount()).toBe(0)
    },
  )
  it.each([
    [
      { status: 'error', errors: { media: 'Invalid media', context: 'Invalid context' } },
      'Invalid media, Invalid context',
    ],
    [{ status: 'error', error: 'Invalid media' }, 'Invalid media'],
  ])('preserves rejection details from HTTP error payload %j', async (data, error) => {
    vi.mocked(axios.post)
      .mockReset()
      .mockRejectedValue({ response: { status: 400, data } })
    vi.mocked(axios.isAxiosError).mockReturnValue(true)
    expect(await reconcileUploadCompletion(completion())).toEqual({
      status: 'error',
      tasks: [],
      error,
    })
  })
  it.each([undefined, 'error', 'queued'])(
    'normalizes a structured rejection with status %s into a terminal error',
    async (status) => {
      vi.mocked(axios.post)
        .mockReset()
        .mockResolvedValueOnce({ headers: { location: '/files/tus/session' } })
        .mockResolvedValue({ data: { status, error: 'Invalid media' } })
      const expected = { status: 'error', tasks: [], error: 'Invalid media' }
      expect(await uploadFiles(request())).toEqual(expected)
      expect(await reconcileUploadCompletion(completion())).toEqual(expected)
      expect(axios.post).toHaveBeenCalledTimes(3)
      expect(axios.head).toHaveBeenCalledTimes(1)
      expect(axios.patch).toHaveBeenCalledTimes(1)
    },
  )
  it.each([0, 401, 403, 404, 408, 409, 412, 422, 429, 500, 503])(
    'retains completion uncertainty after HTTP %i even with an error payload',
    async (status) => {
      vi.mocked(axios.post)
        .mockReset()
        .mockRejectedValue({ response: { status, data: { error: 'Completion interrupted' } } })
      vi.mocked(axios.isAxiosError).mockReturnValue(true)
      const original = completion()
      expect(await reconcileUploadCompletion(original)).toMatchObject({
        status: 'completion_unknown',
        tasks: [],
        uncertainCompletion: original,
      })
      expect(axios.post).toHaveBeenCalledTimes(1)
      expect(axios.head).not.toHaveBeenCalled()
      expect(axios.patch).not.toHaveBeenCalled()
    },
  )
  it.each([
    {
      status: 400,
      data: { status: 400, code: 'invalid_browser_credentials', message: 'Sign in again' },
    },
    {
      status: 400,
      data: { status: 'error', error: 'Sign in again' },
      headers: { 'x-goadmin-auth-error': 'invalid_browser_credentials' },
    },
    { status: 400, data: { error: 'Unrecognized rejection' } },
    { status: 400, data: { status: 'error', errors: ['Malformed validation'] } },
    { status: 410, data: { error: 'Unknown expired resource' } },
  ])('retains uncertainty for unrecognized or authentication rejection %j', async (response) => {
    vi.mocked(axios.post).mockReset().mockRejectedValue({ response })
    vi.mocked(axios.isAxiosError).mockReturnValue(true)
    const original = completion()
    expect(await reconcileUploadCompletion(original)).toMatchObject({
      status: 'completion_unknown',
      uncertainCompletion: original,
    })
  })
  it('retains completion uncertainty after a replay network failure', async () => {
    vi.mocked(axios.post).mockReset().mockRejectedValue(new Error('Network error'))
    const original = completion()
    expect(await reconcileUploadCompletion(original)).toMatchObject({
      status: 'completion_unknown',
      tasks: [],
      uncertainCompletion: original,
    })
    expect(axios.post).toHaveBeenCalledTimes(1)
  })
  it.each(['before replay', 'before dispatch', 'after dispatch'])(
    'retains the earlier uncertainty when cancellation happens %s',
    async (stage) => {
      vi.mocked(axios.post)
        .mockReset()
        .mockImplementation(() => new Promise(() => {}))
      const controller = new AbortController()
      const original = completion()
      if (stage === 'before replay') controller.abort()
      const pending = reconcileUploadCompletion(original, { signal: controller.signal })
      if (stage === 'after dispatch') await tick()
      controller.abort()
      expect(await pending).toMatchObject({
        status: 'completion_unknown',
        tasks: [],
        uncertainCompletion: original,
      })
      expect(axios.post).toHaveBeenCalledTimes(stage === 'after dispatch' ? 1 : 0)
      expect(vi.getTimerCount()).toBe(0)
    },
  )
  it.each([0, -1, NaN, Infinity])(
    'retains the earlier uncertainty when an invalid timeout %s prevents replay',
    async (requestTimeoutMs) => {
      const original = completion()
      expect(await reconcileUploadCompletion(original, { requestTimeoutMs })).toMatchObject({
        status: 'completion_unknown',
        tasks: [],
        uncertainCompletion: original,
      })
      expect(axios.post).not.toHaveBeenCalled()
      expect(vi.getTimerCount()).toBe(0)
    },
  )
  it('does not dispatch completion when the shared scope already has an unresolved session', async () => {
    const onCompletionSession = () => {
      throw new Error('Check the unconfirmed upload before starting another.')
    }
    const result = await uploadFiles({ ...request(), onCompletionSession })
    expect(result.error).toContain('Check the unconfirmed upload')
    expect(axios.post).toHaveBeenCalledTimes(1)
    expect(vi.mocked(axios.post).mock.calls[0][0]).toBe('/files/tus')
  })
  it('does not start a pre-cancelled upload', async () => {
    const controller = new AbortController()
    controller.abort()
    expect((await uploadFiles({ ...request(), signal: controller.signal })).error).toBe(
      'Upload cancelled',
    )
    expect(axios.post).not.toHaveBeenCalled()
  })
  it.each([undefined, '0', '-1', 'NaN', '99'])(
    'rejects invalid/nonadvancing chunk offset %s',
    async (offset) => {
      vi.mocked(axios.patch).mockResolvedValue({
        status: 204,
        headers: { 'upload-offset': offset },
      })
      expect((await uploadFiles(request())).error).toMatch(/offset/i)
      expect(axios.post).toHaveBeenCalledTimes(1)
    },
  )
  it('bounds repeated offset conflicts', async () => {
    vi.mocked(axios.patch).mockResolvedValue({ status: 409, headers: {} })
    expect((await uploadFiles(request())).error).toContain('conflict')
    expect(axios.patch).toHaveBeenCalledTimes(4)
  })
  it.each(['files', 'pending'])(
    'rejects a backwards recovery offset in %s uploads',
    async (kind) => {
      const large = new File([new Uint8Array(11 * 1024 * 1024)], 'large.mp3')
      vi.mocked(axios.patch)
        .mockResolvedValueOnce({
          status: 204,
          headers: { 'upload-offset': String(5 * 1024 * 1024) },
        })
        .mockResolvedValue({ status: 409, headers: {} })
      const failure =
        kind === 'files'
          ? uploadFiles({ ...request(), file: large }).then((result) => result.error)
          : uploadPendingWithTus({
              file: large,
              uploadUrl: '/files/tus',
              fileCategory: 'file',
            }).then(
              () => undefined,
              (error) => error.message,
            )
      expect(await failure).toContain('backwards')
      expect(axios.patch).toHaveBeenCalledTimes(2)
    },
  )
  it('reports acknowledged progress after every chunk of a >15 MiB upload', async () => {
    const size = 16 * 1024 * 1024
    const large = new File([new Uint8Array(size)], 'large.mp3')
    vi.mocked(axios.patch).mockImplementation(async (_url, blob, config) => ({
      status: 204,
      headers: {
        'upload-offset': String(Number(config!.headers!['Upload-Offset']) + (blob as Blob).size),
      },
    }))
    const onProgress = vi.fn()
    expect((await uploadFiles({ ...request(), file: large, onProgress })).error).toBeUndefined()
    expect(onProgress.mock.calls).toEqual([[0], [31], [62], [93], [100]])
  })
  it('applies cancellation to pending-domain uploads too', async () => {
    vi.mocked(axios.patch).mockImplementation(() => new Promise(() => {}))
    const controller = new AbortController()
    const pending = uploadPendingWithTus({
      file: file(),
      uploadUrl: '/files/tus',
      fileCategory: 'file',
      signal: controller.signal,
    })
    const failure = pending.catch((error) => error)
    await tick()
    controller.abort()
    expect(await failure).toMatchObject({ message: 'Upload cancelled' })
    expect(axios.post).toHaveBeenCalledTimes(1)
  })
})
