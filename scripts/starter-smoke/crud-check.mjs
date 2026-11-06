// Focused acceptance against an owned, already initialized smoke stack.
import assert from 'node:assert/strict'
import { readFile, writeFile } from 'node:fs/promises'
import path from 'node:path'
import { chromium } from 'playwright'
import { expect as baseExpect } from 'playwright/test'
import { crudAcceptance } from './crud.mjs'
const folder = path.resolve(process.argv[2] ?? '')
const manifest = JSON.parse(await readFile(path.join(folder, 'run.json'), 'utf8'))
assert.equal(manifest.owner, 'goadmin-starter-smoke-v1')
const selected = process.argv[3]
if (selected) assert.ok(manifest.cases[selected], 'Unknown smoke case')
const expect = baseExpect.configure({ timeout: 20000 })
const browser = await chromium.launch({ headless: true, ...(process.env.PLAYWRIGHT_EXECUTABLE_PATH ? { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH } : {}) })
const report = { runId: manifest.runId, profile: 'crud-only', cases: {} }
async function language(page, name, code) {
 await page.getByRole('button', {name:/^(Язык|Language)$/}).click()
 await page.getByRole('menuitemradio', {name,exact:true}).click()
 await expect(page.locator('html')).toHaveAttribute('lang',code)
}
try {
 for (const [name,config] of Object.entries(manifest.cases).filter(([name])=>!selected||selected===name)) {
  const credentials=JSON.parse(await readFile(path.join(folder,`${name}.credentials.json`),'utf8'))
  const context=await browser.newContext({baseURL:config.url,viewport:{width:1440,height:1000},locale:'en-GB',colorScheme:'light'})
  const page=await context.newPage()
  const result=report.cases[name]={result:'running',pageErrors:[],consoleErrors:[]}
  page.on('pageerror',error=>result.pageErrors.push(error.message))
  page.on('console',message=>{if(message.type()==='error')result.consoleErrors.push(message.text())})
  try {
   await page.goto('/auth/login')
   await page.locator('input[name=email]').fill(credentials.email)
   await page.locator('input[name=password]').fill(credentials.password)
   await page.locator('form button[type=submit]').click()
   await expect(page.locator('[data-profile-menu-trigger]')).toBeVisible()
   await crudAcceptance({page,context,config,folder,name,expect,language,result})
   assert.deepEqual(result.pageErrors,[])
   assert.deepEqual(result.consoleErrors,[])
   result.result='passed'
   console.log(`PASS CRUD ${name}`)
  } catch(error) {
   result.result='failed'
   result.error=String(error.stack).split(credentials.password).join('[redacted]')
   await page.screenshot({path:path.join(folder,`${name}-crud-failure.png`),fullPage:true}).catch(()=>{})
   throw new Error(`CRUD ${name} failed; inspect private crud-report.json`)
  } finally { await context.close() }
 }
} finally {
 await browser.close()
 await writeFile(path.join(folder,'crud-report.json'),JSON.stringify(report,null,2),{mode:0o600})
}
