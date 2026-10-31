import assert from 'node:assert/strict'
import path from 'node:path'
import { withBrowserZoom } from './browser-zoom.mjs'

const screens = [
  ['dashboard', '/'], ['table', '/users'], ['form', '/auth/profile/edit'],
  ['interface', '/auth/profile/settings#interface'], ['branding', '/auth/profile/settings#branding'],
]

// Read actual computed paint (including alpha blending), not source token values.
export async function appearanceGeometry(page) {
  return page.evaluate(() => {
    const canvas = document.createElement('canvas')
    canvas.width = canvas.height = 1
    const drawing = canvas.getContext('2d', { willReadFrequently: true })
    const rgba = css => {
      drawing.clearRect(0, 0, 1, 1)
      drawing.fillStyle = css; drawing.fillRect(0, 0, 1, 1)
      return [...drawing.getImageData(0, 0, 1, 1).data]
    }
    const over = (a, b) => a.slice(0, 3).map((v, i) => v * a[3] / 255 + b[i] * (1 - a[3] / 255))
    const background = node => {
      if (!node) return [255, 255, 255]
      return over(rgba(getComputedStyle(node).backgroundColor), background(node.parentElement))
    }
    const luminance = rgb => rgb.map(v => {
      v /= 255; return v <= 0.04045 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4
    }).reduce((sum, v, i) => sum + v * [0.2126, 0.7152, 0.0722][i], 0)
    const paint = node => {
      const bg = background(node), fg = over(rgba(getComputedStyle(node).color), bg)
      const a = luminance(bg), b = luminance(fg)
      return { background: bg, foreground: fg, contrast: (Math.max(a, b) + .05) / (Math.min(a, b) + .05) }
    }
    const visible = node => {
      const rect = node.getBoundingClientRect()
      return rect.width > 0 && rect.height > 0 && getComputedStyle(node).visibility !== 'hidden'
    }
    const targets = [...document.querySelectorAll('[data-ui="button"], [data-ui="input"], [data-ui="select"][data-part="trigger"], .appearance-select, [data-ui="navigation"] a')]
      .filter(visible).filter(n => !n.disabled).map(node => {
        const rect = node.getBoundingClientRect()
        return { label: node.getAttribute('aria-label') || node.textContent?.trim().slice(0, 60) || node.id || node.tagName,
          width: rect.width, height: rect.height }
      })
    const preview = document.querySelector('[aria-labelledby="branding-title"] [aria-label]')
    const panels = preview ? [...preview.children].map(panel => ({
      dark: panel.classList.contains('dark'),
      link: paint(panel.querySelector('a')),
      field: paint(panel.querySelector('input')),
      button: paint(panel.querySelector('.bg-primary')),
    })) : []
    const heading = document.querySelector('h1, main label, form label')
    const probe = document.createElement('span')
    probe.style.color = 'var(--color-primary)'
    document.body.append(probe)
    const accent = rgba(getComputedStyle(probe).color).slice(0, 3)
    probe.remove()
    return { width: innerWidth, scrollWidth: document.documentElement.scrollWidth,
      dpr: devicePixelRatio, fontSize: getComputedStyle(document.documentElement).fontSize,
      theme: document.documentElement.dataset.themeMode, density: document.documentElement.dataset.density,
      locale: document.documentElement.lang, heading: heading ? paint(heading) : null,
      targets, panels, accent, frames: window.__appearanceFrames || [] }
  })
}

async function installPaintProbe(context) {
  await context.addInitScript(() => {
    window.__appearanceFrames = []
    const read = () => {
      if (document.querySelector('h1, main label, form label')) window.__appearanceFrames.push(document.documentElement.dataset.themeMode)
      if (window.__appearanceFrames.length < 12) requestAnimationFrame(read)
    }
    requestAnimationFrame(read)
  })
}

async function inspect(page, url, expected, expect, { touch = false, standalone = false } = {}) {
  let response = await page.goto(url)
  // Hash-only navigation has no response; reload also exercises server prepaint.
  if (!response) response = await page.reload()
  assert.equal(response.status(), url.includes('/missing-appearance-page') ? 404 : 200, url)
  await expect(page.locator('h1, main label, form label').first()).toBeVisible()
  await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))))
  await expect(page.locator('html')).toHaveAttribute('data-theme-mode', expected.theme)
  if (!standalone) {
    await expect(page.locator('html')).toHaveAttribute('data-density', expected.density)
    await expect(page.locator('html')).toHaveAttribute('lang', expected.locale)
  }
  const geometry = await appearanceGeometry(page)
  assert.ok(geometry.scrollWidth <= geometry.width + 1, `${url}: horizontal page overflow ${geometry.scrollWidth}/${geometry.width}`)
  assert.ok(geometry.heading.contrast >= 4.5, `${url}: heading contrast ${geometry.heading.contrast}`)
  assert.ok(geometry.frames.length > 0 && geometry.frames.every(theme => theme === expected.theme), `${url}: incorrect first-paint theme: ${geometry.frames}`)
  if (touch) {
    assert.ok(geometry.targets.length > 0 || standalone, `${url}: no controls measured`)
    assert.deepEqual(geometry.targets.filter(n => n.height < 43.9 || n.width < 43.9), [], `${url}: touch targets must be at least 44x44 CSS px`)
  }
  if (url.endsWith('#branding')) {
    await expect(page.locator('[aria-labelledby="branding-title"]').getByRole('button', {
      name: expected.locale === 'en' ? 'Apply' : 'Применить', exact: true,
    })).toBeVisible()
    assert.equal(geometry.panels.length, 2, 'Both branding preview panels must be measured')
    assert.deepEqual(geometry.panels.map(panel => panel.dark).sort(), [false, true])
  }
  for (const panel of geometry.panels) {
    for (const part of ['link', 'field', 'button']) assert.ok(panel[part].contrast >= 4.5,
      `${url}: ${panel.dark ? 'dark' : 'light'} preview ${part} contrast ${panel[part].contrast}`)
  }
  if (url.endsWith('#interface')) {
    const theme = page.locator('#appearance-theme')
    await theme.focus()
    await expect(theme).toBeFocused()
    const focus = await theme.evaluate(node => {
      const css = getComputedStyle(node)
      return css.outlineStyle !== 'none' && parseFloat(css.outlineWidth) > 0 || css.boxShadow !== 'none'
    })
    assert.ok(focus, 'Keyboard focus must have a visible indicator')
    await page.keyboard.press('Tab')
    await expect(page.locator('#appearance-locale')).toBeFocused()
  }
  // Keep the report compact; retain actual minima and both preview palettes.
  return { ...geometry, frames: geometry.frames.length,
    targets: { count: geometry.targets.length,
      minHeight: geometry.targets.length ? Math.min(...geometry.targets.map(n => n.height)) : null,
      minWidth: geometry.targets.length ? Math.min(...geometry.targets.map(n => n.width)) : null } }
}

export async function appearanceVisualAcceptance({ browser, context, page, config, folder, name, expect, result, get, send }) {
  const original = await get('/auth/profile/appearance')
  const report = result.appearanceVisual = { matrix: [], system: [], zoom: [] }
  const watch = ctx => {
    const bind = p => {
      p.on('pageerror', error => { (report.pageErrors ||= []).push(error.message) })
      p.on('console', message => {
        if (message.type() === 'error' && !message.location().url.includes('/missing-appearance-page'))
          (report.consoleErrors ||= []).push(message.text())
      })
    }
    ctx.pages().forEach(bind)
    ctx.on('page', bind)
  }
  const save = async payload => {
    const current = await get('/auth/profile/appearance')
    const response = await send('PATCH', '/auth/profile/appearance', { version: current.version, payload })
    assert.equal(response.status(), 200)
  }
  try {
    for (const viewport of [{ width: 1440, height: 1000 }, { width: 390, height: 844 }]) {
      console.log(`Appearance matrix ${name}: viewport ${viewport.width}`)
      const touch = viewport.width < 768
      const account = await browser.newContext({ baseURL: config.url, viewport, hasTouch: touch,
        storageState: await context.storageState(), reducedMotion: 'reduce' })
      const anonymous = await browser.newContext({ baseURL: config.url, viewport, hasTouch: touch, reducedMotion: 'reduce' })
      watch(account); watch(anonymous)
      await installPaintProbe(account); await installPaintProbe(anonymous)
      const authenticatedPage = await account.newPage(), anonymousPage = await anonymous.newPage()
      try {
        for (const theme of ['light', 'dark']) for (const locale of ['ru', 'en']) for (const density of ['default', 'compact', 'comfortable']) {
          const payload = { schemaVersion: 1, theme, locale, density }
          await save(payload)
          for (const [screen, url] of screens) {
            const measured = await inspect(authenticatedPage, url, payload, expect, { touch })
            report.matrix.push({ screen, viewport: viewport.width, requested: payload, ...measured })
            if (theme === 'dark' && locale === 'en' && density === 'compact') await authenticatedPage.screenshot({
              path: path.join(folder, `${name}-matrix-${screen}-${viewport.width}.png`), fullPage: true })
          }
          // The standalone error template uses device preferences and must work
          // without loading private account state. Reach the native 404 with an
          // authenticated context, because anonymous unknown routes go to login.
          await authenticatedPage.evaluate(payload => localStorage.setItem('goadmin.appearance.device.v1', JSON.stringify(payload)), payload)
          const errorPaint = await inspect(authenticatedPage, '/missing-appearance-page', payload, expect, { standalone: true })
          report.matrix.push({ screen: 'error', viewport: viewport.width, requested: payload, ...errorPaint })
          // Anonymous screens intentionally use the separate device preference document.
          await anonymousPage.goto('/auth/login')
          await anonymousPage.evaluate(payload => localStorage.setItem('goadmin.appearance.device.v1', JSON.stringify(payload)), payload)
          for (const [screen, url] of [['login', '/auth/login']]) {
            const measured = await inspect(anonymousPage, url, payload, expect, { touch })
            report.matrix.push({ screen, viewport: viewport.width, requested: payload, ...measured })
          }
        }
        await save({ ...original.payload, theme: 'system' })
        for (const colorScheme of ['light', 'dark']) {
          await authenticatedPage.emulateMedia({ colorScheme })
          report.system.push({ viewport: viewport.width, colorScheme,
            ...await inspect(authenticatedPage, '/auth/profile/settings#interface', { ...original.payload, theme: colorScheme }, expect, { touch }) })
        }
      } finally { await account.close(); await anonymous.close() }
    }
    // Desktop with a coarse pointer must keep the same minimum target size.
    const coarse = await browser.newContext({ baseURL: config.url, viewport: { width: 1440, height: 1000 },
      hasTouch: true, storageState: await context.storageState() })
    watch(coarse)
    await installPaintProbe(coarse)
    try {
      await save({ ...original.payload, theme: 'light', density: 'compact' })
      const coarsePage = await coarse.newPage()
      report.desktopCoarse = await inspect(coarsePage, '/auth/profile/settings#interface',
        { ...original.payload, theme: 'light', density: 'compact' }, expect, { touch: true })
      assert.equal(await coarsePage.evaluate(() => matchMedia('(pointer: coarse)').matches), true)
    } finally { await coarse.close() }

    await withBrowserZoom({ baseURL: config.url, reducedMotion: 'reduce' }, async (zoomContext, zoom) => {
      watch(zoomContext)
      await installPaintProbe(zoomContext)
      await zoomContext.addCookies(await context.cookies())
      const zoomPage = zoomContext.pages()[0]
      await zoomPage.goto(config.url)
      const baseline = await zoomPage.evaluate(() => ({ width: innerWidth, dpr: devicePixelRatio,
        fontSize: getComputedStyle(document.documentElement).fontSize }))
      await zoom(zoomPage, 2)
      await expect.poll(() => zoomPage.evaluate(() => innerWidth)).toBeCloseTo(baseline.width / 2, 0)
      for (const theme of ['light', 'dark']) {
        await save({ ...original.payload, theme, density: 'compact' })
        for (const [screen, url] of screens) {
          const measured = await inspect(zoomPage, url, { ...original.payload, theme, density: 'compact' }, expect, { touch: true })
          assert.ok(Math.abs(measured.dpr / baseline.dpr - 2) < .01, 'Browser zoom must double devicePixelRatio')
          assert.equal(measured.fontSize, baseline.fontSize, 'Browser zoom must not rewrite root font size')
          report.zoom.push({ screen, theme, factor: 2, ...measured })
        }
        const payload = { ...original.payload, theme }
        await zoomPage.evaluate(payload => localStorage.setItem('goadmin.appearance.device.v1', JSON.stringify(payload)), payload)
        report.zoom.push({ screen: 'error', theme, factor: 2,
          ...await inspect(zoomPage, '/missing-appearance-page', payload, expect, { standalone: true }) })
      }
      await zoomContext.clearCookies()
      for (const theme of ['light', 'dark']) {
        const payload = { ...original.payload, theme }
        await zoomPage.evaluate(payload => localStorage.setItem('goadmin.appearance.device.v1', JSON.stringify(payload)), payload)
        report.zoom.push({ screen: 'login', theme, factor: 2,
          ...await inspect(zoomPage, '/auth/login', payload, expect, { touch: true }) })
      }
      for (const row of report.zoom) {
        assert.ok(Math.abs(row.dpr / baseline.dpr - 2) < .01, `${row.screen}: browser zoom must remain 200%`)
        assert.ok(Math.abs(row.width - baseline.width / 2) <= 1, `${row.screen}: browser zoom must reflow the layout viewport`)
      }
      await zoomPage.screenshot({ path: path.join(folder, `${name}-browser-zoom-200.png`), fullPage: true })
    })
    const palettes = Object.fromEntries(['light', 'dark'].map(theme => [theme,
      report.matrix.find(row => row.screen === 'interface' && row.theme === theme).accent]))
    for (const row of report.matrix.filter(row => row.screen === 'branding')) for (const panel of row.panels)
      assert.deepEqual(panel.link.foreground.map(Math.round), palettes[panel.dark ? 'dark' : 'light'],
        'Empty-accent preview must match the selected host pack in each theme')
    assert.deepEqual(report.pageErrors || [], [])
    assert.deepEqual(report.consoleErrors || [], [])
  } finally {
    await save(original.payload)
    await page.goto('/auth/profile/settings#interface')
  }
}
