import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import { initLocale, registerTranslations, setLocale, t, useI18n } from './index'

afterEach(() => {
  vi.restoreAllMocks()
  localStorage.clear()
  setLocale('ru')
})

describe('admin locale', () => {
  it('persists only supported locales and falls back safely after corrupt storage', () => {
    setLocale('en')
    expect(localStorage.getItem('goadmin.locale')).toBe('en')
    expect(document.documentElement.lang).toBe('en')
    expect(t('Пароль')).toBe('Password')
    localStorage.setItem('goadmin.locale', 'javascript:invalid')
    initLocale()
    expect(useI18n().locale.value).toBe('ru')
    expect(t('Пароль')).toBe('Пароль')
  })
  it('accepts explicit consumer dictionaries with plain text interpolation and fallback', () => {
    registerTranslations({
      ru: { 'consumer.title': 'Заявки' },
      en: { 'consumer.title': 'Requests', 'consumer.count': '{count} requests' },
    })
    setLocale('en')
    expect(t('consumer.title')).toBe('Requests')
    expect(t('consumer.count', undefined, { count: 4 })).toBe('4 requests')
    expect(t('unknown.key', 'Safe fallback')).toBe('Safe fallback')
    expect(t('constructor')).toBe('constructor')
    expect(t('Unrecognized user value')).toBe('Unrecognized user value')
  })
  it('updates reactive labels while preserving user field values', async () => {
    const container = document.createElement('div')
    const app = createApp({
      render: () => h('div', [h('label', t('Имя')), h('input', { value: 'Профиль' })]),
    })
    app.mount(container)
    setLocale('en')
    await nextTick()
    expect(container.querySelector('label')?.textContent).toBe('Name')
    expect(container.querySelector('input')?.value).toBe('Профиль')
    app.unmount()
  })
  it('works when browser storage is unavailable', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('blocked')
    })
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('blocked')
    })
    expect(() => initLocale()).not.toThrow()
    expect(() => setLocale('en')).not.toThrow()
    expect(t('Сохранить')).toBe('Save')
  })
})
