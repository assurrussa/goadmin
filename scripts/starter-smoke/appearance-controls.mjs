import assert from 'node:assert/strict'

const themeLabels = {
  system: /^(Системная|System)$/,
  light: /^(Светлая|Light)$/,
  dark: /^(Тёмная|Dark)$/,
}
const themeTrigger = page => page.getByRole('button', { name: /^(Выбрать тему|Choose theme|Select theme):/, includeHidden: true })

// Select the stored preference, then inspect the radio tick independently of
// the effective paint. The System tick must survive either OS color scheme.
export async function selectTheme(page, expect, mode) {
  assert.ok(themeLabels[mode], `unsupported theme ${mode}`)
  const trigger = themeTrigger(page)
  await trigger.click()
  await expect(trigger).toHaveAttribute('aria-haspopup', 'menu')
  await expect(trigger).toHaveAttribute('aria-expanded', 'true')
  await expect(page.getByRole('menuitemradio')).toHaveCount(3)
  const option = page.getByRole('menuitemradio', { name: themeLabels[mode], exact: true })
  const changed = await option.getAttribute('aria-checked') !== 'true'
  const authenticated = await page.locator('[data-profile-menu-trigger]').count() > 0
  const saved = changed && authenticated ? page.waitForResponse(response => {
    if (new URL(response.url()).pathname !== '/auth/profile/appearance' || response.request().method() !== 'PATCH') return false
    return response.request().postDataJSON()?.payload?.theme === mode
  }) : undefined
  await option.click()
  if (saved) assert.equal((await saved).status(), 200, `persist ${mode} theme preference`)
  if (mode !== 'system') await expect(page.locator('html')).toHaveAttribute('data-theme-mode', mode)
  await trigger.focus()
  await page.keyboard.press('Enter')
  const selected = page.getByRole('menuitemradio', { name: themeLabels[mode], exact: true })
  await expect(selected).toHaveAttribute('aria-checked', 'true')
  await expect(page.locator('[role="menuitemradio"][aria-checked="true"]')).toHaveCount(1)
  await page.keyboard.press('Escape')
  await expect(trigger).toHaveAttribute('aria-expanded', 'false')
  await expect(trigger).toBeFocused()
}

export async function assertSystemTheme(page, expect) {
  // Call after System has settled, so reload also proves persistence rather
  // than cancelling an in-flight account preference save.
  for (const mode of ['dark', 'light']) {
    await page.emulateMedia({ colorScheme: mode })
    await expect(page.locator('html')).toHaveAttribute('data-theme-mode', mode)
    await expect(themeTrigger(page)).toHaveAttribute('title', mode === 'dark'
      ? /^(Системная \(тёмная\)|System \(dark\))$/
      : /^(Системная \(светлая\)|System \(light\))$/)
    await page.reload()
    await expect(page.locator('html')).toHaveAttribute('data-theme-mode', mode)
    await selectTheme(page, expect, 'system')
  }
}

export async function selectSettingsSection(page, expect, section, { keyboard = false } = {}) {
  assert.ok(['interface', 'account', 'security', 'branding'].includes(section))
  if (new URL(page.url()).pathname !== '/auth/profile/settings') await page.goto(`/auth/profile/settings#${section}`)
  const nav = page.getByRole('navigation', { name: /^(Разделы настроек|Settings sections)$/ })
  await expect(nav).toBeVisible()
  const select = nav.locator('select')
  if (await select.isVisible()) await select.selectOption(section)
  else {
    const link = nav.locator(`a[href="#${section}"]`)
    if (keyboard) { await link.focus(); await page.keyboard.press('Enter') }
    else await link.click()
  }
  await expect(page).toHaveURL(new RegExp(`/auth/profile/settings#${section}$`))
  await expect(select).toHaveValue(section)
  await expect(nav.locator(`a[href="#${section}"]`)).toHaveAttribute('aria-current', 'page')
}

export async function assertSettingsNavigation(page, expect, { branding = true } = {}) {
  const nav = page.getByRole('navigation', { name: /^(Разделы настроек|Settings sections)$/ })
  const groups = await nav.locator('optgroup').evaluateAll(nodes => nodes.map(node => ({
    label: node.label, values: [...node.children].map(option => option.value),
  })))
  assert.equal(groups.length, branding ? 2 : 1)
  assert.match(groups[0].label, /^(Личные|Personal)$/)
  assert.deepEqual(groups[0].values, ['interface', 'account', 'security'])
  if (branding) {
    assert.match(groups[1].label, /^(Общие|Shared|General)$/)
    assert.deepEqual(groups[1].values, ['branding'])
  }
  await expect(nav.locator('a')).toHaveCount(branding ? 4 : 3)
  const mobile = await nav.locator('select').isVisible()
  if (mobile) await expect(nav.locator('a[href="#interface"]')).toBeHidden()
  else await expect(nav.locator('a[href="#interface"]')).toBeVisible()
  return { mobile, groups }
}

export async function assertBrandingLayout(page, expect) {
  const section = page.locator('[aria-labelledby="branding-title"]')
  await expect(section.getByRole('heading', { name: /^(Название|Name)$/, exact: true })).toBeVisible()
  await expect(section.getByRole('heading', { name: /^(Цвета|Colors)$/, exact: true })).toBeVisible()
  await expect(section.getByRole('heading', { name: /^(Логотипы|Logos)$/, exact: true })).toBeVisible()
  const form = await section.locator('fieldset').boundingBox()
  const preview = await section.locator('[data-part="branding-preview"]').boundingBox()
  assert.ok(form && preview)
  if (page.viewportSize().width >= 1280) assert.ok(preview.x >= form.x + form.width, 'wide branding preview must sit beside the grouped form')
  else assert.ok(preview.y >= form.y + form.height, 'compact branding preview must follow the grouped form')
}
