import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { setAdminCapabilities } from './useAdminCapabilities'
import { configureSessionMaintenance, sessionCoordinator, ensureSessionReady } from '@/http'
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
  configureSessionMaintenance({ props: {} }, {})
  sessionCoordinator.dispose()
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

  it('waits for session readiness before reconnecting and fences disconnect during that check', async () => {
    let resolve!: (response: Response) => void
    vi.stubGlobal(
      'fetch',
      vi.fn<typeof fetch>(
        () =>
          new Promise((done) => {
            resolve = done
          }),
      ),
    )
    setAdminCapabilities({ props: { adminCapabilities: { realtime: true } } })
    configureSessionMaintenance({
      props: { adminSessionMaintenance: { version: 1, sessionEpoch: 'one' } },
    })
    const websocket = useAdminWebSocket()
    websocket.ensureConnected()
    expect(sockets).toHaveLength(0)
    websocket.disconnect()
    const now = Date.now()
    resolve(
      new Response(
        JSON.stringify({
          status: 200,
          code: 'authentication_recovered',
          csrfReady: true,
          maintenance: {
            version: 1,
            serverTime: new Date(now).toISOString(),
            nextCheckAfterMs: 60_000,
            csrfExpiresAt: new Date(now + 300_000).toISOString(),
            hardExpiresAt: new Date(now + 3_600_000).toISOString(),
            sessionEpoch: 'one',
            revision: 1,
            renewed: false,
          },
        }),
        { headers: { 'Content-Type': 'application/json', 'X-Goadmin-Auth-Recovery': '1' } },
      ),
    )
    await ensureSessionReady()
    await vi.advanceTimersByTimeAsync(0)
    expect(sockets).toHaveLength(0)
    websocket.ensureConnected()
    await vi.advanceTimersByTimeAsync(0)
    expect(sockets).toHaveLength(1)
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
