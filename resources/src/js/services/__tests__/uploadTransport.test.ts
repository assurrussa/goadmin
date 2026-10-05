import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import axios from 'axios'
import { uploadFiles, uploadPendingWithTus } from '../fileUploadService'
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
      expect((await pending).error).toContain('timed out')
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
    expect(await pending).toMatchObject({ tasks: [], error: 'Upload cancelled' })
    resolve({ data: { tasks: [{ id: 9, status: 'completed' }] } })
    await tick()
    expect(axios.post).toHaveBeenCalledTimes(2)
    expect(vi.getTimerCount()).toBe(0)
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
