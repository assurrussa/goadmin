<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { computed } from 'vue'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

type Variant =
  | 'primary'
  | 'secondary'
  | 'danger'
  | 'success'
  | 'warning'
  | 'ghost'
  | 'outline'
  | 'link'
type Size = 'xs' | 'sm' | 'md' | 'lg' | 'xl' | 'icon' | 'icon-sm' | 'icon-lg'

interface Props {
  type?: 'button' | 'submit' | 'reset'
  as?: string
  asChild?: boolean
  variant?: Variant
  size?: Size
  loading?: boolean
  disabled?: boolean
  fullWidth?: boolean
  rounded?: boolean
  class?: HTMLAttributes['class']
}

const props = withDefaults(defineProps<Props>(), {
  type: 'button',
  as: 'button',
  asChild: false,
  variant: 'primary',
  size: 'md',
  loading: false,
  disabled: false,
  fullWidth: false,
  rounded: false,
})

const variantMap: Record<
  Variant,
  'default' | 'secondary' | 'destructive' | 'ghost' | 'outline' | 'link'
> = {
  primary: 'default',
  secondary: 'secondary',
  danger: 'destructive',
  success: 'default',
  warning: 'default',
  ghost: 'ghost',
  outline: 'outline',
  link: 'link',
}

const sizeMap: Record<Size, 'default' | 'xs' | 'sm' | 'lg' | 'icon' | 'icon-sm' | 'icon-lg'> = {
  xs: 'xs',
  sm: 'sm',
  md: 'default',
  lg: 'lg',
  xl: 'lg',
  icon: 'icon',
  'icon-sm': 'icon-sm',
  'icon-lg': 'icon-lg',
}

const extraClass = computed(() => {
  const classes: string[] = []

  if (props.fullWidth) classes.push('w-full')
  if (props.rounded) classes.push('rounded-full')
  if (props.loading) classes.push('opacity-70')

  if (props.variant === 'success') {
    classes.push('bg-success text-text-white hover:bg-success/90')
  }
  if (props.variant === 'warning') {
    classes.push('bg-warning text-text-white hover:bg-warning/90')
  }

  if (props.size === 'xl') {
    classes.push('h-11 px-6 text-base')
  }

  // Add theme-aware classes for outline and ghost variants
  if (props.variant === 'outline') {
    classes.push('btn-outline-theme')
  }
  if (props.variant === 'ghost') {
    classes.push('btn-ghost-theme')
  }

  return classes
})
</script>

<template>
  <Button
    :type="type"
    :as="as"
    :as-child="asChild"
    :variant="variantMap[variant]"
    :size="sizeMap[size]"
    :disabled="disabled || loading"
    :class="cn(extraClass, props.class)"
  >
    <slot />
  </Button>
</template>
