import { afterEach, describe, expect, it } from 'vitest'
import {
  hasAdminCapability,
  setAdminCapabilities,
  useAdminCapabilities,
} from './useAdminCapabilities'

afterEach(() => setAdminCapabilities({ props: {} }))

describe('admin capabilities', () => {
  it('preserves legacy hosts while an explicit empty map selects core only', () => {
    setAdminCapabilities({ props: {} })
    expect(hasAdminCapability('realtime')).toBe(true)
    expect(hasAdminCapability('authmail')).toBe(true)
    setAdminCapabilities({ props: { adminCapabilities: {} } })
    expect(hasAdminCapability('realtime')).toBe(false)
    expect(hasAdminCapability('uploads')).toBe(false)
    expect(hasAdminCapability('notifications')).toBe(false)
    expect(hasAdminCapability('authmail')).toBe(false)
  })

  it('updates reactive consumers and enables only explicitly true values', () => {
    const { uploads, realtime } = useAdminCapabilities()
    setAdminCapabilities({ props: { adminCapabilities: { uploads: true, realtime: false } } })
    expect(uploads.value).toBe(true)
    expect(realtime.value).toBe(false)
    setAdminCapabilities({ props: { adminCapabilities: { uploads: 'true' } } })
    expect(uploads.value).toBe(false)
  })
})
