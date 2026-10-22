import { test } from 'node:test'
import assert from 'node:assert/strict'
import { terminalUpload, taskStatus, deferred, deadline } from './helpers.mjs'

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
