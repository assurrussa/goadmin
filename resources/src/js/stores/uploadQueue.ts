import { defineStore } from 'pinia'
import { v4 as uuidv4 } from 'uuid'

export type UploadStatus = 'uploading' | 'processing' | 'completed' | 'error' | 'unknown'

export interface UploadItem {
  id: string
  label: string
  progress: number
  status: UploadStatus
  error?: string
  createdAt: number
}

const timers = new WeakMap<object, Map<string, ReturnType<typeof setTimeout>>>()
const rechecks = new WeakMap<object, Map<string, () => void>>()

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
      item.error = error
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

    bindRecheck(id: string, recheck: () => void): void {
      if (!rechecks.has(this)) rechecks.set(this, new Map())
      rechecks.get(this)!.set(id, recheck)
    },

    recheck(id: string): void {
      rechecks.get(this)?.get(id)?.()
    },

    remove(id: string): void {
      const timer = timers.get(this)?.get(id)
      if (timer !== undefined) clearTimeout(timer)
      timers.get(this)?.delete(id)
      rechecks.get(this)?.delete(id)
      const index = this.uploads.findIndex((upload) => upload.id === id)
      if (index !== -1) {
        this.uploads.splice(index, 1)
      }
    },

    clear(): void {
      timers.get(this)?.forEach((timer) => clearTimeout(timer))
      timers.delete(this)
      rechecks.delete(this)
      this.uploads = []
    },

    scheduleRemove(id: string): void {
      if (typeof window === 'undefined') {
        this.remove(id)
        return
      }
      if (!timers.has(this)) timers.set(this, new Map())
      const pending = timers.get(this)!
      if (pending.has(id)) return
      pending.set(
        id,
        setTimeout(() => {
          this.remove(id)
        }, this.dismissDelay),
      )
    },
  },
})
