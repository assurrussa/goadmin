import test from 'node:test'
import assert from 'node:assert/strict'
import { demoPage, views, presets, participants, sessions, seedDate } from './data.mjs'
import { readFileSync } from 'node:fs'
test('all presentation screens resolve with bounded themes and synthetic capabilities', () => {
  for (const view of views)
    for (const preset of presets) {
      const p = demoPage(`/demo/${view}?preset=${preset}&mode=dark`)
      assert.equal(p.props.view, view)
      assert.equal(p.props.preset, preset)
      assert.equal(p.props.mode, 'dark')
      assert.deepEqual(p.props.adminCapabilities, {})
      assert.ok(p.props.adminMenu.sections[0].items.every((i) => i.href.startsWith('/demo/')))
    }
  assert.equal(demoPage('/demo/missing'), null)
  assert.equal(demoPage('/demo/dashboard?preset=unknown').props.preset, 'compact')
  assert.equal(demoPage('/demo/dashboard?mode=unknown').props.mode, 'light')
})
test('seed is deterministic and identifiers are unique', () => {
  assert.equal(seedDate, '2026-10-19')
  assert.equal(new Set(participants.map((p) => p.id)).size, 24)
  assert.equal(sessions.length, 4)
  assert.deepEqual(demoPage('/demo/dashboard'), demoPage('/demo/dashboard'))
})
test('production entry has no presentation import', () => {
  const entry = readFileSync(new URL('../../src/js/app.ts', import.meta.url), 'utf8')
  assert.doesNotMatch(entry, /sports-club|SportsClubDemo/)
})

// Resolving/creating the demo's empty sources must never overwrite host generation.
test('demo scaffold is temporary, aliased, and leaves generated host bytes unchanged', async () => {
  const { createDemoScaffold } = await import('./scaffold.mjs')
  const { existsSync, readdirSync, realpathSync } = await import('node:fs')
  const { resolve } = await import('node:path')
  const host = new URL('../../.admin-extensions/', import.meta.url)
  const snapshot = () =>
    existsSync(host)
      ? readdirSync(host, { recursive: true })
          .filter((f) => !f.endsWith('/'))
          .map((f) => {
            try {
              return [f, readFileSync(new URL(f, host), 'base64')]
            } catch {
              return [f, null]
            }
          })
      : null
  const serverSource = readFileSync(new URL('./server.mjs', import.meta.url), 'utf8')
  assert.match(serverSource, /createDemoScaffold\(\)/)
  assert.doesNotMatch(serverSource, /generatedRoot|\.admin-extensions/)
  const before = snapshot(),
    scaffold = createDemoScaffold()
  try {
    assert.ok(!scaffold.directory.startsWith(resolve(new URL('../..', import.meta.url).pathname)))
    assert.ok(existsSync(scaffold.directory))
    const { resolveConfig } = await import('vite')
    const config = await resolveConfig(
      {
        configFile: false,
        root: new URL('../..', import.meta.url).pathname,
        resolve: { alias: scaffold.aliases },
      },
      'serve',
    )
    const resolved = await config.createResolver()(
      '../../.admin-extensions/tailwind-sources.css',
      new URL('../../src/css/app.css', import.meta.url).pathname,
    )
    assert.equal(realpathSync(resolved), realpathSync(scaffold.aliases[0].replacement))
    assert.deepEqual(snapshot(), before)
  } finally {
    scaffold.cleanup()
  }
  assert.equal(existsSync(scaffold.directory), false)
  assert.deepEqual(snapshot(), before)
})
test('curated captures use viewport images including the README hero', async () => {
  const { shots, screenshotOptions } = await import('./capture-config.mjs')
  assert.equal(shots.length, 7)
  assert.equal(shots[0][0], 'dashboard-compact-light')
  assert.equal(screenshotOptions.fullPage, false)
  assert.ok(!shots.some((s) => s[1] === 'queue' || s[1] === 'settings'))
})
