import assert from 'node:assert/strict'
import path from 'node:path'

// Real backend snapshots prove data/permissions. One explicit response override
// below verifies stale rendering; it is not a backend-failure acceptance claim.
export async function dashboardAcceptance({ page, folder, name, expect, language, result }) {
  const dataResponse = page.waitForResponse((r) => new URL(r.url()).pathname === '/dashboard/data')
  await page.goto('/')
  const snapshot = await (await dataResponse).json()
  assert.deepEqual(snapshot.cards.map((card) => card.key), [
    'goadmin.queues', 'goadmin.notifications', 'starter.summary',
  ])
  assert.ok(snapshot.cards.every((card) => card.state === 'ready'))
  const host = page.locator('[data-dashboard-card="starter.summary"]')
  const notices = page.locator('[data-dashboard-card="goadmin.notifications"]')
  await expect(host.locator('[data-dashboard-metric="source"]')).toHaveText('starter')
  await expect(notices.locator('[data-dashboard-metric="unread-notifications"]')).toHaveText('2')
  await expect(host.getByRole('link', { name: 'Administrators', exact: true })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Available management sections', exact: true })).toBeVisible()
  const refresh = page.getByRole('button', { name: 'Refresh', exact: true })
  const before = await host.locator('time').getAttribute('datetime')
  await page.route('**/dashboard/data', async (route) => {
    const response = await route.fetch()
    const data = await response.json()
    data.cards = data.cards.map((card) => card.key === 'starter.summary'
      ? { ...card, state: 'unavailable', metrics: [], updatedAt: undefined } : card)
    await route.fulfill({ response, json: data })
  }, { times: 1 })
  await refresh.click()
  await expect(host).toContainText('Showing the last received data')
  await expect(host.locator('[data-dashboard-metric="source"]')).toHaveText('starter')
  await expect(host.locator('time')).toHaveAttribute('datetime', before)
  await refresh.click()
  await expect(host).not.toContainText('Showing the last received data')
  const auto = page.getByRole('checkbox', { name: 'Auto-refresh every minute', exact: true })
  await auto.focus()
  await page.keyboard.press('Space')
  await expect(auto).toBeChecked()
  await page.reload()
  await expect(auto).toBeChecked()
  await auto.uncheck()
  await expect(host.locator('[data-dashboard-metric="source"]')).toHaveText('starter')
  await page.screenshot({ path: path.join(folder, `${name}-dashboard-light-en.png`), fullPage: true })
  await page.getByRole('button', { name: /Switch to dark theme|Переключить на темную тему/i }).click()
  await language(page, 'Русский', 'ru')
  await expect(host.getByRole('heading', { name: 'Карточка хоста', exact: true })).toBeVisible()
  await page.setViewportSize({ width: 390, height: 844 })
  assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), 'dashboard mobile fit')
  await page.screenshot({ path: path.join(folder, `${name}-dashboard-dark-mobile-ru.png`), fullPage: true })
  await page.setViewportSize({ width: 1440, height: 1000 })
  await language(page, 'English', 'en')
  await page.getByRole('button', { name: /Switch to light theme|Переключить на светлую тему/i }).click()
  await notices.getByRole('link', { name: 'Unread', exact: true }).click()
  await expect(page).toHaveURL(/\/notifications\?status=unread$/)
  result.dashboard = { cards: snapshot.cards.map((card) => card.key), unread: 2,
    staleRendering: 'explicit HTTP snapshot override', hostSource: 'starter', liveLocale: true }
}
