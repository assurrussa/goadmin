<template>
  <div class="space-y-3">
    <div v-if="files.length" class="space-y-3">
      <div
        v-for="file in files"
        :key="file.id"
        class="border border-border-primary rounded-md px-4 py-3 space-y-2 transition hover:border-primary cursor-grab active:cursor-grabbing"
        draggable="true"
        @dragstart="(event) => handleDragStart(event, file)"
      >
        <div class="flex gap-3 items-start">
          <div
            class="w-16 h-16 shrink-0 rounded-md overflow-hidden border border-border-primary bg-surface-variant flex items-center justify-center"
          >
            <template v-if="previewUrl(file)">
              <div class="relative h-full w-full">
                <img
                  :src="previewUrl(file)"
                  class="h-full w-full object-cover"
                  :alt="file.originalFilename || file.filename"
                />
                <div
                  v-if="isVideo(file)"
                  class="absolute inset-0 flex items-center justify-center bg-black/40 text-white"
                >
                  <span class="text-lg">▶</span>
                </div>
              </div>
            </template>
            <template v-else-if="isVideo(file)">
              <div
                class="flex h-full w-full flex-col items-center justify-center gap-1 text-primary"
              >
                <span class="text-lg">🎬</span>
                <span class="text-[10px] uppercase tracking-wide text-text-tertiary">video</span>
              </div>
            </template>
            <template v-else>
              <span class="text-xs text-text-tertiary">no preview</span>
            </template>
          </div>

          <div class="min-w-0 flex-1 space-y-1">
            <p class="text-sm font-medium text-text-primary truncate">
              {{ file.originalFilename || file.filename }}
            </p>
            <p class="text-xs text-text-tertiary">
              ID: {{ file.id }} · {{ file.mimeType || 'unknown' }} · {{ formatFileSize(file.size) }}
            </p>
            <p v-if="file.createdAt" class="text-xs text-text-tertiary">
              Загружено: {{ formatDateTime(file.createdAt) }}
            </p>
          </div>

          <div class="flex flex-col items-end gap-1">
            <AppBadge v-if="isAttached(file.id)" variant="success" class="text-[11px]">
              В тексте
            </AppBadge>
            <AppButton
              v-if="allowDeleteUnattached && !isAttached(file.id)"
              type="button"
              variant="danger"
              size="xs"
              :disabled="deletingIds.has(file.id)"
              @click="() => handleDelete(file)"
            >
              <span v-if="deletingIds.has(file.id)">Удаление…</span>
              <span v-else>Удалить</span>
            </AppButton>
          </div>
        </div>

        <a
          v-if="file.publicUrl"
          :href="file.publicUrl"
          class="block text-xs text-primary hover:underline break-all"
          target="_blank"
          rel="noopener"
        >
          {{ file.publicUrl }}
        </a>
        <a
          v-else-if="file.url"
          :href="file.url"
          class="block text-xs text-primary hover:underline break-all"
          target="_blank"
          rel="noopener"
        >
          {{ file.url }}
        </a>
      </div>
    </div>

    <div
      v-else
      class="border border-dashed border-border-primary rounded-md px-4 py-6 text-center text-sm text-text-tertiary"
    >
      Файлы пока не прикреплены
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import AppBadge from '@/components/ui/AppBadge.vue'
import AppButton from '@/components/ui/AppButton.vue'
import { deleteUploadedFile } from '@/services/fileUploadService'
import { RICH_TEXT_ATTACHMENT_MIME, type RichTextAttachmentPayload } from '@/constants/richText'

export interface RichTextAttachmentFile {
  id: number
  objectId?: number | null
  objectType: string
  originalFilename?: string
  filename: string
  mimeType?: string
  size?: number
  folderPath?: string
  url?: string
  publicUrl?: string
  thumbnailUrl?: string
  fullPath?: string
  isPrimary: boolean
  createdAt?: string | Date
  updatedAt?: string | Date
  data?: {
    provider?: {
      driver?: string
    }
    width?: number
    height?: number
    thumbnail?: string
    alt?: string
  } | null
}

const props = defineProps<{
  files: RichTextAttachmentFile[]
  attachedIds?: number[]
  allowDeleteUnattached?: boolean
}>()

const emit = defineEmits<{
  removed: [id: number]
}>()

const deletingIds = ref(new Set<number>())

const attachedIdsSet = computed(() => new Set(props.attachedIds ?? []))

const formatFileSize = (bytes?: number | null): string => {
  if (!bytes || bytes <= 0) {
    return '0 B'
  }

  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const exponent = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  const value = bytes / Math.pow(1024, exponent)
  const formatted = exponent === 0 || value >= 10 ? value.toFixed(0) : value.toFixed(1)

  return `${formatted} ${units[exponent]}`
}

const dateFormatter = new Intl.DateTimeFormat('ru-RU', {
  dateStyle: 'short',
  timeStyle: 'short',
})

const formatDateTime = (value?: string | Date | null): string => {
  if (!value) {
    return ''
  }

  const date = value instanceof Date ? value : new Date(value)
  if (Number.isNaN(date.getTime())) {
    return ''
  }

  return dateFormatter.format(date)
}

const isVideo = (file: RichTextAttachmentFile): boolean => {
  const mime = file.mimeType?.toLowerCase() ?? ''
  if (mime.startsWith('video/')) {
    return true
  }
  const name = (file.filename || file.originalFilename || '').toLowerCase()
  return name.endsWith('.mp4') || name.endsWith('.webm') || name.endsWith('.mov')
}

const previewUrl = (file: RichTextAttachmentFile): string | undefined => {
  if (isVideo(file)) {
    return file.thumbnailUrl ?? undefined
  }

  return file.thumbnailUrl || file.publicUrl || file.url || undefined
}

const isAttached = (fileId: number): boolean => attachedIdsSet.value.has(fileId)

const buildPayload = (file: RichTextAttachmentFile): RichTextAttachmentPayload => ({
  id: file.id,
  url: file.url || undefined,
  publicUrl: file.publicUrl || undefined,
  fullPath: file.fullPath || undefined,
  thumbnailUrl: previewUrl(file) || undefined,
  mimeType: file.mimeType,
  filename: file.filename,
  originalFilename: file.originalFilename,
})

const handleDragStart = (event: DragEvent, file: RichTextAttachmentFile) => {
  const payload = buildPayload(file)

  if (!event.dataTransfer) {
    return
  }

  try {
    const serialized = JSON.stringify(payload)
    event.dataTransfer.setData(RICH_TEXT_ATTACHMENT_MIME, serialized)
    event.dataTransfer.setData('text/plain', serialized)
    const rawUrl = payload.publicUrl || payload.fullPath || payload.url
    if (rawUrl) {
      event.dataTransfer.setData('text/uri-list', rawUrl)
    }
    event.dataTransfer.effectAllowed = 'copyMove'
    event.dataTransfer.dropEffect = 'copy'
  } catch (error) {
    console.warn('RichTextAttachmentList: failed to serialize payload', error)
  }
}

const handleDelete = async (file: RichTextAttachmentFile) => {
  if (!props.allowDeleteUnattached || isAttached(file.id)) {
    return
  }

  if (deletingIds.value.has(file.id)) {
    return
  }

  const confirmed = window.confirm('Удалить файл?')
  if (!confirmed) {
    return
  }

  deletingIds.value.add(file.id)

  try {
    await deleteUploadedFile(file.id)
    emit('removed', file.id)
  } catch (error) {
    console.error('RichTextAttachmentList: failed to delete file', error)
    alert('Не удалось удалить файл. Проверьте консоль для подробностей.')
  } finally {
    deletingIds.value.delete(file.id)
  }
}
</script>
