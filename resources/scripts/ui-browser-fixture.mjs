// UI-only fixture: real Vue layout with synthetic Inertia props; no authentication server.
import { createServer } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const themeImports = process.env.GOADMIN_FIXTURE_BRANDING === '1'
  ? "import '/examples/theme-compact/src/extensions/theme.ts';import '/examples/theme-brand/src/extensions/theme.ts';"
  : ''
const entry = `${themeImports}import {createApp,h,ref} from 'vue';
import {createInertiaApp} from '@inertiajs/vue3';import {createPinia} from 'pinia';
import Button from '/src/js/components/ui/button/Button.vue';
import Input from '/src/js/components/ui/input/Input.vue';
import AdminLayout from '/src/js/components/layout/AdminLayout.vue';
import {initLocale} from '/src/js/i18n/index.ts';
import {setAdminCapabilities} from '/src/js/composables/useAdminCapabilities.ts';
import '/src/css/app.css';
const page={component:'UIFixture',url:'/__ui',version:'fixture',props:{adminCapabilities:{},adminInfo:{version:'UI fixture'},authuser:null,flash:{},adminMenu:{sections:[{key:'main',title:'Управление',items:[{name:'Пользователи',href:'/users',icon:'users',children:[{name:'Роли',href:'/__ui',icon:'roles'}]}]}]}}};
initLocale();setAdminCapabilities(page);
const Fixture={setup(){const text=ref('Synthetic input');return()=>h(AdminLayout,null,{default:()=>[h('h1','UI-only synthetic fixture'),h(Input,{'aria-label':'Synthetic text',modelValue:text.value,'onUpdate:modelValue':v=>text.value=v}),h(Button,{id:'brand-primary'},()=> 'Primary action')]})}};
createInertiaApp({page,resolve:()=>Fixture,setup({el,App,props,plugin}){createApp({render:()=>h(App,props)}).use(plugin).use(createPinia()).mount(el)}});`
const server = await createServer({
  configFile: false,
  root,
  plugins: [
    vue(),
    tailwindcss(),
    {
      name: 'synthetic-ui',
      resolveId(id) {
        if (id === '/__ui-entry.js') return id
      },
      load(id) {
        if (id === '/__ui-entry.js') return entry
      },
      configureServer(s) {
        s.middlewares.use((req, res, next) => {
          if (req.url?.startsWith('/__ui-entry.js')) {
            next()
          } else if (req.url?.startsWith('/__ui')) {
            res.setHeader('Content-Type', 'text/html')
            res.end(
              '<!doctype html><html><meta name="viewport" content="width=device-width,initial-scale=1"><title>UI fixture</title><body><div id="app"></div><script type="module" src="/__ui-entry.js"></script></body></html>',
            )
          } else next()
        })
      },
    },
  ],
  resolve: { alias: { '@': path.join(root, 'src/js'), '~': path.join(root, 'src/js') } },
  server: { host: '127.0.0.1', port: 8766, strictPort: true },
})
await server.listen()
console.log('UI-only fixture: http://127.0.0.1:8766/__ui')
