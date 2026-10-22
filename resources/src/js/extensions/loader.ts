import { registerTranslations } from '@/i18n'
import type { App } from 'vue'
import type { AdminExtension } from '@/extensions/types'
import type { AdminWebSocketHandler } from '@/websocket/types'
import { extensionModules, selectedThemeModuleKey } from '@admin-extensions-generated'
import { resolveTheme } from '@/presentation/selection'
import type { AdminThemePack } from '@/presentation/types'

type ExtensionModule = {
  default?: AdminExtension
}

type LoadedExtensions = {
  theme?: AdminThemePack
  install: (app: App) => void
  webSocketHandlers: AdminWebSocketHandler[]
}

export const loadExtensions = (): LoadedExtensions => {
  const modules = extensionModules as unknown as Record<string, ExtensionModule>
  const extensions = Object.values(modules)
    .map((mod) => mod.default)
    .filter((ext): ext is AdminExtension => Boolean(ext))

  return {
    theme: resolveTheme(modules, selectedThemeModuleKey),
    install(app: App) {
      extensions.forEach((ext) => {
        if (ext.translations) registerTranslations(ext.translations)
        ext.install?.(app)
      })
    },
    webSocketHandlers: extensions.flatMap((ext) => ext.webSocketHandlers ?? []),
  }
}
