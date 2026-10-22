// UI-only audit fixture: real components, synthetic data, no authenticated backend.
import { createServer } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { createDemoScaffold } from '../../demo/sports-club/scaffold.mjs'
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..')
const scaffold = createDemoScaffold()
process.once('exit', scaffold.cleanup)
const names = [
  ['Анна', 'Волкова', 'anna.volkova'],
  ['Михаил', 'Орлов', 'mikhail.orlov'],
  ['Елена', 'Соколова', 'elena.sokolova'],
  ['Дмитрий', 'Лебедев', 'dmitry.lebedev'],
  ['Мария', 'Козлова', 'maria.kozlova'],
  ['Алексей', 'Новиков', 'alexey.novikov'],
  ['Ольга', 'Морозова', 'olga.morozova'],
  ['Иван', 'Петров', 'ivan.petrov'],
]
const rows = names.map(([name, lastName, email], i) => ({
  item: {
    id: 1048 - i,
    name: `${name} ${lastName}`,
    email: `${email}@example.invalid`,
    status: i === 3 ? 'inactive' : 'active',
    createdAt: `2026-10-${String(21 - i).padStart(2, '0')}`,
  },
  actions: [
    { key: 'view', label: 'Посмотреть', icon: 'eye' },
    { key: 'edit', label: 'Редактировать', icon: 'edit' },
  ],
}))
const columns = [
  { key: 'id', label: 'ID', title: 'ID', type: 'number', sortable: true },
  { key: 'name', label: 'Пользователь', title: 'Пользователь', sortable: true },
  { key: 'email', label: 'Email', title: 'Email' },
  {
    key: 'status',
    label: 'Статус',
    title: 'Статус',
    type: 'select',
    filterable: true,
    options: [
      { value: 'active', label: 'Активен' },
      { value: 'inactive', label: 'Неактивен' },
    ],
    badges: {
      active: { label: 'Активен', variant: 'success' },
      inactive: { label: 'Неактивен', variant: 'secondary' },
    },
  },
  { key: 'createdAt', label: 'Регистрация', title: 'Регистрация', type: 'date', sortable: true },
]
const menu = [
  { name: 'Главная', href: '/', icon: 'home' },
  { name: 'Пользователи', href: '/users', icon: 'users' },
  { name: 'Администраторы', href: '/admins', icon: 'users' },
  { name: 'Роли', href: '/roles', icon: 'roles' },
  { name: 'Права доступа', href: '/permissions', icon: 'permissions' },
  { name: 'Очереди', href: '/queues', icon: 'queues' },
  { name: 'Уведомления', href: '/notifications', icon: 'notifications' },
]
function makePage(url) {
  const pathname = new URL(url, 'http://127.0.0.1').pathname
  const query = new URL(url, 'http://127.0.0.1').searchParams
  const search = query.get('search') || ''
  const status = query.get('status') || ''
  const filtered = rows.filter(
    (row) =>
      (!status || row.item.status === status) &&
      (!search ||
        `${row.item.name} ${row.item.email}`.toLowerCase().includes(search.toLowerCase())),
  )
  const page = {
    component: 'users/IndexPage',
    url,
    version: 'design-audit',
    props: {
      title: 'Пользователи',
      adminCapabilities: {},
      adminInfo: { version: '0.13.0 · демо' },
      flash: {},
      authuser: {
        id: '00000000-0000-4000-8000-000000000001',
        name: 'Демо',
        lastName: 'Администратор',
        email: 'admin@example.invalid',
        roles: ['admin'],
        avatarUrl: '',
      },
      adminBreadcrumbs: [{ name: 'Главная', href: '/' }, { name: 'Пользователи' }],
      adminMenu: { sections: [{ key: 'main', title: 'Управление', items: menu }] },
      data: {
        data: filtered,
        config: {
          routePath: '/users',
          columns,
          behaviour: {
            searchable: true,
            creatable: true,
            selectable: true,
            refreshable: true,
            exportable: true,
          },
          ui: { createButtonText: 'Создать пользователя', idKey: 'id' },
        },
        meta: {
          title: 'Пользователи',
          description: 'Учётные записи и доступ к приложению',
          requestQuery: new URL(url, 'http://127.0.0.1').search,
          filters: status ? { status } : {},
          pagination: {
            currentPage: 1,
            perPage: 20,
            total: filtered.length,
            totalPages: 1,
            from: filtered.length ? 1 : 0,
            to: filtered.length,
            links: [{ url: null, label: '1', active: true }],
          },
        },
      },
    },
  }
  if (pathname === '/auth/login') {
    page.component = 'auth/LoginPage'
    page.props.title = 'Вход в систему'
    page.props.authuser = null
  } else if (pathname === '/users/1048/edit' || pathname === '/users/create') {
    page.component = 'users/EditPage'
    page.props.title = 'Редактирование пользователя'
    page.props.data = {
      id: 1048,
      name: 'Анна',
      lastName: 'Волкова',
      username: 'anna.volkova',
      email: 'anna.volkova@example.invalid',
    }
    page.props.old = {}
    page.props.adminBreadcrumbs.push({ name: 'Редактирование' })
  } else if (pathname === '/__design/components') {
    page.component = 'DesignComponents'
    page.props.title = 'Компоненты'
    page.props.adminBreadcrumbs = [{ name: 'Главная', href: '/' }, { name: 'Компоненты' }]
  }
  page.props.errors = {}
  return page
}
const entry = `import {createApp,h} from 'vue';import {createInertiaApp} from '@inertiajs/vue3';import {createPinia} from 'pinia';import Users from '/src/js/Pages/users/IndexPage.vue';import Edit from '/src/js/Pages/users/EditPage.vue';import Login from '/src/js/Pages/auth/LoginPage.vue';import Components from '/audit/design/ComponentsPreview.vue';import AdminLayout from '/src/js/components/layout/AdminLayout.vue';import {toInertiaPage} from '/src/js/inertiaPage.ts';import {useAuthUserStore} from '/src/js/stores/authUser.ts';import {useThemeStore} from '/src/js/stores/theme.ts';import {initLocale} from '/src/js/i18n/index.ts';import {setAdminCapabilities} from '/src/js/composables/useAdminCapabilities.ts';import '/src/css/app.css';const page=window.__DESIGN_PAGE__;initLocale();setAdminCapabilities(page);const pages={'users/IndexPage':Users,'users/EditPage':Edit,'auth/LoginPage':Login,'DesignComponents':Components};for(const [name,component] of Object.entries(pages)){if(!name.startsWith('auth/'))component.layout=(render,page)=>render(AdminLayout,null,{default:()=>page})}await createInertiaApp({page,resolve:(name)=>toInertiaPage(pages[name]),setup({el,App,props,plugin}){const pinia=createPinia();const app=createApp({render:()=>h(App,props)}).use(plugin).use(pinia);useAuthUserStore(pinia).setAuthUser(page.props.authuser);useThemeStore(pinia).init();app.mount(el)}});`
const server = await createServer({
  configFile: false,
  root,
  cacheDir: path.join(scaffold.directory, '.vite'),
  optimizeDeps: {
    include: ['vue', '@inertiajs/vue3', 'pinia', 'reka-ui', 'lucide-vue-next', '@vueuse/core'],
  },
  plugins: [
    vue(),
    tailwindcss(),
    {
      name: 'goadmin-design-baseline',
      resolveId(id) {
        if (id === '/__design-entry.js') return id
      },
      load(id) {
        if (id === '/__design-entry.js') return entry
      },
      configureServer(s) {
        s.middlewares.use((req, res, next) => {
          if (
            ![
              '/',
              '/users',
              '/users/data',
              '/users/create',
              '/users/1048/edit',
              '/auth/login',
              '/__design/components',
            ].includes(new URL(req.url, 'http://127.0.0.1').pathname)
          )
            return next()
          if (req.method !== 'GET') {
            res.statusCode = 405
            res.end('Read-only synthetic design fixture')
            return
          }
          const page = makePage(req.url)
          res.setHeader('Cache-Control', 'no-store')
          if (
            new URL(req.url, 'http://127.0.0.1').pathname === '/users/data' ||
            req.headers['x-inertia'] ||
            req.headers['accept']?.includes('application/json')
          ) {
            res.setHeader('Content-Type', 'application/json')
            if (req.headers['x-inertia']) res.setHeader('X-Inertia', 'true')
            res.end(JSON.stringify(req.headers['x-inertia'] ? page : page.props.data))
            return
          }
          res.setHeader('Content-Type', 'text/html; charset=utf-8')
          res.end(
            `<!doctype html><html lang="ru"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><link rel="icon" href="data:,"><title>GoAdmin · проверка дизайна</title><script>const mode=localStorage.getItem('theme-mode')||'light';localStorage.setItem('theme-mode',mode);document.documentElement.classList.toggle('dark',mode==='dark');document.documentElement.setAttribute('data-theme-mode',mode);</script></head><body><div id="app"></div><script>window.__DESIGN_PAGE__=${JSON.stringify(page).replaceAll('<', '\\u003c')}</script><script type="module" src="/__design-entry.js"></script></body></html>`,
          )
        })
      },
    },
  ],
  resolve: {
    dedupe: ['vue'],
    alias: [
      ...scaffold.aliases,
      { find: '@', replacement: path.join(root, 'src/js') },
      { find: '~', replacement: path.join(root, 'src/js') },
    ],
  },
  server: { host: '127.0.0.1', port: 8768, strictPort: true },
})
try {
  await server.listen()
  console.log('Design audit: http://127.0.0.1:8768/users')
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
