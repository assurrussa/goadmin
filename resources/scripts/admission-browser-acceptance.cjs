'use strict'

// Run only through TestAdmissionRealBrowser: configuration and SQL snapshots use
// private pipes. No credentials, fixture references or database URL are logged.
const assert = require('node:assert/strict')
const readline = require('node:readline')
const { chromium } = require(process.env.GOADMIN_PLAYWRIGHT_MODULE)
const input = readline.createInterface({ input: process.stdin })
const lines = input[Symbol.asyncIterator]()
async function receive() {
  const line = await lines.next()
  assert.equal(line.done, false, 'fixture pipe closed')
  return JSON.parse(line.value)
}
async function observations() {
  process.stdout.write(JSON.stringify({ op: 'observations' }) + '\n')
  return receive()
}
async function snapshot() {
  process.stdout.write(JSON.stringify({ op: 'snapshot' }) + '\n')
  return receive()
}
function record(s, subject = 'alice') { return s.records.find(r => r.subject === subject) }
async function main() {
  const cfg = await receive()
  for (const key of ['GOADMIN_BROWSER_PROFILE', 'GOADMIN_BROWSER_EXECUTABLE']) assert.ok(process.env[key], key)
  const persistent = await chromium.launchPersistentContext(process.env.GOADMIN_BROWSER_PROFILE, {
    executablePath: process.env.GOADMIN_BROWSER_EXECUTABLE,
    headless: process.env.GOADMIN_BROWSER_HEADLESS !== '0',
    chromiumSandbox: true,
    ignoreHTTPSErrors: false,
  })
  const browser = persistent.browser()
  assert.ok(browser, 'browser instance required')
  const contexts = []
  const uiErrors = []
  async function page() {
    const context = await browser.newContext({ ignoreHTTPSErrors: false })
    contexts.push(context)
    const p = await context.newPage()
    p.setDefaultTimeout(15000)
    p.on('pageerror', err => { uiErrors.push(err.message) })
    return p
  }
  async function visit(p, path) {
    const response = await p.goto(cfg.url + path)
    assert.ok(response, 'navigation response required')
    return response
  }
  async function login(p, email) {
    await visit(p, '/auth/login')
    await p.locator('input[name=email]').fill(email)
    await p.locator('input[name=password]').fill(cfg.password)
    await Promise.all([
      p.waitForURL(u => u.pathname !== '/auth/login'),
      p.locator('button[type=submit]').click(),
    ])
    await visit(p, '/auth/profile')
    await p.locator('#app').waitFor()
    assert.equal(new URL(p.url()).pathname, '/auth/profile', 'native operator login failed')
  }
  async function csrf(p) {
    const cookies = await p.context().cookies(cfg.url)
    const token = cookies.find(c => c.name === 'csrf_token')
    assert.ok(token, 'native CSRF token required')
    return decodeURIComponent(token.value)
  }
  async function post(p, path, body, token = true) {
    const value = token ? await csrf(p) : ''
    return p.evaluate(async ({ path, body, value }) => {
      const response = await fetch(path, {
        method: 'POST', credentials: 'same-origin',
        headers: { 'Content-Type': 'application/json', 'X-Inertia': 'true', ...(value ? { 'X-CSRF-Token': value } : {}) },
        body: JSON.stringify(body),
      })
      await response.text()
      return response.status
    }, { path, body, value })
  }
  function approveBody(version) {
    return { subject: 'alice', version, roleIds: [cfg.role], expiresAt: new Date(Date.now() + 3600000).toISOString() }
  }
  try {
    const pending = await page()
    await visit(pending, '/admission-browser/start')
    assert.equal(new URL(pending.url()).pathname, '/auth/admission')
    await pending.locator('#app').waitFor()
    await pending.getByRole('link').filter({ hasText: /SSO|вход|Sign in/i }).first().waitFor()
    let state = await snapshot()
    assert.equal(record(state).status, 'pending')
    assert.equal(state.memberships, 3)
    assert.equal(state.credentials, 3)
    assert.equal(state.sessions, 0)
    for (const path of ['/', '/admins', '/roles', '/permissions', '/files', '/queues', '/notifications', '/ws', '/admissions']) {
      const response = await visit(pending, path)
      assert.ok(response.status() !== 200 || new URL(pending.url()).pathname !== path, 'pending browser accessed protected route ' + path)
    }
    assert.deepEqual(await snapshot(), state, 'protected visits must not persist authority')
    // Neither arbitrary callback state nor a claimed identity creates authority.
    await visit(pending, '/admission-browser/callback?state=untrusted&subject=owner')
    assert.deepEqual(await snapshot(), state)

    const observer = await page()
    await login(observer, 'observer@example.invalid')
    await visit(observer, '/admissions')
    await observer.locator('article').filter({ hasText: 'alice' }).waitFor()
    let before = await snapshot()
    await post(observer, '/admissions/approve', approveBody(record(before).version))
    assert.deepEqual(await snapshot(), before, 'permission-denied approval persisted a mutation')

    const manager = await page()
    await login(manager, 'manager@example.invalid')
    await visit(manager, '/admissions')
    await manager.locator('article').filter({ hasText: 'alice' }).waitFor()
    before = await snapshot()
    await post(manager, '/admissions/approve', approveBody(record(before).version), false)
    assert.deepEqual(await snapshot(), before, 'missing-CSRF approval persisted a mutation')

    // A second localhost TLS port is genuinely cross-origin but same-site, so
    // native SameSite cookies remain attached. Only the browser's preflight is
    // fulfilled; the actual POST goes unchanged to the real native host.
    const foreign = await manager.context().newPage()
    await foreign.goto(cfg.foreignURL)
    const foreignOrigin = new URL(cfg.foreignURL).origin
    const observationCount = (await observations()).length
    await foreign.route('**/admissions/approve', async route => {
      if (route.request().method() === 'OPTIONS') {
        await route.fulfill({ status: 204, headers: {
          'Access-Control-Allow-Origin': foreignOrigin,
          'Access-Control-Allow-Credentials': 'true',
          'Access-Control-Allow-Methods': 'POST',
          'Access-Control-Allow-Headers': 'content-type,x-csrf-token,x-inertia',
        } })
      } else {
        await route.continue()
      }
    })
    const foreignResult = await foreign.evaluate(async ({ url, body, token }) => {
      try {
        const response = await fetch(url + '/admissions/approve', {
          method: 'POST', credentials: 'include',
          headers: { 'Content-Type': 'application/json', 'X-Inertia': 'true', 'X-CSRF-Token': token },
          body: JSON.stringify(body),
        })
        await response.text()
        return { status: response.status }
      } catch {
        // CORS can hide a denial response. The server's private receipt below
        // must still prove the POST reached it with the intended Origin.
        return { corsBlocked: true }
      }
    }, { url: cfg.url, body: approveBody(record(before).version), token: await csrf(manager) })
    const receipts = (await observations()).slice(observationCount)
    assert.equal(receipts.length, 1, 'foreign-origin POST must reach the native host')
    assert.equal(receipts[0].origin, foreignOrigin, 'server must observe the real foreign Origin')
    assert.equal(receipts[0].hasCSRF, true, 'foreign request must carry valid token')
    assert.equal(receipts[0].hasSession, true, 'foreign request must carry native session cookie')
    assert.ok(foreignResult.corsBlocked || foreignResult.status >= 400, 'foreign request must be denied')
    assert.deepEqual(await snapshot(), before, 'received foreign-origin approval persisted a mutation')
    await foreign.close()

    const owner = await page()
    await login(owner, 'owner@example.invalid')
    assert.equal(await post(owner, '/auth/role-preview', { roleId: cfg.previewRole }), 200, 'native role preview must activate')
    before = await snapshot()
    assert.equal(await post(owner, '/admissions/approve', approveBody(record(before).version)), 403, 'active native role preview must deny commands')
    assert.deepEqual(await snapshot(), before, 'role-preview mutation persisted a change')
    assert.equal(await post(owner, '/auth/role-preview', { roleId: 0 }), 200, 'native role preview must clear')

    // Positive control goes through the rendered embedded Vue approval form.
    await visit(manager, '/admissions')
    await manager.locator('input[type=datetime-local]').fill(new Date(Date.now() + 3600000).toISOString().slice(0, 16))
    const alice = manager.locator('article').filter({ hasText: 'alice' })
    await alice.locator('select').selectOption(String(cfg.role))
    await Promise.all([
      manager.waitForResponse(r => r.url().endsWith('/admissions/approve') && r.request().method() === 'POST'),
      alice.getByRole('button', { name: /Approve access|Одобрить доступ/ }).click(),
    ])
    await alice.getByText(/Approved|Одобрено/).waitFor()
    state = await snapshot()
    assert.equal(record(state).status, 'approved')
    assert.equal(state.memberships, 3, 'approval alone must not activate membership')
    assert.equal(state.credentials, 3)
    const approvedSessions = state.sessions

    // Fresh callback is required, and uses the same pending browser's opaque
    // server-generated cookie reference. Native session must also be issued.
    await visit(pending, '/admission-browser/start')
    assert.equal(new URL(pending.url()).pathname, '/auth/profile')
    await pending.locator('#app').waitFor()
    state = await snapshot()
    assert.equal(record(state).status, 'active')
    assert.equal(state.memberships, 4)
    assert.equal(state.roles, before.roles + 1)
    assert.equal(state.credentials, 3, 'SSO activation must not create a local password')
    assert.equal(state.sessions, approvedSessions + 1)
    await visit(pending, '/admissions')
    await pending.locator('article').filter({ hasText: 'alice' }).waitFor()

    // Native invitation form creates an exact-identity invitation.
    await visit(manager, '/admissions')
    await manager.locator('input[name=subject]').fill('invited')
    await manager.locator('select').first().selectOption(String(cfg.role))
    await manager.locator('input[type=datetime-local]').fill(new Date(Date.now() + 3600000).toISOString().slice(0, 16))
    await Promise.all([
      manager.waitForResponse(r => r.url().endsWith('/admissions/invitations') && r.request().method() === 'POST'),
      manager.getByRole('button', { name: /Create invitation|Создать приглашение/ }).click(),
    ])
    await manager.locator('article').filter({ hasText: 'invited' }).waitFor()
    before = await snapshot()
    const invitation = before.invitations.find(i => i.subject === 'invited')
    assert.ok(invitation)
    const wrong = await page()
    await visit(wrong, '/admission-browser/start?scenario=wrong&invitation=' + invitation.id)
    assert.deepEqual(await snapshot(), before, 'wrong identity consumed invitation or created authority')
    const noGrant = await page()
    await visit(noGrant, '/admission-browser/start?scenario=no-project&invitation=' + invitation.id)
    assert.deepEqual(await snapshot(), before, 'missing project grant created authority')
    assert.deepEqual(uiErrors, [], 'embedded UI runtime errors')
    process.stderr.write('Admission real-browser assertions passed; persisted effects verified.\n')
  } finally {
    await Promise.allSettled(contexts.map(context => context.close()))
    await persistent.close()
  }
  process.stdout.write(JSON.stringify({ op: 'done' }) + '\n')
}
main().catch(error => {
  process.stderr.write('Admission browser acceptance failed: ' + error.message + '\n')
  process.stdout.write(JSON.stringify({ op: 'failed' }) + '\n')
  process.exitCode = 1
}).finally(() => {
  input.close()
  process.stdin.destroy()
})
