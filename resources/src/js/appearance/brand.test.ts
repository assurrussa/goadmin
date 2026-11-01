import { expect, it, vi } from 'vitest'
import { deriveBrandPalette, themePrimaryHex } from './brand'

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
    expect(deriveBrandPalette('', false)).toEqual({
      '--color-primary': '#0f766e',
      '--color-primary-contrast': '#ffffff',
    })
    expect(deriveBrandPalette('', true)).toEqual({
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
