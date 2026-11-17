import { test } from 'node:test'
import assert from 'node:assert/strict'
import { paintViolations } from './paint.mjs'
const color = (rgb) => ({ css: `rgb(${rgb.join(', ')})`, rgba: [...rgb, 255] })
const paint = (accent, background, foreground, image = 'none') => ({
  edit: { background: color(accent), foreground: color([255, 255, 255]), image },
  card: { background: color(background), foreground: color(foreground) },
  shell: { background: color(background), foreground: color(foreground) },
})

test('Ocean is visible in the default presentation and leaves the topbar solid', () => {
  const blue = paint([0, 117, 176], [255, 255, 255], [35, 39, 47],
    'linear-gradient(135deg, rgb(0, 117, 176), rgb(30, 58, 138))')
  assert.match(paintViolations(blue, 'topbar', 'light').join(';'), /Edit accent/)
  assert.deepEqual(paintViolations(blue, 'default', 'light'), [])
  blue.edit.image = 'none'
  assert.match(paintViolations(blue, 'default', 'light').join(';'), /Edit fill/)
})

test('correct dark accent alone cannot hide light surfaces and dark foreground', () => {
  const light = paint([15, 118, 108], [255, 255, 255], [35, 39, 47])
  const broken = paint([128, 229, 219], [255, 255, 255], [35, 39, 47])
  const failures = paintViolations(broken, 'topbar', 'dark', light)
  assert.ok(failures.includes('card: dark background did not darken'))
  assert.ok(failures.includes('shell: dark foreground did not lighten'))
  assert.deepEqual(paintViolations(paint([128, 229, 219], [25, 29, 35], [231, 235, 242]),
    'topbar', 'dark', light), [])
})
