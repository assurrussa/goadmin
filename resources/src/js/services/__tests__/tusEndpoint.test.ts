import { describe, expect, it } from 'vitest'
import { resolveTusEndpoint } from '../tusEndpoint'

describe('resolveTusEndpoint', () => {
  it('keeps an explicit CMS transport prefix', () => {
    expect(resolveTusEndpoint('/admin/api/cms/v1/uploads')).toBe('/admin/api/cms/v1/uploads/tus')
  })

  it('uses the generic files endpoint only when no prefix was supplied', () => {
    expect(resolveTusEndpoint()).toBe('/files/tus')
  })

  it('does not append a second tus segment', () => {
    expect(resolveTusEndpoint('/custom/uploads/tus')).toBe('/custom/uploads/tus')
  })

  it('keeps legacy page action URLs on the generic upload transport', () => {
    expect(resolveTusEndpoint('/auth/profile/avatar')).toBe('/files/tus')
  })
})
