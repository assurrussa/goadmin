import { afterEach, expect, it, vi } from 'vitest'

afterEach(() => {
  localStorage.removeItem('theme-mode')
  document.documentElement.classList.remove('dark')
  document.documentElement.removeAttribute('data-theme-mode')
  vi.unstubAllGlobals()
})

async function loadTheme(systemDark: boolean, saved: 'light' | 'dark' | null = null) {
  vi.resetModules()
  if (saved === null) localStorage.removeItem('theme-mode')
  else localStorage.setItem('theme-mode', saved)
  let onChange: ((event: { matches: boolean }) => void) | undefined
  vi.stubGlobal(
    'matchMedia',
    vi.fn(() => ({
      matches: systemDark,
      addEventListener: (_name: string, listener: typeof onChange) => (onChange = listener),
    })),
  )
  const { themeManager } = await import('./theme')
  return { themeManager, changeSystem: (dark: boolean) => onChange!({ matches: dark }) }
}

it.each([false, true])(
  'follows system dark=%s without creating a saved preference',
  async (dark) => {
    const { themeManager, changeSystem } = await loadTheme(dark)
    expect(themeManager.getCurrent()).toBe(dark ? 'dark' : 'light')
    expect(document.documentElement.getAttribute('data-theme-mode')).toBe(dark ? 'dark' : 'light')
    expect(localStorage.getItem('theme-mode')).toBeNull()
    changeSystem(!dark)
    expect(themeManager.getCurrent()).toBe(dark ? 'light' : 'dark')
    expect(document.documentElement.getAttribute('data-theme-mode')).toBe(dark ? 'light' : 'dark')
    expect(localStorage.getItem('theme-mode')).toBeNull()
  },
)

it.each(['light', 'dark'] as const)(
  'preserves explicit %s against system changes',
  async (saved) => {
    const { themeManager, changeSystem } = await loadTheme(saved === 'light', saved)
    expect(themeManager.getCurrent()).toBe(saved)
    changeSystem(saved === 'light')
    expect(themeManager.getCurrent()).toBe(saved)
    expect(localStorage.getItem('theme-mode')).toBe(saved)
    themeManager.toggle()
    const next = saved === 'light' ? 'dark' : 'light'
    expect(themeManager.getCurrent()).toBe(next)
    expect(localStorage.getItem('theme-mode')).toBe(next)
    changeSystem(saved === 'dark')
    expect(themeManager.getCurrent()).toBe(next)
  },
)
