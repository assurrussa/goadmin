import { defineStore } from 'pinia'
import { themeManager } from '@/plugins/theme'

/**
 * Хранилище для управления темой сайта (светлая/темная)
 */
export const useThemeStore = defineStore('theme', {
  state: () => ({
    // Текущая тема (light|dark)
    theme: 'light',
  }),

  getters: {
    // Активна ли темная тема
    isDark: (state) => state.theme === 'dark',
  },

  actions: {
    /**
     * Инициализация темы при загрузке приложения
     * - Синхронизируется с уже примененной темой из DOM
     * - Загружает сохраненную тему из localStorage
     */
    init() {
      try {
        // Когда код выполняется на стороне клиенте
        if (typeof window !== 'undefined') {
          // Получаем текущую тему от плагина
          this.theme = themeManager.getCurrent()
        }
      } catch (error) {
        console.error('Failed to initialize theme:', error)
        // По умолчанию используем светлую тему
        this.theme = 'light'
      }
    },

    /**
     * Переключение между темной и светлой темами
     */
    toggle() {
      themeManager.toggle()
      this.theme = themeManager.getCurrent()
    },

    /**
     * Установка определенной темы
     */
    setTheme(newTheme: 'light' | 'dark') {
      if (this.theme !== newTheme) {
        if (newTheme === 'dark') {
          themeManager.setDark()
        } else {
          themeManager.setLight()
        }
        this.theme = themeManager.getCurrent()
      }
    },
  },
})
