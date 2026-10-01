export const normalizeUrl = (value?: string | null): string | null => {
  if (!value) {
    return null
  }

  const trimmed = value.trim()
  if (!trimmed) {
    return null
  }

  try {
    const base = typeof window !== 'undefined' ? window.location.origin : undefined
    const url = base ? new URL(trimmed, base) : new URL(trimmed)
    url.search = ''
    url.hash = ''
    return url.toString()
  } catch {
    const [path] = trimmed.split('?')
    return path ?? trimmed
  }
}
