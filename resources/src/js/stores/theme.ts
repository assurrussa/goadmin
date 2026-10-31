import { defineStore } from 'pinia'
import { computed } from 'vue'
import { appearance, editAppearance, initAppearance } from '@/appearance/state'
export const useThemeStore = defineStore('theme', () => ({
  theme: computed(() => (appearance.dark ? 'dark' : 'light')),
  isDark: computed(() => appearance.dark),
  init: initAppearance,
  toggle: () => editAppearance({ theme: appearance.dark ? 'light' : 'dark' }),
  setTheme: (theme: 'light' | 'dark') => editAppearance({ theme }),
}))
