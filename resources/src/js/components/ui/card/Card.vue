<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'

const props = defineProps<{
  class?: HTMLAttributes['class']
}>()
</script>

<template>
  <div
    data-ui="card"
    data-part="root"
    :class="
      cn(
        'rounded-[var(--goadmin-card-radius,var(--radius-xl,0.75rem))] border border-[var(--goadmin-card-border,var(--color-border))] bg-[var(--goadmin-card-bg,var(--color-card))] text-[var(--goadmin-card-text,var(--color-card-foreground))] shadow-none',
        props.class,
      )
    "
  >
    <slot />
  </div>
</template>
