<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { computed } from 'vue'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'

type Variant = 'primary' | 'secondary' | 'success' | 'warning' | 'danger' | 'info' | 'outline'

interface Props {
  variant?: Variant
  dot?: boolean
  class?: HTMLAttributes['class']
}

const props = withDefaults(defineProps<Props>(), {
  variant: 'primary',
  dot: false,
})

const variantMap: Record<Variant, 'default' | 'secondary' | 'destructive' | 'outline'> = {
  primary: 'default',
  secondary: 'secondary',
  success: 'secondary',
  warning: 'secondary',
  danger: 'destructive',
  info: 'secondary',
  outline: 'outline',
}

const extraClass = computed(() => {
  const classes: string[] = []
  if (props.variant === 'success') {
    classes.push('bg-success/10 text-success border-success/30')
  }
  if (props.variant === 'warning') {
    classes.push('bg-warning/10 text-warning border-warning/30')
  }
  if (props.variant === 'info') {
    classes.push('bg-info/10 text-info border-info/30')
  }
  if (props.dot) {
    classes.push('h-2.5 w-2.5 rounded-full p-0')
  }
  return classes
})
</script>

<template>
  <Badge :variant="variantMap[variant]" :class="cn(extraClass, props.class)">
    <slot />
  </Badge>
</template>
