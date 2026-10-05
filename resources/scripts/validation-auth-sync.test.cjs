const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')
const { test } = require('node:test')
const { JSDOM } = require('jsdom')
const ts = require('typescript')

// Run the actual app entrypoint with the real, locked Vue/Inertia adapter.
// Only unrelated shell/UI services are stubbed. Router deliberately has no page
// property: current page state belongs to the adapter's public usePage API.
const dom = new JSDOM('<div id="app"></div>', { url: 'https://admin.test/content/authors/create' })
for (const name of [
  'window',
  'document',
  'Element',
  'HTMLElement',
  'SVGElement',
  'CustomEvent',
  'Event',
  'Node',
]) {
  global[name] = dom.window[name]
}
global.requestAnimationFrame = (callback) => setTimeout(callback, 0)
global.cancelAnimationFrame = clearTimeout
window.requestAnimationFrame = global.requestAnimationFrame
window.cancelAnimationFrame = global.cancelAnimationFrame
window.scrollTo = () => {}
const vue = require('vue')
test('validation and navigation synchronize auth from the live Inertia page', async (t) => {
  const inertia = await import('@inertiajs/vue3')
  const pinia = require('pinia')

  function compile(filename) {
    const source = fs
      .readFileSync(filename, 'utf8')
      .replaceAll('import.meta.env', '({})')
      .replace("import.meta.glob('@admin-core/Pages/**/*.vue')", '({})')
    const result = ts.transpileModule(source, {
      fileName: filename,
      reportDiagnostics: true,
      compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
    })
    assert.equal(result.diagnostics?.length ?? 0, 0)
    return result.outputText
  }

  const authModule = { exports: {}, URL }
  vm.runInNewContext(
    compile(path.join(__dirname, '../src/js/auth/browserAuthFailures.ts')),
    authModule,
  )
  const calls = { connected: 0, disconnected: 0, login: 0 }
  const authStore = {
    authUser: null,
    setAuthUser(user) {
      this.authUser = user
    },
  }
  const profileStore = {
    avatarUrl: null,
    setAvatarUrl(url) {
      this.avatarUrl = url
    },
  }
  const initialUser = { id: '7', name: 'Fixture admin', avatarUrl: '/avatar-7.png' }
  const initialPage = {
    component: 'auth/fixture',
    url: '/content/authors/create',
    version: 'test',
    props: { authuser: initialUser, errors: {} },
    flash: {},
    clearHistory: false,
    encryptHistory: false,
  }
  document.getElementById('app').dataset.page = JSON.stringify(initialPage)
  let mountedApp
  let bootstrap
  const component = { render: () => vue.h('input', { name: 'draft' }) }
  const modules = {
    vue: {
      ...vue,
      createApp(...args) {
        mountedApp = vue.createApp(...args)
        return mountedApp
      },
    },
    '@inertiajs/vue3': {
      ...inertia,
      createInertiaApp(options) {
        bootstrap = inertia.createInertiaApp({ ...options, progress: false })
        return bootstrap
      },
    },
    pinia,
    '@/components/layout/AdminLayout.vue': { default: component },
    '@/components/system/InertiaFallbackPage.vue': { default: component },
    '@/components/system/AuthFailureNotice.vue': { default: component },
    '@/stores/theme': { useThemeStore: () => ({ init() {} }) },
    '@/stores/adminProfile': { useAdminProfileStore: () => profileStore },
    '@/stores/authUser': { useAuthUserStore: () => authStore },
    '@/composables/useAdminWebSocket': {
      useAdminWebSocket: () => ({
        ensureConnected() {
          calls.connected++
        },
        disconnect() {
          calls.disconnected++
        },
      }),
    },
    '@/plugins/adminWebSocket': { createAdminWebSocketPlugin: () => ({ install() {} }) },
    '@/composables/useAdminCapabilities': {
      hasAdminCapability: () => true,
      setAdminCapabilities() {},
    },
    '@/plugins/theme': {},
    '@/auth/browserAuthFailures': authModule.exports,
    axios: { default: { defaults: {} } },
    '@/extensions': { extensions: { install() {}, webSocketHandlers: {} } },
    '@admin-extensions-generated': { extensionPages: {} },
    '../css/app.css': {},
  }
  const bootstrapClient = () =>
    vm.runInNewContext(compile(path.join(__dirname, '../src/js/app.ts')), {
      exports: {},
      document,
      console,
      window: {
        location: {
          origin: window.location.origin,
          assign() {
            calls.login++
          },
        },
      },
      require(name) {
        assert.ok(Object.hasOwn(modules, name), `unexpected module: ${name}`)
        return modules[name]
      },
    })

  bootstrapClient()

  function dispatch(type, detail) {
    return document.dispatchEvent(new CustomEvent(`inertia:${type}`, { cancelable: true, detail }))
  }

  async function setPage(props) {
    await new Promise((resolve) =>
      inertia.router.replace({
        props,
        preserveState: true,
        preserveScroll: true,
        onFinish: resolve,
      }),
    )
    await vue.nextTick()
  }

  function assertUser(user) {
    assert.equal(authStore.authUser?.id ?? null, user?.id ?? null)
    assert.equal(authStore.authUser?.name ?? null, user?.name ?? null)
    assert.equal(profileStore.avatarUrl, user?.avatarUrl ?? null)
  }

  await bootstrap
  t.after(() => {
    mountedApp.unmount()
    dom.window.close()
  })
  assert.equal('page' in inertia.router, false)

  await t.test('initial server page establishes identity and realtime', () => {
    assertUser(initialUser)
    assert.equal(calls.connected, 1)
    assert.equal(calls.disconnected, 0)
  })

  await t.test(
    'repeated ordinary validation preserves identity, avatar, socket, and draft',
    async () => {
      const input = document.querySelector('input[name=draft]')
      input.value = 'unsaved content'
      await setPage({ authuser: initialUser, errors: { title: 'Required' } })
      for (let i = 0; i < 2; i++) dispatch('error', { errors: { title: 'Required' } })
      assertUser(initialUser)
      assert.equal(calls.disconnected, 0)
      assert.equal(document.querySelector('input[name=draft]'), input)
      assert.equal(input.value, 'unsaved content')
    },
  )

  await t.test(
    'validation uses newly accepted page auth rather than the initial identity',
    async () => {
      const updated = { ...initialUser, name: 'Updated admin', avatarUrl: '/new-avatar.png' }
      await setPage({ authuser: updated, errors: { body: 'Required' } })
      dispatch('error', { errors: { body: 'Required' } })
      assertUser(updated)
      assert.equal(calls.disconnected, 0)
    },
  )

  await t.test(
    'successful navigation updates identity and a signed-out page clears it',
    async () => {
      const user = { id: '8', name: 'Next admin', avatarUrl: '/avatar-8.png' }
      await setPage({ authuser: user, errors: {} })
      dispatch('success', { page: inertia.usePage() })
      assertUser(user)
      await setPage({ authuser: null, errors: {} })
      dispatch('success', { page: inertia.usePage() })
      assertUser(null)
      assert.equal(calls.disconnected, 1)
    },
  )

  await t.test('an explicitly anonymous validation page also clears existing auth', async () => {
    await setPage({ authuser: initialUser, errors: {} })
    dispatch('success', { page: inertia.usePage() })
    await setPage({ authuser: null, errors: { title: 'Required' } })
    dispatch('error', { errors: { title: 'Required' } })
    assertUser(null)
    assert.equal(calls.disconnected, 2)
  })

  await t.test(
    'late validation and success cannot restore auth after a login redirect',
    async () => {
      await setPage({ authuser: initialUser, errors: {} })
      dispatch('success', { page: inertia.usePage() })
      dispatch('invalid', {
        response: {
          status: 401,
          data: { status: 401, code: 'reauthentication_required', message: 'Sign in again' },
          headers: { 'x-goadmin-auth-error': '1', 'content-type': 'application/json' },
          config: { url: '/protected' },
        },
      })
      assertUser(null)
      assert.equal(calls.login, 1)
      assert.equal(calls.disconnected, 3)
      dispatch('error', { errors: { title: 'Required' } })
      dispatch('success', { page: inertia.usePage() })
      assertUser(null)
      assert.equal(calls.login, 1)
      assert.equal(calls.disconnected, 3)
    },
  )
  await t.test(
    'an anonymous remount never falls back to the previous adapter identity',
    async () => {
      assert.equal(inertia.usePage().props.authuser.id, initialUser.id)
      const connections = calls.connected
      mountedApp.unmount()
      document.body.innerHTML = '<div id="app"></div>'
      document.getElementById('app').dataset.page = JSON.stringify({
        ...initialPage,
        props: { authuser: null, errors: {} },
      })
      bootstrapClient()
      await bootstrap
      assertUser(null)
      assert.equal(calls.connected, connections)
    },
  )
})
