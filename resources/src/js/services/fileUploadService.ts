import axios, { type AxiosRequestConfig, type AxiosResponse } from 'axios'
import { resolveTusEndpoint } from './tusEndpoint'

type EntityID = number | string

type RawFile = {
  id: number
  status?: string
  entityId?: number | null
  filename: string
  originalName: string
  url: string
  publicUrl?: string
  fullPath?: string
  fileType: string
  sortOrder?: number | null
  mimeType?: string
  size?: number
  width?: number
  height?: number
  folderPath?: string
  thumbnailUrl?: string
  isPrimary?: boolean
  createdAt?: string
  updatedAt?: string
  data?: Record<string, unknown> | null
}

type RawTask = {
  id?: number
  taskId?: number
  status: string
  tempUrl?: string | null
  filename?: string
  fileName?: string
  originalName?: string
  mimeType?: string
  size?: number
  entityId?: number | null
  replaceId?: number | null
  error?: string
  file?: RawFile | null
}

type RawUploadResponse = {
  status?: string
  tasks?: RawTask[]
  task?: RawTask
  file?: RawUploadFileEnvelope | RawTask | null
  error?: string
}

type RawUploadFileEnvelope = {
  id: number
  taskId?: number
  status: string
  tempUrl?: string | null
  url: string
  filename?: string
  fileName?: string
  originalName?: string
  mimeType?: string
  size?: number
  replaceId?: number | null
  entityId?: number | null
  fileType?: string
  publicUrl?: string
  fullPath?: string
  sortOrder?: number | null
  width?: number
  height?: number
  folderPath?: string
  thumbnailUrl?: string
  isPrimary?: boolean
  createdAt?: string
  updatedAt?: string
  data?: Record<string, unknown> | null
}

type RawTaskResponse = {
  status: string
  task: RawTask
  file?: RawFile | null
  error?: string
}

type RawDeleteResponse = {
  status: string
  id: number
  entity_id?: number | null
}

type RawListResponse = {
  files: RawFile[]
  entity_id?: number | null
}

export interface UploadedFileSummary {
  id: number
  entityId: number | null
  filename: string
  originalName: string
  url: string
  publicUrl?: string
  fullPath?: string
  fileType: string
  sortOrder: number
  mimeType?: string
  size?: number
  width?: number
  height?: number
  folderPath?: string
  thumbnailUrl?: string | null
  isPrimary?: boolean
  createdAt?: string
  updatedAt?: string
  data?: Record<string, unknown> | null
}

export interface UploadTaskData {
  id: number
  status: string
  tempUrl?: string
  filename?: string
  mimeType?: string
  size?: number
  fileType?: string
  publicUrl?: string
  fullPath?: string
  folderPath?: string
  thumbnailUrl?: string | null
  width?: number
  height?: number
  isPrimary?: boolean
  createdAt?: string
  updatedAt?: string
  data?: Record<string, unknown> | null
  entityId?: number | null
  replaceId?: number | null
  error?: string
  file?: UploadedFileSummary
}

export interface UncertainUploadCompletion {
  location: string
  filename: string
  mimeType?: string
  size?: number
  context?: string
  fileCategory?: string
  entityType: string
  entityId: EntityID
  replaceFileId?: number
}

export interface UploadResponse {
  status: string
  tasks: UploadTaskData[]
  error?: string
  uncertainCompletion?: UncertainUploadCompletion
}

export interface UploadTaskStatusResponse {
  status: string
  task: UploadTaskData
  file?: UploadedFileSummary
  error?: string
}

export interface DeleteResponse {
  status: string
  id: number
  entityId: number | null
}

export interface ListFilesResponse {
  files: UploadedFileSummary[]
  entityId: number | null
}

export interface UploadTransportOptions {
  signal?: AbortSignal
  /** Maximum inactivity per HTTP request, reset by network progress. Default: 120s. */
  requestTimeoutMs?: number
}

export interface UploadRequest extends UploadTransportOptions {
  /** Called before completion dispatch, so cancellation can retain the session. */
  onCompletionSession?: (completion: UncertainUploadCompletion) => void
  entityType: string
  entityId: EntityID | null | undefined
  file?: File
  files?: File[]
  replaceFileId?: number
  context?: string
  fileCategory?: string
  uploadUrl?: string
  skipResize?: boolean
  onProgress?: (percentage: number) => void
}

const ensureEntity = (entityId: EntityID | null | undefined): string => {
  if (entityId === null || entityId === undefined || entityId === '') {
    throw new Error('entity id is required for file uploads')
  }
  return String(entityId)
}

const toOptionalUrl = (value?: string | null): string | null => {
  if (!value) {
    return null
  }
  const trimmed = value.trim()
  return trimmed.length > 0 ? trimmed : null
}

const resolveTempUrl = (source?: unknown, fallback?: string | null): string | undefined => {
  const raw = source as Record<string, unknown> | undefined
  const tempUrl =
    raw?.tempUrl ?? raw?.temp_url ?? raw?.tempFileUrl ?? raw?.temp_file_url ?? fallback

  return toOptionalUrl(typeof tempUrl === 'string' ? tempUrl : undefined) ?? undefined
}

const normalizeFile = (file?: RawFile | null): UploadedFileSummary | undefined => {
  if (!file) {
    return undefined
  }
  return {
    id: file.id,
    entityId: file.entityId ?? null,
    filename: file.filename ?? file.originalName ?? '',
    originalName: file.originalName ?? file.filename ?? '',
    url: file.url,
    publicUrl: file.publicUrl ?? file.url,
    fullPath: file.fullPath ?? file.url,
    fileType: file.fileType ?? 'file',
    sortOrder: file.sortOrder ?? 0,
    mimeType: file.mimeType,
    size: typeof file.size === 'number' ? file.size : undefined,
    width: typeof file.width === 'number' ? file.width : undefined,
    height: typeof file.height === 'number' ? file.height : undefined,
    folderPath: file.folderPath,
    thumbnailUrl: toOptionalUrl(file.thumbnailUrl),
    isPrimary: file.isPrimary ?? undefined,
    createdAt: file.createdAt,
    updatedAt: file.updatedAt,
    data: file.data ?? null,
  }
}

const normalizeTask = (task: RawTask): UploadTaskData => {
  const normalizedFile = normalizeFile(task.file)
  const tempUrl =
    resolveTempUrl(task, normalizedFile?.publicUrl ?? normalizedFile?.url) ??
    normalizedFile?.thumbnailUrl ??
    undefined

  return {
    id: task.id ?? 0,
    status: task.status,
    tempUrl,
    filename: task.filename ?? task.fileName ?? task.originalName,
    mimeType: task.mimeType ?? normalizedFile?.mimeType,
    size: task.size ?? normalizedFile?.size,
    fileType: task.file?.fileType ?? normalizedFile?.fileType,
    publicUrl: normalizedFile?.publicUrl,
    fullPath: normalizedFile?.fullPath,
    folderPath: normalizedFile?.folderPath,
    thumbnailUrl: normalizedFile?.thumbnailUrl ?? null,
    width: normalizedFile?.width,
    height: normalizedFile?.height,
    isPrimary: normalizedFile?.isPrimary,
    createdAt: normalizedFile?.createdAt,
    updatedAt: normalizedFile?.updatedAt,
    data: normalizedFile?.data ?? null,
    entityId: task.entityId ?? normalizedFile?.entityId ?? null,
    replaceId: task.replaceId ?? null,
    error: task.error,
    file: normalizedFile,
  }
}

const normalizeTasks = (tasks?: RawTask[]): UploadTaskData[] => {
  if (!tasks?.length) {
    return []
  }
  return tasks.map(normalizeTask)
}

const buildTaskFromFileEnvelope = (payload: RawUploadFileEnvelope): RawTask => {
  const filename = payload.filename ?? payload.fileName ?? payload.originalName ?? ''
  const originalName = payload.originalName ?? filename
  const tempUrl = resolveTempUrl(
    payload,
    payload.tempUrl ?? payload.url ?? payload.publicUrl ?? null,
  )

  return {
    id: payload.taskId ?? payload.id,
    taskId: payload.taskId,
    status: payload.status ?? 'queued',
    tempUrl,
    filename,
    fileName: filename,
    originalName,
    mimeType: payload.mimeType,
    size: payload.size,
    entityId: payload.entityId ?? null,
    replaceId: payload.replaceId ?? null,
    file: {
      id: payload.id,
      entityId: payload.entityId ?? null,
      filename,
      originalName,
      url: payload.url,
      publicUrl: payload.publicUrl ?? payload.url,
      fullPath: payload.fullPath ?? payload.url,
      fileType: payload.fileType ?? payload.mimeType ?? 'file',
      sortOrder: payload.sortOrder ?? 0,
      mimeType: payload.mimeType,
      size: payload.size,
      width: payload.width,
      height: payload.height,
      folderPath: payload.folderPath,
      thumbnailUrl: toOptionalUrl(payload.thumbnailUrl) || undefined,
      isPrimary: payload.isPrimary,
      createdAt: payload.createdAt,
      updatedAt: payload.updatedAt,
      data: payload.data ?? null,
    },
  }
}

const normalizeUploadFilePayload = (payload: RawUploadResponse['file']): RawTask | undefined => {
  if (!payload) {
    return undefined
  }

  if (typeof payload === 'object' && 'url' in payload && typeof payload.url === 'string') {
    return buildTaskFromFileEnvelope(payload as RawUploadFileEnvelope)
  }

  return payload as RawTask
}

const normalizeUploadResponsePayload = (
  data: RawUploadResponse,
): { status: string; tasks: RawTask[]; error?: string } => {
  const tasks: RawTask[] = []

  if (Array.isArray(data.tasks) && data.tasks.length > 0) {
    tasks.push(...data.tasks)
  }

  if (data.task) {
    tasks.push(data.task)
  }

  const payloadTask = normalizeUploadFilePayload(data.file)
  if (payloadTask) {
    tasks.push(payloadTask)
  }

  for (const task of tasks) {
    if (!task.status && task.file && task.file.url) {
      task.status = 'completed'
    }
  }

  const status = data.status ?? tasks[0]?.status ?? 'queued'

  return {
    status,
    tasks,
    error: data.error,
  }
}

const buildQueuedResponse = (taskId: number): UploadTaskStatusResponse => ({
  status: 'queued',
  task: {
    id: taskId,
    status: 'queued',
  },
})

const normalizeTaskResponse = (taskId: number, data: RawTaskResponse): UploadTaskStatusResponse => {
  if (data.status === 'error' && data.error === 'task not found') {
    return buildQueuedResponse(taskId)
  }

  const normalizedFile = normalizeFile(data.file ?? data.task?.file ?? undefined)
  const resolvedTempUrl = resolveTempUrl(
    data.task ?? data,
    normalizedFile?.publicUrl ?? normalizedFile?.url,
  )
  let status =
    data.status ||
    data.task?.status ||
    data.file?.status ||
    data.task?.file?.status ||
    (normalizedFile?.data?.status as string) ||
    'queued'

  if (normalizedFile?.url && (!status || status === 'queued' || status === 'processing')) {
    status = 'completed'
  }

  const taskPayload: UploadTaskData =
    data.task && data.task.id
      ? normalizeTask(data.task)
      : {
          id: taskId,
          status,
          tempUrl: resolvedTempUrl,
          filename: normalizedFile?.filename ?? normalizedFile?.originalName,
          mimeType: normalizedFile?.mimeType,
          size: normalizedFile?.size,
          fileType: normalizedFile?.fileType,
          publicUrl: normalizedFile?.publicUrl,
          fullPath: normalizedFile?.fullPath,
          folderPath: normalizedFile?.folderPath,
          thumbnailUrl: normalizedFile?.thumbnailUrl ?? null,
          width: normalizedFile?.width,
          height: normalizedFile?.height,
          isPrimary: normalizedFile?.isPrimary,
          createdAt: normalizedFile?.createdAt,
          updatedAt: normalizedFile?.updatedAt,
          data: normalizedFile?.data ?? null,
          entityId: normalizedFile?.entityId ?? null,
          replaceId: null,
          error: data.error,
          file: normalizedFile,
        }

  return {
    status,
    task: { ...taskPayload, status },
    file: normalizedFile,
    error: data.error,
  }
}

const TUS_VERSION = '1.0.0'
const TUS_CONTENT_TYPE = 'application/offset+octet-stream'
const TUS_CHUNK_SIZE = 5 * 1024 * 1024

// Bound a stalled request, not the duration of the entire upload. Aborting the
// transport also stops pending XHRs; the local rejection fences late adapters.
const tusRequest = <T>(
  options: UploadTransportOptions,
  send: (config: AxiosRequestConfig) => Promise<T>,
): Promise<T> =>
  new Promise((resolve, reject) => {
    const timeout = options.requestTimeoutMs ?? 120_000
    if (!Number.isFinite(timeout) || timeout <= 0) {
      reject(new Error('Upload request timeout must be a positive number'))
      return
    }
    const controller = new AbortController()
    let settled = false
    let timer: ReturnType<typeof setTimeout>
    const cleanup = () => {
      clearTimeout(timer)
      options.signal?.removeEventListener('abort', cancel)
    }
    const stop = (error: Error) => {
      if (settled) return
      settled = true
      cleanup()
      controller.abort()
      reject(error)
    }
    const cancel = () => stop(new Error('Upload cancelled'))
    const progress = () => {
      if (settled) return
      clearTimeout(timer)
      timer = setTimeout(
        () => stop(new Error('Upload request timed out. Check your connection and try again.')),
        timeout,
      )
    }
    if (options.signal?.aborted) {
      cancel()
      return
    }
    options.signal?.addEventListener('abort', cancel, { once: true })
    progress()
    Promise.resolve()
      .then(() => {
        if (settled) throw new Error('Upload cancelled')
        return send({
          signal: controller.signal,
          onUploadProgress: progress,
          onDownloadProgress: progress,
        })
      })
      .then(
        (value) => {
          if (settled) return
          settled = true
          cleanup()
          resolve(value)
        },
        (error) => {
          if (settled) return
          settled = true
          cleanup()
          reject(error)
        },
      )
  })

const readTusOffset = (response: AxiosResponse): number => {
  const header = response.headers['upload-offset'] ?? response.headers['Upload-Offset']
  if (header === undefined || !/^\d+$/.test(String(header)))
    throw new Error('Invalid upload offset')
  const offset = Number(header)
  if (!Number.isSafeInteger(offset)) throw new Error('Invalid upload offset')
  return offset
}

const encodeTusValue = (value: string): string => {
  if (typeof btoa === 'function') {
    const bytes = new TextEncoder().encode(value)
    let binary = ''
    for (const item of bytes) {
      binary += String.fromCharCode(item)
    }
    return btoa(binary)
  }

  const buffer = (
    globalThis as unknown as {
      Buffer: { from: (v: string, enc: string) => { toString: (fmt: string) => string } }
    }
  )?.Buffer
  if (buffer) {
    return buffer.from(value, 'utf-8').toString('base64')
  }

  throw new Error('base64 encoding is not available')
}

const buildTusMetadata = (
  metadata: Record<string, string | number | boolean | null | undefined>,
): string => {
  const entries = Object.entries(metadata)
    .filter(([, value]) => value !== undefined && value !== null && value !== '')
    .map(([key, value]) => `${key} ${encodeTusValue(String(value))}`)

  return entries.join(',')
}

const readTusError = (error: unknown): string => {
  if (axios.isAxiosError(error) && error.response) {
    const data = error.response.data as { error?: string; errors?: Record<string, string> }
    if (data?.error && typeof data.error === 'string') {
      return data.error
    }
    if (data?.errors && typeof data.errors === 'object') {
      return Object.values(data.errors).join(', ')
    }
    return error.response.statusText || `Upload failed with status ${error.response.status}`
  }
  return error instanceof Error ? error.message : String(error)
}

const createTusUpload = async (
  file: File,
  metadata: Record<string, string | number | boolean | null | undefined>,
  uploadUrl?: string,
  options: UploadTransportOptions = {},
): Promise<string> => {
  try {
    const endpoint = resolveTusEndpoint(uploadUrl)
    const response = await tusRequest(options, (config) =>
      axios.post(
        endpoint,
        {},
        {
          ...config,
          headers: {
            'Tus-Resumable': TUS_VERSION,
            'Upload-Length': String(file.size),
            'Upload-Metadata': buildTusMetadata(metadata),
          },
        },
      ),
    )

    // Axios headers keys are usually lowercased
    let location = response.headers['location'] || response.headers['Location']
    if (!location) {
      throw new Error('missing upload location')
    }

    // Resolve relative location against the endpoint
    if (!/^https?:\/\//.test(location) && !location.startsWith('/')) {
      const separatorIndex = endpoint.lastIndexOf('/')
      if (separatorIndex !== -1) {
        const baseUrl = endpoint.substring(0, separatorIndex + 1)
        location = baseUrl + location
      } else {
        location = '/' + location
      }
    }

    return location
  } catch (error) {
    throw new Error(readTusError(error))
  }
}

const fetchTusOffset = async (
  uploadUrl: string,
  options: UploadTransportOptions = {},
): Promise<number> => {
  try {
    const response = await tusRequest(options, (config) =>
      axios.head(uploadUrl, {
        ...config,
        headers: { 'Tus-Resumable': TUS_VERSION },
      }),
    )
    return readTusOffset(response)
  } catch (error) {
    throw new Error(readTusError(error))
  }
}

const patchTusChunk = async (
  uploadUrl: string,
  offset: number,
  chunk: Blob,
  options: UploadTransportOptions = {},
): Promise<AxiosResponse> => {
  try {
    return await tusRequest(options, (config) =>
      axios.patch(uploadUrl, chunk, {
        ...config,
        headers: {
          'Tus-Resumable': TUS_VERSION,
          'Upload-Offset': String(offset),
          'Content-Type': TUS_CONTENT_TYPE,
        },
      }),
    )
  } catch (error) {
    if (axios.isAxiosError(error) && error.response) {
      // Return response for 409 handling in loop
      return error.response
    }
    throw new Error(readTusError(error))
  }
}

const UNCERTAIN_COMPLETION_MESSAGE =
  'Upload completion is unconfirmed. Check this upload before starting another.'

class UncertainCompletionError extends Error {
  constructor(readonly completion: UncertainUploadCompletion) {
    super(UNCERTAIN_COMPLETION_MESSAGE)
  }
}

const finalizeTusUpload = async (
  completion: UncertainUploadCompletion,
  options: UploadTransportOptions = {},
  onDispatch?: (completion: UncertainUploadCompletion) => void,
): Promise<RawUploadResponse> => {
  const endpoint = completion.location.replace(/\/$/, '')
  let dispatched = false
  try {
    const response = await tusRequest(options, (config) => {
      onDispatch?.(completion)
      dispatched = true
      return axios.post<RawUploadResponse>(
        `${endpoint}/complete`,
        {},
        {
          ...config,
          headers: { 'Tus-Resumable': TUS_VERSION },
        },
      )
    })
    if (!response.data.error) {
      const tasks = normalizeTasks(normalizeUploadResponsePayload(response.data).tasks)
      if (tasks.length !== 1 || !Number.isSafeInteger(tasks[0].id) || tasks[0].id <= 0) {
        throw new UncertainCompletionError(completion)
      }
    }
    return response.data
  } catch (error) {
    // A response can be lost after durable finalization. Keep the same session
    // for explicit reconciliation; creating a fresh upload can duplicate work.
    if (
      dispatched &&
      (!axios.isAxiosError(error) ||
        !error.response ||
        error.response.status >= 500 ||
        [408, 409].includes(error.response.status))
    ) {
      throw new UncertainCompletionError(completion)
    }
    throw new Error(readTusError(error))
  }
}

/** Explicitly reconcile an uncertain completion; never creates a new session. */
export async function reconcileUploadCompletion(
  completion: UncertainUploadCompletion,
  options: UploadTransportOptions = {},
): Promise<UploadResponse> {
  try {
    const normalized = normalizeUploadResponsePayload(await finalizeTusUpload(completion, options))
    return {
      status: normalized.status,
      tasks: normalizeTasks(normalized.tasks),
      error: normalized.error,
    }
  } catch {
    return {
      status: 'completion_unknown',
      tasks: [],
      error: UNCERTAIN_COMPLETION_MESSAGE,
      uncertainCompletion: completion,
    }
  }
}

const uploadFileWithTus = async (
  request: UploadRequest,
  file: File,
): Promise<RawUploadResponse> => {
  const entityIdStr = ensureEntity(request.entityId)
  const uploadUrl = await createTusUpload(
    file,
    {
      filename: file.name,
      filetype: file.type,
      entity_type: request.entityType,
      entity_id: entityIdStr,
      context: request.context,
      file_type: request.fileCategory,
      replace_file_id: request.replaceFileId,
      skip_resize: request.skipResize ? 'true' : undefined,
    },
    request.uploadUrl,
    request,
  )

  let offset = await fetchTusOffset(uploadUrl, request)

  if (request.onProgress && file.size > 0) {
    request.onProgress(Math.floor((offset / file.size) * 100))
  }

  if (offset > file.size) throw new Error('Upload offset exceeds file size')
  let conflicts = 0
  while (offset < file.size) {
    const chunk = file.slice(offset, offset + TUS_CHUNK_SIZE)
    const response = await patchTusChunk(uploadUrl, offset, chunk, request)

    if (response.status === 409) {
      if (++conflicts > 3) throw new Error('Upload offset conflict. Please try again.')
      const recoveredOffset = await fetchTusOffset(uploadUrl, request)
      if (recoveredOffset < offset) throw new Error('Upload offset moved backwards')
      offset = recoveredOffset
      if (offset > file.size) throw new Error('Upload offset exceeds file size')
      continue
    }

    if (response.status !== 204) {
      // Should be handled by catch block in patchTusChunk but for safety
      throw new Error(`Upload failed with status ${response.status}`)
    }

    const updatedOffset = readTusOffset(response)
    if (updatedOffset !== offset + chunk.size || updatedOffset > file.size) {
      throw new Error('Upload offset did not advance by the acknowledged chunk')
    }
    conflicts = 0
    offset = updatedOffset

    if (request.onProgress && file.size > 0) {
      request.onProgress(Math.min(100, Math.floor((offset / file.size) * 100)))
    }
  }

  return finalizeTusUpload(
    {
      location: uploadUrl,
      filename: file.name,
      mimeType: file.type,
      size: file.size,
      context: request.context,
      fileCategory: request.fileCategory,
      entityType: request.entityType,
      entityId: ensureEntity(request.entityId),
      replaceFileId: request.replaceFileId,
    },
    request,
    request.onCompletionSession,
  )
}

export interface PendingTusUploadRequest extends UploadTransportOptions {
  file: File
  uploadUrl: string
  fileCategory: 'image' | 'video' | 'file'
  resumeLocation?: string
  onSession?: (location: string) => void
  onProgress?: (percentage: number) => void
}

export interface PendingTusUpload {
  id: string
  location: string
}

// uploadPendingWithTus intentionally stops before the generic /complete route.
// A domain owner such as gocms must register and finalize the quarantine session.
export const uploadPendingWithTus = async (
  request: PendingTusUploadRequest,
): Promise<PendingTusUpload> => {
  const { file } = request
  const location =
    request.resumeLocation ||
    (await createTusUpload(
      file,
      {
        filename: file.name,
        filetype: file.type,
        context: 'cms',
        file_type: request.fileCategory,
      },
      request.uploadUrl,
      request,
    ))
  request.onSession?.(location)

  let offset = await fetchTusOffset(location, request)
  request.onProgress?.(file.size > 0 ? Math.floor((offset / file.size) * 100) : 100)
  if (offset > file.size) throw new Error('Upload offset exceeds file size')
  let conflicts = 0
  while (offset < file.size) {
    const chunk = file.slice(offset, offset + TUS_CHUNK_SIZE)
    const response = await patchTusChunk(location, offset, chunk, request)
    if (response.status === 409) {
      if (++conflicts > 3) throw new Error('Upload offset conflict. Please try again.')
      const recoveredOffset = await fetchTusOffset(location, request)
      if (recoveredOffset < offset) throw new Error('Upload offset moved backwards')
      offset = recoveredOffset
      if (offset > file.size) throw new Error('Upload offset exceeds file size')
      continue
    }
    if (response.status !== 204) {
      throw new Error(`Upload failed with status ${response.status}`)
    }
    const updatedOffset = readTusOffset(response)
    if (updatedOffset !== offset + chunk.size || updatedOffset > file.size) {
      throw new Error('Upload offset did not advance by the acknowledged chunk')
    }
    conflicts = 0
    offset = updatedOffset
    request.onProgress?.(
      file.size > 0 ? Math.min(100, Math.floor((offset / file.size) * 100)) : 100,
    )
  }

  const id = location.replace(/\/$/, '').split('/').at(-1)
  if (!id) throw new Error('TUS upload location does not contain an upload ID')
  return { id, location }
}

export async function uploadFiles(request: UploadRequest): Promise<UploadResponse> {
  const {
    entityType,
    entityId,
    file,
    files,
    replaceFileId,
    context,
    fileCategory,
    uploadUrl,
    skipResize,
    onProgress,
    signal,
    requestTimeoutMs,
    onCompletionSession,
  } = request

  const queue = files?.length ? files : file ? [file] : []

  if (queue.length === 0) {
    throw new Error('no files provided for upload')
  }

  const aggregatedTasks: UploadTaskData[] = []
  let status = 'queued'
  let error: string | undefined
  let uncertainCompletion: UncertainUploadCompletion | undefined

  for (const item of queue) {
    try {
      const raw = await uploadFileWithTus(
        {
          entityType,
          entityId,
          replaceFileId,
          context,
          fileCategory,
          uploadUrl,
          skipResize,
          onProgress,
          signal,
          requestTimeoutMs,
          onCompletionSession,
        },
        item,
      )
      const normalized = normalizeUploadResponsePayload(raw)
      if (normalized.error) {
        error = normalized.error
      }
      status = normalized.status
      aggregatedTasks.push(...normalizeTasks(normalized.tasks))
    } catch (e: unknown) {
      const err = e instanceof Error ? e : new Error(String(e))
      console.error('File upload failed (expected):', err)
      error = err.message || 'Upload failed'
      if (e instanceof UncertainCompletionError) uncertainCompletion = e.completion
      break // Stop on first error for now to avoid multiple error states
    }
  }

  if (aggregatedTasks.length > 0) {
    status = aggregatedTasks[0].status || status
  }

  return {
    status: uncertainCompletion ? 'completion_unknown' : status,
    tasks: aggregatedTasks,
    error,
    ...(uncertainCompletion ? { uncertainCompletion } : {}),
  }
}

export async function fetchUploadTask(
  taskId: number,
  signal?: AbortSignal,
): Promise<UploadTaskStatusResponse> {
  try {
    const { data } = await axios.get<RawTaskResponse>(`/files/tasks/${taskId}`, { signal })
    return normalizeTaskResponse(taskId, data)
  } catch (error) {
    if (axios.isAxiosError(error) && error.response?.status === 404) {
      try {
        const { data } = await axios.get<RawTaskResponse>(`/files/upload/tasks/${taskId}`, {
          signal,
        })
        return normalizeTaskResponse(taskId, data)
      } catch (legacyError) {
        if (axios.isAxiosError(legacyError) && legacyError.response?.status === 404) {
          return buildQueuedResponse(taskId)
        }
        throw legacyError
      }
    }
    throw error
  }
}

export async function deleteUploadedFile(fileId: number): Promise<DeleteResponse> {
  const { data } = await axios.delete<RawDeleteResponse>(`/files/${fileId}`, {
    params: { confirm: true },
  })

  return {
    status: data.status,
    id: data.id,
    entityId: data.entity_id ?? null,
  }
}

export async function listEntityFiles(
  entityType: string,
  entityId: EntityID,
): Promise<ListFilesResponse> {
  const entityIdStr = ensureEntity(entityId)
  const { data } = await axios.get<RawListResponse>('/files', {
    params: {
      entity_type: entityType,
      entity_id: entityIdStr,
    },
  })

  const files = (data.files ?? [])
    .map((file) => normalizeFile(file))
    .filter((item): item is UploadedFileSummary => Boolean(item))

  return {
    files,
    entityId: data.entity_id ?? null,
  }
}
