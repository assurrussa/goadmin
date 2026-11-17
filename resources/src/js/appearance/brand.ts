export interface Brand {
  version: number
  name: string
  accentColorEnd?: string
  accentColor: string
  logoLightUrl: string
  logoDarkUrl: string
}
export const emptyBrand: Brand = {
  version: 0,
  name: '',
  accentColor: '',
  accentColorEnd: '',
  logoLightUrl: '',
  logoDarkUrl: '',
}
export const oceanAccent = { start: '#38BDF8', end: '#1E3A8A' }
export function safeLogo(value: unknown): string {
  if (typeof value !== 'string') return ''
  try {
    const url = new URL(value, window.location.origin)
    return url.origin === window.location.origin && url.pathname.includes('/public/branding/')
      ? url.href
      : ''
  } catch {
    return ''
  }
}
export function readBrand(value: unknown): Brand {
  if (!value || typeof value !== 'object') return { ...emptyBrand }
  const item = value as Brand
  return {
    version: Number.isSafeInteger(item.version) ? item.version : 0,
    name: typeof item.name === 'string' ? item.name : '',
    accentColor: /^#[\da-f]{6}$/i.test(item.accentColor) ? item.accentColor : '',
    accentColorEnd:
      /^#[\da-f]{6}$/i.test(item.accentColor) && /^#[\da-f]{6}$/i.test(item.accentColorEnd || '')
        ? item.accentColorEnd
        : '',
    logoLightUrl: safeLogo(item.logoLightUrl),
    logoDarkUrl: safeLogo(item.logoDarkUrl),
  }
}
const luminance = (rgb: number[]) =>
  rgb
    .map((v) => {
      v /= 255
      return v <= 0.04045 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4
    })
    .reduce((s, v, i) => s + v * [0.2126, 0.7152, 0.0722][i]!, 0)
// Keep these stock tokens in sync with app.css. Checking both modes preserves
// extensions that customize only one mode, even when they export no theme pack.
function stockAccent(dark: boolean): Record<string, string> {
  const tone = (lightness: number, suffix = '') => `hsl(217 69% ${lightness}%${suffix})`
  const primary = tone(dark ? 74 : 46)
  const foreground = dark ? 'hsl(220 18% 12%)' : 'hsl(0 0% 100%)'
  return {
    '--color-primary': primary,
    '--color-primary-light': tone(dark ? 80 : 58),
    '--color-primary-lighter': tone(dark ? 86 : 70),
    '--color-primary-dark': tone(dark ? 64 : 36),
    '--color-primary-darker': tone(dark ? 54 : 28),
    '--color-primary-contrast': foreground,
    '--color-primary-foreground': foreground,
    '--color-primary-rgb': dark ? '143, 178, 234' : '37, 99, 200',
    '--color-ring': primary,
    '--color-focus-ring': primary,
    '--color-border-focus': primary,
    '--outline-focus': `2px solid ${primary}`,
    '--color-badge-primary-bg': dark ? tone(26, ' / 0.5') : tone(94, ' / 0.9'),
    '--color-badge-primary-border': dark ? tone(52, ' / 0.45') : tone(46, ' / 0.35'),
    '--color-badge-primary-text': tone(dark ? 76 : 38),
  }
}
const hostFillTokens = {
  '--goadmin-primary-image': 'none',
  '--goadmin-primary-fill-start': 'initial',
  '--goadmin-primary-fill-end': 'initial',
  '--goadmin-primary-fill-foreground': '',
  '--goadmin-selection-image': 'none',
  '--goadmin-selection-foreground': '',
  '--goadmin-row-accent': '',
  '--goadmin-brand-surface-image': 'none',
  '--goadmin-brand-auth-image': 'none',
}
const normalize = (value: string) =>
  value
    .replace(/\/\*[^]*?\*\//g, ' ')
    .replace(/[,\s]+/g, ' ')
    .replace(/\s*([()/])\s*/g, '$1')
    .trim()
    .toLowerCase()
type HostAccent = {
  stock: boolean
  hex?: string
  customSelection: boolean
  complete: boolean
  palette: Record<string, string>
}
type InlineDeclaration = { value: string; priority: string }
const applied = new Map<string, { value: string; previous: InlineDeclaration }>()
function inlineDeclaration(style: CSSStyleDeclaration, name: string): InlineDeclaration {
  return { value: style.getPropertyValue(name), priority: style.getPropertyPriority(name) }
}
function hostDeclaration(style: CSSStyleDeclaration, name: string): InlineDeclaration {
  const current = inlineDeclaration(style, name)
  const override = applied.get(name)
  return override?.value === current.value && !current.priority ? override.previous : current
}
type MediaState = Map<string, { query: MediaQueryList; matches: boolean }>
type HostSheet = { css: string; media: string; complete: boolean }
let onMediaChange: (() => void) | undefined
let revision = 0
let packCache:
  | {
      signature: string
      css: string[]
      media: MediaState
      light: HostAccent
      dark: HostAccent
      stock: boolean
      revision: number
    }
  | undefined

function updateMediaListeners(media: MediaState | undefined, add: boolean) {
  if (!onMediaChange) return
  for (const { query } of media?.values() ?? []) {
    if (add) query.addEventListener('change', onMediaChange)
    else query.removeEventListener('change', onMediaChange)
  }
}
export function observeBrandMedia(onChange: () => void) {
  onMediaChange = onChange
  updateMediaListeners(packCache?.media, true)
  return () => {
    updateMediaListeners(packCache?.media, false)
    onMediaChange = undefined
  }
}

function mediaMatches(media: MediaState, text: string) {
  if (!text || !window.matchMedia) return true
  let state = media.get(text)
  if (!state) {
    const query = window.matchMedia(text)
    state = { query, matches: query.matches }
    media.set(text, state)
  }
  return state.matches
}

function copyStyle(target: Document, css: string): CSSStyleSheet | null {
  const frameWindow = target.defaultView as (Window & typeof globalThis) | null
  // Constructable sheets use CSSOM, so same-origin styles keep working when
  // style-src forbids new inline style elements.
  if (frameWindow?.CSSStyleSheet.prototype.replaceSync && 'adoptedStyleSheets' in target) {
    const sheet = new frameWindow.CSSStyleSheet()
    sheet.replaceSync(css)
    target.adoptedStyleSheets = [...target.adoptedStyleSheets, sheet]
    return sheet
  }
  const style = target.createElement('style')
  style.textContent = css
  target.head.append(style)
  return style.sheet
}

function snapshotHostSheet(sheet: CSSStyleSheet): HostSheet {
  const media = sheet.media?.mediaText || ''
  try {
    // CSSOM edits can replace or mutate rules without changing the sheet
    // object, its rule count, or the owning style element's text.
    const rules = [...sheet.cssRules]
    // Constructable sheets cannot preserve imports that may carry host branding.
    const localRules = rules.filter((rule) => rule.type !== CSSRule.IMPORT_RULE)
    return {
      css: localRules.map((rule) => rule.cssText).join('\n'),
      media,
      complete: localRules.length === rules.length,
    }
  } catch {
    return { css: '', media, complete: false }
  }
}

function copyHostStyles(target: Document, sheets: HostSheet[], media: MediaState) {
  const anchorMedia = (rules: CSSRuleList) => {
    for (const rule of rules) {
      if (rule.type === CSSRule.MEDIA_RULE) {
        const query = (rule as CSSMediaRule).media
        // Nested documents can inherit a different preferred color scheme.
        // Evaluate every conditional media rule in the real app window.
        query.mediaText = mediaMatches(media, query.mediaText) ? 'all' : 'not all'
      }
      if ('cssRules' in rule) anchorMedia((rule as CSSGroupingRule).cssRules)
    }
  }
  let complete = true
  for (const sheet of sheets) {
    if (!mediaMatches(media, sheet.media)) continue
    complete &&= sheet.complete
    if (!sheet.css) continue
    try {
      const copy = copyStyle(target, sheet.css)
      if (copy) anchorMedia(copy.cssRules)
      else complete = false
    } catch {
      complete = false
    }
  }
  return complete
}

function readHostAccent(
  dark: boolean,
  probe?: HTMLElement,
  computed?: CSSStyleDeclaration,
  copied = false,
): HostAccent {
  const read = (key: string) => computed?.getPropertyValue(key).trim() || ''
  const palette = stockAccent(dark)
  const complete = copied && Boolean(read('--color-primary'))
  let stock = complete
  for (const [key, fallback] of Object.entries(palette)) {
    const value = read(key)
    stock &&=
      normalize(value) === normalize(fallback) || equivalentAccent(probe, key, value, fallback)
    if (value) palette[key] = value
  }
  for (const [key, fallback] of Object.entries(hostFillTokens)) {
    const value = read(key)
    if (value) stock = false
    palette[key] = value || fallback
  }
  const primary = palette['--color-primary']!
  const channels = probe && resolveRGBA(primary, probe)
  const rgb = channels?.[3] === 1 ? channels.slice(0, 3) : undefined
  const customSelection = Boolean(palette['--goadmin-selection-foreground'])
  palette['--goadmin-selection-foreground'] ||= rgb ? selectionForeground(rgb, dark) : primary
  palette['--goadmin-primary-fill-foreground'] ||=
    read('--color-primary-foreground') || palette['--color-primary-contrast']!
  palette['--goadmin-row-accent'] ||= primary
  return { palette, stock, complete, customSelection, hex: rgbToHex(rgb) }
}

// Resolve both host modes in an isolated document without live brand overrides.
// Keep results through ordinary renders and resize events within a breakpoint.
function hostAccents() {
  const root = document.documentElement
  const attributes = [...root.attributes].filter(
    (attribute) => !['style', 'class', 'data-theme-mode'].includes(attribute.name),
  )
  const classes = [...root.classList].filter((name) => name !== 'dark').join(' ')
  const sheets = [...document.styleSheets, ...(document.adoptedStyleSheets || [])]
    .filter((sheet) => !sheet.disabled)
    .map(snapshotHostSheet)
  const inline = Array.from(root.style, (name) => ({ name, ...hostDeclaration(root.style, name) }))
    .filter(({ value }) => value)
    .sort((a, b) => a.name.localeCompare(b.name))
  const signature = JSON.stringify([
    attributes.map(({ name, value }) => [name, value]),
    classes,
    inline,
    sheets.map(({ media, complete }) => [media, complete]),
  ])
  if (
    packCache?.signature !== signature ||
    sheets.some((sheet, index) => sheet.css !== packCache!.css[index]) ||
    [...packCache.media.values()].some(({ query, matches }) => query.matches !== matches)
  ) {
    const frame = document.createElement('iframe')
    frame.tabIndex = -1
    frame.setAttribute('aria-hidden', 'true')
    Object.assign(frame.style, {
      position: 'fixed',
      top: '0',
      left: '0',
      width: `${window.innerWidth}px`,
      height: `${window.innerHeight}px`,
      border: '0',
      visibility: 'hidden',
      pointerEvents: 'none',
    })
    document.body.append(frame)
    try {
      const media: MediaState = new Map()
      const target = frame.contentDocument
      const frameWindow = frame.contentWindow
      const targetRoot = target?.documentElement
      const probe = target?.createElement('span')
      if (targetRoot) {
        attributes.forEach(({ name, value }) => targetRoot.setAttribute(name, value))
        inline.forEach(({ name, value, priority }) =>
          targetRoot.style.setProperty(name, value, priority),
        )
      }
      const copied = !!target && copyHostStyles(target, sheets, media)
      if (target && probe) target.body.append(probe)
      const resolve = (variant: boolean) => {
        if (!targetRoot || !frameWindow) return readHostAccent(variant)
        targetRoot.className = classes
        targetRoot.classList.toggle('dark', variant)
        targetRoot.dataset.themeMode = variant ? 'dark' : 'light'
        return readHostAccent(variant, probe, frameWindow.getComputedStyle(targetRoot), copied)
      }
      const light = resolve(false)
      const dark = resolve(true)
      updateMediaListeners(packCache?.media, false)
      packCache = {
        signature,
        css: sheets.map((sheet) => sheet.css),
        media,
        light,
        dark,
        stock: light.stock && dark.stock,
        revision: ++revision,
      }
      updateMediaListeners(media, true)
    } finally {
      frame.remove()
    }
  }
  return packCache
}
function rgbToHex(rgb?: number[]): string | undefined {
  return rgb && `#${rgb.map((v) => Math.round(v).toString(16).padStart(2, '0')).join('')}`
}
/** Supply the project's accent as an sRGB HEX starting point for the color picker. */
export function themePrimaryHex(dark: boolean): string | undefined {
  const accents = hostAccents()
  return accents.stock ? oceanAccent.start : accents[dark ? 'dark' : 'light'].hex
}
function equivalentAccent(
  probe: HTMLElement | undefined,
  key: string,
  value: string,
  expected: string,
): boolean {
  if (!probe || !value || key === '--color-primary-rgb') return false
  if (key === '--outline-focus') {
    const outlineColor = (outline: string) => {
      probe.style.removeProperty('outline')
      probe.style.outline = outline
      return probe.style.outlineWidth === '2px' && probe.style.outlineStyle === 'solid'
        ? probe.style.outlineColor
        : ''
    }
    value = outlineColor(value)
    expected = outlineColor(expected)
  }
  const left = resolveRGBA(value, probe)
  const right = resolveRGBA(expected, probe)
  return !!left && !!right && left.every((channel, index) => channel === right[index])
}
function resolveRGBA(color: string, probe: HTMLElement): number[] | undefined {
  // Computed style resolves ordinary HEX/HSL without requiring canvas (also in
  // DOM-only tests). Canvas is the fallback for modern color spaces.
  probe.style.removeProperty('color')
  probe.style.setProperty('color', color, 'important')
  if (!probe.style.color) return undefined
  const computed = probe.ownerDocument.defaultView!.getComputedStyle(probe).color
  const rgb = computed.match(
    /^rgba?\(\s*([\d.]+)[, ]+([\d.]+)[, ]+([\d.]+)(?:[, /]+([\d.]+))?\s*\)$/,
  )
  if (rgb) return [Number(rgb[1]), Number(rgb[2]), Number(rgb[3]), Number(rgb[4] ?? 1)]
  const canvas = document.createElement('canvas')
  canvas.width = canvas.height = 1
  const context = canvas.getContext('2d', { willReadFrequently: true })
  if (!context) return undefined
  // Invalid assignments leave fillStyle unchanged; two sentinels distinguish them
  // from a legitimate color equal to either sentinel.
  context.fillStyle = '#010203'
  context.fillStyle = color
  const first = context.fillStyle
  context.fillStyle = '#040506'
  context.fillStyle = color
  if (first !== context.fillStyle) return undefined
  context.clearRect(0, 0, 1, 1)
  context.fillRect(0, 0, 1, 1)
  const [r, g, b, a] = context.getImageData(0, 0, 1, 1).data
  return [r!, g!, b!, a! / 255]
}
function selectionForeground(rgb: number[], dark: boolean, endChannels?: number[]): string {
  // Navigation already has a 10% primary background; the gradient adds another
  // 10% layer; hover can replace the base tint with a plain surface. Bound the
  // final sRGB background with two intensity levels of rounding slack. The
  // built-in card, page and hover surfaces have light channels >=230 and dark
  // channels <=60. Dark luminance is convex along the gradient,
  // so its maximum is at an endpoint. For light mode the componentwise minimum
  // gives a conservative lower bound even for opposing-hue interior minima.
  const surface = dark ? 60 : 230
  const selectionBases = [
    rgb.map((channel) => channel * 0.1 + surface * 0.9),
    [surface, surface, surface],
  ]
  const selectionStops = endChannels
    ? [rgb, endChannels].flatMap((stop) =>
        selectionBases.map((base) => stop.map((channel, i) => channel * 0.1 + base[i]! * 0.9)),
      )
    : selectionBases
  const selectionBackground = dark
    ? Math.max(...selectionStops.map((stop) => luminance(stop.map((v) => Math.min(255, v + 2)))))
    : luminance(
        rgb.map((_, i) => Math.max(0, Math.min(...selectionStops.map((stop) => stop[i]!)) - 2)),
      )
  let selectionText = [...rgb]
  while (
    (Math.max(luminance(selectionText), selectionBackground) + 0.05) /
      (Math.min(luminance(selectionText), selectionBackground) + 0.05) <
    4.55
  ) {
    selectionText = selectionText.map((v) => (dark ? Math.min(255, v + 1) : Math.max(0, v - 1)))
  }
  return `rgb(${selectionText.map(Math.round).join(' ')})`
}
/** Maintain AA contrast for accent text and filled controls, including pale custom accents. */
export function deriveBrandPalette(hex: string, dark: boolean, end = ''): Record<string, string> {
  if (!/^#[\da-f]{6}$/i.test(hex)) {
    const accents = hostAccents()
    if (!accents.stock) return accents[dark ? 'dark' : 'light'].palette
    hex = oceanAccent.start
    end = oceanAccent.end
  }
  let rgb = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16))
  const background = dark ? 0.033 : 1
  for (let i = 0; i < 100; i++) {
    const l = luminance(rgb)
    if ((Math.max(l, background) + 0.05) / (Math.min(l, background) + 0.05) >= 5) break
    rgb = rgb.map((v) => (dark ? Math.min(255, v + 3) : Math.max(0, v - 3)))
  }
  const color = `rgb(${rgb.map(Math.round).join(' ')})`
  const foreground = luminance(rgb) > 0.179 ? '#000000' : '#ffffff'
  // sRGB luminance is convex: every interpolated point is at most the
  // brighter endpoint. White therefore meets the target across the whole fill,
  // including opposing hues whose midpoint is easy to miss with endpoint tests.
  const fill = (value: string) => {
    let channels = [1, 3, 5].map((i) => parseInt(value.slice(i, i + 2), 16))
    while (luminance(channels) > 0.16) channels = channels.map((v) => Math.max(0, v - 1))
    return channels
  }
  const gradient = /^#[\da-f]{6}$/i.test(end)
  const startChannels = gradient ? fill(hex) : rgb
  const endChannels = gradient ? fill(end) : rgb
  const startFill = `rgb(${startChannels.join(' ')})`
  const endFill = `rgb(${endChannels.join(' ')})`
  const surface = gradient
    ? `linear-gradient(135deg in srgb, color-mix(in srgb, ${hex} ${dark ? 8 : 6}%, transparent), color-mix(in srgb, ${end} ${dark ? 8 : 6}%, transparent))`
    : 'none'
  return {
    '--goadmin-brand-surface-image': surface,
    '--goadmin-brand-auth-image': gradient
      ? `linear-gradient(to bottom, transparent, var(--color-background) 28rem), ${surface}`
      : 'none',
    '--goadmin-row-accent': gradient ? `color-mix(in srgb, ${color} 50%, ${endFill})` : color,
    '--goadmin-primary-image': gradient
      ? `linear-gradient(135deg in srgb, ${startFill}, ${endFill})`
      : 'none',
    '--goadmin-primary-fill-start': startFill,
    '--goadmin-primary-fill-end': endFill,
    '--goadmin-primary-fill-foreground': gradient ? '#ffffff' : foreground,
    '--goadmin-selection-image': gradient
      ? `linear-gradient(135deg in srgb, color-mix(in srgb, ${color} 10%, transparent), color-mix(in srgb, ${endFill} 10%, transparent))`
      : 'none',
    '--goadmin-selection-foreground': selectionForeground(
      rgb,
      dark,
      gradient ? endChannels : undefined,
    ),
    '--color-primary': color,
    '--color-primary-rgb': rgb.join(', '),
    '--color-primary-dark': color,
    '--color-primary-light': color,
    '--color-primary-darker': color,
    '--color-primary-lighter': color,
    '--color-primary-contrast': foreground,
    '--color-primary-foreground': foreground,
    '--color-ring': color,
    '--color-focus-ring': color,
    '--color-border-focus': color,
    '--outline-focus': `2px solid ${color}`,
    '--color-badge-primary-bg': `color-mix(in srgb, ${color} 10%, ${dark ? '#20242b' : '#ffffff'})`,
    '--color-badge-primary-border': `color-mix(in srgb, ${color} 40%, transparent)`,
    '--color-badge-primary-text': color,
  }
}
export function applyBrand(brand: Brand, dark: boolean) {
  const accents = hostAccents()
  let palette: Record<string, string>
  if (brand.accentColor) {
    palette = deriveBrandPalette(brand.accentColor, dark, brand.accentColorEnd)
  } else {
    const host = accents[dark ? 'dark' : 'light']
    palette = accents.stock
      ? deriveBrandPalette(oceanAccent.start, dark, oceanAccent.end)
      : !host.complete || host.customSelection
        ? {}
        : { '--goadmin-selection-foreground': host.palette['--goadmin-selection-foreground']! }
  }
  const style = document.documentElement.style
  for (const key of applied.keys()) {
    if (key in palette) continue
    const { value, priority } = hostDeclaration(style, key)
    if (value) style.setProperty(key, value, priority)
    else style.removeProperty(key)
    applied.delete(key)
  }
  for (const [key, value] of Object.entries(palette)) {
    const previous = hostDeclaration(style, key)
    const current = inlineDeclaration(style, key)
    if (current.value !== value || current.priority) style.setProperty(key, value)
    applied.set(key, { value, previous })
  }
  return accents.revision
}
