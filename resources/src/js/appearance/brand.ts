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
// Resolve the selected host pack in a separate document: live root inline brand
// overrides and its current dark mode must not leak into the opposite preview.
let packCache:
  | {
      signature: string
      sheets: CSSStyleSheet[]
      light: Record<string, string>
      dark: Record<string, string>
    }
  | undefined
function packPalette(dark: boolean, fallback = true): Record<string, string> {
  const root = document.documentElement
  const attributes = [...root.attributes].filter(
    (attribute) => !['style', 'class', 'data-theme-mode'].includes(attribute.name),
  )
  const classes = [...root.classList].filter((name) => name !== 'dark').join(' ')
  const sheets = [...document.styleSheets, ...(document.adoptedStyleSheets || [])]
  const signature = JSON.stringify([attributes.map(({ name, value }) => [name, value]), classes])
  if (
    packCache?.signature !== signature ||
    sheets.length !== packCache.sheets.length ||
    sheets.some((sheet, index) => sheet !== packCache!.sheets[index])
  ) {
    const css = sheets
      .map((sheet) => {
        try {
          return [...sheet.cssRules].map((rule) => rule.cssText).join('\n')
        } catch {
          return ''
        }
      })
      .join('\n')
    const frame = document.createElement('iframe')
    frame.hidden = true
    frame.setAttribute('aria-hidden', 'true')
    document.body.append(frame)
    try {
      const target = frame.contentDocument!
      const targetRoot = target.documentElement
      attributes.forEach(({ name, value }) => targetRoot.setAttribute(name, value))
      const style = target.createElement('style')
      style.textContent = css
      target.head.append(style)
      const resolve = (variant: boolean) => {
        targetRoot.className = classes
        targetRoot.classList.toggle('dark', variant)
        targetRoot.dataset.themeMode = variant ? 'dark' : 'light'
        const computed = frame.contentWindow!.getComputedStyle(targetRoot)
        const primary = computed.getPropertyValue('--color-primary').trim()
        const contrast = computed.getPropertyValue('--color-primary-contrast').trim()
        return {
          '--color-primary': primary,
          '--color-primary-contrast': contrast || (variant ? 'hsl(220 18% 12%)' : '#ffffff'),
        }
      }
      packCache = { signature, sheets, light: resolve(false), dark: resolve(true) }
    } finally {
      frame.remove()
    }
  }
  const palette = { ...packCache[dark ? 'dark' : 'light'] }
  if (fallback && !palette['--color-primary'])
    palette['--color-primary'] = `hsl(217 69% ${dark ? 74 : 46}%)`
  return palette
}
/** Convert the active host theme primary to the sRGB hex required by input[type=color].
 * Canvas resolves modern CSS colors (including oklch) without inventing a fallback.
 */
export function themePrimaryHex(dark: boolean): string | undefined {
  const color = packPalette(dark, false)['--color-primary']
  if (!color) return undefined
  const rgb = resolveRGB(color)
  return rgb
    ? `#${rgb.map((v) => Math.round(v).toString(16).padStart(2, '0')).join('')}`
    : undefined
}
function resolveRGB(color: string): number[] | undefined {
  // Computed style resolves ordinary HEX/HSL without requiring canvas (also in
  // DOM-only tests). Canvas is the fallback for modern color spaces.
  const probe = document.createElement('span')
  probe.style.color = color
  if (!probe.style.color) return undefined
  document.body.append(probe)
  const computed = getComputedStyle(probe).color
  probe.remove()
  const rgb = computed.match(/^rgb\(\s*([\d.]+)[, ]+([\d.]+)[, ]+([\d.]+)\s*\)$/)
  if (rgb) return rgb.slice(1).map(Number)
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
  if (a !== 255) return undefined
  return [r!, g!, b!]
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
    const palette = packPalette(dark)
    const rgb = resolveRGB(palette['--color-primary']!)
    return {
      ...palette,
      '--goadmin-primary-image': 'none',
      '--goadmin-selection-image': 'none',
      '--goadmin-brand-surface-image': 'none',
      '--goadmin-brand-auth-image': 'none',
      '--goadmin-selection-foreground': rgb
        ? selectionForeground(rgb, dark)
        : palette['--color-primary']!,
      '--goadmin-primary-fill-foreground': palette['--color-primary-contrast']!,
    }
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
    '--color-primary-dark': color,
    '--color-primary-light': color,
    '--color-primary-darker': color,
    '--color-primary-lighter': color,
    '--color-primary-contrast': foreground,
    '--color-primary-foreground': foreground,
    '--color-ring': color,
    '--color-focus-ring': color,
    '--color-border-focus': color,
    '--color-badge-primary-bg': `color-mix(in srgb, ${color} 10%, ${dark ? '#20242b' : '#ffffff'})`,
    '--color-badge-primary-border': `color-mix(in srgb, ${color} 40%, transparent)`,
    '--color-badge-primary-text': color,
  }
}
let applied: string[] = []
export function applyBrand(brand: Brand, dark: boolean) {
  applied.forEach((key) => document.documentElement.style.removeProperty(key))
  // An unset live accent must retain the selected CSS theme pack.
  const palette = brand.accentColor
    ? deriveBrandPalette(brand.accentColor, dark, brand.accentColorEnd)
    : {
        '--goadmin-selection-foreground': deriveBrandPalette('', dark)[
          '--goadmin-selection-foreground'
        ]!,
      }
  applied = Object.keys(palette)
  Object.entries(palette).forEach(([key, value]) =>
    document.documentElement.style.setProperty(key, value),
  )
}
