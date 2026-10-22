import { computed, ref, inject, provide } from 'vue'
import type { InjectionKey, Ref } from 'vue'
import { router, usePage } from '@inertiajs/vue3'
import { useAuthUserStore } from '@/stores/authUser'
import type { AdminAuthUser } from '@/types/models'
const headerStateKey: InjectionKey<Ref<boolean>> = Symbol('goadmin.header-preview')
export function provideAdminHeaderState() {
  const existing = inject(headerStateKey, null)
  provide(headerStateKey, existing ?? ref(false))
}
export function useAdminHeader() {
  const authUserStore = useAuthUserStore()
  const authuser = computed<AdminAuthUser | null>(() => authUserStore.authUser)
  const page = usePage<{
    notificationsSummary?: { unread?: number }
    rolePreview?: { available: boolean; active: boolean }
  }>()
  type AccountMenuItem = { name: string; href?: string }
  const accountMenuItems = computed(() => {
    const menu = page.props.adminMenu as
      | { sections?: { placement?: string; items?: AccountMenuItem[] }[] }
      | undefined
    return (menu?.sections ?? [])
      .filter((section) => section.placement === 'account')
      .flatMap((section) => section.items ?? [])
      .filter(
        (item): item is AccountMenuItem & { href: string } =>
          typeof item.href === 'string' &&
          item.href.startsWith('/') &&
          !item.href.startsWith('//') &&
          !/[\\\x00-\x1f\x7f]/.test(item.href),
      )
  })
  const rolePreviewOpen = inject(headerStateKey, ref(false))
  const canPreviewRole = computed(() =>
    Boolean(page.props.rolePreview?.available || page.props.rolePreview?.active),
  )
  function onMenuCloseAutoFocus(event: Event) {
    if (rolePreviewOpen.value) event.preventDefault()
  }

  // Computed
  const avatarUrl = computed(() => authUserStore.avatarUrl)
  const formatRoleLabel = (role?: string): string => {
    if (!role) return ''
    return role
      .split('_')
      .filter(Boolean)
      .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
      .join(' ')
  }
  const primaryRoleLabel = computed(() => {
    const roles = authuser.value?.roles
    if (!roles || roles.length === 0) {
      return ''
    }
    return formatRoleLabel(roles[0])
  })

  const userInitials = computed(() => {
    const name = authuser.value?.name ?? 'U'
    return name
      .split(' ')
      .map((part) => part.charAt(0))
      .join('')
      .toUpperCase()
      .slice(0, 2)
  })

  const unreadNotifications = computed(() => {
    const summary = page.props.notificationsSummary
    if (!summary) return 0
    const value = (summary as { unread?: number }).unread
    return typeof value === 'number' ? value : 0
  })

  // Methods
  const handleLogout = async () => {
    router.delete('/auth/logout')
  }

  return {
    accountMenuItems,
    rolePreviewOpen,
    canPreviewRole,
    onMenuCloseAutoFocus,
    avatarUrl,
    primaryRoleLabel,
    userInitials,
    authuser,
    unreadNotifications,
    handleLogout,
  }
}
