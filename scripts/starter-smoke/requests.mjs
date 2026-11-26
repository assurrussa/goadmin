import assert from 'node:assert/strict'

const requests = '/requests'
const view = { name: /^(View|Просмотр)$/ }
const back = { name: /^(Back to list|К списку)$/ }

function locationOf(url) {
  const parsed = new URL(url)
  return parsed.pathname + parsed.search
}

async function confirmCommand(page, label) {
  await page.getByRole('button', { name: label, exact: true }).click()
  return page.locator('[data-confirmation-accept]:visible')
}

export async function requestsReaderAcceptance({ page, context, config, expect }) {
  await page.goto(requests)
  const list = await context.request.get(requests + '/data?limit=1')
  assert.equal(list.status(), 200)
  const row = (await list.json()).data[0].item
  const recordPath = requests + '/records/' + encodeURIComponent(row.id)
  await page.goto(recordPath)
  await expect(page.getByText(row.subject, { exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: /^(Одобрить|Отклонить)$/ })).toHaveCount(0)
  const csrf = (await context.cookies(config.url)).find(cookie => cookie.name === 'csrf_token')?.value
  assert.ok(csrf)
  for (const key of ['approve', 'reject']) {
    const data = { _revision: row._revision }
    if (key === 'reject') data.params = { reason: 'Проверка разрешений' }
    const response = await context.request.post(recordPath + '/actions/' + key, {
      data, maxRedirects: 0, headers: { Accept: 'application/json', Origin: config.url,
        Referer: config.url + recordPath, 'X-CSRF-Token': csrf },
    })
    assert.equal(response.status(), 403, key)
  }
}

export async function requestsAcceptance({ page, context, expect, result }) {
  // Each layout owns its database. The standalone starter supplies six pending requests.
  await page.goto(requests + '?limit=2')
  const search = page.locator('[data-grid-search]')
  await search.fill('Доступ')
  await expect.poll(() => new URL(page.url()).searchParams.get('search')).toBe('Доступ')
  const filters = page.locator('[data-part="filters"]')
  if (!(await filters.getByRole('combobox').isVisible())) await filters.getByRole('button', { name: /Filters|Фильтры/ }).click()
  await filters.getByRole('combobox').click()
  await page.getByRole('option', { name: 'Ожидает решения' }).click()
  await expect.poll(() => new URL(page.url()).searchParams.get('status')).toBe('pending')
  await page.locator('[data-part="sort"]').click()
  await expect.poll(() => new URL(page.url()).searchParams.get('sortOrder')).toBe('desc')
  await page.locator('[data-part="pagination"]').getByRole('button', { name: '2', exact: true }).click()
  await expect.poll(() => new URL(page.url()).searchParams.get('page')).toBe('2')
  const origin = locationOf(page.url())
  const links = page.locator('[data-part="table"]').getByRole('link', view)
  await expect(links).toHaveCount(2)
  await expect(page.getByRole('cell', { name: 'Ожидает решения', exact: true })).toHaveCount(2)
  await expect(page.locator('[data-part="table"]').getByRole('checkbox')).toHaveCount(0)
  await expect(page.getByRole('button', { name: /^(Одобрить|Отклонить)$/ })).toHaveCount(0)
  for (const link of await links.all()) {
    await expect.poll(async () => new URL(await link.getAttribute('href'), page.url()).searchParams.get('returnTo')).toBe(origin)
  }
  const firstHref = await links.first().getAttribute('href')
  const copiedHref = await links.last().getAttribute('href')
  assert.notEqual(firstHref, copiedHref)
  const [approved] = await Promise.all([context.waitForEvent('page', { timeout: 10000 }), links.first().click({ button: 'middle' })])
  const rejected = await context.newPage()
  const stale = await context.newPage()
  try {
    await rejected.goto(copiedHref)
    await stale.goto(firstHref)
    await stale.getByRole('button', { name: 'Отклонить', exact: true }).click()
    const accept = await confirmCommand(approved, 'Одобрить')
    await accept.click()
    await expect(approved.getByText('Одобрена', { exact: true })).toBeVisible()
    const conflict = stale.waitForResponse(r => r.request().method() === 'POST' && r.url().includes('/actions/reject'))
    await stale.locator('[data-confirmation-accept]:visible').click()
    assert.equal((await conflict).status(), 409)
    await expect(stale.getByText('The record changed. This action is no longer available.', { exact: true })).toBeVisible()
    await stale.getByRole('button', { name: 'Open record', exact: true }).click()
    await expect(stale.getByText('Одобрена', { exact: true })).toBeVisible()
    await expect(stale.getByRole('button', { name: 'Отклонить', exact: true })).toHaveCount(0)
    const decline = await confirmCommand(rejected, 'Отклонить')
    const validation = rejected.waitForResponse(r => r.request().method() === 'POST' && r.url().includes('/actions/reject'))
    await decline.click()
    assert.equal((await validation).status(), 422)
    const reason = rejected.locator('[name="reason"]')
    await expect(reason).toHaveAttribute('aria-invalid', 'true')
    await reason.fill('Не указан срок доступа')
    const submitted = rejected.waitForResponse(r => r.request().method() === 'POST' && r.url().includes('/actions/reject'))
    await decline.click()
    assert.equal((await submitted).status(), 204)
    await expect(rejected.getByText('Отклонена', { exact: true })).toBeVisible()
    await expect(rejected.getByText('Не указан срок доступа', { exact: true })).toBeVisible()
    for (const detail of [approved, rejected]) {
      const list = detail.getByRole('link', back)
      await expect(list).toHaveAttribute('href', origin)
      await list.click()
      await expect.poll(() => locationOf(detail.url())).toBe(origin)
    }
    // Open a remaining record in the same tab and use browser history.
    await rejected.locator('[data-part="table"]').getByRole('link', view).first().click()
    await expect(rejected).toHaveURL(/\/requests\/records\//)
    await rejected.goBack()
    await expect.poll(() => locationOf(rejected.url())).toBe(origin)
    result.requestQueue = { approved: firstHref, rejected: copiedHref, emptyReason: 422, staleEmptyReason: 409 }
  } finally {
    await Promise.all([approved.close(), rejected.close(), stale.close()])
  }
}

// A v1 preference document produced from the former explicit Products columns.
export async function productsLegacyViewAcceptance({ page, config, ownedSQL, expect }) {
  const keys = ['name', 'stock', 'active', 'available', 'category', 'status']
  const columns = ['status', 'name', 'stock']
  const stored = { schemaVersion: 1, columns, knownColumns: keys, defaultViewId: 'legacy-products', views: [{
    id: 'legacy-products', name: 'Legacy catalog view', state: {
      columns, knownColumns: keys, sortBy: 'stock', sortOrder: 'desc', limit: 20,
      search: 'Legacy catalog', filters: { category: 'premium', status: 'published' },
    },
  }] }
  ownedSQL(config, `INSERT INTO goadmin_admin_preferences(admin_id,namespace,payload)
    SELECT min(id),'crud.products','${JSON.stringify(stored)}'::jsonb FROM administrations
    ON CONFLICT(admin_id,namespace) DO UPDATE SET payload=excluded.payload,version=goadmin_admin_preferences.version+1;
    INSERT INTO starter_products(owner_admin_id,name,description,stock,active,available,category,status)
    SELECT a.id,'Legacy catalog '||p.name,'',p.stock,true,'',p.category,p.status FROM
    (SELECT min(id) AS id FROM administrations) a CROSS JOIN
    (VALUES ('first',19,'premium','published'),('second',8,'premium','published'),
      ('draft',30,'premium','draft'),('standard',40,'standard','published')) p(name,stock,category,status)`)
  await page.goto('/products')
  const table = page.locator('[data-part="table"]')
  await expect(page.locator('[data-part="workflow"] select').first()).toHaveValue('legacy-products')
  await expect(table.locator('tbody tr')).toHaveCount(2)
  await expect(table.locator('tbody tr').first()).toContainText('Legacy catalog first')
  await expect(table.locator('tbody tr').last()).toContainText('Legacy catalog second')
  await expect(table.locator('thead th[scope="col"]')).toHaveText(['Status', 'Name', 'Stock'])
  for (const [key, value] of Object.entries({ sortBy: 'stock', sortOrder: 'desc', category: 'premium', status: 'published' })) {
    await expect.poll(() => new URL(page.url()).searchParams.get(key)).toBe(value)
  }
}
