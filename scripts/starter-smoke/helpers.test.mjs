import { test } from 'node:test'
import assert from 'node:assert/strict'
import { terminalUpload, taskStatus, deferred, deadline } from './helpers.mjs'
import { parseCSV, tableFixtureSQL } from './table-workflow.mjs'

test('only genuine upload terminal messages open the upload evidence gate', () => {
  for (const message of ['invalid', '{}', '{"status":"completed"}',
    '{"eventType":"files.upload.status","status":"processing"}',
    '{"eventType":"file.after.process","status":"completed"}']) {
    assert.equal(terminalUpload(message), null)
  }
  const message = { eventType: 'files.upload.status', status: 'completed', taskId: 17,
    file: { id: 9, url: '/uploads/media/v1/avatar.jpg' } }
  assert.deepEqual(terminalUpload(Buffer.from(JSON.stringify(message))), message)
})

test('navigation gate releases original evidence without replacing payload', async () => {
  const gate = deferred()
  const payload = Buffer.from('{"real":"server"}')
  let delivered = false
  const pending = gate.promise.then(() => { delivered = true; return payload })
  await Promise.resolve()
  assert.equal(delivered, false)
  gate.resolve()
  assert.equal(await deadline(pending, 'delivery'), payload)
})

test('terminal polling gate recognizes the canonical file envelope without changing it', () => {
  const queued = { file: { id: 1, status: 'queued', url: '' } }
  const completed = { file: { id: 1, status: 'completed', url: '/uploads/media/v1/main.png' } }
  const original = JSON.stringify(completed)
  assert.equal(taskStatus(queued), 'queued')
  assert.equal(taskStatus(completed), 'completed')
  assert.equal(taskStatus({ status: 'failed', task: { id: 1 } }), 'failed')
  assert.equal(taskStatus({ task: { status: 'processing', file: { url: '/ready.png' } } }), 'completed')
  assert.equal(JSON.stringify(completed), original)
})


test('table CSV evidence preserves quoted commas, quotes, newlines, exact IDs and UTF8 headers', () => {
  assert.deepEqual(parseCSV('\uFEFFИмя,ID,Email\r\n"Doe, Jane",9007199254740993,"a""b@example.test"\r\n"line1\nline2",2,plain\n'), [
    ['Имя', 'ID', 'Email'], ['Doe, Jane', '9007199254740993', 'a"b@example.test'], ['line1\nline2', '2', 'plain'],
  ])
  assert.deepEqual(parseCSV('Header\n"ends\r"\n'), [['Header'], ['ends\r']])
  assert.deepEqual(parseCSV('a,b\n'), [['a', 'b']])
  assert.throws(() => parseCSV('"unterminated'), /unterminated/)
})

test('table smoke fixtures remain credentialless and never add admin membership or roles', () => {
  const sql = tableFixtureSQL('topbar')
  assert.match(sql, /generate_series\(1,105\)/)
  for (const table of ['auth_subjects', 'auth_identifiers', 'auth_basic_profiles', 'users', 'jobs', 'jobs_failed']) assert.ok(sql.includes(`INSERT INTO ${table}`))
  assert.doesNotMatch(sql, /INSERT INTO (?:administrations|auth_local_credentials|.*roles|.*permissions)/i)
  assert.match(sql, /available_at.*created_at[\s\S]*interval '1 day'/)
  assert.match(sql, /table-workflow-topbar-failed/)
  assert.throws(() => tableFixtureSQL("bad'; DELETE FROM users; --"))
})
