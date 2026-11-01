// Run only against an owned, initialized local starter-smoke stack.
import assert from 'node:assert/strict'
import { readFile, writeFile } from 'node:fs/promises'
import path from 'node:path'
import { createServer } from 'node:http'
import { chromium } from 'playwright'
import { expect as baseExpect } from 'playwright/test'

const expect = baseExpect.configure({ timeout: 20000 })
const folder = path.resolve(process.argv[2] ?? '')
const manifest = JSON.parse(await readFile(path.join(folder, 'run.json'), 'utf8'))
assert.equal(manifest.owner, 'goadmin-starter-smoke-v1')
const selected = process.argv[3]
if (selected) assert.ok(manifest.cases[selected], 'Unknown smoke case')
const report = { runId: manifest.runId, profile: 'appearance-csrf', cases: {} }
const browser = await chromium.launch({ headless: true, chromiumSandbox: true,
  ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH } : {}) })
try {
  for (const [name, config] of Object.entries(manifest.cases).filter(([name]) => !selected || selected === name)) {
    assert.ok(['localhost', '127.0.0.1', '[::1]'].includes(new URL(config.url).hostname), 'Local fixture required')
    const credentials = JSON.parse(await readFile(path.join(folder, `${name}.credentials.json`), 'utf8'))
    const context = await browser.newContext({ baseURL: config.url })
    const result = report.cases[name] = { result: 'running', commands: [], observedMutations: [], pageErrors: [] }
    const page = await context.newPage()
    const origin = new URL(config.url).origin
    const cdp = await context.newCDPSession(page)
    const requestTrace = []
    const wireModes = new Map()
    const networkFailures = []
    await cdp.send('Network.enable')
    cdp.on('Network.requestWillBeSent', event => {
      requestTrace.push({ id: event.requestId, url: event.request.url, method: event.request.method })
    })
    cdp.on('Network.requestWillBeSentExtraInfo', event => {
      const mode = Object.entries(event.headers).find(([key]) => key.toLowerCase() === 'sec-fetch-mode')?.[1]
      if (mode) wireModes.set(event.requestId, mode)
    })
    cdp.on('Network.loadingFailed', event => {
      networkFailures.push({ id: event.requestId, blockedReason: event.blockedReason })
    })
    let targetServer
    page.on('pageerror', error => result.pageErrors.push(error.message))
    page.on('request', request => {
      if (!['PUT', 'PATCH'].includes(request.method())) return
      const url = new URL(request.url())
      const observation = { origin: url.origin, path: url.pathname, method: request.method() }
      // Redirect candidates can emit request events before Chrome blocks them.
      // Keep observations, but count original same-origin business commands separately.
      result.observedMutations.push(observation)
      if (url.origin === origin) result.commands.push({ path: url.pathname, method: request.method() })
    })
    try {
      await page.goto('/auth/login')
      await page.locator('input[name="email"]').fill(credentials.email)
      await page.locator('input[name="password"]').fill(credentials.password)
      await page.locator('form button[type="submit"]').click()
      await expect(page.locator('[data-profile-menu-trigger]')).toBeVisible()
      // Reproduce an external SSO host with no renewal capability, while using
      // a local test login. Do not replace the actual CSRF/auth middleware.
      await page.route('**/auth/profile/settings', async route => {
        const response = await route.fetch()
        const body = await response.text()
        const patched = body.replace(/(&#34;adminSessionMaintenance&#34;:)\{[^{}]*\}/, '$1{}')
        assert.notEqual(patched, body, 'Expected server-rendered maintenance capability')
        await route.fulfill({ response, body: patched })
      })
      await page.goto('/auth/profile/settings#branding')
      const brand = page.locator('[aria-labelledby="branding-title"]')
      const input = brand.locator('input').first()
      const apply = brand.getByRole('button', { name: /^(Применить|Apply)$/ })
      await expect(apply).toBeEnabled()
      const original = await input.inputValue()
      // Save unchanged branding: exercises a real protected PUT without
      // changing this owned fixture's configuration or logos.
      const success = page.waitForResponse(r => new URL(r.url()).pathname === '/auth/branding' && r.request().method() === 'PUT')
      // Two synchronous events must still send only one protected command.
      await apply.evaluate(button => { button.click(); button.click() })
      const saved = await success
      assert.equal(saved.status(), 200)
      assert.ok((await saved.request().allHeaders())['x-csrf-token'])
      assert.equal((await saved.request().allHeaders())['x-inertia'], undefined)
      let freshRequest
      await expect.poll(() => {
        freshRequest = requestTrace.find(request => request.method === 'PUT' && request.url === new URL('/auth/branding', config.url).href)
        return Boolean(freshRequest)
      }, { message: 'Observe the exact successful original PUT' }).toBe(true)
      // Playwright allHeaders can omit this header on macOS. ExtraInfo observes
      // the actual wire header for this request, not an unrelated earlier read.
      await expect.poll(() => wireModes.get(freshRequest.id)).toBe('same-origin')
      result.fetchMode = wireModes.get(freshRequest.id)
      assert.equal(result.commands.length, 1)

      await input.fill('Unsaved CSRF regression draft')
      const currentProof = (await context.cookies()).find(cookie => cookie.name === 'csrf_token')
      assert.ok(currentProof)
      await context.addCookies([{ ...currentProof, value: 'invalid-stale-proof' }])
      const stale = page.waitForResponse(r => new URL(r.url()).pathname === '/auth/branding' && r.request().method() === 'PUT')
      await apply.click()
      const staleResponse = await stale
      assert.equal(staleResponse.status(), 419)
      assert.match(staleResponse.headers()['content-type'], /application\/json/)
      assert.equal(staleResponse.headers().location, undefined)
      await expect(input).toHaveValue('Unsaved CSRF regression draft')
      assert.equal(result.commands.length, 2)

      await context.clearCookies({ name: 'csrf_token' })
      const rejected = page.waitForResponse(r => new URL(r.url()).pathname === '/auth/branding' && r.request().method() === 'PUT')
      await apply.click()
      const failure = await rejected
      assert.equal(failure.status(), 419)
      assert.match(failure.headers()['content-type'], /application\/json/)
      assert.equal(failure.headers().location, undefined)
      await expect(input).toHaveValue('Unsaved CSRF regression draft')
      await expect(page.locator('.notification').filter({ hasText: /Сеанс или защита|session or request protection/ }).last()).toBeVisible()
      // A real later read repairs the cookie but must not replay the command.
      await page.evaluate(() => fetch('/auth/profile/settings', { credentials: 'same-origin' }))
      await expect.poll(async () => (await context.cookies()).some(cookie => cookie.name === 'csrf_token')).toBe(true)
      assert.equal(result.commands.length, 3)
      await expect(input).toHaveValue('Unsaved CSRF regression draft')

      // Repair only makes a subsequent explicit click eligible; never replay.
      const retried = page.waitForResponse(r => new URL(r.url()).pathname === '/auth/branding' && r.request().method() === 'PUT')
      await apply.click()
      assert.equal((await retried).status(), 200)
      assert.equal(result.commands.length, 4)
      await expect(input).toHaveValue('Unsaved CSRF regression draft')
      await input.fill(original)
      const restored = page.waitForResponse(r => new URL(r.url()).pathname === '/auth/branding' && r.request().method() === 'PUT')
      await apply.click()
      assert.equal((await restored).status(), 200)
      assert.equal(result.commands.length, 5)

      // The browser must not forward the readable proof on an external
      // redirect. Use a real loopback target: an observable browser request
      // candidate is not evidence that a request or its token reached a server.
      const targetRequests = []
      targetServer = createServer((request, response) => {
        targetRequests.push({ csrfPresent: Boolean(request.headers['x-csrf-token']) })
        response.writeHead(200, { 'Content-Type': 'text/plain' })
        response.end('Unexpected redirect target')
      })
      await new Promise((resolve, reject) => {
        targetServer.once('error', reject)
        targetServer.listen(0, '127.0.0.1', resolve)
      })
      const address = targetServer.address()
      assert.ok(address && typeof address !== 'string')
      const target = new URL(`http://127.0.0.1:${address.port}/csrf-redirect-target`)
      assert.notEqual(target.origin, origin)
      const redirect = async route => {
        if (route.request().method() !== 'PUT') return route.continue()
        await route.fulfill({ status: 302, headers: { Location: target.href } })
      }
      await page.route('**/auth/branding', redirect)
      await input.fill('Unsaved redirect draft')
      const commandsBeforeRedirect = result.commands.length
      const traceBeforeRedirect = requestTrace.length
      await apply.click()
      await expect(brand.getByRole('alert')).toContainText(/Не удалось сохранить|Could not save|Failed to save/)
      const redirectTrace = () => requestTrace.slice(traceBeforeRedirect)
      await expect.poll(() => redirectTrace().some(request => request.url === target.href && networkFailures.some(failure => failure.id === request.id && failure.blockedReason === 'origin'))).toBe(true)
      assert.equal(redirectTrace().filter(request => request.method === 'PUT' && request.url === new URL('/auth/branding', config.url).href).length, 1)
      assert.equal(result.commands.length, commandsBeforeRedirect + 1)
      assert.equal(result.commands.length, 6)
      assert.equal(targetRequests.length, 0, 'No request may reach the actual redirect target server')
      assert.equal(targetRequests.filter(request => request.csrfPresent).length, 0, 'No CSRF proof may reach the target')
      result.redirectGuard = { originalCommands: 1, blockedReason: 'origin', targetServerHits: targetRequests.length, targetCSRFHits: targetRequests.filter(request => request.csrfPresent).length }
      await expect(input).toHaveValue('Unsaved redirect draft')
      await page.unroute('**/auth/branding', redirect)

      // Remove the login while keeping its proof: a rejected session must not
      // redirect, replay a PUT, navigate away, or discard the mounted draft.
      const csrf = (await context.cookies()).filter(cookie => cookie.name === 'csrf_token')
      await context.clearCookies()
      await context.addCookies(csrf)
      const expired = page.waitForResponse(r => new URL(r.url()).pathname === '/auth/branding' && r.request().method() === 'PUT')
      await apply.click()
      const expiredResponse = await expired
      assert.ok([401, 419].includes(expiredResponse.status()))
      assert.match(expiredResponse.headers()['content-type'], /application\/json/)
      assert.equal(expiredResponse.headers().location, undefined)
      await expect(input).toHaveValue('Unsaved redirect draft')
      await expect(page).toHaveURL(/\/auth\/profile\/settings#branding$/)
      assert.equal(result.commands.length, 7)
      assert.ok(result.commands.every(command => command.path === '/auth/branding'))
      assert.equal(targetRequests.length, 0, 'The redirect target must also remain untouched by late requests')
      assert.equal(targetRequests.filter(request => request.csrfPresent).length, 0)
      assert.deepEqual(result.pageErrors, [])
      result.result = 'passed'
      result.originalNameUnchanged = original
    } finally {
      try { await context.close() } finally {
        if (targetServer?.listening) await new Promise((resolve, reject) => targetServer.close(error => error ? reject(error) : resolve()))
      }
    }
  }
} finally {
  await browser.close()
  await writeFile(path.join(folder, 'appearance-csrf-report.json'), JSON.stringify(report, null, 2) + '\n')
}


