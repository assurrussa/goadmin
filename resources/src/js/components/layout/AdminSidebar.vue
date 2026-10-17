<template>
  <div class="flex flex-col h-full">
    <!-- Logo -->
    <div
      class="flex h-[68px] shrink-0 items-center gap-2 px-3 border-b border-border-primary"
      :class="collapsed ? 'justify-center' : 'justify-between'"
    >
      <div :class="collapsed ? 'hidden' : 'flex min-w-0 items-center space-x-3 whitespace-nowrap'">
        <div
          class="h-9 w-9 shrink-0 rounded-xl bg-primary/15 border border-primary/20 flex items-center justify-center"
        >
          <span class="text-primary-dark dark:text-primary-light font-bold text-sm tracking-wide"
            >A</span
          >
        </div>
        <div :class="collapsed ? 'sr-only' : ''">
          <p class="text-sm font-semibold text-text-primary uppercase tracking-wider">
            {{ t('Админка') }}
          </p>
          <p class="text-xs text-text-tertiary">{{ t('Панель управления') }}</p>
        </div>
      </div>

      <AppButton
        variant="ghost"
        size="icon-sm"
        class="hidden lg:inline-flex"
        :aria-label="collapsed ? t('Развернуть') : t('Свернуть')"
        :title="collapsed ? t('Развернуть') : t('Свернуть')"
        :aria-expanded="!collapsed"
        @click="$emit('toggle-collapse')"
        ><PanelLeft class="h-5 w-5"
      /></AppButton>
      <!-- Close button for mobile -->
      <AppButton
        variant="ghost"
        size="icon-sm"
        class="lg:hidden"
        :aria-label="t('Закрыть меню')"
        @click="$emit('close')"
      >
        <X class="h-5 w-5" />
      </AppButton>
    </div>

    <!-- Navigation -->
    <nav class="flex-1 min-h-0 px-0 py-3 space-y-1 overflow-y-auto overflow-x-hidden">
      <template v-for="(section, index) in sections" :key="section.key">
        <div class="space-y-1 px-2">
          <div v-if="section.title" :class="collapsed ? 'sr-only' : 'px-2 py-2'">
            <h3 class="text-[11px] font-semibold text-text-tertiary uppercase tracking-widest">
              {{ t(section.title) }}
            </h3>
          </div>
          <SidebarMenuItems
            :items="section.items"
            :depth="0"
            :collapsed="collapsed"
            :is-active="isActive"
            :resolve-icon="resolveIcon"
          />
        </div>

        <div
          v-if="index < sections.length - 1"
          class="border-t border-border-primary"
          :class="collapsed ? 'my-2' : 'my-4'"
        ></div>
      </template>
    </nav>

    <!-- Footer -->
    <footer :class="collapsed ? 'sr-only' : 'shrink-0 border-t border-border-primary p-4'">
      <p class="text-xs font-medium text-text-secondary">{{ t('Об админке') }}</p>
      <p class="mt-1 text-xs text-text-tertiary break-words">
        {{ t('GoAdmin · версия') }} {{ adminVersion }}
      </p>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { t } from '@/i18n'

// Computed и другие импорты
import { computed } from 'vue'
import { usePage } from '@inertiajs/vue3'
import AppButton from '@/components/ui/AppButton.vue'
import {
  Bell,
  BookOpen,
  Home,
  ListTodo,
  ShieldCheck,
  Users,
  X,
  PanelLeft,
  FolderKey,
  KeyRound,
  Monitor,
  LifeBuoy,
  Mail,
  RotateCcw,
} from 'lucide-vue-next'
import SidebarMenuItems from './SidebarMenuItems.vue'
import type { Component } from 'vue'

// Emits
defineEmits<{
  close: []
  'toggle-collapse': []
}>()

defineProps<{ collapsed?: boolean }>()

// Current page
const page = usePage()

const adminVersion = computed(() => {
  const info = page.props.adminInfo as { version?: string } | undefined
  if (info?.version === 'dev') return t('разработка')
  return info?.version || t('неизвестна')
})

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
  const currentUrl = page.url.split('?')[0] ?? '/'

  if (currentUrl === href) {
    return true
  }

  if (href === '/') {
    return false
  }

  return currentUrl.startsWith(href.replace(/\/$/, '') + '/')
}
</script>
