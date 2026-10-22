import { expect } from 'playwright/test'

// Published presentation examples intentionally use different accent palettes.
const primary = {
  default: { light: [36, 98, 198], dark: [143, 178, 234] },
  topbar: { light: [15, 118, 108], dark: [128, 229, 219] },
}

export function luminance(rgb) {
  const linear = rgb.slice(0, 3).map((value) => {
    const channel = value / 255
    return channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4
  })
  return linear[0] * 0.2126 + linear[1] * 0.7152 + linear[2] * 0.0722
}

export function paintViolations(paint, pack, mode, light) {
  const errors = []
  const expected = primary[pack]?.[mode]
  if (!expected) throw new Error(`Unknown presentation paint contract: ${pack}/${mode}`)
  const actual = paint.edit.background.rgba
  if (actual[3] !== 255 || expected.some((channel, index) => Math.abs(channel - actual[index]) > 2)) {
    errors.push(`Edit accent: expected RGB ${expected.join(',')}, painted ${paint.edit.background.css}`)
  }
  for (const part of ['card', 'shell']) {
    const background = luminance(paint[part].background.rgba)
    const foreground = luminance(paint[part].foreground.rgba)
    if (mode === 'light') {
      if (background < 0.65) errors.push(`${part}: light background is not light`)
      if (foreground > 0.25) errors.push(`${part}: light foreground is not dark`)
    } else {
      if (!light) throw new Error('Dark paint must be compared with measured light paint')
      if (background >= luminance(light[part].background.rgba) * 0.5) {
        errors.push(`${part}: dark background did not darken`)
      }
      if (foreground <= luminance(light[part].foreground.rgba) + 0.2) {
        errors.push(`${part}: dark foreground did not lighten`)
      }
    }
  }
  return errors
}

export async function capturePaint(page) {
  return page.evaluate(() => {
    const edit = document.querySelector('a[href="/auth/profile/edit"]')
    const card = edit?.closest('[data-ui="card"][data-part="root"]')
    const shell = document.querySelector('[data-ui="admin-shell"]')
    if (!edit || !card || !shell) throw new Error('Profile Edit anchor, card and shell must be mounted')
    const canvas = document.createElement('canvas')
    canvas.width = canvas.height = 1
    const drawing = canvas.getContext('2d', { willReadFrequently: true })
    const color = (css) => {
      drawing.clearRect(0, 0, 1, 1)
      drawing.fillStyle = css
      drawing.fillRect(0, 0, 1, 1)
      return { css, rgba: [...drawing.getImageData(0, 0, 1, 1).data] }
    }
    const read = (element) => {
      const computed = getComputedStyle(element)
      return { background: color(computed.backgroundColor), foreground: color(computed.color) }
    }
    return { edit: read(edit), card: read(card), shell: read(shell) }
  })
}

export async function waitForPaint(page, pack, mode, light, record, timeout = 20000) {
  let painted
  await expect.poll(async () => {
    painted = await capturePaint(page)
    record(painted)
    return paintViolations(painted, pack, mode, light)
  }, { timeout, message: `${pack}/${mode}: actual profile paint must match the selected palette` }).toEqual([])
  return painted
}
