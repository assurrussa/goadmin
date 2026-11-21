import assert from 'node:assert/strict'
import path from 'node:path'

export const permissionCheckbox = (page, key) =>
  page.locator('label').filter({ has: page.getByText(key, { exact: true }) }).getByRole('checkbox')

export async function roleEditorAcceptance({ page, context, config, expect, folder, name, result, expectConflict }) {
  const prefix = `smoke-role-edit-${name}`
  async function legacyPost(uri, data, referer = '/roles/create') {
    const token = (await context.cookies(config.url)).find(cookie => cookie.name === 'csrf_token')?.value
    assert.ok(token)
    const response = await context.request.post(uri, {
      data,
      maxRedirects: 0,
      headers: { Origin: config.url, Referer: config.url + referer, 'X-CSRF-Token': token },
    })
    assert.ok([302, 303].includes(response.status()), `${uri}: ${response.status()}`)
  }
  for (const suffix of ['a', 'b', 'c']) {
    await legacyPost('/roles', {
      slug: `${prefix}-${suffix}`, name: `Role fixture ${suffix}`, permissions: ['roles:read'],
    })
  }

  await page.goto(`/roles?page=2&limit=1&sortBy=slug&sortOrder=asc&search=${prefix}&isSystem=regular`)
  const row = page.getByRole('row').filter({ hasText: `${prefix}-b` })
  await expect(row).toBeVisible()
  const listURL = page.url()
  await row.getByRole('button', { name: 'View', exact: true }).click()
  await expect(page).toHaveURL(/\/roles\/\d+\?returnTo=/)
  await page.getByRole('link', { name: 'Back to list', exact: true }).click()
  await expect(page).toHaveURL(listURL)
  await row.getByRole('button', { name: 'Edit', exact: true }).click()
  await expect(page).toHaveURL(/\/roles\/\d+\/edit\?returnTo=/)
  const editURL = page.url()
  assert.equal(new URL(editURL).searchParams.get('returnTo'), listURL.slice(config.url.length))
  const mutationPath = new URL(editURL).pathname.replace(/\/edit$/, '')
  function savedBy(tab) {
    return tab.waitForResponse(response => response.request().method() === 'PUT' &&
      new URL(response.url()).pathname === mutationPath)
  }

  const nameInput = page.locator('input[name="name"]')
  await expect(nameInput).toHaveValue('Role fixture b')
  await nameInput.fill('My role draft')
  await permissionCheckbox(page, 'roles:read').click()
  await permissionCheckbox(page, 'roles:assign').click()
  page.once('dialog', dialog => dialog.dismiss())
  await page.getByRole('link', { name: 'Cancel', exact: true }).click()
  await expect(page).toHaveURL(editURL)

  const other = await context.newPage()
  try {
    await other.goto(editURL)
    const otherName = other.locator('input[name="name"]')
    await otherName.fill('Saved in another tab')
    const [saved] = await Promise.all([savedBy(other), otherName.press('Enter')])
    assert.equal(saved.status(), 200)
    await expect(other).toHaveURL(editURL)
    await expect(other.locator('.notification--success').filter({ hasText: 'Role updated' })).toHaveCount(1)
  } finally {
    await other.close()
  }

  await legacyPost(mutationPath + '/permissions', { permissions: ['permissions:read'] }, editURL.slice(config.url.length))
  expectConflict(mutationPath)
  const saveReturn = page.getByRole('button', { name: 'Save and return to list', exact: true })
  const [conflict] = await Promise.all([savedBy(page), saveReturn.click()])
  assert.equal(conflict.status(), 409)
  const saved = await conflict.json()
  assert.equal(saved.role.name, 'Saved in another tab')
  await expect(page).toHaveURL(editURL)
  await expect(nameInput).toHaveValue('My role draft')
  const review = page.getByRole('region', { name: 'The role changed after you opened the form' })
  await expect(review.getByText('Saved in another tab', { exact: true })).toBeVisible()
  await expect(review.getByText('My role draft', { exact: true })).toBeVisible()
  await expect(review.getByText('permissions:read', { exact: true })).toBeVisible()
  await expect(review.getByText('roles:assign', { exact: true })).toBeVisible()
  await expect(saveReturn).toBeDisabled()
  await page.screenshot({ path: path.join(folder, `${name}-role-edit-conflict.png`), fullPage: true })
  const beforeAcknowledgement = result.requests.filter(request => request.method === 'PUT' && request.path === mutationPath).length
  await review.getByRole('button', { name: 'Keep my draft' }).click()
  await expect(saveReturn).toBeEnabled()
  assert.equal(result.requests.filter(request => request.method === 'PUT' && request.path === mutationPath).length, beforeAcknowledgement)
  await expect(nameInput).toHaveValue('My role draft')

  const [reapplied] = await Promise.all([savedBy(page), saveReturn.click()])
  assert.equal(reapplied.status(), 200)
  const reply = await reapplied.json()
  assert.notEqual(reply.role.revision, saved.role.revision)
  assert.deepEqual(reply.role.permissions.map(value => `${value.domain}:${value.action}`), ['roles:assign'])
  await expect(page).toHaveURL(listURL)
  await expect(row).toContainText('My role draft')
  await expect(page.locator('.notification--success').filter({ hasText: 'Role updated' })).toHaveCount(1)
  await page.goto('/permissions?domain=roles&action=read')
  const permission = page.getByRole('row').filter({ hasText: 'View (read)' })
  await expect(permission).toContainText('Roles (roles)')
  result.roleEditor = { listURL, concurrentMetadata: true, legacyPermissionWrite: true, conflictStatus: 409, reapplyStatus: 200 }
}
