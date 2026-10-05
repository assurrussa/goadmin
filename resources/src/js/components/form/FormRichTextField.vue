<template>
  <div class="form-control w-full">
    <!-- Лейбл -->
    <label v-if="label" class="label">
      <span class="label-text" :class="labelClass">
        {{ label }}
        <span v-if="required" class="text-error ml-1">*</span>
      </span>
      <span v-if="labelAlt" class="label-text-alt">{{ labelAlt }}</span>
    </label>

    <!-- Rich Text Editor Wrapper -->
    <div class="rich-text-form-field" :class="fieldClasses">
      <RichTextEditor
        :model-value="editorModelValue"
        :placeholder="placeholder"
        :disabled="disabled"
        :readonly="readonly"
        :show-character-count="showCharacterCount"
        :show-word-count="showWordCount"
        :max-characters="maxCharacters"
        :entity-type="entityType"
        :entity-id="entityId"
        :upload-recovery-key="uploadRecoveryKey || name"
        :allow-file-upload="allowFileUpload"
        :max-file-size="maxFileSize"
        :max-video-file-size="maxVideoFileSize"
        :accepted-file-types="acceptedFileTypes"
        :min-height="minHeight"
        :max-height="maxHeight"
        :allowed-nodes="allowedNodes"
        :allowed-marks="allowedMarks"
        :allowed-features="allowedFeatures"
        :media-picker="mediaPicker"
        :value-version="valueVersion"
        :on-invalid-content="onInvalidContent"
        @update:model-value="handleUpdate"
        @file-upload="handleFileUpload"
        @file-error="handleFileError"
        @files-uploaded="handleFilesUploaded"
        @focus="handleFocus"
        @blur="handleBlur"
        @invalid-content="handleInvalidContent"
      />
    </div>

    <!-- Описание и ошибки -->
    <label class="label" v-if="description || error || hint">
      <span v-if="description" class="label-text-alt text-text-secondary">
        {{ description }}
      </span>
      <span v-if="error" class="label-text-alt text-error">
        {{ error }}
      </span>
      <span v-if="hint && !error" class="label-text-alt text-text-tertiary">
        {{ hint }}
      </span>
    </label>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { RichTextEditor } from '@/components/form'
import type {
  RichTextInvalidContent,
  RichTextMediaPicker,
  RichTextValueVersion,
} from './richTextContract'

export interface RichTextFormFieldProps {
  modelValue?: unknown
  label?: string
  labelAlt?: string
  placeholder?: string
  description?: string
  error?: string
  hint?: string
  size?: 'xs' | 'sm' | 'md' | 'lg' | 'xl'
  variant?:
    | 'neutral'
    | 'primary'
    | 'secondary'
    | 'accent'
    | 'info'
    | 'success'
    | 'warning'
    | 'error'
  disabled?: boolean
  readonly?: boolean
  required?: boolean
  labelClass?: string

  /** Stable form field name used to restore only this editor's upload. */
  name?: string
  uploadRecoveryKey?: string
  entityType?: string
  entityId?: string | number | null

  // Rich Text specific props
  showCharacterCount?: boolean
  showWordCount?: boolean
  maxCharacters?: number
  allowFileUpload?: boolean
  maxFileSize?: number // в MB
  maxVideoFileSize?: number // в MB
  acceptedFileTypes?: string[]
  minHeight?: string
  maxHeight?: string
  allowedNodes?: string[] | null
  allowedMarks?: string[] | null
  allowedFeatures?: string[] | null
  mediaPicker?: RichTextMediaPicker | null
  valueVersion?: RichTextValueVersion
  onInvalidContent?: ((issue: RichTextInvalidContent) => void) | null

  // Auto-load example content
  autoLoadExample?: boolean
  exampleContent?: unknown
}

const props = withDefaults(defineProps<RichTextFormFieldProps>(), {
  size: 'md',
  disabled: false,
  readonly: false,
  required: false,
  showCharacterCount: true,
  showWordCount: false,
  maxCharacters: 10000,
  allowFileUpload: true,
  maxFileSize: 10,
  maxVideoFileSize: 50,
  acceptedFileTypes: () => ['image/*', 'video/*'],
  minHeight: '300px',
  autoLoadExample: false,
  entityType: '',
  entityId: null,
  allowedNodes: null,
  allowedMarks: null,
  allowedFeatures: null,
  mediaPicker: null,
  valueVersion: null,
  onInvalidContent: null,
})

const emit = defineEmits<{
  'update:modelValue': [value: unknown]
  'file-upload': [file: File]
  'file-error': [error: string]
  'files-uploaded': [fileIds: number[]]
  focus: [event: FocusEvent]
  blur: [event: FocusEvent]
  change: [value: unknown]
  'invalid-content': [issue: RichTextInvalidContent]
}>()

const editorModelValue = computed(() => props.modelValue as string | object)

// Вычисляемые классы для поля
const fieldClasses = computed(() => {
  const classes = ['rich-text-field']

  // Размер
  if (props.size !== 'md') {
    classes.push(`rich-text-field-${props.size}`)
  }

  // Вариант цвета (если есть ошибка)
  if (props.error) {
    classes.push('rich-text-field-error')
  }

  // Состояния
  if (props.disabled) {
    classes.push('rich-text-field-disabled')
  }

  if (props.readonly) {
    classes.push('rich-text-field-readonly')
  }

  return classes.join(' ')
})

// Обработчики событий
const handleUpdate = (value: unknown) => {
  emit('update:modelValue', value)
  emit('change', value)
}

const handleFileUpload = (file: File) => {
  emit('file-upload', file)
}

const handleFileError = (error: string) => {
  emit('file-error', error)
}

const handleFilesUploaded = (fileIds: number[]) => {
  emit('files-uploaded', fileIds)
}

const handleFocus = (event: FocusEvent) => {
  emit('focus', event)
}

const handleBlur = (event: FocusEvent) => {
  emit('blur', event)
}

const handleInvalidContent = (issue: RichTextInvalidContent) => {
  emit('invalid-content', issue)
}

// Автоматическая загрузка примера контента при монтировании
import { onMounted } from 'vue'

onMounted(() => {
  if (props.autoLoadExample && !props.modelValue && props.exampleContent) {
    emit('update:modelValue', props.exampleContent)
  }
})
</script>

<style scoped>
.rich-text-form-field {
  @apply w-full;
}

.rich-text-field {
  border-radius: var(--rounded-btn, 0.5rem);
}

.rich-text-field-error {
  border-color: var(--fallback-er, oklch(var(--er) / var(--tw-border-opacity)));
}

.rich-text-field-disabled {
  opacity: 0.6;
  pointer-events: none;
}

.rich-text-field-readonly {
  background-color: var(--fallback-b2, oklch(var(--b2) / var(--tw-bg-opacity)));
}

/* Адаптивные размеры */
.rich-text-field-xs :deep(.ProseMirror) {
  min-height: 200px;
  font-size: 0.75rem;
}

.rich-text-field-sm :deep(.ProseMirror) {
  min-height: 250px;
  font-size: 0.875rem;
}

.rich-text-field-md :deep(.ProseMirror) {
  min-height: 300px;
  font-size: 1rem;
}

.rich-text-field-lg :deep(.ProseMirror) {
  min-height: 400px;
  font-size: 1.125rem;
}

.rich-text-field-xl :deep(.ProseMirror) {
  min-height: 500px;
  font-size: 1.25rem;
}
</style>
