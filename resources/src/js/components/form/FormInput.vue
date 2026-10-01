<template>
  <div class="space-y-1">
    <!-- Label -->
    <label v-if="label" :for="inputId" class="block text-sm font-medium text-text-primary">
      {{ label }}
      <span v-if="required" class="text-error ml-1">*</span>
    </label>

    <!-- Input wrapper with icon support -->
    <div class="relative">
      <AppInput
        :id="inputId"
        :aria-invalid="error ? 'true' : undefined"
        :aria-describedby="error ? `${inputId}-error` : helpText ? `${inputId}-help` : undefined"
        :model-value="modelValue"
        :type="type"
        :placeholder="placeholder"
        :required="required"
        :readonly="readonly"
        :disabled="disabled"
        :autocomplete="autocomplete"
        :class="inputClasses"
        v-bind="$attrs"
        @update:modelValue="handleUpdate"
        @blur="handleBlur"
        @focus="handleFocus"
      />

      <!-- Left icon slot -->
      <div
        v-if="$slots.leftIcon"
        class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none"
      >
        <slot name="leftIcon" />
      </div>

      <!-- Right icon slot (interactive) -->
      <div
        v-if="$slots.rightIcon"
        class="absolute inset-y-0 right-0 pr-3 flex items-center"
        :class="{ 'pointer-events-none': readonly || disabled }"
      >
        <slot name="rightIcon" />
      </div>

      <!-- Loading spinner -->
      <div
        v-if="loading"
        class="absolute inset-y-0 right-0 pr-3 flex items-center pointer-events-none"
      >
        <svg class="animate-spin h-4 w-4 text-text-tertiary" fill="none" viewBox="0 0 24 24">
          <circle
            class="opacity-25"
            cx="12"
            cy="12"
            r="10"
            stroke="currentColor"
            stroke-width="4"
          />
          <path
            class="opacity-75"
            fill="currentColor"
            d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
          />
        </svg>
      </div>
    </div>

    <!-- Error message -->
    <p v-if="error" :id="`${inputId}-error`" class="text-sm text-error flex items-center space-x-1">
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
    <p v-if="helpText && !error" :id="`${inputId}-help`" class="text-sm text-text-secondary">
      {{ helpText }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed, useId, useSlots } from 'vue'
import AppInput from '@/components/ui/AppInput.vue'

interface Props {
  modelValue?: string | number | null
  label?: string
  type?: 'text' | 'email' | 'password' | 'tel' | 'url' | 'search' | 'number'
  placeholder?: string
  required?: boolean
  readonly?: boolean
  disabled?: boolean
  loading?: boolean
  autocomplete?: string
  error?: string | string[] | null
  helpText?: string
  size?: 'sm' | 'md' | 'lg'
  id?: string
}

const props = withDefaults(defineProps<Props>(), {
  modelValue: null,
  type: 'text',
  size: 'md',
  required: false,
  readonly: false,
  disabled: false,
  loading: false,
})

const emit = defineEmits<{
  'update:modelValue': [value: string | number | null]
  focus: [event: FocusEvent]
  blur: [event: FocusEvent]
}>()

const slots = useSlots()

// Generate unique ID if not provided
const inputId = computed(() => props.id || useId())

// Computed classes for input styling
const inputClasses = computed(() => {
  const classes: string[] = []

  if (props.size === 'sm') {
    classes.push('h-8 text-xs')
  } else if (props.size === 'lg') {
    classes.push('h-10 text-sm')
  } else {
    classes.push('h-9 text-sm')
  }

  if (props.error) {
    classes.push('border-destructive/70 focus-visible:ring-destructive/40')
  } else if (props.readonly) {
    classes.push('bg-surface-variant text-text-tertiary')
  }

  if (slots.leftIcon) {
    classes.push('pl-10')
  }
  if (slots.rightIcon || props.loading) {
    classes.push('pr-10')
  }

  return classes.join(' ')
})

// Event handlers
const handleUpdate = (value: string | number) => {
  const updated = props.type === 'number' ? Number(value) : value
  emit('update:modelValue', updated)
}

const handleFocus = (event: FocusEvent) => {
  emit('focus', event)
}

const handleBlur = (event: FocusEvent) => {
  emit('blur', event)
}
</script>
