import { expect, it, vi } from 'vitest'
import { applyBrand, deriveBrandPalette, emptyBrand, themePrimaryHex } from './brand'

function luminance(color: string) {
  const element = document.createElement('span')
  element.style.color = color
  document.body.append(element)
  const rgb = getComputedStyle(element)
    .color.match(/[\d.]+/g)!
    .slice(0, 3)
    .map(Number)
  element.remove()
  return rgb.reduce((sum, channel, index) => {
    const value = channel / 255
    const linear = value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4
    return sum + linear * [0.2126, 0.7152, 0.0722][index]!
  }, 0)
}
function contrast(a: string, b: string) {
  const values = [luminance(a), luminance(b)].sort((a, b) => a - b)
  return (values[1]! + 0.05) / (values[0]! + 0.05)
}
it.each([false, true])(
  'provides AA link and button contrast for an unset accent in dark=%s previews',
  (dark) => {
    const palette = deriveBrandPalette('', dark)
    const primary = palette['--color-primary']!
    expect(contrast(primary, dark ? '#20242b' : '#ffffff')).toBeGreaterThanOrEqual(4.5)
    expect(contrast(primary, palette['--color-primary-contrast']!)).toBeGreaterThanOrEqual(4.5)
  },
)

it.each([
  ['default', 'hsl(217 69% 46%)', 'hsl(217 69% 74%)'],
  ['topbar', 'hsl(174 78% 26%)', 'hsl(174 66% 70%)'],
])(
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
    root.style.setProperty('--color-primary', '#ff00ff')
    root.style.setProperty('--color-primary-contrast', '#000000')
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
  root.style.setProperty('--color-primary', '#ff00ff')
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

it.each([false, true])('keeps whole gradient contrast and solid text tokens in dark=%s', (dark) => {
  for (const [start, end] of [
    ['#000000', '#ffffff'],
    ['#ff0000', '#00ff00'],
    ['#0000ff', '#ffff00'],
    ['#ffffff', '#ffffff'],
    ['#010101', '#020202'],
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
it('overrides inherited gradient fills in an unset preview and uses none for a solid', () => {
  expect(deriveBrandPalette('', false)['--goadmin-primary-image']).toBe('none')
  expect(deriveBrandPalette('#123456', false)['--goadmin-primary-image']).toBe('none')
})

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
    ]) {
      const palette = deriveBrandPalette(start!, dark, end!)
      const primary = channels(palette['--color-primary']!)
      const foreground = palette['--goadmin-selection-foreground'] || palette['--color-primary']!
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
              contrast(foreground, `rgb(${background.join(' ')})`),
              `${start}/${end || 'solid'} at ${step}% over ${base.join(',')}`,
            ).toBeGreaterThanOrEqual(4.5)
          }
        }
      }
    }
  },
)
