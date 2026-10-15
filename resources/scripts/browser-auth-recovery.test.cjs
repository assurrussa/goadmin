const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const { test } = require('node:test')
const ts = require('typescript')

const filename = path.join(__dirname, '../src/js/auth/browserAuthRecovery.ts')
const compiled = ts.transpileModule(fs.readFileSync(filename, 'utf8'), {
  fileName: filename,
  reportDiagnostics: true,
  compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
})
assert.equal(compiled.diagnostics?.length ?? 0, 0)
const sandbox = { exports: {}, AbortController, setTimeout, clearTimeout }
vm.runInNewContext(compiled.outputText, sandbox, { filename })
const { recoverBrowserAuthentication, authRecoveryPath } = sandbox.exports

function reply(status = 200, code = 'authentication_recovered', extra = {}) {
  const headers = new Map([
    ['x-goadmin-auth-recovery', '1'],
    ['content-type', 'application/json; charset=utf-8'],
  ])
  return {
    status, redirected: false, headers,
    json: async () => ({ status, code, csrfReady: true }),
    ...extra,
  }
}

test('recovery makes only fixed safe same-origin requests and settles rotated cookies', async () => {
  const calls = []
  const request = async (url, init) => {
    calls.push({ url, init })
    return calls.length === 1 ? reply(409, 'authentication_recovery_continue') : reply()
  }
  const result = await recoverBrowserAuthentication(new AbortController().signal, request)
  assert.equal(result, true)
  assert.equal(authRecoveryPath, '/auth/session/recover')
  assert.equal(calls.length, 2)
  for (const { url, init } of calls) {
    assert.equal(url, authRecoveryPath)
    assert.equal(init.method, 'GET')
    assert.equal(init.credentials, 'same-origin')
    assert.equal(init.mode, 'same-origin')
    assert.equal(init.redirect, 'error')
    assert.equal(init.cache, 'no-store')
    assert.equal(init.headers['X-Goadmin-Auth-Recovery'], '1')
    assert.equal(init.headers.Accept, 'application/json')
    assert.equal(init.body, undefined)
  }
})

test('already settled matching cookies need only one recovery request', async () => {
  let calls = 0
  assert.equal(await recoverBrowserAuthentication(new AbortController().signal, async () => {
    calls++
    return reply()
  }), true)
  assert.equal(calls, 1)
})

test('repeated rotation is bounded to two requests, never retried indefinitely', async () => {
  let calls = 0
  const result = await recoverBrowserAuthentication(new AbortController().signal, async () => {
    calls++
    return reply(409, 'authentication_recovery_continue')
  })
  assert.equal(result, false)
  assert.equal(calls, 2)
})

for (const [name, value] of [
  ['revoked', reply(401, 'reauthentication_required')],
  ['absolute expired', reply(401, 'reauthentication_required')],
  ['different user', reply(403, 'admission_denied')],
  ['dependency unavailable', reply(503, 'authentication_unavailable')],
  ['missing matching CSRF', reply(200, 'authentication_recovered', { json: async () => ({ status: 200, code: 'authentication_recovered', csrfReady: false }) })],
  ['HTML login', reply(200, '', { headers: new Map([['content-type', 'text/html']]) })],
  ['redirect', reply(200, '', { redirected: true })],
  ['wrong status contract', reply(200, '', { json: async () => ({ status: 409, code: 'authentication_recovered', csrfReady: true }) })],
  ['unknown response version', reply(200, '', { headers: new Map([['content-type', 'application/json'], ['x-goadmin-auth-recovery', '2']]) })],
  ['missing marker', reply(200, '', { headers: new Map([['content-type', 'application/json']]) })],
  ['invalid JSON', reply(200, '', { json: async () => { throw new Error('json') } })],
  ['null JSON', reply(200, '', { json: async () => null })],
]) {
  test(`recovery fails closed: ${name}`, async () => {
    let calls = 0
    assert.equal(await recoverBrowserAuthentication(new AbortController().signal, async () => {
      calls++
      return value
    }), false)
    assert.equal(calls, 1)
  })
}

test('network failure does not retry any request', async () => {
  let calls = 0
  assert.equal(await recoverBrowserAuthentication(new AbortController().signal, async () => {
    calls++
    throw new Error('network')
  }), false)
  assert.equal(calls, 1)
})

test('already cancelled lifecycle sends nothing', async () => {
  const controller = new AbortController()
  controller.abort()
  let calls = 0
  assert.equal(await recoverBrowserAuthentication(controller.signal, async () => {
    calls++
    return reply()
  }), false)
  assert.equal(calls, 0)
})

test('cancellation fences a late successful response and prevents the settle request', async () => {
  const controller = new AbortController()
  let calls = 0
  assert.equal(await recoverBrowserAuthentication(controller.signal, async (_url, init) => {
    calls++
    controller.abort()
    assert.equal(init.signal.aborted, true)
    return reply(409, 'authentication_recovery_continue')
  }), false)
  assert.equal(calls, 1)
})
