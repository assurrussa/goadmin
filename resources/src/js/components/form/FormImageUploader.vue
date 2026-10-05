<template>
  <div class="form-control w-full">
    <label v-if="label" class="label">
      <span class="label-text" :class="labelClass">
        {{ label }}
        <span v-if="required" class="text-error ml-1">*</span>
      </span>
      <span v-if="labelAlt" class="label-text-alt">{{ labelAlt }}</span>
    </label>

    <ImageUploader
      :model-value="modelValue"
      :object-type="objectType"
      :object-id="objectId"
      :upload-recovery-key="uploadRecoveryKey || name"
      :disabled="disabled || !uploads"
      :readonly="readonly"
      :show-progress="showProgress"
      :upload-label="uploadLabel"
      :max-file-size="maxFileSize"
      :accepted-file-types="acceptedFileTypes"
      :upload-url="uploadUrl"
      :delete-url="deleteUrl"
      :context="context"
      :enable-cropper="props.enableCropper"
      :crop-enabled="cropEnabled"
      :show-crop-toggle="false"
      :cropper-default-aspect-ratio="cropperDefaultAspectRatio"
      :cropper-aspect-ratio-options="cropperAspectRatioOptions"
      :cropper-max-width="cropperMaxWidth"
      :cropper-max-height="cropperMaxHeight"
      :cropper-output-mime-type="cropperOutputMimeType"
      @update:model-value="handleUpdate"
      @success="handleSuccess"
      @error="handleError"
      @progress="handleProgress"
      @deleted="handleDeleted"
    />

    <div
      v-if="props.enableCropper"
      class="mt-2 flex items-center justify-between rounded-lg border border-border-secondary bg-card px-3 py-2 text-xs text-text-secondary"
    >
      <label class="flex cursor-pointer select-none items-center gap-2">
        <input
          v-model="cropEnabled"
          type="checkbox"
          class="checkbox checkbox-sm"
          :disabled="!canToggleCrop"
        />
        <span class="font-medium text-text-primary">Обрезать изображение</span>
      </label>
      <span class="text-text-tertiary">Можно отключить перед загрузкой</span>
    </div>

    <label class="label" v-if="description || internalError || hint">
      <span v-if="description" class="label-text-alt text-text-secondary">
        {{ description }}
      </span>
      <span v-if="internalError" class="label-text-alt text-error">
        {{ internalError }}
      </span>
      <span v-if="hint && !internalError" class="label-text-alt text-text-tertiary">
        {{ hint }}
      </span>
    </label>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useAdminCapabilities } from '@/composables/useAdminCapabilities'
import ImageUploader, {
  type CropperAspectRatioOption,
  type UploadedFile,
} from './ImageUploader.vue'

export interface FormImageUploaderProps {
  name?: string
  uploadRecoveryKey?: string
  modelValue?: UploadedFile | null
  label?: string
  labelAlt?: string
  description?: string
  error?: string
  hint?: string
  disabled?: boolean
  readonly?: boolean
  required?: boolean
  labelClass?: string

  objectType: string
  objectId: string | number | null

  showProgress?: boolean
  uploadLabel?: string
  maxFileSize?: number // in MB
  acceptedFileTypes?: string[]
  uploadUrl?: string
  deleteUrl?: string
  context?: string
  enableCropper?: boolean
  cropperDefaultAspectRatio?: string | number
  cropperAspectRatioOptions?: CropperAspectRatioOption[]
  cropperMaxWidth?: number
  cropperMaxHeight?: number
  cropperOutputMimeType?: string
}

const props = withDefaults(defineProps<FormImageUploaderProps>(), {
  disabled: false,
  readonly: false,
  required: false,
  showProgress: true,
  maxFileSize: 10,
  acceptedFileTypes: () => ['image/jpeg', 'image/png', 'image/gif', 'image/webp'],
  uploadUrl: '/files',
  deleteUrl: undefined,
  enableCropper: false,
})

const emit = defineEmits<{
  'update:modelValue': [value: UploadedFile | null]
  success: [response: unknown]
  error: [error: string]
  change: [value: UploadedFile | null]
  deleted: []
  progress: [percentage: number]
}>()

const { uploads } = useAdminCapabilities()

const internalError = ref<string | null>(props.error || null)
const cropEnabled = ref(props.enableCropper)

const canToggleCrop = computed(() => uploads.value && !(props.disabled || props.readonly))

watch(
  () => props.error,
  (newError) => {
    internalError.value = newError || null
  },
)

watch(
  () => props.enableCropper,
  (enabled) => {
    if (!enabled) {
      cropEnabled.value = false
      return
    }
    if (!cropEnabled.value) {
      cropEnabled.value = true
    }
  },
)

const handleUpdate = (value: UploadedFile | null) => {
  emit('update:modelValue', value)
  emit('change', value)
  internalError.value = null // Clear error on successful update
}

const handleSuccess = (response: unknown) => {
  emit('success', response)
}

const handleError = (error: string) => {
  internalError.value = error
  emit('error', error)
}

const handleProgress = (percentage: number) => {
  emit('progress', percentage)
}

const handleDeleted = () => {
  emit('deleted')
}
</script>
