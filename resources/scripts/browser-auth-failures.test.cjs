const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const { test } = require('node:test')
const ts = require('typescript')

// Execute the actual dependency-free TypeScript policy. This is part of the
// existing scripts/*.test.cjs gate; Vue/Inertia integration has its own Vitest test.
const filename = path.join(__dirname, '../src/js/auth/browserAuthFailures.ts')
const compiled = ts.transpileModule(fs.readFileSync(filename, 'utf8'), {
  fileName: filename,
  reportDiagnostics: true,
  compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
})
assert.equal(compiled.diagnostics?.length ?? 0, 0)
const sandbox = { exports: {}, URL, AbortController }
vm.runInNewContext(compiled.outputText, sandbox, { filename })
const { createAuthFailureHandler } = sandbox.exports

function response(status = 503, code = 'authentication_retry_required') {
  return {
    status,
    data: { status, code, message: 'SERVER_MESSAGE_MUST_NOT_BE_RENDERED' },
    headers: {
      'x-goadmin-auth-error': '1',
      'content-type': 'application/json; charset=utf-8',
      'retry-after': '1',
    },
    config: { url: 'https://admin.test/protected', method: 'post', data: 'sensitive form data' },
  }
}

function eventFor(value) {
  return {
    defaultPrevented: false,
    detail: { response: value },
    preventDefault() { this.defaultPrevented = true },
  }
}

function fixture() {
  const events = []
  const notices = []
  const draft = { email: 'draft@example.test', message: 'unsaved input' }
  let auth = { id: 1 }
  const handler = createAuthFailureHandler({
    origin: 'https://admin.test',
    clearAuth: () => { auth = null; events.push('clear') },
    disconnect: () => events.push('disconnect'),
    navigateToLogin: () => events.push('GET /auth/login'),
    showNotice: (message) => notices.push(message),
  })
  return { handler, events, notices, draft, get auth() { return auth } }
}

test('401 clears auth and disconnects before one separate login GET', () => {
  const f = fixture()
  for (let i = 0; i < 3; i++) {
    const event = eventFor(response(401, 'reauthentication_required'))
    f.handler(event)
    assert.equal(event.defaultPrevented, true)
  }
  assert.equal(f.auth, null)
  assert.deepEqual(f.events, ['clear', 'disconnect', 'GET /auth/login'])
  assert.deepEqual(f.notices, [])
})

for (const code of ['authentication_retry_required', 'authentication_unavailable']) {
  test(`503 ${code} preserves draft and authentication without navigation or replay`, () => {
    const f = fixture()
    const before = { ...f.draft }
    const event = eventFor(response(503, code))
    f.handler(event)
    assert.equal(event.defaultPrevented, true)
    assert.deepEqual(f.events, [])
    assert.deepEqual(f.auth, { id: 1 })
    assert.deepEqual(f.draft, before)
    assert.equal(f.notices.length, 1)
    assert.match(f.notices[0], /вручную/)
    assert.match(f.notices[0], /1 с\./)
    assert.doesNotMatch(f.notices[0], /SERVER_MESSAGE|sensitive/)
  })
}

test('400 is a controlled notice, not a mutation retry or logout', () => {
  const f = fixture()
  const event = eventFor(response(400, 'invalid_browser_credentials'))
  f.handler(event)
  assert.equal(event.defaultPrevented, true)
  assert.deepEqual(f.events, [])
  assert.equal(f.notices.length, 1)
})

test('case-insensitive response headers and same-origin relative URL work', () => {
  const f = fixture()
  const value = response()
  value.headers = { 'X-Goadmin-Auth-Error': '1', 'Content-Type': 'application/json' }
  value.config.url = '/protected'
  const event = eventFor(value)
  f.handler(event)
  assert.equal(event.defaultPrevented, true)
})

test('missing or untrusted Retry-After is not echoed into the notice', () => {
  for (const delay of [undefined, '-1', '0', '1000000', '<script>alert(1)</script>']) {
    const f = fixture()
    const value = response()
    value.headers['retry-after'] = delay
    f.handler(eventFor(value))
    assert.match(f.notices[0], /через несколько секунд/)
  }
})

for (const [name, change] of [
  ['missing marker', (r) => { delete r.headers['x-goadmin-auth-error'] }],
  ['unknown version', (r) => { r.headers['x-goadmin-auth-error'] = '2' }],
  ['foreign origin', (r) => { r.config.url = 'https://other.test/protected' }],
  ['credential URL', (r) => { r.config.url = 'https://user:pass@admin.test/protected' }],
  ['missing URL', (r) => { delete r.config.url }],
  ['HTML response', (r) => { r.headers['content-type'] = 'text/html' }],
  ['Inertia page', (r) => { r.headers['x-inertia'] = 'true' }],
  ['unknown code', (r) => { r.data.code = 'payment_failed' }],
  ['status mismatch', (r) => { r.data.status = 401 }],
  ['wrong HTTP status', (r) => { r.status = 200; r.data.status = 200 }],
  ['non-object JSON', (r) => { r.data = 'unexpected string' }],
  ['missing message', (r) => { delete r.data.message }],
]) {
  test(`unrelated invalid response retains Inertia diagnostics: ${name}`, () => {
    const f = fixture()
    const value = response()
    change(value)
    const event = eventFor(value)
    f.handler(event)
    assert.equal(event.defaultPrevented, false)
    assert.deepEqual(f.events, [])
    assert.deepEqual(f.notices, [])
  })
}

test('already cancelled invalid event is not handled twice', () => {
  const f = fixture()
  const event = eventFor(response())
  event.preventDefault()
  f.handler(event)
  assert.deepEqual(f.events, [])
  assert.deepEqual(f.notices, [])
})

test('late retryable response does not replace the pending login transition', () => {
  const f = fixture()
  f.handler(eventFor(response(401, 'reauthentication_required')))
  const late = eventFor(response())
  f.handler(late)
  assert.equal(late.defaultPrevented, true)
  assert.deepEqual(f.events, ['clear', 'disconnect', 'GET /auth/login'])
  assert.deepEqual(f.notices, [])
})

function recoveryFixture(recover) {
  const calls = []
  const notices = []
  const handler = createAuthFailureHandler({
    origin: 'https://admin.test',
    clearAuth: () => calls.push('clear'),
    disconnect: () => calls.push('disconnect'),
    navigateToLogin: () => calls.push('navigate'),
    showNotice: (value) => notices.push(value),
    recoverAuth: (signal) => { calls.push('recover'); return recover(signal) },
    showLogin: () => calls.push('login-button'),
  })
  const value = response(401, 'reauthentication_required')
  value.headers['x-goadmin-auth-recovery'] = '1'
  return { handler, value, calls, notices }
}

const flush = async () => { await Promise.resolve(); await Promise.resolve(); await Promise.resolve() }

test('opt-in 401 recovers once without clearing, navigating or replaying the command', async () => {
  let finish
  const f = recoveryFixture(() => new Promise((resolve) => { finish = resolve }))
  for (let i = 0; i < 4; i++) f.handler(eventFor(f.value))
  assert.deepEqual(f.calls, ['disconnect', 'recover'])
  finish(true)
  await flush()
  assert.deepEqual(f.calls, ['disconnect', 'recover'])
  assert.match(f.notices.at(-1), /Доступ восстановлен/)
  assert.match(f.notices.at(-1), /ещё раз/)
  // A second failure cannot launch an unbounded refresh loop.
  f.handler(eventFor(f.value))
  assert.equal(f.calls.filter((call) => call === 'recover').length, 1)
  assert.equal(f.calls.includes('navigate'), false)
  assert.equal(f.calls.at(-1), 'login-button')
})

for (const outcome of [false, 'reject']) {
  test(`failed recovery ${outcome} preserves the page and offers explicit login`, async () => {
    const f = recoveryFixture(() => outcome === 'reject' ? Promise.reject(new Error('network')) : Promise.resolve(false))
    f.handler(eventFor(f.value))
    await flush()
    assert.deepEqual(f.calls, ['disconnect', 'recover', 'clear', 'disconnect', 'login-button'])
    assert.match(f.notices.at(-1), /Данные формы остались/)
    assert.equal(f.calls.includes('navigate'), false)
  })
}

test('new navigation aborts old recovery and fences its late response', async () => {
  let finish
  let signal
  const f = recoveryFixture((value) => {
    signal = value
    return new Promise((resolve) => { finish = resolve })
  })
  f.handler(eventFor(f.value))
  f.handler.cancel()
  assert.equal(signal.aborted, true)
  const noticesBefore = f.notices.length
  finish(true)
  await flush()
  assert.equal(f.notices.length, noticesBefore)
  // Cancellation does not create another retry budget.
  f.handler(eventFor(f.value))
  assert.equal(f.calls.filter((call) => call === 'recover').length, 1)
})

test('successful page lifecycle resets the bounded recovery budget', async () => {
  const f = recoveryFixture(() => Promise.resolve(true))
  f.handler(eventFor(f.value))
  await flush()
  f.handler.reset()
  f.handler(eventFor(f.value))
  await flush()
  assert.equal(f.calls.filter((call) => call === 'recover').length, 2)
})

test('a host must explicitly opt in; unknown recovery versions use legacy handling', () => {
  const f = recoveryFixture(() => Promise.resolve(true))
  f.value.headers['x-goadmin-auth-recovery'] = '2'
  f.handler(eventFor(f.value))
  assert.deepEqual(f.calls, ['clear', 'disconnect', 'navigate'])
})
