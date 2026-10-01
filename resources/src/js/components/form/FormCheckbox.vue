<template>
  <div class="flex items-center space-x-3">
    <Checkbox
      :id="inputId"
      :aria-invalid="error ? 'true' : undefined"
      :aria-describedby="error ? `${inputId}-error` : helpText ? `${inputId}-help` : undefined"
      :checked="modelValue || false"
      :disabled="disabled"
      :required="required"
      :class="checkboxClasses"
      @update:checked="handleUpdate"
    />

    <label
      v-if="label"
      :for="inputId"
      class="text-sm font-medium text-text-primary select-none cursor-pointer"
      :class="{ 'cursor-not-allowed opacity-60': disabled }"
    >
      {{ label }}
      <span v-if="required" class="text-error ml-1">*</span>
    </label>
  </div>

  <!-- Error message -->
  <p
    v-if="error"
    :id="`${inputId}-error`"
    class="mt-1 text-sm text-error flex items-center space-x-1"
  >
    <svg class="w-4 h-4 flex-shrink-0" fill="currentColor" viewBox="0 0 20 20">
      <path
        fill-rule="evenodd"
        d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7 4a1 1 0 11-2 0 1 1 0 012 0zm-1-9a1 1 0 00-1 1v4a1 1 0 102 0V6a1 1 0 00-1-1z"
        clip-rule="evenodd"
      />
    </svg>
    <span>{{ Array.isArray(error) ? error[0] : error }}</span>
  </p>

  <!-- Help text -->
  <p v-if="helpText && !error" :id="`${inputId}-help`" class="mt-1 text-sm text-text-secondary">
    {{ helpText }}
  </p>
</template>

<script setup lang="ts">
import { computed, useId } from 'vue'
import { Checkbox } from '@/components/ui/checkbox'

interface Props {
  modelValue?: boolean | null
  label?: string
  required?: boolean
  disabled?: boolean
  error?: string | string[] | null
  helpText?: string
  size?: 'sm' | 'md' | 'lg'
  id?: string
}

const props = withDefaults(defineProps<Props>(), {
  modelValue: false,
  size: 'md',
  required: false,
  disabled: false,
})

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
}>()

// Generate unique ID if not provided
const inputId = computed(() => props.id || useId())

// Computed classes for checkbox styling
const checkboxClasses = computed(() => {
  const classes = []

  if (props.size === 'sm') {
    classes.push('h-3.5 w-3.5')
  }

  if (props.size === 'lg') {
    classes.push('h-5 w-5')
  }

  if (props.error) {
    classes.push('border-destructive/70 data-[state=checked]:bg-destructive')
  }

  if (props.disabled) {
    classes.push('opacity-60 cursor-not-allowed')
  }

  return classes.join(' ')
})

// Event handlers
const handleUpdate = (checked: boolean) => {
  emit('update:modelValue', checked)
}
</script>
