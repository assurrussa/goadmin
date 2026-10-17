<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { Card } from '@/components/ui/card'
import { cn } from '@/lib/utils'

interface Props {
  class?: HTMLAttributes['class']
}

const props = defineProps<Props>()
</script>

<template>
  <Card
    :class="
      cn(
        'rounded-[var(--goadmin-card-radius,var(--radius-xl,0.75rem))] border border-border bg-card text-card-foreground shadow-sm',
        props.class,
      )
    "
  >
    <slot />
  </Card>
</template>
