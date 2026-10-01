<template>
  <div class="image-cropper-modal" @click="handleBackdropClick">
    <div class="modal-content" @click.stop>
      <div class="modal-header">
        <h3 class="modal-title">Обрезать изображение</h3>
        <AppButton
          @click="closeModal"
          variant="ghost"
          size="icon-sm"
          aria-label="Закрыть"
          :disabled="isProcessing"
          :class="{ 'opacity-60 cursor-not-allowed': isProcessing }"
        >
          &times;
        </AppButton>
      </div>

      <div class="modal-body">
        <div class="crop-container">
          <img ref="imageRef" :src="imageUrl" :alt="fileName" class="crop-image" />
        </div>

        <div class="crop-controls">
          <div class="control-group">
            <label class="control-label">Соотношение сторон:</label>
            <select
              v-model="aspectRatio"
              @change="updateAspectRatio"
              class="select select-sm"
              :disabled="isProcessing"
            >
              <option
                v-for="option in availableAspectRatios"
                :key="option.value"
                :value="option.value"
              >
                {{ option.label }}
              </option>
            </select>
          </div>

          <div class="control-group">
            <label class="control-label">Качество JPEG:</label>
            <input
              v-model="jpegQuality"
              type="range"
              min="0.1"
              max="1"
              step="0.1"
              class="quality-slider"
              :disabled="isProcessing"
            />
            <span class="quality-value">{{ Math.round(jpegQuality * 100) }}%</span>
          </div>
        </div>
      </div>

      <div class="modal-footer">
        <AppButton @click="closeModal" variant="secondary" type="button" :disabled="isProcessing">
          Отмена
        </AppButton>
        <AppButton @click="cropImage" variant="primary" type="button" :disabled="isProcessing">
          <span v-if="isProcessing" class="inline-flex items-center gap-2">
            <Loader2 class="h-4 w-4 animate-spin" aria-hidden="true" />
            Обработка...
          </span>
          <span v-else>Обрезать</span>
        </AppButton>
      </div>

      <div v-if="isProcessing" class="processing-overlay" role="status" aria-live="polite">
        <div class="processing-card">
          <span class="inline-spinner" aria-hidden="true"></span>
          <span>Готовим изображение...</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue'
import AppButton from '@/components/ui/AppButton.vue'
import Cropper from 'cropperjs'
import { Loader2 } from 'lucide-vue-next'
import 'cropperjs/dist/cropper.css'

type GetCroppedCanvasOptions = Parameters<Cropper['getCroppedCanvas']>[0]

interface Props {
  imageUrl: string
  fileName: string
  defaultAspectRatio?: string | number
  aspectRatioOptions?: AspectRatioOption[]
  maxWidth?: number
  maxHeight?: number
  outputMimeType?: string
}

interface AspectRatioOption {
  value: string
  label: string
}

interface Emits {
  (e: 'close'): void
  (e: 'crop', croppedFile: File): void
}

const props = withDefaults(defineProps<Props>(), {
  defaultAspectRatio: 'free',
  aspectRatioOptions: () => [
    { value: 'free', label: 'Свободное' },
    { value: '1', label: '1:1 (квадрат)' },
    { value: '1.333', label: '4:3' },
    { value: '1.777', label: '16:9' },
    { value: '2', label: '2:1' },
  ],
  maxWidth: 1200,
  maxHeight: 1200,
})
const emit = defineEmits<Emits>()

const imageRef = ref<HTMLImageElement>()
const aspectRatio = ref(String(props.defaultAspectRatio))
const jpegQuality = ref(0.8)
const availableAspectRatios = computed(() => props.aspectRatioOptions)
const isProcessing = ref(false)

let cropper: Cropper | null = null

onMounted(() => {
  nextTick(() => {
    initializeCropper()
  })
})

onUnmounted(() => {
  if (cropper) {
    cropper.destroy()
  }
})

const initializeCropper = () => {
  if (!imageRef.value) return

  cropper = new Cropper(imageRef.value, {
    aspectRatio: aspectRatio.value === 'free' ? NaN : parseFloat(aspectRatio.value),
    viewMode: 1,
    dragMode: 'move',
    autoCropArea: 0.8,
    responsive: true,
    cropBoxResizable: true,
    cropBoxMovable: true,
    guides: true,
    center: true,
    highlight: false,
    background: false,
    zoomable: true,
    scalable: true,
    ready() {
      updateAspectRatio()
    },
  })
}

const updateAspectRatio = () => {
  if (!cropper) return

  const ratio = aspectRatio.value === 'free' ? NaN : parseFloat(aspectRatio.value)
  cropper.setAspectRatio(ratio)
}

const cropImage = () => {
  if (!cropper || isProcessing.value) return

  isProcessing.value = true

  const options: GetCroppedCanvasOptions = {
    imageSmoothingEnabled: true,
    imageSmoothingQuality: 'high',
  }

  if (props.maxWidth) {
    options.width = props.maxWidth
  }

  if (props.maxHeight) {
    options.height = props.maxHeight
  }

  const canvas = cropper.getCroppedCanvas(options)

  if (!canvas) {
    isProcessing.value = false
    return
  }

  // Определяем тип файла
  const normalizedFileName = props.fileName.toLowerCase()
  const isJpeg = normalizedFileName.endsWith('.jpg') || normalizedFileName.endsWith('.jpeg')
  const inferredType = isJpeg ? 'image/jpeg' : 'image/png'
  const mimeType = props.outputMimeType ?? inferredType
  const adjustedFileName = adjustFileExtension(props.fileName, mimeType)

  canvas.toBlob(
    (blob) => {
      isProcessing.value = false
      if (blob) {
        const file = new File([blob], adjustedFileName, {
          type: mimeType,
          lastModified: Date.now(),
        })
        emit('crop', file)
      }
    },
    mimeType,
    isJpeg ? jpegQuality.value : 1,
  )
}

const closeModal = () => {
  if (isProcessing.value) {
    return
  }
  emit('close')
}

const handleBackdropClick = () => {
  if (isProcessing.value) {
    return
  }
  closeModal()
}

const adjustFileExtension = (fileName: string, mimeType: string): string => {
  const extensionMap: Record<string, string> = {
    'image/jpeg': '.jpg',
    'image/png': '.png',
    'image/webp': '.webp',
  }

  const desiredExtension = extensionMap[mimeType]
  if (!desiredExtension) {
    return fileName
  }

  const dotIndex = fileName.lastIndexOf('.')
  const baseName = dotIndex >= 0 ? fileName.substring(0, dotIndex) : fileName
  return `${baseName}${desiredExtension}`
}
</script>

<style scoped>
.image-cropper-modal {
  position: fixed;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  background: rgba(0, 0, 0, 0.8);
  display: flex;
  justify-content: center;
  align-items: center;
  z-index: 10000;
}

.modal-content {
  position: relative;
  background: var(--color-card, white);
  border-radius: 0.5rem;
  max-width: 800px;
  max-height: 80vh;
  width: 90%;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  box-shadow:
    0 20px 25px -5px rgba(0, 0, 0, 0.1),
    0 10px 10px -5px rgba(0, 0, 0, 0.04);
}

.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 1rem;
  border-bottom: 1px solid var(--color-border-primary, #e0e0e0);
  background: var(--color-surface, #f8f9fa);
}

.modal-title {
  margin: 0;
  font-size: 1.25rem;
  font-weight: 600;
  color: var(--color-text-primary, #333);
}

.close-btn {
  background: none;
  border: none;
  font-size: 1.5rem;
  cursor: pointer;
  color: var(--color-text-secondary, #666);
  padding: 0;
  width: 30px;
  height: 30px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 0.25rem;
  transition: all 0.2s;
}

.close-btn:hover {
  color: var(--color-text-primary, #000);
  background: var(--color-surface-hover, #e9ecef);
}

.close-btn.is-disabled {
  cursor: not-allowed;
  opacity: 0.6;
}

.modal-body {
  padding: 1rem;
  flex: 1;
  overflow: auto;
}

.crop-container {
  width: 100%;
  height: 400px;
  display: flex;
  justify-content: center;
  align-items: center;
  background: var(--color-surface, #f5f5f5);
  border-radius: 0.5rem;
  overflow: hidden;
}

.crop-image {
  max-width: 100%;
  max-height: 100%;
  display: block;
}

.crop-controls {
  display: flex;
  gap: 2rem;
  align-items: center;
  margin-top: 1rem;
  padding: 1rem;
  background: var(--color-surface, #f8f9fa);
  border-radius: 0.5rem;
}

.control-group {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.control-label {
  font-weight: 500;
  font-size: 0.875rem;
  color: var(--color-text-primary, #333);
}

.quality-slider {
  width: 100px;
  margin: 0 0.5rem;
}

.quality-value {
  font-size: 0.875rem;
  color: var(--color-text-secondary, #666);
  font-weight: 500;
}

.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 0.75rem;
  padding: 1rem;
  border-top: 1px solid var(--color-border-primary, #e0e0e0);
  background: var(--color-surface, #f8f9fa);
}

.processing-overlay {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(15, 23, 42, 0.22);
  backdrop-filter: blur(2px);
  z-index: 5;
}

.processing-card {
  display: inline-flex;
  align-items: center;
  gap: 0.6rem;
  border-radius: 0.75rem;
  border: 1px solid var(--color-border-primary, #e0e0e0);
  background: var(--color-card, #fff);
  padding: 0.65rem 0.9rem;
  font-size: 0.9rem;
  font-weight: 600;
  color: var(--color-text-primary, #111827);
  box-shadow:
    0 10px 20px -12px rgba(0, 0, 0, 0.35),
    0 4px 8px -6px rgba(0, 0, 0, 0.25);
}

.inline-spinner {
  width: 1.1rem;
  height: 1.1rem;
  border-radius: 9999px;
  border: 2px solid rgba(148, 163, 184, 0.5);
  border-top-color: var(--color-primary, #2563eb);
  animation: cropper-spin 0.9s linear infinite;
}

@keyframes cropper-spin {
  to {
    transform: rotate(360deg);
  }
}

/* Responsive */
@media (max-width: 768px) {
  .modal-content {
    width: 95%;
    max-height: 90vh;
  }

  .crop-container {
    height: 300px;
  }

  .crop-controls {
    flex-direction: column;
    gap: 1rem;
  }

  .control-group {
    justify-content: space-between;
    width: 100%;
  }
}
</style>
