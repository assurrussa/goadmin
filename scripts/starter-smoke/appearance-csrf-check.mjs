// Run only against an owned, initialized local starter-smoke stack.
import assert from 'node:assert/strict'
import { readFile, writeFile } from 'node:fs/promises'
import path from 'node:path'
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
    const result = report.cases[name] = { result: 'running', commands: [], pageErrors: [] }
    const page = await context.newPage()
    page.on('pageerror', error => result.pageErrors.push(error.message))
    page.on('request', request => {
      if (['PUT', 'PATCH'].includes(request.method())) result.commands.push({ path: new URL(request.url()).pathname, method: request.method() })
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
      assert.equal((await saved.request().allHeaders())['sec-fetch-mode'], 'same-origin')
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
      // redirect. Both origins are loopback; the target is intercepted locally.
      const target = new URL('/csrf-redirect-target', config.url)
      target.hostname = target.hostname === 'localhost' ? '127.0.0.1' : 'localhost'
      let targetReached = false
      await page.route(target.href, async route => {
        targetReached = true
        await route.fulfill({ status: 200, body: 'Unexpected redirect target' })
      })
      const redirect = async route => {
        if (route.request().method() !== 'PUT') return route.continue()
        await route.fulfill({ status: 302, headers: { Location: target.href } })
      }
      await page.route('**/auth/branding', redirect)
      await input.fill('Unsaved redirect draft')
      await apply.click()
      await expect(brand.getByRole('alert')).toContainText(/Не удалось сохранить|Could not save|Failed to save/)
      assert.equal(targetReached, false)
      assert.equal(result.commands.length, 6)
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
      assert.deepEqual(result.pageErrors, [])
      result.result = 'passed'
      result.originalNameUnchanged = original
    } finally { await context.close() }
  }
} finally {
  await browser.close()
  await writeFile(path.join(folder, 'appearance-csrf-report.json'), JSON.stringify(report, null, 2) + '\n')
}

