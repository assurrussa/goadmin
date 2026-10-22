// UI-only audit fixture: real components, synthetic data, no authenticated backend.
import { createServer } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { readFileSync, writeFileSync } from 'node:fs'
import { execFileSync } from 'node:child_process'
import { homedir } from 'node:os'
import { createDemoScaffold } from '../../demo/sports-club/scaffold.mjs'
import { admins, gridData, gridRoutes, makePage } from './data.mjs'

const directory = path.dirname(fileURLToPath(import.meta.url))
const root = path.resolve(directory, '../..')
const sourceRoot = path.resolve(process.env.GOADMIN_DESIGN_SOURCE_ROOT || root)
const baseline = sourceRoot !== root
const port = Number(process.env.GOADMIN_DESIGN_PORT || 8768)
if (!Number.isInteger(port) || port < 1 || port > 65535)
  throw new Error('Invalid GOADMIN_DESIGN_PORT')
const scaffold = createDemoScaffold()
process.once('exit', scaffold.cleanup)

// Reuse the actual canonical inline initializer without persisting an unset preference.
const appTemplate = readFileSync(path.resolve(sourceRoot, '../views/app.gohtml'), 'utf8')
const themeScript = [...appTemplate.matchAll(/<script>([\s\S]*?)<\/script>/g)]
  .map((match) => match[1])
  .find((script) => script.includes("localStorage.getItem('theme-mode')"))
if (!themeScript) throw new Error('Canonical inline theme initializer not found')

// Go's html/template owns all conditional error copy and escaping. The helper is
// copied into the owned temporary scaffold so no Go package enters the SDK tree.
const renderer = path.join(scaffold.directory, 'render-error.go')
writeFileSync(renderer, readFileSync(path.join(directory, 'render-error.go.txt')))
const sharedCache =
  process.env.GO_SHARED_CACHE_ROOT || path.join(homedir(), 'dev/projects/.cache/go')
const errorPages = JSON.parse(
  execFileSync('go', ['run', renderer, path.resolve(sourceRoot, '../views/error.gohtml')], {
    cwd: root,
    env: {
      ...process.env,
      GOCACHE: process.env.GOCACHE || path.join(sharedCache, 'build'),
      GOMODCACHE: process.env.GOMODCACHE || path.join(sharedCache, 'mod'),
    },
    encoding: 'utf8',
    timeout: 120000,
  }),
)

const pageModules = {
  'dashboard/IndexPage': '/src/js/Pages/dashboard/IndexPage.vue',
  'users/IndexPage': '/src/js/Pages/users/IndexPage.vue',
  'users/EditPage': '/src/js/Pages/users/EditPage.vue',
  'auth/LoginPage': '/src/js/Pages/auth/LoginPage.vue',
  'permissions/IndexPage': '/src/js/Pages/permissions/IndexPage.vue',
  'roles/IndexPage': '/src/js/Pages/roles/IndexPage.vue',
  'roles/ViewPage': '/src/js/Pages/roles/ViewPage.vue',
  'roles/EditPage': '/src/js/Pages/roles/EditPage.vue',
  'profile/ViewPage': '/src/js/Pages/profile/ViewPage.vue',
  'profile/EditPage': '/src/js/Pages/profile/EditPage.vue',
  'profile/SettingsPage': '/src/js/Pages/profile/SettingsPage.vue',
  'notifications/IndexPage': '/src/js/Pages/notifications/IndexPage.vue',
  'queues/IndexPage': '/src/js/Pages/queues/IndexPage.vue',
  'queues/ActivePage': '/src/js/Pages/queues/ActivePage.vue',
  'queues/FailedPage': '/src/js/Pages/queues/FailedPage.vue',
  DesignDialog: '/audit/design/DialogPreview.vue',
  ...(!baseline ? { DesignComponents: '/audit/design/ComponentsPreview.vue' } : {}),
}
const imports = Object.values(pageModules)
  .map((file, index) => `import Page${index} from ${JSON.stringify(file)};`)
  .join('\n')
const components = Object.keys(pageModules)
  .map((name, index) => `${JSON.stringify(name)}:Page${index}`)
  .join(',')
const entry = `${imports}
import {createApp,h} from 'vue';import {createInertiaApp} from '@inertiajs/vue3';import {createPinia} from 'pinia';
import AdminLayout from '/src/js/components/layout/AdminLayout.vue';import {toInertiaPage} from '/src/js/inertiaPage.ts';
import {useAuthUserStore} from '/src/js/stores/authUser.ts';import {useThemeStore} from '/src/js/stores/theme.ts';
import {initLocale} from '/src/js/i18n/index.ts';import {setAdminCapabilities} from '/src/js/composables/useAdminCapabilities.ts';import '/src/css/app.css';
const page=window.__DESIGN_PAGE__;initLocale();setAdminCapabilities(page);const pages={${components}};
for(const [name,component] of Object.entries(pages)){if(!name.startsWith('auth/'))component.layout=(render,page)=>render(AdminLayout,null,{default:()=>page})}
await createInertiaApp({page,resolve:(name)=>toInertiaPage(pages[name]),setup({el,App,props,plugin}){const pinia=createPinia();const app=createApp({render:()=>h(App,props)}).use(plugin).use(pinia);useAuthUserStore(pinia).setAuthUser(page.props.authuser);useThemeStore(pinia).init();app.mount(el)}});`
const requests = new Map()
const json = (res, data, status = 200) => {
  res.statusCode = status
  res.setHeader('Content-Type', 'application/json')
  res.end(JSON.stringify(data))
}
const server = await createServer({
  configFile: false,
  root,
  cacheDir: path.join(scaffold.directory, '.vite'),
  optimizeDeps: {
    // The virtual entry is the only application. Never scan unrelated current
    // audit HTML while comparing a historical source root.
    entries: [],
    noDiscovery: true,
    include: [
      'vue',
      '@inertiajs/vue3',
      'pinia',
      'reka-ui',
      'lucide-vue-next',
      '@vueuse/core',
      '@heroicons/vue/24/outline',
      'class-variance-authority',
      'clsx',
      'tailwind-merge',
      'cropperjs',
      'axios',
      'qrcode',
      'uuid',
      '@tiptap/core',
      '@tiptap/vue-3',
      '@tiptap/starter-kit',
      '@tiptap/extension-underline',
      '@tiptap/extension-link',
      '@tiptap/extension-image',
      '@tiptap/extension-task-list',
      '@tiptap/extension-task-item',
      '@tiptap/extension-character-count',
      '@tiptap/extension-table',
      '@tiptap/extension-table-cell',
      '@tiptap/extension-table-header',
      '@tiptap/extension-table-row',
    ],
  },
  plugins: [
    {
      name: 'goadmin-design-baseline-utilities',
      enforce: 'pre',
      transform(code, id) {
        if (baseline && id.split('?')[0] === path.join(sourceRoot, 'src/css/app.css')) {
          return `${code}\n@source ${JSON.stringify(path.join(sourceRoot, 'src'))};\n`
        }
      },
    },
    vue(),
    tailwindcss(),
    {
      name: 'goadmin-design-audit',
      resolveId(id) {
        if (id === '/__design-entry.js') return id
      },
      load(id) {
        if (id === '/__design-entry.js') return entry
      },
      configureServer(s) {
        s.middlewares.use(async (req, res, next) => {
          const url = new URL(req.url, 'http://127.0.0.1')
          const { pathname, searchParams } = url
          // Read-only fixture: mutations never reach Vite or a real backend.
          if (req.method !== 'GET' && req.method !== 'HEAD') {
            return json(
              res,
              { error: 'Read-only synthetic design fixture: mutations are disabled' },
              405,
            )
          }
          res.setHeader('Cache-Control', 'no-store')
          if (pathname === '/public/favicon.ico' || pathname === '/favicon.ico') {
            res.statusCode = 204
            res.end()
            return
          }
          if (pathname === '/__design/theme') {
            res.setHeader('Content-Type', 'text/html; charset=utf-8')
            res.end(
              `<!doctype html><html lang="ru"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Fixture theme preference</title><script>${themeScript}</script></head><body><h1>Fixture theme preference</h1><p>Synthetic visual audit only. These buttons set the existing local theme preference.</p><button id="light">Manual light</button> <button id="dark">Manual dark</button> <button id="system">System preference (unset)</button><p id="state"></p><a href="/">Dashboard</a> · <a href="/users">Users</a> · <a href="/__design/error/404">Standalone error</a><script type="module">import {themeManager} from '/src/js/plugins/theme.ts';const show=()=>document.querySelector('#state').textContent='Saved: '+(localStorage.getItem('theme-mode')??'unset')+'; applied: '+themeManager.getCurrent();document.querySelector('#light').onclick=()=>{themeManager.setLight();show()};document.querySelector('#dark').onclick=()=>{themeManager.setDark();show()};document.querySelector('#system').onclick=()=>{localStorage.removeItem('theme-mode');location.reload()};show();</script></body></html>`,
            )
            return
          }
          const errorCode = pathname.match(/^\/__design\/error\/(\d+)$/)?.[1]
          if (errorCode && errorPages[errorCode]) {
            res.statusCode = Number(errorCode)
            res.setHeader('Content-Type', 'text/html; charset=utf-8')
            res.end(errorPages[errorCode])
            return
          }
          if (pathname === '/roles/1/admins' || pathname === '/roles/1/admins/search') {
            const search = (searchParams.get('search') || '').toLowerCase()
            return json(res, {
              admins: admins
                .filter((admin) => Object.values(admin).join(' ').toLowerCase().includes(search))
                .map((admin) => ({ ...admin, roles: ['admin'], hasRole: true })),
            })
          }
          const gridRoute = pathname.endsWith('/data') ? pathname.slice(0, -5) : null
          if (gridRoutes.includes(gridRoute)) {
            const scenario = searchParams.get('scenario') || ''
            const key = `${gridRoute}:${scenario}`
            const count = (requests.get(key) || 0) + 1
            requests.set(key, count)
            if (gridRoute === '/users' && scenario === 'loading' && count === 1)
              await new Promise((resolve) => setTimeout(resolve, 12000))
            if (
              gridRoute === '/users' &&
              (scenario === 'error' || (scenario === 'recovery' && count === 1))
            )
              return json(res, { error: 'Synthetic data endpoint failure' }, 500)
            return json(res, gridData(gridRoute, req.url))
          }
          if (baseline && pathname === '/__design/components') {
            res.statusCode = 404
            res.end('Component fixture unavailable for baseline source')
            return
          }
          const page = makePage(req.url)
          if (!page) return next()
          // The actual header reads this page prop; it performs no count GET.
          page.props.notificationsSummary = { unread: 2 }
          if (!req.headers['x-inertia'] && !req.headers['accept']?.includes('application/json'))
            requests.set(`/users:${searchParams.get('scenario') || ''}`, 0)
          if (req.headers['x-inertia'] || req.headers['accept']?.includes('application/json')) {
            if (req.headers['x-inertia']) res.setHeader('X-Inertia', 'true')
            return json(res, req.headers['x-inertia'] ? page : page.props.data)
          }
          res.setHeader('Content-Type', 'text/html; charset=utf-8')
          res.end(
            `<!doctype html><html lang="ru"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><link rel="icon" href="data:,"><title>GoAdmin · synthetic visual audit</title><script>${themeScript}</script></head><body><div id="app"></div><script>window.__DESIGN_PAGE__=${JSON.stringify(page).replaceAll('<', '\\u003c')}</script><script type="module" src="/__design-entry.js"></script></body></html>`,
          )
        })
      },
    },
  ],
  resolve: {
    dedupe: ['vue'],
    alias: [
      ...scaffold.aliases,
      { find: /^\/src\//, replacement: `${path.join(sourceRoot, 'src')}/` },
      { find: '@', replacement: path.join(sourceRoot, 'src/js') },
      { find: '~', replacement: path.join(sourceRoot, 'src/js') },
    ],
  },
  server: {
    host: '127.0.0.1',
    port,
    strictPort: true,
    fs: { allow: [root, sourceRoot, scaffold.directory] },
  },
})
try {
  await server.listen()
  console.log(
    `Design audit (${baseline ? 'baseline source' : 'current source'}): http://127.0.0.1:${port}/users; theme controls /__design/theme`,
  )
} catch (error) {
  await server.close()
  scaffold.cleanup()
  throw error
}
for (const signal of ['SIGINT', 'SIGTERM'])
  process.on(signal, async () => {
    await server.close()
    scaffold.cleanup()
    process.exit(0)
  })
