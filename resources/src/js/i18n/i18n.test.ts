import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import { initLocale, registerTranslations, setLocale, t, useI18n } from './index'

afterEach(() => {
  vi.restoreAllMocks()
  localStorage.clear()
  setLocale('ru')
})

describe('admin locale', () => {
  it('initializes from validated document language without importing legacy storage', () => {
    setLocale('en')
    expect(localStorage.getItem('goadmin.locale')).toBeNull()
    expect(document.documentElement.lang).toBe('en')
    expect(t('Пароль')).toBe('Password')
    localStorage.setItem('goadmin.locale', 'javascript:invalid')
    document.documentElement.lang = 'ru'
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

describe('native constraint labels', () => {
  it('relabels required errors without changing values or constraints and preserves consumer custom validity', () => {
    initLocale()
    const input = document.createElement('input')
    input.required = true
    document.body.append(input)
    expect(input.checkValidity()).toBe(false)
    expect(input.validationMessage).toBe('Заполните это поле.')
    setLocale('en')
    expect(input.validationMessage).toBe('Fill in this field.')
    expect(input.required).toBe(true)
    expect(input.value).toBe('')
    input.value = 'Профиль'
    input.dispatchEvent(new Event('input', { bubbles: true }))
    expect(input.checkValidity()).toBe(true)
    expect(input.value).toBe('Профиль')
    input.setCustomValidity('Consumer validation')
    expect(input.checkValidity()).toBe(false)
    setLocale('ru')
    expect(input.validationMessage).toBe('Consumer validation')
    input.remove()
  })
})

it('relinquishes native-error ownership when a consumer installs a validation rule', () => {
  initLocale()
  const input = document.createElement('input')
  input.required = true
  document.body.append(input)
  expect(input.checkValidity()).toBe(false)
  input.value = 'synthetic'
  input.setCustomValidity('Consumer business rule')
  setLocale('en')
  expect(input.validationMessage).toBe('Consumer business rule')
  expect(input.checkValidity()).toBe(false)
  input.dispatchEvent(new Event('input', { bubbles: true }))
  expect(input.validationMessage).toBe('Consumer business rule')
  expect(input.checkValidity()).toBe(false)
  input.remove()
})

it('switches explicit core UI messages without translating consumer values', () => {
  for (const [key, ru, en] of [
    ['core.ui.Email', 'Электронная почта', 'Email'],
    ['core.ui.Next', 'Следующая', 'Next'],
    ['core.ui.Add Image', 'Добавить изображение', 'Add Image'],
    ['core.ui.Check upload', 'Проверить загрузку', 'Check upload'],
  ]) {
    setLocale('ru')
    expect(t(key!, en)).toBe(ru)
    setLocale('en')
    expect(t(key!, en)).toBe(en)
  }
  expect(t('Name')).toBe('Name')
})
