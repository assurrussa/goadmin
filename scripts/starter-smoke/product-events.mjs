import assert from 'node:assert/strict'
import path from 'node:path'

const journal = '/product-events'
const actionLabels = {
  created: 'Created', updated: 'Updated', publish: 'Published', unpublish: 'Unpublished', deleted: 'Deleted',
}

// The CRUD acceptance just deleted this product through the real application.
export async function productEventsAcceptance({ page, context, browser, config, folder, name, expect, result, ownedSQL }) {
  const id = result.crud.form.id
  assert.match(id, /^[1-9]\d*$/)
  // Seed equal timestamps on either side of a page boundary in the owned smoke DB.
  ownedSQL(config, `UPDATE starter_product_events SET created_at='2099-10-01 10:11:12+09'
    WHERE id IN (SELECT id FROM starter_product_events WHERE product_id='${id}' ORDER BY id DESC LIMIT 2)`)
  const search = page.locator('[data-grid-search]')
  await page.goto(journal)
  await expect(page.getByRole('heading', { name: 'Product activity', exact: true })).toBeVisible()
  await search.fill(`missing-${id}`)
  await expect(page.getByRole('heading', { name: 'Nothing found', exact: true })).toBeVisible()
  const [response] = await Promise.all([
    page.waitForResponse(r => {
      const url = new URL(r.url())
      return url.pathname === journal + '/data' && url.searchParams.get('search') === id
    }),
    search.fill(id),
  ])
  assert.equal(response.status(), 200)
  const grid = await response.json()
  assert.ok(grid.data.length >= 2)
  for (const row of grid.data) {
    assert.equal(row.item.productId, id)
    assert.deepEqual(row.actions.map(action => action.key), ['view'])
    assert.equal('_revision' in row.item, false)
  }
  assert.equal(grid.data[0].item.action, 'deleted')
  const first = grid.data[0].item
  const expected = grid.data[1].item
  assert.equal(first.createdAt, '2099-10-01 01:11:12 UTC')
  assert.equal(expected.createdAt, first.createdAt)
  assert.ok(BigInt(first.id) > BigInt(expected.id), 'equal timestamps sort by event ID descending')
  assert.ok(!grid.config.workflow.exportUrl)
  await expect(page.getByRole('cell', { name: 'Deleted', exact: true })).toBeVisible()
  const workflow = page.locator('[data-part="workflow"]')
  const viewName = `Activity ${name}`
  await workflow.getByRole('textbox', { name: 'View name', exact: true }).fill(viewName)
  const [saved] = await Promise.all([
    page.waitForResponse(r => new URL(r.url()).pathname === journal + '/table-preferences' && r.request().method() === 'PUT'),
    workflow.getByRole('button', { name: 'Save as new', exact: true }).click(),
  ])
  assert.equal(saved.status(), 200)
  const preferences = await context.request.get(journal + '/table-preferences')
  const view = (await preferences.json()).payload.views.find(view => view.name === viewName)
  assert.equal(view.state.search, id)

  const conditions = { search: id, page: '2', limit: '1', sortBy: 'createdAt', sortOrder: 'desc' }
  const location = journal + '?' + new URLSearchParams(conditions)
  const assertRoundTrip = async (target, viewport) => {
    await target.goto(location)
    const table = target.locator('[data-part="table"]')
    const row = table.locator('tbody tr')
    await expect(row).toHaveCount(1)
    await expect(row.getByRole('cell', { name: expected.createdAt, exact: true })).toBeVisible()
    await expect(table.getByRole('checkbox')).toHaveCount(0)
    await expect(target.getByRole('button', { name: /^(Create|Selected CSV|Filtered CSV|Clear selection)$/i })).toHaveCount(0)
    await expect(target.locator('[data-part="workflow"]').getByText(/Selected: \d/)).toHaveCount(0)
    await row.getByRole('button', { name: 'View', exact: true }).click()
    await expect(target.locator('dl dd').filter({ hasText: expected.createdAt })).toBeVisible()
    await expect(target.locator('dl dd').filter({ hasText: actionLabels[expected.action] })).toBeVisible()
    assert.equal(new URL(target.url()).pathname, journal + '/records/' + expected.id)
    await expect(target.getByRole('button', { name: /^(Edit|Delete)$/, exact: true })).toHaveCount(0)
    if (viewport === 'mobile') {
      const timestamp = target.getByText(expected.createdAt, { exact: true })
      await timestamp.scrollIntoViewIfNeeded()
      await expect(timestamp).toBeInViewport()
      await target.screenshot({ path: path.join(folder, `${name}-product-event-detail-mobile.png`) })
    }
    await target.getByRole('link', { name: 'Back to list', exact: true }).click()
    await expect(row.getByRole('cell', { name: expected.createdAt, exact: true })).toBeVisible()
    assert.deepEqual(Object.fromEntries(new URL(target.url()).searchParams), conditions)
    await target.screenshot({ path: path.join(folder, `${name}-product-events-${viewport}.png`) })
  }
  await assertRoundTrip(page, 'desktop')
  const mobile = await browser.newContext({
    baseURL: config.url, storageState: await context.storageState(),
    viewport: { width: 390, height: 844 }, timezoneId: 'Asia/Tokyo', locale: 'en-GB',
  })
  try {
    await assertRoundTrip(await mobile.newPage(), 'mobile')
  } finally {
    await mobile.close()
  }

  // Bury the archived reasons beneath more rows than one maximum-size page.
  ownedSQL(config, `INSERT INTO starter_product_events(owner_admin_id, product_id, action, created_at)
    SELECT event.owner_admin_id, event.product_id, 'updated', '2100-01-01 00:00:00+00'
    FROM (SELECT owner_admin_id, product_id FROM starter_product_events
          WHERE product_id='${id}' ORDER BY id DESC LIMIT 1) AS event, generate_series(1, 101)`)
  await page.goto(journal + '?' + new URLSearchParams({ search: id }))
  const filterPanel = page.locator('[data-part="filters"]')
  await filterPanel.getByRole('button', { name: 'Filters', exact: true }).click()
  await filterPanel.getByRole('combobox').click()
  const [filtered] = await Promise.all([
    page.waitForResponse(r => {
      const url = new URL(r.url())
      return url.pathname === journal + '/data' && url.searchParams.get('search') === id &&
        url.searchParams.get('action') === 'unpublish'
    }),
    page.getByRole('option', { name: 'Unpublished', exact: true }).click(),
  ])
  assert.equal(filtered.status(), 200)
  const resultPage = await filtered.json()
  const removals = resultPage.data.map(row => row.item)
  assert.equal(removals.length, 2, 'Validation and revision conflicts must leave no extra journal entry')
  assert.ok(removals.every(row => row.productId === id && row.action === 'unpublish' && !Object.hasOwn(row, 'reason')))
  await expect(page.locator('[data-part="table"] tbody tr')).toHaveCount(2)
  const reasons = []
  for (const removal of removals) {
    await page.goto(journal + '/records/' + removal.id)
    const term = page.locator('dt').filter({ hasText: /^Reason/ })
    await expect(term).toContainText('Reason entered by the operator when unpublishing, if recorded.')
    const text = page.locator('dl > div').filter({ has: term }).locator('dd')
    await expect(text).toBeVisible()
    reasons.push(await text.textContent())
  }
  assert.deepEqual(reasons.sort(), [result.crud.unpublish.cardReason, result.crud.unpublish.listReason].sort())
  result.productEvents = { archivedProduct: id, savedView: viewName, checkedTimezones: ['UTC', 'Asia/Tokyo'], verifiedReasons: reasons.length }
}
