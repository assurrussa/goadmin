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
          <aside class="bg-card p-4"><a class="goadmin-selection-fill bg-primary/10 text-primary block p-3">Active navigation</a></aside>
          <main class="bg-background p-4"><button class="goadmin-selection-fill bg-primary/10 text-primary hover:bg-surface-variant p-3">Active tab</button></main>
          <button class="goadmin-primary-fill goadmin-semantic-fill bg-error text-white p-3">Semantic action</button>`
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
      }
    }
  }
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
