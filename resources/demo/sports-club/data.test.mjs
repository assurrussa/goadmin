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
