<template>
  <FormInput
    v-model="internalValue"
    :type="showPassword ? 'text' : 'password'"
    :label="label"
    :placeholder="placeholder"
    :required="required"
    :readonly="readonly"
    :disabled="disabled"
    :loading="loading"
    :autocomplete="autocomplete"
    :error="error"
    :help-text="helpText"
    :size="size"
    :id="id"
    v-bind="$attrs"
  >
    <template #rightIcon>
      <button
        type="button"
        @click="togglePasswordVisibility"
        class="text-text-tertiary hover:text-text-secondary transition-colors duration-200"
        :disabled="readonly || disabled"
        tabindex="-1"
      >
        <EyeIcon v-if="!showPassword" class="w-5 h-5" />
        <EyeSlashIcon v-else class="w-5 h-5" />
      </button>
    </template>
  </FormInput>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { EyeIcon, EyeSlashIcon } from '@heroicons/vue/24/outline'
import FormInput from './FormInput.vue'

interface Props {
  modelValue?: string | null
  label?: string
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
  autocomplete: 'current-password',
  placeholder: '••••••••',
})

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

// Password visibility state
const showPassword = ref(false)

// Internal model value
const internalValue = computed({
  get: () => props.modelValue || '',
  set: (value: string) => emit('update:modelValue', value),
})

// Toggle password visibility
const togglePasswordVisibility = () => {
  showPassword.value = !showPassword.value
}
</script>
