import { t } from '@/i18n'
import { crudDisplay, type CrudField } from './index'

export function displayFieldValue(field: CrudField, value: unknown): string {
  if (value == null) return '—'
  if (field.type === 'boolean') return t(value ? 'Да' : 'Нет')
  const display = crudDisplay(field, value)
  return field.type === 'select' ? t(display) : display
}
