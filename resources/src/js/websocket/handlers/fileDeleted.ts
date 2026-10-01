import { FILE_DELETED_EVENT, type FileDeletedEvent } from '@/composables/useAdminWebSocket'
import { useAuthUserStore } from '@/stores/authUser'
import { useAdminProfileStore } from '@/stores/adminProfile'
import type { AdminWebSocketHandler } from '../types'
import { normalizeUrl } from './utils'

const registerFileDeletedHandler: AdminWebSocketHandler = ({ bus }) => {
  const authUserStore = useAuthUserStore()
  const profileStore = useAdminProfileStore()

  const handler = (event: FileDeletedEvent) => {
    if (event.status !== 'completed') {
      return
    }

    const filePath = event.filePath
    if (!filePath) {
      return
    }

    const authUser = authUserStore.authUser
    if (!authUser) {
      return
    }

    const normalizedEventPath = normalizeUrl(filePath)
    const knownAvatars = [
      normalizeUrl(authUser.avatarUrl ?? null),
      normalizeUrl(profileStore.avatarUrl),
    ].filter((value): value is string => Boolean(value))

    const matchesKnownAvatar = knownAvatars.some((url) => url === normalizedEventPath)

    if (!matchesKnownAvatar) {
      return
    }

    authUserStore.setAvatarUrl(null)
    profileStore.setAvatarUrl(null)
  }

  return bus.on(FILE_DELETED_EVENT, handler)
}

export default registerFileDeletedHandler
