import {
  FILE_UPLOAD_STATUS_EVENT,
  type FileUploadStatusEvent,
} from '@/composables/useAdminWebSocket'
import { useAuthUserStore } from '@/stores/authUser'
import { useAdminProfileStore } from '@/stores/adminProfile'
import type { AdminWebSocketHandler } from '../types'

const registerFilesUploadStatusHandler: AdminWebSocketHandler = ({ bus }) => {
  const authUserStore = useAuthUserStore()
  const profileStore = useAdminProfileStore()

  const handler = (event: FileUploadStatusEvent) => {
    if (event.status !== 'completed') {
      return
    }

    const meta = event.metadata ?? {}
    const objectType = typeof meta.objectType === 'string' ? meta.objectType.toLowerCase() : ''
    if (objectType !== 'admin') {
      return
    }

    const authUser = authUserStore.authUser
    if (!authUser) {
      return
    }

    const metaObjectId = meta.objectId != null ? String(meta.objectId) : null
    if (metaObjectId && String(authUser.id) !== metaObjectId) {
      return
    }

    const nextUrl = event.file?.url || (typeof meta.fileUrl === 'string' ? meta.fileUrl : undefined)
    if (!nextUrl) {
      return
    }

    authUserStore.setAvatarUrl(nextUrl)
    profileStore.setAvatarUrl(nextUrl)
  }

  return bus.on(FILE_UPLOAD_STATUS_EVENT, handler)
}

export default registerFilesUploadStatusHandler
