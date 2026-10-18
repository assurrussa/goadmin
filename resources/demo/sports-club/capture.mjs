import assert from 'node:assert/strict'
import { mkdir, writeFile } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
// Optional overrides let maintainers reuse an installed Playwright/browser.
const { chromium } = await import(
  process.env.PLAYWRIGHT_MODULE ? pathToFileURL(process.env.PLAYWRIGHT_MODULE).href : 'playwright'
)
const origin = 'http://127.0.0.1:8767'
const out = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../../../docs/screenshots')
await mkdir(out, { recursive: true })
const shots = [
  ['dashboard-compact-light', 'dashboard', 'compact', 'light', 1440, 1100],
  ['dashboard-default-light', 'dashboard', 'default', 'light', 1440, 1100],
  ['dashboard-branded-dark', 'dashboard', 'branded', 'dark', 1440, 1100],
  ['participants-compact-light', 'participants', 'compact', 'light', 1440, 1000],
  ['training-branded-light', 'training', 'branded', 'light', 1440, 1000],
  ['permissions-compact-dark', 'permissions', 'compact', 'dark', 1440, 1000],
  ['queue-compact-light', 'queue', 'compact', 'light', 1440, 1000],
  ['settings-default-light', 'settings', 'default', 'light', 1440, 1000],
  ['dashboard-compact-mobile', 'dashboard', 'compact', 'light', 390, 844],
  ['participants-compact-mobile', 'participants', 'compact', 'light', 390, 844],
]
const browser = await chromium.launch({
  headless: true,
  ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH
    ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH }
    : {}),
})
const errors = [],
  network = []
try {
  for (const [name, view, preset, mode, width, height] of shots) {
    const context = await browser.newContext({
      viewport: { width, height },
      deviceScaleFactor: 1,
      locale: 'en-GB',
      timezoneId: 'UTC',
      colorScheme: mode,
      reducedMotion: 'reduce',
    })
    // No external network is needed or permitted by this synthetic presentation.
    await context.route('**/*', (route) => {
      const u = new URL(route.request().url())
      if (u.origin !== origin) {
        network.push(u.origin)
        return route.abort()
      }
      return route.continue()
    })
    const page = await context.newPage()
    page.setDefaultTimeout(10000)
    page.on('console', (m) => {
      if (m.type() === 'error') errors.push(`${name}: ${m.text()}`)
    })
    page.on('pageerror', (e) => errors.push(`${name}: ${e.message}`))
    await page.goto(`${origin}/demo/${view}?preset=${preset}&mode=${mode}`, {
      waitUntil: 'networkidle',
    })
    await page.locator('[data-demo-ready]').waitFor()
    await page.evaluate(async () => {
      await document.fonts.ready
      await Promise.all([...document.images].map((i) => i.decode()))
      await new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r)))
    })
    assert.equal(
      await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
      true,
      `${name}: document overflow`,
    )
    assert.equal(await page.locator('h1').count(), 1)
    assert.equal(
      await page.locator('[aria-current="page"]').count(),
      1,
      `${name}: active navigation`,
    )
    await page.screenshot({
      path: path.join(out, `${name}.png`),
      fullPage: true,
      animations: 'disabled',
    })
    // Exercise representative behavior after taking the clean initial-state image.
    if (name === 'participants-compact-light') {
      await page.getByRole('button', { name: 'Next', exact: true }).click()
      assert.match(await page.locator('.pagination').innerText(), /Page 2 of 4/)
      await page.getByPlaceholder('Search by name…').fill('Jamie')
      assert.equal(await page.locator('tbody tr').count(), 3)
      await page.getByPlaceholder('Search by name…').fill('Alex')
      await page.getByLabel('Status', { exact: true }).selectOption('Pending')
      assert.equal(await page.locator('tbody tr').count(), 1)
      await page.getByRole('button', { name: /View Alex/ }).click()
      assert.match(await page.getByRole('status').innerText(), /local preview only/)
      await page.getByRole('link', { name: 'Overview', exact: true }).click()
      await page.getByRole('heading', { name: 'Club overview', exact: true }).waitFor()
      assert.equal(await page.locator('[aria-current="page"]').innerText(), 'Overview')
    }
    if (name === 'training-branded-light') {
      await page.getByLabel('Session title', { exact: true }).fill('Movement fundamentals')
      assert.equal(await page.locator('.preview-title').innerText(), 'Movement fundamentals')
      await page.getByRole('button', { name: 'Save local draft', exact: true }).click()
      assert.match(await page.getByRole('status').innerText(), /Training draft saved/)
    }
    if (name === 'permissions-compact-dark') {
      await page.getByRole('button', { name: 'Administrator', exact: true }).click()
      assert.equal(await page.locator('input:checked').count(), 6)
      await page.getByLabel('Manage roles', { exact: false }).uncheck()
      assert.equal(await page.locator('input:checked').count(), 5)
      await page.getByRole('button', { name: 'Coach', exact: true }).click()
      assert.equal(await page.locator('input:checked').count(), 3)
    }
    if (name === 'dashboard-compact-mobile') {
      const menu = page.getByRole('button', { name: 'Open menu', exact: true })
      await menu.click()
      const sidebar = page.locator('#admin-sidebar[role=dialog]')
      await sidebar.waitFor()
      assert.equal(await sidebar.evaluate((el) => el.contains(document.activeElement)), true)
      const focusable = sidebar
        .locator('a[href],button,select,[tabindex="0"]')
        .filter({ visible: true })
      await focusable.last().focus()
      await page.keyboard.press('Tab')
      assert.equal(await focusable.first().evaluate((el) => el === document.activeElement), true)
      await page.keyboard.press('Escape')
      assert.equal(await menu.evaluate((el) => el === document.activeElement), true)
      assert.equal(await page.locator('#admin-sidebar').getAttribute('inert'), '')
    }
    if (name === 'settings-default-light') {
      await page.getByRole('button', { name: 'Switch to dark theme', exact: true }).click()
      assert.equal(await page.locator('html').getAttribute('data-theme-mode'), 'dark')
      await page.locator('[data-profile-menu-trigger]').click()
      await page.getByRole('menuitem', { name: 'Profile', exact: true }).waitFor()
      await page.keyboard.press('Escape')
      assert.equal(
        await page
          .locator('[data-profile-menu-trigger]')
          .evaluate((el) => el === document.activeElement),
        true,
      )
    }
    await context.close()
    console.log(`PASS ${name}: capture, console, overflow, navigation`)
  }
  assert.deepEqual(errors, [], 'Browser console/page errors')
  assert.deepEqual(network, [], 'Unexpected external requests')
  await writeFile(
    path.join(out, 'capture-manifest.json'),
    JSON.stringify(
      {
        seedDate: '2026-10-19',
        browser: browser.version(),
        playwright: chromium.name(),
        deviceScaleFactor: 1,
        locale: 'en-GB',
        timezone: 'UTC',
        screenshots: shots.map(([file, view, preset, mode, width, height]) => ({
          file: `${file}.png`,
          view,
          preset,
          mode,
          viewport: { width, height },
        })),
      },
      null,
      2,
    ) + '\n',
  )
  console.log('PASS all screenshots and representative interactions; no external requests')
} finally {
  await browser.close()
}
