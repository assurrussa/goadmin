const assert = require('node:assert/strict')
const fs = require('node:fs')
const os = require('node:os')
const path = require('node:path')
const { test } = require('node:test')
const { fingerprintSource, materializeAdminExtensions } = require('./materialize-admin-extensions.cjs')
const root = path.resolve(__dirname, '../examples/theme-brand')
const css = fs.readFileSync(path.join(root, 'src/theme.css'), 'utf8')
const luminance = (hex) => {
  const c = hex.match(/[a-f0-9]{2}/gi).map(x => parseInt(x, 16) / 255)
    .map(x => x <= 0.04045 ? x / 12.92 : ((x + 0.055) / 1.055) ** 2.4)
  return c[0] * 0.2126 + c[1] * 0.7152 + c[2] * 0.0722
}
const contrast = (a, b) => {
  const x = luminance(a), y = luminance(b)
  return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05)
}
test('primary text pairs meet 4.5:1 in the example palette', () => {
  const blocks = [...css.matchAll(/\{([^{}]+)\}/g)].map(match => match[1])
  assert.equal(blocks.length, 2)
  for (const block of blocks) {
    const color = block.match(/--color-primary:\s*(#[a-f0-9]{6});/i)?.[1]
    const foreground = block.match(/--color-primary-foreground:\s*(#[a-f0-9]{6});/i)?.[1]
    assert.ok(color && foreground, 'missing explicit color pair')
    assert.ok(contrast(color, foreground) >= 4.5)
  }
})
test('branding does not override density, status colors or focus visibility', () => {
  assert.doesNotMatch(css, /--goadmin-(?:control|header|sidebar|card|content)/)
  assert.doesNotMatch(css, /--(?:color-)?(?:success|warning|error|destructive|info)[\w-]*\s*:/)
  assert.doesNotMatch(css, /outline\s*:\s*(?:0|none)|@import|url\(/)
  assert.ok(css.includes(":root[data-theme-mode='dark']"))
})
test('branding composes with compact through verified opt-in manifests', (t) => {
  const manifestPath = path.join(root, 'extension.json')
  const manifest = JSON.parse(fs.readFileSync(manifestPath, 'utf8')).extensions[0]
  assert.equal(manifest.fingerprint, fingerprintSource(path.join(root, 'src')))
  assert.equal(fs.readFileSync(path.join(root, 'src/extensions/theme.ts'), 'utf8'), "import '../theme.css'\nexport default {}\n")
  const generatedRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'goadmin-brand-'))
  t.after(() => fs.rmSync(generatedRoot, { recursive: true, force: true }))
  const result = materializeAdminExtensions({ generatedRoot, manifestPaths: [path.resolve(root, '../theme-compact/extension.json'), manifestPath] })
  assert.deepEqual(result.bundleManifest.extensions.map(e => e.key), ['example.brand-theme', 'example.compact-theme'])
  assert.equal(materializeAdminExtensions({ generatedRoot, manifestPaths: [], legacyRoot: path.join(generatedRoot, 'absent') }).bundleManifest.extensions.length, 0)
})
