<template>
  <div class="min-h-screen bg-background text-text-primary">
    <!-- Sidebar -->
    <div
      ref="sidebarElement"
      id="admin-sidebar"
      :inert="isMobile && !sidebarOpen"
      :role="sidebarOpen ? 'dialog' : undefined"
      :aria-modal="sidebarOpen ? true : undefined"
      :aria-label="t('Разделы')"
      :class="[
        'fixed inset-y-0 left-0 z-50 w-64 bg-card border-r border-border-primary shadow-lg overflow-x-hidden transform transition-[width,transform] duration-200 ease-in-out motion-reduce:transition-none',
        sidebarCollapsed && !sidebarOpen ? 'lg:w-20' : 'lg:w-64',
        sidebarOpen ? 'translate-x-0' : '-translate-x-full lg:translate-x-0',
      ]"
    >
      <AdminSidebar
        :collapsed="sidebarCollapsed && !sidebarOpen"
        @close="closeSidebar"
        @toggle-collapse="toggleCollapse"
      />
    </div>

    <!-- Overlay для мобильных устройств -->
    <div
      v-if="sidebarOpen"
      class="fixed inset-0 z-40 lg:hidden backdrop-blur-sm"
      style="background-color: var(--color-overlay)"
      @click="closeSidebar"
    />

    <!-- Main content -->
    <div
      :class="sidebarCollapsed ? 'lg:pl-20' : 'lg:pl-64'"
      class="transition-[padding] duration-200 ease-in-out motion-reduce:transition-none"
    >
      <!-- Header -->
      <AdminHeader :sidebar-open="sidebarOpen" @toggle-sidebar="toggleSidebar" />

      <!-- Page content -->
      <main class="py-6">
        <div class="max-w-7xl mx-auto w-full px-4 sm:px-6 lg:px-8 space-y-6">
          <AdminBreadcrumbs />
          <slot />
        </div>
      </main>
    </div>

    <!-- Notifications Container -->
    <NotificationsContainer />
    <UploadQueueContainer v-if="uploads" />
  </div>
</template>

<script setup lang="ts">
import { ref, watch, nextTick, onMounted, onUnmounted } from 'vue'
import { t } from '@/i18n'
import { router } from '@inertiajs/vue3'
import { useAdminCapabilities } from '@/composables/useAdminCapabilities'
import AdminHeader from './AdminHeader.vue'
import AdminSidebar from './AdminSidebar.vue'
import AdminBreadcrumbs from './AdminBreadcrumbs.vue'
import NotificationsContainer from '@/components/NotificationsContainer.vue'
import UploadQueueContainer from '@/components/UploadQueueContainer.vue'
import { useFlashMessages } from '~/composables/useFlashMessages'

const { uploads } = useAdminCapabilities()

useFlashMessages()

// Состояние бокового меню
const sidebarOpen = ref(false)
const isMobile = ref(
  typeof window !== 'undefined' && window.matchMedia('(max-width: 1023px)').matches,
)
let mobileQuery: MediaQueryList | undefined
const syncViewport = () => {
  isMobile.value = mobileQuery?.matches ?? false
  closeSidebar()
}
const sidebarElement = ref<HTMLElement | null>(null)
let focusReturn: HTMLElement | null = null
let previousOverflow = ''
watch(sidebarOpen, async (open) => {
  if (open) {
    focusReturn = document.activeElement as HTMLElement | null
    previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    await nextTick()
    sidebarElement.value?.querySelector<HTMLElement>('button:not(.hidden), a[href]')?.focus()
  } else {
    document.body.style.overflow = previousOverflow
    focusReturn?.focus()
  }
})
const sidebarCollapsed = ref(false)
const collapseKey = 'goadmin.sidebar.collapsed'
try {
  sidebarCollapsed.value = localStorage.getItem(collapseKey) === 'true'
} catch {
  /* Keep expanded. */
}
const toggleCollapse = () => {
  sidebarCollapsed.value = !sidebarCollapsed.value
  try {
    localStorage.setItem(collapseKey, String(sidebarCollapsed.value))
  } catch {
    /* Session-only choice. */
  }
}

// Тема инициализируется глобально в app.ts

const toggleSidebar = () => {
  sidebarOpen.value = !sidebarOpen.value
}

const closeSidebar = () => {
  sidebarOpen.value = false
}

// Закрытие сайдбара при нажатии Escape
const handleKeydown = (event: KeyboardEvent) => {
  if (sidebarOpen.value && event.key === 'Tab') {
    const nodes = Array.from(
      sidebarElement.value?.querySelectorAll<HTMLElement>('a[href],button,select,[tabindex="0"]') ??
        [],
    ).filter((el) => el.getClientRects().length > 0)
    const first = nodes[0],
      last = nodes.at(-1)
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault()
      last?.focus()
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault()
      first?.focus()
    }
  }
  if (event.key === 'Escape') {
    closeSidebar()
  }
}

onMounted(() => {
  // Добавляем обработчик клавиш
  document.addEventListener('keydown', handleKeydown)
  mobileQuery = window.matchMedia('(max-width: 1023px)')
  mobileQuery.addEventListener('change', syncViewport)
})

const stopNavigation = router.on('navigate', closeSidebar)
onUnmounted(() => {
  stopNavigation()
  if (sidebarOpen.value) document.body.style.overflow = previousOverflow
  document.removeEventListener('keydown', handleKeydown)
  mobileQuery?.removeEventListener('change', syncViewport)
})
</script>
