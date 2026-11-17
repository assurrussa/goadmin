import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import {
  applyBrand,
  deriveBrandPalette,
  emptyBrand,
  observeBrandMedia,
  themePrimaryHex,
} from './brand'
import { installStockBrandStyles } from './brand.fixture'

beforeEach(() => installStockBrandStyles())
afterEach(() => {
  document.documentElement.removeAttribute('style')
})

function luminance(color: string) {
  const element = document.createElement('span')
  element.style.color = color
  document.body.append(element)
  const rgb = getComputedStyle(element)
    .color.match(/[\d.]+/g)!
    .slice(0, 3)
    .map(Number)
  element.remove()
  return rgbLuminance(rgb)
}
function rgbLuminance(rgb: number[]) {
  return rgb.reduce((sum, channel, index) => {
    const value = channel / 255
    const linear = value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4
    return sum + linear * [0.2126, 0.7152, 0.0722][index]!
  }, 0)
}
function contrast(a: string, b: string) {
  return luminanceContrast(luminance(a), luminance(b))
}
function luminanceContrast(a: number, b: number) {
  return (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05)
}
it.each([false, true])(
  'provides AA link and button contrast for an unset accent in dark=%s previews',
  (dark) => {
    const palette = deriveBrandPalette('', dark)
    const primary = palette['--color-primary']!
    expect(palette).toEqual(deriveBrandPalette('#38BDF8', dark, '#1E3A8A'))
    expect(contrast(primary, dark ? '#20242b' : '#ffffff')).toBeGreaterThanOrEqual(4.5)
    for (const stop of ['start', 'end'])
      expect(
        contrast(
          palette[`--goadmin-primary-fill-${stop}`]!,
          palette['--goadmin-primary-fill-foreground']!,
        ),
      ).toBeGreaterThanOrEqual(4.5)
    applyBrand(emptyBrand, dark)
    for (const [name, value] of Object.entries(palette))
      expect(document.documentElement.style.getPropertyValue(name)).toBe(value)
  },
)

it.each([['topbar', 'hsl(174 78% 26%)', 'hsl(174 66% 70%)']])(
  'keeps unset %s pack highlights readable without overriding host colors',
  (pack, light, dark) => {
    const root = document.documentElement
    const attributes = ['style', 'class', 'data-admin-theme', 'data-theme-mode'].map(
      (name) => [name, root.getAttribute(name)] as const,
    )
    const sheet = document.createElement('style')
    sheet.textContent = `
    :root[data-admin-theme='test.${pack}'] { --color-primary: ${light}; --color-ring: ${light}; }
    :root[data-admin-theme='test.${pack}'].dark { --color-primary: ${dark}; --color-ring: ${dark}; }
  `
    document.head.append(sheet)
    try {
      root.dataset.adminTheme = `test.${pack}`
      for (const isDark of [false, true]) {
        root.classList.toggle('dark', isDark)
        const palette = deriveBrandPalette('', isDark)
        const primary = document.createElement('span')
        primary.style.color = palette['--color-primary']!
        document.body.append(primary)
        const rgb = getComputedStyle(primary)
          .color.match(/[\d.]+/g)!
          .map(Number)
        primary.remove()
        for (const surface of isDark
          ? [
              [32, 36, 43],
              [45, 49, 57],
            ]
          : [
              [255, 255, 255],
              [246, 247, 249],
              [232, 234, 238],
            ]) {
          const background = rgb.map((channel, i) => Math.round(channel * 0.1 + surface[i]! * 0.9))
          expect(
            contrast(palette['--goadmin-selection-foreground']!, `rgb(${background.join(' ')})`),
            `${pack}/${isDark ? 'dark' : 'light'} over ${surface.join(',')}`,
          ).toBeGreaterThanOrEqual(4.5)
        }
        applyBrand({ ...emptyBrand, accentColor: '#ff0000', accentColorEnd: '#00ff00' }, isDark)
        applyBrand(emptyBrand, isDark)
        expect(root.style.getPropertyValue('--color-primary')).toBe('')
        expect(root.style.getPropertyValue('--color-ring')).toBe('')
        expect(getComputedStyle(root).getPropertyValue('--color-primary').trim()).toBe(
          isDark ? dark : light,
        )
        expect(getComputedStyle(root).getPropertyValue('--color-ring').trim()).toBe(
          isDark ? dark : light,
        )
        expect(root.style.getPropertyValue('--goadmin-selection-foreground')).toBe(
          palette['--goadmin-selection-foreground'],
        )
      }
    } finally {
      applyBrand(emptyBrand, false)
      sheet.remove()
      for (const [name, value] of attributes) {
        if (value === null) root.removeAttribute(name)
        else root.setAttribute(name, value)
      }
    }
  },
)

it('keeps CSS-only colors in both modes when the host overrides only a dark accent token', () => {
  const sheet = document.createElement('style')
  sheet.textContent = 'html.dark { --color-ring: #9bc123; }'
  document.head.append(sheet)
  try {
    const light = deriveBrandPalette('', false)
    const dark = deriveBrandPalette('', true)
    expect(light['--goadmin-primary-image']).toBe('none')
    expect(dark['--goadmin-primary-image']).toBe('none')
    expect(dark['--color-ring']).toBe('#9bc123')
    applyBrand(emptyBrand, false)
    expect(document.documentElement.style.getPropertyValue('--goadmin-primary-image')).toBe('')
  } finally {
    sheet.remove()
  }
})

it('compares rewritten stock colors by RGBA and preserves an actual alpha override', () => {
  const sheet = document.createElement('style')
  sheet.textContent = `html:not(.dark) { --color-primary-contrast: #fff; --color-primary-foreground: white;
    --color-badge-primary-bg: hsla(217, 69%, 94%, .9); }`
  document.head.append(sheet)
  try {
    expect(deriveBrandPalette('', false)['--goadmin-primary-image']).toContain('135deg')
    sheet.textContent = sheet.textContent.replace('.9)', '.8)')
    expect(deriveBrandPalette('', false)['--goadmin-primary-image']).toBe('none')
  } finally {
    sheet.remove()
  }
})

it('repaints both directions when a host stylesheet media condition changes', () => {
  const root = document.documentElement
  const sheet = document.createElement('style')
  sheet.textContent = 'html { --color-primary: #065f46; }'
  document.head.append(sheet)
  Object.defineProperty(sheet.sheet, 'media', { value: { mediaText: '(min-width: 900px)' } })
  const breakpoint = new EventTarget()
  let wide = false
  Object.defineProperty(breakpoint, 'matches', { get: () => wide })
  vi.stubGlobal('matchMedia', () => breakpoint)
  const repaint = () => applyBrand(emptyBrand, false)
  const stop = observeBrandMedia(repaint)
  const image = '--goadmin-primary-image'
  try {
    repaint()
    expect(root.style.getPropertyValue(image)).toContain('135deg')
    wide = true
    breakpoint.dispatchEvent(new Event('change'))
    expect(root.style.getPropertyValue(image)).toBe('')
    expect(deriveBrandPalette('', true)['--color-primary']).toBe('#065f46')
    wide = false
    breakpoint.dispatchEvent(new Event('change'))
    expect(root.style.getPropertyValue(image)).toContain('135deg')
  } finally {
    stop()
    vi.unstubAllGlobals()
    sheet.remove()
  }
})

it('leaves the live palette alone when an active stylesheet is unreadable', () => {
  const root = document.documentElement
  const unreadableSheet = {
    get cssRules() {
      throw new DOMException('Cross-origin stylesheet', 'SecurityError')
    },
  }
  const sheets = vi
    .spyOn(document, 'styleSheets', 'get')
    .mockReturnValue([...document.styleSheets, unreadableSheet] as unknown as StyleSheetList)
  try {
    applyBrand(emptyBrand, false)
    expect(root.style.getPropertyValue('--goadmin-primary-image')).toBe('')
    expect(root.style.getPropertyValue('--goadmin-selection-foreground')).toBe('')
    expect(deriveBrandPalette('', true)['--goadmin-primary-image']).toBe('none')
  } finally {
    sheets.mockRestore()
  }
})

it('preserves host fills on reset and uses the correct fill per mode in isolated previews', () => {
  const root = document.documentElement
  const sheet = document.createElement('style')
  const light = 'linear-gradient(90deg, #075985, #172554)'
  const dark = 'linear-gradient(90deg, #0c4a6e, #1e1b4b)'
  sheet.textContent = `
    html { --goadmin-primary-image: ${light}; --goadmin-primary-fill-foreground: #ffffff; }
    html.dark { --goadmin-primary-image: ${dark}; }
  `
  document.head.append(sheet)
  try {
    const append = vi.spyOn(document.body, 'append')
    applyBrand({ ...emptyBrand, accentColor: '#a21caf', accentColorEnd: '#4a044e' }, false)
    expect(deriveBrandPalette('', true)).toMatchObject({
      '--goadmin-primary-image': dark,
      '--goadmin-primary-fill-foreground': '#ffffff',
    })
    expect(deriveBrandPalette('', false)['--goadmin-primary-image']).toBe(light)
    applyBrand({ ...emptyBrand, accentColor: '#123456' }, false)
    expect(root.style.getPropertyValue('--goadmin-primary-image')).toBe('none')
    applyBrand(emptyBrand, false)
    expect(root.style.getPropertyValue('--goadmin-primary-image')).toBe('')
    expect(getComputedStyle(root).getPropertyValue('--goadmin-primary-image').trim()).toBe(light)
    expect(append.mock.calls.filter(([node]) => node instanceof HTMLIFrameElement)).toHaveLength(1)
    append.mockRestore()
  } finally {
    vi.restoreAllMocks()
    sheet.remove()
  }
})

it('resolves both host pack palettes without inheriting the applied live custom accent', () => {
  const root = document.documentElement
  const before = root.outerHTML.slice(0, root.outerHTML.indexOf('>') + 1)
  const theme = root.getAttribute('data-admin-theme')
  const styleAttribute = root.getAttribute('style')
  const classes = root.getAttribute('class')
  const mode = root.getAttribute('data-theme-mode')
  const sheet = document.createElement('style')
  sheet.textContent = `
    :root[data-admin-theme='test.teal'] {
      --color-primary: #0f766e;
      --color-primary-contrast: #ffffff;
    }
    :root[data-admin-theme='test.teal'].dark {
      --color-primary: #80e5db;
      --color-primary-contrast: #0e2522;
    }
  `
  document.head.append(sheet)
  try {
    root.dataset.adminTheme = 'test.teal'
    root.classList.add('dark')
    root.dataset.themeMode = 'dark'
    applyBrand({ ...emptyBrand, accentColor: '#ff00ff' }, true)
    const live = root.style.cssText
    expect(deriveBrandPalette('', false)).toMatchObject({
      '--color-primary': '#0f766e',
      '--color-primary-contrast': '#ffffff',
    })
    expect(deriveBrandPalette('', true)).toMatchObject({
      '--color-primary': '#80e5db',
      '--color-primary-contrast': '#0e2522',
    })
    expect(root.style.cssText).toBe(live)
    expect(root.classList.contains('dark')).toBe(true)
    expect(document.querySelectorAll('iframe')).toHaveLength(0)
  } finally {
    sheet.remove()
    for (const [name, value] of [
      ['class', classes],
      ['data-admin-theme', theme],
      ['style', styleAttribute],
      ['data-theme-mode', mode],
    ]) {
      if (value === null) root.removeAttribute(name!)
      else root.setAttribute(name!, value!)
    }
  }
  expect(root.outerHTML.slice(0, root.outerHTML.indexOf('>') + 1)).toBe(before)
})

it('resolves an oklch theme token through canvas without using the live override', () => {
  const root = document.documentElement
  const oldTheme = root.getAttribute('data-admin-theme')
  const oldStyle = root.getAttribute('style')
  const sheet = document.createElement('style')
  sheet.textContent = ":root[data-admin-theme='test.oklch'] { --color-primary: oklch(65% .18 35); }"
  document.head.append(sheet)
  root.dataset.adminTheme = 'test.oklch'
  let color = ''
  const assigned: string[] = []
  const context = {
    get fillStyle() {
      return color
    },
    set fillStyle(value: string) {
      color = value
      assigned.push(value)
    },
    clearRect() {},
    fillRect() {},
    getImageData: () => ({ data: new Uint8ClampedArray([212, 102, 70, 255]) }),
  }
  const spy = vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(context as never)
  try {
    applyBrand({ ...emptyBrand, accentColor: '#ff00ff' }, false)
    expect(themePrimaryHex(false)).toBe('#d46646')
    expect(assigned).toContain('oklch(65% .18 35)')
    expect(assigned).not.toContain('#ff00ff')
  } finally {
    spy.mockRestore()
    sheet.remove()
    if (oldTheme === null) root.removeAttribute('data-admin-theme')
    else root.setAttribute('data-admin-theme', oldTheme)
    if (oldStyle === null) root.removeAttribute('style')
    else root.setAttribute('style', oldStyle)
  }
})

it('temporarily replaces host inline accent and restores its value and priority on reset', () => {
  const style = document.documentElement.style
  const hostImage = 'linear-gradient(100deg, #14372b, #2e3713)'
  style.setProperty('--goadmin-primary-image', hostImage, 'important')
  style.setProperty('--color-primary', '#136793')
  try {
    applyBrand({ ...emptyBrand, accentColor: '#a21caf' }, false)
    expect(style.getPropertyValue('--goadmin-primary-image')).toBe('none')
    expect(deriveBrandPalette('', true)).toMatchObject({
      '--goadmin-primary-image': hostImage,
      '--color-primary': '#136793',
    })
    applyBrand(emptyBrand, true)
    expect(style.getPropertyValue('--goadmin-primary-image')).toBe(hostImage)
    expect(style.getPropertyPriority('--goadmin-primary-image')).toBe('important')
    expect(style.getPropertyValue('--color-primary')).toBe('#136793')
  } finally {
    style.removeProperty('--goadmin-primary-image')
    style.removeProperty('--color-primary')
  }
})

it.each([false, true])('keeps whole gradient contrast and solid text tokens in dark=%s', (dark) => {
  for (const [start, end] of [
    ['#000000', '#ffffff'],
    ['#ff0000', '#00ff00'],
    ['#0000ff', '#ffff00'],
    ['#ffffff', '#ffffff'],
    ['#010101', '#020202'],
    ['#38BDF8', '#1E3A8A'],
  ]) {
    const palette = deriveBrandPalette(start!, dark, end!)
    expect(palette['--color-primary']).toBe(deriveBrandPalette(start!, dark)['--color-primary'])
    expect(palette['--goadmin-primary-image']).toContain('135deg in srgb')
    const channels = (key: string) => palette[key]!.match(/\d+/g)!.map(Number)
    const a = channels('--goadmin-primary-fill-start')
    const b = channels('--goadmin-primary-fill-end')
    for (let step = 0; step <= 100; step++) {
      const color = `rgb(${a.map((channel, i) => channel + ((b[i]! - channel) * step) / 100).join(' ')})`
      expect(contrast(color, palette['--goadmin-primary-fill-foreground']!)).toBeGreaterThanOrEqual(
        4.99,
      )
    }
  }
})
it('overrides inherited gradient fills for an explicit solid accent', () => {
  expect(deriveBrandPalette('#123456', false)['--goadmin-primary-image']).toBe('none')
})

it.each([false, true])(
  'switches decorative backgrounds from custom through solid to Ocean in dark=%s',
  (dark) => {
    const gradient = { ...emptyBrand, accentColor: '#1ca698', accentColorEnd: '#d04f47' }
    applyBrand(gradient, dark)
    expect(
      document.documentElement.style.getPropertyValue('--goadmin-brand-surface-image'),
    ).toContain('linear-gradient')
    expect(document.documentElement.style.getPropertyValue('--goadmin-brand-auth-image')).toContain(
      'linear-gradient',
    )
    applyBrand({ ...gradient, accentColorEnd: '' }, dark)
    expect(document.documentElement.style.getPropertyValue('--goadmin-brand-surface-image')).toBe(
      'none',
    )
    expect(document.documentElement.style.getPropertyValue('--goadmin-brand-auth-image')).toBe(
      'none',
    )
    applyBrand(emptyBrand, dark)
    expect(document.documentElement.style.getPropertyValue('--goadmin-brand-surface-image')).toBe(
      deriveBrandPalette('', dark)['--goadmin-brand-surface-image'],
    )
    expect(document.documentElement.style.getPropertyValue('--goadmin-row-accent')).toBe(
      deriveBrandPalette('', dark)['--goadmin-row-accent'],
    )
  },
)

it.each([false, true])(
  'keeps selection text readable over the final tinted backgrounds in dark=%s',
  (dark) => {
    const channels = (color: string) => color.match(/[\d.]+/g)!.map(Number)
    const blend = (front: number[], back: number[]) =>
      front.map((channel, i) => Math.round(channel * 0.1 + back[i]! * 0.9))
    const surfaces = dark
      ? [
          [0, 0, 0],
          [32, 36, 43],
          [51, 51, 51],
          [45, 49, 57],
        ]
      : [
          [255, 255, 255],
          [246, 247, 249],
          [232, 234, 238],
        ]
    for (const [start, end] of [
      ['#ff0000', ''],
      ['#ffffff', ''],
      ['#000000', ''],
      ['#ff0000', '#ff0000'],
      ['#ff0000', '#00ff00'],
      ['#0000ff', '#ffff00'],
      ['#00ffff', '#ff00ff'],
      ['#000000', '#ffffff'],
      ['#ffffff', '#000000'],
      ['#38BDF8', '#1E3A8A'],
    ]) {
      const palette = deriveBrandPalette(start!, dark, end!)
      const primary = channels(palette['--color-primary']!)
      const foreground = palette['--goadmin-selection-foreground'] || palette['--color-primary']!
      const textLuminance = luminance(foreground)
      // These are the actual alpha-image endpoints, not the opaque button fill.
      const stops = [...palette['--goadmin-selection-image']!.matchAll(/rgb\(([^)]+)\)/g)].map(
        (match) => channels(match[1]!),
      )
      for (const surface of surfaces) {
        // Hover may replace bg-primary/10 with a plain surface-variant color.
        for (const base of [blend(primary, surface), surface]) {
          for (let step = 0; step <= 100; step++) {
            const background = end
              ? blend(
                  stops[0]!.map((channel, i) => channel + ((stops[1]![i]! - channel) * step) / 100),
                  base,
                )
              : base
            expect(
              luminanceContrast(textLuminance, rgbLuminance(background)),
              `${start}/${end || 'solid'} at ${step}% over ${base.join(',')}`,
            ).toBeGreaterThanOrEqual(4.5)
          }
        }
      }
    }
  },
)
