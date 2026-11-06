import { afterEach, expect, it, vi } from 'vitest'
import {
  clearBrandingRecovery,
  pauseBrandingRecovery,
  registerBrandingRecovery,
  takeBrandingRecovery,
  validateBrandingRecovery,
} from './brandingRecovery'

const snapshot = () => ({
  owner: '1',
  origin: location.origin,
  path: '/auth/profile/settings',
  created: Date.now(),
  version: 1,
  name: 'Name',
  accentColor: '#123456',
  baseline: { name: '', accentColor: '' },
})
afterEach(() => {
  clearBrandingRecovery()
  vi.restoreAllMocks()
  vi.useRealTimers()
})
it('is one-use and expires after 30 minutes', () => {
  vi.useFakeTimers()
  registerBrandingRecovery(snapshot)
  pauseBrandingRecovery()
  expect(takeBrandingRecovery('1')?.accentColor).toBe('#123456')
  expect(takeBrandingRecovery('1')).toBeUndefined()
  registerBrandingRecovery(snapshot)
  pauseBrandingRecovery()
  vi.advanceTimersByTime(30 * 60 * 1000 + 1)
  expect(takeBrandingRecovery('1')).toBeUndefined()
})
it('discards on owner or permission change and on explicit clear', () => {
  registerBrandingRecovery(snapshot)
  pauseBrandingRecovery()
  validateBrandingRecovery('2', true)
  expect(sessionStorage.length).toBe(0)
  registerBrandingRecovery(snapshot)
  pauseBrandingRecovery()
  validateBrandingRecovery('1', false)
  expect(sessionStorage.length).toBe(0)
  registerBrandingRecovery(snapshot)
  pauseBrandingRecovery()
  clearBrandingRecovery()
  pauseBrandingRecovery()
  expect(sessionStorage.length).toBe(0)
})
it('does not throw when tab storage is unavailable', () => {
  registerBrandingRecovery(snapshot)
  vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
    throw new Error('denied')
  })
  expect(() => pauseBrandingRecovery()).not.toThrow()
  expect(takeBrandingRecovery('1')).toBeUndefined()
})

it('recovers both gradient endpoints and preserves an explicit unresolved conflict', () => {
  registerBrandingRecovery(() => ({
    ...snapshot(),
    accentColorEnd: '#abcdef',
    conflict: true,
    baseline: { name: '', accentColor: '#234567', accentColorEnd: '#345678' },
  }))
  pauseBrandingRecovery()
  expect(takeBrandingRecovery('1')).toMatchObject({
    accentColor: '#123456',
    accentColorEnd: '#abcdef',
    conflict: true,
    baseline: { accentColorEnd: '#345678' },
  })
})
it('normalizes legacy recovery to solid and rejects malformed pairs atomically', () => {
  registerBrandingRecovery(snapshot)
  pauseBrandingRecovery()
  expect(takeBrandingRecovery('1')).toMatchObject({
    accentColorEnd: '',
    baseline: { accentColorEnd: '' },
  })
  registerBrandingRecovery(() => ({ ...snapshot(), accentColor: '', accentColorEnd: '#abcdef' }))
  pauseBrandingRecovery()
  expect(takeBrandingRecovery('1')).toBeUndefined()
})
