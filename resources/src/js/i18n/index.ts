import { readonly, ref } from 'vue'
import en from './en.json'
import ru from './ru.json'

export type AdminLocale = 'ru' | 'en'
export type AdminTranslations = Partial<Record<AdminLocale, Record<string, string>>>
let preferenceHandler: ((value: AdminLocale) => void) | undefined
export function installLocalePreferenceHandler(handler: (value: AdminLocale) => void) {
  preferenceHandler = handler
}
const currentLocale = ref<AdminLocale>('ru')
export const locale = readonly(currentLocale)
type ValidityField = HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement
const invalidFields = new Map<ValidityField, string>()
let validationInitialized = false
const dictionaries = {
  ru: Object.assign(Object.create(null), ru),
  en: Object.assign(Object.create(null), en),
} as Record<AdminLocale, Record<string, string>>

/** Register explicit UI messages. User-provided content is never translated implicitly. */
export function registerTranslations(messages: AdminTranslations): void {
  for (const lang of ['ru', 'en'] as const) {
    for (const [key, value] of Object.entries(messages[lang] ?? {})) {
      if (typeof value === 'string' && !['__proto__', 'prototype', 'constructor'].includes(key)) {
        dictionaries[lang][key] = value
      }
    }
  }
}

export function setLocale(value: string): void {
  if (preferenceHandler) {
    preferenceHandler(value === 'en' ? 'en' : 'ru')
    return
  }
  applyLocale(value)
}

export function applyLocale(value: string): void {
  currentLocale.value = value === 'en' ? 'en' : 'ru'
  if (typeof document !== 'undefined') {
    document.documentElement.lang = currentLocale.value
    for (const field of invalidFields.keys()) {
      if (field.isConnected) updateValidityMessage(field)
      else invalidFields.delete(field)
    }
  }
}

export function initLocale(): void {
  applyLocale(typeof document !== 'undefined' ? document.documentElement.lang : 'ru')
  initValidationMessages()
}

export function t(
  key: string,
  fallback?: string,
  params?: Record<string, string | number>,
): string {
  const message = dictionaries[currentLocale.value][key] ?? dictionaries.ru[key] ?? fallback ?? key
  return message.replace(/\{(\w+)\}/g, (token, name: string) =>
    params?.[name] === undefined ? token : String(params[name]),
  )
}

export function formatDateTime(value: string | number | Date): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return t('Не указано')
  return (
    new Intl.DateTimeFormat(currentLocale.value === 'en' ? 'en-GB' : 'ru-RU', {
      dateStyle: 'medium',
      timeStyle: 'short',
      timeZone: Intl.DateTimeFormat().resolvedOptions().timeZone,
    }).format(date) +
    ' (' +
    Intl.DateTimeFormat().resolvedOptions().timeZone +
    ')'
  )
}

export function useI18n() {
  return { t, locale, setLocale, formatDateTime }
}

// Only relabel browser-owned constraints. Consumer custom validity is preserved.
function updateValidityMessage(field: ValidityField): void {
  const ownedMessage = invalidFields.get(field)
  if (ownedMessage !== undefined && field.validationMessage !== ownedMessage) {
    invalidFields.delete(field)
    return
  }
  field.setCustomValidity('')
  if (field.validity.valid) {
    invalidFields.delete(field)
    return
  }
  const validity = field.validity
  const message = validity.valueMissing
    ? 'Заполните это поле.'
    : validity.typeMismatch
      ? 'Проверьте формат значения.'
      : validity.patternMismatch
        ? 'Используйте указанный формат.'
        : validity.tooShort
          ? 'Значение слишком короткое.'
          : validity.tooLong
            ? 'Значение слишком длинное.'
            : validity.rangeUnderflow || validity.rangeOverflow
              ? 'Значение вне допустимого диапазона.'
              : validity.stepMismatch
                ? 'Используйте допустимый шаг значения.'
                : 'Проверьте значение.'
  const translated = t(message)
  field.setCustomValidity(translated)
  invalidFields.set(field, translated)
}
function initValidationMessages(): void {
  if (validationInitialized || typeof document === 'undefined') return
  validationInitialized = true
  const isField = (value: EventTarget | null): value is ValidityField =>
    value instanceof HTMLInputElement ||
    value instanceof HTMLTextAreaElement ||
    value instanceof HTMLSelectElement
  document.addEventListener(
    'invalid',
    (event) => {
      const field = event.target
      if (!isField(field) || (field.validity.customError && !invalidFields.has(field))) return
      updateValidityMessage(field)
    },
    true,
  )
  document.addEventListener('input', (event) => {
    const field = event.target
    if (!isField(field)) return
    const ownedMessage = invalidFields.get(field)
    if (ownedMessage !== undefined && field.validationMessage === ownedMessage)
      field.setCustomValidity('')
    invalidFields.delete(field)
  })
}
