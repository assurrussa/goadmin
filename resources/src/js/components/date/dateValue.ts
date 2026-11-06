import { parseDate, parseDateTime } from '@internationalized/date'

export type DatePart = 'day' | 'month' | 'year' | 'hour' | 'minute'
export type DateParts = Record<DatePart, string>

/** Parse only the supported wire shape; malformed server values remain invalid drafts. */
export function dateParts(value: string, withTime: boolean): DateParts {
  const result: DateParts = { day: '', month: '', year: '', hour: '', minute: '' }
  if (!value) return result
  const match = value.match(
    withTime ? /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})$/ : /^(\d{4})-(\d{2})-(\d{2})$/,
  )
  if (!match) return { ...result, year: value }
  return {
    year: match[1]!,
    month: match[2]!,
    day: match[3]!,
    hour: match[4] ?? '',
    minute: match[5] ?? '',
  }
}

/** Empty is distinct from incomplete/invalid. Never clamp an impossible Gregorian date. */
export function dateValue(parts: DateParts, withTime: boolean): string | null {
  const keys: DatePart[] = withTime
    ? ['day', 'month', 'year', 'hour', 'minute']
    : ['day', 'month', 'year']
  if (keys.every((key) => parts[key] === '')) return ''
  if (keys.some((key) => !/^\d+$/.test(parts[key])) || parts.year.length !== 4) return null
  const pad = (part: DatePart) => parts[part].padStart(2, '0')
  const value = `${parts.year}-${pad('month')}-${pad('day')}${withTime ? `T${pad('hour')}:${pad('minute')}` : ''}`
  try {
    const parsed = withTime ? parseDateTime(value) : parseDate(value)
    // parseDateTime serializes seconds, whereas the host expects local minute precision.
    return parsed.toString().slice(0, withTime ? 16 : 10) === value ? value : null
  } catch {
    return null
  }
}
