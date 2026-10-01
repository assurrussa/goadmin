import { afterEach, describe, expect, it, vi } from 'vitest'
import { pollUploadTask } from '../pollUploadTask'
import { fetchUploadTask } from '../fileUploadService'

vi.mock('../fileUploadService', () => ({ fetchUploadTask: vi.fn() }))
const fetchTask = vi.mocked(fetchUploadTask)

afterEach(() => {
  vi.useRealTimers()
  fetchTask.mockReset()
})

describe('upload status polling without realtime', () => {
  it('polls serially until a completed task is available', async () => {
    vi.useFakeTimers()
    fetchTask.mockResolvedValueOnce({ status: 'queued', task: { id: 42, status: 'queued' } })
    fetchTask.mockResolvedValueOnce({ status: 'completed', task: { id: 42, status: 'completed' } })
    const signal = new AbortController().signal
    const result = pollUploadTask(42, { signal })
    expect(fetchTask).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(1000)
    expect((await result).status).toBe('completed')
    expect(fetchTask).toHaveBeenCalledTimes(2)
    expect(fetchTask).toHaveBeenLastCalledWith(42, signal)
  })

  it('stops immediately when the owning component cancels', async () => {
    vi.useFakeTimers()
    fetchTask.mockResolvedValue({ status: 'processing', task: { id: 42, status: 'processing' } })
    const controller = new AbortController()
    const result = pollUploadTask(42, { signal: controller.signal })
    const rejection = result.catch((reason: unknown) => reason)
    await Promise.resolve()
    controller.abort()
    expect(await rejection).toMatchObject({ name: 'AbortError' })
    await vi.advanceTimersByTimeAsync(60_000)
    expect(fetchTask).toHaveBeenCalledTimes(1)
  })

  it('bounds polling when a task never finishes', async () => {
    fetchTask.mockResolvedValue({ status: 'queued', task: { id: 42, status: 'queued' } })
    await expect(
      pollUploadTask(42, { signal: new AbortController().signal, maxAttempts: 1 }),
    ).rejects.toThrow('Не удалось дождаться')
    expect(fetchTask).toHaveBeenCalledTimes(1)
  })
})
