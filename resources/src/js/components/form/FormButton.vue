<template>
  <AppButton
    :type="type"
    :variant="variant"
    :size="size"
    :loading="loading"
    :disabled="disabled"
    :fullWidth="fullWidth"
    :rounded="rounded"
    @click="handleClick"
  >
    <!-- Loading spinner -->
    <span v-if="loading" class="animate-spin flex-shrink-0" :class="iconSizeClasses">
      <svg fill="none" viewBox="0 0 24 24" class="w-full h-full">
        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
        <path
          class="opacity-75"
          fill="currentColor"
          d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
        />
      </svg>
    </span>

    <!-- Left icon -->
    <span v-else-if="$slots.leftIcon" class="flex-shrink-0" :class="iconSizeClasses">
      <slot name="leftIcon" />
    </span>

    <!-- Button text -->
    <span v-if="$slots.default || loading">
      <slot v-if="!loading" />
      <span v-else>{{ loadingText }}</span>
    </span>

    <!-- Right icon -->
    <span v-if="$slots.rightIcon && !loading" class="flex-shrink-0" :class="iconSizeClasses">
      <slot name="rightIcon" />
    </span>
  </AppButton>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import AppButton from '@/components/ui/AppButton.vue'

interface Props {
  type?: 'button' | 'submit' | 'reset'
  variant?: 'primary' | 'secondary' | 'danger' | 'success' | 'warning' | 'ghost' | 'outline'
  size?: 'xs' | 'sm' | 'md' | 'lg' | 'xl'
  loading?: boolean
  disabled?: boolean
  loadingText?: string
  fullWidth?: boolean
  rounded?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  type: 'button',
  variant: 'primary',
  size: 'md',
  loading: false,
  disabled: false,
  loadingText: 'Загрузка...',
  fullWidth: false,
  rounded: false,
})

const emit = defineEmits<{
  click: [event: MouseEvent]
}>()

// Icon size classes
const iconSizeClasses = computed(() => {
  const sizes = {
    xs: 'w-3 h-3',
    sm: 'w-4 h-4',
    md: 'w-4 h-4',
    lg: 'w-5 h-5',
    xl: 'w-5 h-5',
  }
  return sizes[props.size]
})

// Event handlers
const handleClick = (event: MouseEvent) => {
  if (!props.disabled && !props.loading) {
    emit('click', event)
  }
}
</script>
