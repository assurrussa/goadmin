// Read-only palette diagnostic for an already running smoke host; uses a separate browser context.
import { readFile, writeFile } from 'node:fs/promises'
import path from 'node:path'
import { chromium } from 'playwright'
import { expect } from 'playwright/test'
import { waitForPaint, paintViolations } from './paint.mjs'
const folder = path.resolve(process.argv[2])
const manifest = JSON.parse(await readFile(path.join(folder, 'run.json'), 'utf8'))
if (manifest.owner !== 'goadmin-starter-smoke-v1') throw new Error('Not an owned smoke run')
const report = { runId: manifest.runId, diagnosticOnly: true, cases: {} }
const browser = await chromium.launch({ headless: true,
  ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH } : {}) })
let failed = false
try {
  for (const [name, config] of Object.entries(manifest.cases)) {
    const credentials = JSON.parse(await readFile(path.join(folder, `${name}.credentials.json`), 'utf8'))
    const context = await browser.newContext({ baseURL: config.url, colorScheme: 'light',
      viewport: { width: 1440, height: 1000 }, reducedMotion: 'reduce' })
    const page = await context.newPage()
    const result = report.cases[name] = { paint: {}, errors: {} }
    try {
      await page.goto('/auth/login')
      await page.locator('input[name="email"]').fill(credentials.email)
      await page.locator('input[name="password"]').fill(credentials.password)
      await Promise.all([
        page.waitForResponse((response) => new URL(response.url()).pathname === '/auth/login' && response.request().method() === 'POST'),
        page.locator('form button[type="submit"]').click(),
      ])
      await expect(page.locator('[data-profile-menu-trigger]')).toBeVisible()
      await page.goto('/auth/profile')
      for (const mode of ['light', 'dark']) {
        if (mode === 'dark') {
          await page.getByRole('button', { name: /Switch to dark theme|Переключить на темную тему/i }).click()
          await page.reload()
        }
        await expect(page.locator('html')).toHaveAttribute('data-theme-mode', mode)
        try {
          await waitForPaint(page, name, mode, result.paint.light,
            (measured) => { result.paint[mode] = measured }, 3000)
        } catch {
          failed = true
          result.errors[mode] = paintViolations(result.paint[mode], name, mode, result.paint.light)
        }
        console.log(`${result.errors[mode] ? 'FAIL' : 'PASS'} ${name}/${mode}: ${JSON.stringify(result.errors[mode] ?? [])}`)
      }
    } finally { await context.close() }
  }
} finally {
  await browser.close()
  await writeFile(path.join(folder, 'paint-regression-report.json'), JSON.stringify(report, null, 2) + '\n')
}
if (failed) process.exitCode = 1
