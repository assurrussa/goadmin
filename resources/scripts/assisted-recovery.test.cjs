const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const { test } = require('node:test')

const source = fs.readFileSync(path.join(__dirname, '../../host/assisted_recovery.go'), 'utf8')
const script = source.match(/const recoveryInitiationScript = `([\s\S]*?)`/)[1]

async function submit(statusCode, networkFailure = false) {
  let listener
  const status = { textContent: '' }
  const button = { disabled: false }
  const form = {
    action: 'https://admin.test/account-recovery', dataset: { nativeCsrf: 'native-session-token' },
    elements: { password: { value: 'secret' }, code: { value: 'factor' } },
    querySelector: selector => selector === 'button' ? button : status,
    addEventListener: (_, callback) => { listener = callback },
  }
  const requests = []
  vm.runInNewContext(script, {
    document: { querySelector: () => form }, URLSearchParams,
    FormData: class { *[Symbol.iterator]() { yield ['password', 'secret'] } },
    fetch: async (url, options) => {
      requests.push({ url, options })
      if (networkFailure) throw new Error('connection lost')
      return { status: statusCode, ok: statusCode >= 200 && statusCode < 300 }
    },
  })
  await listener({ preventDefault() {} })
  await listener({ preventDefault() {} })
  assert.equal(requests.length, 1, 'submission never automatically replays')
  assert.equal(button.disabled, true)
  assert.equal(requests[0].options.headers['X-CSRF-Token'], 'native-session-token')
  assert.equal(requests[0].options.redirect, 'error')
  return { text: status.textContent, form }
}

for (const code of [500, 502, 503, 504]) {
  test(`HTTP ${code} preserves uncertain issuance outcome`, async () => {
    const { text, form } = await submit(code)
    assert.match(text, /outcome is unknown/i)
    assert.match(text, /service owner.*reconcile/i)
    assert.doesNotMatch(text, /rejected|reload/i)
    assert.equal(form.elements.password.value, '')
    assert.equal(form.elements.code.value, '')
  })
}
test('network failure preserves uncertain outcome', async () => {
  const { text } = await submit(0, true)
  assert.match(text, /outcome is unknown/i)
  assert.doesNotMatch(text, /rejected|reload/i)
})
test('confirmed success and rejection remain distinct', async () => {
  assert.match((await submit(202)).text, /invitation was queued/i)
  assert.match((await submit(400)).text, /rejected/i)
})
