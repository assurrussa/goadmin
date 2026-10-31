import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import path from 'node:path'
import { tableExportMatrix, tableSecurityMatrix } from './table-contracts.mjs'

// Fixtures belong only to the run-owned PostgreSQL, via run.mjs's isolation-checked executor.
// They have canonical subjects/identifiers but no credentials or admin membership.
export function tableFixtureSQL(pack) {
  assert.match(pack, /^[a-z][a-z0-9-]*$/)
  const prefix = `table-workflow-${pack}-`
  return `WITH fixtures AS (
    SELECT gen_random_uuid() AS subject_id, '${prefix}' || lpad(n::text,3,'0') AS fixture_name
    FROM generate_series(1,105) AS n
  ), subjects AS (
    INSERT INTO auth_subjects(id,status,security_version,created_at,updated_at)
    SELECT subject_id,'active',1,now(),now() FROM fixtures RETURNING id
  ), identifiers AS (
    INSERT INTO auth_identifiers(id,subject_id,scheme,display_value,normalized_value,is_primary,created_at,updated_at)
    SELECT gen_random_uuid(),f.subject_id,'email',f.fixture_name || '@example.test',f.fixture_name || '@example.test',true,now(),now()
    FROM fixtures f JOIN subjects s ON s.id=f.subject_id
  ), profiles AS (
    INSERT INTO auth_basic_profiles(subject_id,username,display_name,given_name,family_name,created_at,updated_at)
    SELECT f.subject_id,f.fixture_name,f.fixture_name,f.fixture_name,'Fixture',now(),now()
    FROM fixtures f JOIN subjects s ON s.id=f.subject_id
  ) INSERT INTO users(subject_id,uuid,created_at,updated_at)
    SELECT id,gen_random_uuid(),now(),now() FROM subjects;
  INSERT INTO jobs(id,queue,name,payload,attempts,available_at,created_at)
    VALUES(gen_random_uuid(),'starter-smoke','${prefix}job','{"token":"smoke-secret-token"}',0,now()+interval '1 day',now());
  INSERT INTO jobs_failed(id,job_id,connection,queue,name,payload,reason,exception)
    SELECT gen_random_uuid(),gen_random_uuid(),'pgsql','starter-smoke','${prefix}failed','{"token":"smoke-secret-token"}','Isolated smoke fixture','Isolated smoke fixture'
    FROM generate_series(1,2) AS n;`
}

export function parseCSV(source) {
  const rows = []
  let row = [], value = '', quoted = false
  const text = source.replace(/^\uFEFF/, '')
  for (let index = 0; index < text.length; index++) {
    const char = text[index]
    if (char === '"') {
      if (quoted && text[index + 1] === '"') { value += '"'; index++ }
      else quoted = !quoted
    } else if (char === ',' && !quoted) { row.push(value); value = '' }
    else if (char === '\r' && !quoted && text[index + 1] === '\n') continue
    else if (char === '\n' && !quoted) {
      row.push(value); rows.push(row); row = []; value = ''
    } else value += char
  }
  assert.equal(quoted, false, 'unterminated CSV field')
  if (value || row.length) { row.push(value); rows.push(row) }
  return rows
}

export async function tableWorkflowAcceptance({ page, context, config, credentials, ownedSQL, folder, name, expect, language, login, logout, result }) {
  await language(page, 'English', 'en')
  const prefix = `table-workflow-${name}-`
  ownedSQL(config, tableFixtureSQL(name))
  const report = result.tableWorkflow = { fixtures: { users: 105, activeJobs: 1, failedJobs: 2 }, metadata: {}, bulk: {} }
  const get = async (url) => {
    const response = await context.request.get(url)
    assert.ok(response.ok(), `${url}: ${response.status()}`)
    return response.json()
  }
  for (const resource of ['/users', '/admins', '/roles', '/permissions', '/queues/jobs', '/queues/jobs-failed']) {
    const snapshot = await get(`${resource}/data?limit=1`)
    assert.ok(snapshot.config.workflow?.key, `${resource} must opt in`)
    const prefs = await get(snapshot.config.workflow.preferencesUrl)
    assert.equal(prefs.payload.schemaVersion, 1)
    report.metadata[resource] = { key: snapshot.config.workflow.key, owner: prefs.owner }
  }
  assert.equal(new Set(Object.values(report.metadata).map(value => value.key)).size, 6)
  assert.equal(new Set(Object.values(report.metadata).map(value => value.owner)).size, 1)
  report.csvMatrix = await tableExportMatrix({ context, get, prefix, parseCSV })
  report.security = await tableSecurityMatrix({ page, context, config, credentials, get, prefix, login, logout })
  const list = `/users?limit=25&sortBy=id&sortOrder=asc&status=active&search=${prefix}`
  const snapshot = await get(`/users/data?${list.split('?')[1]}`)
  const metadata = snapshot.config.workflow
  const schema = snapshot.config.columns
  const ui = () => page.locator('[data-part="workflow"]')
  const ready = async (target = page) => {
    const toolbar = target.locator('[data-part="workflow"]')
    // Preference readiness is exposed by the visible view selector. Column controls
    // are intentionally inside a closed details element after each navigation.
    await expect(toolbar.locator('select')).toBeVisible()
    await expect(toolbar.locator('select')).toBeEnabled()
    return toolbar
  }
  const openColumns = async (toolbar = ui()) => {
    if (!await toolbar.locator('details').getAttribute('open').then(value => value !== null)) await toolbar.locator('summary').click()
  }
  const setColumns = async (keys, toolbar = ui()) => {
    await openColumns(toolbar)
    const boxes = toolbar.locator('details input[type="checkbox"]')
    for (let index = 0; index < schema.length; index++) if (keys.includes(schema[index].key)) await boxes.nth(index).check()
    for (let index = 0; index < schema.length; index++) if (!keys.includes(schema[index].key)) await boxes.nth(index).uncheck()
  }
  const save = async (button, target = page, status = 200) => {
    const [response] = await Promise.all([
      target.waitForResponse(response => new URL(response.url()).pathname === metadata.preferencesUrl && response.request().method() === 'PUT'),
      button.click(),
    ])
    assert.equal(response.status(), status)
    return response
  }
  await page.goto(list)
  await ready()
  await setColumns(['id', 'email', 'name'])
  await ui().locator('ol li').nth(2).getByRole('button', { name: 'Move up', exact: true }).click()
  await ui().locator('ol li').nth(1).getByRole('button', { name: 'Move up', exact: true }).focus()
  await page.keyboard.press('Enter')
  const visibleColumns = ['name', 'id', 'email']
  await save(ui().getByRole('button', { name: 'Save columns', exact: true }))
  await page.reload()
  await ready()
  assert.deepEqual((await get(metadata.preferencesUrl)).payload.columns, visibleColumns)
  const headers = () => page.locator('[data-part="table"] thead th').allTextContents().then(values => values.map(value => value.trim()).filter(value => value && value !== 'Actions'))
  assert.deepEqual(await headers(), ['Name', 'ID', 'Email'])
  await ui().getByRole('textbox', { name: 'View name', exact: true }).fill(`Smoke view ${name}`)
  await save(ui().getByRole('button', { name: 'Save as new', exact: true }))
  let saved = await get(metadata.preferencesUrl)
  const view = saved.payload.views.find(view => view.name === `Smoke view ${name}`)
  assert.ok(view)
  assert.deepEqual(view.state.columns, visibleColumns)
  assert.equal(view.state.search, prefix)
  assert.equal(view.state.filters.status, 'active')
  assert.equal(view.state.sortBy, 'id')
  assert.equal(view.state.sortOrder, 'asc')
  assert.equal(view.state.limit, 25)
  assert.equal('page' in view.state, false)
  assert.equal('selection' in view.state, false)
  await save(ui().getByRole('button', { name: 'Default', exact: true }))
  await logout(page)
  await login(page, credentials.email, credentials.password)
  await page.goto('/users')
  await ready()
  await expect(page).toHaveURL(new RegExp(`search=${prefix}`))
  assert.deepEqual(await headers(), ['Name', 'ID', 'Email'])
  await expect(ui().locator('select')).toHaveValue(view.id)
  await expect(ui().locator('[data-part="view-dirty"]')).toHaveCount(0)
  // Named views require explicit updates, including rename. Exercise the entire CRUD loop.
  await ui().getByRole('textbox', { name: 'View name', exact: true }).fill(`Renamed ${name}`)
  await expect(ui().locator('[data-part="view-dirty"]')).toBeVisible()
  assert.equal((await get(metadata.preferencesUrl)).payload.views[0].name, `Smoke view ${name}`)
  await save(ui().getByRole('button', { name: 'Save / rename', exact: true }))
  await expect(ui().locator('[data-part="view-dirty"]')).toHaveCount(0)
  assert.equal((await get(metadata.preferencesUrl)).payload.views[0].name, `Renamed ${name}`)
  await ui().getByRole('textbox', { name: 'View name', exact: true }).fill(`Disposable ${name}`)
  await save(ui().getByRole('button', { name: 'Save as new', exact: true }))
  assert.equal((await get(metadata.preferencesUrl)).payload.views.length, 2)
  await save(ui().getByRole('button', { name: 'Delete view', exact: true }))
  assert.equal((await get(metadata.preferencesUrl)).payload.views.length, 1)
  await page.goto('/users?search=definitely-unmatched&limit=1')
  await ready()
  await expect(page).toHaveURL(/search=definitely-unmatched/)
  await ui().locator('select').selectOption(view.id)
  await expect(page).toHaveURL(new RegExp(`search=${prefix}`))
  await expect(page.getByRole('checkbox', { name: 'Select all rows', exact: true })).toBeVisible()
  report.personalView = { columns: visibleColumns, fullQuery: view.state, defaultBareURL: true, explicitURLWins: true, keyboardReorder: true, reloginRestored: true, explicitUpdateRenameDelete: true }

  const selectedIds = []
  const selectedCount = count => expect(ui().getByRole('status').filter({ hasText: /^Selected:/ })).toHaveText(`Selected: ${count} of 100`)
  for (let current = 1; current <= 4; current++) {
    if (current > 1) {
      const next = page.waitForResponse(response => new URL(response.url()).pathname === '/users/data' && new URL(response.url()).searchParams.get('page') === String(current))
      await page.locator('[data-part="pagination"]').getByRole('button', { name: String(current), exact: true }).click()
      assert.ok((await next).ok())
    }
    const data = await get(`/users/data?${list.split('?')[1]}&page=${current}`)
    selectedIds.push(...data.data.map(row => row.selectionId))
    await page.getByRole('checkbox', { name: 'Select all rows', exact: true }).check()
    await selectedCount(current * 25)
  }
  await page.getByRole('checkbox', { name: 'Select all rows', exact: true }).uncheck()
  await selectedCount(75)
  await page.getByRole('checkbox', { name: 'Select all rows', exact: true }).check()
  await selectedCount(100)
  const fifth = page.waitForResponse(response => new URL(response.url()).pathname === '/users/data' && new URL(response.url()).searchParams.get('page') === '5')
  await page.locator('[data-part="pagination"]').getByRole('button', { name: '5', exact: true }).click()
  assert.ok((await fifth).ok())
  await selectedCount(100)
  const lastRows = page.locator('[data-part="table"] tbody [role="checkbox"]')
  await expect(lastRows).toHaveCount(5)
  for (const checkbox of await lastRows.all()) await expect(checkbox).toBeDisabled()
  const downloadCSV = async (label) => {
    const [download] = await Promise.all([page.waitForEvent('download'), ui().getByRole('button', { name: label, exact: true }).click()])
    assert.equal(await download.failure(), null)
    return parseCSV(await readFile(await download.path(), 'utf8'))
  }
  const csv = await downloadCSV('Selected CSV')
  assert.deepEqual(csv[0], visibleColumns.map(key => schema.find(column => column.key === key).label))
  assert.equal(csv.length, 101)
  assert.deepEqual(new Set(csv.slice(1).map(row => row[1])), new Set(selectedIds))
  const filteredCSV = await downloadCSV('Filtered CSV')
  assert.equal(filteredCSV.length, 106)
  assert.deepEqual(filteredCSV[0], csv[0])
  report.selection = { acrossPages: 100, cap: 100, currentPageUnselectOnly: true, selectedCSVRows: csv.length - 1, filteredCSVRows: filteredCSV.length - 1 }
  await page.screenshot({ path: path.join(folder, `${name}-table-workflow-light-en.png`), fullPage: true })
  await page.getByRole('button', { name: /Switch to dark theme|Переключить на темную тему/i }).click()
  await language(page, 'Русский', 'ru')
  await page.setViewportSize({ width: 390, height: 844 })
  await expect(ui()).toContainText('Выбрано: 100 из 100')
  assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), 'workflow mobile fit')
  await page.screenshot({ path: path.join(folder, `${name}-table-workflow-dark-mobile-ru.png`), fullPage: true })
  await page.setViewportSize({ width: 1440, height: 1000 })
  await language(page, 'English', 'en')
  await page.getByRole('button', { name: /Switch to light theme|Переключить на светлую тему/i }).click()
  const [searchResponse] = await Promise.all([
    page.waitForResponse(response => new URL(response.url()).pathname === '/users/data' && new URL(response.url()).searchParams.get('search') === 'no-match'),
    page.getByPlaceholder('Search...', { exact: true }).fill('no-match'),
  ])
  assert.ok(searchResponse.ok())
  await selectedCount(0)

  // A real second editor produces the server's compare-and-swap conflict. No route override.
  await page.goto(list)
  await ready()
  const second = await context.newPage()
  const conflictConsole = []
  second.on('pageerror', error => { result.pageErrors.push(error.message) })
  second.on('console', message => { if (message.type() === 'error') conflictConsole.push(message.text()) })
  try {
    await second.goto(list)
    const secondUI = await ready(second)
    await setColumns(['name', 'id'])
    await save(ui().getByRole('button', { name: 'Save columns', exact: true }))
    await setColumns(['name', 'email'], secondUI)
    await save(secondUI.getByRole('button', { name: 'Save columns', exact: true }), second, 409)
    await expect(secondUI.getByRole('alert')).toContainText('Your edits remain in the editor')
    await expect(secondUI.locator('details input[type="checkbox"]').nth(schema.findIndex(column => column.key === 'id'))).not.toBeChecked()
    assert.deepEqual((await get(metadata.preferencesUrl)).payload.columns, ['name', 'id'])
    assert.ok(conflictConsole.every(message => /409/.test(message)), `Unexpected conflict-tab console errors: ${conflictConsole.join('\n')}`)
    report.preferencesConflict = { status: 409, retainedDraft: ['name', 'email'], serverColumns: ['name', 'id'], expectedConsole: conflictConsole }
  } finally { await second.close() }
  await setColumns(visibleColumns)
  await save(ui().getByRole('button', { name: 'Save columns', exact: true }))

  const runBulk = async (endpoint, actionLabel, expectedCount) => {
    await ui().getByRole('button', { name: actionLabel, exact: true }).click()
    const dialog = page.getByRole('dialog')
    await expect(dialog).toContainText(`${expectedCount} selected records across all pages`)
    assert.ok(await dialog.evaluate(node => node.contains(document.activeElement)), 'bulk dialog contains focus')
    const [response] = await Promise.all([
      page.waitForResponse(response => new URL(response.url()).pathname === endpoint && response.request().method() === 'POST'),
      dialog.getByRole('button', { name: 'Confirm', exact: true }).click(),
    ])
    assert.equal(response.status(), 200)
    const body = await response.json()
    await expect(ui().getByRole('region', { name: 'Operation results', exact: true })).toBeVisible()
    if (endpoint.startsWith('/queues/')) {
      const actual = await get('/queues/stats')
      await expect(page.locator('[data-queue-counter="jobs"]')).toHaveText(String(actual.jobs))
      await expect(page.locator('[data-queue-counter="failed"]')).toHaveText(String(actual.failed))
    }
    return body.results
  }
  const fixtureID = snapshot.data[0].selectionId
  const ownerRows = await get(`/users/data?search=${encodeURIComponent(credentials.email)}&limit=10`)
  const ownerID = ownerRows.data.find(row => row.item.email === credentials.email)?.selectionId
  assert.ok(ownerID, 'active current administrator must have its existing user projection')
  await page.goto('/users?limit=100&sortBy=id&sortOrder=asc')
  await ready()
  await page.getByRole('checkbox', { name: `Select row ${fixtureID}`, exact: true }).check()
  await page.getByRole('checkbox', { name: `Select row ${ownerID}`, exact: true }).check()
  const deleted = await runBulk(metadata.bulkUrl, 'Delete', 2)
  assert.equal(deleted.find(row => row.id === fixtureID)?.status, 'succeeded')
  assert.equal(deleted.find(row => row.id === ownerID)?.status, 'skipped')
  await expect(page.locator('[data-profile-menu-trigger]')).toBeVisible()
  await page.goto(`/users?status=deleted&search=${prefix}&limit=25`)
  await ready()
  await page.getByRole('checkbox', { name: `Select row ${fixtureID}`, exact: true }).check()
  const restored = await runBulk(metadata.bulkUrl, 'Restore', 1)
  assert.equal(restored.find(row => row.id === fixtureID)?.status, 'succeeded')
  const active = await get(`/users/data?status=active&search=${prefix}&limit=25&sortBy=id&sortOrder=asc`)
  assert.ok(active.data.some(row => row.selectionId === fixtureID))
  report.bulk.users = { deleted, restored, activeAdministratorSkipped: true }

  const jobs = await get(`/queues/jobs/data?name=${prefix}job`)
  assert.equal(jobs.data.length, 1, 'actual active fixture must exist')
  await page.goto(`/queues/jobs?name=${prefix}job`)
  await ready()
  await page.getByRole('checkbox', { name: `Select row ${jobs.data[0].selectionId}`, exact: true }).check()
  const jobDeleted = await runBulk(jobs.config.workflow.bulkUrl, 'Delete', 1)
  assert.equal(jobDeleted[0].status, 'succeeded')
  assert.equal((await get(`/queues/jobs/data?name=${prefix}job`)).data.length, 0)
  const failed = await get(`/queues/jobs-failed/data?name=${prefix}failed`)
  assert.equal(failed.data.length, 2, 'actual DLQ fixtures must exist')
  await page.goto(`/queues/jobs-failed?name=${prefix}failed`)
  await ready()
  await page.getByRole('checkbox', { name: `Select row ${failed.data[0].selectionId}`, exact: true }).check()
  const retried = await runBulk(failed.config.workflow.bulkUrl, 'Retry', 1)
  assert.equal(retried[0].status, 'succeeded')
  assert.ok(retried[0].relatedId, 'retry must identify the actual new active job')
  await page.getByRole('checkbox', { name: `Select row ${failed.data[1].selectionId}`, exact: true }).check()
  const failedDeleted = await runBulk(failed.config.workflow.bulkUrl, 'Delete', 1)
  assert.equal(failedDeleted[0].status, 'succeeded')
  assert.equal((await get(`/queues/jobs-failed/data?name=${prefix}failed`)).data.length, 0)
  const retriedJobs = await get(`/queues/jobs/data?name=${prefix}failed`)
  assert.ok(retriedJobs.data.some(row => row.selectionId === retried[0].relatedId), 'retry must persist new active job')
  report.bulk.queues = { activeDelete: jobDeleted, failedRetry: retried, failedDelete: failedDeleted }
  // Remove the intentionally unregistered retry fixture through the same real bulk command.
  await page.goto(`/queues/jobs?name=${prefix}failed`)
  await ready()
  await page.getByRole('checkbox', { name: `Select row ${retried[0].relatedId}`, exact: true }).check()
  await runBulk(jobs.config.workflow.bulkUrl, 'Delete', 1)
  saved = await get(metadata.preferencesUrl)
  assert.equal(saved.payload.views.length, 1, 'unrelated workflows must preserve personal views')
}
