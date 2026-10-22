<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { useVModel } from '@vueuse/core'
import { cn } from '@/lib/utils'

const props = defineProps<{
  defaultValue?: string | number
  modelValue?: string | number
  class?: HTMLAttributes['class']
}>()

const emits = defineEmits<{
  (e: 'update:modelValue', payload: string | number): void
}>()

const modelValue = useVModel(props, 'modelValue', emits, {
  passive: true,
  defaultValue: props.defaultValue,
})
</script>

<template>
  <input
    data-ui="input"
    data-part="root"
    v-model="modelValue"
    :class="
      cn(
        'flex h-[var(--goadmin-control-height,2.25rem)] w-full rounded-[var(--goadmin-control-radius,var(--radius-md,0.375rem))] border border-[var(--goadmin-control-border,var(--color-input))] bg-[var(--goadmin-control-bg,transparent)] px-3 py-1 text-sm shadow-none transition-colors file:border-0 file:bg-[var(--goadmin-control-bg,transparent)] file:text-sm file:font-medium placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50',
        props.class,
      )
    "
  />
</template>
