import assert from 'node:assert/strict'
import path from 'node:path'

const dialog = page => page.locator('[data-crud-action-dialog]')
const reason = page => dialog(page).locator('textarea[name="reason"]')
const accept = page => dialog(page).getByRole('button', { name: 'Unpublish', exact: true })
const isCommand = (request, url) => request.method() === 'POST' && new URL(request.url()).pathname === url

async function confirm(page, url, status) {
  const [response] = await Promise.all([
    page.waitForResponse(r => isCommand(r.request(), url)),
    accept(page).click(),
  ])
  assert.equal(response.status(), status)
  return response
}

export async function unpublishFromCard({ page, expect, recordPath, id, language, folder, name }) {
  const url = recordPath + '/actions/unpublish'
  const writes = []
  const track = req => { if (isCommand(req, url)) writes.push(req.postDataJSON()) }
  page.on('request', track)
  const open = () => page.getByRole('button', { name: 'Unpublish', exact: true }).click()
  const explanation = 'Price under review\nVerify 🙂 before publishing again.'
  try {
    await language(page, 'Русский', 'ru')
    await page.getByRole('button', { name: 'Снять с публикации', exact: true }).click()
    await expect(dialog(page)).toContainText(id)
    await expect(reason(page)).toHaveAccessibleDescription(
      'Укажите причину снятия с публикации. Она останется в карточке события даже после удаления товара. До 500 символов',
    )
    await reason(page).fill('Нужно уточнить описание товара.')
    await page.screenshot({ path: path.join(folder, name + '-unpublish-reason-ru.png') })
    await page.keyboard.press('Escape')
    await expect(dialog(page)).toHaveCount(0)
    await language(page, 'English', 'en')
    await open()
    await expect(dialog(page).getByText('The product will become a draft. This action and its reason are recorded in Product activity.')).toBeVisible()
    await expect(reason(page)).toHaveAccessibleDescription(
      'Explain why you are removing this product. The reason remains in the activity entry after deletion. Up to 500 characters',
    )
    await expect(reason(page)).toHaveValue('')
    await reason(page).fill('\u0085  ')
    if (await accept(page).isEnabled()) await accept(page).click()
    await reason(page).fill('🙂'.repeat(501))
    if (await accept(page).isEnabled()) await accept(page).click()
    await expect(reason(page)).toHaveAttribute('aria-invalid', 'true')
    await reason(page).fill('First line')
    await reason(page).press('Enter')
    await expect(reason(page)).toHaveValue('First line\n')
    assert.equal(writes.length, 0, 'cancel, invalid input and a newline must not unpublish')
    await reason(page).fill('\u0085  ' + explanation + '\u2003')
    await confirm(page, url, 204)
    await expect(dialog(page)).toHaveCount(0)
    assert.equal(writes.length, 1)
    return explanation
  } finally {
    page.off('request', track)
  }
}

export async function unpublishFromList({ context, pageTwo, recordPath, name, folder, expect, conditions, currentName }) {
  const url = recordPath + '/actions/unpublish'
  const page = await context.newPage()
  const colleague = await context.newPage()
  const writes = []
  const errors = []
  page.on('pageerror', err => errors.push(err.message))
  page.on('console', message => {
    if (message.type() !== 'error') return
    const location = message.location().url
    // Chromium emits a network console error for each intentionally rejected POST.
    if (location && new URL(location).pathname === url &&
      /^Failed to load resource: the server responded with a status of (409|422)\b/.test(message.text())) return
    errors.push(message.text())
  })
  page.on('request', req => { if (isCommand(req, url)) writes.push(req.postDataJSON()) })
  const explanation = 'Confirm specifications\nKeep  internal spacing 🙂.'
  const raw = '   ' + explanation + ' \n'
  const updatedName = currentName + '-reviewed'
  try {
    await page.goto(pageTwo)
    await page.locator('[data-part="table"] tbody tr').filter({ hasText: currentName })
      .getByRole('button', { name: 'More', exact: true }).click()
    await page.getByRole('menuitem', { name: 'Unpublish', exact: true }).click()
    await reason(page).fill(raw)
    // Corrupt only the outbound envelope once. The running server supplies the real 422.
    await page.route(reqUrl => reqUrl.pathname === url, async route => {
      const data = route.request().postDataJSON()
      await route.continue({ postData: JSON.stringify({ ...data, params: { ...data.params, unexpected: true } }) })
    }, { times: 1 })
    const rejected = await confirm(page, url, 422)
    assert.ok((await rejected.json()).errors.reason.length)
    await expect(reason(page)).toHaveValue(raw)
    await expect(reason(page)).toHaveAttribute('aria-invalid', 'true')
    await expect(reason(page)).toBeFocused()
    // A real edit in the other tab advances the revision kept by the open dialog.
    await colleague.goto(recordPath + '/edit')
    await colleague.locator('[name="name"]').fill(updatedName)
    const [saved] = await Promise.all([
      colleague.waitForResponse(r => r.request().method() === 'PUT' && new URL(r.url()).pathname === recordPath),
      colleague.getByRole('button', { name: 'Save', exact: true }).click(),
    ])
    assert.equal(saved.status(), 303)
    await confirm(page, url, 409)
    await expect(dialog(page)).toContainText(updatedName)
    await expect(reason(page)).toHaveValue(raw)
    assert.equal(writes.length, 2, 'Refreshing a conflict must not replay the command')
    await page.screenshot({ path: path.join(folder, name + '-unpublish-conflict.png') })
    await confirm(page, url, 204)
    await expect(page.getByRole('link', { name: 'Back to list', exact: true })).toBeVisible()
    assert.equal(new URL(page.url()).pathname, recordPath)
    assert.equal(writes.length, 3)
    assert.equal(writes[0]._revision, writes[1]._revision)
    assert.notEqual(writes[1]._revision, writes[2]._revision)
    await page.getByRole('link', { name: 'Back to list', exact: true }).click()
    await expect(page.locator('[data-part="table"] tbody tr').filter({ hasText: updatedName })).toBeVisible()
    assert.deepEqual(Object.fromEntries(new URL(page.url()).searchParams), conditions)
    assert.deepEqual(errors, [])
    return explanation
  } finally {
    await colleague.close()
    await page.close()
  }
}
