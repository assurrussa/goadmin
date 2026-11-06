// Per-tab, short-lived recovery only for non-secret branding fields. Image bytes
// are deliberately excluded. A different actor or revoked permission destroys it.
const prefix = 'goadmin.branding.recovery.v1:'
const ttl = 30 * 60 * 1000
export type BrandingRecovery = {
  owner: string
  origin: string
  path: string
  created: number
  version: number
  name: string
  accentColor: string
  accentColorEnd?: string
  conflict?: boolean
  baseline: { name: string; accentColor: string; accentColorEnd?: string }
  logoLight?: ''
  logoDark?: ''
}
let capture: (() => BrandingRecovery | undefined) | undefined
export function registerBrandingRecovery(fn: () => BrandingRecovery | undefined) {
  capture = fn
  return () => {
    if (capture === fn) capture = undefined
  }
}
function keys() {
  return Object.keys(sessionStorage).filter((key) => key.startsWith(prefix))
}
export function clearBrandingRecovery() {
  capture = undefined
  try {
    keys().forEach((key) => sessionStorage.removeItem(key))
  } catch {
    /* Storage unavailable. */
  }
}
export function pauseBrandingRecovery() {
  try {
    const value = capture?.()
    if (!value) return
    clearBrandingRecovery()
    sessionStorage.setItem(
      prefix + encodeURIComponent(value.origin) + ':' + encodeURIComponent(value.owner),
      JSON.stringify(value),
    )
  } catch {
    /* The UI must not claim persistence when storage fails. */
  }
}
export function validateBrandingRecovery(owner: string, allowed: boolean) {
  try {
    for (const key of keys()) {
      let item: BrandingRecovery
      try {
        item = JSON.parse(sessionStorage.getItem(key) || '')
      } catch {
        sessionStorage.removeItem(key)
        continue
      }
      if (
        !allowed ||
        item.owner !== owner ||
        item.origin !== location.origin ||
        item.path !== '/auth/profile/settings' ||
        !Number.isFinite(item.created) ||
        Date.now() - item.created < 0 ||
        Date.now() - item.created > ttl
      )
        sessionStorage.removeItem(key)
    }
  } catch {
    /* Storage unavailable. */
  }
}
export function hasBrandingRecovery(owner: string): boolean {
  try {
    validateBrandingRecovery(owner, true)
    return (
      sessionStorage.getItem(
        prefix + encodeURIComponent(location.origin) + ':' + encodeURIComponent(owner),
      ) !== null
    )
  } catch {
    return false
  }
}
export function takeBrandingRecovery(owner: string): BrandingRecovery | undefined {
  try {
    validateBrandingRecovery(owner, true)
    const key = prefix + encodeURIComponent(location.origin) + ':' + encodeURIComponent(owner)
    const raw = sessionStorage.getItem(key)
    if (!raw) return
    sessionStorage.removeItem(key)
    const item = JSON.parse(raw) as BrandingRecovery
    if (
      !Number.isSafeInteger(item.version) ||
      item.version < 0 ||
      typeof item.name !== 'string' ||
      typeof item.accentColor !== 'string' ||
      typeof item.baseline?.name !== 'string' ||
      typeof item.baseline?.accentColor !== 'string'
    )
      return
    // Old drafts have a solid accent. Reject malformed pairs as a whole.
    item.accentColorEnd ??= ''
    item.baseline.accentColorEnd ??= ''
    const validPair = (start: string, end: unknown) =>
      typeof end === 'string' &&
      (start === '' || /^#[\da-f]{6}$/i.test(start)) &&
      (end === '' || (start !== '' && /^#[\da-f]{6}$/i.test(end)))
    if (
      !validPair(item.accentColor, item.accentColorEnd) ||
      !validPair(item.baseline.accentColor, item.baseline.accentColorEnd)
    )
      return
    return item
  } catch {
    return
  }
}
