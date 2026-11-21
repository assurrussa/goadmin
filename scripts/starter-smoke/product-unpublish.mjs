import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { randomBytes, randomUUID } from 'node:crypto'
import { cpSync, mkdtempSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { setTimeout as delay } from 'node:timers/promises'
import { firefox, expect } from 'playwright/test'
import { connectProductEventsMCP as connectStdio } from './mcp-client.mjs'
import { permissionCheckbox } from './role-editor.mjs'

// product-unpublish.py owns this fresh database, app, port and cleanup.
const { ADMIN_URL: origin, UNPUBLISH_QA_PROJECT: project, UNPUBLISH_QA_CLI: binary } = process.env
assert.match(project, /^goadmin-unpublish-[a-f0-9]{12}$/)
const compose = (...args) =>
  execFileSync(
    'docker',
    ['compose', '-p', project, '-f', process.env.UNPUBLISH_QA_COMPOSE || 'compose.yaml', ...args],
    {
      encoding: 'utf8',
      timeout: 120000,
    },
  ).trim()
const sql = (stmt) =>
  compose(
    'exec',
    '-T',
    'postgres',
    'psql',
    '-XAtq',
    '-v',
    'ON_ERROR_STOP=1',
    '-U',
    'fixture',
    '-d',
    'fixture',
    '-c',
    stmt,
  )
const options = { executablePath: process.env.QA_FIREFOX || undefined }
const profile = process.env.UNPUBLISH_QA_PROFILE
const browser = profile ? undefined : await firefox.launch(options)
const contexts = []
const clients = []
const password = randomBytes(24).toString('base64url') + 'Aa1!'
const email = 'owner@example.invalid'
const connectPath = '/agent/product-unpublish/connect'
const approve = 'Снять с публикации'
const decline = 'Отклонить'
let stage = 'register owner'

async function newPage() {
  let ctx
  if (profile) {
    const isolated = mkdtempSync(join(dirname(profile), 'login-'))
    cpSync(profile, isolated, { recursive: true })
    ctx = await firefox.launchPersistentContext(isolated, {
      ...options,
      baseURL: origin,
      locale: 'ru-RU',
    })
  } else {
    ctx = await browser.newContext({ baseURL: origin })
  }
  contexts.push(ctx)
  return ctx.newPage()
}
async function fill(page, values) {
  for (const [name, value] of Object.entries(values))
    await page.locator(`input[name="${name}"]`).fill(value)
}
async function submit(
  page,
  route,
  button = page.locator('form button[type="submit"]').first(),
  method = 'POST',
) {
  const [reply] = await Promise.all([
    page.waitForResponse(
      (r) => new URL(r.url()).pathname === route && r.request().method() === method,
    ),
    button.click(),
  ])
  assert.ok(reply.status() < 400, `${route} returned ${reply.status()}`)
}
async function login(page, account = email) {
  await page.goto('/auth/login')
  await fill(page, { email: account, password })
  await submit(page, '/auth/login')
  await expect(page.locator('[data-profile-menu-trigger]')).toBeVisible()
}
async function actorID(page) {
  await page.goto('/auth/profile')
  const id = await page
    .locator('#app')
    .evaluate((app) => JSON.parse(app.dataset.page).props.authuser.idExact)
  assert.match(id, /^[1-9]\d*$/)
  return id
}
function seed(owner) {
  assert.match(owner, /^[1-9]\d*$/)
  return sql(`INSERT INTO starter_products(owner_admin_id,name,description,stock,active,available,status)
    VALUES(${owner},'MCP fixture','',0,true,'','published') RETURNING id`)
}
async function newClient() {
  const cli = await connectStdio(binary, origin)
  clients.push(cli)
  return cli
}
async function tool(cli, name, args, expectedError) {
  const result = await cli.callTool(name, args)
  assert.equal(
    result.isError === true,
    Boolean(expectedError),
    `${name} ${result.structuredContent?.error ?? ''}`,
  )
  if (expectedError) assert.equal(result.structuredContent.error, expectedError)
  return result.structuredContent
}
function input(product) {
  return {
    request_id: randomUUID(),
    product_id: product,
    reason: 'Причина проверена оператором',
  }
}
const propose = (cli, args, err) => tool(cli, 'propose_unpublish', args, err)
const status = (cli, args, err) =>
  tool(cli, 'get_unpublish_request', { request_id: args.request_id }, err)
async function pair(page, cli) {
  const key = { request_id: randomUUID() }
  const instruction = await status(cli, key, 'authorization_required')
  assert.equal(instruction.verification_url, origin + connectPath)
  await page.goto(instruction.verification_url)
  await page.locator('input[name="code"]').fill(instruction.code)
  await submit(page, connectPath, page.getByRole('button', { name: /^(Подключить|Connect)$/ }))
  await status(cli, key, 'not_found')
}
async function review(page, req, action) {
  await page.goto(req.review_url)
  await expect(page.locator('dd').filter({ hasText: 'MCP fixture' })).toBeVisible()
  await expect(page.getByText('Причина проверена оператором', { exact: true })).toBeVisible()
  await submit(
    page,
    new URL(req.review_url).pathname,
    page.getByRole('button', { name: action, exact: true }),
  )
  await expect(page.getByRole('button', { name: approve, exact: true })).toHaveCount(0)
}
async function checkActionAppearance(page, reviewURL) {
  const original = page.viewportSize()
  for (const colorScheme of ['light', 'dark']) {
    await page.emulateMedia({ colorScheme })
    await page.setViewportSize({ width: colorScheme === 'dark' ? 390 : 1280, height: 844 })
    for (const route of [connectPath, reviewURL]) {
      await page.goto(route)
      await expect(page.locator('html')).toHaveAttribute('data-theme-mode', colorScheme)
      assert.ok(
        await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 1),
      )
      if (route === connectPath) {
        const input = page.locator('input[name="code"]')
        await input.fill('ABCDE-FGHIJ')
        await input.press('Tab')
        await expect(page.getByRole('button', { name: /^(Подключить|Connect)$/ })).toBeFocused()
      } else {
        const button = page.getByRole('button', { name: approve, exact: true })
        await button.focus()
        await page.keyboard.press('Tab')
        await expect(page.getByRole('button', { name: decline, exact: true })).toBeFocused()
      }
    }
  }
  if (original) await page.setViewportSize(original)
  await page.emulateMedia({ colorScheme: null })
}

async function restrictedOperator(owner) {
  const address = 'operator@example.invalid'
  const slug = 'unpublish-pilot-operator'
  await owner.goto('/admins/create')
  await fill(owner, {
    name: 'Operator',
    email: address,
    username: 'pilotoperator',
    password,
    passwordConfirm: password,
  })
  await submit(owner, '/admins/create')
  await expect(owner).toHaveURL(/\/admins\/\d+$/)
  await owner.goto('/roles/create')
  await fill(owner, {
    name: 'Fixture operator',
    slug,
    description: 'Unpublish permission test',
  })
  await permissionCheckbox(owner, 'dashboard:read').click()
  await permissionCheckbox(owner, 'products:unpublish').click()
  await submit(owner, '/roles')
  await owner.goto('/roles')
  await owner
    .getByRole('row')
    .filter({ hasText: slug })
    .getByRole('button', { name: /^(Просмотр|View)$/ })
    .click()
  await expect(owner).toHaveURL(/\/roles\/\d+(?:\?.*)?$/)
  const role = new URL(owner.url()).pathname
  await owner.getByPlaceholder(/Имя, фамилия или email|Name or email/i).fill(address)
  await Promise.all([
    owner.waitForResponse((r) => new URL(r.url()).pathname === role + '/admins/search'),
    owner.getByRole('button', { name: /^(Фильтр|Filter)$/ }).click(),
  ])
  const row = owner.getByText(address, { exact: true }).locator('xpath=ancestor::div[.//button][1]')
  await submit(
    owner,
    role + '/admins/attach',
    row.getByRole('button', { name: /^(Назначить|Assign)$/ }),
  )
  const page = await newPage()
  await login(page, address)
  return { page, role }
}

try {
  const page = await newPage()
  await page.goto('/auth/register')
  await fill(page, {
    name: 'Owner',
    lastName: 'Fixture',
    email,
    username: 'pilotowner',
    password,
    passwordConfirm: password,
    setupToken: process.env.FIRST_ADMIN_SETUP_TOKEN,
  })
  await submit(page, '/auth/register')
  await expect(page.locator('[data-profile-menu-trigger]')).toBeVisible()
  const owner = await actorID(page)
  if (profile) {
    const cookie = (await page.context().cookies()).find((c) => c.name === '__Host-goadmin_session')
    assert.ok(
      cookie?.secure && cookie.httpOnly && cookie.path === '/' && cookie.domain === 'localhost',
    )
  }
  const first = await newClient()
  assert.deepEqual((await first.listTools()).tools.map((t) => t.name).sort(), [
    'get_unpublish_request',
    'propose_unpublish',
  ])
  await pair(page, first)
  stage = 'immutable proposal and receipt'
  const id = seed(owner)
  const args = input(id)
  const req = await propose(first, args)
  assert.equal(req.status, 'pending')
  assert.deepEqual(await propose(first, args), req)
  await propose(first, { ...args, reason: 'Другая причина' }, 'conflict')
  assert.equal(sql(`SELECT status FROM starter_products WHERE id=${id}`), 'published')
  const otherLogin = await newPage()
  await login(otherLogin)
  const wrong = await otherLogin.goto(req.review_url)
  assert.ok([403, 404].includes(wrong.status()))
  if (profile) await checkActionAppearance(page, req.review_url)
  await review(page, req, approve)
  if (process.env.UNPUBLISH_QA_JOURNAL) {
    const journal = page.locator('[role="status"] ~ a[href="/product-events"]')
    await expect(journal).toBeVisible()
    await page.reload()
    await expect(journal).toBeVisible()
    await journal.click()
    await expect(page).toHaveURL(/\/product-events$/)
  }
  const receipt = await status(first, args)
  assert.equal(receipt.status, 'succeeded')
  assert.ok(/^[1-9]\d*$/.test(receipt.event_id))
  assert.equal(sql(`SELECT status||':'||revision FROM starter_products WHERE id=${id}`), 'draft:2')
  assert.equal(
    sql(
      `SELECT reason FROM starter_product_events WHERE product_id='${id}' AND action='unpublish'`,
    ),
    args.reason,
  )
  assert.deepEqual(await propose(first, args), receipt)
  console.log('PASS browser approval, isolated login, unchanged proposal, single event and replay')

  stage = 'status while the database is unavailable'
  compose('pause', 'postgres')
  try {
    await status(first, args, 'outcome_unknown')
  } finally {
    compose('unpause', 'postgres')
  }
  await expect
    .poll(
      () => {
        try {
          return sql('SELECT 1')
        } catch {
          return ''
        }
      },
      { timeout: 60000 },
    )
    .toBe('1')
  await expect
    .poll(
      async () => {
        const reply = await first.callTool('get_unpublish_request', {
          request_id: args.request_id,
        })
        if (reply.isError) assert.equal(reply.structuredContent.error, 'outcome_unknown')
        return reply.structuredContent
      },
      { timeout: 60000 },
    )
    .toEqual(receipt)
  console.log('PASS unavailable database, then recovery of the same committed event')

  stage = 'decline and stale version'
  const id2 = seed(owner)
  const rejected = input(id2)
  await review(page, await propose(first, rejected), decline)
  assert.equal((await propose(first, rejected)).status, 'declined')
  const stale = input(id2)
  const pending = await propose(first, stale)
  sql(`UPDATE starter_products SET revision=revision+1 WHERE id=${id2}`)
  await review(page, pending, approve)
  assert.equal((await status(first, stale)).status, 'conflict')
  assert.equal(sql(`SELECT status FROM starter_products WHERE id=${id2}`), 'published')

  stage = 'loss of the operator permission'
  const restricted = await restrictedOperator(page)
  const operatorID = await actorID(restricted.page)
  const restrictedClient = await newClient()
  await pair(restricted.page, restrictedClient)
  if (process.env.UNPUBLISH_QA_JOURNAL) {
    const permitted = await propose(restrictedClient, input(seed(operatorID)))
    await review(restricted.page, permitted, approve)
    await expect(
      restricted.page.locator('[role="status"] ~ a[href="/product-events"]'),
    ).toHaveCount(0)
  }
  const scoped = input(seed(operatorID))
  await propose(restrictedClient, args, 'not_found')
  const original = await propose(restrictedClient, scoped)
  await restricted.page.goto(original.review_url)
  await page.goto(restricted.role + '/edit')
  await permissionCheckbox(page, 'products:unpublish').click()
  await submit(page, restricted.role, page.locator('form button[type="submit"]').first(), 'PUT')
  const [denial] = await Promise.all([
    restricted.page.waitForResponse(
      (r) => r.url() === original.review_url && r.request().method() === 'POST',
    ),
    restricted.page.getByRole('button', { name: approve, exact: true }).click(),
  ])
  assert.ok(
    [401, 403, 404].includes(denial.status()),
    'review form opened before role removal must be denied',
  )
  assert.equal(
    sql(`SELECT status FROM starter_products WHERE id=${scoped.product_id}`),
    'published',
  )
  console.log(
    'PASS decline, concurrent edit, owner scope and removal of products:unpublish before confirmation',
  )

  stage = 'closed connections and deadline'
  const ended = input(id2)
  const open = await propose(first, ended)
  await page.goto('/auth/profile')
  await page.locator('[data-profile-menu-trigger]').click()
  await page.getByRole('menuitem', { name: /^(Выйти|Log out|Logout|Sign out)$/i }).click()
  await expect(page).toHaveURL(/\/auth\/login/)
  await status(first, ended, 'authorization_expired')
  await login(page)
  await pair(page, first)
  assert.equal((await status(first, ended)).status, 'expired')
  await page.goto(open.review_url)
  await expect(page.getByRole('button', { name: approve, exact: true })).toHaveCount(0)
  const deadline = input(id2)
  const expiring = await propose(first, deadline)
  await page.goto(expiring.review_url)
  if (process.env.QA_NATURAL_EXPIRY === '1') {
    console.log('Waiting for the original 15-minute connection deadline')
    await delay(901000)
  } else {
    sql(`UPDATE admin_confirmed_actions SET created_at=statement_timestamp()-interval '16 minutes',
      expires_at=statement_timestamp()-interval '1 minute' WHERE request_id='${deadline.request_id}'`)
  }
  await page.reload()
  await expect(page.getByRole('button', { name: approve, exact: true })).toHaveCount(0)
  assert.equal(sql(`SELECT status FROM starter_products WHERE id=${id2}`), 'published')
  console.log('PASS original login and confirmation deadline')

  stage = 'recovery after a process restart'
  sql(`DELETE FROM starter_products WHERE id=${id}`)
  compose('restart', 'admin')
  await expect
    .poll(
      async () => {
        try {
          return (await fetch(origin + '/healthz')).ok
        } catch {
          return false
        }
      },
      { timeout: 90000 },
    )
    .toBe(true)
  const fresh = await newClient()
  await pair(page, fresh)
  assert.deepEqual(await status(fresh, args), receipt)
  console.log('PASS exact result and event after deletion, restart and fresh pairing')
} catch (error) {
  console.error('FAILED ' + stage)
  throw error
} finally {
  for (const c of clients) await c.close()
  for (const ctx of contexts) await ctx.close()
  await browser?.close()
}
