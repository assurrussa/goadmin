import { reactive } from 'vue'
import { applyLocale, installLocalePreferenceHandler } from '@/i18n'
import { adminFetch } from '@/http'
import { applyBrand, emptyBrand, readBrand } from './brand'

export type Preferences = {
  schemaVersion: 1
  theme: 'system' | 'light' | 'dark'
  locale: 'ru' | 'en'
  density: 'default' | 'compact' | 'comfortable'
}
export interface AppearanceDocument {
  owner: string
  version: number
  payload: Preferences
  unavailable?: boolean
}
export const deviceKey = 'goadmin.appearance.device.v1'
const defaults = (): Preferences => ({
  schemaVersion: 1,
  theme: 'system',
  locale: 'ru',
  density: 'default',
})
export function readPreferences(value: unknown): Preferences {
  const p = (value || {}) as Partial<Preferences>
  return {
    schemaVersion: 1,
    theme: ['system', 'light', 'dark'].includes(p.theme || '') ? p.theme! : 'system',
    locale: p.locale === 'en' ? 'en' : 'ru',
    density: ['default', 'compact', 'comfortable'].includes(p.density || '')
      ? p.density!
      : 'default',
  }
}
export const appearance = reactive({
  owner: '',
  version: 0,
  draft: defaults(),
  saved: defaults(),
  status: 'saved' as 'saved' | 'saving' | 'error' | 'conflict',
  readOnly: false,
  brandingRead: false,
  brandingUpdate: false,
  brand: { ...emptyBrand },
  dark: false,
  resolving: false,
})
let generation = 0
let controller: AbortController | undefined
let initialized = false
let media: MediaQueryList | undefined
let pending = false
const changed = () =>
  (['theme', 'locale', 'density'] as const).filter(
    (key) => appearance.draft[key] !== appearance.saved[key],
  )
function device(): Preferences {
  try {
    return readPreferences(JSON.parse(localStorage.getItem(deviceKey) || 'null'))
  } catch {
    return defaults()
  }
}
export function paintAppearance() {
  appearance.dark =
    appearance.draft.theme === 'dark' || (appearance.draft.theme === 'system' && !!media?.matches)
  document.documentElement.classList.toggle('dark', appearance.dark)
  document.documentElement.dataset.themeMode = appearance.dark ? 'dark' : 'light'
  document.documentElement.dataset.density = appearance.draft.density
  applyLocale(appearance.draft.locale)
  applyBrand(appearance.brand, appearance.dark)
}
export function initAppearance() {
  if (initialized) return
  initialized = true
  media = window.matchMedia?.('(prefers-color-scheme: dark)')
  media?.addEventListener('change', paintAppearance)
  const root = document.documentElement
  // Validated head attributes seed paint only; server props still own identity.
  appearance.draft = root.dataset.appearanceOwner
    ? readPreferences({
        theme: root.dataset.themePreference,
        locale: root.lang,
        density: root.dataset.density,
      })
    : device()
  appearance.saved = { ...appearance.draft }
  installLocalePreferenceHandler((value) => editAppearance({ locale: value }))
  paintAppearance()
}
export function syncAppearance(pageLike: unknown) {
  initAppearance()
  const props = (pageLike as { props?: Record<string, unknown> })?.props || {}
  const doc = props.authuser ? (props.appearance as AppearanceDocument | null | undefined) : null
  const access = props.appearanceAccess as
    | { readOnly?: boolean; brandingRead?: boolean; brandingUpdate?: boolean }
    | undefined
  const actor = props.authuser as { id?: string | number; idExact?: string } | undefined
  const authID = actor?.idExact || actor?.id
  const ownerMismatch = authID !== undefined && String(authID) !== doc?.owner
  const unavailable =
    ownerMismatch ||
    props.appearanceUnavailable === true ||
    doc?.unavailable === true ||
    (!!props.authuser && !doc?.payload)
  const readOnly = !!access?.readOnly || unavailable
  const owner = typeof doc?.owner === 'string' ? doc.owner : ''
  const identityChanged = owner !== appearance.owner
  const fence = identityChanged || readOnly !== appearance.readOnly
  if (fence) {
    generation++
    controller?.abort()
    controller = undefined
    pending = false
    appearance.resolving = false
    appearance.status = 'saved'
  }
  appearance.readOnly = readOnly
  appearance.brandingRead = !!access?.brandingRead
  appearance.brandingUpdate = !!access?.brandingUpdate
  if (props.branding) {
    const brand = readBrand(props.branding)
    if (brand.version >= appearance.brand.version) appearance.brand = brand
  }
  const version = doc?.version || 0
  if (
    (identityChanged || version >= appearance.version) &&
    (fence || (!changed().length && appearance.status === 'saved'))
  ) {
    appearance.owner = owner
    appearance.version = version
    appearance.draft = owner ? readPreferences(doc?.payload) : device()
    appearance.saved = { ...appearance.draft }
  } else if (fence) {
    // A permission fence cancels local edits without restoring stale history props.
    appearance.draft = { ...appearance.saved }
  }
  if (unavailable) appearance.status = 'error'
  paintAppearance()
}
export function discardAppearance() {
  generation++
  controller?.abort()
  controller = undefined
  pending = false
  appearance.resolving = false
  appearance.owner = ''
  appearance.version = 0
  appearance.readOnly = true
  appearance.brandingRead = false
  appearance.brandingUpdate = false
  appearance.status = 'saved'
  appearance.draft = device()
  appearance.saved = { ...appearance.draft }
  paintAppearance()
}
export function disposeAppearance() {
  discardAppearance()
  media?.removeEventListener('change', paintAppearance)
  initialized = false
}
export function editAppearance(patch: Partial<Preferences>) {
  initAppearance()
  if (appearance.readOnly || appearance.resolving) return
  appearance.draft = readPreferences({ ...appearance.draft, ...patch })
  paintAppearance()
  if (!appearance.owner) {
    try {
      localStorage.setItem(deviceKey, JSON.stringify(appearance.draft))
    } catch {
      /* Device storage is optional. */
    }
    appearance.saved = { ...appearance.draft }
    return
  }
  if (appearance.status === 'conflict' || appearance.status === 'error') return
  void saveAppearance()
}
export async function saveAppearance() {
  if (
    pending ||
    !appearance.owner ||
    appearance.readOnly ||
    appearance.status === 'conflict' ||
    !changed().length
  )
    return
  const epoch = generation
  const owner = appearance.owner
  const payload = { ...appearance.draft }
  pending = true
  appearance.status = 'saving'
  const request = new AbortController()
  controller = request
  try {
    const response = await adminFetch('/auth/profile/appearance', {
      method: 'PATCH',
      signal: request.signal,
      headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
      body: JSON.stringify({ version: appearance.version, payload }),
    })
    if (epoch !== generation) return
    if (response.status === 409) {
      appearance.status = 'conflict'
      return
    }
    if (!response.ok) throw new Error('save')
    const doc = (await response.json()) as AppearanceDocument
    if (epoch !== generation) return
    if (doc.owner !== owner || !Number.isSafeInteger(doc.version) || !doc.payload)
      throw new Error('owner')
    appearance.version = doc.version
    appearance.saved = readPreferences(doc.payload)
    appearance.status = 'saved'
  } catch {
    if (epoch === generation) appearance.status = 'error'
  } finally {
    if (epoch === generation) {
      pending = false
      controller = undefined
      if (appearance.status === 'saved' && changed().length) void saveAppearance()
    }
  }
}
export async function reloadAppearance(applyDraft = false) {
  if (appearance.readOnly || !appearance.owner || pending) return
  const epoch = generation
  appearance.resolving = true
  const keys = changed()
  const draft = { ...appearance.draft }
  pending = true
  const request = new AbortController()
  controller = request
  try {
    const response = await adminFetch('/auth/profile/appearance', {
      signal: request.signal,
      headers: { Accept: 'application/json' },
    })
    if (!response.ok) throw new Error('load')
    const doc = (await response.json()) as AppearanceDocument
    if (epoch !== generation) return
    if (doc.owner !== appearance.owner || !Number.isSafeInteger(doc.version) || !doc.payload)
      throw new Error('owner')
    appearance.version = doc.version
    appearance.saved = readPreferences(doc.payload)
    appearance.draft = { ...appearance.saved }
    if (applyDraft)
      keys.forEach((key) => {
        Object.assign(appearance.draft, { [key]: draft[key] })
      })
    appearance.status = 'saved'
    paintAppearance()
  } catch {
    if (epoch === generation) appearance.status = 'error'
  } finally {
    if (epoch === generation) {
      pending = false
      controller = undefined
      appearance.resolving = false
      if (applyDraft && appearance.status === 'saved') void saveAppearance()
    }
  }
}
