import { afterEach, expect, it, vi } from 'vitest'
let dispose: (() => void) | undefined
afterEach(() => {
  dispose?.()
  localStorage.clear()
  vi.unstubAllGlobals()
})
async function loadTheme(systemDark: boolean, saved: 'light' | 'dark' | null = null) {
  vi.resetModules()
  if (saved) localStorage.setItem('goadmin.appearance.device.v1', JSON.stringify({ theme: saved }))
  const media = { matches: systemDark, addEventListener: vi.fn(), removeEventListener: vi.fn() }
  vi.stubGlobal(
    'matchMedia',
    vi.fn(() => media),
  )
  const { themeManager } = await import('./theme')
  const { disposeAppearance } = await import('@/appearance/state')
  dispose = disposeAppearance
  themeManager.init()
  return {
    themeManager,
    changeSystem: (dark: boolean) => {
      media.matches = dark
      media.addEventListener.mock.calls[0]![1]()
    },
  }
}
it.each([false, true])('follows system dark=%s without saving device preferences', async (dark) => {
  const { themeManager, changeSystem } = await loadTheme(dark)
  expect(themeManager.getCurrent()).toBe(dark ? 'dark' : 'light')
  changeSystem(!dark)
  expect(themeManager.getCurrent()).toBe(dark ? 'light' : 'dark')
  expect(localStorage.getItem('goadmin.appearance.device.v1')).toBeNull()
})
it.each(['light', 'dark'] as const)(
  'preserves explicit device %s against system changes',
  async (saved) => {
    const { themeManager, changeSystem } = await loadTheme(saved === 'light', saved)
    expect(themeManager.getCurrent()).toBe(saved)
    changeSystem(saved === 'light')
    expect(themeManager.getCurrent()).toBe(saved)
    themeManager.toggle()
    const next = saved === 'light' ? 'dark' : 'light'
    expect(themeManager.getCurrent()).toBe(next)
    expect(JSON.parse(localStorage.getItem('goadmin.appearance.device.v1')!).theme).toBe(next)
  },
)
it('does not import legacy shared browser theme into an account or device', async () => {
  localStorage.setItem('theme-mode', 'dark')
  const { themeManager } = await loadTheme(false)
  expect(themeManager.getCurrent()).toBe('light')
})
