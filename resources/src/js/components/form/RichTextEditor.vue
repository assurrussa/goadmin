<template>
  <div class="rich-text-editor">
    <!-- Toolbar -->
    <div v-if="showToolbar" class="editor-toolbar">
      <!-- Mobile toggle for toolbar sections -->
      <div class="toolbar-mobile-toggle sm:hidden">
        <AppButton
          variant="ghost"
          size="sm"
          @click="showMobileToolbar = !showMobileToolbar"
          :aria-expanded="showMobileToolbar"
          :aria-controls="toolbarContentId"
          title="Показать или скрыть панель форматирования"
        >
          {{ showMobileToolbar ? 'Скрыть форматирование' : 'Показать форматирование' }}
        </AppButton>
      </div>

      <!-- Toolbar Content -->
      <div
        :id="toolbarContentId"
        class="toolbar-content"
        :class="{ 'mobile-hidden': !showMobileToolbar }"
      >
        <!-- Basic formatting -->
        <div
          v-if="canUseAnyFeature(['bold', 'italic', 'underline', 'strike'])"
          class="toolbar-section"
        >
          <div
            class="flex items-center gap-0.5 border border-border-primary rounded-md p-0.5 bg-surface"
          >
            <AppButton
              v-if="canUseFeature('bold')"
              variant="ghost"
              size="icon-sm"
              :class="{ 'bg-surface-variant text-primary': editor?.isActive('bold') }"
              @click="editor?.chain().focus().toggleBold().run()"
              title="Bold (Ctrl+B)"
            >
              <strong>B</strong>
            </AppButton>
            <AppButton
              v-if="canUseFeature('italic')"
              variant="ghost"
              size="icon-sm"
              :class="{ 'bg-surface-variant text-primary': editor?.isActive('italic') }"
              @click="editor?.chain().focus().toggleItalic().run()"
              title="Italic (Ctrl+I)"
            >
              <em>I</em>
            </AppButton>
            <AppButton
              v-if="canUseFeature('underline')"
              variant="ghost"
              size="icon-sm"
              :class="{ 'bg-surface-variant text-primary': editor?.isActive('underline') }"
              @click="editor?.chain().focus().toggleUnderline().run()"
              title="Underline (Ctrl+U)"
            >
              <u>U</u>
            </AppButton>
            <AppButton
              v-if="canUseFeature('strike')"
              variant="ghost"
              size="icon-sm"
              :class="{ 'bg-surface-variant text-primary': editor?.isActive('strike') }"
              @click="editor?.chain().focus().toggleStrike().run()"
              title="Strikethrough"
            >
              <s>S</s>
            </AppButton>
          </div>
        </div>

        <!-- Headings -->
        <div v-if="canUseFeature('heading')" class="toolbar-section">
          <div
            class="flex items-center gap-0.5 border border-border-primary rounded-md p-0.5 bg-surface"
          >
            <AppButton
              variant="ghost"
              size="icon-sm"
              :class="{
                'bg-surface-variant text-primary': editor?.isActive('heading', { level: 2 }),
              }"
              @click="editor?.chain().focus().toggleHeading({ level: 2 }).run()"
              title="Heading 2"
            >
              H2
            </AppButton>
            <AppButton
              variant="ghost"
              size="icon-sm"
              :class="{
                'bg-surface-variant text-primary': editor?.isActive('heading', { level: 3 }),
              }"
              @click="editor?.chain().focus().toggleHeading({ level: 3 }).run()"
              title="Heading 3"
            >
              H3
            </AppButton>
            <AppButton
              variant="ghost"
              size="icon-sm"
              :class="{
                'bg-surface-variant text-primary': editor?.isActive('heading', { level: 4 }),
              }"
              @click="editor?.chain().focus().toggleHeading({ level: 4 }).run()"
              title="Heading 4"
            >
              H4
            </AppButton>
          </div>
        </div>

        <!-- Lists -->
        <div
          v-if="canUseAnyFeature(['bulletList', 'orderedList', 'taskList'])"
          class="toolbar-section"
        >
          <div
            class="flex items-center gap-0.5 border border-border-primary rounded-md p-0.5 bg-surface"
          >
            <AppButton
              v-if="canUseFeature('bulletList')"
              variant="ghost"
              size="icon-sm"
              :class="{ 'bg-surface-variant text-primary': editor?.isActive('bulletList') }"
              @click="editor?.chain().focus().toggleBulletList().run()"
              title="Bullet List"
            >
              •
            </AppButton>
            <AppButton
              v-if="canUseFeature('orderedList')"
              variant="ghost"
              size="icon-sm"
              :class="{ 'bg-surface-variant text-primary': editor?.isActive('orderedList') }"
              @click="editor?.chain().focus().toggleOrderedList().run()"
              title="Numbered List"
            >
              1.
            </AppButton>
            <AppButton
              v-if="canUseFeature('taskList')"
              variant="ghost"
              size="icon-sm"
              :class="{ 'bg-surface-variant text-primary': editor?.isActive('taskList') }"
              @click="editor?.chain().focus().toggleTaskList().run()"
              title="Task List"
            >
              ☐
            </AppButton>
          </div>
        </div>

        <!-- Media -->
        <div
          v-if="canUseAnyFeature(['image', 'video', 'fileLink', 'link'])"
          class="toolbar-section"
        >
          <div
            class="flex items-center gap-0.5 border border-border-primary rounded-md p-0.5 bg-surface"
          >
            <AppButton
              v-if="canUseFeature('image')"
              variant="ghost"
              size="icon-sm"
              @click="addImage"
              title="Add Image"
            >
              🖼️
            </AppButton>
            <AppButton
              v-if="canUseFeature('video')"
              variant="ghost"
              size="icon-sm"
              @click="addVideo"
              title="Add Video"
            >
              🎥
            </AppButton>
            <AppButton
              v-if="canUseFeature('fileLink')"
              variant="ghost"
              size="icon-sm"
              @click="addFileLink"
              title="Add file link"
            >
              📎
            </AppButton>
            <AppButton
              v-if="canUseFeature('link')"
              variant="ghost"
              size="icon-sm"
              :class="{ 'bg-surface-variant text-primary': editor?.isActive('link') }"
              @click="addLink"
              title="Add Link"
            >
              🔗
            </AppButton>
          </div>
          <label
            v-if="canUseFeature('video') && allowFileUpload && !mediaPicker"
            class="ml-3 flex items-center gap-2 text-xs font-semibold transition duration-150 ease-out text-text-secondary hover:text-text-primary"
          >
            <span>Без ужимания видео</span>
            <Checkbox
              :checked="uploadVideoWithoutCompression"
              :title="
                uploadVideoWithoutCompression
                  ? 'Видео загрузится как есть'
                  : 'Видео отправится в ресайзер'
              "
              @update:checked="handleUploadVideoToggle"
            />
          </label>
          <AppBadge
            v-if="uploadVideoWithoutCompression"
            variant="success"
            class="ml-2 uppercase tracking-wide text-xs font-semibold"
          >
            Видео без сжатия
          </AppBadge>
        </div>

        <!-- Tables -->
        <div v-if="canUseFeature('table')" class="toolbar-section">
          <div
            class="flex items-center gap-0.5 border border-border-primary rounded-md p-0.5 bg-surface"
          >
            <AppButton
              variant="ghost"
              size="icon-sm"
              @click="
                editor?.chain().focus().insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run()
              "
              title="Insert table"
            >
              ▦
            </AppButton>
            <AppButton
              variant="ghost"
              size="icon-sm"
              :disabled="!editor?.isActive('table')"
              @click="editor?.chain().focus().addRowAfter().run()"
              title="Add row"
            >
              +↕
            </AppButton>
            <AppButton
              variant="ghost"
              size="icon-sm"
              :disabled="!editor?.isActive('table')"
              @click="editor?.chain().focus().addColumnAfter().run()"
              title="Add column"
            >
              +↔
            </AppButton>
            <AppButton
              variant="ghost"
              size="icon-sm"
              :disabled="!editor?.isActive('table')"
              @click="editor?.chain().focus().deleteTable().run()"
              title="Delete table"
            >
              ⌫
            </AppButton>
          </div>
        </div>

        <!-- Advanced -->
        <div
          v-if="canUseAnyFeature(['blockquote', 'codeBlock', 'horizontalRule'])"
          class="toolbar-section"
        >
          <div
            class="flex items-center gap-0.5 border border-border-primary rounded-md p-0.5 bg-surface"
          >
            <AppButton
              v-if="canUseFeature('blockquote')"
              variant="ghost"
              size="icon-sm"
              :class="{ 'bg-surface-variant text-primary': editor?.isActive('blockquote') }"
              @click="editor?.chain().focus().toggleBlockquote().run()"
              title="Quote"
            >
              "
            </AppButton>
            <AppButton
              v-if="canUseFeature('codeBlock')"
              variant="ghost"
              size="icon-sm"
              :class="{ 'bg-surface-variant text-primary': editor?.isActive('codeBlock') }"
              @click="editor?.chain().focus().toggleCodeBlock().run()"
              title="Code Block"
            >
              { }
            </AppButton>
            <AppButton
              v-if="canUseFeature('horizontalRule')"
              variant="ghost"
              size="icon-sm"
              @click="addHorizontalRule"
              title="Horizontal Line"
            >
              ―
            </AppButton>
          </div>
        </div>

        <!-- Undo/Redo -->
        <div class="toolbar-section">
          <div
            class="flex items-center gap-0.5 border border-border-primary rounded-md p-0.5 bg-surface"
          >
            <AppButton
              variant="ghost"
              size="icon-sm"
              @click="editor?.chain().focus().undo().run()"
              :disabled="!editor?.can().undo()"
              title="Undo (Ctrl+Z)"
            >
              ↺
            </AppButton>
            <AppButton
              variant="ghost"
              size="icon-sm"
              @click="editor?.chain().focus().redo().run()"
              :disabled="!editor?.can().redo()"
              title="Redo (Ctrl+Y)"
            >
              ↻
            </AppButton>
          </div>
        </div>

        <!-- View Mode Toggle -->
        <div v-if="canUseFeature('rawJson')" class="toolbar-section ml-auto">
          <AppButton
            variant="outline"
            size="sm"
            @click="toggleViewMode"
            :title="viewMode === 'visual' ? 'View Raw JSON' : 'View Visual'"
          >
            <span v-if="viewMode === 'visual'">&lt;/&gt;</span>
            <span v-else>👁</span>
            <span class="hidden sm:inline ml-1">
              {{ viewMode === 'visual' ? 'Raw JSON' : 'Visual' }}
            </span>
          </AppButton>
        </div>
      </div>
    </div>

    <!-- Editor Content -->
    <div
      class="editor-content"
      :style="{
        minHeight: minHeight,
        maxHeight: maxHeight !== 'none' ? maxHeight : undefined,
      }"
    >
      <!-- Visual Editor -->
      <div v-if="viewMode === 'visual'" class="visual-editor">
        <EditorContent :editor="editor" class="prose prose-sm max-w-none min-h-[200px] p-4" />

        <div v-if="isUploading" class="upload-floating-indicator" role="status" aria-live="polite">
          <span class="upload-floating-icon" aria-hidden="true">🔄</span>
          <div class="upload-floating-body">
            <span class="upload-floating-text">{{ uploadStatusMessage }}</span>
            <div class="upload-floating-track" aria-hidden="true">
              <div
                class="upload-floating-fill"
                :style="{ width: `${uploadProgressPercent}%` }"
              ></div>
            </div>
          </div>
        </div>

        <!-- Image Upload Overlay -->
        <div
          v-if="isDragging"
          class="upload-overlay"
          @drop="handleFileDrop"
          @dragover.prevent
          @dragenter.prevent
        >
          <div class="upload-message">
            <div class="upload-icon">📁</div>
            <div class="upload-text">Drop images or videos here</div>
          </div>
        </div>
      </div>

      <!-- Raw JSON Editor -->
      <div v-else class="json-editor">
        <div class="json-editor-header">
          <h4 class="text-sm font-semibold text-text-secondary">Raw JSON Editor</h4>
          <AppButton variant="ghost" size="xs" @click="formatJSON" title="Format JSON">
            🎨 Format
          </AppButton>
        </div>
        <textarea
          v-model="rawJSON"
          @input="updateFromRawJSON"
          class="json-textarea"
          placeholder="Edit raw JSON content..."
          :class="{ 'json-error': jsonError }"
        ></textarea>
        <div v-if="jsonError" class="json-error-message">❌ Invalid JSON: {{ jsonError }}</div>
      </div>
    </div>

    <!-- Status Bar -->
    <div class="status-bar">
      <div class="status-left">
        <!-- Character Count -->
        <div v-if="showCharacterCount && characterCount !== null" class="character-count">
          <span class="count-current">{{ characterCount }}</span>
          <span v-if="maxCharacters" class="count-max">/ {{ maxCharacters }}</span>
          <span class="count-label">characters</span>
          <span v-if="maxCharacters && characterCount > maxCharacters" class="count-warning">
            ({{ characterCount - maxCharacters }} over limit)
          </span>
        </div>

        <!-- Word Count -->
        <div v-if="showWordCount && wordCount !== null" class="word-count">
          <span class="count-current">{{ wordCount }}</span>
          <span class="count-label">words</span>
        </div>
      </div>

      <div class="status-right">
        <div v-if="uncertainUpload && !recoveredForInsertion" role="status">
          Upload completion is unconfirmed. Check this upload before starting another.
          <button type="button" :disabled="!canCheckUpload" @click="checkUpload">
            Check upload
          </button>
        </div>
        <div v-if="recoveredForInsertion" role="status">
          Upload confirmed: {{ recoveredForInsertion.filename }}. The original field is unknown.
          <button type="button" :disabled="!canInsertRecovered" @click="insertRecovered">
            Insert into this field
          </button>
          <button type="button" :disabled="!canInsertRecovered" @click="dismissRecovered">
            Dismiss confirmed upload
          </button>
        </div>
        <!-- Upload Status -->
        <div v-if="isUploading" class="upload-status" role="status" aria-live="polite">
          <span class="loading-spinner" aria-hidden="true">🔄</span>
          <span class="upload-status-text">{{ uploadStatusMessage }}</span>
          <div class="upload-progress-track" aria-hidden="true">
            <div class="upload-progress-fill" :style="{ width: `${uploadProgressPercent}%` }"></div>
          </div>
        </div>

        <!-- View Mode Indicator -->
        <div class="view-mode-indicator">
          {{ viewMode === 'visual' ? 'Visual Mode' : 'Raw JSON Mode' }}
        </div>
      </div>
    </div>

    <!-- Hidden File Input -->
    <input
      v-if="canUseAnyFeature(['image', 'video'])"
      ref="fileInput"
      :id="fileInputId"
      type="file"
      name="rich-text-media"
      :disabled="!isCurrentScope || !!uncertainUpload || isUploading || disabled || readonly"
      aria-label="Добавить изображение или видео"
      :accept="effectiveAcceptedFileTypes.join(',')"
      multiple
      @change="handleFileSelect"
      class="hidden"
    />

    <!-- Image Cropper Modal -->
    <ImageCropper
      v-if="showImageCropper"
      :image-url="cropperImageUrl"
      :file-name="cropperFileName"
      @close="closeCropper"
      @crop="handleCroppedImage"
    />
  </div>
</template>

<script setup lang="ts">
defineOptions({
  inheritAttrs: false,
})
import { ref, watch, onBeforeUnmount, nextTick, onMounted, computed, useId } from 'vue'
import { useEditor, EditorContent } from '@tiptap/vue-3'
import StarterKit from '@tiptap/starter-kit'
import Underline from '@tiptap/extension-underline'
import Link from '@tiptap/extension-link'
import Image from '@tiptap/extension-image'
import TaskList from '@tiptap/extension-task-list'
import TaskItem from '@tiptap/extension-task-item'
import CharacterCount from '@tiptap/extension-character-count'
import Table from '@tiptap/extension-table'
import TableCell from '@tiptap/extension-table-cell'
import TableHeader from '@tiptap/extension-table-header'
import TableRow from '@tiptap/extension-table-row'
import AppButton from '@/components/ui/AppButton.vue'
import AppBadge from '@/components/ui/AppBadge.vue'
import { Checkbox } from '@/components/ui/checkbox'
import { ImageCropper } from '../imageCropper'
import {
  uploadFiles,
  fetchUploadTask,
  reconcileUploadCompletion,
  type UncertainUploadCompletion,
} from '@/services/fileUploadService'
import type { UploadedFileSummary, UploadTaskStatusResponse } from '@/services/fileUploadService'
import { useAdminWebSocket, type FileUploadStatusEvent } from '@/composables/useAdminWebSocket'
import { useUploadCompletion } from '@/composables/useUploadCompletion'
import { useUploadQueueStore } from '@/stores/uploadQueue'
import { RICH_TEXT_ATTACHMENT_MIME, type RichTextAttachmentPayload } from '@/constants/richText'
import VideoExtension from './extensions/Video'
import { CmsFileLink, CmsImage, CmsVideo } from './extensions/CmsMedia'
import {
  isCanonicalMediaSelection,
  isRichTextFeatureAllowed,
  type RichTextFeature,
  type RichTextInvalidContent,
  type RichTextInvalidContentSource,
  type RichTextMediaPicker,
  type RichTextMediaSelection,
  type RichTextValueVersion,
} from './richTextContract'

// Temporary disable video extension to avoid conflicts
// const VideoExtension = Image.extend({
//   name: 'video',
//   // ... config
// })

// Props
interface Props {
  modelValue?: string | object
  placeholder?: string
  disabled?: boolean
  readonly?: boolean
  showToolbar?: boolean
  showCharacterCount?: boolean
  showWordCount?: boolean
  maxCharacters?: number | null
  outputFormat?: 'json' | 'html'
  allowFileUpload?: boolean
  maxFileSize?: number // in MB
  maxVideoFileSize?: number // in MB
  acceptedFileTypes?: string[]
  minHeight?: string
  maxHeight?: string
  entityId?: number | string | null // ID сущности для привязки файлов
  entityType?: string // Тип сущности (exercise, post, etc.)
  uploadUrl?: string
  /** Stable, unique field name within this entity; enables automatic recovery after remount. */
  uploadRecoveryKey?: string
  allowedNodes?: string[] | null
  allowedMarks?: string[] | null
  allowedFeatures?: string[] | null
  mediaPicker?: RichTextMediaPicker | null
  valueVersion?: RichTextValueVersion
  onInvalidContent?: ((issue: RichTextInvalidContent) => void) | null
}

const props = withDefaults(defineProps<Props>(), {
  modelValue: '',
  placeholder: 'Start typing...',
  disabled: false,
  readonly: false,
  showToolbar: true,
  showCharacterCount: true,
  showWordCount: false,
  maxCharacters: null,
  outputFormat: 'json',
  allowFileUpload: true,
  maxFileSize: 10, // 10MB default
  maxVideoFileSize: 50,
  acceptedFileTypes: () => ['image/*', 'video/*'],
  minHeight: '300px',
  maxHeight: 'none',
  entityId: null,
  entityType: 'rich-text',
  uploadUrl: '/files',
  allowedNodes: null,
  allowedMarks: null,
  allowedFeatures: null,
  mediaPicker: null,
  valueVersion: null,
  onInvalidContent: null,
})

// Emits
const emit = defineEmits<{
  'update:modelValue': [value: string | object]
  'file-upload': [file: File]
  'file-error': [error: string]
  focus: [event: FocusEvent]
  blur: [event: FocusEvent]
  'files-uploaded': [fileIds: number[]]
  'upload-progress': [percentage: number]
  'invalid-content': [issue: RichTextInvalidContent]
}>()

// Reactive state
const viewMode = ref<'visual' | 'json'>('visual')
const rawJSON = ref('')
const jsonError = ref('')
const showMobileToolbar = ref(false)
const isDragging = ref(false)
const fileInput = ref<HTMLInputElement>()
const fileInputId = useId()
const toolbarContentId = useId()
const characterCount = ref<number | null>(null)
const wordCount = ref<number | null>(null)
const showImageCropper = ref(false)
const cropperImageUrl = ref('')
const cropperFileName = ref('')
const isUploading = ref(false)
const {
  pending: uncertainUpload,
  retain: retainCompletion,
  release: releaseCompletion,
  isCurrentScope,
} = useUploadCompletion(
  () => props.entityType,
  () => props.entityId,
  () => 'rich-text',
  () => props.uploadUrl,
  () => props.uploadRecoveryKey,
)
const ownedCompletionLocation = ref<string | null>(null)
const recoveredForInsertion = ref<{
  file: UploadResult
  completion: UncertainUploadCompletion
  filename: string
  category: 'image' | 'video'
  entityType: string
  entityId: string
} | null>(null)
let uploadController: AbortController | null = null
let unmounted = false
const canCheckUpload = computed(() => {
  const pending = uncertainUpload.value
  return (
    !!pending &&
    isCurrentScope.value &&
    !isUploading.value &&
    !props.disabled &&
    !props.readonly &&
    pending.completion.entityType === props.entityType &&
    String(pending.completion.entityId) === String(props.entityId)
  )
})
let checkingCompletion = false
const checkUpload = async () => {
  const pending = uncertainUpload.value
  if (!canCheckUpload.value || !pending || checkingCompletion) return
  if (
    pending.completion.entityType !== props.entityType ||
    String(pending.completion.entityId) !== String(props.entityId)
  )
    return
  checkingCompletion = true
  try {
    const category = pending.completion.fileCategory === 'video' ? 'video' : 'image'
    const uploadedFile = await uploadMediaToServer(pending.file, category)
    if (
      unmounted ||
      !isCurrentScope.value ||
      pending.completion.entityType !== props.entityType ||
      String(pending.completion.entityId) !== String(props.entityId)
    )
      return
    if (!props.uploadRecoveryKey && ownedCompletionLocation.value !== pending.completion.location) {
      // Without a stable field identity, a remounted/neighboring editor cannot
      // infer where this media belongs. A separate explicit insertion is required.
      recoveredForInsertion.value = {
        file: uploadedFile,
        completion: pending.completion,
        filename: pending.completion.filename,
        category,
        entityType: pending.completion.entityType,
        entityId: String(pending.completion.entityId),
      }
      return
    }
    if (category === 'video') insertVideoIntoEditor(uploadedFile, pending.completion.filename)
    else insertImageIntoEditor(uploadedFile, pending.completion.filename)
    if (pending.file) emit('file-upload', pending.file)
    emit('files-uploaded', [uploadedFile.id])
    releaseCompletion(pending.completion)
  } catch (error) {
    emit('file-error', String(error))
  } finally {
    checkingCompletion = false
  }
}
const canInsertRecovered = computed(() => {
  const result = recoveredForInsertion.value
  return (
    !!result &&
    isCurrentScope.value &&
    !props.disabled &&
    !props.readonly &&
    result.entityType === props.entityType &&
    result.entityId === String(props.entityId)
  )
})
const insertRecovered = () => {
  const result = recoveredForInsertion.value
  if (!result || !canInsertRecovered.value) return
  if (result.category === 'video') insertVideoIntoEditor(result.file, result.filename)
  else insertImageIntoEditor(result.file, result.filename)
  emit('files-uploaded', [result.file.id])
  releaseCompletion(result.completion)
  recoveredForInsertion.value = null
}
const dismissRecovered = () => {
  const result = recoveredForInsertion.value
  if (!result || !canInsertRecovered.value) return
  releaseCompletion(result.completion)
  recoveredForInsertion.value = null
}
const uploadProgress = ref(0)
const uploadedFiles = ref<number[]>([]) // Для отслеживания загруженных файлов
const currentUploadType = ref<'image' | 'video' | null>(null)
type UploadStage = 'idle' | 'preparing' | 'uploading' | 'processing' | 'finalizing'
const uploadStage = ref<UploadStage>('idle')
const uploadVideoWithoutCompression = ref(false)

const canUseFeature = (feature: RichTextFeature) =>
  isRichTextFeatureAllowed(feature, props.allowedFeatures, props.allowedNodes, props.allowedMarks)

const canUseAnyFeature = (features: RichTextFeature[]) => features.some(canUseFeature)

const effectiveAcceptedFileTypes = computed(() => {
  const supportedPrefixes = new Set<string>()
  if (canUseFeature('image')) supportedPrefixes.add('image/')
  if (canUseFeature('video')) supportedPrefixes.add('video/')
  return props.acceptedFileTypes.filter((type) => {
    if (type.startsWith('image/')) return supportedPrefixes.has('image/')
    if (type.startsWith('video/')) return supportedPrefixes.has('video/')
    return false
  })
})

const handleUploadVideoToggle = (value: boolean | 'indeterminate') => {
  uploadVideoWithoutCompression.value = value === true
}
const normalizedEventFile = ref<UploadResult | null>(null)
const currentTaskId = ref<number | null>(null)
const unsubscribe = ref<(() => void) | null>(null)
const { on, ensureConnected } = useAdminWebSocket()
const uploadQueue = useUploadQueueStore()

type UploadResult = UploadedFileSummary & {
  fullPath?: string
  publicUrl?: string
  thumbnailUrl?: string | null
  mimeType?: string
  size?: number
  width?: number
  height?: number
  isPrimary?: boolean
  folderPath?: string
  createdAt?: string
  updatedAt?: string
  data?: Record<string, unknown> | null
}

interface WaitForTaskOptions {
  attempts?: number
  interval?: number
  onStatusChange?: (status: string, payload?: UploadResult) => void
}

const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms))

const uploadProgressPercent = computed(() => {
  if (!isUploading.value) {
    return 0
  }
  const next = Math.min(Math.max(uploadProgress.value, 5), 100)
  return Number.isFinite(next) ? next : 0
})

const uploadStatusMessage = computed(() => {
  if (!isUploading.value || uploadStage.value === 'idle') {
    return ''
  }

  const label =
    currentUploadType.value === 'video'
      ? 'видео'
      : currentUploadType.value === 'image'
        ? 'изображение'
        : 'файл'

  switch (uploadStage.value) {
    case 'uploading':
      return uploadVideoWithoutCompression.value && label === 'видео'
        ? `Загружаем ${label} без сжатия...`
        : `Загружаем ${label}...`
    case 'processing':
      return `Обрабатываем ${label}...`
    case 'finalizing':
      return `Финализируем ${label}...`
    default:
      return `Подготавливаем ${label}...`
  }
})

const IMAGE_EXTENSIONS = new Set(['jpg', 'jpeg', 'png', 'gif', 'webp', 'avif', 'svg'])
const VIDEO_EXTENSIONS = new Set(['mp4', 'mov', 'm4v', 'webm', 'mkv', 'avi'])
const MIME_FALLBACKS = new Map<string, string>([
  ['jpg', 'image/jpeg'],
  ['jpeg', 'image/jpeg'],
  ['png', 'image/png'],
  ['gif', 'image/gif'],
  ['webp', 'image/webp'],
  ['avif', 'image/avif'],
  ['svg', 'image/svg+xml'],
  ['mp4', 'video/mp4'],
  ['m4v', 'video/x-m4v'],
  ['mov', 'video/quicktime'],
  ['webm', 'video/webm'],
  ['mkv', 'video/x-matroska'],
  ['avi', 'video/x-msvideo'],
])

type DetectedFileKind = 'image' | 'video' | 'unknown'

const detectFileKind = (
  file: File,
): { kind: DetectedFileKind; normalizedMime?: string; extension?: string } => {
  const rawMime = (file.type || '').toLowerCase()
  const name = (file.name || '').toLowerCase()
  const extension = name.includes('.') ? name.substring(name.lastIndexOf('.') + 1) : undefined

  let kind: DetectedFileKind = 'unknown'
  if (rawMime.startsWith('image/')) {
    kind = 'image'
  } else if (rawMime.startsWith('video/')) {
    kind = 'video'
  } else if (extension) {
    if (IMAGE_EXTENSIONS.has(extension)) {
      kind = 'image'
    } else if (VIDEO_EXTENSIONS.has(extension)) {
      kind = 'video'
    }
  }

  let normalizedMime = rawMime
  if (!normalizedMime && extension) {
    normalizedMime = MIME_FALLBACKS.get(extension) ?? ''
  }

  if (!normalizedMime) {
    if (kind === 'image') {
      normalizedMime = 'image/' + (extension ?? 'jpeg')
    } else if (kind === 'video') {
      normalizedMime = 'video/' + (extension ?? 'mp4')
    }
  }

  return { kind, normalizedMime: normalizedMime || undefined, extension }
}

const prepareFileForUpload = (file: File, mime?: string): File => {
  if (!mime || mime === file.type) {
    return file
  }

  try {
    return new File([file], file.name, {
      type: mime,
      lastModified: file.lastModified,
    })
  } catch (error) {
    console.warn('RichTextEditor: failed to normalize file mime type', error)
    return file
  }
}

const cleanupTaskSubscription = () => {
  if (unsubscribe.value) {
    unsubscribe.value()
    unsubscribe.value = null
  }
  currentTaskId.value = null
}

const fetchTaskStatus = async (taskId: number): Promise<UploadTaskStatusResponse> => {
  return fetchUploadTask(taskId)
}

interface RawUploadedFile {
  id: number
  entityId?: number | null
  filename?: string
  fileName?: string
  originalName?: string
  originalFilename?: string
  url?: string
  publicUrl?: string
  fullPath?: string
  fileType?: string
  mimeType?: string
  folderPath?: string
  createdAt?: string
  updatedAt?: string
  size?: number
  width?: number
  height?: number
  isPrimary?: boolean
  data?: Record<string, unknown> | null
  sortOrder?: number
  thumbnailUrl?: string
  previewUrl?: string
}

const normalizeUploadedFile = (file?: UploadedFileSummary | null): UploadResult | undefined => {
  if (!file) {
    return undefined
  }

  const raw = file as unknown as RawUploadedFile

  const absoluteUrl = resolveAbsoluteUrl(raw.publicUrl ?? raw.url)
  const absoluteFull = resolveAbsoluteUrl(raw.fullPath ?? raw.url) ?? absoluteUrl
  const rawThumbnail = raw.thumbnailUrl ?? raw.previewUrl ?? raw.publicUrl ?? raw.url
  const absoluteThumb = resolveAbsoluteUrl(rawThumbnail ?? '') ?? null
  const mimeType = raw.mimeType
  const folderPath = raw.folderPath
  const createdAt = raw.createdAt
  const updatedAt = raw.updatedAt
  const size = typeof raw.size === 'number' ? raw.size : undefined
  const width = typeof raw.width === 'number' ? raw.width : (raw.data?.width as number | undefined)
  const height =
    typeof raw.height === 'number' ? raw.height : (raw.data?.height as number | undefined)
  const isPrimary = typeof raw.isPrimary === 'boolean' ? raw.isPrimary : undefined
  const data = raw.data ?? null

  return {
    id: raw.id,
    entityId: raw.entityId ?? null,
    filename: raw.filename ?? '',
    originalName: raw.originalName ?? '',
    url: raw.url ?? '',
    publicUrl: absoluteUrl ?? raw.url ?? '',
    fullPath: absoluteFull ?? raw.url ?? '',
    fileType: raw.fileType ?? 'file',
    sortOrder: raw.sortOrder ?? 0,
    thumbnailUrl: absoluteThumb,
    mimeType: typeof mimeType === 'string' ? mimeType : undefined,
    folderPath: typeof folderPath === 'string' ? folderPath : undefined,
    createdAt: typeof createdAt === 'string' ? createdAt : undefined,
    updatedAt: typeof updatedAt === 'string' ? updatedAt : undefined,
    size,
    width: typeof width === 'number' ? width : undefined,
    height: typeof height === 'number' ? height : undefined,
    isPrimary,
    data: data,
  }
}

const mapEventFileFromEvent = (event: FileUploadStatusEvent): UploadResult | undefined => {
  const eventFile = event.file
  if (!eventFile) {
    return undefined
  }

  const metadata = (event.metadata ?? {}) as Record<string, unknown>
  const fallbackUrl = typeof metadata.fileUrl === 'string' ? metadata.fileUrl : undefined

  const normalized = normalizeUploadedFile({
    id: eventFile.id,
    entityId: (metadata.objectId as number | null) ?? null,
    filename: eventFile.fileName,
    originalName: eventFile.originalName ?? eventFile.fileName,
    url: eventFile.url || fallbackUrl || '',
    publicUrl: fallbackUrl ?? eventFile.url,
    fullPath: fallbackUrl ?? eventFile.url,
    fileType: eventFile.mimeType?.startsWith('image/')
      ? 'image'
      : eventFile.mimeType?.startsWith('video/')
        ? 'video'
        : 'file',
    sortOrder: 0,
    mimeType: eventFile.mimeType,
    size: typeof eventFile.size === 'number' ? eventFile.size : undefined,
    width: typeof eventFile.width === 'number' ? eventFile.width : undefined,
    height: typeof eventFile.height === 'number' ? eventFile.height : undefined,
  } as unknown as UploadedFileSummary)

  return normalized
}

const waitForTaskCompletion = async (
  taskId: number,
  options: WaitForTaskOptions = {},
): Promise<UploadResult> => {
  const { attempts = 120, interval = 1000, onStatusChange } = options
  ensureConnected()

  return new Promise<UploadResult>(async (resolve, reject) => {
    let resolved = false

    cleanupTaskSubscription()
    currentTaskId.value = taskId

    const reportStatus = (status: string | undefined, payload?: UploadResult) => {
      if (!status) {
        return
      }
      try {
        onStatusChange?.(status.toLowerCase(), payload)
      } catch (error) {
        console.error('waitForTaskCompletion status handler failed:', error)
      }
    }

    reportStatus('queued')

    unsubscribe.value = on('files.upload.status', (event) => {
      if (event.eventType !== 'files.upload.status' || event.taskId !== taskId || resolved) {
        return
      }

      const eventStatus = (event.status || '').toLowerCase()
      const finalFile = mapEventFileFromEvent(event)
      reportStatus(eventStatus, finalFile)

      if (eventStatus === 'completed' && finalFile) {
        resolved = true
        cleanupTaskSubscription()
        resolve(finalFile)
      } else if (eventStatus === 'failed' || eventStatus === 'error') {
        resolved = true
        cleanupTaskSubscription()
        reject(new Error(event.error || 'Обработка файла завершилась с ошибкой'))
      }
    })

    try {
      for (let attempt = 0; attempt < attempts; attempt++) {
        if (resolved) {
          return
        }

        const { status, task, file } = await fetchTaskStatus(taskId)
        const effectiveStatus = (task.status ?? status ?? '').toLowerCase()
        const finalFile = normalizeUploadedFile(file ?? task.file)

        reportStatus(effectiveStatus, finalFile)

        if (effectiveStatus === 'completed' && finalFile) {
          resolved = true
          cleanupTaskSubscription()
          resolve(finalFile)
          return
        }

        if (effectiveStatus === 'failed' || effectiveStatus === 'error') {
          throw new Error(task.error || 'Обработка файла завершилась с ошибкой')
        }

        await sleep(interval)
      }

      if (!resolved) {
        throw new Error('Превышено время ожидания обработки файла')
      }
    } catch (error) {
      if (!resolved) {
        cleanupTaskSubscription()
        reject(error instanceof Error ? error : new Error(String(error)))
      }
    }
  })
}

// Editor configuration. The allowlists are initialization-time schema inputs;
// consumers should remount the editor when switching to another catalog policy.
const nodeAllowed = (...names: string[]) =>
  !props.allowedNodes || names.some((name) => props.allowedNodes?.includes(name))
const markAllowed = (...names: string[]) =>
  !props.allowedMarks || names.some((name) => props.allowedMarks?.includes(name))

const extensions = [
  StarterKit.configure({
    blockquote: nodeAllowed('blockquote') ? {} : false,
    bold: markAllowed('bold') ? {} : false,
    bulletList: nodeAllowed('bulletList') ? {} : false,
    code: markAllowed('code') ? {} : false,
    codeBlock: nodeAllowed('codeBlock') ? { languageClassPrefix: 'language-' } : false,
    hardBreak: nodeAllowed('hardBreak') ? {} : false,
    heading: nodeAllowed('heading') ? { levels: [2, 3, 4] } : false,
    horizontalRule: nodeAllowed('horizontalRule') ? {} : false,
    italic: markAllowed('italic') ? {} : false,
    listItem: nodeAllowed('listItem') ? {} : false,
    orderedList: nodeAllowed('orderedList') ? {} : false,
    strike: markAllowed('strike') ? {} : false,
  }),
  ...(markAllowed('underline') ? [Underline] : []),
  ...(markAllowed('link')
    ? [
        Link.configure({
          openOnClick: false,
          HTMLAttributes: {
            class: 'link link-primary',
          },
        }),
      ]
    : []),
  ...(nodeAllowed('image')
    ? [
        Image.configure({
          inline: false,
          allowBase64: true,
          HTMLAttributes: {
            class: 'editor-image',
          },
        }),
      ]
    : []),
  ...(nodeAllowed('video')
    ? [
        VideoExtension.configure({
          HTMLAttributes: {
            class: 'editor-video',
            controls: true,
            preload: 'metadata',
          },
        }),
      ]
    : []),
  ...(nodeAllowed('cmsImage') ? [CmsImage] : []),
  ...(nodeAllowed('cmsVideo') ? [CmsVideo] : []),
  ...(markAllowed('cmsFileLink') ? [CmsFileLink] : []),
  ...(nodeAllowed('table') && nodeAllowed('tableRow') && nodeAllowed('tableCell')
    ? [
        Table.configure({ resizable: true }),
        TableRow,
        ...(nodeAllowed('tableHeader') ? [TableHeader] : []),
        TableCell,
      ]
    : []),
  ...(nodeAllowed('taskList') ? [TaskList] : []),
  ...(nodeAllowed('taskItem') ? [TaskItem.configure({ nested: true })] : []),
  CharacterCount,
]

const reportInvalidContent = (
  error: Error,
  source: RichTextInvalidContentSource,
  value: unknown,
) => {
  const issue: RichTextInvalidContent = {
    error,
    source,
    value,
    valueVersion: props.valueVersion,
  }
  props.onInvalidContent?.(issue)
  emit('invalid-content', issue)
}

// Initialize editor
const editor = useEditor({
  extensions,
  content: props.modelValue,
  enableContentCheck: true,
  editable: !props.disabled && !props.readonly,
  editorProps: {
    attributes: {
      class: 'rich-text-content focus:outline-none',
      'data-placeholder': props.placeholder,
    },
    handleDrop: (view, event) => {
      if (!props.allowFileUpload || !canUseAnyFeature(['image', 'video'])) return false

      const dropPos = view.posAtCoords({ left: event.clientX, top: event.clientY })?.pos ?? null

      const attachmentPayload =
        event.dataTransfer?.getData(RICH_TEXT_ATTACHMENT_MIME) ||
        event.dataTransfer?.getData('text/plain')
      if (attachmentPayload) {
        try {
          const parsed = JSON.parse(attachmentPayload) as AttachmentPayload
          event.preventDefault()
          if (dropPos !== null) {
            setSelectionAt(dropPos)
          }
          insertAttachment(parsed)
          return true
        } catch (error) {
          console.warn('RichTextEditor: failed to parse attachment payload', error)
        }
      }

      const files = Array.from(event.dataTransfer?.files || [])
      if (files.length > 0) {
        event.preventDefault()
        if (dropPos !== null) {
          setSelectionAt(dropPos)
        }
        handleFiles(files, { skipCropper: true })
        return true
      }
      return false
    },
    handlePaste: (view, event) => {
      if (!props.allowFileUpload || !canUseAnyFeature(['image', 'video'])) return false

      const files = Array.from(event.clipboardData?.files || [])
      if (files.length > 0) {
        event.preventDefault()
        handleFiles(files, { skipCropper: true })
        return true
      }
      return false
    },
  },
  onUpdate: () => {
    updateCounts()
    emitContent()
  },
  onContentError: ({ error }) => {
    reportInvalidContent(error, 'initial', props.modelValue)
  },
  onFocus: ({ event }) => {
    emit('focus', event as FocusEvent)
  },
  onBlur: ({ event }) => {
    emit('blur', event as FocusEvent)
  },
})

let lastKnownSelection: { from: number; to: number } | null = null

const clampSelection = (pos: number, docSize: number) => {
  return Math.max(0, Math.min(pos, docSize))
}

const captureCurrentSelection = () => {
  if (!editor.value) return
  const { from, to } = editor.value.state.selection
  lastKnownSelection = { from, to }
}

const restoreSelection = () => {
  if (!editor.value || !lastKnownSelection) return

  const docSize = editor.value.state.doc.content.size
  const from = clampSelection(lastKnownSelection.from, docSize)
  const to = clampSelection(lastKnownSelection.to, docSize)

  editor.value.commands.setTextSelection({ from, to })
}

const setSelectionAt = (position: number) => {
  if (!editor.value) return
  const docSize = editor.value.state.doc.content.size
  const pos = clampSelection(position, docSize)
  editor.value.commands.setTextSelection({ from: pos, to: pos })
  lastKnownSelection = { from: pos, to: pos }
}

watch(
  editor,
  (instance, _prev, onCleanup) => {
    if (!instance) {
      return
    }

    const syncSelection = () => {
      const { from, to } = instance.state.selection
      lastKnownSelection = { from, to }
    }

    instance.on('selectionUpdate', syncSelection)
    instance.on('focus', syncSelection)
    syncSelection()

    onCleanup(() => {
      instance.off('selectionUpdate', syncSelection)
      instance.off('focus', syncSelection)
    })
  },
  { immediate: true },
)

// Computed properties
// Removed unused currentContent computed property

// Methods
const updateCounts = () => {
  if (!editor.value) return

  if (props.showCharacterCount) {
    try {
      characterCount.value = editor.value.storage?.characterCount?.characters?.() || 0
    } catch {
      characterCount.value = 0
    }
  }

  if (props.showWordCount) {
    try {
      const text = editor.value.getText()
      wordCount.value = text.trim() ? text.trim().split(/\s+/).length : 0
    } catch {
      wordCount.value = 0
    }
  }
}

const emitContent = () => {
  if (!editor.value) return

  const content = props.outputFormat === 'json' ? editor.value.getJSON() : editor.value.getHTML()
  emit('update:modelValue', content || (props.outputFormat === 'json' ? {} : ''))
}

const toggleViewMode = () => {
  if (viewMode.value === 'visual') {
    updateRawJSON()
    viewMode.value = 'json'
  } else {
    viewMode.value = 'visual'
  }
}

const updateRawJSON = () => {
  if (editor.value) {
    try {
      rawJSON.value = JSON.stringify(editor.value.getJSON(), null, 2)
      jsonError.value = ''
    } catch (error) {
      jsonError.value = error instanceof Error ? error.message : 'Unknown error'
    }
  }
}

const setValidatedContent = (
  value: string | object,
  source: Exclude<RichTextInvalidContentSource, 'initial'>,
) => {
  if (!editor.value) return false
  try {
    editor.value.commands.setContent(value || '', false, {}, { errorOnInvalidContent: true })
    updateCounts()
    return true
  } catch (error) {
    reportInvalidContent(error instanceof Error ? error : new Error(String(error)), source, value)
    return false
  }
}

const updateFromRawJSON = () => {
  try {
    const jsonData = JSON.parse(rawJSON.value)
    if (setValidatedContent(jsonData, 'raw-json')) {
      jsonError.value = ''
    } else {
      jsonError.value = 'Content does not match the allowed rich-text schema'
    }
  } catch (error) {
    jsonError.value = error instanceof Error ? error.message : 'Invalid JSON format'
  }
}

const formatJSON = () => {
  try {
    const parsed = JSON.parse(rawJSON.value)
    rawJSON.value = JSON.stringify(parsed, null, 2)
    jsonError.value = ''
  } catch {
    jsonError.value = 'Cannot format invalid JSON'
  }
}

// File handling
type AttachmentPayload = RichTextAttachmentPayload

interface FileHandleOptions {
  skipCropper?: boolean
}

const resolveAbsoluteUrl = (value?: string): string | undefined => value?.trim() || undefined

const insertImageIntoEditor = (
  uploadedFile: UploadResult,
  fallbackName?: string,
  explicitSrc?: string,
) => {
  if (!editor.value) return

  const name = fallbackName || uploadedFile.originalName || uploadedFile.filename
  const rawSrc = explicitSrc || uploadedFile.publicUrl || uploadedFile.fullPath || uploadedFile.url
  const src = resolveAbsoluteUrl(rawSrc)

  if (!src) {
    emit('file-error', 'Не удалось определить URL вложения для вставки')
    return
  }

  restoreSelection()

  const imageAttrs = {
    src,
    alt: name,
    title: name,
    'data-type': 'image',
    'data-file-id': uploadedFile.id,
  } as Record<string, unknown> & { src: string; alt?: string; title?: string }

  editor.value.chain().focus().setImage(imageAttrs).run()
}

const insertVideoIntoEditor = (
  uploadedFile: UploadResult,
  fallbackName?: string,
  explicitSrc?: string,
) => {
  if (!editor.value) return

  const name = fallbackName || uploadedFile.originalName || uploadedFile.filename
  const rawSrc = explicitSrc || uploadedFile.publicUrl || uploadedFile.fullPath || uploadedFile.url
  const src = resolveAbsoluteUrl(rawSrc)

  if (!src) {
    emit('file-error', 'Не удалось определить URL видео для вставки')
    return
  }

  restoreSelection()

  editor.value
    .chain()
    .focus()
    .setVideo({
      src,
      title: name,
      controls: true,
      'data-file-id': uploadedFile.id,
      'data-type': 'video',
    })
    .run()
}

const resolveEntityId = (): number | null => {
  if (typeof props.entityId === 'number') {
    return props.entityId
  }

  if (typeof props.entityId === 'string') {
    const parsed = Number(props.entityId)
    return Number.isFinite(parsed) ? parsed : null
  }

  return null
}

const insertAttachment = (payload: AttachmentPayload) => {
  if (!payload?.url && !payload?.publicUrl && !payload?.fullPath) {
    emit('file-error', 'Не удалось определить URL вложения')
    return
  }

  const mimeType = payload.mimeType || ''
  const isImage = mimeType.startsWith('image/')
  const isVideo = mimeType.startsWith('video/')

  if (mimeType && !isImage && !isVideo) {
    emit('file-error', 'Перетащить в редактор можно только изображения и видео')
    return
  }
  if ((isImage && !canUseFeature('image')) || (isVideo && !canUseFeature('video'))) {
    emit('file-error', 'Этот тип media запрещён текущей rich-text policy')
    return
  }

  const uploadResult: UploadResult = {
    id: payload.id,
    entityId: resolveEntityId(),
    filename: payload.filename || payload.originalFilename || `file-${payload.id}`,
    originalName: payload.originalFilename || payload.filename || `file-${payload.id}`,
    url: payload.url || payload.publicUrl || payload.fullPath || '',
    publicUrl: payload.publicUrl,
    fullPath: payload.fullPath,
    fileType: isVideo ? 'video' : 'image',
    sortOrder: 0,
  }

  const targetUrl = payload.publicUrl || payload.fullPath
  if (isVideo) {
    insertVideoIntoEditor(uploadResult, uploadResult.originalName, targetUrl)
  } else {
    insertImageIntoEditor(uploadResult, uploadResult.originalName, targetUrl)
  }
  emit('files-uploaded', [payload.id])
}

const handleFiles = async (files: File[], options: FileHandleOptions = {}) => {
  const batchEntityType = props.entityType
  const batchEntityId = props.entityId
  if (unmounted || !isCurrentScope.value) return
  if (uncertainUpload.value || isUploading.value || checkingCompletion) return
  captureCurrentSelection()
  console.log('📁 handleFiles called with:', files.length, 'files')
  for (const file of files) {
    if (
      unmounted ||
      !isCurrentScope.value ||
      batchEntityType !== props.entityType ||
      String(batchEntityId) !== String(props.entityId) ||
      uncertainUpload.value
    )
      break
    console.log('📁 Processing file:', file.name, 'type:', file.type, 'size:', file.size)

    const detected = detectFileKind(file)
    const preparedFile = prepareFileForUpload(file, detected.normalizedMime)
    const isVideoFile = detected.kind === 'video'
    const isImageFile = detected.kind === 'image'
    const limitMb = isVideoFile ? (props.maxVideoFileSize ?? props.maxFileSize) : props.maxFileSize

    if (limitMb && file.size > limitMb * 1024 * 1024) {
      console.log('❌ File too large:', file.name)
      const limitLabel = isVideoFile ? (props.maxVideoFileSize ?? limitMb) : limitMb
      emit('file-error', `File ${file.name} is too large. Maximum size is ${limitLabel}MB`)
      continue
    }

    if (isImageFile && !canUseFeature('image')) {
      emit('file-error', 'Изображения запрещены текущей rich-text policy')
    } else if (isVideoFile && !canUseFeature('video')) {
      emit('file-error', 'Видео запрещено текущей rich-text policy')
    } else if (isImageFile) {
      console.log('🖼️ File identified as image, calling handleImageUpload')
      await handleImageUpload(preparedFile, options)
    } else if (isVideoFile) {
      console.log('🎥 File identified as video, calling handleVideoUpload')
      await handleVideoUpload(preparedFile)
    } else {
      console.log('❌ Unsupported file type:', file.type || file.name)
      emit(
        'file-error',
        `Файл ${file.name} имеет неподдерживаемый тип. Допустимо загружать изображения (${Array.from(IMAGE_EXTENSIONS).join(', ')}) и видео (${Array.from(VIDEO_EXTENSIONS).join(', ')}).`,
      )
    }
  }
}

// Функция для загрузки файла на сервер
const uploadMediaToServer = async (
  file: File | undefined,
  fileCategory: 'image' | 'video',
): Promise<UploadResult> => {
  if (unmounted || !isCurrentScope.value) throw new Error('Upload cancelled')
  const pending = uncertainUpload.value
  if (pending && (!checkingCompletion || pending.file !== file)) {
    throw new Error('Check the unconfirmed upload before starting another.')
  }
  const baseLabel = fileCategory === 'video' ? 'Загрузка видео' : 'Загрузка изображения'
  const filename = file?.name ?? pending?.completion.filename
  const uploadLabel = filename ? `${baseLabel}: ${filename}` : baseLabel
  let completion = pending?.completion
  const originalEntityType = props.entityType
  const originalEntityId = props.entityId
  const controller = new AbortController()
  uploadController = controller
  let uploadQueueId: string | null = null

  try {
    if (!props.entityId) {
      throw new Error('Нельзя загрузить файл без идентификатора сущности')
    }

    uploadQueueId = uploadQueue.start(uploadLabel, 0)

    currentUploadType.value = fileCategory
    uploadStage.value = 'preparing'
    normalizedEventFile.value = null
    isUploading.value = true
    uploadProgress.value = 10
    uploadStage.value = 'uploading'

    const response =
      pending && checkingCompletion
        ? await reconcileUploadCompletion(pending.completion, { signal: controller.signal })
        : await uploadFiles({
            file,
            signal: controller.signal,
            onCompletionSession: (session: UncertainUploadCompletion) => {
              if (!retainCompletion(session, file)) {
                throw new Error('Check the unconfirmed upload before starting another.')
              }
              ownedCompletionLocation.value = session.location
              completion = session
            },
            entityType: props.entityType,
            entityId: props.entityId,
            context: 'rich-text',
            fileCategory,
            uploadUrl: props.uploadUrl,
            skipResize: fileCategory === 'video' && uploadVideoWithoutCompression.value,
            onProgress: (percentage) => {
              const next = Math.min(100, Math.max(0, Math.round(percentage)))
              if (next > uploadProgress.value) {
                uploadProgress.value = next
              }
              emit('upload-progress', next)
              if (uploadQueueId) {
                uploadQueue.update(uploadQueueId, next)
              }
            },
          })

    if (response.uncertainCompletion) {
      completion = response.uncertainCompletion
      if (!pending) ownedCompletionLocation.value = completion.location
      retainCompletion(completion, file)
    }
    if (!response.uncertainCompletion && response.error && completion) releaseCompletion(completion)
    if (
      unmounted ||
      !isCurrentScope.value ||
      originalEntityType !== props.entityType ||
      String(originalEntityId) !== String(props.entityId)
    )
      throw new Error('Return to the original entity to check this upload.')
    if (response.status === 'completion_unknown') {
      throw new Error(
        response.error ||
          'Upload completion is unconfirmed. Check this upload before starting another.',
      )
    }

    if (uploadQueueId) {
      uploadQueue.setStatus(uploadQueueId, 'processing')
    }

    uploadProgress.value = 35

    if (response.tasks.length === 0) {
      throw new Error(response.error || 'Upload failed')
    }

    const task = response.tasks[0]
    if (!task?.id) throw new Error('Upload task ID is missing in server response')
    console.log('📬 Upload task created:', task)

    const taskStatus = (task.status || response.status || '').toLowerCase()

    const releaseTerminalUncertainty = () => {
      if (
        completion &&
        completion.entityType === props.entityType &&
        String(completion.entityId) === String(props.entityId)
      ) {
        if (completion) releaseCompletion(completion)
      }
    }
    if (taskStatus === 'failed' || taskStatus === 'error') {
      releaseTerminalUncertainty()
      throw new Error(task.error || 'Upload processing failed')
    }

    let finalFileSummary: UploadResult | UploadedFileSummary | undefined
    const isCompleted = taskStatus === 'completed'

    if (isCompleted && task.file) {
      uploadStage.value = 'finalizing'
      finalFileSummary = task.file
      uploadProgress.value = 95
    } else {
      uploadStage.value = 'processing'
      uploadProgress.value = 65
      const attempts = fileCategory === 'video' ? 180 : 120
      const interval = fileCategory === 'video' ? 1500 : 1000
      finalFileSummary = await waitForTaskCompletion(task.id, {
        attempts,
        interval,
        onStatusChange: (status, payload) => {
          if (status === 'queued') {
            uploadProgress.value = Math.max(uploadProgress.value, 55)
            uploadStage.value = 'processing'
          } else if (status === 'processing') {
            const baseline = fileCategory === 'video' ? 85 : 80
            uploadProgress.value = Math.max(uploadProgress.value, baseline)
            uploadStage.value = 'processing'
          } else if (status === 'completed') {
            uploadProgress.value = 98
            uploadStage.value = 'finalizing'
          } else if (status === 'failed' || status === 'error') {
            releaseTerminalUncertainty()
            uploadStage.value = 'processing'
          }
          if (payload) {
            // keep the latest payload handy when provided by websockets
            normalizedEventFile.value = payload
          }
        },
      })
    }

    if (!finalFileSummary) {
      throw new Error('Upload processing finished without file info')
    }

    uploadStage.value = 'finalizing'
    uploadProgress.value = 99

    let normalized = normalizeUploadedFile(finalFileSummary as UploadedFileSummary)

    if (
      !normalized &&
      finalFileSummary &&
      typeof finalFileSummary === 'object' &&
      'publicUrl' in finalFileSummary
    ) {
      normalized = finalFileSummary as UploadResult
    }

    if (!normalized && normalizedEventFile.value) {
      normalized = normalizedEventFile.value
    }
    if (!normalized) {
      throw new Error('Upload processing finished without valid file info')
    }

    if (
      unmounted ||
      !isCurrentScope.value ||
      originalEntityType !== props.entityType ||
      String(originalEntityId) !== String(props.entityId)
    )
      throw new Error('Return to the original entity to check this upload.')
    if (completion && !pending) releaseCompletion(completion)
    uploadProgress.value = 100
    emit('upload-progress', 100)
    uploadedFiles.value.push(normalized.id)

    if (uploadQueueId) {
      uploadQueue.complete(uploadQueueId)
    }

    return normalized
  } catch (error: unknown) {
    console.error('❌ Upload error:', error)
    const err = error as { response?: { data?: { error?: string } }; message?: string }
    const message = err?.response?.data?.error || err?.message || 'Upload failed'
    cleanupTaskSubscription()
    if (uploadQueueId) {
      uploadQueue.fail(uploadQueueId, message)
    }
    throw new Error(message)
  } finally {
    isUploading.value = false
    uploadProgress.value = 0
    currentUploadType.value = null
    uploadStage.value = 'idle'
    normalizedEventFile.value = null
  }
}

const uploadImageToServer = async (file: File): Promise<UploadResult> => {
  return uploadMediaToServer(file, 'image')
}

const uploadVideoToServer = async (file: File): Promise<UploadResult> => {
  return uploadMediaToServer(file, 'video')
}

const handleImageUpload = async (file: File, options: FileHandleOptions = {}) => {
  try {
    if (options.skipCropper) {
      const uploadedFile = await uploadImageToServer(file)
      uploadedFile.mimeType = uploadedFile.mimeType ?? (file.type || undefined)
      uploadedFile.size = uploadedFile.size ?? file.size
      insertImageIntoEditor(uploadedFile, file.name)
      emit('file-upload', file)
      emit('files-uploaded', [uploadedFile.id])
      return
    }

    console.log('🖼️ handleImageUpload called for:', file.name, 'type:', file.type)
    // Show image cropper for image files
    const reader = new FileReader()
    reader.onload = (e) => {
      const src = e.target?.result as string
      console.log('🖼️ Image loaded for cropper, src starts with:', src.substring(0, 50))
      cropperImageUrl.value = src
      cropperFileName.value = file.name
      showImageCropper.value = true
      console.log('🖼️ ImageCropper opened')
    }
    reader.readAsDataURL(file)
  } catch (error) {
    console.error('❌ Error in handleImageUpload:', error)
    emit('file-error', `Failed to upload image: ${String(error)}`)
  }
}

const handleVideoUpload = async (file: File) => {
  try {
    const uploadedFile = await uploadVideoToServer(file)
    uploadedFile.mimeType = uploadedFile.mimeType ?? (file.type || undefined)
    uploadedFile.size = uploadedFile.size ?? file.size
    insertVideoIntoEditor(uploadedFile, file.name)
    emit('file-upload', file)
    emit('files-uploaded', [uploadedFile.id])
  } catch (error) {
    console.error('❌ Error in handleVideoUpload:', error)
    emit('file-error', `Failed to upload video: ${String(error)}`)
  }
}

const handleFileSelect = (event: Event) => {
  const target = event.target as HTMLInputElement
  const files = target.files
  if (files) {
    handleFiles(Array.from(files))
  }
  // Reset input
  target.value = ''
}

const handleFileDrop = (event: DragEvent) => {
  const files = Array.from(event.dataTransfer?.files || [])

  if (files.length === 0) {
    isDragging.value = false
    return
  }

  event.preventDefault()
  isDragging.value = false
  handleFiles(files, { skipCropper: true })
}

// Toolbar actions
const insertPickedMedia = (kind: 'image' | 'video' | 'file', selection: RichTextMediaSelection) => {
  if (!editor.value) return
  restoreSelection()

  if (isCanonicalMediaSelection(selection)) {
    const supported =
      kind === 'file'
        ? selection.node.type === 'text' &&
          selection.node.marks?.some((mark) => mark.type === 'cmsFileLink') &&
          markAllowed('cmsFileLink')
        : (kind === 'image' ? ['cmsImage', 'image'] : ['cmsVideo', 'video']).includes(
            selection.node.type,
          ) && nodeAllowed(selection.node.type)
    if (!supported) {
      emit('file-error', `Media picker returned unsupported ${kind} node`)
      return
    }
    editor.value.chain().focus().insertContent(selection.node).run()
    return
  }

  if (!selection.url) {
    emit('file-error', 'Media picker did not return a usable URL')
    return
  }
  if (kind === 'file' && markAllowed('cmsFileLink')) {
    editor.value
      .chain()
      .focus()
      .insertContent({
        type: 'text',
        text: selection.filename || 'File',
        marks: [
          {
            type: 'cmsFileLink',
            attrs: {
              id: String(selection.id),
              assetId: String(selection.id),
              label: selection.filename || 'File',
              download: true,
            },
          },
        ],
      })
      .run()
    return
  }
  if (kind === 'image' && nodeAllowed('image')) {
    editor.value
      .chain()
      .focus()
      .setImage({
        src: selection.url,
        alt: selection.filename,
        title: selection.filename,
        'data-file-id': selection.id,
      } as Record<string, unknown> & { src: string })
      .run()
    return
  }
  if (kind === 'video' && nodeAllowed('video')) {
    editor.value
      .chain()
      .focus()
      .setVideo({
        src: selection.url,
        title: selection.filename,
        controls: true,
        'data-file-id': selection.id,
        'data-type': 'video',
      })
      .run()
    return
  }
  emit('file-error', `Media picker must return a canonical CMS ${kind} node`)
}

const pickMedia = async (kind: 'image' | 'video' | 'file') => {
  if (!props.mediaPicker) return false
  try {
    const selection = await props.mediaPicker({
      kind,
      allowedNodes: props.allowedNodes ? [...props.allowedNodes] : null,
      entityId: props.entityId,
      entityType: props.entityType,
      valueVersion: props.valueVersion,
    })
    if (selection) insertPickedMedia(kind, selection)
  } catch (error) {
    emit('file-error', error instanceof Error ? error.message : String(error))
  }
  return true
}

const addImage = async () => {
  captureCurrentSelection()
  if (await pickMedia('image')) return
  if (props.allowFileUpload) {
    // Set accept attribute for images only
    if (fileInput.value) {
      fileInput.value.accept = 'image/*'
      fileInput.value.click()
      // Reset to original accept
      nextTick(() => {
        if (fileInput.value) {
          fileInput.value.accept = props.acceptedFileTypes.join(',')
        }
      })
    }
  } else {
    const url = window.prompt('Enter image URL')
    if (url) {
      restoreSelection()
      editor.value?.chain().focus().setImage({ src: url }).run()
    }
  }
}

const addVideo = async () => {
  captureCurrentSelection()
  if (await pickMedia('video')) return
  if (props.allowFileUpload) {
    // Set accept attribute for video only
    if (fileInput.value) {
      fileInput.value.accept = 'video/*'
      fileInput.value.click()
      // Reset to original accept
      nextTick(() => {
        if (fileInput.value) {
          fileInput.value.accept = props.acceptedFileTypes.join(',')
        }
      })
    }
  } else {
    const url = window.prompt('Enter video URL')
    if (url) {
      restoreSelection()
      const videoHTML = `<video src="${url}" controls style="max-width: 100%; height: auto;"></video>`
      editor.value?.chain().focus().insertContent(videoHTML).run()
    }
  }
}

const addFileLink = async () => {
  captureCurrentSelection()
  if (await pickMedia('file')) return
  emit('file-error', 'File links require a configured media picker')
}

const addLink = () => {
  const url = window.prompt('Enter URL')
  if (url) {
    editor.value?.chain().focus().setLink({ href: url }).run()
  }
}

const addHorizontalRule = () => {
  editor.value?.chain().focus().setHorizontalRule().run()
}

// Drag and drop handling
const handleDragEnter = (event: DragEvent) => {
  const hasFiles = Array.from(event.dataTransfer?.types ?? []).includes('Files')
  if (
    props.allowFileUpload &&
    canUseAnyFeature(['image', 'video']) &&
    viewMode.value === 'visual' &&
    hasFiles
  ) {
    isDragging.value = true
  }
}

const handleDragLeave = (event: DragEvent) => {
  if (!event.relatedTarget) {
    isDragging.value = false
  }
}

// Image Cropper functions
const closeCropper = () => {
  showImageCropper.value = false
  cropperImageUrl.value = ''
  cropperFileName.value = ''
}

const handleCroppedImage = async (croppedFile: File) => {
  console.log('✂️ handleCroppedImage called for:', croppedFile.name, 'type:', croppedFile.type)
  closeCropper()

  try {
    // Загружаем обрезанное изображение на сервер
    const uploadedFile = await uploadImageToServer(croppedFile)
    uploadedFile.mimeType = uploadedFile.mimeType ?? (croppedFile.type || undefined)
    uploadedFile.size = uploadedFile.size ?? croppedFile.size

    // Вставляем изображение в редактор с URL с сервера
    insertImageIntoEditor(uploadedFile, croppedFile.name)

    emit('file-upload', croppedFile)
    emit('files-uploaded', [uploadedFile.id])
  } catch (error) {
    console.error('❌ Error processing cropped image:', error)
    emit('file-error', `Failed to upload image: ${String(error)}`)
  }
}

// Watch for external changes
watch(
  [() => props.modelValue, () => props.valueVersion],
  ([newValue]) => {
    if (!editor.value) return

    try {
      const currentContent =
        props.outputFormat === 'json' ? editor.value.getJSON() : editor.value.getHTML()
      const isSame =
        props.outputFormat === 'json'
          ? JSON.stringify(currentContent) === JSON.stringify(newValue)
          : currentContent === newValue

      if (isSame) return

      setValidatedContent(newValue || '', 'external')
    } catch (error) {
      console.warn('RichTextEditor: Failed to update content:', error)
    }
  },
  { immediate: true },
)

// Watch disabled/readonly state
watch([() => props.disabled, () => props.readonly], ([disabled, readonly]) => {
  editor.value?.setEditable(!disabled && !readonly)
})

// Initialize
onMounted(() => {
  // Set up drag and drop listeners
  if (props.allowFileUpload && canUseAnyFeature(['image', 'video'])) {
    document.addEventListener('dragenter', handleDragEnter)
    document.addEventListener('dragleave', handleDragLeave)
    document.addEventListener('dragover', (e) => e.preventDefault())
  }

  // Wait for editor to be ready and update counts
  nextTick(() => {
    setTimeout(() => {
      updateCounts()
    }, 100)
  })
})

// Cleanup
onBeforeUnmount(() => {
  unmounted = true
  uploadController?.abort()
  editor.value?.destroy()
  cleanupTaskSubscription()

  if (props.allowFileUpload && canUseAnyFeature(['image', 'video'])) {
    document.removeEventListener('dragenter', handleDragEnter)
    document.removeEventListener('dragleave', handleDragLeave)
  }
})
</script>

<style scoped>
.rich-text-editor {
  overflow: visible;
  background-color: var(--color-card);
  border: 1px solid var(--color-border-primary);
  border-radius: 0.5rem;
  position: relative;
}

/* Toolbar Styles */
.editor-toolbar {
  position: sticky;
  top: 0;
  z-index: 20;
  background-color: var(--color-card);
  border-bottom: 1px solid var(--color-border-primary);
  padding: 0.75rem;
  box-shadow: 0 1px 3px color-mix(in srgb, var(--color-text-primary) 12%, transparent);
  border-radius: 0.5rem 0.5rem 0 0;
}

.toolbar-content {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  flex-wrap: wrap;
}

.toolbar-section {
  display: flex;
  align-items: center;
  gap: 0.25rem;
}

.toolbar-section:not(:last-child):after {
  content: '';
  width: 1px;
  height: 1.5rem;
  background-color: var(--color-border-primary);
  margin: 0 0.5rem;
}

/* Mobile Toolbar */
.toolbar-mobile-toggle {
  margin-bottom: 0.5rem;
}

@media (max-width: 1024px) {
  .editor-toolbar {
    position: static;
  }
}

/* Editor Content */
.editor-content {
  background-color: var(--color-card);
  position: relative;
  min-height: 200px;
}

.visual-editor {
  position: relative;
}

/* JSON Editor */
.json-editor {
  position: relative;
}

.json-editor-header {
  display: flex;
  justify-content: between;
  align-items: center;
  padding: 0.75rem 1rem 0.5rem;
  border-bottom: 1px solid var(--color-border-secondary);
}

.json-textarea {
  width: 100%;
  min-height: 300px;
  padding: 1rem;
  border: none;
  background-color: var(--color-surface);
  color: var(--color-text-primary);
  font-family: 'JetBrains Mono', 'SF Mono', Monaco, Consolas, monospace;
  font-size: 0.875rem;
  line-height: 1.5;
  resize: vertical;
  outline: none;
}

.json-textarea.json-error {
  border-left: 4px solid var(--color-error);
}

.json-error-message {
  padding: 0.5rem 1rem;
  background-color: var(--color-error);
  color: white;
  font-size: 0.875rem;
}

/* Upload Overlay */
.upload-overlay {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background-color: rgba(var(--color-primary-rgb), 0.1);
  border: 2px dashed var(--color-primary);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 10;
}

.upload-message {
  text-align: center;
  color: var(--color-primary);
}

.upload-icon {
  font-size: 3rem;
  margin-bottom: 0.5rem;
}

.upload-text {
  font-size: 1.125rem;
  font-weight: 600;
}

/* Status Bar */
.status-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0.5rem 1rem;
  background-color: var(--color-surface);
  border-top: 1px solid var(--color-border-primary);
  font-size: 0.75rem;
  color: var(--color-text-secondary);
  flex-wrap: wrap;
  gap: 1rem;
}

.status-left,
.status-right {
  display: flex;
  align-items: center;
  gap: 1rem;
}

.character-count,
.word-count {
  display: flex;
  align-items: center;
  gap: 0.25rem;
}

.count-current {
  font-weight: 600;
  color: var(--color-text-primary);
}

.count-warning {
  color: var(--color-error);
  font-weight: 600;
}

.view-mode-indicator {
  font-weight: 500;
  color: var(--color-text-primary);
}

.upload-status {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-weight: 500;
  color: var(--color-primary);
}

.loading-spinner {
  animation: spin 1s linear infinite;
}

.upload-status-text {
  min-width: 0;
  white-space: nowrap;
}

.upload-progress-track {
  width: 120px;
  height: 6px;
  border-radius: 9999px;
  background-color: rgba(148, 163, 184, 0.35);
  overflow: hidden;
}

.upload-progress-fill {
  height: 100%;
  background-color: var(--color-primary);
  transition: width 0.3s ease;
}

.upload-floating-indicator {
  position: absolute;
  top: 1rem;
  right: 1rem;
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.75rem 1rem;
  border-radius: 0.75rem;
  background-color: rgba(15, 23, 42, 0.8);
  backdrop-filter: blur(6px);
  color: #fff;
  box-shadow: 0 10px 25px rgba(15, 23, 42, 0.35);
  pointer-events: none;
  z-index: 15;
}

.upload-floating-icon {
  font-size: 1.25rem;
  animation: spin 1s linear infinite;
}

.upload-floating-body {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
}

.upload-floating-track {
  width: 160px;
  height: 6px;
  border-radius: 9999px;
  background-color: rgba(255, 255, 255, 0.35);
  overflow: hidden;
}

.upload-floating-fill {
  height: 100%;
  background-color: rgba(255, 255, 255, 0.9);
  transition: width 0.3s ease;
}

@keyframes spin {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}

/* TipTap Editor Styles */
:deep(.ProseMirror) {
  outline: none;
  padding: 1rem;
  color: var(--color-text-primary);
  background-color: var(--color-card);
  line-height: 1.6;
}

:deep(.ProseMirror h1) {
  font-size: 1.875rem;
  font-weight: bold;
  margin: 1.5rem 0 1rem;
  color: var(--color-text-primary);
}

:deep(.ProseMirror h2) {
  font-size: 1.5rem;
  font-weight: bold;
  margin: 1.25rem 0 0.75rem;
  color: var(--color-text-primary);
}

:deep(.ProseMirror h3) {
  font-size: 1.25rem;
  font-weight: bold;
  margin: 1rem 0 0.5rem;
  color: var(--color-text-primary);
}

:deep(.ProseMirror p) {
  margin-bottom: 0.2em;
  color: var(--color-text-primary);
}

:deep(.ProseMirror div) {
  display: inline-flex;
  margin-left: 0.7rem;
}

:deep(.ProseMirror ul, .ProseMirror ol) {
  margin: 1rem 0;
  padding-left: 1rem;
}

:deep(.ProseMirror li) {
  margin-bottom: 0.2em;
}

:deep(.ProseMirror blockquote) {
  border-left: 4px solid var(--color-primary);
  padding-left: 1rem;
  margin: 1.5rem 0;
  font-style: italic;
  color: var(--color-text-secondary);
}

:deep(.ProseMirror pre) {
  background-color: var(--color-surface);
  border: 1px solid var(--color-border-primary);
  border-radius: 0.5rem;
  padding: 1rem;
  margin: 1rem 0;
  overflow-x: auto;
}

:deep(.ProseMirror code) {
  background-color: var(--color-surface-variant);
  color: var(--color-text-primary);
  padding: 0.125rem 0.375rem;
  border-radius: 0.25rem;
  font-size: 0.875rem;
  border: 1px solid var(--color-border-primary);
}

:deep(.ProseMirror pre code) {
  background-color: transparent;
  padding: 0;
  border: none;
}

:deep(.ProseMirror a) {
  color: var(--color-primary);
  text-decoration: underline;
}

:deep(.ProseMirror [data-type='taskList']) {
  list-style: none;
  padding-left: 0;
}

:deep(.ProseMirror [data-type='taskList'] div) {
  display: inline-flex;
}

:deep(.ProseMirror [data-type='taskList'] p) {
  display: inline-flex;
}

:deep(.ProseMirror [data-type='taskItem']) {
  display: flex;
  align-items: flex-start;
  gap: 0.5rem;
}

:deep(.ProseMirror [data-type='taskItem'] input[type='checkbox']) {
  margin-top: 0.25rem;
}

:deep(.ProseMirror hr) {
  border: none;
  border-top: 2px solid var(--color-border-primary);
  margin: 2rem 0;
}

/* Media Styles */
:deep(.ProseMirror .editor-image) {
  max-width: 100%;
  height: auto;
  border-radius: 0.5rem;
  margin: 1rem 0;
  box-shadow: 0 4px 6px rgba(0, 0, 0, 0.1);
}

:deep(.ProseMirror .editor-video) {
  max-width: 100%;
  height: auto;
  border-radius: 0.5rem;
  margin: 1rem 0;
  display: block;
}

:deep(.ProseMirror .editor-video-wrapper) {
  position: relative;
  width: 100%;
  border-radius: 0.75rem;
  overflow: hidden;
  background: rgba(0, 0, 0, 0.05);
}

/* Placeholder */
:deep(.ProseMirror p.is-editor-empty:first-child::before) {
  content: attr(data-placeholder);
  float: left;
  color: var(--color-text-tertiary);
  pointer-events: none;
  height: 0;
}

/* Responsive Design */
@media (max-width: 640px) {
  .mobile-hidden {
    display: none;
  }

  .toolbar-content {
    flex-direction: column;
    align-items: stretch;
  }

  .toolbar-section {
    justify-content: center;
    padding: 0.25rem 0;
  }

  .status-bar {
    flex-direction: column;
    align-items: flex-start;
    gap: 0.5rem;
  }

  .toolbar-section:not(:last-child):after {
    display: none;
  }

  :deep(.ProseMirror) {
    padding: 0.75rem;
  }

  .json-textarea {
    padding: 0.75rem;
    font-size: 0.8rem;
  }
}

/* Hidden input */
.hidden {
  display: none;
}

:deep(.ProseMirror ul) {
  list-style-type: disc;
  margin-left: 2em;
  padding-left: 0;
}
:deep(.ProseMirror ol) {
  list-style-type: decimal;
  margin-left: 2em;
  padding-left: 0;
}
:deep(.ProseMirror ul li) {
  margin-bottom: 0.2em;
  position: relative;
  padding-left: 0.7em;
}
:deep(.ProseMirror ol li) {
  margin-bottom: 0.2em;
  position: relative;
  padding-left: 0.7em;
}
:deep(.ProseMirror ul li::marker) {
  color: var(--color-primary, #3b82f6);
  font-size: 1.1em;
}
:deep(.ProseMirror ol li::marker) {
  color: var(--color-primary, #3b82f6);
  font-size: 1.1em;
}
:deep(.ProseMirror [data-type='taskList']) {
  list-style: none;
  margin-left: 1em;
  padding-left: 0;
}
:deep(.ProseMirror [data-type='taskItem']) {
  display: flex;
  align-items: flex-start;
  gap: 0.5em;
  margin-bottom: 0.2em;
  padding-left: 0;
}
:deep(.ProseMirror [data-type='taskItem'] input[type='checkbox']) {
  accent-color: var(--color-primary, #3b82f6);
  margin-top: 0.2em;
  margin-right: 0.5em;
  width: 1.1em;
  height: 1.1em;
}
</style>
