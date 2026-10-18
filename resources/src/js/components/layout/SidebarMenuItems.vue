<template>
  <ul class="space-y-1" :aria-label="depth === 0 ? t('Разделы') : undefined">
    <li v-for="(item, index) in items" :key="itemKey(item, index)">
      <div class="flex items-center gap-0.5">
        <Link
          v-if="item.href"
          :href="item.href"
          :title="collapsed ? t(item.name) : undefined"
          :aria-label="t(item.name)"
          :aria-current="isActive(item.href) ? 'page' : undefined"
          :style="{
            paddingLeft: collapsed ? '0.5rem' : '0.75rem',
            paddingRight: collapsed ? '0.5rem' : '0.75rem',
          }"
          class="group flex min-w-0 flex-1 items-center rounded-lg py-2 text-sm focus-visible:ring-2 focus-visible:ring-primary"
          :class="[
            collapsed ? 'justify-center' : '',
            isActive(item.href)
              ? 'bg-primary/10 text-primary font-semibold'
              : isGroupActive(item)
                ? 'text-primary font-medium hover:bg-surface-variant'
                : 'text-text-secondary hover:bg-surface-variant',
          ]"
        >
          <component
            v-if="resolveIcon(item.icon)"
            :is="resolveIcon(item.icon)"
            class="h-5 w-5 shrink-0"
            aria-hidden="true"
          />
          <span
            v-else
            class="flex h-5 w-5 shrink-0 items-center justify-center text-xs font-semibold"
            aria-hidden="true"
            >{{ t(item.name).slice(0, 1).toUpperCase() }}</span
          >
          <span :class="collapsed ? 'sr-only' : 'ml-3 min-w-0 flex-1 truncate'">{{
            t(item.name)
          }}</span>
          <AppBadge
            v-if="!collapsed && showBadge(item.badge)"
            :variant="item.badge?.variant as any"
            class="ml-auto"
            >{{ item.badge?.text }}</AppBadge
          >
        </Link>
        <span
          v-else
          class="flex min-w-0 flex-1 items-center px-2 py-2 text-xs font-semibold text-text-tertiary"
          :title="collapsed ? t(item.name) : undefined"
        >
          <component
            v-if="collapsed && resolveIcon(item.icon)"
            :is="resolveIcon(item.icon)"
            class="h-5 w-5 shrink-0"
            aria-hidden="true"
          />
          <span
            v-else-if="collapsed"
            class="flex h-5 w-5 items-center justify-center"
            aria-hidden="true"
            >{{ Array.from(t(item.name))[0]?.toUpperCase() }}</span
          >
          <span :class="collapsed ? 'sr-only' : 'truncate'">{{ t(item.name) }}</span>
        </span>
        <button
          v-if="hasChildren(item)"
          type="button"
          :aria-label="`${isOpen(item, index) ? t('Свернуть') : t('Развернуть')} ${t(item.name)}`"
          :title="`${isOpen(item, index) ? t('Свернуть') : t('Развернуть')} ${t(item.name)}`"
          :aria-expanded="isOpen(item, index)"
          :aria-controls="groupID(index)"
          class="flex h-7 w-5 shrink-0 items-center justify-center rounded hover:bg-surface-variant focus-visible:ring-2 focus-visible:ring-primary"
          @click="toggleGroup(item, index)"
        >
          <ChevronDown
            class="h-4 w-4"
            :class="isOpen(item, index) ? 'rotate-180' : ''"
            aria-hidden="true"
          />
        </button>
      </div>
      <div
        v-if="hasChildren(item) && isOpen(item, index)"
        :id="groupID(index)"
        class="mt-1"
        :class="
          collapsed ? 'border-l border-border-primary' : 'ml-4 pl-3 border-l border-border-primary'
        "
      >
        <SidebarMenuItems
          :items="item.children ?? []"
          :depth="depth + 1"
          :is-active="isActive"
          :resolve-icon="resolveIcon"
          :collapsed="collapsed"
        />
      </div>
    </li>
  </ul>
</template>

<script setup lang="ts">
import { reactive, useId } from 'vue'
import type { Component } from 'vue'
import { Link } from '@inertiajs/vue3'
import { ChevronDown } from 'lucide-vue-next'
import AppBadge from '@/components/ui/AppBadge.vue'
import { t } from '@/i18n'

defineOptions({ name: 'SidebarMenuItems' })
type MenuIcon = string | Component | undefined
type MenuBadge = { text?: string; variant?: string; dot?: boolean; hideIfZero?: boolean }
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
  collapsed?: boolean
  isActive: (href: string) => boolean
  resolveIcon: (icon: MenuIcon) => Component | undefined
}>()
const groupPrefix = useId()
const groupID = (index: number) => `${groupPrefix}-${index}`
const openState = reactive<Record<string, boolean>>({})
const hasChildren = (item: AdminMenuItem) => Boolean(item.children?.length)
const itemKey = (item: AdminMenuItem, index: number) =>
  item.href || `${props.depth}:${index}:${item.name}`
const isGroupActive = (item: AdminMenuItem): boolean =>
  Boolean(item.href && props.isActive(item.href)) || Boolean(item.children?.some(isGroupActive))
const isOpen = (item: AdminMenuItem, index: number) =>
  hasChildren(item) && (openState[itemKey(item, index)] ?? isGroupActive(item))
const toggleGroup = (item: AdminMenuItem, index: number) => {
  openState[itemKey(item, index)] = !isOpen(item, index)
}
const showBadge = (badge?: MenuBadge) =>
  Boolean(badge && (badge.dot || (badge.text && !(badge.hideIfZero && badge.text === '0'))))
</script>
