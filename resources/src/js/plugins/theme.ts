import { appearance, editAppearance, initAppearance } from '@/appearance/state'
export const themeManager = {
  init: initAppearance,
  toggle: () => editAppearance({ theme: appearance.dark ? 'light' : 'dark' }),
  setDark: () => editAppearance({ theme: 'dark' }),
  setLight: () => editAppearance({ theme: 'light' }),
  getCurrent: () => (appearance.dark ? 'dark' : 'light'),
}
