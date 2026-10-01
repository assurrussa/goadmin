export interface AdminAuthUser {
  id: string
  name: string
  lastName: string
  email: string
  roles: string[]
  avatarUrl: string
}

export interface AdminAvatarFile {
  id: number
  url: string
  fileName?: string
  mimeType?: string
  size?: number
  thumbnail?: string
  width?: number
  height?: number
}

export interface AdminNotificationPayload {
  url?: string
  href?: string
  link?: string
  ctaText?: string
  [key: string]: unknown
}

export interface AdminNotification {
  id: number
  adminId?: number
  title: string
  message: string
  level: string
  isRead: boolean
  createdAt: string
  updatedAt?: string
  readAt?: string | null
  payload?: AdminNotificationPayload | null
}
