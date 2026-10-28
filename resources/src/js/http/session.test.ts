import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createSessionCoordinator, type SessionMaintenance } from './session'

const capability = { props: { adminSessionMaintenance: { version: 1, sessionEpoch: 'epoch-a' } } }
const base = Date.parse('2026-10-28T10:00:00Z')
function metadata(overrides: Partial<SessionMaintenance> = {}): SessionMaintenance {
  return {
    version: 1,
    serverTime: new Date(Date.now()).toISOString(),
    nextCheckAfterMs: 60_000,
    csrfExpiresAt: new Date(Date.now() + 300_000).toISOString(),
    hardExpiresAt: new Date(base + 3_600_000).toISOString(),
    sessionEpoch: 'epoch-a',
    revision: 1,
    renewed: false,
    ...overrides,
  }
}
function reply(status = 200, code = 'authentication_recovered', maintenance = metadata()) {
  return new Response(JSON.stringify({ status, code, csrfReady: true, maintenance }), {
    status,
    headers: { 'Content-Type': 'application/json', 'X-Goadmin-Auth-Recovery': '1' },
  })
}
const coordinators: ReturnType<typeof createSessionCoordinator>[] = []
function coordinator(request = vi.fn<typeof fetch>().mockImplementation(async () => reply())) {
  const onState = vi.fn()
  const instance = createSessionCoordinator({ request, onState })
  coordinators.push(instance)
  return { instance, request, onState }
}
beforeEach(() => {
  vi.useFakeTimers()
  vi.setSystemTime(base)
})
afterEach(() => {
  for (const instance of coordinators.splice(0)) instance.dispose()
  vi.useRealTimers()
})

describe('versioned session maintenance coordinator', () => {
  it('does nothing without the exact initial capability and never falls back', async () => {
    const { instance, request } = coordinator()
    instance.start()
    instance.configure({ props: { adminSessionMaintenance: { version: 2 } } })
    await instance.ensure()
    window.dispatchEvent(new Event('focus'))
    await vi.advanceTimersByTimeAsync(180_000)
    expect(request).not.toHaveBeenCalled()
    expect(instance.getState()).toBe('disabled')
  })

  it.each([undefined, '', ' ', 123])(
    'fails closed for advertised v1 with unusable document epoch %s',
    async (sessionEpoch) => {
      const { instance, request } = coordinator()
      instance.start()
      instance.configure({ props: { adminSessionMaintenance: { version: 1, sessionEpoch } } })
      await expect(instance.ensure({ force: true })).rejects.toMatchObject({
        code: 'authentication_unavailable',
      })
      window.dispatchEvent(new Event('focus'))
      await vi.advanceTimersByTimeAsync(120_000)
      expect(instance.isEnabled()).toBe(true)
      expect(instance.getState()).toBe('authentication_unavailable')
      expect(request).not.toHaveBeenCalled()
      expect(instance.getMaintenance()).toBeNull()
    },
  )

  it('retains the watchdog after navigation cancels commands during a periodic check', async () => {
    let finish!: (response: Response) => void
    const request = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(reply())
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            finish = resolve
          }),
      )
      .mockImplementation(async () => reply())
    const { instance } = coordinator(request)
    instance.configure(capability)
    await instance.ensure()
    await vi.advanceTimersByTimeAsync(60_000)
    expect(request).toHaveBeenCalledTimes(2)
    const cancelled = expect(instance.ensure()).rejects.toMatchObject({ code: 'cancelled' })
    instance.cancel()
    instance.configure(capability)
    await cancelled
    finish(reply())
    await vi.advanceTimersByTimeAsync(0)
    expect(instance.getState()).toBe('ready')
    await vi.advanceTimersByTimeAsync(60_000)
    expect(request).toHaveBeenCalledTimes(3)
    expect(instance.getState()).toBe('ready')
  })

  it('does not resume a terminal watchdog when command cancellation is followed by navigation', async () => {
    const { instance, request } = coordinator()
    instance.configure(capability)
    await instance.ensure()
    instance.reportFailure('reauthentication_required')
    instance.cancel()
    instance.configure(capability)
    await vi.advanceTimersByTimeAsync(180_000)
    expect(instance.getState()).toBe('reauthentication_required')
    expect(request).toHaveBeenCalledTimes(1)
  })

  it('coalesces startup, commands and focus/visible/pageshow/online into one in-flight GET', async () => {
    let resolve!: (response: Response) => void
    const request = vi.fn<typeof fetch>(
      () =>
        new Promise((done) => {
          resolve = done
        }),
    )
    const { instance } = coordinator(request)
    instance.start()
    instance.configure(capability)
    const command = instance.ensure()
    for (const event of ['focus', 'pageshow', 'online']) window.dispatchEvent(new Event(event))
    document.dispatchEvent(new Event('visibilitychange'))
    expect(request).toHaveBeenCalledTimes(1)
    resolve(reply())
    await command
    expect(request).toHaveBeenCalledWith(
      '/auth/session/maintain',
      expect.objectContaining({
        method: 'GET',
        credentials: 'same-origin',
        mode: 'same-origin',
        cache: 'no-store',
        redirect: 'error',
        headers: { Accept: 'application/json', 'X-Goadmin-Auth-Recovery': '1' },
      }),
    )
    expect(instance.getState()).toBe('ready')
    window.dispatchEvent(new Event('focus'))
    await instance.ensure()
    expect(request).toHaveBeenCalledTimes(1)
  })

  it('caps visible heartbeats at 60 seconds and hidden at 120 seconds', async () => {
    const hidden = vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
    const { instance, request } = coordinator(
      vi
        .fn<typeof fetch>()
        .mockImplementation(async () =>
          reply(200, 'authentication_recovered', metadata({ nextCheckAfterMs: 600_000 })),
        ),
    )
    instance.configure(capability)
    await instance.ensure()
    await vi.advanceTimersByTimeAsync(60_000)
    expect(request).toHaveBeenCalledTimes(2)
    hidden.mockReturnValue(true)
    await instance.ensure({ force: true })
    await vi.advanceTimersByTimeAsync(119_999)
    expect(request).toHaveBeenCalledTimes(3)
    await vi.advanceTimersByTimeAsync(1)
    expect(request).toHaveBeenCalledTimes(4)
    hidden.mockRestore()
  })

  it('uses server-relative deadlines despite client clock skew', async () => {
    const { instance, request } = coordinator(
      vi.fn<typeof fetch>().mockResolvedValue(
        reply(
          200,
          'authentication_recovered',
          metadata({
            serverTime: '2026-01-01T00:00:00Z',
            csrfExpiresAt: '2026-01-01T00:00:15Z',
            hardExpiresAt: '2026-01-01T01:00:00Z',
          }),
        ),
      ),
    )
    instance.configure(capability)
    await instance.ensure()
    await vi.advanceTimersByTimeAsync(13_999)
    expect(request).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(1)
    expect(request).toHaveBeenCalledTimes(2)
  })

  it('honors a server check deadline even when a delayed timer leaves the cache younger than 30 seconds', async () => {
    const { instance, request } = coordinator(
      vi
        .fn<typeof fetch>()
        .mockImplementation(async () =>
          reply(200, 'authentication_recovered', metadata({ nextCheckAfterMs: 1000 })),
        ),
    )
    instance.configure(capability)
    await instance.ensure()
    vi.setSystemTime(base + 2000)
    await instance.ensure()
    expect(request).toHaveBeenCalledTimes(2)
  })

  it.each(['csrfExpiresAt', 'externalValidUntil'] as const)(
    'checks an imminent %s before sending despite a delayed timer',
    async (deadline) => {
      const { instance, request } = coordinator(
        vi
          .fn<typeof fetch>()
          .mockImplementation(async () =>
            reply(
              200,
              'authentication_recovered',
              metadata({ [deadline]: new Date(Date.now() + 5000).toISOString() }),
            ),
          ),
      )
      instance.configure(capability)
      await instance.ensure()
      vi.setSystemTime(base + 4500)
      await instance.ensure()
      expect(request).toHaveBeenCalledTimes(2)
    },
  )

  it('subtracts the whole request elapsed time from maintenance and hard-expiry deadlines', async () => {
    let resolve!: (response: Response) => void
    const delayed = reply(
      200,
      'authentication_recovered',
      metadata({ nextCheckAfterMs: 5000, hardExpiresAt: new Date(base + 6000).toISOString() }),
    )
    const { instance, request } = coordinator(
      vi.fn<typeof fetch>(
        () =>
          new Promise((done) => {
            resolve = done
          }),
      ),
    )
    instance.configure(capability)
    const ready = instance.ensure()
    vi.setSystemTime(base + 3000)
    resolve(delayed)
    await ready
    // The request began at t=0, so the hard deadline remains t=6, not t=9.
    vi.setSystemTime(base + 6000)
    await expect(instance.ensure()).rejects.toMatchObject({ code: 'reauthentication_required' })
    expect(request).toHaveBeenCalledTimes(1)
  })

  it('subtracts request elapsed time from the next scheduled check as well', async () => {
    let resolve!: (response: Response) => void
    const delayed = reply(200, 'authentication_recovered', metadata({ nextCheckAfterMs: 5000 }))
    const request = vi
      .fn<typeof fetch>()
      .mockImplementationOnce(
        () =>
          new Promise((done) => {
            resolve = done
          }),
      )
      .mockImplementation(async () => reply())
    const { instance } = coordinator(request)
    instance.configure(capability)
    const ready = instance.ensure()
    await vi.advanceTimersByTimeAsync(3000)
    resolve(delayed)
    await ready
    await vi.advanceTimersByTimeAsync(1999)
    expect(request).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(1)
    expect(request).toHaveBeenCalledTimes(2)
  })

  it('backs off repeated failures at 1, 2, 5, 10 and 30 seconds and resets after success', async () => {
    const request = vi
      .fn<typeof fetch>()
      .mockImplementation(async () => reply(503, 'authentication_unavailable'))
    const { instance } = coordinator(request)
    instance.configure(capability)
    await expect(instance.ensure()).rejects.toMatchObject({ code: 'authentication_unavailable' })
    for (const [index, delay] of [1000, 2000, 5000, 10000, 30000].entries()) {
      await vi.advanceTimersByTimeAsync(delay - 1)
      expect(request).toHaveBeenCalledTimes(index + 1)
      await vi.advanceTimersByTimeAsync(1)
      expect(request).toHaveBeenCalledTimes(index + 2)
    }
    request.mockImplementation(async () => reply())
    await instance.ensure({ force: true })
    request.mockImplementation(async () => reply(503, 'authentication_unavailable'))
    await expect(instance.ensure({ force: true })).rejects.toMatchObject({
      code: 'authentication_unavailable',
    })
    const count = request.mock.calls.length
    await vi.advanceTimersByTimeAsync(999)
    expect(request).toHaveBeenCalledTimes(count)
    await vi.advanceTimersByTimeAsync(1)
    expect(request).toHaveBeenCalledTimes(count + 1)
  })

  it.each([
    ['0', 1000],
    ['90', 90000],
    ['999999', 120000],
  ] as const)(
    'clamps Retry-After %s while suppressing ordinary foreground probes',
    async (hint, delay) => {
      const unavailable = () => {
        const response = reply(503, 'authentication_unavailable')
        response.headers.set('Retry-After', hint)
        return response
      }
      const { instance, request } = coordinator(
        vi.fn<typeof fetch>().mockImplementation(async () => unavailable()),
      )
      instance.start()
      instance.configure(capability)
      await expect(instance.ensure()).rejects.toMatchObject({ code: 'authentication_unavailable' })
      window.dispatchEvent(new Event('focus'))
      await expect(instance.ensure()).rejects.toMatchObject({ code: 'authentication_unavailable' })
      expect(request).toHaveBeenCalledTimes(1)
      await vi.advanceTimersByTimeAsync(delay - 1)
      expect(request).toHaveBeenCalledTimes(1)
      await vi.advanceTimersByTimeAsync(1)
      expect(request).toHaveBeenCalledTimes(2)
    },
  )

  it('limits outage Retry-After by a still-valid credential deadline', async () => {
    const request = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(
        reply(
          200,
          'authentication_recovered',
          metadata({ csrfExpiresAt: new Date(base + 5000).toISOString() }),
        ),
      )
      .mockImplementation(async () => {
        const response = reply(503, 'authentication_unavailable')
        response.headers.set('Retry-After', '120')
        return response
      })
    const { instance } = coordinator(request)
    instance.configure(capability)
    await instance.ensure()
    await expect(instance.ensure({ force: true })).rejects.toMatchObject({
      code: 'authentication_unavailable',
    })
    await vi.advanceTimersByTimeAsync(3999)
    expect(request).toHaveBeenCalledTimes(2)
    await vi.advanceTimersByTimeAsync(1)
    expect(request).toHaveBeenCalledTimes(3)
  })

  it('accepts one continuation GET, and refuses an endless 409 loop', async () => {
    const { instance, request } = coordinator(
      vi
        .fn<typeof fetch>()
        .mockResolvedValueOnce(reply(409, 'authentication_recovery_continue'))
        .mockResolvedValueOnce(reply()),
    )
    instance.configure(capability)
    await instance.ensure()
    expect(request).toHaveBeenCalledTimes(2)
    request.mockResolvedValue(reply(409, 'authentication_recovery_continue'))
    await expect(instance.ensure({ force: true })).rejects.toMatchObject({
      code: 'authentication_unavailable',
    })
    expect(request).toHaveBeenCalledTimes(4)
  })

  for (const [status, code] of [
    [401, 'reauthentication_required'],
    [403, 'access_denied'],
    [503, 'authentication_unavailable'],
  ] as const) {
    it(`retains the typed ${status} result`, async () => {
      const { instance, request } = coordinator(
        vi.fn<typeof fetch>().mockResolvedValue(reply(status, code)),
      )
      instance.configure(capability)
      await expect(instance.ensure()).rejects.toMatchObject({ code })
      expect(instance.getState()).toBe(code)
      if (status !== 503) {
        await expect(instance.ensure()).rejects.toMatchObject({ code })
        expect(request).toHaveBeenCalledTimes(1)
      }
    })
  }

  it('bounds an unresponsive foreground check and discards late success', async () => {
    let resolve!: (response: Response) => void
    const { instance } = coordinator(
      vi.fn<typeof fetch>(
        () =>
          new Promise((done) => {
            resolve = done
          }),
      ),
    )
    instance.configure(capability)
    const command = expect(instance.ensure()).rejects.toMatchObject({
      code: 'authentication_unavailable',
    })
    await vi.advanceTimersByTimeAsync(10_000)
    await command
    resolve(reply())
    await vi.advanceTimersByTimeAsync(0)
    expect(instance.getState()).toBe('authentication_unavailable')
    expect(instance.getMaintenance()).toBeNull()
  })

  it('rejects a cancelled command while allowing the shared check to finish', async () => {
    let resolve!: (response: Response) => void
    const { instance } = coordinator(
      vi.fn<typeof fetch>(
        () =>
          new Promise((done) => {
            resolve = done
          }),
      ),
    )
    instance.configure(capability)
    const abort = new AbortController()
    const command = expect(instance.ensure({ signal: abort.signal })).rejects.toMatchObject({
      code: 'cancelled',
    })
    abort.abort()
    await command
    resolve(reply())
    await instance.ensure()
    expect(instance.getState()).toBe('ready')
  })

  it('logout/cancellation fences a late response and promptly rejects pending continuations', async () => {
    let resolve!: (response: Response) => void
    const { instance } = coordinator(
      vi.fn<typeof fetch>(
        () =>
          new Promise((done) => {
            resolve = done
          }),
      ),
    )
    instance.configure(capability)
    const command = expect(instance.ensure()).rejects.toMatchObject({ code: 'cancelled' })
    instance.reportFailure('reauthentication_required')
    await command
    resolve(reply())
    await vi.advanceTimersByTimeAsync(0)
    expect(instance.getState()).toBe('reauthentication_required')
    expect(instance.getMaintenance()).toBeNull()
  })

  it('blocks all unsent continuations after an epoch change until a new document starts', async () => {
    const { instance, request } = coordinator()
    instance.configure(capability)
    await instance.ensure()
    request.mockResolvedValue(
      reply(200, 'authentication_recovered', metadata({ sessionEpoch: 'epoch-b', revision: 8 })),
    )
    await expect(instance.ensure({ force: true })).rejects.toMatchObject({
      code: 'session_changed',
    })
    expect(instance.getState()).toBe('session_changed')
    await expect(instance.ensure({ force: true })).rejects.toMatchObject({
      code: 'session_changed',
    })
    expect(request).toHaveBeenCalledTimes(2)
  })

  it('does not emit readiness or continue background maintenance after an epoch change', async () => {
    const request = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(reply())
      .mockImplementation(async () =>
        reply(200, 'authentication_recovered', metadata({ sessionEpoch: 'epoch-b' })),
      )
    const onReady = vi.fn()
    const instance = createSessionCoordinator({ request, onReady })
    coordinators.push(instance)
    instance.configure(capability)
    await instance.ensure()
    expect(onReady).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(60_000)
    expect(instance.getState()).toBe('session_changed')
    expect(onReady).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(120_000)
    expect(request).toHaveBeenCalledTimes(2)
    await expect(instance.ensure()).rejects.toMatchObject({ code: 'session_changed' })
  })

  it('rejects a decreasing revision within the same epoch', async () => {
    const { instance, request } = coordinator()
    instance.configure(capability)
    await instance.ensure()
    request.mockResolvedValue(reply(200, 'authentication_recovered', metadata({ revision: 8 })))
    await instance.ensure({ force: true })
    request.mockResolvedValue(reply(200, 'authentication_recovered', metadata({ revision: 7 })))
    await expect(instance.ensure({ force: true })).rejects.toMatchObject({
      code: 'authentication_unavailable',
    })
    expect(instance.getMaintenance()?.revision).toBe(8)
  })

  it('hard expiry requires explicit login without dispatching another admission check', async () => {
    const { instance, request } = coordinator(
      vi
        .fn<typeof fetch>()
        .mockResolvedValue(
          reply(
            200,
            'authentication_recovered',
            metadata({ hardExpiresAt: new Date(base + 2000).toISOString() }),
          ),
        ),
    )
    instance.configure(capability)
    await instance.ensure()
    vi.setSystemTime(base + 2000)
    await expect(instance.ensure()).rejects.toMatchObject({ code: 'reauthentication_required' })
    expect(request).toHaveBeenCalledTimes(1)
  })
})
