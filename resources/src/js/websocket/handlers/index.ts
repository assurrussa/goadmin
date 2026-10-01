import type { AdminWebSocketHandler } from '../types'
import fileAfterProcessHandler from './fileAfterProcess'
import fileDeletedHandler from './fileDeleted'
import filesUploadStatusHandler from './filesUploadStatus'

export const defaultAdminWebSocketHandlers: AdminWebSocketHandler[] = [
  filesUploadStatusHandler,
  fileAfterProcessHandler,
  fileDeletedHandler,
]
