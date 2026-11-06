import { defineStore } from 'pinia'
import { computed } from 'vue'
import type { Preferences } from '@/appearance/state'
import { appearance, editAppearance, initAppearance } from '@/appearance/state'
export const useThemeStore = defineStore('theme', () => ({
  theme: computed(() => (appearance.dark ? 'dark' : 'light')),
  preference: computed(() => appearance.draft.theme),
  isDark: computed(() => appearance.dark),
  init: initAppearance,
  toggle: () => editAppearance({ theme: appearance.dark ? 'light' : 'dark' }),
  setTheme: (theme: Preferences['theme']) => editAppearance({ theme }),
}))
