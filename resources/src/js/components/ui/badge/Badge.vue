<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import type { BadgeVariants } from '.'
import { cn } from '@/lib/utils'
import { badgeVariants } from '.'

const props = defineProps<{
  variant?: BadgeVariants['variant']
  class?: HTMLAttributes['class']
}>()
</script>

<template>
  <div data-ui="badge" data-part="root" :class="cn(badgeVariants({ variant }), props.class)">
    <slot />
  </div>
</template>
