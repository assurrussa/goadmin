import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { setAdminCapabilities } from './useAdminCapabilities'
import { FILE_AFTER_PROCESS_EVENT, useAdminWebSocket } from './useAdminWebSocket'

const sockets: FakeSocket[] = []
class FakeSocket {
  static OPEN = 1
  readyState = 0
  onopen: (() => void) | null = null
  onclose: (() => void) | null = null
  onmessage: ((event: { data: string }) => void) | null = null
  close = vi.fn()
  constructor() {
    sockets.push(this)
  }
}

beforeEach(() => {
  sockets.length = 0
  vi.useFakeTimers()
  vi.stubGlobal('WebSocket', FakeSocket)
})
afterEach(async () => {
  useAdminWebSocket().disconnect()
  setAdminCapabilities({ props: {} })
  await nextTick()
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

describe('realtime capability', () => {
  it('dispatches plain JSON text without changing the upload event schema', () => {
    setAdminCapabilities({ props: { adminCapabilities: { realtime: true } } })
    const websocket = useAdminWebSocket()
    const handler = vi.fn()
    const dispose = websocket.on(FILE_AFTER_PROCESS_EVENT, handler)
    const event = {
      id: 'e7366c15-61b2-4f68-b97a-f6cdd951b11c',
      eventType: FILE_AFTER_PROCESS_EVENT,
      fileId: 42,
      filePath: '/uploads/фото.png',
      status: 'completed',
      eventTrigger: 'avatar',
    }
    websocket.ensureConnected()
    sockets[0].onmessage?.({ data: JSON.stringify(event) })
    expect(handler).toHaveBeenCalledExactlyOnceWith(event)
    dispose()
  })

  it('never opens a socket when realtime is disabled', () => {
    setAdminCapabilities({ props: { adminCapabilities: {} } })
    useAdminWebSocket().ensureConnected()
    vi.advanceTimersByTime(60_000)
    expect(sockets).toHaveLength(0)
  })

  it('cancels a connecting socket and rejects a late open after disabling', async () => {
    setAdminCapabilities({ props: { adminCapabilities: { realtime: true } } })
    const websocket = useAdminWebSocket()
    websocket.ensureConnected()
    expect(sockets).toHaveLength(1)
    setAdminCapabilities({ props: { adminCapabilities: {} } })
    await nextTick()
    expect(sockets[0].close).toHaveBeenCalled()
    sockets[0].onopen?.()
    sockets[0].onclose?.()
    vi.advanceTimersByTime(60_000)
    expect(websocket.isConnected.value).toBe(false)
    expect(sockets).toHaveLength(1)
  })
})
