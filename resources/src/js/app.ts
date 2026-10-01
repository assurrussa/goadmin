import { createApp, h, defineComponent, Fragment, ref } from 'vue'
import { createInertiaApp, router } from '@inertiajs/vue3'
import { createPinia } from 'pinia'
import type { Component, DefineComponent, VNode } from 'vue'
import AdminLayout from '@/components/layout/AdminLayout.vue'
import { useThemeStore } from '@/stores/theme'
import { useAdminProfileStore } from '@/stores/adminProfile'
import { useAuthUserStore } from '@/stores/authUser'
import { useAdminWebSocket } from '@/composables/useAdminWebSocket'
import { createAdminWebSocketPlugin } from '@/plugins/adminWebSocket'
import { hasAdminCapability, setAdminCapabilities } from '@/composables/useAdminCapabilities'
import '@/plugins/theme'
import InertiaFallbackPage from '@/components/system/InertiaFallbackPage.vue'
import AuthFailureNotice from '@/components/system/AuthFailureNotice.vue'
import { createAuthFailureHandler } from '@/auth/browserAuthFailures'
import axios from 'axios'
import type { AdminAuthUser } from '@/types/models'
import { extensions } from '@/extensions'
import { extensionPages } from '@admin-extensions-generated'

axios.defaults.withCredentials = true
axios.defaults.xsrfCookieName = import.meta.env.VITE_ADMIN_CSRF_COOKIE_NAME?.trim() || 'csrf_token'
axios.defaults.xsrfHeaderName = 'X-CSRF-Token'

import '../css/app.css'

type LayoutComponent = Component
type LayoutResolver = (render: typeof h, page: VNode) => VNode
type PageWithLayout = DefineComponent & {
  layout?: LayoutComponent | LayoutComponent[] | LayoutResolver
}
type PageModule = { default: PageWithLayout }
type PageLoader = () => Promise<PageModule>
const renderAdminLayout: LayoutResolver = (render, page) =>
  render(AdminLayout, null, { default: () => page })
const toPageMap = (pages: Record<string, PageLoader>) => {
  const map = new Map<string, PageLoader>()
  Object.entries(pages).forEach(([path, mod]) => {
    const match = path.match(/\/Pages\/(.+)\.vue$/)
    if (!match) {
      return
    }
    map.set(match[1], mod)
  })
  return map
}
const corePages = import.meta.glob('@admin-core/Pages/**/*.vue') as Record<string, PageLoader>
const corePageMap = toPageMap(corePages)
const extPageMap = new Map<string, PageLoader>(
  Object.entries(extensionPages) as [string, PageLoader][],
)

void createInertiaApp({
  progress: {
    delay: 250,
    color: '#29d',
    includeCSS: true,
    showSpinner: false,
  },
  id: 'app',
  resolve: async (name) => {
    const load = extPageMap.get(name) ?? corePageMap.get(name)

    if (load) {
      const mod = await load()
      // Определяем layout в зависимости от пути страницы
      if (!mod.default.layout && !name.startsWith('auth/')) {
        mod.default.layout = renderAdminLayout
      }
      return mod.default
    }

    // Fallback: компонент не найден — отображаем подсказку в UI
    const FallbackWrapper = defineComponent({
      name: 'InertiaFallbackWrapper',
      setup: () => () => h(InertiaFallbackPage, { name }),
    }) as DefineComponent

    // Навсякий случай проверяем, есть ли шаблон нужный, если нет, то будет FallbackWrapper
    ;(FallbackWrapper as PageWithLayout).layout = renderAdminLayout
    return FallbackWrapper
  },
  setup({ el, App, props, plugin }) {
    const pinia = createPinia()

    // Тема инициализируется через inline script в HTML head
    // Здесь только синхронизируемся с уже примененным состоянием

    const authFailureMessage = ref('')
    const stopListeners: Array<() => void> = []
    const app = createApp({
      render: () =>
        h(Fragment, null, [
          h(App, props),
          h(AuthFailureNotice, {
            message: authFailureMessage.value,
            onDismiss: () => {
              authFailureMessage.value = ''
            },
          }),
        ]),
      unmounted: () => stopListeners.forEach((stop) => stop()),
    })
      .use(plugin)
      .use(pinia)

    setAdminCapabilities((props as { initialPage?: unknown }).initialPage)

    if (typeof window !== 'undefined' && hasAdminCapability('realtime')) {
      app.use(
        createAdminWebSocketPlugin({
          handlers: extensions.webSocketHandlers,
        }),
      )
    }
    extensions.install(app)

    // Инициализируем store после создания Pinia, но до монтирования
    if (typeof window !== 'undefined') {
      const themeStore = useThemeStore()
      themeStore.init()

      const profileStore = useAdminProfileStore()
      const authUserStore = useAuthUserStore()
      const websocket = useAdminWebSocket()
      const { ensureConnected, disconnect } = websocket

      let currentAuthId: string | null = null
      let leavingForLogin = false

      const extractAuth = (pageLike: unknown): AdminAuthUser | null => {
        if (typeof pageLike !== 'object' || pageLike === null) {
          return null
        }
        const page = pageLike as { props?: { authuser?: AdminAuthUser | null } }
        return page.props?.authuser ?? null
      }

      const syncAuthState = (auth: AdminAuthUser | null) => {
        const nextId = auth?.id ?? null

        authUserStore.setAuthUser(auth)
        profileStore.setAvatarUrl(auth?.avatarUrl ?? null)

        if (nextId && hasAdminCapability('realtime')) {
          ensureConnected()
        } else if (currentAuthId) {
          disconnect()
        }

        currentAuthId = nextId
      }

      const initialAuth =
        extractAuth((props as { initialPage?: unknown }).initialPage) ??
        extractAuth((router as { page?: unknown }).page)
      syncAuthState(initialAuth)

      stopListeners.push(
        router.on(
          'invalid',
          createAuthFailureHandler({
            origin: window.location.origin,
            clearAuth: () => {
              leavingForLogin = true
              currentAuthId = null
              authUserStore.setAuthUser(null)
              profileStore.setAvatarUrl(null)
            },
            disconnect,
            navigateToLogin: () => window.location.assign('/auth/login'),
            showNotice: (message) => {
              authFailureMessage.value = message
            },
          }),
        ),
        router.on('success', (event) => {
          // A late page response must not restore stale auth while the separate
          // login navigation is in progress.
          if (!leavingForLogin) {
            authFailureMessage.value = ''
            setAdminCapabilities(event.detail?.page)
            syncAuthState(extractAuth(event.detail?.page))
          }
        }),
        router.on('error', () => {
          if (!leavingForLogin) {
            syncAuthState(extractAuth((router as { page?: unknown }).page))
          }
        }),
      )
    }

    // Монтируем приложение после инициализации store
    app.mount(el)
  },
}).catch((error: unknown) => {
  console.error('[goadmin] failed to bootstrap admin client', error)
  const root = document.getElementById('app')
  if (root) {
    root.textContent = 'Не удалось загрузить панель администратора. Обновите страницу.'
    root.setAttribute('role', 'alert')
  }
})
