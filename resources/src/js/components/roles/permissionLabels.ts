import { t } from '@/i18n'

export interface RolePermission {
  domain: string
  action: string
  description?: string | null
  key?: string
}

const domains = new Map([
  ['admins', 'Администраторы'],
  ['roles', 'Роли'],
  ['permissions', 'Разрешения'],
  ['dashboard', 'Панель управления'],
  ['users', 'Пользователи'],
  ['uploads', 'Файлы'],
  ['queues', 'Очереди'],
  ['operations', 'Служебные операции'],
  ['branding', 'Оформление'],
])
const actions = new Map([
  ['read', 'Просмотр'],
  ['create', 'Создание'],
  ['update', 'Редактирование'],
  ['delete', 'Удаление'],
  ['assign', 'Назначение'],
  ['sync', 'Синхронизация'],
])

function label(labels: Map<string, string>, key: string) {
  const message = labels.get(key)
  return message
    ? t(message)
    : key.replace(/[_-]+/g, ' ').replace(/^./u, (first) => first.toUpperCase())
}

export const domainLabel = (domain: string) => label(domains, domain)
export const actionLabel = (action: string) => label(actions, action)
export const permissionKey = (permission: RolePermission) =>
  permission.key || `${permission.domain}:${permission.action}`

export function permissionLabel(key: string) {
  const separator = key.indexOf(':')
  if (separator < 0) return key
  return `${domainLabel(key.slice(0, separator))} · ${actionLabel(key.slice(separator + 1))}`
}
