<script setup lang="ts">
import { ref } from 'vue'
import { t } from '@/i18n'
import { usePresentation } from '@/presentation/context'
import AdminHeader from './AdminHeader.vue'
import AdminSidebar from './AdminSidebar.vue'
import AdminTopNavigation from './AdminTopNavigation.vue'
import AdminMobileNavigation from './AdminMobileNavigation.vue'
import AdminNavigationToggle from './AdminNavigationToggle.vue'
import AdminHeaderUtilities from './AdminHeaderUtilities.vue'
import AdminAccountMenu from './AdminAccountMenu.vue'
import AdminBreadcrumbs from './AdminBreadcrumbs.vue'
import { provideAdminHeaderState } from './useAdminHeader'

const { adminShell } = usePresentation()
provideAdminHeaderState()
const sidebarOpen = ref(false)
const collapsed = ref(false)
const collapseKey = 'goadmin.sidebar.collapsed'
try {
  collapsed.value = localStorage.getItem(collapseKey) === 'true'
} catch {
  /* Session-only. */
}
const toggleCollapse = () => {
  collapsed.value = !collapsed.value
  try {
    localStorage.setItem(collapseKey, String(collapsed.value))
  } catch {
    /* Session-only. */
  }
}
</script>
<template>
  <component :is="adminShell" :collapsed="collapsed">
    <template #header
      ><AdminHeader :sidebar-open="sidebarOpen" @toggle-sidebar="sidebarOpen = !sidebarOpen"
    /></template>
    <template #navigation="{ orientation, collapsed: compact }">
      <AdminTopNavigation v-if="orientation === 'horizontal'" />
      <AdminSidebar v-else :collapsed="compact" @toggle-collapse="toggleCollapse" />
    </template>
    <template #mobileNavigation
      ><AdminMobileNavigation :open="sidebarOpen" @close="sidebarOpen = false"
    /></template>
    <template #navigationToggle
      ><AdminNavigationToggle :open="sidebarOpen" @toggle="sidebarOpen = !sidebarOpen"
    /></template>
    <template #utilities><AdminHeaderUtilities /></template>
    <template #account><AdminAccountMenu /></template>
    <template #brand
      ><span class="text-sm font-semibold text-text-primary">{{ t('Админка') }}</span></template
    >
    <template #breadcrumbs><AdminBreadcrumbs /></template>
    <slot />
  </component>
</template>
