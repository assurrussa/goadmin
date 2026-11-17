// Native computed-paint acceptance without a server or database. Use after the
// resource build; both presentation packs share the same branding fill CSS.
import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import { readFile, writeFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import path from 'node:path'
import esbuild from '../../resources/node_modules/esbuild/lib/main.js'
import { chromium } from 'playwright'
import { appearanceGeometry } from './appearance-visual.mjs'

const root = fileURLToPath(new URL('../../', import.meta.url))
const compiled = await esbuild.build({
  entryPoints: [path.join(root, 'resources/src/js/appearance/brand.ts')],
  bundle: true, write: false, format: 'iife', globalName: 'BrandPalette',
})
const css = await readFile(path.join(root, 'public/dist/css/app.css'), 'utf8')
const topbar = await readFile(path.join(root, 'resources/examples/theme-topbar/src/theme.css'), 'utf8')
const browser = await chromium.launch({ headless: true,
  ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH } : {}) })
const report = { result: 'running', browser: await browser.version(),
  cssSHA256: createHash('sha256').update(css).digest('hex'), cases: [], minimumContrast: Infinity }

async function checkResponsiveHost(page) {
  const origin = 'http://goadmin.test'
  const hostCSS = "@media (min-width: 768px) { :root { --color-primary: hotpink; --goadmin-primary-image: linear-gradient(red, blue); } }"
  await page.route(origin + '/**', async route => {
    if (new URL(route.request().url()).pathname === '/theme.css') {
      return route.fulfill({ contentType: 'text/css', body: css + '\n' + hostCSS })
    }
    await route.fulfill({ contentType: 'text/html',
      headers: { 'Content-Security-Policy': "style-src 'self'" },
      body: '<!doctype html><link rel="stylesheet" href="/theme.css"><button class="goadmin-primary-fill">Save</button>' })
  })
  await page.goto(origin)
  await page.addScriptTag({ content: compiled.outputFiles[0].text })
  const button = page.getByRole('button', { name: 'Save' })
  for (const [width, expected] of [[1200, 'rgb(255, 0, 0)'], [390, 'rgb(0, 117, 176)'], [1200, 'rgb(255, 0, 0)']]) {
    await page.setViewportSize({ width, height: 800 })
    await page.evaluate(() => BrandPalette.applyBrand(BrandPalette.emptyBrand, false))
    const image = await button.evaluate(node => getComputedStyle(node).backgroundImage)
    assert.ok(image.includes(expected), `Embedded CSS with style-src 'self' at ${width}px: ${image}`)
  }
}

try {
  const page = await browser.newPage()
  for (const pack of ['default', 'topbar']) {
    await page.setContent('<html><head></head><body></body></html>')
    await page.addStyleTag({ content: css })
    if (pack === 'topbar') await page.addStyleTag({ content: topbar })
    await page.addScriptTag({ content: compiled.outputFiles[0].text })
    await page.evaluate(pack => { document.documentElement.dataset.adminTheme = pack === 'topbar' ? 'example.topbar' : 'default' }, pack)
    for (const dark of [false, true]) for (const [start, end] of [
      ['', ''],
      ['#ff0000', ''], ['#ff0000', '#ff0000'], ['#ff0000', '#00ff00'],
      ['#0000ff', '#ffff00'], ['#00ff00', '#ff00ff'], ['#000000', '#ffffff'], ['#ffffff', '#000000'],
    ]) {
      await page.evaluate(({ dark, start, end }) => {
        const root = document.documentElement
        root.classList.toggle('dark', dark)
        root.dataset.themeMode = dark ? 'dark' : 'light'
        root.style.cssText = ''
        for (const [key, value] of Object.entries(BrandPalette.deriveBrandPalette(start, dark, end))) root.style.setProperty(key, value)
        document.body.innerHTML = `
          <header data-ui="admin-header" class="bg-card p-4"><p data-brand-paint-probe>Header heading</p><a class="goadmin-selection-fill bg-primary/10 text-primary block p-3">Active navigation</a></header>
          <main class="bg-background p-4"><button class="goadmin-selection-fill bg-primary/10 text-primary hover:bg-surface-variant p-3">Active tab</button></main>
          <button class="goadmin-primary-fill goadmin-semantic-fill bg-error text-white p-3">Semantic action</button>
          <div data-row-scroll style="width:360px;overflow:auto"><table style="width:960px;border-collapse:collapse"><tbody>
            ${['plain', 'selected'].map(state => `<tr data-row="${state}" class="goadmin-data-row ${state === 'selected' ? 'goadmin-selected-row' : ''}">
              ${['first', 'middle', 'last'].map((cell, index) => `<td class="bg-card p-3 text-text-primary" style="min-width:${index === 1 ? 800 : 80}px;width:${index === 1 ? 800 : 80}px;${index === 0 ? 'position:sticky;left:0' : index === 2 ? 'position:sticky;right:0' : ''}">${state} ${cell}</td>`).join('')}
            </tr>`).join('')}
          </tbody></table></div>
          <section data-ui="auth-shell" class="bg-background p-4"><p data-brand-paint-probe>Login heading</p></section>`
      }, { dark, start, end })
      for (const hover of [false, true]) {
        if (hover) await page.getByRole('button', { name: 'Active tab', exact: true }).hover()
        else await page.mouse.move(0, 0)
        const geometry = await appearanceGeometry(page)
        assert.equal(geometry.selections.length, 2)
        const minimum = Math.min(...geometry.selections.map(item => item.contrast))
        const identity = `${pack}/${dark ? 'dark' : 'light'}/${start}/${end || 'solid'}/${hover ? 'hover' : 'rest'}`
        report.minimumContrast = Math.min(report.minimumContrast, minimum)
        report.cases.push({ pack, dark, start, end, hover, minimumContrast: minimum,
          surfaces: geometry.selections.map(item => ({ contrast: item.contrast, samples: item.samples })) })
        assert.ok(minimum >= 4.5, `${identity}: composited text contrast ${minimum}`)
        assert.equal(await page.locator('.goadmin-semantic-fill').evaluate(node => getComputedStyle(node).backgroundImage), 'none')
        for (const decoration of geometry.decorations) assert.ok(decoration.contrast >= 4.5, `${identity}: decorative background contrast ${decoration.contrast}`)
        const gradient = (!start && pack === 'default') || (!!start && !!end)
        for (const selector of ['[data-ui="admin-header"]', '[data-ui="auth-shell"]']) {
          assert.equal((await page.locator(selector).evaluate(node => getComputedStyle(node).backgroundImage)) !== 'none', gradient)
        }
        const rowPaint = async state => page.locator(`[data-row="${state}"] td`).evaluateAll(cells => cells.map(cell => ({
          color: getComputedStyle(cell).backgroundColor, image: getComputedStyle(cell).backgroundImage,
        })))
        const assertUniform = paints => { assert.equal(new Set(paints.map(paint => paint.color)).size, 1); assert.ok(paints.every(paint => paint.image === 'none')) }
        const selected = await rowPaint('selected'); assertUniform(selected)
        const plain = await rowPaint('plain'); assertUniform(plain)
        await page.getByText('plain first', { exact: true }).hover()
        const hovered = await rowPaint('plain'); assertUniform(hovered)
        assert.notEqual(hovered[0].color, plain[0].color)
        await page.getByText('selected first', { exact: true }).hover()
        assert.deepEqual(await rowPaint('selected'), selected, 'selection takes priority over hover')
        await page.locator('[data-row-scroll]').evaluate(node => { node.scrollLeft = node.scrollWidth })
        assert.deepEqual(await rowPaint('selected'), selected, 'sticky cells retain the row background after horizontal scroll')
        await page.locator('[data-row-scroll]').evaluate(node => { node.scrollLeft = 0 })
        for (const radius of await page.locator('button').evaluateAll(nodes => nodes.map(node => getComputedStyle(node).borderRadius))) assert.equal(radius, '4px')
      }
    }
  }
  await checkResponsiveHost(page)
  report.responsiveHostUnderCSP = true
  report.result = 'passed'
  console.log(`PASS native selection contrast: ${report.cases.length} cases, minimum ${report.minimumContrast.toFixed(3)}:1`)
} catch (error) {
  report.result = 'failed'
  report.error = String(error)
  throw error
} finally {
  await browser.close()
  if (process.argv[2]) await writeFile(path.resolve(process.argv[2]), JSON.stringify(report, null, 2) + '\n', { mode: 0o600 })
}
