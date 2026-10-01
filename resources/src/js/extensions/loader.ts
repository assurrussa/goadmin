import type { App } from 'vue'
import type { AdminExtension } from '@/extensions/types'
import type { AdminWebSocketHandler } from '@/websocket/types'
import { extensionModules } from '@admin-extensions-generated'

type ExtensionModule = {
  default?: AdminExtension
}

type LoadedExtensions = {
  install: (app: App) => void
  webSocketHandlers: AdminWebSocketHandler[]
}

export const loadExtensions = (): LoadedExtensions => {
  const modules = extensionModules as unknown as Record<string, ExtensionModule>
  const extensions = Object.values(modules)
    .map((mod) => mod.default)
    .filter((ext): ext is AdminExtension => Boolean(ext))

  return {
    install(app: App) {
      extensions.forEach((ext) => {
        ext.install?.(app)
      })
    },
    webSocketHandlers: extensions.flatMap((ext) => ext.webSocketHandlers ?? []),
  }
}
