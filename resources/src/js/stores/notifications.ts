import { defineStore } from 'pinia'
import { v4 as uuidv4 } from 'uuid'

export type NotificationType = 'success' | 'error' | 'info' | 'warning'

export interface Notification {
  id: string
  message: string
  type: NotificationType
  title?: string
  timeout?: number // Время в мс, после которого уведомление автоматически скроется
  closable?: boolean // Можно ли закрыть уведомление
  createdAt: number // Timestamp создания
}

export const useNotificationsStore = defineStore('notifications', {
  state: () => ({
    notifications: [] as Notification[],
    defaultTimeout: 5000, // 5 секунд по умолчанию
  }),

  actions: {
    add(
      notification: Partial<Notification> & {
        message: string
        type: NotificationType
      },
    ): string {
      const id = notification.id || uuidv4()

      const newNotification: Notification = {
        id,
        message: notification.message,
        type: notification.type,
        title: notification.title || this.getDefaultTitle(notification.type),
        timeout: notification.timeout !== undefined ? notification.timeout : this.defaultTimeout,
        closable: notification.closable !== undefined ? notification.closable : true,
        createdAt: Date.now(),
      }

      this.notifications.push(newNotification)

      // Таймаут перенесен в компонент NotificationItem для лучшего контроля
      // при наведении мыши и для реализации прогресс-бара

      return id
    },

    remove(id: string): void {
      const index = this.notifications.findIndex((n) => n.id === id)
      if (index !== -1) {
        this.notifications.splice(index, 1)
      }
    },

    clearAll(): void {
      this.notifications = []
    },

    // Вспомогательные методы для добавления уведомлений разных типов
    success(
      message: string,
      options?: Partial<Omit<Notification, 'id' | 'message' | 'type' | 'createdAt'>>,
    ): string {
      return this.add({ message, type: 'success', ...options })
    },

    error(
      message: string,
      options?: Partial<Omit<Notification, 'id' | 'message' | 'type' | 'createdAt'>>,
    ): string {
      return this.add({ message, type: 'error', ...options })
    },

    info(
      message: string,
      options?: Partial<Omit<Notification, 'id' | 'message' | 'type' | 'createdAt'>>,
    ): string {
      return this.add({ message, type: 'info', ...options })
    },

    warning(
      message: string,
      options?: Partial<Omit<Notification, 'id' | 'message' | 'type' | 'createdAt'>>,
    ): string {
      return this.add({ message, type: 'warning', ...options })
    },

    // Вспомогательный метод для получения заголовка по умолчанию
    getDefaultTitle(type: NotificationType): string {
      switch (type) {
        case 'success':
          return 'Успешно'
        case 'error':
          return 'Ошибка'
        case 'info':
          return 'Информация'
        case 'warning':
          return 'Предупреждение'
        default:
          return ''
      }
    },
  },
})
