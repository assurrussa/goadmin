import { describe, expect, it } from 'vitest'
import { dateParts, dateValue, type DateParts } from './dateValue'

describe('strict calendar wire values', () => {
  it('accepts leap days without Date timezone conversion', () => {
    expect(dateValue(dateParts('2024-02-29', false), false)).toBe('2024-02-29')
    expect(dateValue(dateParts('2026-11-06T23:05', true), true)).toBe('2026-11-06T23:05')
  })
  it.each(['2025-02-29', '2026-04-31', '2026-00-02', '2026-11-06Z', '2026-11'])(
    'rejects %s',
    (value) => {
      expect(dateValue(dateParts(value, false), false)).toBeNull()
    },
  )
  it('distinguishes an empty draft from a partial or impossible draft', () => {
    expect(dateValue(dateParts('', false), false)).toBe('')
    expect(dateValue({ ...dateParts('', false), day: '2' }, false)).toBeNull()
    expect(dateValue({ ...dateParts('2024-02-29', false), year: '2025' }, false)).toBeNull()
  })
  it('rejects missing time and out-of-range 24-hour values', () => {
    const parts: DateParts = { ...dateParts('2026-11-06T00:05', true), hour: '24' }
    expect(dateValue(parts, true)).toBeNull()
    expect(dateValue({ ...parts, hour: '' }, true)).toBeNull()
  })
})
