import assert from 'node:assert/strict'
import path from 'node:path'
import { appearanceGeometry, appearanceVisualAcceptance } from './appearance-visual.mjs'

export async function appearanceAcceptance({ page, context, browser, config, credentials, folder, name, expect, login, logout, result }) {
  await page.setViewportSize({ width: 1440, height: 1000 })
  const get = async url => {
    const response = await context.request.get(url)
    assert.equal(response.status(), 200, url)
    return response.json()
  }
  const send = async (method, url, data, csrf = true, origin = config.url) => {
    const token = (await context.cookies(config.url)).find(cookie => cookie.name === 'csrf_token')?.value
    return context.request.fetch(url, { method, data, maxRedirects: 0,
      headers: { Accept: 'application/json', Origin: origin, Referer: `${config.url}/auth/profile/settings`,
        ...(csrf ? { 'X-CSRF-Token': token } : {}) } })
  }
  // Establish a known baseline on owned reruns that previously stopped after Apply.
  const previousBrand = await get('/auth/branding')
  if (Object.values(previousBrand.payload).some(Boolean)) {
    assert.equal((await send('PUT', '/auth/branding', { version: previousBrand.version, reset: true })).status(), 200)
  }
  await page.goto('/auth/profile/settings#interface')
  await expect(page.getByRole('heading', { name: /^(Settings|Настройки)$/, exact: true })).toBeVisible()
  const set = async (label, value) => {
    const select = page.locator('section[aria-labelledby="appearance-title"]').getByLabel(label, { exact: true })
    const settled = () => expect(page.locator('section[aria-labelledby="appearance-title"]').getByRole('status')).toHaveText(/^(Saved|Сохранено)$/)
    if (await select.inputValue() === value) {
      await settled()
      return get('/auth/profile/appearance')
    }
    const [response] = await Promise.all([
      page.waitForResponse(r => new URL(r.url()).pathname === '/auth/profile/appearance' && r.request().method() === 'PATCH'),
      select.selectOption(value),
    ])
    assert.equal(response.status(), 200)
    const document = await response.json()
    await settled()
    return document
  }
  // Inertia Back restores cached page props. A newer successful header save must
  // retain its version and paint when returning to the older settings page.
  await set(/^(Theme|Тема)$/, 'light')
  await page.locator('[data-profile-menu-trigger]').click()
  await page.locator('[role="menu"] a[href="/auth/profile"]').click()
  await expect(page).toHaveURL(/\/auth\/profile$/)
  const [headerSave] = await Promise.all([
    page.waitForResponse(r => new URL(r.url()).pathname === '/auth/profile/appearance' && r.request().method() === 'PATCH'),
    page.getByRole('button', { name: /Switch to dark theme|Переключить на темную тему/i }).click(),
  ])
  assert.equal(headerSave.status(), 200)
  const headerVersion = (await headerSave.json()).version
  await page.goBack()
  await expect(page).toHaveURL(/\/auth\/profile\/settings/)
  await expect(page.locator('html')).toHaveAttribute('data-theme-mode', 'dark')
  await expect(page.locator('#appearance-theme')).toHaveValue('dark')
  const currentDensity = await page.locator('#appearance-density').inputValue()
  const densityChange = await set(/^(Density|Плотность)$/, currentDensity === 'comfortable' ? 'default' : 'comfortable')
  assert.equal(densityChange.version, headerVersion + 1, 'Back must preserve the newest CAS version')
  await set(/^(Density|Плотность)$/, 'compact')
  await set(/^(Language|Язык)$/, 'en')
  await page.goto('/users')
  const cells = page.locator('[data-ui="table"][data-part="cell"]')
  await expect(cells.first()).toBeVisible()
  const tableMeasurement = async density => {
    let measured
    await expect.poll(async () => {
      measured = await cells.first().evaluate(async n => {
        await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)))
        const before = n.getBoundingClientRect().height
        await new Promise(resolve => requestAnimationFrame(resolve))
        const height = n.getBoundingClientRect().height
        return { height, stable: n.isConnected && height > 0 && Math.abs(before - height) < .1,
          row: n.parentElement?.querySelector('[role="checkbox"]')?.getAttribute('aria-label'),
          padding: getComputedStyle(n).paddingTop, density: document.documentElement.dataset.density,
          viewport: innerWidth, locale: document.documentElement.lang }
      })
      return measured.stable && measured.density === density && measured.viewport === 1440 &&
        measured.locale === 'en' && measured.padding === (density === 'compact' ? '6px' : '20px')
    }, { message: `Wait for a stable visible ${density} data row` }).toBe(true)
    assert.ok(measured.row, 'Measure an actual record row with a selection identity')
    return measured
  }
  const compact = await tableMeasurement('compact')
  const compactRowHeight = compact.height
  await page.goto('/auth/profile/settings#interface')
  await set(/^(Language|Язык)$/, 'ru')
  const saved = await get('/auth/profile/appearance')
  assert.deepEqual(saved.payload, { schemaVersion: 1, theme: 'dark', locale: 'ru', density: 'compact' })
  await logout(page)
  await login(page, credentials.email, credentials.password)
  await expect(page.locator('html')).toHaveAttribute('lang', 'ru')
  await expect(page.locator('html')).toHaveAttribute('data-theme-mode', 'dark')
  await expect(page.locator('html')).toHaveAttribute('data-density', 'compact')

  // A distinct browser has no localStorage or cookies from this account.
  const other = await browser.newContext({ baseURL: config.url, viewport: { width: 1440, height: 1000 }, colorScheme: 'light' })
  try {
    const fresh = await other.newPage()
    await login(fresh, credentials.email, credentials.password)
    await expect(fresh.locator('html')).toHaveAttribute('data-theme-mode', 'dark')
    await expect(fresh.locator('html')).toHaveAttribute('lang', 'ru')
    await expect(fresh.locator('html')).toHaveAttribute('data-density', 'compact')
  } finally { await other.close() }

  await page.goto('/auth/profile/settings#branding')
  const brandTitle = 'M'.repeat(80)
  await page.getByLabel('Название админки', { exact: true }).fill(brandTitle)
  const baselineAccent = (await appearanceGeometry(page)).accent
  let customPreviews
  const customContrast = []
  for (const accent of ['#ffffff', '#000000', '#7c3aed']) {
    await page.getByLabel('Акцентный цвет', { exact: true }).fill(accent)
    await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))))
    const geometry = await appearanceGeometry(page)
    assert.deepEqual(geometry.accent, baselineAccent, 'Draft accent must not change the published root palette')
    assert.equal(geometry.panels.length, 2)
    assert.deepEqual(geometry.panels.map(panel => panel.dark).sort(), [false, true])
    for (const panel of geometry.panels) for (const part of ['link', 'button', 'field']) {
      assert.ok(panel[part].contrast >= 4.5, `${accent}/${panel.dark}/${part}: actual custom preview contrast ${panel[part].contrast}`)
    }
    customPreviews = geometry.panels
    customContrast.push({ accent, panels: geometry.panels })
  }
  const png = Buffer.from(await page.evaluate(() => {
    const canvas = document.createElement('canvas'); canvas.width = 64; canvas.height = 32
    const c = canvas.getContext('2d'); c.fillStyle = '#7c3aed'; c.fillRect(0, 0, 64, 32)
    return Array.from(Uint8Array.from(atob(canvas.toDataURL('image/png').split(',')[1]), c => c.charCodeAt(0)))
  }))
  for (const label of ['Логотип для светлой темы', 'Логотип для тёмной темы']) {
    await page.getByLabel(label, { exact: true }).setInputFiles({ name: 'brand.png', mimeType: 'image/png', buffer: png })
  }
  const apply = page.getByRole('button', { name: 'Применить', exact: true })
  await expect(apply).toBeEnabled()
  const [published] = await Promise.all([
    page.waitForResponse(r => new URL(r.url()).pathname === '/auth/branding' && r.request().method() === 'PUT'), apply.click(),
  ])
  assert.equal(published.status(), 200)
  const expectedAccent = dark => customPreviews.find(panel => panel.dark === dark).link.foreground.map(Math.round)
  await expect.poll(async () => (await appearanceGeometry(page)).accent).toEqual(expectedAccent(true))
  assert.notDeepEqual(expectedAccent(true), baselineAccent, 'Successful Apply must change the live palette')
  const branding = await get('/auth/branding')
  assert.equal(branding.payload.name, brandTitle)
  await page.locator('[data-profile-menu-trigger]').click()
  await page.locator('[role="menu"] a[href="/auth/profile"]').click()
  await expect(page).toHaveURL(/\/auth\/profile$/)
  await page.goBack()
  await expect(page).toHaveURL(/\/auth\/profile\/settings#branding$/)
  await expect(page.locator('[data-ui="admin-brand"]').filter({ visible: true }).first()).toContainText(brandTitle)
  await page.setViewportSize({ width: 390, height: 844 })
  await expect(page.getByLabel('Название админки', { exact: true })).toHaveValue(brandTitle)
  assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), '80 unbroken brand characters must reflow on mobile')
  await page.screenshot({ path: path.join(folder, `${name}-branding-long-mobile.png`), fullPage: true })
  await page.setViewportSize({ width: 1440, height: 1000 })
  const anonymous = await browser.newContext({ baseURL: config.url })
  try {
    for (const variant of ['light', 'dark']) {
      const asset = await anonymous.request.get(`/public/branding/${variant}`)
      assert.equal(asset.status(), 200)
      assert.match(asset.headers()['content-type'], /image\/png/)
      const etag = asset.headers().etag; assert.ok(etag)
      assert.equal((await anonymous.request.get(`/public/branding/${variant}`, { headers: { 'If-None-Match': etag } })).status(), 304)
    }
    assert.equal((await anonymous.request.get('/public/branding/other')).status(), 404)
    const loginPage = await anonymous.newPage()
    await loginPage.goto('/auth/login')
    await expect(loginPage.getByText(brandTitle, { exact: true }).filter({ visible: true }).first()).toBeVisible()
  } finally { await anonymous.close() }
  await page.screenshot({ path: path.join(folder, `${name}-appearance-brand-dark-ru.png`), fullPage: true })

  // Actual native middleware rejects both requests before persistence.
  for (const csrf of [false, true]) {
    for (const [method, url, data] of [
      ['PATCH', '/auth/profile/appearance', { version: saved.version, payload: saved.payload }],
      ['PUT', '/auth/branding', { version: branding.version, name: 'MUST NOT SAVE', accentColor: '' }],
    ]) {
      const denied = await send(method, url, data, csrf, csrf ? 'https://foreign.example.test' : config.url)
      assert.equal(denied.status(), 302)
    }
  }
  assert.equal((await get('/auth/branding')).payload.name, brandTitle)
  assert.deepEqual(await get('/auth/profile/appearance'), saved)
  const forged = await send('PATCH', '/auth/profile/appearance', { version: saved.version, payload: saved.payload, owner: '999999' })
  assert.equal(forged.status(), 400)
  assert.equal((await send('PATCH', '/auth/profile/appearance', { version: saved.version - 1, payload: saved.payload })).status(), 409)
  assert.equal((await send('PUT', '/auth/branding', { version: branding.version - 1, name: 'CONFLICT', accentColor: '' })).status(), 409)
  const roles = await get('/roles/data?limit=100')
  const superRole = roles.data.find(r => r.item.slug === 'super_admin')
  assert.equal((await send('POST', '/auth/role-preview', { roleId: Number(superRole.selectionId) })).status(), 200)
  try {
    assert.equal((await send('PATCH', '/auth/profile/appearance', { version: saved.version, payload: saved.payload })).status(), 403)
    assert.equal((await send('PUT', '/auth/branding', { version: branding.version, reset: true })).status(), 403)
  } finally { assert.equal((await send('POST', '/auth/role-preview', { roleId: 0 })).status(), 200) }
  await logout(page)
  await login(page, credentials.restrictedEmail, credentials.password)
  assert.equal((await context.request.get('/auth/branding')).status(), 403)
  const restricted = await get('/auth/profile/appearance')
  assert.notEqual(restricted.owner, saved.owner)
  assert.equal(restricted.version, 0)
  await logout(page)
  await login(page, credentials.email, credentials.password)

  await page.goto('/auth/profile/settings#interface')
  await set('Тема', 'light')
  await expect.poll(async () => (await appearanceGeometry(page)).accent).toEqual(expectedAccent(false))
  await set('Плотность', 'comfortable')
  await set('Язык', 'en')
  await page.setViewportSize({ width: 390, height: 844 })
  assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth))
  await page.screenshot({ path: path.join(folder, `${name}-appearance-comfortable-mobile-en.png`), fullPage: true })
  await page.setViewportSize({ width: 1440, height: 1000 })
  await page.goto('/users')
  await expect(cells.first()).toBeVisible()
  const comfortable = await tableMeasurement('comfortable')
  const comfortableRowHeight = comfortable.height
  assert.equal(comfortable.row, compact.row, 'Compare the same record in both densities')
  assert.ok(comfortableRowHeight > compactRowHeight, `comfortable table rows must be larger than compact rows: ${JSON.stringify({ compact, comfortable })}`)
  await page.goto('/auth/profile/settings#interface')
  await set(/^(Density|Плотность)$/, 'default')
  const brandLink = page.getByRole('link', { name: 'Admin branding', exact: true })
  await brandLink.focus(); await page.keyboard.press('Enter')
  const resetButton = page.getByRole('button', { name: 'Reset branding', exact: true })
  await expect(resetButton).toBeEnabled()
  await resetButton.focus(); await page.keyboard.press('Enter')
  const dialog = page.getByRole('dialog')
  await expect(dialog).toContainText('all users')
  assert.ok(await dialog.evaluate(n => n.contains(document.activeElement)))
  await page.keyboard.press('Escape')
  await expect(dialog).toHaveCount(0)
  assert.equal((await get('/auth/branding')).payload.name, brandTitle, 'cancel reset must retain published brand')
  await resetButton.focus(); await page.keyboard.press('Enter')
  const confirmation = dialog.getByRole('button', { name: 'Confirm reset', exact: true })
  await confirmation.focus()
  const [reset] = await Promise.all([
    page.waitForResponse(r => new URL(r.url()).pathname === '/auth/branding' && r.request().method() === 'PUT'),
    page.keyboard.press('Enter'),
  ])
  assert.equal(reset.status(), 200)
  assert.equal((await get('/auth/branding')).payload.name, '')
  await appearanceVisualAcceptance({ browser, context, page, config, folder, name, expect, result, get, send })
  result.appearance = { crossBrowser: true, relogin: true, autosave: true, ownerIsolation: true,
    history: { appearance: true, branding: true }, longBrandMobile: true,
    branding: { published: true, publicPNG: true, etag: true, reset: true, resetCancelAndKeyboard: true,
      customContrast, appliedLightAndDark: true },
    densityGeometry: { compactRowHeight, comfortableRowHeight },
    browserZoom200Percent: true, nativeCSRF: true, nativeRBAC: true, preview: true }
}
