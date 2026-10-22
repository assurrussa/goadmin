<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'

const props = defineProps<{
  class?: HTMLAttributes['class']
}>()
</script>

<template>
  <tr
    data-ui="table"
    data-part="row"
    :class="
      cn(
        'border-b transition-colors hover:bg-[var(--goadmin-table-row-hover,color-mix(in_srgb,var(--color-muted)_50%,transparent))] data-[state=selected]:bg-muted',
        props.class,
      )
    "
  >
    <slot />
  </tr>
</template>
