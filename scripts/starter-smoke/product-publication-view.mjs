import assert from 'node:assert/strict'

const products = '/products'
const tableRows = page => page.locator('[data-part="table"] tbody tr')

function commandResponse(page, pathname) {
  return page.waitForResponse(response => response.request().method() === 'POST' && new URL(response.url()).pathname === pathname)
}

function detailValue(page, label) {
  return page.locator('dl > div').filter({ has: page.locator('dt').filter({ hasText: label }) }).locator('dd')
}

export async function productPublicationViewAcceptance({ page, context, config, name, expect, result, ownedSQL }) {
  assert.match(name, /^[a-z][a-z0-9-]*$/)
  const cohort = `publication-view-${name}`
  const baseline = `${cohort} Baseline`
  const candidate = `${cohort} Candidate`
  const standard = `${cohort} Standard`
  // Each layout has its own run-owned PostgreSQL database. Only the fixture
  // setup uses SQL. The browser performs both publication commands.
  ownedSQL(config, `INSERT INTO starter_products(owner_admin_id,name,description,stock,active,available,category,status)
    SELECT a.id, p.name, '', 1, true, '', p.category, p.status
    FROM (SELECT min(id) AS id FROM administrations) a
    CROSS JOIN (VALUES ('${baseline}', 'premium', 'published'), ('${candidate}', 'premium', 'draft'),
      ('${standard}', 'standard', 'published')) p(name,category,status)`)

  const initialURL = products + '?' + new URLSearchParams({ search: cohort })
  const rows = tableRows(page)
  const rowFor = product => rows.filter({ has: page.getByRole('cell', { name: product, exact: true }) })
  const assertMembership = async (...names) => {
    await expect(rows).toHaveCount(names.length)
    for (const product of names) await expect(rowFor(product)).toBeVisible()
  }
  const conditions = { search: cohort, status: 'published', category: 'premium' }
  const assertFilters = async () => {
    await expect.poll(() => {
      const query = new URL(page.url()).searchParams
      return Object.fromEntries(Object.keys(conditions).map(key => [key, query.get(key)]))
    }).toEqual(conditions)
  }

  await page.goto(initialURL)
  await assertMembership(baseline, candidate, standard)
  const candidateLink = await rowFor(candidate).getByRole('link', { name: 'View', exact: true }).getAttribute('href')
  assert.ok(candidateLink)
  const recordPath = new URL(candidateLink, page.url()).pathname
  const productId = recordPath.split('/').at(-1)
  assert.match(productId, /^[1-9]\d*$/)

  const filters = page.locator('[data-part="filters"]')
  await filters.getByRole('button', { name: 'Filters', exact: true }).click()
  for (const [filter, value] of [['Status', 'Published'], ['Category', 'Premium']]) {
    await filters.getByRole('combobox', { name: filter, exact: true }).click()
    await page.getByRole('option', { name: value, exact: true }).click()
  }
  await assertFilters()
  await assertMembership(baseline)

  const workflow = page.locator('[data-part="workflow"]')
  const viewName = `Published Premium ${name}`
  await workflow.getByRole('textbox', { name: 'View name', exact: true }).fill(viewName)
  const [saved] = await Promise.all([
    page.waitForResponse(r => new URL(r.url()).pathname === products + '/table-preferences' && r.request().method() === 'PUT'),
    workflow.getByRole('button', { name: 'Save as new', exact: true }).click(),
  ])
  assert.equal(saved.status(), 200)
  await page.goto(initialURL)
  await assertMembership(baseline, candidate, standard)
  await workflow.locator('select').selectOption({ label: viewName })
  await assertFilters()
  await assertMembership(baseline)

  const colleague = await context.newPage()
  try {
    await colleague.goto(candidateLink)
    await expect(detailValue(colleague, /^Status/)).toHaveText('Draft')
    await colleague.getByRole('button', { name: 'Publish', exact: true }).click()
    const [published] = await Promise.all([
      commandResponse(colleague, recordPath + '/actions/publish'),
      colleague.locator('[data-confirmation-accept]:visible').click(),
    ])
    assert.equal(published.status(), 303)
    await expect(detailValue(colleague, /^Status/)).toHaveText('Published')
  } finally {
    await colleague.close()
  }

  const list = new URL(page.url())
  const returnTo = list.pathname + list.search
  const [refreshed] = await Promise.all([
    page.waitForResponse(r => new URL(r.url()).pathname === products + '/data' && r.request().method() === 'GET'),
    page.locator('[data-part="header"]').getByRole('button', { name: 'Refresh', exact: true }).click(),
  ])
  assert.equal(refreshed.status(), 200)
  await expect(page).toHaveURL(list.href)
  await assertMembership(baseline, candidate)

  await rowFor(candidate).getByRole('button', { name: 'More', exact: true }).click()
  await page.getByRole('menuitem', { name: 'Unpublish', exact: true }).click()
  const dialog = page.locator('[data-crud-action-dialog]')
  const explanation = 'Review the Premium listing.\nKeep  the current price 🙂.'
  await dialog.locator('textarea[name="reason"]').fill(explanation)
  const [unpublished] = await Promise.all([
    commandResponse(page, recordPath + '/actions/unpublish'),
    dialog.getByRole('button', { name: 'Unpublish', exact: true }).click(),
  ])
  assert.equal(unpublished.status(), 204)
  await expect.poll(() => new URL(page.url()).pathname).toBe(recordPath)
  await expect(detailValue(page, /^Status/)).toHaveText('Draft')

  const back = page.getByRole('link', { name: 'Back to list', exact: true })
  await expect(back).toHaveAttribute('href', returnTo)
  await back.click()
  await expect(page).toHaveURL(list.href)
  await assertFilters()
  await assertMembership(baseline)

  await page.goto('/product-events?' + new URLSearchParams({ search: productId, action: 'unpublish' }))
  const event = tableRows(page)
  await expect(event).toHaveCount(1)
  await expect(event.getByRole('cell', { name: 'Unpublished', exact: true })).toBeVisible()
  await event.getByRole('link', { name: 'View', exact: true }).click()
  const reason = detailValue(page, /^Reason/)
  await expect(reason).toBeVisible()
  assert.equal(await reason.textContent(), explanation, 'Activity detail must preserve the exact reason')
  result.productPublicationView = { view: viewName, filters: conditions, productId, counts: [1, 2, 1], reason: explanation }
}
