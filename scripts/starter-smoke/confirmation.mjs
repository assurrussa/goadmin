import assert from 'node:assert/strict'
import path from 'node:path'
import { writeFile } from 'node:fs/promises'
import { selectTheme } from './appearance-controls.mjs'
import { withBrowserZoom } from './browser-zoom.mjs'

// Playwright's native zoom screenshots can clip physical pixels to CSS sizes.
// With no clip, CDP captures the actual compositor viewport at its native size.
async function screenshotViewport(page, filename, zoomed) {
  if (!zoomed) return page.screenshot({ path: filename })
  const session = await page.context().newCDPSession(page)
  try {
    const { data } = await session.send('Page.captureScreenshot', {
      format: 'png', fromSurface: true, captureBeyondViewport: false,
    })
    await writeFile(filename, Buffer.from(data, 'base64'))
  } finally { await session.detach() }
}

const viewports = [
  { width: 320, height: 568 }, { width: 390, height: 844 },
  { width: 844, height: 390 }, { width: 1440, height: 1000 },
]
const prose = {
  en: Array.from({ length: 8 }, (_, index) => `Review ${index + 1}: Publishing this product makes its information available to the intended audience. Please check the description, category, availability and stock before continuing. You can cancel this confirmation and return to the unchanged draft to review the details.`).join('\n\n'),
  ru: Array.from({ length: 8 }, (_, index) => `Проверка ${index + 1}: После публикации информация о товаре станет доступна выбранной аудитории. Проверьте описание, категорию, дату доступности и остаток перед продолжением. Можно отменить подтверждение и вернуться к неизменённому черновику для проверки всех подробностей.`).join('\n\n'),
}

// Change only one real Show response's command prose. The application still
// supplies every route, permission, revision and the compiled confirmation UI.
export async function confirmationAcceptance({ page, context, config, folder, name, expect, language, result, recordPath, prefix }) {
  const report = result.crud.confirmationLayout = { matrix: [], pageErrors: [], consoleErrors: [], commandRequests: [] }
  const originalViewport = page.viewportSize()
  const watch = target => {
    const error = value => report.pageErrors.push(value.message)
    const consoleError = value => { if (value.type() === 'error') report.consoleErrors.push(value.text()) }
    const request = value => {
      if (new URL(value.url()).pathname.startsWith(recordPath) && !['GET', 'HEAD'].includes(value.method()))
        report.commandRequests.push({ method: value.method(), url: value.url() })
    }
    target.on('pageerror', error); target.on('console', consoleError); target.on('request', request)
    return () => { target.off('pageerror', error); target.off('console', consoleError); target.off('request', request) }
  }
  const run = async (target, { zoomed = false, baseline, zoom } = {}) => {
    const unwatch = watch(target)
    let locale = 'en', intercepted = 0
    const handler = async route => {
      const request = route.request()
      if (request.method() !== 'GET' || new URL(request.url()).pathname !== recordPath || request.headers()['x-inertia'] !== 'true') return route.continue()
      const response = await route.fetch()
      assert.equal(response.status(), 200)
      const body = await response.json()
      assert.equal(body.component, 'crud/Show')
      const action = body.props.resource.actions.find(action => action.key === 'publish')
      assert.ok(action, 'The real draft must expose the publish command')
      action.confirmation = prose[locale]
      intercepted++
      await route.fulfill({ response, json: body })
    }
    await target.route('**/products/records/**', handler)
    try {
      for (const [code, label] of [['ru', 'Русский'], ['en', 'English']]) {
        locale = code
        // Keep global controls available before entering narrow viewports.
        if (!zoomed) await target.setViewportSize({ width: 1440, height: 1000 })
        await language(target, label, code)
        for (const theme of ['light', 'dark']) {
          if (!zoomed) await target.setViewportSize({ width: 1440, height: 1000 })
          await selectTheme(target, expect, theme)
          await target.goto(`/products?search=${encodeURIComponent(prefix)}`)
          const before = intercepted
          await target.getByRole('button', { name: code === 'ru' ? 'Просмотр' : 'View', exact: true }).click()
          await expect(target).toHaveURL(new RegExp(`${recordPath}(?:\\?.*)?$`))
          assert.ok(intercepted > before, 'Long prose must come from a genuine Inertia Show GET')
          for (const viewport of zoomed ? [null] : viewports) {
            if (viewport) await target.setViewportSize(viewport)
            const measuredViewport = await target.evaluate(() => ({ width: innerWidth, height: innerHeight, dpr: devicePixelRatio, font: getComputedStyle(document.documentElement).fontSize }))
            if (zoomed) {
              assert.equal(await zoom(target, 2), 2, 'Verify real browser getZoom at every matrix entry')
              assert.ok(Math.abs(measuredViewport.width - baseline.width / 2) <= 1)
              assert.ok(Math.abs(measuredViewport.dpr / baseline.dpr - 2) < .01)
              assert.equal(measuredViewport.font, baseline.font)
            }
            const opener = target.getByRole('button', { name: code === 'ru' ? 'Опубликовать' : 'Publish', exact: true })
            const dialog = target.locator('.goadmin-confirmation-dialog')
            const open = async () => {
              await opener.click()
              await expect(dialog).toBeVisible()
              await expect(dialog.locator('[data-confirmation-cancel]')).toBeFocused()
              await expect(dialog).toContainText(prose[code])
              // Raw mouse clicks have no locator actionability wait. Let the
              // entrance finish and Reka install its deferred outside handler.
              await dialog.evaluate(async node => {
                let deadline
                try {
                  await Promise.race([
                    (async () => {
                      await Promise.all(node.getAnimations().map(animation => animation.finished.catch(() => {})))
                      await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)))
                    })(),
                    new Promise((_, reject) => {
                      deadline = setTimeout(() => reject(new Error('Confirmation entrance did not settle within 5000ms')), 5000)
                    }),
                  ])
                } finally { clearTimeout(deadline) }
              })
            }
            await open()
            // Wait for Reka's entrance animation before measuring real bounds.
            await expect.poll(() => dialog.evaluate(node => {
              const r = node.getBoundingClientRect()
              return r.left >= 15.5 && r.top >= 15.5 && r.right <= innerWidth - 15.5 && r.bottom <= innerHeight - 15.5
            })).toBe(true)
            // Initial Cancel focus may scroll its footer into view. Capture
            // the start of the prose, then prove the user can reach the end.
            await dialog.evaluate(node => {
              for (const element of [node, ...node.querySelectorAll('*')])
                if (/auto|scroll/.test(getComputedStyle(element).overflowY)) element.scrollTop = 0
            })
            const geometry = await dialog.evaluate(node => {
              const bounds = element => { const r = element.getBoundingClientRect(); return { top: r.top, bottom: r.bottom, left: r.left, right: r.right } }
              const cancel = node.querySelector('[data-confirmation-cancel]'), accept = node.querySelector('[data-confirmation-accept]')
              const names = (node.getAttribute('aria-labelledby') || '').split(/\s+/).filter(Boolean)
              const descriptions = (node.getAttribute('aria-describedby') || '').split(/\s+/).filter(Boolean)
              const linked = ids => ids.length > 0 && ids.every(id => !!document.getElementById(id)?.textContent.trim())
              const scrollable = [node, ...node.querySelectorAll('*')].filter(element => /auto|scroll/.test(getComputedStyle(element).overflowY) && element.scrollHeight > element.clientHeight + 1)
              return { rect: bounds(node), cancel: bounds(cancel), accept: bounds(accept), role: node.getAttribute('role'), modal: node.getAttribute('aria-modal'), named: linked(names), described: linked(descriptions),
                horizontalOverflow: node.scrollWidth > node.clientWidth + 1 || document.documentElement.scrollWidth > innerWidth + 1,
                scrollable: scrollable.length }
            })
            assert.equal(geometry.role, 'dialog'); assert.equal(geometry.modal, 'true')
            assert.ok(geometry.named && geometry.described, 'Accessible name and description must resolve to nonempty real elements')
            assert.equal(geometry.horizontalOverflow, false)
            if (measuredViewport.width < 640) {
              const buttons = [geometry.cancel, geometry.accept].sort((a, b) => a.top - b.top)
              assert.ok(buttons[1].top - buttons[0].bottom >= 7.5, 'Stacked mobile buttons need at least 8px vertical separation')
            }
            const screenshot = folder && ((viewport?.width === 320) || zoomed) && ((code === 'ru' && theme === 'dark') || (code === 'en' && theme === 'light'))
            const filename = `${name}-confirmation-${code}-${theme}-${zoomed ? 'zoom200' : `${viewport.width}x${viewport.height}`}`
            if (screenshot) await screenshotViewport(target, path.join(folder, `${filename}-before-scroll.png`), zoomed)
            const scrolled = await dialog.evaluate(node => {
              const element = [node, ...node.querySelectorAll('*')].find(element => /auto|scroll/.test(getComputedStyle(element).overflowY) && element.scrollHeight > element.clientHeight + 1)
              if (!element) return false
              element.scrollTop = element.scrollHeight
              return element.scrollTop > 0
            })
            if (measuredViewport.height <= 568 || zoomed) assert.equal(scrolled, true, 'Long text must really scroll at short viewport sizes')
            const accept = dialog.locator('[data-confirmation-accept]')
            await accept.scrollIntoViewIfNeeded()
            const footer = await accept.boundingBox()
            assert.ok(footer && footer.y >= 16 && footer.y + footer.height <= measuredViewport.height - 15.5, 'Footer must be reachable after scrolling')
            if (screenshot) await screenshotViewport(target, path.join(folder, `${filename}-after-scroll.png`), zoomed)
            // More than two complete cycles in each direction, including wrap.
            const focusables = await dialog.locator('button:not([disabled]), [href], input:not([disabled]), [tabindex="0"]').count()
            for (const key of ['Tab', 'Shift+Tab']) for (let index = 0; index < focusables * 2 + 1; index++) {
              await target.keyboard.press(key)
              assert.ok(await dialog.evaluate(node => node.contains(document.activeElement)), `${key} must remain within the modal`)
            }
            for (const dismissal of ['escape', 'close', 'outside', 'cancel']) {
              if (dismissal !== 'escape') await open()
              if (dismissal === 'escape') await target.keyboard.press('Escape')
              if (dismissal === 'close') await dialog.locator('[data-part="close"]').click()
              if (dismissal === 'outside') await target.mouse.click(4, 4)
              if (dismissal === 'cancel') await dialog.locator('[data-confirmation-cancel]').click()
              const caseName = `${code}/${theme}/${measuredViewport.width}x${measuredViewport.height}/${dismissal}`
              await expect(dialog, caseName).toHaveCount(0)
              await expect(opener).toBeFocused()
              assert.deepEqual(report.commandRequests, [], `${dismissal} must not issue a command request`)
            }
            report.matrix.push({ locale: code, theme, ...measuredViewport, zoom: zoomed ? 2 : 1, scrolled, dismissals: ['escape', 'close', 'outside', 'cancel'] })
          }
        }
      }
    } finally { await target.unroute('**/products/records/**', handler); unwatch() }
  }
  try {
    await run(page)
    await withBrowserZoom({ baseURL: config.url, reducedMotion: 'reduce' }, async (zoomContext, zoom) => {
      await zoomContext.addCookies(await context.cookies())
      const zoomPage = zoomContext.pages()[0]
      const unwatchInitialNavigation = watch(zoomPage)
      await zoomPage.goto(recordPath)
      const baseline = await zoomPage.evaluate(() => ({ width: innerWidth, dpr: devicePixelRatio, font: getComputedStyle(document.documentElement).fontSize }))
      await zoom(zoomPage, 2)
      await expect.poll(() => zoomPage.evaluate(() => innerWidth)).toBeCloseTo(baseline.width / 2, 0)
      unwatchInitialNavigation()
      await run(zoomPage, { zoomed: true, baseline, zoom })
    })
    assert.equal(report.matrix.length, 20)
    assert.deepEqual(report.pageErrors, [])
    assert.deepEqual(report.consoleErrors, [])
    assert.deepEqual(report.commandRequests, [])
  } finally {
    if (originalViewport) await page.setViewportSize(originalViewport)
    // The zoom context saved preferences for the same account; refresh the
    // main page's appearance revision before its next authenticated save.
    await page.goto(recordPath)
    await language(page, 'English', 'en')
    await selectTheme(page, expect, 'light')
    await page.goto(recordPath)
  }
}
