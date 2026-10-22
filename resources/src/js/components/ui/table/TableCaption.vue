<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'

const props = defineProps<{
  class?: HTMLAttributes['class']
}>()
</script>

<template>
  <caption
    data-ui="table"
    data-part="caption"
    :class="cn('mt-4 text-sm text-muted-foreground', props.class)"
  >
    <slot />
  </caption>
</template>
