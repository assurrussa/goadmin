import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import path from 'node:path'
import { parseCSV } from './table-workflow.mjs'

// Runs only inside run.mjs's authenticated, isolated starter case. All records
// are created through the public host resource; no fixture SQL or route mocks.
export async function crudAcceptance({ page, context, config, folder, name, expect, language, result }) {
  await language(page, 'English', 'en')
  const prefix = `crud-smoke-${name}-${Date.now()}`
  const report = result.crud = { resource: 'products', pack: name }
  const get = async url => {
    const response = await context.request.get(url)
    assert.equal(response.status(), 200, url)
    return response.json()
  }
  const send = async (url, data, method = 'PUT') => {
    const csrf = (await context.cookies(config.url)).find(cookie => cookie.name === 'csrf_token')?.value
    assert.ok(csrf, 'Native session must expose its CSRF proof')
    return context.request.fetch(url, {
      method, data, maxRedirects: 0,
      headers: {
        Accept: 'application/json', Origin: new URL(config.url).origin,
        Referer: `${config.url}/products`, 'X-CSRF-Token': csrf,
      },
    })
  }
  const field = key => page.locator(`[name="${key}"]`)
  const save = async (pathname, method) => {
    const [response] = await Promise.all([
      page.waitForResponse(response => new URL(response.url()).pathname === pathname && response.request().method() === method),
      page.getByRole('button', { name: 'Save', exact: true }).click(),
    ])
    assert.equal(response.status(), 303, `${method} ${pathname}: must commit and redirect`)
  }
  await page.goto('/products')
  const createButton = page.getByRole('button', { name: 'Create', exact: true }).first()
  await expect(createButton).toBeVisible()
  await createButton.click()
  await expect(page).toHaveURL(/\/products\/create(?:\?.*)?$/)
  await expect(field('name')).toBeVisible()
  await expect(field('description')).toHaveJSProperty('tagName', 'TEXTAREA')
  await expect(field('stock')).toHaveAttribute('type', 'number')
  await expect(field('active')).toHaveAttribute('type', 'checkbox')
  await expect(field('available')).toHaveAttribute('type', 'date')
  await expect(field('category')).toHaveJSProperty('tagName', 'SELECT')
  await field('name').fill(prefix)
  await field('description').fill('First paragraph\nSecond paragraph')
  await field('stock').fill('12')
  await field('active').check()
  await field('available').fill('2026-11-07')
  // An optional select's own blank value must be a valid untouched input.
  await expect(field('category')).toHaveValue('')
  await expect(field('status')).toHaveCount(0)
  await save('/products', 'POST')
  await expect(page).toHaveURL(/\/products\/records\/[^/?]+\/edit(?:\?.*)?$/)
  const editPath = new URL(page.url()).pathname
  const recordPath = editPath.replace(/\/edit$/, '')
  const id = decodeURIComponent(recordPath.split('/').at(-1))
  assert.ok(id)
  await expect(field('name')).toHaveValue(prefix)
  await expect(field('description')).toHaveValue('First paragraph\nSecond paragraph')
  await expect(field('stock')).toHaveValue('12')
  await expect(field('active')).toBeChecked()
  await expect(field('available')).toHaveValue('2026-11-07')
  await expect(field('category')).toHaveValue('')

  const updatedName = `${prefix}-edited`
  await field('name').fill(updatedName)
  await field('description').fill('Edited description\nStill multiline')
  await field('stock').fill('27')
  await field('category').selectOption('premium')
  await field('active').uncheck()
  await field('available').fill('2026-12-01')
  await save(recordPath, 'PUT')
  await expect(page).toHaveURL(new RegExp(`${editPath}$`))
  await expect(field('name')).toHaveValue(updatedName)
  await expect(field('description')).toHaveValue('Edited description\nStill multiline')
  await expect(field('stock')).toHaveValue('27')
  await expect(field('active')).not.toBeChecked()
  await expect(field('available')).toHaveValue('2026-12-01')
  await expect(field('category')).toHaveValue('premium')
  report.form = { id, createRedirectsToEdit: true, saveStaysOnEdit: true, types: ['text', 'textarea', 'number', 'boolean', 'date', 'select'] }

  await page.goto(recordPath)
  const display = label => page.locator('dl > div').filter({ has: page.locator('dt').filter({ hasText: new RegExp(`^${label}$`) }) }).locator('dd')
  await expect(display('Name')).toHaveText(updatedName)
  await expect(display('Description')).toHaveText('Edited description\nStill multiline')
  await expect(display('Stock')).toHaveText('27')
  await expect(display('Active')).toHaveText('No')
  await expect(display('Available')).toHaveText('2026-12-01')
  await expect(display('Status')).toHaveText('Draft')
  await expect(display('Category')).toHaveText('Premium')
  const confirm = async dialog => {
    assert.equal(dialog.type(), 'confirm')
    assert.equal(dialog.message(), 'Publish this product?')
    await dialog.accept()
  }
  page.once('dialog', confirm)
  const [published] = await Promise.all([
    page.waitForResponse(response => new URL(response.url()).pathname === `${recordPath}/actions/publish` && response.request().method() === 'POST'),
    page.getByRole('button', { name: 'Publish', exact: true }).click(),
  ])
  assert.equal(published.status(), 303)
  await expect(page).toHaveURL(new RegExp(`${recordPath}$`))
  await expect(display('Status')).toHaveText('Published')
  await expect(page.getByRole('button', { name: 'Publish', exact: true })).toHaveCount(0)
  await page.getByRole('link', { name: 'Edit', exact: true }).click()
  await expect(page).toHaveURL(new RegExp(`${editPath}(?:\\?.*)?$`))
  await expect(field('category')).toHaveValue('premium')
  report.publish = { confirmed: true, shownOnShow: true, persisted: true, noLongerAvailable: true }

  const list = `/products?search=${encodeURIComponent(prefix)}&sortBy=name&sortOrder=asc&limit=20`
  const snapshot = await get(`/products/data?${list.split('?')[1]}`)
  assert.equal(snapshot.data.length, 1)
  assert.equal(snapshot.data[0].selectionId, id)
  const metadata = snapshot.config.workflow
  assert.ok(metadata?.key, 'Host resource must opt into personal table workflows')
  const schema = snapshot.config.columns
  const columns = ['name', 'stock', 'status']
  const labels = columns.map(key => schema.find(column => column.key === key).label)
  const toolbar = () => page.locator('[data-part="workflow"]')
  const ready = async () => {
    await expect(toolbar().locator('select')).toBeVisible()
    await expect(toolbar().locator('select')).toBeEnabled()
  }
  const savePreferences = async button => {
    const [response] = await Promise.all([
      page.waitForResponse(response => new URL(response.url()).pathname === metadata.preferencesUrl && response.request().method() === 'PUT'),
      button.click(),
    ])
    assert.equal(response.status(), 200)
    return response.json()
  }
  await page.goto(list)
  await ready()
  await toolbar().locator('summary').click()
  const boxes = toolbar().locator('details input[type="checkbox"]')
  for (let index = 0; index < schema.length; index++) if (columns.includes(schema[index].key)) await boxes.nth(index).check()
  for (let index = 0; index < schema.length; index++) if (!columns.includes(schema[index].key)) await boxes.nth(index).uncheck()
  await savePreferences(toolbar().getByRole('button', { name: 'Save columns', exact: true }))
  await toolbar().getByRole('textbox', { name: 'View name', exact: true }).fill(`Product view ${prefix}`)
  await savePreferences(toolbar().getByRole('button', { name: 'Save as new', exact: true }))
  const preferences = await get(metadata.preferencesUrl)
  const view = preferences.payload.views.find(view => view.name === `Product view ${prefix}`)
  assert.ok(view)
  assert.deepEqual(view.state.columns, columns)
  assert.equal(view.state.search, prefix)
  assert.equal(view.state.sortBy, 'name')
  assert.equal(view.state.sortOrder, 'asc')
  await page.reload()
  await ready()
  await toolbar().locator('select').selectOption(view.id)
  await expect(page).toHaveURL(new RegExp(`search=${encodeURIComponent(prefix)}`))
  const headers = await page.locator('[data-part="table"] thead th').allTextContents()
  assert.deepEqual(headers.map(label => label.trim()).filter(label => label && label !== 'Actions'), labels)
  await page.getByRole('checkbox', { name: `Select row ${id}`, exact: true }).check()
  await expect(toolbar().getByRole('status').filter({ hasText: /^Selected:/ })).toHaveText('Selected: 1 of 100')
  const downloadCSV = async label => {
    const [download] = await Promise.all([
      page.waitForEvent('download'), toolbar().getByRole('button', { name: label, exact: true }).click(),
    ])
    assert.equal(await download.failure(), null)
    return parseCSV(await readFile(await download.path(), 'utf8'))
  }
  const selected = await downloadCSV('Selected CSV')
  const filtered = await downloadCSV('Filtered CSV')
  assert.deepEqual(selected[0], labels)
  assert.deepEqual(selected[1], [updatedName, '27', 'published'])
  assert.equal(selected.length, 2)
  assert.deepEqual(filtered, selected)
  report.workflow = { key: metadata.key, viewId: view.id, columns, selectedCSVRows: 1, filteredCSVRows: 1 }
  if (folder) await page.screenshot({ path: path.join(folder, `${name}-crud-workflow.png`), fullPage: true })

  // Two independent writers share a known revision. Exactly one may commit;
  // the loser must receive an explicit conflict and leave the winner intact.
  const before = (await get(`/products/data?search=${encodeURIComponent(prefix)}`)).data[0].item
  const values = { name: updatedName, description: 'Edited description\nStill multiline', stock: 27, active: false, available: '2026-12-01', category: 'premium' }
  const writes = [
    { ...values, name: `${prefix}-winner-a`, _revision: before._revision },
    { ...values, name: `${prefix}-winner-b`, _revision: before._revision },
  ]
  const raced = await Promise.all(writes.map(data => send(recordPath, data)))
  assert.deepEqual(raced.map(response => response.status()).sort(), [303, 409])
  const winnerIndex = raced.findIndex(response => response.status() === 303)
  const winner = (await get(`/products/data?search=${encodeURIComponent(prefix)}`)).data[0].item
  assert.equal(winner.name, writes[winnerIndex].name)
  assert.notEqual(winner._revision, before._revision)
  const stale = await send(recordPath, { ...values, name: `${prefix}-stale-must-not-save`, _revision: before._revision })
  assert.equal(stale.status(), 409)
  const conflict = await stale.json()
  assert.match(conflict.message, /changed|reload/i)
  const unchanged = async () => {
    const rows = (await get(`/products/data?search=${encodeURIComponent(prefix)}`)).data
    assert.equal(rows.length, 1)
    assert.deepEqual(rows[0].item, winner, 'Rejected commands must not change fields or revision')
  }
  await unchanged()
  for (const key of ['id', 'owner_admin_id', 'revision', '_actions', 'status']) {
    const forged = await send(recordPath, { ...values, _revision: winner._revision, [key]: 'forged' })
    assert.equal(forged.status(), 422, `Forged read-only key ${key}`)
    const body = await forged.json()
    assert.match(JSON.stringify(body.errors), /read-only|unexpected/i)
    await unchanged()
  }
  report.concurrency = { raceStatuses: [303, 409], staleStatus: 409, winnerPreserved: true }
  const forgedCreate = await send('/products', { ...values, name: `${prefix}-forged-create`, status: 'published' }, 'POST')
  assert.equal(forgedCreate.status(), 422, 'Publication status cannot bypass the dedicated command via Create')
  await unchanged()
  report.inputBoundary = { rejectedKeys: ['id', 'owner_admin_id', 'revision', '_actions', 'status'], status: 422, noOverwrite: true }

  // Enter through the actual row navigation so its list query remains attached
  // to Show, Edit, successful saves and the explicit conflict reload.
  await page.goto(list)
  await ready()
  const listURL = new URL(page.url())
  const returnTo = listURL.pathname + listURL.search
  await page.locator('[data-part="table"] tbody').getByRole('button', { name: 'View', exact: true }).click()
  await expect(page).toHaveURL(new RegExp(`${recordPath}\\?`))
  assert.equal(new URL(page.url()).searchParams.get('returnTo'), returnTo)
  await page.getByRole('link', { name: 'Edit', exact: true }).click()
  await expect(field('name')).toHaveValue(winner.name)
  assert.equal(new URL(page.url()).searchParams.get('returnTo'), returnTo)
  await field('stock').fill('28')
  await save(recordPath, 'PUT')
  await expect(field('stock')).toHaveValue('28')
  assert.equal(new URL(page.url()).pathname, editPath)
  assert.equal(new URL(page.url()).searchParams.get('returnTo'), returnTo)
  await expect(page.getByRole('link', { name: 'Back to list', exact: true })).toHaveAttribute('href', returnTo)
  report.returnNavigation = { list: returnTo, show: true, edit: true, successfulSave: true }

  const pinned = (await get(`/products/data?search=${encodeURIComponent(prefix)}`)).data[0].item
  const localDraft = `${prefix}-local-unsaved`
  await field('name').fill(localDraft)
  const canonicalValues = { ...values, name: `${prefix}-canonical-new`, stock: 29 }
  const externalWrite = await send(recordPath, { ...canonicalValues, _revision: pinned._revision })
  assert.equal(externalWrite.status(), 303)
  const canonical = (await get(`/products/data?search=${encodeURIComponent(prefix)}`)).data[0].item
  assert.notEqual(canonical._revision, pinned._revision)
  const rejectedRevisions = []
  for (let attempt = 0; attempt < 2; attempt++) {
    const [response] = await Promise.all([
      page.waitForResponse(response => new URL(response.url()).pathname === recordPath && response.request().method() === 'PUT'),
      page.getByRole('button', { name: 'Save', exact: true }).click(),
    ])
    assert.equal(response.status(), 303, 'Inertia errors must use the ordinary error redirect')
    const submitted = response.request().postDataJSON()
    rejectedRevisions.push(submitted._revision)
    assert.equal(submitted._revision, pinned._revision, 'A rejected draft cannot adopt a fresh revision implicitly')
    assert.equal(submitted.name, localDraft)
    await expect(page.getByRole('alert')).toContainText('Record changed; reload before saving')
    await expect(field('name')).toHaveValue(localDraft)
    await expect(field('stock')).toHaveValue('28')
    await expect(page.getByRole('button', { name: 'Save', exact: true })).toBeEnabled()
    const current = (await get(`/products/data?search=${encodeURIComponent(prefix)}`)).data[0].item
    assert.deepEqual(current, canonical, 'Repeated rejected UI saves must leave the canonical record intact')
  }
  let reloadConfirmed = false
  page.once('dialog', async dialog => {
    assert.equal(dialog.type(), 'confirm')
    assert.match(dialog.message(), /unsaved|несохран/i)
    reloadConfirmed = true
    await dialog.accept()
  })
  await page.getByRole('button', { name: 'Reload current record', exact: true }).click()
  await expect(field('name')).toHaveValue(canonicalValues.name)
  await expect(field('stock')).toHaveValue('29')
  await expect(page.getByRole('alert')).toHaveCount(0)
  assert.equal(reloadConfirmed, true)
  assert.equal(new URL(page.url()).searchParams.get('returnTo'), returnTo)
  report.uiConflict = { rejectedRevisions, canonicalRevision: canonical._revision, retainedDraft: true, explicitReloadConfirmed: true }

  await language(page, 'Русский', 'ru')
  await page.goto(`${recordPath}?${new URLSearchParams({ returnTo })}`)
  await page.setViewportSize({ width: 390, height: 844 })
  await expect(display('Name')).toHaveText(canonicalValues.name)
  assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), 'Russian mobile Show must fit the viewport')
  if (folder) await page.screenshot({ path: path.join(folder, `${name}-crud-show-mobile-ru.png`), fullPage: true })
  await page.getByRole('link', { name: 'Редактировать', exact: true }).click()
  await expect(field('name')).toHaveValue(canonicalValues.name)
  await expect(page.getByRole('button', { name: 'Сохранить', exact: true })).toBeVisible()
  assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), 'Russian mobile Edit must fit the viewport')
  const booleanTarget = await field('active').evaluate(input => input.closest('label').getBoundingClientRect().height)
  assert.ok(booleanTarget >= 44, 'Mobile boolean field must have a 44px clickable label')
  if (folder) await page.screenshot({ path: path.join(folder, `${name}-crud-edit-mobile-ru.png`), fullPage: true })
  await page.setViewportSize({ width: 1440, height: 1000 })
  await language(page, 'English', 'en')
  report.mobile = { locale: 'ru', viewport: '390x844', show: true, edit: true, noHorizontalOverflow: true }

  await page.goto(`${recordPath}?${new URLSearchParams({ returnTo })}`)
  let deleteConfirmed = false
  page.once('dialog', async dialog => {
    assert.equal(dialog.type(), 'confirm')
    assert.match(dialog.message(), /delete|удал/i)
    deleteConfirmed = true
    await dialog.accept()
  })
  const [deleted] = await Promise.all([
    page.waitForResponse(response => new URL(response.url()).pathname === recordPath && response.request().method() === 'DELETE'),
    page.getByRole('button', { name: 'Delete', exact: true }).click(),
  ])
  assert.equal(deleted.status(), 303)
  await expect(page).toHaveURL(/\/products(?:\?.*)?$/)
  assert.equal(deleteConfirmed, true)
  const actualQuery = new URL(page.url()).searchParams
  const expectedQuery = new URL(returnTo, config.url).searchParams
  actualQuery.sort()
  expectedQuery.sort()
  assert.deepEqual([...actualQuery], [...expectedQuery], 'Delete restores every originating list condition')
  const remaining = await get(`/products/data?search=${encodeURIComponent(prefix)}`)
  assert.equal(remaining.data.length, 0)
  report.delete = { confirmed: true, returnedToList: true, remaining: 0 }

}
