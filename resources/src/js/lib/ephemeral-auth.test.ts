import { afterEach, describe, expect, it, vi } from 'vitest'
import { bindEphemeralAuthPage } from './ephemeral-auth'

describe('transient authentication lifecycle', () => {
 let dispose = () => {}
 afterEach(() => dispose())
 it('clears secrets on leaving and fences delayed results and repeated submissions', async () => {
  let secret = 'one-time'
  const send = vi.fn()
  const flow = bindEphemeralAuthPage(() => { secret = '' }, vi.fn())
  dispose = flow.dispose
  const submit = () => { if (flow.isActive()) send() }
  submit()
  window.dispatchEvent(new Event('pagehide'))
  expect(secret).toBe('')
  submit()
  await Promise.resolve()
  if (flow.isActive()) secret = 'late response'
  expect(send).toHaveBeenCalledTimes(1)
  expect(secret).toBe('')
 })
 it('reloads BFCache restoration without reviving the old request generation', () => {
  const reload = vi.fn()
  const flow = bindEphemeralAuthPage(vi.fn(), reload)
  dispose = flow.dispose
  window.dispatchEvent(new Event('pagehide'))
  const restored = new Event('pageshow')
  Object.defineProperty(restored, 'persisted', { value: true })
  window.dispatchEvent(restored)
  expect(reload).toHaveBeenCalledTimes(1)
  expect(flow.isActive()).toBe(false)
 })
 it('clears secrets and removes event handlers on unmount', () => {
  const clear = vi.fn()
  const reload = vi.fn()
  const flow = bindEphemeralAuthPage(clear, reload)
  flow.dispose()
  const restored = new Event('pageshow')
  Object.defineProperty(restored, 'persisted', { value: true })
  window.dispatchEvent(restored)
  expect(clear).toHaveBeenCalledTimes(1)
  expect(reload).not.toHaveBeenCalled()
  expect(flow.isActive()).toBe(false)
 })
})
