<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'

const props = defineProps<{
  class?: HTMLAttributes['class']
}>()
</script>

<template>
  <h3
    data-ui="card"
    data-part="title"
    :class="cn('font-semibold leading-none tracking-tight', props.class)"
  >
    <slot />
  </h3>
</template>
