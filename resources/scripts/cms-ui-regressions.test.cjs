const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const test = require('node:test')

const richText = fs.readFileSync(
  path.join(__dirname, '..', 'src', 'js', 'components', 'form', 'RichTextEditor.vue'),
  'utf8',
)
const imageUploader = fs.readFileSync(
  path.join(__dirname, '..', 'src', 'js', 'components', 'form', 'ImageUploader.vue'),
  'utf8',
)
const appCSS = fs.readFileSync(path.join(__dirname, '..', 'src', 'css', 'app.css'), 'utf8')

test('keeps the rich-text toolbar visible while scrolling on desktop', () => {
  const tabletRules = richText.slice(
    richText.indexOf('@media (max-width: 1024px)'),
    richText.indexOf('/* Editor Content */'),
  )
  assert.match(richText, /\.rich-text-editor\s*\{[^}]*overflow: visible/s)
  assert.match(richText, /\.editor-toolbar\s*\{[^}]*position: sticky[^}]*top: 0/s)
  assert.match(
    richText,
    /@media \(max-width: 1024px\)\s*\{[^}]*\.editor-toolbar\s*\{[^}]*position: static/s,
  )
  assert.doesNotMatch(tabletRules, /\.mobile-hidden\s*\{\s*display: none/)
  assert.match(richText, /@media \(max-width: 640px\)[\s\S]*?\.mobile-hidden\s*\{\s*display: none/)
  assert.match(richText, /class="toolbar-mobile-toggle sm:hidden"/)
  assert.match(richText, /:aria-controls="toolbarContentId"/)
  assert.match(richText, /const toolbarContentId = useId\(\)/)
})

test('gives programmatic media inputs stable form identities', () => {
  assert.match(richText, /:id="fileInputId"/)
  assert.match(richText, /name="rich-text-media"/)
  assert.match(richText, /:aria-label="t\('Добавить изображение или видео'\)"/)
  assert.match(imageUploader, /:id="fileInputId"/)
  assert.match(imageUploader, /name="image-upload"/)
  assert.match(imageUploader, /:aria-label="t\('Загрузить изображение'\)"/)
})

test('uses readable dark-theme semantic tokens', () => {
  const root = appCSS.match(/:root\s*\{([^}]+)\}/)?.[1]
  const dark = appCSS.match(/\.dark,\s*\[data-theme-mode='dark'\]\s*\{([^}]+)\}/)?.[1]
  assert.ok(root && dark, 'canonical root and manual dark palettes must exist')
  const declarations = (block) =>
    Object.fromEntries(
      [...block.matchAll(/(--[\w-]+):\s*([^;]+);/g)].map((match) => [match[1], match[2]]),
    )
  const tokens = { ...declarations(root), ...declarations(dark) }
  const resolve = (value, seen = new Set()) =>
    value.replace(/var\((--[\w-]+)\)/g, (_, name) => {
      assert.ok(tokens[name], `missing token ${name}`)
      assert.ok(!seen.has(name), `cyclic token ${name}`)
      return resolve(tokens[name], new Set([...seen, name]))
    })
  const luminance = (name) => {
    assert.ok(tokens[name], `missing token ${name}`)
    const value = resolve(tokens[name])
    const hsl = value.match(/^hsl\(\s*([\d.]+)[,\s]+([\d.]+)%[,\s]+([\d.]+)%\s*\)$/)
    assert.ok(hsl, `${name} must resolve to an opaque HSL color, got ${value}`)
    const [, hue, saturation, lightness] = hsl.map(Number)
    assert.ok(hue >= 0 && hue <= 360 && saturation <= 100 && lightness <= 100)
    const s = saturation / 100
    const l = lightness / 100
    const a = s * Math.min(l, 1 - l)
    const channel = (n) => {
      const k = (n + hue / 30) % 12
      const c = l - a * Math.max(-1, Math.min(k - 3, 9 - k, 1))
      return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
    }
    return 0.2126 * channel(0) + 0.7152 * channel(8) + 0.0722 * channel(4)
  }
  // Normal-size text must remain readable on every canonical dark surface.
  for (const text of ['primary', 'secondary', 'tertiary']) {
    for (const background of ['background', 'card', 'surface', 'surface-variant']) {
      const foreground = luminance(`--color-text-${text}`)
      const backdrop = luminance(`--color-${background}`)
      const ratio =
        (Math.max(foreground, backdrop) + 0.05) / (Math.min(foreground, backdrop) + 0.05)
      assert.ok(ratio >= 4.5, `${text} text on ${background}: ${ratio.toFixed(2)}:1 is below 4.5:1`)
    }
  }
  // Inactive controls are exempt from normal text contrast. Borders are not text.
  // Validate their tokens and disabled wiring without imposing a text threshold.
  luminance('--color-text-disabled')
  luminance('--color-border-primary')
  assert.match(appCSS, /input:disabled,[\s\S]*color: var\(--color-text-disabled\)/)
  assert.match(appCSS, /html\.dark,[\s\S]*color-scheme: dark/)
  assert.match(
    appCSS,
    /input:not\(\[type='checkbox'\]\):not\(\[type='radio'\]\),[\s\S]*color: var\(--color-text-primary\)/,
  )
  assert.match(appCSS, /--color-card-foreground: var\(--color-text-primary\)/)
  assert.match(appCSS, /--color-popover-foreground: var\(--color-text-primary\)/)
  assert.match(appCSS, /--color-secondary-foreground: var\(--color-text-primary\)/)
  assert.match(appCSS, /--color-muted-foreground: var\(--color-text-secondary\)/)
  assert.match(appCSS, /--color-accent-foreground: var\(--color-text-primary\)/)
  assert.match(appCSS, /--color-border: var\(--color-border-primary\)/)
})
