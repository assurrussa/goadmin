<template>
  <div
    :class="[
      'notification mb-4 p-4 rounded-lg shadow-lg border-l-4 transform transition-all duration-300 ease-in-out',
      'hover:shadow-xl',
      notificationClasses,
    ]"
    @mouseenter="pauseTimeout"
    @mouseleave="resumeTimeout"
  >
    <div class="flex items-start">
      <div class="flex-shrink-0">
        <component :is="iconComponent" :class="['h-5 w-5', iconClass]" />
      </div>

      <div class="ml-3 flex-1">
        <h3 v-if="notification.title" class="text-sm font-semibold mb-1 text-text-primary">
          {{ notification.title }}
        </h3>
        <p class="text-sm text-text-secondary">{{ notification.message }}</p>
      </div>

      <div v-if="notification.closable" class="ml-4 flex-shrink-0">
        <button
          @click="$emit('close')"
          class="inline-flex text-text-tertiary hover:text-text-primary focus:outline-none focus:text-text-primary transition ease-in-out duration-150"
        >
          <svg class="h-4 w-4" fill="currentColor" viewBox="0 0 20 20">
            <path
              fill-rule="evenodd"
              d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z"
              clip-rule="evenodd"
            />
          </svg>
        </button>
      </div>
    </div>

    <!-- Progress bar -->
    <div
      v-if="notification.timeout && notification.timeout > 0"
      class="mt-3 w-full bg-border-secondary rounded-full h-1.5 overflow-hidden transition-opacity duration-150"
      :class="isPaused ? 'opacity-60' : 'opacity-100'"
    >
      <div
        class="h-1.5 rounded-full transition-[width] duration-100 ease-linear"
        :style="progressBarStyle"
      ></div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import type { Notification } from '@/stores/notifications'
import {
  CheckCircleIcon,
  ExclamationCircleIcon,
  InformationCircleIcon,
  ExclamationTriangleIcon,
} from '@heroicons/vue/24/outline'

const emit = defineEmits<{
  close: []
}>()

const props = defineProps<{
  notification: Notification
}>()

const remaining = ref(0)
const isPaused = ref(false)
let intervalId: ReturnType<typeof setInterval> | undefined

const notificationClasses = computed(() => {
  switch (props.notification.type) {
    case 'success':
      return 'notification--success'
    case 'error':
      return 'notification--error'
    case 'warning':
      return 'notification--warning'
    case 'info':
      return 'notification--info'
    default:
      return ''
  }
})

const iconComponent = computed(() => {
  switch (props.notification.type) {
    case 'success':
      return CheckCircleIcon
    case 'error':
      return ExclamationCircleIcon
    case 'warning':
      return ExclamationTriangleIcon
    case 'info':
      return InformationCircleIcon
    default:
      return InformationCircleIcon
  }
})

const iconClass = computed(() => {
  switch (props.notification.type) {
    case 'success':
      return 'text-success'
    case 'error':
      return 'text-error'
    case 'warning':
      return 'text-warning'
    case 'info':
      return 'text-info'
    default:
      return 'text-text-tertiary'
  }
})

const progressBarColor = computed(() => {
  switch (props.notification.type) {
    case 'success':
      return 'var(--color-success)'
    case 'error':
      return 'var(--color-error)'
    case 'warning':
      return 'var(--color-warning)'
    case 'info':
      return 'var(--color-info)'
    default:
      return 'var(--color-text-tertiary)'
  }
})

const progressPercent = computed(() => {
  const timeout = props.notification.timeout ?? 0
  if (!timeout) return 0
  const percent = (remaining.value / timeout) * 100
  return Math.max(0, Math.min(100, percent))
})

const progressBarStyle = computed(() => ({
  width: `${progressPercent.value}%`,
  backgroundColor: progressBarColor.value,
}))

const startTimeout = () => {
  const timeout = props.notification.timeout ?? 0
  if (timeout <= 0) return

  remaining.value = timeout

  intervalId = setInterval(() => {
    if (!isPaused.value) {
      remaining.value -= 100
      if (remaining.value <= 0) {
        emit('close')
      }
    }
  }, 100)
}

const pauseTimeout = () => {
  isPaused.value = true
}

const resumeTimeout = () => {
  isPaused.value = false
}

onMounted(() => {
  startTimeout()
})

watch(
  () => props.notification.timeout,
  (value) => {
    remaining.value = value ?? 0
  },
  { immediate: true },
)

onUnmounted(() => {
  if (intervalId) {
    clearInterval(intervalId)
  }
})
</script>
