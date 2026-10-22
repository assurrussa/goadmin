const assert = require('node:assert/strict')
const fs = require('node:fs')
const os = require('node:os')
const path = require('node:path')
const { spawnSync } = require('node:child_process')
const test = require('node:test')
const { verifyThemeLayerOrder } = require('./theme-layer-order.cjs')

// Use the complete production entry/config/extension graph: compiling a CSS
// snippet alone misses Vite hoisting extension CSS before the core stylesheet.
test('production default and topbar builds establish the canonical CSS layer precedence', (t) => {
  const resources = path.resolve(__dirname, '..')
  const temporary = fs.mkdtempSync(path.join(os.tmpdir(), 'goadmin-theme-order-'))
  t.after(() => fs.rmSync(temporary, { recursive: true, force: true }))
  const fixture = path.join(temporary, 'resources')
  fs.cpSync(resources, fixture, {
    recursive: true,
    filter: (source) =>
      !['node_modules', '.admin-extensions', 'dist', 'coverage', '.typecheck-cache'].includes(
        path.basename(source),
      ),
  })
  fs.symlinkSync(
    path.join(resources, 'node_modules'),
    path.join(fixture, 'node_modules'),
    process.platform === 'win32' ? 'junction' : 'dir',
  )

  for (const theme of ['default', 'topbar']) {
    const publicRoot = path.join(temporary, theme)
    const result = spawnSync(
      process.execPath,
      [path.join(resources, 'node_modules/vite/bin/vite.js'), 'build', '--logLevel', 'error'],
      {
        cwd: fixture,
        encoding: 'utf8',
        timeout: 120_000,
        env: {
          ...process.env,
          VITE_ADMIN_EXT_ROOT: '',
          VITE_ADMIN_PUBLIC_ROOT: publicRoot,
          VITE_ADMIN_EXTENSION_MANIFESTS:
            theme === 'topbar' ? path.join(fixture, 'examples/theme-topbar/extension.json') : '',
        },
      },
    )
    assert.ifError(result.error)
    assert.equal(result.status, 0, `${theme}: ${result.stdout}${result.stderr}`)
    const css = fs.readFileSync(path.join(publicRoot, 'dist/css/app.css'), 'utf8')
    t.diagnostic(`${theme}: first layers ${verifyThemeLayerOrder(css).join(', ')}`)
    if (theme === 'topbar') assert.match(css, /data-admin-theme[=]["']?example\.topbar/)
    const verified = spawnSync(
      process.execPath,
      [path.join(fixture, 'scripts/verify-client-assets.cjs')],
      {
        cwd: fixture,
        encoding: 'utf8',
        env: { ...process.env, VITE_ADMIN_PUBLIC_ROOT: publicRoot },
      },
    )
    assert.ifError(verified.error)
    assert.equal(verified.status, 0, `${theme} asset gate: ${verified.stdout}${verified.stderr}`)
  }
})
