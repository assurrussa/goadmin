import { test } from 'node:test'
import assert from 'node:assert/strict'
import { paintViolations } from './paint.mjs'
const color = (rgb) => ({ css: `rgb(${rgb.join(', ')})`, rgba: [...rgb, 255] })
const paint = (accent, background, foreground) => ({
  edit: { background: color(accent), foreground: color([255, 255, 255]) },
  card: { background: color(background), foreground: color(foreground) },
  shell: { background: color(background), foreground: color(foreground) },
})

test('paint contract rejects default blue inside the selected topbar presentation', () => {
  const blue = paint([36, 98, 198], [255, 255, 255], [35, 39, 47])
  assert.match(paintViolations(blue, 'topbar', 'light').join(';'), /Edit accent/)
  assert.deepEqual(paintViolations(blue, 'default', 'light'), [])
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
