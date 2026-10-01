import {
  FILE_AFTER_PROCESS_EVENT,
  type FileAfterProcessEvent,
} from '@/composables/useAdminWebSocket'
import { useAuthUserStore } from '@/stores/authUser'
import { useAdminProfileStore } from '@/stores/adminProfile'
import type { AdminWebSocketHandler } from '../types'

const ADMIN_PREVIEW_TRIGGER = 'admin_preview_attach'

const registerFileAfterProcessHandler: AdminWebSocketHandler = ({ bus }) => {
  const authUserStore = useAuthUserStore()
  const profileStore = useAdminProfileStore()

  const handler = (event: FileAfterProcessEvent) => {
    if (event.status !== 'completed') {
      return
    }

    if (event.eventTrigger !== ADMIN_PREVIEW_TRIGGER) {
      return
    }

    const filePath = typeof event.filePath === 'string' ? event.filePath.trim() : ''
    if (!filePath) {
      return
    }

    if (!authUserStore.authUser) {
      return
    }

    authUserStore.setAvatarUrl(filePath)
    profileStore.setAvatarUrl(filePath)
  }

  return bus.on(FILE_AFTER_PROCESS_EVENT, handler)
}

export default registerFileAfterProcessHandler
