<template>
  <div class="flex flex-col h-full">
    <!-- Logo -->
    <div class="flex items-center justify-between px-5 py-4 border-b border-border-primary">
      <div class="flex items-center space-x-3">
        <div
          class="h-9 w-9 rounded-xl bg-primary/15 border border-primary/20 flex items-center justify-center"
        >
          <span class="text-primary font-bold text-sm tracking-wide">A</span>
        </div>
        <div>
          <p class="text-sm font-semibold text-text-primary uppercase tracking-wider">Админка</p>
          <p class="text-xs text-text-tertiary">Панель управления</p>
        </div>
      </div>

      <!-- Close button for mobile -->
      <AppButton
        variant="ghost"
        size="icon-sm"
        class="lg:hidden"
        aria-label="Закрыть меню"
        @click="$emit('close')"
      >
        <X class="h-5 w-5" />
      </AppButton>
    </div>

    <!-- Navigation -->
    <nav class="flex-1 min-h-0 px-0 py-4 space-y-1 overflow-y-auto">
      <template v-for="(section, index) in sections" :key="section.key">
        <div class="space-y-1 px-2">
          <div v-if="section.title" class="px-2 py-2">
            <h3 class="text-[11px] font-semibold text-text-tertiary uppercase tracking-widest">
              {{ section.title }}
            </h3>
          </div>
          <SidebarMenuItems
            :items="section.items"
            :depth="0"
            :is-active="isActive"
            :resolve-icon="resolveIcon"
          />
        </div>

        <div v-if="index < sections.length - 1" class="border-t border-border-primary my-4"></div>
      </template>
    </nav>

    <!-- Footer -->
    <footer class="shrink-0 border-t border-border-primary p-4">
      <p class="text-xs font-medium text-text-secondary">Об админке</p>
      <p class="mt-1 text-xs text-text-tertiary break-words">GoAdmin · версия {{ adminVersion }}</p>
    </footer>
  </div>
</template>

<script setup lang="ts">
// Computed и другие импорты
import { computed } from 'vue'
import { usePage } from '@inertiajs/vue3'
import AppButton from '@/components/ui/AppButton.vue'
import { Bell, BookOpen, Home, ListTodo, ShieldCheck, Users, X } from 'lucide-vue-next'
import SidebarMenuItems from './SidebarMenuItems.vue'
import type { Component } from 'vue'

// Emits
defineEmits<{
  close: []
}>()

// Current page
const page = usePage()

const adminVersion = computed(() => {
  const info = page.props.adminInfo as { version?: string } | undefined
  if (info?.version === 'dev') return 'разработка'
  return info?.version || 'неизвестна'
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
}

const sections = computed(() => {
  const propsMenu = (page.props as { adminMenu?: Partial<AdminMenu> }).adminMenu
  if (!Array.isArray(propsMenu?.sections)) return []
  return propsMenu.sections.filter((section) => section.items?.length > 0)
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
  const currentUrl = page.url

  if (currentUrl === href) {
    return true
  }

  if (href === '/') {
    return false
  }

  return currentUrl.startsWith(href)
}
</script>
