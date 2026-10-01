import { afterEach, describe, expect, it, vi } from 'vitest'
import axios from 'axios'
import { fetchUploadTask } from '../fileUploadService'

vi.mock('axios', () => ({ default: { get: vi.fn(), isAxiosError: vi.fn(() => false) } }))
afterEach(() => vi.mocked(axios.get).mockReset())

describe('protected upload task status', () => {
  it.each(['queued', 'processing', 'failed', 'completed'])(
    'reads file.status=%s from the server envelope',
    async (status) => {
      vi.mocked(axios.get).mockResolvedValue({
        data: {
          file: {
            id: 42,
            status,
            filename: 'avatar.png',
            originalName: 'avatar.png',
            fileType: 'image',
            url: status === 'completed' ? '/avatar.png' : '',
          },
        },
      })
      const signal = new AbortController().signal
      const result = await fetchUploadTask(42, signal)
      expect(result.status).toBe(status)
      expect(axios.get).toHaveBeenCalledWith('/files/tasks/42', { signal })
    },
  )
})
