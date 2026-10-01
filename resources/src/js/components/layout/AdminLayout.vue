<template>
  <div class="min-h-screen bg-background text-text-primary">
    <!-- Sidebar -->
    <div
      :class="[
        'fixed inset-y-0 left-0 z-50 w-64 bg-card border-r border-border-primary shadow-lg transform transition-transform duration-300 ease-in-out',
        sidebarOpen ? 'translate-x-0' : '-translate-x-full lg:translate-x-0',
      ]"
    >
      <AdminSidebar @close="closeSidebar" />
    </div>

    <!-- Overlay для мобильных устройств -->
    <div
      v-if="sidebarOpen"
      class="fixed inset-0 z-40 lg:hidden backdrop-blur-sm"
      style="background-color: var(--color-overlay)"
      @click="closeSidebar"
    />

    <!-- Main content -->
    <div class="lg:pl-64">
      <!-- Header -->
      <AdminHeader @toggle-sidebar="toggleSidebar" />

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
import { ref, onMounted, onUnmounted } from 'vue'
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

// Тема инициализируется глобально в app.ts

const toggleSidebar = () => {
  sidebarOpen.value = !sidebarOpen.value
}

const closeSidebar = () => {
  sidebarOpen.value = false
}

// Закрытие сайдбара при нажатии Escape
const handleKeydown = (event: KeyboardEvent) => {
  if (event.key === 'Escape') {
    closeSidebar()
  }
}

onMounted(() => {
  // Добавляем обработчик клавиш
  document.addEventListener('keydown', handleKeydown)
})

onUnmounted(() => {
  document.removeEventListener('keydown', handleKeydown)
})
</script>
