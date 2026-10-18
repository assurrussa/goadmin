// Separate UI-only Vite fixture; never imported by the embedded library build.
import { createServer } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'
import path from 'node:path'
import { createRequire } from 'node:module'
const require = createRequire(import.meta.url)
const { materializeAdminExtensions } = require('../../scripts/materialize-admin-extensions.cjs')
materializeAdminExtensions({
  generatedRoot: path.resolve(
    path.dirname(fileURLToPath(import.meta.url)),
    '../../.admin-extensions',
  ),
  manifestPaths: [],
  legacyRoot: '',
  includeLegacy: false,
})
import { fileURLToPath } from 'node:url'
import { demoPage } from './data.mjs'
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..')
const entry = `import {createApp,h} from 'vue';import {createInertiaApp} from '@inertiajs/vue3';import {createPinia} from 'pinia';import {useAuthUserStore} from '/src/js/stores/authUser.ts';import Demo from '/demo/sports-club/SportsClubDemo.vue';import {initLocale,setLocale} from '/src/js/i18n/index.ts';import {setAdminCapabilities} from '/src/js/composables/useAdminCapabilities.ts';import '/src/css/app.css';const page=window.__DEMO_PAGE__;initLocale();setLocale('en');setAdminCapabilities(page);await createInertiaApp({page,resolve:()=>Demo,setup({el,App,props,plugin}){const pinia=createPinia();const app=createApp({render:()=>h(App,props)}).use(plugin).use(pinia);useAuthUserStore(pinia).setAuthUser(page.props.authuser);app.mount(el)}});`
const server = await createServer({
  configFile: false,
  root,
  plugins: [
    vue(),
    tailwindcss(),
    {
      name: 'sports-club-demo',
      resolveId(id) {
        if (id === '/__club-entry.js') return id
      },
      load(id) {
        if (id === '/__club-entry.js') return entry
      },
      configureServer(s) {
        s.middlewares.use((req, res, next) => {
          if (!req.url?.startsWith('/demo/')) return next()
          // Vite owns the source assets under /demo/sports-club/.
          if (req.url.startsWith('/demo/sports-club/')) return next()
          const route = new URL(req.url, 'http://127.0.0.1')
          if (req.headers['x-inertia'] && !route.search) {
            const previous = new URL(req.headers.referer || '/', 'http://127.0.0.1')
            route.search = previous.search
          }
          const page = demoPage(route.pathname + route.search)
          if (!page) {
            res.statusCode = 404
            res.end('Unknown demo screen')
            return
          }
          res.setHeader('Cache-Control', 'no-store')
          if (req.headers['x-inertia']) {
            res.setHeader('Content-Type', 'application/json')
            res.setHeader('X-Inertia', 'true')
            res.setHeader('Vary', 'X-Inertia')
            res.end(JSON.stringify(page))
            return
          }
          const preset = page.props.preset
          const styles =
            (preset !== 'default'
              ? '<link rel="stylesheet" href="/examples/theme-compact/src/theme.css">'
              : '') +
            (preset === 'branded'
              ? '<link rel="stylesheet" href="/examples/theme-brand/src/theme.css">'
              : '')
          res.setHeader('Content-Type', 'text/html; charset=utf-8')
          res.end(
            `<!doctype html><html lang="en" class="${page.props.mode === 'dark' ? 'dark' : ''}" data-theme-mode="${page.props.mode}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><link rel="icon" href="data:,"><title>Northline · GoAdmin demo</title>${styles}</head><body><div id="app"></div><script>window.__DEMO_PAGE__=${JSON.stringify(page).replaceAll('<', '\\u003c')};localStorage.setItem('theme-mode',${JSON.stringify(page.props.mode)});</script><script type="module" src="/__club-entry.js"></script></body></html>`,
          )
        })
      },
    },
  ],
  resolve: { alias: { '@': path.join(root, 'src/js'), '~': path.join(root, 'src/js') } },
  server: { host: '127.0.0.1', port: 8767, strictPort: true },
})
await server.listen()
console.log('Sports club demo: http://127.0.0.1:8767/demo/dashboard')
for (const signal of ['SIGINT', 'SIGTERM'])
  process.on(signal, async () => {
    await server.close()
    process.exit(0)
  })
