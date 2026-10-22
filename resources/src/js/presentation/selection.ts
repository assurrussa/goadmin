import type { AdminThemePack } from './types'

export function resolveTheme(
  modules: Record<string, { default?: { theme?: AdminThemePack } }>,
  selectedKey: string | null,
): AdminThemePack | undefined {
  for (const [key, module] of Object.entries(modules)) {
    if (module.default?.theme && key !== selectedKey)
      throw new Error(`Admin theme ${key} must be selected with themeModule in its source manifest`)
  }
  if (selectedKey === null) return undefined
  const theme = modules[selectedKey]?.default?.theme
  if (!theme || theme.apiVersion !== 1 || !/^[a-z][a-z0-9._-]*$/.test(theme.id))
    throw new Error(`Admin theme ${selectedKey} must export theme with a valid id and apiVersion 1`)
  for (const shell of [theme.adminShell, theme.authShell]) {
    if (shell !== undefined && (shell === null || !['object', 'function'].includes(typeof shell)))
      throw new Error(`Admin theme ${selectedKey} exports an invalid shell component`)
  }
  return theme
}
