import { t } from '@/i18n'
import type { CrudField, CrudValue } from './index'
import { belowMinimum, exactWireNumber } from './number'

export function constraintError(field: CrudField, value: CrudValue | undefined) {
  if (field.maxLength !== undefined && typeof value === 'string') {
    const length = [...value].length
    if (length > field.maxLength) {
      return t('{length} of {limit} characters', undefined, { length, limit: field.maxLength })
    }
  }
  if (
    field.minimum !== undefined &&
    (typeof value === 'number' || typeof value === 'string') &&
    value !== '' &&
    belowMinimum(value, field.minimum)
  ) {
    return t('Enter a value of at least {minimum}', undefined, { minimum: field.minimum })
  }
  if (
    field.type === 'number' &&
    typeof value === 'string' &&
    value !== '' &&
    !exactWireNumber(value)
  )
    return t('This number cannot be represented accurately')
}
