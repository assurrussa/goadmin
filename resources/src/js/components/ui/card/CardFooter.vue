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
    data-part="footer"
    :class="cn('flex items-center p-[var(--goadmin-card-padding,1.5rem)] pt-0', props.class)"
  >
    <slot />
  </div>
</template>
