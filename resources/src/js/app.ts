// Establish the core layer order before component and extension styles.
import '../css/app.css'
// Install history provenance before Inertia can restore a lazily loaded grid page.
import '@/components/datagrid/historyRestoration'
import { initLocale } from '@/i18n'
import { createApp, h, defineComponent, Fragment, ref } from 'vue'
import { createInertiaApp, router, usePage } from '@inertiajs/vue3'
import { createPinia } from 'pinia'
import { toInertiaPage } from '@/inertiaPage'
import type { LayoutResolver, PageLoader, PageModule, PageWithLayout } from '@/inertiaPage'
import AdminLayout from '@/components/layout/AdminLayout.vue'
import AdminRuntimeFrame from '@/components/layout/AdminRuntimeFrame.vue'
import { installPresentation } from '@/presentation/context'
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
import { recoverBrowserAuthentication } from '@/auth/browserAuthRecovery'
import {
  configureSessionMaintenance,
  sessionCoordinator,
  installSessionAxiosInterceptor,
  reportSessionAuthFailure,
  SessionReadinessError,
} from '@/http'
import axios from 'axios'
import { installSessionNavigationCancellation } from '@/http/sessionNavigation'
import type { AdminAuthUser } from '@/types/models'
import { extensions } from '@/extensions'
import { extensionPages } from '@admin-extensions-generated'

initLocale()

axios.defaults.withCredentials = true
axios.defaults.xsrfCookieName = import.meta.env.VITE_ADMIN_CSRF_COOKIE_NAME?.trim() || 'csrf_token'
axios.defaults.xsrfHeaderName = 'X-CSRF-Token'

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
const corePages = import.meta.glob<PageModule>('@admin-core/Pages/**/*.vue')
const corePageMap = toPageMap(corePages)
const extPageMap = new Map<string, PageLoader>(Object.entries(extensionPages))

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
      return toInertiaPage(mod.default)
    }

    // Fallback: компонент не найден — отображаем подсказку в UI
    const FallbackWrapper: PageWithLayout = defineComponent({
      name: 'InertiaFallbackWrapper',
      setup: () => () => h(InertiaFallbackPage, { name }),
    })

    // Навсякий случай проверяем, есть ли шаблон нужный, если нет, то будет FallbackWrapper
    FallbackWrapper.layout = renderAdminLayout
    return toInertiaPage(FallbackWrapper)
  },
  setup({ el, App, props, plugin }) {
    const pinia = createPinia()

    // Тема инициализируется через inline script в HTML head
    // Здесь только синхронизируемся с уже примененным состоянием

    const authFailureMessage = ref('')
    const authFailureLogin = ref(false)
    const stopListeners: Array<() => void> = []
    let cancelAuthRecovery = () => {}
    const app = createApp({
      render: () =>
        h(Fragment, null, [
          h(AdminRuntimeFrame, null, { default: () => h(App, props) }),
          h(AuthFailureNotice, {
            message: authFailureMessage.value,
            loginRequired: authFailureLogin.value,
            onLogin: () => {
              cancelAuthRecovery()
              window.location.assign('/auth/login')
            },
            onDismiss: () => {
              cancelAuthRecovery()
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
    installPresentation(app, extensions.theme)

    // Инициализируем store после создания Pinia, но до монтирования
    if (typeof window !== 'undefined') {
      const themeStore = useThemeStore()
      themeStore.init()

      const profileStore = useAdminProfileStore()
      const authUserStore = useAuthUserStore()
      const websocket = useAdminWebSocket()
      const { ensureConnected, disconnect } = websocket
      const currentPage = usePage()

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

        if (
          nextId &&
          hasAdminCapability('realtime') &&
          (!sessionCoordinator.isEnabled() || sessionCoordinator.getState() === 'ready')
        ) {
          ensureConnected()
        } else if (currentAuthId) {
          disconnect()
        }

        currentAuthId = nextId
      }

      stopListeners.push(installSessionAxiosInterceptor())
      stopListeners.push(installSessionNavigationCancellation(currentPage))
      let manualRetryAfterRepair = false
      configureSessionMaintenance((props as { initialPage?: unknown }).initialPage, {
        onState: (state) => {
          if (
            state === 'reauthentication_required' ||
            state === 'access_denied' ||
            state === 'session_changed'
          ) {
            authUserStore.setAuthUser(null)
            profileStore.setAvatarUrl(null)
            currentAuthId = null
            disconnect()
            authFailureLogin.value = true
            authFailureMessage.value =
              state === 'session_changed'
                ? 'Сеанс изменился. Данные формы сохранены. Войдите снова перед отправкой.'
                : 'Сеанс завершён или доступ закрыт. Данные формы сохранены. Войдите снова.'
          } else if (state === 'csrf_needs_sync') {
            manualRetryAfterRepair = true
            authFailureMessage.value = 'Восстанавливаем доступ. Данные формы остаются на странице.'
          } else if (state === 'authentication_unavailable') {
            authFailureMessage.value =
              'Сервис авторизации временно недоступен. Данные формы сохранены. Повторите попытку.'
          } else if (state === 'ready') {
            authFailureMessage.value = manualRetryAfterRepair
              ? 'Доступ восстановлен. Данные формы сохранены на странице. Нажмите «Сохранить» ещё раз.'
              : ''
            manualRetryAfterRepair = false
            authFailureLogin.value = false
          }
        },
        onReady: () => {
          if (currentAuthId && hasAdminCapability('realtime')) ensureConnected()
        },
      })
      stopListeners.push(() => sessionCoordinator.dispose())

      const initialAuth = extractAuth((props as { initialPage?: unknown }).initialPage)
      syncAuthState(initialAuth)

      const authFailureHandler = createAuthFailureHandler({
        origin: window.location.origin,
        clearAuth: () => {
          currentAuthId = null
          authUserStore.setAuthUser(null)
          profileStore.setAvatarUrl(null)
        },
        disconnect,
        navigateToLogin: () => {
          leavingForLogin = true
          window.location.assign('/auth/login')
        },
        recoverAuth: recoverBrowserAuthentication,
        resumeNavigation: (url) => router.get(url),
        showLogin: () => {
          authFailureLogin.value = true
        },
        showNotice: (message) => {
          authFailureMessage.value = message
        },
      })
      cancelAuthRecovery = () => {
        authFailureHandler.cancel()
        // Discard command continuations while the shared watchdog keeps running.
        sessionCoordinator.cancel()
      }
      window.addEventListener('pagehide', cancelAuthRecovery)
      stopListeners.push(
        () => window.removeEventListener('pagehide', cancelAuthRecovery),
        () => authFailureHandler.reset(),
        router.on('invalid', (event) => {
          if (!sessionCoordinator.isEnabled()) {
            authFailureHandler(event)
            return
          }
          const response = event.detail.response
          if (
            reportSessionAuthFailure(
              response.status,
              {
                get: (name) =>
                  typeof response.headers.get === 'function'
                    ? response.headers.get(name)
                    : response.headers[name],
              },
              response.data,
              false,
            )
          )
            event.preventDefault()
        }),
        router.on('exception', (event) => {
          if (
            sessionCoordinator.isEnabled() &&
            axios.isAxiosError(event.detail.exception) &&
            !event.detail.exception.response
          ) {
            event.preventDefault()
          }
          if (event.detail.exception instanceof SessionReadinessError) {
            event.preventDefault()
            if (event.detail.exception.code === 'session_changed') {
              authFailureMessage.value = event.detail.exception.message
            }
          }
        }),
        router.on('start', (event) => authFailureHandler.start(event.detail.visit)),
        router.on('finish', (event) => authFailureHandler.finish(event.detail.visit)),
        router.on('navigate', () => authFailureHandler.cancel()),
        router.on('success', (event) => {
          // A late page response must not restore stale auth while the separate
          // login navigation is in progress.
          if (
            !leavingForLogin &&
            (!['reauthentication_required', 'access_denied', 'session_changed'].includes(
              sessionCoordinator.getState(),
            ) ||
              extractAuth(event.detail?.page) === null)
          ) {
            authFailureHandler.reset()
            authFailureMessage.value = ''
            authFailureLogin.value = false
            setAdminCapabilities(event.detail?.page)
            configureSessionMaintenance(event.detail?.page)
            syncAuthState(extractAuth(event.detail?.page))
          }
        }),
        router.on('error', () => {
          if (
            !leavingForLogin &&
            !['reauthentication_required', 'access_denied', 'session_changed'].includes(
              sessionCoordinator.getState(),
            )
          ) {
            syncAuthState(extractAuth(currentPage))
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
