// Focused rerun against an owned, already initialized starter smoke stack.
import assert from 'node:assert/strict'
import { readFile, writeFile } from 'node:fs/promises'
import path from 'node:path'
import { chromium } from 'playwright'
import { expect as baseExpect } from 'playwright/test'
import { appearanceAcceptance } from './appearance.mjs'
const expect = baseExpect.configure({ timeout: 20000 })
const folder = path.resolve(process.argv[2] ?? '')
const manifest = JSON.parse(await readFile(path.join(folder, 'run.json'), 'utf8'))
assert.equal(manifest.owner, 'goadmin-starter-smoke-v1')
const selected = process.argv[3]
if (selected) assert.ok(manifest.cases[selected], 'Unknown smoke case')
const report = { runId: manifest.runId, profile: 'appearance-only', cases: {} }
const browser = await chromium.launch({ headless: true,
  ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH } : {}) })
async function login(page, email, password) {
  await page.goto('/auth/login')
  await page.locator('input[name="email"]').fill(email)
  await page.locator('input[name="password"]').fill(password)
  const [response] = await Promise.all([
    page.waitForResponse(r => new URL(r.url()).pathname === '/auth/login' && r.request().method() === 'POST'),
    page.locator('form button[type="submit"]').click(),
  ])
  assert.ok(response.status() < 400)
  await expect(page.locator('[data-profile-menu-trigger]')).toBeVisible()
}
async function logout(page) {
  await page.locator('[data-profile-menu-trigger]').click()
  await page.getByRole('menuitem', { name: /^(Выйти|Log out|Logout|Sign out)$/i }).click()
  await expect(page).toHaveURL(/\/auth\/login/)
}
try {
  for (const [name, config] of Object.entries(manifest.cases).filter(([name]) => !selected || selected === name)) {
    const credentials = JSON.parse(await readFile(path.join(folder, `${name}.credentials.json`), 'utf8'))
    const context = await browser.newContext({ baseURL: config.url, viewport: { width: 1440, height: 1000 }, reducedMotion: 'reduce' })
    const result = report.cases[name] = { result: 'running', pageErrors: [], consoleErrors: [] }
    const page = await context.newPage()
    page.on('pageerror', error => result.pageErrors.push(error.message))
    page.on('console', message => { if (message.type() === 'error') result.consoleErrors.push(message.text()) })
    try {
      await login(page, credentials.email, credentials.password)
      await appearanceAcceptance({ page, context, browser, config, credentials, folder, name, expect, login, logout, result })
      assert.deepEqual(result.pageErrors, []); assert.deepEqual(result.consoleErrors, [])
      result.result = 'passed'
      console.log(`PASS ${name}: full appearance acceptance`)
    } catch (error) {
      result.result = 'failed'; result.error = error.stack
      await page.screenshot({ path: path.join(folder, `${name}-appearance-failure.png`), fullPage: true }).catch(() => {})
      throw error
    } finally { await context.close() }
  }
} finally {
  await browser.close()
  await writeFile(path.join(folder, `appearance${selected ? `-${selected}` : ''}-report.json`), JSON.stringify(report, null, 2) + '\n')
}
