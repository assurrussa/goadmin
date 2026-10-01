import { useNotificationsStore, type Notification } from '~/stores/notifications'

/**
 * Composable для удобного управления уведомлениями
 * Позволяет добавлять, удалять и очищать уведомления из любого компонента
 */
export function useNotifications() {
  const store = useNotificationsStore()

  // Функция для показа уведомления
  const notify = (
    message: string,
    type: 'success' | 'error' | 'info' | 'warning',
    options?: Partial<Omit<Notification, 'id' | 'message' | 'type' | 'createdAt'>>,
  ) => {
    return store.add({ message, type, ...options })
  }

  // Сокращения для различных типов уведомлений
  const success = (
    message: string,
    options?: Partial<Omit<Notification, 'id' | 'message' | 'type' | 'createdAt'>>,
  ) => {
    return store.success(message, options)
  }

  const error = (
    message: string,
    options?: Partial<Omit<Notification, 'id' | 'message' | 'type' | 'createdAt'>>,
  ) => {
    return store.error(message, options)
  }

  const info = (
    message: string,
    options?: Partial<Omit<Notification, 'id' | 'message' | 'type' | 'createdAt'>>,
  ) => {
    return store.info(message, options)
  }

  const warning = (
    message: string,
    options?: Partial<Omit<Notification, 'id' | 'message' | 'type' | 'createdAt'>>,
  ) => {
    return store.warning(message, options)
  }

  // Закрытие уведомления по ID
  const close = (id: string) => {
    store.remove(id)
  }

  // Закрытие всех уведомлений
  const closeAll = () => {
    store.clearAll()
  }

  return {
    notify,
    success,
    error,
    info,
    warning,
    close,
    closeAll,
  }
}
