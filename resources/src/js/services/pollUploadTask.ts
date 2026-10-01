import { fetchUploadTask, type UploadTaskStatusResponse } from './fileUploadService'

interface PollOptions {
  signal: AbortSignal
  intervalMs?: number
  maxAttempts?: number
}

function delay(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    const abort = () => {
      clearTimeout(timer)
      reject(signal.reason ?? new DOMException('Upload status polling cancelled', 'AbortError'))
    }
    const timer = setTimeout(() => {
      signal.removeEventListener('abort', abort)
      resolve()
    }, ms)
    signal.addEventListener('abort', abort, { once: true })
    if (signal.aborted) abort()
  })
}

// Serial requests avoid overlaps; callers cancel on replacement or unmount.
export async function pollUploadTask(
  taskId: number,
  { signal, intervalMs = 1000, maxAttempts = 120 }: PollOptions,
): Promise<UploadTaskStatusResponse> {
  for (let attempt = 0; attempt < maxAttempts; attempt++) {
    signal.throwIfAborted()
    const response = await fetchUploadTask(taskId, signal)
    if (
      response.status === 'completed' ||
      response.status === 'failed' ||
      response.status === 'error'
    ) {
      return response
    }
    if (attempt + 1 < maxAttempts) await delay(intervalMs, signal)
  }
  throw new Error('Не удалось дождаться завершения обработки файла. Обновите страницу.')
}
