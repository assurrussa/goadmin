<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { Circle } from 'lucide-vue-next'
import { cn } from '@/lib/utils'

type Variant = 'primary' | 'secondary' | 'success' | 'warning' | 'danger' | 'info' | 'outline'

const props = withDefaults(defineProps<{ variant?: Variant; class?: HTMLAttributes['class'] }>(), {
  variant: 'secondary',
})

const dotColors: Record<Variant, string> = {
  primary: 'text-primary',
  secondary: 'text-text-tertiary',
  success: 'text-success',
  warning: 'text-warning',
  danger: 'text-error',
  info: 'text-info',
  outline: 'text-text-tertiary',
}
</script>

<template>
  <span
    :class="cn('inline-flex items-center gap-2 text-xs font-medium text-text-primary', props.class)"
  >
    <Circle
      aria-hidden="true"
      :stroke-width="0"
      :class="cn('h-2 w-2 shrink-0 fill-current', dotColors[props.variant])"
    />
    <slot />
  </span>
</template>
