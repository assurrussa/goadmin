import { defineStore } from 'pinia'
import { v4 as uuidv4 } from 'uuid'

export type UploadStatus = 'uploading' | 'processing' | 'completed' | 'error'

export interface UploadItem {
  id: string
  label: string
  progress: number
  status: UploadStatus
  error?: string
  createdAt: number
}

export const useUploadQueueStore = defineStore('uploadQueue', {
  state: () => ({
    uploads: [] as UploadItem[],
    dismissDelay: 2500,
  }),

  actions: {
    start(label: string, progress = 0): string {
      const id = uuidv4()
      const nextProgress = Math.min(100, Math.max(0, Math.round(progress)))
      this.uploads.push({
        id,
        label: label.trim() || 'Uploading file',
        progress: nextProgress,
        status: 'uploading',
        createdAt: Date.now(),
      })
      return id
    },

    update(id: string, progress: number): void {
      const item = this.uploads.find((upload) => upload.id === id)
      if (!item) {
        return
      }
      const nextProgress = Math.min(100, Math.max(0, Math.round(progress)))
      if (nextProgress < item.progress && item.status === 'uploading') {
        return
      }
      item.progress = nextProgress
    },

    setStatus(id: string, status: UploadStatus, error?: string): void {
      const item = this.uploads.find((upload) => upload.id === id)
      if (!item) {
        return
      }
      item.status = status
      if (error) {
        item.error = error
      }
    },

    complete(id: string): void {
      const item = this.uploads.find((upload) => upload.id === id)
      if (!item) {
        return
      }
      item.progress = 100
      item.status = 'completed'
      this.scheduleRemove(id)
    },

    fail(id: string, error?: string): void {
      const item = this.uploads.find((upload) => upload.id === id)
      if (!item) {
        return
      }
      item.status = 'error'
      if (error) {
        item.error = error
      }
      this.scheduleRemove(id)
    },

    remove(id: string): void {
      const index = this.uploads.findIndex((upload) => upload.id === id)
      if (index !== -1) {
        this.uploads.splice(index, 1)
      }
    },

    clear(): void {
      this.uploads = []
    },

    scheduleRemove(id: string): void {
      if (typeof window === 'undefined') {
        this.remove(id)
        return
      }
      window.setTimeout(() => {
        this.remove(id)
      }, this.dismissDelay)
    },
  },
})
