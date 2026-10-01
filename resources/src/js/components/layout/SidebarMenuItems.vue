<template>
  <div class="space-y-1">
    <template v-for="(item, index) in items" :key="itemKey(item, index)">
      <div :class="depth > 0 ? 'pl-3' : ''">
        <SidebarLink
          v-if="item.href"
          class="w-full"
          :href="item.href"
          :icon="resolveIcon(item.icon)"
          :active="isGroupActive(item)"
        >
          <span class="flex w-full items-center">
            <span class="flex-1 truncate">{{ item.name }}</span>
            <AppBadge
              v-if="showBadge(item.badge)"
              :variant="item.badge?.variant as any"
              class="ml-auto"
            >
              {{ item.badge?.text }}
            </AppBadge>
            <button
              v-if="hasChildren(item)"
              type="button"
              :aria-label="`${isOpen(item, index) ? 'Свернуть' : 'Развернуть'} ${item.name}`"
              class="ml-1 h-7 w-7 flex items-center justify-center rounded-lg text-text-tertiary hover:text-text-primary hover:bg-surface-variant transition-colors duration-150"
              :aria-expanded="isOpen(item, index) ? 'true' : 'false'"
              @click.stop.prevent="toggleGroup(item, index)"
            >
              <ChevronDown
                class="h-4 w-4 transition-transform duration-200"
                :class="isOpen(item, index) ? 'rotate-180 text-primary' : ''"
              />
            </button>
          </span>
        </SidebarLink>
        <div
          v-else
          class="px-3 py-2 text-xs font-semibold uppercase tracking-widest text-text-tertiary"
        >
          {{ item.name }}
        </div>
      </div>

      <div
        v-if="hasChildren(item) && isOpen(item, index)"
        class="ml-4 pl-3 border-l border-border-primary/70"
      >
        <SidebarMenuItems
          :items="item.children ?? []"
          :depth="depth + 1"
          :is-active="isActive"
          :resolve-icon="resolveIcon"
        />
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive } from 'vue'
import type { Component } from 'vue'
import { ChevronDown } from 'lucide-vue-next'
import AppBadge from '@/components/ui/AppBadge.vue'
import SidebarLink from './SidebarLink.vue'

defineOptions({ name: 'SidebarMenuItems' })

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

const props = defineProps<{
  items: AdminMenuItem[]
  depth: number
  isActive: (href: string) => boolean
  resolveIcon: (icon: MenuIcon) => Component | undefined
}>()

const items = computed(() => props.items)
const depth = computed(() => props.depth)
const isActive = props.isActive
const resolveIcon = props.resolveIcon

const openState = reactive<Record<string, boolean>>({})

const hasChildren = (item: AdminMenuItem): boolean => {
  return Array.isArray(item.children) && item.children.length > 0
}

const itemKey = (item: AdminMenuItem, index: number): string => {
  if (item.href) return item.href
  return `${depth.value}:${index}:${item.name}`
}

const isGroupActive = (item: AdminMenuItem): boolean => {
  if (item.href && isActive(item.href)) {
    return true
  }
  if (!hasChildren(item)) {
    return false
  }
  return item.children!.some((child) => isGroupActive(child))
}

const isOpen = (item: AdminMenuItem, index: number): boolean => {
  if (!hasChildren(item)) {
    return false
  }
  // If any descendant is active, keep the group expanded.
  if (isGroupActive(item)) {
    return true
  }
  const key = itemKey(item, index)
  return Boolean(openState[key])
}

const toggleGroup = (item: AdminMenuItem, index: number): void => {
  if (!hasChildren(item)) return
  if (isGroupActive(item)) return
  const key = itemKey(item, index)
  openState[key] = !isOpen(item, index)
}

const showBadge = (badge?: MenuBadge): boolean => {
  if (!badge) return false
  if (badge.dot) return true
  const text = badge.text ?? ''
  if (badge.hideIfZero && (text === '' || text === '0')) {
    return false
  }
  return text.length > 0
}
</script>
