import assert from 'node:assert/strict'
import { readFile, writeFile } from 'node:fs/promises'
import path from 'node:path'
import { execFileSync } from 'node:child_process'
import { chromium } from 'playwright'
import { expect as baseExpect } from 'playwright/test'
import { terminalUpload, taskStatus, deferred, deadline } from './helpers.mjs'
import { waitForPaint } from './paint.mjs'
import { dashboardAcceptance } from './dashboard.mjs'
import { tableWorkflowAcceptance } from './table-workflow.mjs'

const expect = baseExpect.configure({ timeout: 20000 })

const folder = path.resolve(process.argv[2] ?? '')
const manifest = JSON.parse(await readFile(path.join(folder, 'run.json'), 'utf8'))
assert.equal(manifest.owner, 'goadmin-starter-smoke-v1')
const report = { profile: process.argv.includes('--records-only') ? 'record-editing' : 'full', runId: manifest.runId, revision: manifest.revision, dirty: manifest.dirty,
  playwright: JSON.parse(await readFile(new URL('./package.json', import.meta.url))).devDependencies.playwright,
  startedAt: new Date().toISOString(), cases: {} }
const browser = await chromium.launch({ headless: true,
  ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH } : {}) })
report.browser = browser.version()

async function fill(page, values) {
  for (const [name, value] of Object.entries(values)) await page.locator(`input[name="${name}"]`).fill(value)
}
function ownedSQL(config, statement) {
  execFileSync('docker', ['compose', '--project-directory', folder, '--env-file', config.env,
    '-p', config.project, '-f', config.compose, 'exec', '-T', 'postgres',
    'psql', '-U', 'smoke', '-d', 'smoke', '-v', 'ON_ERROR_STOP=1', '-c', statement], { stdio: 'pipe' })
}
async function submit(page, pathname) {
  const [result] = await Promise.all([
    page.waitForResponse((r) => new URL(r.url()).pathname === pathname && r.request().method() === 'POST'),
    page.locator('form button[type="submit"]').click(),
  ])
  assert.ok(result.status() < 400, `${pathname}: ${result.status()}`)
  await expect(page.locator('form[aria-busy="true"]')).toHaveCount(0)
  return result
}
async function navigationLink(page, href) {
  const target = page.locator(`a[href="${href}"]`).filter({ visible: true })
  if (await target.count()) return target.first()
  // Topbar groups materialize their real links only while the dropdown is open.
  for (const trigger of await page.locator('[data-part="horizontal"] button').all()) {
    await trigger.click()
    await expect(page.getByRole('menu').first()).toBeVisible()
    if (await target.count()) return target.first()
    for (const submenu of await page.locator('[role="menuitem"][aria-haspopup="menu"]').all()) {
      await submenu.focus()
      await page.keyboard.press('ArrowRight')
      await expect(page.getByRole('menu')).toHaveCount(2)
      if (await target.count()) return target.first()
      await page.keyboard.press('Escape')
    }
    await page.keyboard.press('Escape')
  }
  return null
}
async function language(page, name, code) {
  await page.getByRole('button', { name: /^(Язык|Language)$/ }).click()
  await page.getByRole('menuitemradio', { name, exact: true }).click()
  await expect(page.locator('html')).toHaveAttribute('lang', code)
}
async function login(page, email, password) {
  await page.goto('/auth/login')
  await fill(page, { email, password })
  await submit(page, '/auth/login')
  await expect(page.locator('[data-profile-menu-trigger]')).toBeVisible()
}
async function logout(page) {
  await page.locator('[data-profile-menu-trigger]').click()
  await page.getByRole('menuitem', { name: /^(Выйти|Log out|Logout|Sign out)$/i }).click()
  await expect(page).toHaveURL(/\/auth\/login/)
  await page.goto('/auth/profile')
  await expect(page).toHaveURL(/\/auth\/login/)
}

async function runCase(name, config) {
  const credentials = JSON.parse(await readFile(path.join(folder, `${name}.credentials.json`), 'utf8'))
  const result = report.cases[name] = { url: config.url, checks: [], consoleErrors: [], pageErrors: [],
    websocketFrames: [], polling: [], requests: [], result: 'running' }
  const context = await browser.newContext({ baseURL: config.url, viewport: { width: 1440, height: 1000 },
    locale: 'en-GB', timezoneId: 'UTC', colorScheme: 'light', reducedMotion: 'reduce' })
  await context.tracing.start({ screenshots: true, snapshots: true, sources: true })
  const page = await context.newPage()
  page.setDefaultTimeout(20000)
  page.setDefaultNavigationTimeout(45000)
  page.on('pageerror', (error) => result.pageErrors.push(error.message))
  page.on('console', (message) => { if (message.type() === 'error') result.consoleErrors.push(message.text()) })
  page.on('response', (response) => {
    const url = new URL(response.url())
    if (url.origin === config.url) result.requests.push({ method: response.request().method(),
      path: url.pathname, status: response.status(), inertia: response.request().headers()['x-inertia'] === 'true' })
  })
  let releaseNavigation = deferred()
  let terminalReceived = deferred()
  let buffering = false
  const pendingFrames = []
  await page.routeWebSocket('**/ws', (ws) => {
    const server = ws.connectToServer()
    server.onMessage((message) => {
      const terminal = terminalUpload(message)
      let afterProcess = false
      try { afterProcess = JSON.parse(message.toString()).eventType === 'file.after.process' } catch {}
      const held = buffering && (!!terminal || afterProcess)
      result.websocketFrames.push({ source: 'real-server', receivedAt: new Date().toISOString(),
        buffered: held, message: message.toString() })
      if (held) {
        pendingFrames.push(() => ws.send(message))
        if (terminal) terminalReceived.resolve(terminal)
      } else ws.send(message)
    })
  })
  await page.route(/\/files\/(?:upload\/)?tasks\/\d+(?:\?.*)?$/, async (route) => {
    const response = await route.fetch()
    const payload = await response.json()
    const status = taskStatus(payload)
    result.polling.push({ path: new URL(route.request().url()).pathname, status, source: 'real-server' })
    if (buffering && ['completed', 'failed', 'error'].includes(status)) {
      await deadline(releaseNavigation.promise, 'Inertia destination before terminal polling release', 45000)
    }
    await route.fulfill({ response })
  })
  const check = (label) => { result.checks.push(label); console.log(`PASS ${name}: ${label}`) }
  try {
    await page.goto('/auth/profile')
    await expect(page).toHaveURL(/\/auth\/login/)
    const pack = name === 'topbar' ? 'example.topbar' : 'default'
    await expect(page.locator('html')).toHaveAttribute('data-admin-theme', pack)
    const authShell = page.locator('[data-ui="auth-shell"]')
    await expect(authShell).toHaveCount(1)
    assert.equal(await authShell.getAttribute('data-part'), name === 'topbar' ? 'split' : null)
    await page.goto('/auth/register')
    await fill(page, { name: 'Smoke Owner', lastName: 'Initial', email: credentials.email,
      username: `smoke-${name}`, password: credentials.password, passwordConfirm: credentials.password,
      setupToken: credentials.setupToken })
    await submit(page, '/auth/register')
    // Registration may create a session or return the first administrator to login.
    if (new URL(page.url()).pathname.startsWith('/auth/login')) await login(page, credentials.email, credentials.password)
    await expect(page.locator('[data-profile-menu-trigger]')).toBeVisible()
    await expect(page.locator('[data-ui="admin-shell"]')).toHaveAttribute('data-part', name === 'topbar' ? 'topbar' : 'sidebar')
    result.presentation = { pack, adminShell: name === 'topbar' ? 'topbar' : 'sidebar',
      authShell: name === 'topbar' ? 'split' : 'standard' }
    check('selected presentation ID and actual auth/admin shells match the build')
    check('first administrator setup and authenticated session; anonymous profile denied')
    await language(page, 'English', 'en')
    await page.goto('/auth/profile/edit')
    await fill(page, { name: '   ' })
    const invalid = await submit(page, '/auth/profile/edit')
    assert.ok(invalid.request().headers()['x-inertia'], 'validation must reach the server through Inertia')
    await expect(page.locator('input[name="name"]')).toHaveAttribute('aria-invalid', 'true')
    const nameErrorId = await page.locator('input[name="name"]').getAttribute('aria-describedby')
    assert.ok(nameErrorId, 'invalid field must reference its error message')
    await expect(page.locator(`[id="${nameErrorId}"]`)).toBeVisible()
    await expect(page.locator(`[id="${nameErrorId}"]`)).not.toBeEmpty()
    await expect(page.locator('input[name="name"]')).toBeFocused()
    const savedName = `Saved ${name}`
    await fill(page, { name: savedName, lastName: 'Acceptance', username: `saved-${name}` })
    await submit(page, '/auth/profile/edit')
    await expect(page).toHaveURL(/\/auth\/profile\/edit$/)
    await expect(page.locator('.notification--success')).toHaveCount(1)
    await page.reload()
    await expect(page.locator('.notification--success')).toHaveCount(0)
    await expect(page.locator('input[name="name"]')).toHaveValue(savedName)
    await expect(page.locator('input[name="lastName"]')).toHaveValue('Acceptance')
    await expect(page.locator('input[name="username"]')).toHaveValue(`saved-${name}`)
    await page.goto('/auth/profile')
    await expect(page.locator('h1')).toContainText(savedName)
    check('profile server validation, focus, save, one flash and reload persistence')

    await page.goto('/auth/profile')
    await page.evaluate(() => { window.__starterSmokeDocument = 'same-inertia-document' })
    // Real image bytes generated by the browser, then decoded/cropped by the actual cropper.
    const png = await page.evaluate(() => {
      const canvas = document.createElement('canvas'); canvas.width = 640; canvas.height = 480
      const drawing = canvas.getContext('2d'); drawing.fillStyle = '#1e40af'; drawing.fillRect(0, 0, 640, 480)
      drawing.fillStyle = '#fbbf24'; drawing.fillRect(180, 100, 280, 280)
      return canvas.toDataURL('image/png').split(',')[1]
    })
    buffering = true
    await page.locator('input[name="image-upload"]').setInputFiles({ name: `avatar-${name}.png`,
      mimeType: 'image/png', buffer: Buffer.from(png, 'base64') })
    const cropper = page.getByRole('dialog')
    await expect(cropper).toBeVisible()
    const [acceptance] = await Promise.all([
      page.waitForResponse((response) =>
        /^\/files\/tus\/[^/]+\/complete$/.test(new URL(response.url()).pathname) && response.request().method() === 'POST'),
      cropper.getByRole('button', { name: /^(Обрезать|Crop)$/ }).click(),
    ])
    assert.ok(acceptance.ok(), 'real TUS completion must be accepted')
    const acceptancePayload = await acceptance.json()
    result.uploadAcceptance = { status: acceptance.status(), payload: acceptancePayload }
    const acceptedTaskId = acceptancePayload.tasks?.[0]?.id ?? acceptancePayload.task?.id ??
      acceptancePayload.file?.taskId ?? acceptancePayload.file?.id
    assert.ok(Number(acceptedTaskId) > 0, 'accepted response must identify the server task')
    await expect(page.locator('[data-upload-status="processing"]')).toBeVisible()
    const queuesLink = await navigationLink(page, '/queues')
    assert.ok(queuesLink, 'queues must be available through the actual navigation')
    const [navigationResponse] = await Promise.all([
      page.waitForResponse((response) => new URL(response.url()).pathname === '/queues' &&
        response.request().headers()['x-inertia'] === 'true'),
      queuesLink.click(),
    ])
    assert.ok(navigationResponse.ok())
    await expect(page).toHaveURL(/\/queues(?:\?.*)?$/)
    assert.equal(await page.evaluate(() => window.__starterSmokeDocument), 'same-inertia-document')
    await expect(page.locator('[data-upload-status="processing"]')).toBeVisible()
    const terminal = await deadline(terminalReceived.promise, 'actual terminal upload WebSocket frame')
    assert.equal(Number(terminal.taskId), Number(acceptedTaskId), 'terminal WS must belong to the accepted task')
    assert.equal(terminal.status, 'completed', JSON.stringify(terminal))
    assert.ok(terminal.file?.url, 'terminal WS frame must identify actual persisted file')
    buffering = false
    // Release the real WS frame first. Polling remains blocked until UI proves WS completion.
    for (const send of pendingFrames.splice(0)) send()
    await expect(page.locator('[data-upload-status="completed"]')).toBeVisible()
    await expect(page.locator('[data-profile-menu-trigger] img')).toHaveAttribute('src', terminal.file.url)
    releaseNavigation.resolve()
    const stored = await context.request.get(terminal.file.url)
    assert.ok(stored.ok(), `persisted file GET: ${stored.status()}`)
    assert.match(stored.headers()['content-type'], /^image\//)
    assert.ok((await stored.body()).length > 0)
    await page.reload()
    await expect(page.locator('[data-profile-menu-trigger] img')).toHaveAttribute('src', terminal.file.url)
    result.uploadTerminal = terminal
    check('cropped real TUS upload; Inertia unmount; real terminal WS completes queue and avatar; persisted file survives reload')

    // A frame may observe completion, while only the original widget can settle
    // its durable field fence. Exercise that explicit recovery without uploading again.
    await page.goto('/auth/profile')
    const checkUpload = page.getByRole('button', { name: 'Check upload', exact: true })
    await expect(checkUpload).toBeVisible()
    await expect(page.getByRole('status').filter({ hasText: 'Upload completion is unconfirmed.' })).toBeVisible()
    const recoveryStart = result.requests.length
    await checkUpload.click()
    const useRecovered = page.getByRole('button', { name: 'Use image in this field', exact: true })
    await expect.poll(async () => (await useRecovered.count()) > 0 || (await checkUpload.count()) === 0).toBe(true)
    const confirmedAssignment = (await useRecovered.count()) > 0
    if (confirmedAssignment) await useRecovered.click()
    await expect(checkUpload).toHaveCount(0)
    await expect(useRecovered).toHaveCount(0)
    await expect(page.locator('input[name="image-upload"]')).toBeEnabled()
    await expect(page.locator('.image-uploader .image-preview img')).toHaveAttribute('src', terminal.file.url)
    const recoveryRequests = result.requests.slice(recoveryStart)
    assert.deepEqual(recoveryRequests.filter((request) => request.method === 'PATCH' ||
      (request.method === 'POST' && /^\/files\/tus\/?$/.test(request.path))), [],
    'field recovery must not resend TUS bytes or create another upload')
    for (const request of recoveryRequests.filter((request) => request.method === 'POST' && request.path.endsWith('/complete'))) {
      assert.equal(request.path, new URL(acceptance.url()).pathname, 'recovery must reconcile the original completion session')
    }
    await page.reload()
    await expect(page.locator('[data-profile-menu-trigger] img')).toHaveAttribute('src', terminal.file.url)
    await expect(checkUpload).toHaveCount(0)
    result.uploadRecovery = { confirmedAssignment, newTransfers: 0, requests: recoveryRequests }
    check('matching widget explicitly reconciles retained upload fence and confirms recovered image without retransferring bytes')

    // Screens and keyboard checks use the same real authenticated host, in both presentation builds.
    await page.goto('/auth/profile')
    result.paint = {}
    const lightPaint = await waitForPaint(page, name, 'light', undefined,
      (measured) => { result.paint.light = measured })
    await page.screenshot({ path: path.join(folder, `${name}-desktop-light-en.png`), fullPage: true })
    await page.getByRole('button', { name: /Switch to dark theme|Переключить на темную тему/i }).click()
    await expect(page.locator('html')).toHaveAttribute('data-theme-mode', 'dark')
    await page.reload()
    await expect(page.locator('html')).toHaveAttribute('data-theme-mode', 'dark')
    await waitForPaint(page, name, 'dark', lightPaint,
      (measured) => { result.paint.dark = measured })
    await page.screenshot({ path: path.join(folder, `${name}-desktop-dark-en.png`), fullPage: true })
    await language(page, 'Русский', 'ru')
    await page.reload()
    await expect(page.locator('html')).toHaveAttribute('lang', 'ru')
    const account = page.locator('[data-profile-menu-trigger]')
    await account.click()
    await expect(page.getByRole('menu')).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(account).toBeFocused()
    await page.setViewportSize({ width: 390, height: 844 })
    const toggle = page.getByRole('button', { name: /^(Открыть меню|Open menu)$/ })
    await toggle.click()
    const mobile = page.locator('#admin-mobile-navigation')
    await expect(mobile).toHaveAttribute('role', 'dialog')
    assert.ok(await mobile.evaluate((node) => node.contains(document.activeElement)))
    const focusable = mobile.locator('a[href],button:not([disabled]),select:not([disabled]),[tabindex="0"]').filter({ visible: true })
    await focusable.last().focus()
    await page.keyboard.press('Tab')
    await expect(focusable.first()).toBeFocused()
    await page.screenshot({ path: path.join(folder, `${name}-mobile-dark-ru.png`), fullPage: true })
    await page.keyboard.press('Escape')
    await expect(toggle).toBeFocused()
    await expect(mobile).toHaveAttribute('inert', '')
    assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), 'mobile document must fit viewport')
    await page.setViewportSize({ width: 1440, height: 1000 })
    await language(page, 'English', 'en')
    await page.getByRole('button', { name: /Switch to light theme|Переключить на светлую тему/i }).click()
    await expect(page.locator('html')).toHaveAttribute('data-theme-mode', 'light')
    await waitForPaint(page, name, 'light', undefined,
      (measured) => { result.paint.returnedLight = measured })
    check('mobile layout and keyboard focus trap/restore; persisted light/dark and RU/EN')

    await page.goto('/admins/create')
    await fill(page, { name: `Queue Reader ${name}`, email: credentials.restrictedEmail,
      username: `queues-${name}`, password: credentials.password, passwordConfirm: credentials.password })
    await submit(page, '/admins/create')
    await expect(page).toHaveURL(/\/admins\/\d+$/)
    // Only this run's isolated PostgreSQL receives projections for the two
    // UI-created canonical accounts. No production seed endpoint is added.
    ownedSQL(config, 'INSERT INTO users(subject_id,uuid,created_at,updated_at) SELECT subject_id,gen_random_uuid(),now(),now() FROM administrations ON CONFLICT (subject_id) DO NOTHING')
    for (const resource of ['admins', 'users']) {
      await page.goto(`/${resource}?page=2&limit=1&sortBy=name&sortOrder=asc&search=smoke&status=active`)
      await expect(page.getByRole('row').filter({ has: page.getByRole('button', { name: /^(Просмотр|View)$/ }) })).toHaveCount(1)
      const listURL = page.url()
      await page.getByRole('button', { name: /^(Просмотр|View)$/ }).click()
      await expect(page).toHaveURL(new RegExp(`/${resource}/\\d+\\?returnTo=`))
      await page.getByRole('link', { name: /^(Редактировать|Edit)$/ }).first().click()
      await expect(page).toHaveURL(new RegExp(`/${resource}/\\d+/edit\\?returnTo=`))
      const editURL = page.url()
      await page.reload()
      await page.screenshot({ path: path.join(folder, `${name}-${resource}-edit-light-en.png`), fullPage: true })
      await page.getByRole('button', { name: /Switch to dark theme|Переключить на темную тему/i }).click()
      await language(page, 'Русский', 'ru')
      await expect(page.getByRole('link', { name: 'К списку', exact: true })).toBeVisible()
      await page.setViewportSize({ width: 390, height: 844 })
      assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), 'edit form mobile fit')
      await page.screenshot({ path: path.join(folder, `${name}-${resource}-edit-dark-mobile-ru.png`), fullPage: true })
      await page.setViewportSize({ width: 1440, height: 1000 })
      await language(page, 'English', 'en')
      await page.getByRole('button', { name: /Switch to light theme|Переключить на светлую тему/i }).click()
      await page.locator('input[name="name"]').fill(`Workflow ${resource} ${name}`)
      page.once('dialog', (dialog) => dialog.dismiss())
      await page.getByRole('link', { name: /^(Отмена|Cancel)$/ }).click()
      await expect(page).toHaveURL(editURL)
      await expect(page.locator('input[name="name"]')).toHaveValue(`Workflow ${resource} ${name}`)
      const mutation = page.waitForResponse((response) => response.request().method() === 'PUT' && new URL(response.url()).pathname === new URL(editURL).pathname.replace(/\/edit$/, ''))
      await page.getByRole('button', { name: /^(Сохранить|Save)$/ }).click()
      assert.ok((await mutation).status() < 400)
      await expect(page.locator('form[aria-busy="true"]')).toHaveCount(0)
      await expect(page).toHaveURL(editURL)
      await expect(page.getByText(resource === 'users' ? 'User updated successfully!' : 'Record updated successfully!', { exact: true })).toHaveCount(1)
      await page.reload()
      await expect(page.locator('input[name="name"]')).toHaveValue(`Workflow ${resource} ${name}`)
      await page.getByRole('link', { name: /^(К списку|Back to list)$/ }).click()
      await expect(page).toHaveURL(listURL)
      await expect(page.getByRole('row').filter({ hasText: `Workflow ${resource} ${name}` })).toBeVisible()
      await page.getByRole('button', { name: /^(Редактировать|Edit)$/ }).click()
      await page.locator('input[name="name"]').fill('Discard this change')
      page.once('dialog', (dialog) => dialog.accept())
      await page.getByRole('link', { name: /^(Отмена|Cancel)$/ }).click()
      await expect(page).toHaveURL(listURL)
      await page.screenshot({ path: path.join(folder, `${name}-${resource}-workflow.png`), fullPage: true })
    }
    check('users/admins filtered second-page view/edit/save/reload/return and dirty leave confirmation')
    if (process.argv.includes('--records-only')) {
      assert.deepEqual(result.pageErrors, [], 'uncaught browser errors')
      assert.deepEqual(result.consoleErrors, [], 'browser console errors')
      result.result = 'passed'
      return
    }
    ownedSQL(config, `UPDATE admin_notifications SET is_read=TRUE;
      INSERT INTO admin_notifications(admin_id,title,message)
      SELECT a.id,'Dashboard fixture','Isolated dashboard fixture'
      FROM administrations a CROSS JOIN LATERAL generate_series(1, CASE WHEN a.id=(SELECT MIN(id) FROM administrations) THEN 2 ELSE 1 END) AS copies;`)
    await dashboardAcceptance({ page, folder, name, expect, language, result })
    check('dashboard real queues/private notifications/host card, manual refresh, persisted opt-in, simulated stale rendering, mobile and live locale')
    await page.goto('/roles/create')
    const slug = `smoke-queues-${name}`
    await fill(page, { slug, name: `Smoke Queue Reader ${name}`, description: 'Automated real restricted session' })
    const queueCard = page.getByText('queues', { exact: true }).locator('xpath=ancestor::div[.//label][1]')
    await queueCard.locator('label').filter({ has: page.getByText('read', { exact: true }) }).getByRole('checkbox').click()
    const dashboardPermission = page.getByText('dashboard', { exact: true }).locator('xpath=ancestor::div[.//label][1]')
    await dashboardPermission.locator('label').filter({ has: page.getByText('read', { exact: true }) }).getByRole('checkbox').click()
    await submit(page, '/roles')
    await page.goto('/roles')
    const roleRow = page.getByRole('row').filter({ hasText: slug })
    await expect(roleRow).toBeVisible()
    const [assignedRead, searchRead] = await Promise.all([
      page.waitForResponse((response) => response.request().method() === 'GET' && /\/roles\/\d+\/admins$/.test(new URL(response.url()).pathname)),
      page.waitForResponse((response) => response.request().method() === 'GET' && /\/roles\/\d+\/admins\/search$/.test(new URL(response.url()).pathname)),
      roleRow.getByRole('button', { name: /^(Просмотр|View)$/ }).click(),
    ])
    assert.ok(assignedRead.ok(), 'mounted role page must load assigned admins')
    assert.ok(searchRead.ok(), 'mounted role page must load admin search')
    await expect(page).toHaveURL(/\/roles\/\d+$/)
    await page.goBack()
    await expect(page).toHaveURL(/\/roles(?:\?.*)?$/)
    await expect(roleRow).toBeVisible()
    const [restoredAssignedRead, restoredSearchRead] = await Promise.all([
      page.waitForResponse((response) => response.request().method() === 'GET' && /\/roles\/\d+\/admins$/.test(new URL(response.url()).pathname)),
      page.waitForResponse((response) => response.request().method() === 'GET' && /\/roles\/\d+\/admins\/search$/.test(new URL(response.url()).pathname)),
      page.goForward(),
    ])
    assert.ok(restoredAssignedRead.ok(), 'history-restored role page must load assigned admins')
    assert.ok(restoredSearchRead.ok(), 'history-restored role page must load admin search')
    const filter = page.getByPlaceholder(/Имя, фамилия или email|Name or email/i)
    await filter.fill(credentials.restrictedEmail)
    const [searchResponse] = await Promise.all([
      page.waitForResponse((response) => /\/roles\/\d+\/admins\/search/.test(new URL(response.url()).pathname)),
      page.getByRole('button', { name: /^(Фильтр|Filter)$/ }).click(),
    ])
    assert.ok(searchResponse.ok(), 'role admin search must succeed')
    const assign = page.getByRole('button', { name: /^(Назначить|Assign)$/ })
    await expect(assign).toHaveCount(1)
    const [attachResponse] = await Promise.all([
      page.waitForResponse((response) => /\/roles\/\d+\/admins\/attach/.test(new URL(response.url()).pathname)),
      assign.click(),
    ])
    assert.ok(attachResponse.status() < 400)
    await expect(page.getByText(/^(Уже назначено|Already assigned)$/).first()).toBeVisible()
    await logout(page)
    await login(page, credentials.restrictedEmail, credentials.password)
    await page.goto('/queues')
    await expect(page).toHaveURL(/\/queues(?:\?.*)?$/)
    const restrictedQueues = await navigationLink(page, '/queues')
    assert.ok(restrictedQueues, 'restricted admin must see queues in navigation')
    await expect(restrictedQueues).toBeVisible()
    await page.keyboard.press('Escape')
    assert.equal(await navigationLink(page, '/admins'), null, 'restricted admin must not see admins in any navigation group')
    const denied = await context.request.get('/admins', { maxRedirects: 0 })
    assert.equal(denied.status(), 403, 'real restricted session must be denied by server guard')
    result.restrictedAccess = { granted: '/queues', denied: '/admins', status: denied.status(), role: slug }
    await page.screenshot({ path: path.join(folder, `${name}-restricted-queues.png`), fullPage: true })
    const restrictedSnapshot = page.waitForResponse((r) => new URL(r.url()).pathname === '/dashboard/data')
    await page.goto('/')
    const privateData = await (await restrictedSnapshot).json()
    const unread = privateData.cards.find((card) => card.key === 'goadmin.notifications')
    assert.equal(unread.metrics.find((metric) => metric.key === 'unread-notifications').value, '1', 'notifications must be scoped to the restricted actor')
    const restrictedHost = page.locator('[data-dashboard-card="starter.summary"]')
    await expect(restrictedHost.getByRole('link', { name: 'Administrators', exact: true })).toHaveCount(0)
    result.dashboard.restrictedUnread = 1
    await logout(page)
    await login(page, credentials.email, credentials.password)
    check('UI-created admin and queues:read role assignment; real restricted login, menu filtering, server 403 and logout protection')
    await tableWorkflowAcceptance({ page, context, config, credentials, ownedSQL, folder, name, expect, language, login, logout, result })
    check('table workflows: view CRUD/dirty/relogin, native CSRF/RBAC/preview/owner isolation, both CSV modes on six grids, cross-page cap100, real conflict, bulk results and server queue counters')
    assert.deepEqual(result.pageErrors, [], 'uncaught browser errors')
    assert.deepEqual(result.consoleErrors, [], 'browser console errors')
    result.result = 'passed'
  } catch (error) {
    result.result = 'failed'; result.error = error.stack
    await page.screenshot({ path: path.join(folder, `${name}-failure.png`), fullPage: true }).catch(() => {})
    await writeFile(path.join(folder, `${name}-failure.html`), await page.content()).catch(() => {})
    throw error
  } finally {
    buffering = false
    releaseNavigation.resolve()
    await context.tracing.stop({ path: path.join(folder, `${name}-trace.zip`) })
    await context.close()
    await writeFile(path.join(folder, 'browser-report.json'), JSON.stringify(report, null, 2) + '\n')
  }
}
let failed = false
try {
  for (const [name, config] of Object.entries(manifest.cases)) {
    try { await runCase(name, config) } catch (error) { failed = true; console.error(`${name}: ${error.stack}`) }
  }
} finally {
  report.finishedAt = new Date().toISOString()
  await writeFile(path.join(folder, 'browser-report.json'), JSON.stringify(report, null, 2) + '\n')
  await browser.close()
}
if (failed) process.exitCode = 1
