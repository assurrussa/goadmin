import { afterEach, describe, expect, it, vi } from 'vitest'
import axios from 'axios'
import { fetchUploadTask, uploadFiles, type UploadRequest } from '../fileUploadService'

vi.mock('axios', () => ({
  default: {
    get: vi.fn(),
    post: vi.fn(),
    head: vi.fn(),
    patch: vi.fn(),
    isAxiosError: vi.fn(() => false),
  },
}))
afterEach(() => vi.resetAllMocks())

describe('canonical audio upload transport', () => {
  it('retains audio metadata and final URL in task status', async () => {
    vi.mocked(axios.get).mockResolvedValue({
      data: {
        file: {
          id: 42,
          status: 'completed',
          filename: 'track.mp3',
          originalName: 'track.mp3',
          fileType: '7',
          mimeType: 'audio/mpeg',
          url: '/uploads/media/v1/track.mp3',
        },
      },
    })

    const result = await fetchUploadTask(42)

    expect(axios.get).toHaveBeenCalledWith('/files/tasks/42', { signal: undefined })
    expect(result).toMatchObject({
      status: 'completed',
      task: { id: 42, fileType: '7', mimeType: 'audio/mpeg' },
      file: { fileType: '7', publicUrl: '/uploads/media/v1/track.mp3' },
    })
  })

  it.each([
    ['track.mp3', 'audio/mpeg'],
    ['track.wav', 'audio/wav'],
  ])('uploads %s through the complete files pipeline', async (filename, mimeType) => {
    const file = new File(['audio bytes'], filename, { type: mimeType })
    const location = '/files/tus/audio-session'
    vi.mocked(axios.post)
      .mockResolvedValueOnce({ headers: { location } })
      .mockResolvedValueOnce({
        data: {
          file: {
            id: 42,
            status: 'queued',
            filename,
            originalName: filename,
            fileType: '7',
            mimeType,
            url: '',
          },
        },
      })
    vi.mocked(axios.head).mockResolvedValue({ headers: { 'upload-offset': '0' } })
    vi.mocked(axios.patch).mockResolvedValue({
      status: 204,
      headers: { 'upload-offset': String(file.size) },
    })
    const request: UploadRequest = {
      file,
      entityType: 'meditation',
      entityId: 17,
      context: 'meditation-audio',
      fileCategory: 'audio',
    }

    const result = await uploadFiles(request)

    const metadata = vi.mocked(axios.post).mock.calls[0]?.[2]?.headers?.['Upload-Metadata']
    expect(metadata).toContain(`file_type ${btoa('audio')}`)
    expect(metadata).toContain(`context ${btoa('meditation-audio')}`)
    expect(metadata).toContain(`entity_type ${btoa('meditation')}`)
    expect(metadata).toContain(`entity_id ${btoa('17')}`)
    expect(axios.post).toHaveBeenNthCalledWith(1, '/files/tus', {}, expect.any(Object))
    expect(axios.patch).toHaveBeenCalledWith(location, expect.any(Blob), {
      headers: {
        'Tus-Resumable': '1.0.0',
        'Upload-Offset': '0',
        'Content-Type': 'application/offset+octet-stream',
      },
    })
    expect(axios.post).toHaveBeenNthCalledWith(
      2,
      `${location}/complete`,
      {},
      {
        headers: { 'Tus-Resumable': '1.0.0' },
      },
    )
    expect(result.error).toBeUndefined()
    expect(result.tasks[0]).toMatchObject({ id: 42, status: 'queued', fileType: '7', mimeType })
  })
})
