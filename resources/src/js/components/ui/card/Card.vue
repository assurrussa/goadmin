<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'

const props = defineProps<{
  class?: HTMLAttributes['class']
}>()
</script>

<template>
  <div
    :class="
      cn(
        'rounded-[var(--goadmin-card-radius,var(--radius-xl,0.75rem))] border bg-card text-card-foreground shadow-none',
        props.class,
      )
    "
  >
    <slot />
  </div>
</template>
