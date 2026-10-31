import assert from 'node:assert/strict'
import { mkdtemp, mkdir, writeFile, rm } from 'node:fs/promises'
import os from 'node:os'
import path from 'node:path'
import { chromium } from 'playwright'

// Real browser page zoom, not CSS zoom, root font sizing or pinch emulation.
// Chromium's extension API also lets the test verify the applied zoom factor.
// https://playwright.dev/docs/chrome-extensions
// https://developer.chrome.com/docs/extensions/reference/api/tabs#method-setZoom
export async function withBrowserZoom(options, run) {
  const temporary = await mkdtemp(path.join(os.tmpdir(), 'goadmin-browser-zoom-'))
  const extension = path.join(temporary, 'extension')
  let context
  try {
    await mkdir(extension)
    await writeFile(path.join(extension, 'manifest.json'), JSON.stringify({
      manifest_version: 3, name: 'GoAdmin isolated acceptance zoom', version: '1.0',
      permissions: ['tabs'], background: { service_worker: 'worker.js' },
    }))
    await writeFile(path.join(extension, 'worker.js'), 'chrome.runtime.onInstalled.addListener(() => {});')
    context = await chromium.launchPersistentContext(path.join(temporary, 'profile'), {
      ...options, channel: 'chromium', headless: true, viewport: null,
      ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH } : {}),
      args: [`--disable-extensions-except=${extension}`, `--load-extension=${extension}`, '--window-size=1440,1000'],
    })
    const worker = context.serviceWorkers()[0] || await context.waitForEvent('serviceworker')
    const zoom = async (page, factor) => {
      const url = page.url()
      const actual = await worker.evaluate(async ({ url, factor }) => {
        const tabs = await chrome.tabs.query({})
        const tab = tabs.find(tab => tab.url === url)
        if (!tab) throw new Error('Zoom target tab missing')
        await chrome.tabs.setZoom(tab.id, factor)
        return chrome.tabs.getZoom(tab.id)
      }, { url, factor })
      assert.equal(actual, factor, 'Chromium must apply the requested browser zoom')
      return actual
    }
    return await run(context, zoom)
  } finally {
    await context?.close()
    await rm(temporary, { recursive: true, force: true })
  }
}
