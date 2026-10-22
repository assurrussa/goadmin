import { t } from '@/i18n'
import { inject, onUnmounted, provide, watch, type InjectionKey } from 'vue'
import { useAuthUserStore } from '@/stores/authUser'
import { useUploadQueueStore } from '@/stores/uploadQueue'
import { useAdminCapabilities } from './useAdminCapabilities'
import { useAdminWebSocket, type FileUploadStatusEvent } from './useAdminWebSocket'
import { pollUploadTask } from '@/services/pollUploadTask'
import type { UploadTaskData, UploadedFileSummary } from '@/services/fileUploadService'

type Owner = ReturnType<typeof useAuthUserStore>['id']
interface OwnerToken {
  owner: Owner
  generation: number
}
export interface UploadTaskSnapshot {
  status: string
  file?: UploadedFileSummary
  error?: string
}
export interface UploadTaskObservation {
  subscribe: (listener: (snapshot: UploadTaskSnapshot) => void, onCancel?: () => void) => () => void
}
interface Entry {
  id: number
  token: OwnerToken
  queueIds: Set<string>
  snapshot: UploadTaskSnapshot
  listeners: Set<(snapshot: UploadTaskSnapshot) => void>
  cancellations: Set<() => void>
  controller?: AbortController
  attempts: number
  interval: number
  deadline?: ReturnType<typeof setTimeout>
}
const key: InjectionKey<ReturnType<typeof provideUploadTaskTracker>> = Symbol('uploadTaskTracker')
const terminal = (status: string) => ['completed', 'failed', 'error'].includes(status)

/** Frame-owned server observation only. Never releases a completion fence or assigns a field. */
export function provideUploadTaskTracker() {
  const auth = useAuthUserStore()
  const queue = useUploadQueueStore()
  const { uploads, realtime } = useAdminCapabilities()
  const socket = useAdminWebSocket()
  const entries = new Map<number, Entry>()
  let generation = 0
  let disposed = false
  const capture = (): OwnerToken => ({ owner: auth.id, generation })
  const isCurrent = (token: OwnerToken) =>
    !disposed &&
    token.owner !== null &&
    uploads.value &&
    token.owner === auth.id &&
    token.generation === generation
  const active = (entry: Entry) => isCurrent(entry.token) && entries.get(entry.id) === entry

  const prune = () => {
    const visible = new Set(queue.uploads.map((item) => item.id))
    for (const [id, entry] of entries) {
      if (
        terminal(entry.snapshot.status) &&
        ![...entry.queueIds].some((queueId) => visible.has(queueId))
      ) {
        entry.listeners.clear()
        entries.delete(id)
      }
    }
  }
  const stopRows = watch(() => queue.uploads.map((item) => item.id), prune, { flush: 'sync' })
  const publish = (entry: Entry, snapshot: UploadTaskSnapshot) => {
    if (!active(entry) || terminal(entry.snapshot.status)) return
    // Missing final file metadata cannot be used to claim a usable completion.
    if (snapshot.status === 'completed' && !snapshot.file?.url) {
      snapshot = {
        status: 'unknown',
        error: t(
          'core.ui.Processing finished, but file details are unavailable.',
          'Processing finished, but file details are unavailable.',
        ),
      }
    }
    entry.snapshot = snapshot
    for (const id of entry.queueIds) {
      if (snapshot.status === 'completed') queue.complete(id)
      else if (snapshot.status === 'failed' || snapshot.status === 'error')
        queue.fail(id, snapshot.error)
      else
        queue.setStatus(
          id,
          snapshot.status === 'unknown' ? 'unknown' : 'processing',
          snapshot.error,
        )
    }
    if (terminal(snapshot.status)) {
      clearTimeout(entry.deadline)
      entry.controller?.abort()
      entry.controller = undefined
    }
    for (const listener of [...entry.listeners]) listener(snapshot)
    if (terminal(snapshot.status)) {
      entry.listeners.clear()
      entry.cancellations.clear()
      prune()
    }
  }
  const observe = (entry: Entry) => {
    if (!active(entry) || terminal(entry.snapshot.status) || entry.controller) return
    const controller = new AbortController()
    entry.controller = controller
    queueStatus(entry, 'processing')
    // Bound even a stalled HTTP request; a deadline says nothing about server failure.
    entry.deadline = setTimeout(
      () => {
        if (!active(entry) || entry.controller !== controller) return
        publish(entry, {
          status: 'unknown',
          error: t(
            'core.ui.Processing status check timed out. Check again.',
            'Processing status check timed out. Check again.',
          ),
        })
        controller.abort()
        entry.controller = undefined
      },
      entry.attempts * entry.interval + 15_000,
    )
    void pollUploadTask(entry.id, {
      signal: controller.signal,
      maxAttempts: entry.attempts,
      intervalMs: entry.interval,
    })
      .then((result) => {
        if (controller.signal.aborted || !active(entry)) return
        publish(entry, {
          status: (result.task.status || result.status).toLowerCase(),
          file: result.file ?? result.task.file,
          error: result.error ?? result.task.error,
        })
      })
      .catch((reason: unknown) => {
        if (controller.signal.aborted || !active(entry)) return
        publish(entry, {
          status: 'unknown',
          error: reason instanceof Error ? reason.message : t('Не удалось проверить статус файла.'),
        })
      })
      .finally(() => {
        if (entry.controller === controller) {
          clearTimeout(entry.deadline)
          entry.controller = undefined
        }
      })
  }
  const queueStatus = (entry: Entry, status: 'processing') => {
    entry.snapshot = { status }
    for (const id of entry.queueIds) queue.setStatus(id, status)
  }
  const accept = (
    token: OwnerToken,
    task: UploadTaskData,
    responseStatus: string,
    queueId: string,
    policy = { attempts: 120, interval: 1000 },
  ): UploadTaskObservation | undefined => {
    if (!isCurrent(token) || !task.id) return
    let entry = entries.get(task.id)
    if (!entry) {
      entry = {
        id: task.id,
        token,
        queueIds: new Set(),
        snapshot: { status: 'processing' },
        listeners: new Set(),
        cancellations: new Set(),
        ...policy,
      }
      entries.set(task.id, entry)
    }
    const existingRow = queue.uploads.find((item) => entry.queueIds.has(item.id))
    if (existingRow && existingRow.id !== queueId) queue.remove(queueId)
    else {
      entry.queueIds.clear()
      entry.queueIds.add(queueId)
      queue.bindRecheck(queueId, () => observe(entry))
    }
    if (terminal(entry.snapshot.status)) {
      for (const id of entry.queueIds) {
        if (entry.snapshot.status === 'completed') queue.complete(id)
        else queue.fail(id, entry.snapshot.error)
      }
    } else {
      publish(entry, {
        status: (task.status || responseStatus).toLowerCase(),
        file: task.file,
        error: task.error,
      })
      observe(entry)
    }
    const observed = entry
    return {
      subscribe(listener, onCancel) {
        if (!isCurrent(observed.token)) {
          onCancel?.()
          return () => {}
        }
        if (terminal(observed.snapshot.status)) {
          listener(observed.snapshot)
          return () => {}
        }
        if (!active(observed)) return () => {}
        observed.listeners.add(listener)
        if (onCancel) observed.cancellations.add(onCancel)
        listener(observed.snapshot)
        return () => {
          observed.listeners.delete(listener)
          if (onCancel) observed.cancellations.delete(onCancel)
        }
      },
    }
  }
  const stop = () => {
    generation++
    for (const entry of entries.values()) {
      clearTimeout(entry.deadline)
      entry.controller?.abort()
      for (const cancel of [...entry.cancellations]) cancel()
      entry.cancellations.clear()
      entry.listeners.clear()
    }
    entries.clear()
    queue.clear()
  }
  const stopOwner = watch(() => auth.id, stop, { flush: 'sync' })
  const stopUploads = watch(
    uploads,
    (enabled) => {
      if (!enabled) stop()
    },
    { flush: 'sync' },
  )
  // Subscribe once for this frame; navigation only detaches widget observers.
  const off = socket.on('files.upload.status', (event: FileUploadStatusEvent) => {
    const entry = entries.get(event.taskId)
    if (!entry || !realtime.value || !active(entry)) return
    publish(entry, {
      status: event.status.toLowerCase(),
      error: event.error,
      file: event.file
        ? {
            ...event.file,
            filename: event.file.fileName,
            originalName: event.file.originalName ?? event.file.fileName,
            url:
              event.file.url ||
              (typeof event.metadata?.fileUrl === 'string' ? event.metadata.fileUrl : ''),
            publicUrl:
              typeof event.metadata?.fileUrl === 'string' ? event.metadata.fileUrl : event.file.url,
            fullPath:
              typeof event.metadata?.fileUrl === 'string' ? event.metadata.fileUrl : event.file.url,
            entityId: (event.metadata?.objectId as number | null) ?? null,
            sortOrder: 0,
            fileType: event.file.mimeType?.split('/')[0] ?? '',
          }
        : undefined,
    })
  })
  const tracker = { capture, isCurrent, accept }
  provide(key, tracker)
  onUnmounted(() => {
    disposed = true
    stopOwner()
    stopUploads()
    stopRows()
    off?.()
    stop()
  })
  return tracker
}

export function useUploadTaskTracker() {
  const tracker = inject(key)
  if (!tracker) throw new Error('Upload widgets require AdminRuntimeFrame.')
  return tracker
}
