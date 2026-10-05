<template>
  <div
    class="image-uploader relative aspect-square w-full border-2 border-dashed border-border-primary/70 transition-colors hover:border-primary"
    :class="{
      'is-loading': isLoading,
      'is-disabled opacity-50 cursor-not-allowed': !canInteract,
      'has-error border-error': error,
    }"
    @dragover.prevent="handleDragOver"
    @dragleave.prevent="handleDragLeave"
    @drop.prevent="handleDrop"
  >
    <input
      ref="fileInput"
      :id="fileInputId"
      type="file"
      name="image-upload"
      aria-label="Загрузить изображение"
      :accept="acceptedFileTypes.join(',')"
      :disabled="!canInteract"
      class="hidden"
      @change="handleFileSelect"
    />

    <div
      v-if="previewUrl"
      class="image-preview group relative h-full w-full overflow-hidden rounded-md"
    >
      <img :src="previewUrl" alt="Image Preview" class="h-full w-full object-cover" />
      <div
        class="image-overlay absolute inset-0 flex flex-col items-center justify-center gap-3 bg-black/50 opacity-0 transition-opacity group-hover:opacity-100"
      >
        <AppButton
          variant="primary"
          size="icon-sm"
          rounded
          @click="triggerFileSelect"
          :disabled="!canInteract"
        >
          <span>&#x270E;</span>
        </AppButton>
        <AppButton
          variant="danger"
          size="icon-sm"
          rounded
          @click="removeImage"
          :disabled="!canDelete"
        >
          <span>&#x1F5D1;</span>
        </AppButton>
        <span class="text-xs text-white/80 uppercase tracking-wide">Удалить</span>
      </div>

      <!-- Success Indicator -->
      <div
        v-if="showSuccessIndicator"
        class="absolute inset-0 flex items-center justify-center rounded-md overlay-success transition-opacity"
      >
        <svg
          class="h-10 w-10 text-white"
          xmlns="http://www.w3.org/2000/svg"
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
        >
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            stroke-width="2"
            d="M5 13l4 4L19 7"
          />
        </svg>
      </div>
    </div>

    <div
      v-else
      class="upload-placeholder flex h-full w-full cursor-pointer items-center justify-center"
      @click="triggerFileSelect"
    >
      <slot name="placeholder">
        <div class="placeholder-content text-center">
          <!-- Replace with an icon -->
          <span class="text-4xl">&#x1F4F7;</span>
          <p class="mt-2 text-sm text-text-secondary">Click to upload or drag & drop</p>
          <p class="mt-1 text-xs text-text-tertiary">Max file size: {{ maxFileSize }}MB</p>
        </div>
      </slot>
    </div>

    <div
      v-if="isLoading"
      class="loading-overlay absolute inset-0 flex items-center justify-center rounded-lg bg-card/80"
    >
      <div
        class="h-9 w-9 animate-spin rounded-full border-2 border-border-primary border-t-primary"
      ></div>
    </div>

    <div
      v-if="props.showProgress && uploadProgress !== null"
      class="pointer-events-none absolute inset-x-0 bottom-0 z-10 h-1 overflow-hidden rounded-b-lg bg-surface-variant/70"
      aria-hidden="true"
    >
      <div
        class="h-full bg-primary transition-[width] duration-200"
        :style="{ width: `${progressPercent}%` }"
      ></div>
    </div>

    <div
      v-if="error && !uncertainUpload"
      class="error-message absolute -bottom-6 left-0 text-xs text-error"
    >
      <p>{{ error }}</p>
    </div>
  </div>
  <p v-if="uncertainUpload" class="mt-2 text-xs text-error" role="status">
    Upload completion is unconfirmed. Check this upload before starting another.
  </p>
  <button
    v-if="uncertainUpload"
    type="button"
    class="mt-7 text-sm underline"
    :disabled="!canCheckUpload"
    @click="checkUpload"
  >
    Check upload
  </button>
  <div
    v-if="shouldShowCropToggle"
    class="mt-2 flex items-center justify-between rounded-lg border border-border-secondary bg-card px-3 py-2 text-xs text-text-secondary"
  >
    <label class="flex items-center gap-2 cursor-pointer select-none">
      <input
        v-model="cropEnabled"
        type="checkbox"
        class="checkbox checkbox-sm"
        :disabled="!canInteract"
      />
      <span class="font-medium text-text-primary">Обрезать изображение</span>
    </label>
    <span class="text-text-tertiary">Можно отключить перед загрузкой</span>
  </div>
  <ImageCropper
    v-if="showImageCropper && cropperImageUrl"
    :image-url="cropperImageUrl"
    :file-name="cropperFileName"
    :default-aspect-ratio="props.cropperDefaultAspectRatio"
    :aspect-ratio-options="props.cropperAspectRatioOptions"
    :max-width="props.cropperMaxWidth"
    :max-height="props.cropperMaxHeight"
    :output-mime-type="props.cropperOutputMimeType"
    @close="closeCropper"
    @crop="handleCropConfirmed"
  />
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref, useId, watch } from 'vue'
import axios from 'axios'
import { useNotifications } from '@/composables/useNotifications'
import { useAdminCapabilities } from '@/composables/useAdminCapabilities'
import { pollUploadTask } from '@/services/pollUploadTask'
import { useUploadQueueStore } from '@/stores/uploadQueue'
import {
  useAdminWebSocket,
  FILE_UPLOAD_STATUS_EVENT,
  type FileUploadStatusEvent,
} from '@/composables/useAdminWebSocket'
import {
  deleteUploadedFile,
  uploadFiles,
  reconcileUploadCompletion,
  type UncertainUploadCompletion,
  type UploadedFileSummary,
} from '@/services/fileUploadService'
import AppButton from '@/components/ui/AppButton.vue'
import { ImageCropper } from '../imageCropper'

interface UploadedFile {
  id: number
  url: string
  filename?: string
  originalName?: string
  fileName?: string
  fileType?: string
  mimeType?: string
  size?: number
  thumbnail?: string
}

interface FileUploadEventFile {
  id: number
  fileName: string
  originalName: string
  url: string
  size: number
  mimeType: string
}

interface ImageUploaderProps {
  modelValue?: UploadedFile | null
  objectType: string
  objectId: string | number | null
  disabled?: boolean
  readonly?: boolean
  preventDeleteIfSingle?: boolean
  showProgress?: boolean
  uploadLabel?: string
  maxFileSize?: number // in MB
  acceptedFileTypes?: string[]
  uploadUrl?: string
  deleteUrl?: string
  context?: string
  enableCropper?: boolean
  cropEnabled?: boolean
  showCropToggle?: boolean
  cropperDefaultAspectRatio?: string | number
  cropperAspectRatioOptions?: CropperAspectRatioOption[]
  cropperMaxWidth?: number
  cropperMaxHeight?: number
  cropperOutputMimeType?: string
}

interface CropperAspectRatioOption {
  value: string
  label: string
}

const props = withDefaults(defineProps<ImageUploaderProps>(), {
  modelValue: null,
  disabled: false,
  readonly: false,
  preventDeleteIfSingle: false,
  showProgress: true,
  uploadLabel: 'Загрузка файла',
  maxFileSize: 10,
  acceptedFileTypes: () => ['image/jpeg', 'image/png', 'image/gif', 'image/webp'],
  uploadUrl: '/files',
  deleteUrl: undefined,
  context: 'image-uploader',
  enableCropper: false,
})

const emit = defineEmits<{
  (e: 'update:modelValue', value: UploadedFile | null): void
  (e: 'success', response: unknown): void
  (e: 'error', error: string): void
  (e: 'deleted'): void
  (e: 'progress', percentage: number): void
}>()

const fileInput = ref<HTMLInputElement | null>(null)
const fileInputId = useId()
const isLoading = ref(false)
const error = ref<string | null>(null)
const currentFile = ref<UploadedFile | null>(props.modelValue ?? null)
const previewUrl = ref<string | null>(props.modelValue?.url ?? null)
const uploadProgress = ref<number | null>(null)
const latestTempPreview = ref<string | null>(null)
const showSuccessIndicator = ref(false)
const currentTaskId = ref<number | null>(null)
const currentUploadId = ref<string | null>(null)
const unsubscribe = ref<(() => void) | null>(null)
const showImageCropper = ref(false)
const cropperImageUrl = ref<string | null>(null)
const cropperFileName = ref('')
const cropEnabled = ref(props.cropEnabled ?? props.enableCropper)
let pollController: AbortController | null = null
const { uploads, realtime } = useAdminCapabilities()

const { on, ensureConnected } = useAdminWebSocket()
const { success: notifySuccess, error: notifyError } = useNotifications()
const uploadQueue = useUploadQueueStore()

if (typeof window !== 'undefined' && uploads.value && realtime.value) {
  ensureConnected()
}

watch(
  () => props.modelValue,
  (newValue) => {
    currentFile.value = newValue ?? null
    previewUrl.value = newValue?.url ?? latestTempPreview.value ?? null
  },
)

watch(
  () => props.enableCropper,
  (enabled) => {
    if (!enabled) {
      cropEnabled.value = false
      return
    }
    if (props.cropEnabled === undefined && !cropEnabled.value) {
      cropEnabled.value = true
    }
  },
)

watch(
  () => props.cropEnabled,
  (enabled) => {
    if (typeof enabled === 'boolean') {
      cropEnabled.value = enabled
      if (!enabled && showImageCropper.value) {
        closeCropper()
      }
    }
  },
)

const uncertainUpload = ref<{ file: File; completion: UncertainUploadCompletion } | null>(null)
const canCheckUpload = computed(
  () =>
    uploads.value &&
    !props.disabled &&
    !props.readonly &&
    !isLoading.value &&
    (!uncertainUpload.value ||
      (uncertainUpload.value.completion.entityType === props.objectType &&
        String(uncertainUpload.value.completion.entityId) === String(props.objectId))),
)
const canInteract = computed(() => canCheckUpload.value && !uncertainUpload.value)
const checkUpload = () => {
  if (
    canCheckUpload.value &&
    uncertainUpload.value &&
    uncertainUpload.value.completion.entityType === props.objectType &&
    String(uncertainUpload.value.completion.entityId) === String(props.objectId)
  ) {
    void uploadFile(uncertainUpload.value.file, true)
  }
}
const shouldShowCropToggle = computed(
  () => props.enableCropper && props.showCropToggle !== false && props.cropEnabled === undefined,
)
const progressPercent = computed(() => {
  if (uploadProgress.value === null) {
    return 0
  }
  return Math.min(100, Math.max(0, Math.round(uploadProgress.value)))
})
const canDelete = computed(() => {
  if (!canInteract.value) {
    return false
  }
  if (!currentFile.value) {
    return false
  }
  return !props.preventDeleteIfSingle
})

const mapEventFile = (file: FileUploadEventFile): UploadedFile => ({
  id: file.id,
  url: file.url,
  filename: file.fileName,
  originalName: file.originalName ?? file.fileName,
  fileName: file.fileName,
  mimeType: file.mimeType,
  size: file.size,
  fileType: file.mimeType?.startsWith('image/') ? 'image' : undefined,
})

const mapSummaryFile = (file: UploadedFileSummary): UploadedFile => ({
  id: file.id,
  url: file.url,
  filename: file.filename,
  originalName: file.originalName,
  fileName: file.filename,
  fileType: file.fileType,
})

const detachListener = () => {
  pollController?.abort()
  pollController = null
  if (unsubscribe.value) {
    unsubscribe.value()
    unsubscribe.value = null
  }
}

const handleFileStatusEvent = (event: FileUploadStatusEvent) => {
  if (event.eventType !== FILE_UPLOAD_STATUS_EVENT || event.taskId !== currentTaskId.value) {
    return
  }

  if (event.status === 'completed' && event.file?.url) {
    const uploaded = mapEventFile(event.file)
    currentFile.value = uploaded
    previewUrl.value = uploaded.url
    latestTempPreview.value = null
    emit('update:modelValue', uploaded)
    emit('success', event)
    currentTaskId.value = null
    detachListener()
    if (currentUploadId.value) {
      uploadQueue.complete(currentUploadId.value)
      currentUploadId.value = null
    }

    showSuccessIndicator.value = true
    window.setTimeout(() => {
      showSuccessIndicator.value = false
    }, 3000)
  } else if (event.status === 'failed') {
    error.value = event.error || 'File processing failed after upload.'
    emit('error', error.value)
    currentTaskId.value = null
    detachListener()
    previewUrl.value = currentFile.value?.url ?? null
    if (currentUploadId.value) {
      uploadQueue.fail(currentUploadId.value, error.value)
      currentUploadId.value = null
    }
  }
}

const triggerFileSelect = () => {
  if (!canInteract.value) {
    return
  }
  fileInput.value?.click()
}

const handleFileSelect = (event: Event) => {
  const target = event.target as HTMLInputElement
  if (target.files && target.files.length > 0) {
    const file = target.files[0]
    handleFile(file)
  }
}

const handleDragOver = (event: DragEvent) => {
  if (!canInteract.value || !event.dataTransfer) {
    return
  }
  event.dataTransfer.dropEffect = 'copy'
}

const handleDragLeave = () => {}

const handleDrop = (event: DragEvent) => {
  if (!canInteract.value) {
    return
  }
  if (event.dataTransfer?.files && event.dataTransfer.files.length > 0) {
    const file = event.dataTransfer.files[0]
    handleFile(file)
  }
}

const handleFile = (file: File) => {
  if (!canInteract.value) return
  error.value = null

  if (!validateFile(file)) {
    return
  }

  if (props.enableCropper && cropEnabled.value && file.type.startsWith('image/')) {
    openCropper(file)
    return
  }

  processSelectedFile(file)
}

const processSelectedFile = (file: File) => {
  setPreviewFromFile(file)
  uploadFile(file)
}

const validateFile = (file: File) => {
  if (!isAcceptedFileType(file)) {
    error.value = 'Invalid file type.'
    emit('error', error.value)
    return false
  }

  if (file.size > props.maxFileSize * 1024 * 1024) {
    error.value = `File is too large. Max size is ${props.maxFileSize}MB.`
    emit('error', error.value)
    return false
  }

  return true
}

const isAcceptedFileType = (file: File) => {
  const mime = file.type
  return props.acceptedFileTypes.some((type) => {
    if (type === mime) {
      return true
    }
    if (type.endsWith('/*')) {
      const prefix = type.slice(0, -1)
      return mime.startsWith(prefix)
    }
    return false
  })
}

const setPreviewFromFile = (file: File) => {
  error.value = null

  const reader = new FileReader()
  reader.onload = (e) => {
    const result = e.target?.result as string
    previewUrl.value = result
    latestTempPreview.value = previewUrl.value
  }
  reader.readAsDataURL(file)
}

const uploadFile = async (file: File, reconcile = false) => {
  if (reconcile ? !canCheckUpload.value || !uncertainUpload.value : !canInteract.value) return
  detachListener()
  if (!props.objectId) {
    error.value = 'Cannot upload file without a valid objectId.'
    emit('error', error.value)
    previewUrl.value = currentFile.value?.url ?? null
    return
  }

  isLoading.value = true
  error.value = null
  uploadProgress.value = 0
  emit('progress', 0)
  if (currentUploadId.value) {
    uploadQueue.remove(currentUploadId.value)
  }
  currentUploadId.value = uploadQueue.start(props.uploadLabel ?? 'Загрузка файла', 0)

  try {
    const response =
      reconcile && uncertainUpload.value
        ? await reconcileUploadCompletion(uncertainUpload.value.completion)
        : await uploadFiles({
            file,
            entityType: props.objectType,
            entityId: props.objectId,
            replaceFileId: currentFile.value?.id,
            fileCategory: 'image',
            context: props.context,
            uploadUrl: props.uploadUrl,
            onProgress: (percentage) => {
              const next = Math.min(100, Math.max(0, Math.round(percentage)))
              if (uploadProgress.value === next) {
                return
              }
              uploadProgress.value = next
              emit('progress', next)
              if (currentUploadId.value) {
                uploadQueue.update(currentUploadId.value, next)
              }
            },
          })
    if (uploadProgress.value !== null && uploadProgress.value < 100) {
      uploadProgress.value = 100
      emit('progress', 100)
      if (currentUploadId.value) {
        uploadQueue.update(currentUploadId.value, 100)
      }
    }

    if (response.uncertainCompletion) {
      uncertainUpload.value = { file, completion: response.uncertainCompletion }
    }
    const terminalTask = response.tasks[0]
    if (
      reconcile &&
      terminalTask?.id &&
      ['failed', 'error'].includes((terminalTask.status || response.status).toLowerCase()) &&
      uncertainUpload.value?.completion.entityType === props.objectType &&
      String(uncertainUpload.value?.completion.entityId) === String(props.objectId)
    ) {
      uncertainUpload.value = null
      throw new Error(terminalTask.error || response.error || 'Upload processing failed')
    }
    if (response.status !== 'queued' && !(reconcile && response.status === 'completed')) {
      throw new Error(response.error || 'Failed to enqueue upload task')
    }

    const task = response.tasks[0]
    if (!task?.id) {
      throw new Error(response.error || 'Upload task ID is missing in server response')
    }

    if (
      reconcile &&
      uncertainUpload.value &&
      (uncertainUpload.value.completion.entityType !== props.objectType ||
        String(uncertainUpload.value.completion.entityId) !== String(props.objectId))
    )
      throw new Error('Return to the original entity to check this upload.')
    uncertainUpload.value = null

    if (task.tempUrl) {
      previewUrl.value = task.tempUrl
      latestTempPreview.value = task.tempUrl
    }

    if (task.file) {
      const uploaded = mapSummaryFile(task.file)
      currentFile.value = uploaded
      previewUrl.value = uploaded.url
      latestTempPreview.value = null
      emit('update:modelValue', uploaded)
    }

    detachListener()
    currentTaskId.value = task.id
    if (reconcile && response.status === 'completed' && task.file?.url) {
      handleFileStatusEvent({
        eventId: `reconcile-${task.id}`,
        eventType: FILE_UPLOAD_STATUS_EVENT,
        taskId: task.id,
        status: 'completed',
        file: {
          id: task.file.id,
          fileName: task.file.filename,
          originalName: task.file.originalName,
          url: task.file.url,
          size: task.file.size ?? 0,
          mimeType: task.file.mimeType ?? '',
        },
      })
      return
    }
    if (realtime.value) {
      ensureConnected()
      unsubscribe.value = on('files.upload.status', handleFileStatusEvent)
    } else {
      const controller = new AbortController()
      pollController = controller
      void pollUploadTask(task.id, { signal: controller.signal })
        .then((result) => {
          if (controller.signal.aborted) return
          const file = result.file ?? result.task.file
          if (result.status === 'completed' && !file?.url) {
            throw new Error('Обработка файла завершена, но адрес изображения недоступен.')
          }
          handleFileStatusEvent({
            eventId: `poll-${task.id}`,
            eventType: FILE_UPLOAD_STATUS_EVENT,
            taskId: task.id,
            status: result.status === 'error' ? 'failed' : result.status,
            error: result.error ?? result.task.error,
            file: file
              ? {
                  id: file.id,
                  fileName: file.filename,
                  originalName: file.originalName,
                  url: file.url,
                  size: file.size ?? 0,
                  mimeType: file.mimeType ?? '',
                }
              : undefined,
          })
        })
        .catch((reason: unknown) => {
          if (controller.signal.aborted) return
          handleFileStatusEvent({
            eventId: `poll-${task.id}`,
            eventType: FILE_UPLOAD_STATUS_EVENT,
            taskId: task.id,
            status: 'failed',
            error: reason instanceof Error ? reason.message : 'Не удалось проверить статус файла.',
          })
        })
    }

    emit('success', response)
    if (currentUploadId.value) {
      uploadQueue.setStatus(currentUploadId.value, 'processing')
    }
  } catch (e: unknown) {
    console.error('File upload error:', e)
    const errorMessage =
      (e as { response?: { data?: { error?: string; message?: string } }; message?: string })
        ?.response?.data?.error ||
      (e as { response?: { data?: { error?: string; message?: string } }; message?: string })
        ?.response?.data?.message ||
      (e as { message?: string })?.message ||
      'An unknown error occurred.'
    error.value = errorMessage
    emit('error', errorMessage)
    if (!uncertainUpload.value) {
      previewUrl.value = currentFile.value?.url ?? null
      latestTempPreview.value = null
    }
    if (currentUploadId.value) {
      uploadQueue.fail(currentUploadId.value, errorMessage)
      currentUploadId.value = null
    }
  } finally {
    isLoading.value = false
    uploadProgress.value = null
  }
}

const removeImage = async () => {
  if (!canDelete.value || !currentFile.value?.id) {
    return
  }

  try {
    isLoading.value = true
    error.value = null

    let deleteMessage: string | undefined
    if (props.deleteUrl) {
      const { data } = await axios.delete(props.deleteUrl)
      deleteMessage = data?.message || (data?.status === 'deleted' ? 'Файл удалён' : undefined)
    } else {
      const result = await deleteUploadedFile(currentFile.value.id)
      if (result.status === 'deleted') {
        deleteMessage = 'Файл удалён'
      }
    }

    currentFile.value = null
    previewUrl.value = null
    latestTempPreview.value = null
    emit('update:modelValue', null)
    emit('deleted')

    if (deleteMessage) {
      notifySuccess(deleteMessage)
    }
  } catch (e: unknown) {
    console.error('File delete error:', e)
    const errorMessage =
      (e as { response?: { data?: { error?: string; message?: string } }; message?: string })
        ?.response?.data?.error ||
      (e as { response?: { data?: { error?: string; message?: string } }; message?: string })
        ?.response?.data?.message ||
      (e as { message?: string })?.message ||
      'Не удалось удалить файл.'
    error.value = errorMessage
    emit('error', errorMessage)
    notifyError(errorMessage)
  } finally {
    isLoading.value = false
  }
}

onUnmounted(() => {
  detachListener()
})

export type { UploadedFile, CropperAspectRatioOption }

const openCropper = (file: File) => {
  const reader = new FileReader()
  reader.onload = (event) => {
    cropperImageUrl.value = event.target?.result as string
    cropperFileName.value = file.name
    showImageCropper.value = true
  }
  reader.readAsDataURL(file)
}

const closeCropper = () => {
  showImageCropper.value = false
  cropperImageUrl.value = null
  cropperFileName.value = ''
}

const handleCropConfirmed = (croppedFile: File) => {
  // Always close the cropper immediately to avoid UI blocking during upload.
  closeCropper()

  if (!canInteract.value) return
  if (!validateFile(croppedFile)) {
    return
  }

  setPreviewFromFile(croppedFile)
  uploadFile(croppedFile)
}
</script>
