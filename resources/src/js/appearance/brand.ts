export interface Brand {
  version: number
  name: string
  accentColor: string
  logoLightUrl: string
  logoDarkUrl: string
}
export const emptyBrand: Brand = {
  version: 0,
  name: '',
  accentColor: '',
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
function packPalette(dark: boolean): Record<string, string> {
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
          '--color-primary': primary || `hsl(217 69% ${variant ? 74 : 46}%)`,
          '--color-primary-contrast': contrast || (variant ? 'hsl(220 18% 12%)' : '#ffffff'),
        }
      }
      packCache = { signature, sheets, light: resolve(false), dark: resolve(true) }
    } finally {
      frame.remove()
    }
  }
  return { ...packCache[dark ? 'dark' : 'light'] }
}
/** Maintain AA contrast for accent text and filled controls, including pale custom accents. */
export function deriveBrandPalette(hex: string, dark: boolean): Record<string, string> {
  if (!/^#[\da-f]{6}$/i.test(hex)) return packPalette(dark)
  let rgb = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16))
  const background = dark ? 0.033 : 1
  for (let i = 0; i < 100; i++) {
    const l = luminance(rgb)
    if ((Math.max(l, background) + 0.05) / (Math.min(l, background) + 0.05) >= 5) break
    rgb = rgb.map((v) => (dark ? Math.min(255, v + 3) : Math.max(0, v - 3)))
  }
  const color = `rgb(${rgb.map(Math.round).join(' ')})`
  const foreground = luminance(rgb) > 0.179 ? '#000000' : '#ffffff'
  return {
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
  const palette = brand.accentColor ? deriveBrandPalette(brand.accentColor, dark) : {}
  applied = Object.keys(palette)
  Object.entries(palette).forEach(([key, value]) =>
    document.documentElement.style.setProperty(key, value),
  )
}
