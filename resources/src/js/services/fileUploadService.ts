import axios, { type AxiosResponse } from 'axios'
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

export interface UploadResponse {
  status: string
  tasks: UploadTaskData[]
  error?: string
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

export interface UploadRequest {
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
): Promise<string> => {
  try {
    const endpoint = resolveTusEndpoint(uploadUrl)
    const response = await axios.post(
      endpoint,
      {},
      {
        headers: {
          'Tus-Resumable': TUS_VERSION,
          'Upload-Length': String(file.size),
          'Upload-Metadata': buildTusMetadata(metadata),
        },
      },
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

const fetchTusOffset = async (uploadUrl: string): Promise<number> => {
  try {
    const response = await axios.head(uploadUrl, {
      headers: {
        'Tus-Resumable': TUS_VERSION,
      },
    })

    const offsetHeader =
      response.headers['upload-offset'] || response.headers['Upload-Offset'] || '0'
    const offset = Number(offsetHeader)
    return Number.isFinite(offset) ? offset : 0
  } catch (error) {
    throw new Error(readTusError(error))
  }
}

const patchTusChunk = async (
  uploadUrl: string,
  offset: number,
  chunk: Blob,
): Promise<AxiosResponse> => {
  try {
    return await axios.patch(uploadUrl, chunk, {
      headers: {
        'Tus-Resumable': TUS_VERSION,
        'Upload-Offset': String(offset),
        'Content-Type': TUS_CONTENT_TYPE,
      },
    })
  } catch (error) {
    if (axios.isAxiosError(error) && error.response) {
      // Return response for 409 handling in loop
      return error.response
    }
    throw new Error(readTusError(error))
  }
}

const finalizeTusUpload = async (uploadUrl: string): Promise<RawUploadResponse> => {
  const endpoint = uploadUrl.replace(/\/$/, '')
  try {
    const response = await axios.post<RawUploadResponse>(
      `${endpoint}/complete`,
      {},
      {
        headers: {
          'Tus-Resumable': TUS_VERSION,
        },
      },
    )
    return response.data
  } catch (error) {
    throw new Error(readTusError(error))
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
  )

  let offset = await fetchTusOffset(uploadUrl)

  if (request.onProgress && file.size > 0) {
    request.onProgress(Math.floor((offset / file.size) * 100))
  }

  while (offset < file.size) {
    const chunk = file.slice(offset, offset + TUS_CHUNK_SIZE)
    const response = await patchTusChunk(uploadUrl, offset, chunk)

    if (response.status === 409) {
      offset = await fetchTusOffset(uploadUrl)
      continue
    }

    if (response.status < 200 || response.status >= 300) {
      // Should be handled by catch block in patchTusChunk but for safety
      throw new Error(`Upload failed with status ${response.status}`)
    }

    const offsetHeader = response.headers['upload-offset'] || response.headers['Upload-Offset']
    const updatedOffset = offsetHeader ? Number(offsetHeader) : Number.NaN
    offset = Number.isFinite(updatedOffset) ? updatedOffset : offset + chunk.size

    if (request.onProgress && file.size > 0) {
      request.onProgress(Math.min(100, Math.floor((offset / file.size) * 100)))
    }
  }

  return finalizeTusUpload(uploadUrl)
}

export interface PendingTusUploadRequest {
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
    ))
  request.onSession?.(location)

  let offset = await fetchTusOffset(location)
  request.onProgress?.(file.size > 0 ? Math.floor((offset / file.size) * 100) : 100)
  while (offset < file.size) {
    const chunk = file.slice(offset, offset + TUS_CHUNK_SIZE)
    const response = await patchTusChunk(location, offset, chunk)
    if (response.status === 409) {
      offset = await fetchTusOffset(location)
      continue
    }
    if (response.status < 200 || response.status >= 300) {
      throw new Error(`Upload failed with status ${response.status}`)
    }
    const offsetHeader = response.headers['upload-offset'] || response.headers['Upload-Offset']
    const updatedOffset = offsetHeader ? Number(offsetHeader) : Number.NaN
    offset = Number.isFinite(updatedOffset) ? updatedOffset : offset + chunk.size
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
  } = request

  const queue = files?.length ? files : file ? [file] : []

  if (queue.length === 0) {
    throw new Error('no files provided for upload')
  }

  const aggregatedTasks: UploadTaskData[] = []
  let status = 'queued'
  let error: string | undefined

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
      break // Stop on first error for now to avoid multiple error states
    }
  }

  if (aggregatedTasks.length > 0) {
    status = aggregatedTasks[0].status || status
  }

  return {
    status,
    tasks: aggregatedTasks,
    error,
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
