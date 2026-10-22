import { computed } from 'vue'
import type { Component } from 'vue'
import { usePage } from '@inertiajs/vue3'
import {
  Bell,
  BookOpen,
  Home,
  ListTodo,
  ShieldCheck,
  Users,
  FolderKey,
  KeyRound,
  Monitor,
  LifeBuoy,
  Mail,
  RotateCcw,
} from 'lucide-vue-next'
export function useAdminNavigation() {
  const page = usePage()
  // Navigation items
  type MenuIcon = string | Component | undefined

  type MenuBadge = {
    text?: string
    variant?: string
    dot?: boolean
    hideIfZero?: boolean
  }

  type AdminMenuItem = {
    name: string
    href: string
    icon?: MenuIcon
    badge?: MenuBadge
    children?: AdminMenuItem[]
  }

  type AdminMenuSection = {
    placement?: string
    key: string
    title?: string
    order?: number
    items: AdminMenuItem[]
  }

  type AdminMenu = {
    sections: AdminMenuSection[]
  }

  const iconMap: Record<string, Component> = {
    home: Home,
    users: Users,
    admins: Users,
    content: BookOpen,
    roles: ShieldCheck,
    permissions: ShieldCheck,
    notifications: Bell,
    queues: ListTodo,
    projects: FolderKey,
    access: KeyRound,
    sessions: Monitor,
    support: LifeBuoy,
    mail: Mail,
    recovery: RotateCcw,
  }

  const sections = computed(() => {
    const propsMenu = (page.props as { adminMenu?: Partial<AdminMenu> }).adminMenu
    if (!Array.isArray(propsMenu?.sections)) return []
    return propsMenu.sections.filter(
      (section) => section.placement !== 'account' && section.items?.length > 0,
    )
  })

  const resolveIcon = (icon: MenuIcon) => {
    if (!icon) {
      return undefined
    }
    if (typeof icon !== 'string') {
      return icon
    }
    return iconMap[icon]
  }

  // Check if route is active
  const isActive = (href: string) => {
    const currentUrl = page.url.split(/[?#]/)[0] ?? '/'

    if (currentUrl === href) {
      return true
    }

    if (href === '/') {
      return false
    }

    return currentUrl.startsWith(href.replace(/\/$/, '') + '/')
  }

  const isBranchActive = (item: AdminMenuItem): boolean =>
    Boolean(item.href && isActive(item.href)) || Boolean(item.children?.some(isBranchActive))
  return { sections, resolveIcon, isActive, isBranchActive }
}
