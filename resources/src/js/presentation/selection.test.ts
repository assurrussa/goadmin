import { describe, expect, it } from 'vitest'
import { resolveTheme } from './selection'
import type { AdminThemePack } from './types'

describe('declared presentation selection', () => {
  it('uses defaults without a selected pack and permits legacy CSS modules', () => {
    expect(resolveTheme({ 'brand/theme': { default: {} } }, null)).toBeUndefined()
  })
  it('selects exactly the declared module, with optional shell fallbacks', () => {
    const theme: AdminThemePack = { id: 'host.custom', apiVersion: 1 }
    expect(resolveTheme({ 'host/theme': { default: { theme } } }, 'host/theme')).toBe(theme)
  })
  it('rejects undeclared themes rather than relying on import order', () => {
    const modules = { 'a/theme': { default: { theme: { id: 'host.a', apiVersion: 1 as const } } } }
    expect(() => resolveTheme(modules, null)).toThrow('themeModule')
    expect(() => resolveTheme(modules, 'b/theme')).toThrow('themeModule')
  })
  it.each([
    undefined,
    {},
    { id: 'bad id', apiVersion: 1 },
    { id: 'host.a', apiVersion: 2 },
    { id: 'host.a', apiVersion: 1, adminShell: 42 },
  ])('rejects malformed runtime exports before mounting: %j', (theme) => {
    const modules = { 'host/theme': { default: { theme: theme as AdminThemePack } } }
    expect(() => resolveTheme(modules, 'host/theme')).toThrow('Admin theme')
  })
})
