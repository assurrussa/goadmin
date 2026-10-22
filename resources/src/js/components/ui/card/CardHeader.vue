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
    data-part="header"
    :class="cn('flex flex-col gap-y-1.5 p-[var(--goadmin-card-padding,1.5rem)]', props.class)"
  >
    <slot />
  </div>
</template>
