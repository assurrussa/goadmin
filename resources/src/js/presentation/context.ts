import { inject } from 'vue'
import type { App, InjectionKey } from 'vue'
import type { AdminThemePack } from './types'
import DefaultAdminShell from './DefaultAdminShell.vue'
import DefaultAuthShell from './DefaultAuthShell.vue'

const key: InjectionKey<AdminThemePack> = Symbol('goadmin.presentation')
export const installPresentation = (app: App, theme?: AdminThemePack) => {
  const selected = theme ?? { id: 'default', apiVersion: 1 }
  app.provide(key, selected)
  if (typeof document !== 'undefined') document.documentElement.dataset.adminTheme = selected.id
}
export const usePresentation = () => {
  const theme = inject(key, undefined)
  return {
    adminShell: theme?.adminShell ?? DefaultAdminShell,
    authShell: theme?.authShell ?? DefaultAuthShell,
  }
}
