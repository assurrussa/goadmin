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
  assert.match(richText, /aria-label="Добавить изображение или видео"/)
  assert.match(imageUploader, /:id="fileInputId"/)
  assert.match(imageUploader, /name="image-upload"/)
  assert.match(imageUploader, /aria-label="Загрузить изображение"/)
})

test('uses readable dark-theme semantic tokens', () => {
  assert.match(appCSS, /--color-text-secondary: hsl\([^;]*78%\)/)
  assert.match(appCSS, /--color-text-tertiary: hsl\([^;]*66%\)/)
  assert.match(appCSS, /--color-text-disabled: hsl\([^;]*54%\)/)
  assert.match(appCSS, /--color-border-primary: hsl\([^;]*38%\)/)
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
