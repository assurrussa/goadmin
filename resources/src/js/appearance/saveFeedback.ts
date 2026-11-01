import { useNotifications } from '@/composables/useNotifications'
import { t } from '@/i18n'

export class AppearanceSaveError extends Error {
  constructor(readonly status: number) {
    super('Appearance save failed')
  }
}

/** Show only fixed local messages, never a server HTML/error body. */
export function notifySaveFailure(error: unknown): string {
  const status = error instanceof AppearanceSaveError ? error.status : undefined
  const message =
    status === 401 || status === 419
      ? 'Сеанс или защита запроса устарели. Войдите снова и повторите сохранение. Изменения остаются на странице.'
      : status === 403
        ? 'Нет доступа для сохранения. Изменения остаются на странице.'
        : 'Не удалось сохранить. Ваши изменения остаются на странице.'
  useNotifications().error(t(message))
  return message
}
