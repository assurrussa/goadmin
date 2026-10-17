import { readonly, ref } from 'vue'
import en from './en.json'

export type AdminLocale = 'ru' | 'en'
export type AdminTranslations = Partial<Record<AdminLocale, Record<string, string>>>
const storageKey = 'goadmin.locale'
const currentLocale = ref<AdminLocale>('ru')
const dictionaries = {
  ru: Object.create(null),
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
  currentLocale.value = value === 'en' ? 'en' : 'ru'
  if (typeof document !== 'undefined') document.documentElement.lang = currentLocale.value
  try {
    localStorage.setItem(storageKey, currentLocale.value)
  } catch {
    /* Storage may be disabled. */
  }
}

export function initLocale(): void {
  let stored: string | null = null
  try {
    stored = localStorage.getItem(storageKey)
  } catch {
    /* Use the safe default. */
  }
  setLocale(stored ?? 'ru')
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
  return { t, locale: readonly(currentLocale), setLocale, formatDateTime }
}
