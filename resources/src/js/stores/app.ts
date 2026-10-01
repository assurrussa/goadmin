import { defineStore } from 'pinia'
import { ref, computed } from 'vue'

export const useAppStore = defineStore('app', () => {
  // State
  const isLoading = ref(false)
  const theme = ref<'light' | 'dark'>('light')
  const sidebarOpen = ref(false)
  const notifications = ref<
    Array<{
      id: string
      type: 'success' | 'error' | 'warning' | 'info'
      title: string
      message: string
      timestamp: Date
    }>
  >([])

  // Getters
  const isDarkTheme = computed(() => theme.value === 'dark')
  const unreadNotifications = computed(() => notifications.value.length)

  // Actions
  const setLoading = (loading: boolean) => {
    isLoading.value = loading
  }

  const toggleTheme = () => {
    theme.value = theme.value === 'light' ? 'dark' : 'light'
    // Обновляем класс на body для dark mode
    if (typeof document !== 'undefined') {
      document.documentElement.classList.toggle('dark', theme.value === 'dark')
    }
  }

  const toggleSidebar = () => {
    sidebarOpen.value = !sidebarOpen.value
  }

  const addNotification = (
    notification: Omit<(typeof notifications.value)[0], 'id' | 'timestamp'>,
  ) => {
    const id = Date.now().toString()
    notifications.value.push({
      ...notification,
      id,
      timestamp: new Date(),
    })

    // Автоматически удаляем уведомление через 5 секунд
    setTimeout(() => {
      removeNotification(id)
    }, 5000)
  }

  const removeNotification = (id: string) => {
    const index = notifications.value.findIndex((n) => n.id === id)
    if (index > -1) {
      notifications.value.splice(index, 1)
    }
  }

  return {
    // State
    isLoading,
    theme,
    sidebarOpen,
    notifications,

    // Getters
    isDarkTheme,
    unreadNotifications,

    // Actions
    setLoading,
    toggleTheme,
    toggleSidebar,
    addNotification,
    removeNotification,
  }
})
