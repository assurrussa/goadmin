const assert = require('node:assert/strict')
const fs = require('node:fs')
const os = require('node:os')
const path = require('node:path')
const test = require('node:test')
const {
  fingerprintSource,
  materializeAdminExtensions,
} = require('./materialize-admin-extensions.cjs')
for (const name of ['compact', 'comfortable']) {
  test(`${name} density materializes as an opt-in CSS-only extension`, (t) => {
    const root = path.resolve(__dirname, `../examples/theme-${name}`)
    const manifestPath = path.join(root, 'extension.json')
    const manifest = JSON.parse(fs.readFileSync(manifestPath, 'utf8')).extensions[0]
    assert.equal(manifest.fingerprint, fingerprintSource(path.join(root, 'src')))
    const generatedRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'goadmin-density-'))
    t.after(() => fs.rmSync(generatedRoot, { recursive: true, force: true }))
    const result = materializeAdminExtensions({ generatedRoot, manifestPaths: [manifestPath] })
    assert.equal(result.bundleManifest.extensions.length, 1)
    assert.equal(result.bundleManifest.extensions[0].key, `example.${name}-theme`)
    assert.equal(
      fs.readFileSync(path.join(root, 'src/extensions/theme.ts'), 'utf8'),
      "import '../theme.css'\nexport default {}\n",
    )
    assert.equal(
      materializeAdminExtensions({
        generatedRoot,
        manifestPaths: [],
        legacyRoot: path.join(generatedRoot, 'absent'),
      }).bundleManifest.extensions.length,
      0,
    )
  })
  test(`${name} density preserves semantic styling and supplies touch dimensions`, () => {
    const css = fs.readFileSync(
      path.resolve(__dirname, `../examples/theme-${name}/src/theme.css`),
      'utf8',
    )
    const allowed = new Set(
      [
        'header-height',
        'sidebar-width',
        'sidebar-collapsed-width',
        'content-max-width',
        'menu-radius',
        'control-height-xs',
        'control-height-sm',
        'control-height',
        'control-height-lg',
        'control-height-xl',
        'control-radius',
        'control-radius-xs',
        'textarea-min-height',
        'card-radius',
        'card-padding',
      ].map((token) => `--goadmin-${token}`),
    )
    assert.ok([...css.matchAll(/(--[\w-]+)\s*:/g)].every((match) => allowed.has(match[1])))
    assert.doesNotMatch(css, /(?:color|font|shadow|outline|transition)\s*:/)
    const touch = css.slice(css.indexOf('@media'))
    assert.match(touch, /\(max-width: 767px\), \(pointer: coarse\)/)
    for (const size of ['-xs', '-sm', '', '-lg', '-xl']) {
      const match = touch.match(new RegExp(`--goadmin-control-height${size}: (\\d+)px`))
      assert.ok(match && Number(match[1]) >= 44)
    }
    assert.match(touch, /--goadmin-card-padding: 16px/)
  })
}
