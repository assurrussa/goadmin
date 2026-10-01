<template>
  <div class="space-y-1">
    <label v-if="label" :for="selectId" class="block text-sm font-medium text-text-primary">
      {{ label }}
      <span v-if="required" class="text-error ml-1">*</span>
    </label>

    <AppSelect
      :id="selectId"
      :aria-invalid="error ? 'true' : undefined"
      :aria-describedby="error ? `${selectId}-error` : helpText ? `${selectId}-help` : undefined"
      :model-value="modelValue"
      :options="options"
      :placeholder="placeholder"
      :disabled="disabled"
      :error="Boolean(error)"
      :size="size"
      v-bind="$attrs"
      @update:modelValue="handleUpdate"
    />

    <p
      v-if="error"
      :id="`${selectId}-error`"
      class="text-sm text-error flex items-center space-x-1"
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

    <p v-if="helpText && !error" :id="`${selectId}-help`" class="text-sm text-text-secondary">
      {{ helpText }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed, useId } from 'vue'
import AppSelect from '@/components/ui/AppSelect.vue'

export interface SelectOption {
  label: string
  value: string | number
  disabled?: boolean
  group?: string
}

interface Props {
  modelValue?: string | number | bigint | null
  options?: SelectOption[]
  label?: string
  placeholder?: string
  placeholderValue?: string | number
  error?: string | string[] | null
  helpText?: string
  size?: 'sm' | 'md' | 'lg'
  disabled?: boolean
  required?: boolean
  id?: string
}

const props = withDefaults(defineProps<Props>(), {
  size: 'md',
  disabled: false,
  required: false,
  placeholderValue: '' as string | number,
})

const emit = defineEmits<{
  'update:modelValue': [value: string | number | bigint | null]
}>()
const selectId = computed(() => props.id || useId())

// grouped options are now handled inside AppSelect

const handleUpdate = (value: unknown) => {
  emit('update:modelValue', value as string | number | bigint | null)
}
</script>
