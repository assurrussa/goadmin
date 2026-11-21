import assert from 'node:assert/strict'
import { connectProductEventsMCP } from './mcp-client.mjs'
import { permissionCheckbox } from './role-editor.mjs'

const journal = '/product-events'
const gateway = '/agent/product-events'
const connect = gateway + '/connect'
const search = 'search_product_events'
const get = 'get_product_event'
const decisionNames = {
  approve: /^(Allow reading|Разрешить чтение)$/,
  deny: /^(Deny|Отклонить)$/,
  revoke: /^(Revoke|Отозвать)$/,
}

async function json(context, url) {
  const response = await context.request.get(url, { maxRedirects: 0 })
  assert.equal(response.status(), 200, url)
  return response.json()
}

async function sessionPost(context, url, route, data, proof = true) {
  const csrf = (await context.cookies(url)).find(cookie => cookie.name === 'csrf_token')
  assert.ok(csrf)
  return context.request.post(route, { data, maxRedirects: 0, headers: {
    Accept: 'application/json', Origin: url, Referer: url + connect,
    ...(proof ? { 'X-CSRF-Token': csrf.value } : {}),
  } })
}

export async function journalMCPDisabled(context, url) {
  assert.equal((await context.request.get(connect, { maxRedirects: 0 })).status(), 404)
  for (const route of ['connect', 'pair', 'token', 'search', 'get']) {
    assert.equal((await sessionPost(context, url, gateway + '/' + route, {})).status(), 404, route)
  }
}

async function call(client, name, args, expectedError) {
  const reply = await client.callTool(name, args)
  assert.equal(reply.isError === true, Boolean(expectedError), name)
  const body = reply.structuredContent
  assert.ok(body, name)
  if (expectedError) assert.equal(body.error, expectedError, name)
  return body
}

function query(product, page = 1, size = 25) {
  return { product_id: product, action: 'unpublish', page, page_size: size }
}

async function comparePage(client, context, args) {
  const params = new URLSearchParams({ search: args.product_id, action: args.action,
    page: String(args.page), limit: String(args.page_size) })
  const grid = await json(context, journal + '/data?' + params)
  const found = await call(client, search, args)
  const rows = grid.data.map(({ item }) => {
    assert.ok(!Object.hasOwn(item, 'reason'))
    return { id: item.id, productId: item.productId, action: item.action, createdAt: item.createdAt }
  })
  assert.deepEqual(found, { rows, total: grid.meta.pagination.total })
  return found
}

async function compareDetail(client, browserPage, row, expect) {
  const detail = await call(client, get, { event_id: row.id })
  assert.deepEqual(detail, { ...row, reason: detail.reason })
  assert.ok(detail.reason === null || typeof detail.reason === 'string')
  await browserPage.goto(journal + '/records/' + row.id)
  const field = browserPage.locator('dl > div').filter({
    has: browserPage.locator('dt').filter({ hasText: /^(Reason|Причина)/ }),
  }).locator('dd')
  await expect(field).toBeVisible()
  assert.equal(await field.textContent(), detail.reason ?? '—')
  return detail.reason
}

async function setJournalPermission(page, id, enabled, expect) {
  await page.goto('/roles/' + id + '/edit')
  const permission = permissionCheckbox(page, 'product_events:read')
  await expect(permission).toHaveAttribute('aria-checked', String(!enabled))
  await permission.click()
  const [saved] = await Promise.all([
    page.waitForResponse(r => new URL(r.url()).pathname === '/roles/' + id && r.request().method() === 'PUT'),
    page.getByRole('button', { name: /^(Сохранить|Save)$/ }).click(),
  ])
  assert.ok(saved.status() < 400, 'role editor saved the journal permission')
  await expect(page.locator('form[aria-busy="true"]')).toHaveCount(0)
}

async function decide(page, decision, scope = page) {
  const [reply] = await Promise.all([
    page.waitForResponse(r => new URL(r.url()).pathname === connect && r.request().method() === 'POST'),
    scope.getByRole('button', { name: decisionNames[decision] }).click(),
  ])
  assert.equal(reply.status(), 200, decision)
  assert.ok(Array.isArray((await reply.json()).grants))
}

// Pairing codes and browser sessions stay in memory. In particular these contexts
// are never traced, screenshotted or exported as storageState by the smoke runner.
export async function journalMCPAcceptance({ browser, config, credentials, binary, roleSlug, product, reasons,
  ownedSQL, login, logout, language, expect }) {
  const contexts = []
  const clients = []
  const secrets = new Set([credentials.password])
  async function operator(email) {
    const context = await browser.newContext({ baseURL: config.url, locale: 'en-GB' })
    contexts.push(context)
    const page = await context.newPage()
    await login(page, email, credentials.password)
    return { context, page }
  }
  async function newClient() {
    const client = await connectProductEventsMCP(binary, config.url)
    clients.push(client)
    return client
  }
  async function pair(client, operator, args, decision = 'approve') {
    const instruction = await call(client, search, args, 'authorization_required')
    assert.ok(typeof instruction.code === 'string' && /^[A-Z2-7]{5}-[A-Z2-7]{5}$/.test(instruction.code), 'expected pairing code')
    secrets.add(instruction.code)
    assert.equal(instruction.verification_url, config.url + connect)
    const pending = await call(client, search, args, 'authorization_pending')
    assert.ok(pending.code === instruction.code, 'pending code must match')
    await operator.page.goto(instruction.verification_url)
    const body = { decision: 'approve', code: instruction.code }
    assert.equal((await sessionPost(operator.context, config.url, connect, body, false)).status(), 419)
    await operator.page.locator('input[name="code"]').fill(instruction.code)
    await decide(operator.page, decision)
    assert.equal((await sessionPost(operator.context, config.url, connect, body)).status(), 404, 'a used code cannot approve another grant')
    return instruction.code
  }

  try {
    const owner = await operator(credentials.email)
    const restricted = await operator(credentials.restrictedEmail)
    const appearance = await json(restricted.context, '/auth/profile/appearance')
    const roles = await json(owner.context, '/roles/data?limit=100')
    const role = roles.data.find(row => row.item.slug === roleSlug)
    assert.ok(role)
    const roleID = Number(role.selectionId)
    assert.ok(Number.isSafeInteger(roleID) && roleID > 0)
    await restricted.page.goto('/auth/profile')
    const actorID = await restricted.page.locator('#app').evaluate(app => JSON.parse(app.dataset.page).props.authuser.idExact)
    assert.match(actorID, /^[1-9]\d*$/)
    assert.match(product, /^[1-9]\d*$/)
    const fixture = String(Date.now())
    const historical = fixture + '1'
    const otherProduct = fixture + '2'
    ownedSQL(config, `INSERT INTO starter_product_events(owner_admin_id, product_id, action, reason)
      SELECT owner_admin_id, '${historical}', 'unpublish', NULL FROM starter_product_events
      WHERE product_id='${product}' LIMIT 1;
      INSERT INTO starter_product_events(owner_admin_id, product_id, action, reason)
      VALUES (${actorID}, '${otherProduct}', 'unpublish', 'Second operator fixture')`)

    const first = await newClient()
    const tools = (await first.listTools()).tools
    assert.deepEqual(tools.map(tool => tool.name).sort(), [get, search])
    assert.ok(tools.every(tool => tool.annotations?.readOnlyHint === true))
    const approvedCode = await pair(first, owner, query(product))
    await call(first, search, { ...query(product), action: 'unknown-event' }, 'invalid_input')
    const archived = await comparePage(first, owner.context, query(product))
    assert.equal(archived.total, 2)
    assert.equal(archived.rows.length, 2)
    for (let page = 1; page <= 3; page++) {
      const single = await comparePage(first, owner.context, query(product, page, 1))
      assert.deepEqual(single.rows, archived.rows.slice(page - 1, page))
    }
    const actual = []
    for (const row of archived.rows) actual.push(await compareDetail(first, owner.page, row, expect))
    assert.deepEqual(actual.sort(), [...reasons].sort())
    const old = await comparePage(first, owner.context, query(historical))
    assert.equal(old.rows.length, 1)
    assert.equal(await compareDetail(first, owner.page, old.rows[0], expect), null)

    const ownerEvent = { event_id: archived.rows[0].id }
    const rolePreview = (id) => sessionPost(owner.context, config.url, '/auth/role-preview', { roleId: id })
    assert.equal((await rolePreview(roleID)).status(), 200)
    try {
      assert.equal((await owner.context.request.get(journal + '/data', { maxRedirects: 0 })).status(), 403)
      await call(first, get, ownerEvent, 'denied')
    } finally {
      assert.equal((await rolePreview(0)).status(), 200)
    }
    assert.equal((await call(first, get, ownerEvent)).id, ownerEvent.event_id)
    assert.equal((await restricted.context.request.get(connect, { maxRedirects: 0 })).status(), 403)
    const second = await newClient()
    let restrictedEvent
    await setJournalPermission(owner.page, roleID, true, expect)
    try {
      await pair(second, restricted, query(otherProduct))
      const own = await comparePage(second, restricted.context, query(otherProduct))
      assert.equal(own.total, 1)
      assert.equal(await compareDetail(second, restricted.page, own.rows[0], expect), 'Second operator fixture')
      assert.deepEqual(await comparePage(second, restricted.context, query(product)), { rows: [], total: 0 })
      assert.deepEqual(await comparePage(first, owner.context, query(otherProduct)), { rows: [], total: 0 })
      await call(second, get, ownerEvent, 'not_found')
      restrictedEvent = { event_id: own.rows[0].id }
      await call(first, get, restrictedEvent, 'not_found')
    } finally {
      await setJournalPermission(owner.page, roleID, false, expect)
    }
    assert.equal((await restricted.context.request.get(journal + '/records/' + restrictedEvent.event_id, { maxRedirects: 0 })).status(), 403)
    await call(second, get, restrictedEvent, 'denied')

    await owner.page.goto(connect)
    await language(owner.page, 'Русский', 'ru')
    await expect(owner.page.getByText('Подключения к журналу товаров', { exact: true })).toBeVisible()
    await language(owner.page, 'English', 'en')
    await expect(owner.page.getByText('Product journal connections', { exact: true })).toBeVisible()
    const grant = owner.page.locator('li').filter({ has: owner.page.getByText(approvedCode, { exact: true }) })
    await decide(owner.page, 'revoke', grant)
    await call(first, get, ownerEvent, 'authorization_expired')
    await pair(first, owner, query(product), 'deny')
    await call(first, search, query(product), 'denied')
    await pair(first, owner, query(product))
    assert.equal((await call(first, get, ownerEvent)).id, ownerEvent.event_id)
    await logout(owner.page)
    await call(first, get, ownerEvent, 'authorization_expired')
    assert.deepEqual(await json(restricted.context, '/auth/profile/appearance'), appearance)
    return { tools: tools.map(tool => tool.name), archivedReasons: actual.length, historicalNull: true,
      ownerIsolation: true, permissionRevocation: true, rolePreview: true, grantRevocation: true, logout: true }
  } catch (error) {
    let report = String(error.stack ?? error)
    for (const secret of secrets) report = report.replaceAll(secret, '[REDACTED]')
    throw new Error(report)
  } finally {
    await Promise.allSettled(clients.map(client => client.close()))
    await Promise.allSettled(contexts.map(context => context.close()))
  }
}
