<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'

const props = defineProps<{
  class?: HTMLAttributes['class']
}>()
</script>

<template>
  <thead
    data-ui="table"
    data-part="header"
    :class="cn('bg-[var(--goadmin-table-header-bg,transparent)] [&_tr]:border-b', props.class)"
  >
    <slot />
  </thead>
</template>
