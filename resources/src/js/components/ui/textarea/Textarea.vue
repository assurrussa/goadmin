<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { useVModel } from '@vueuse/core'
import { cn } from '@/lib/utils'

const props = defineProps<{
  class?: HTMLAttributes['class']
  defaultValue?: string | number
  modelValue?: string | number
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
  <textarea
    data-ui="textarea"
    data-part="root"
    v-model="modelValue"
    :class="
      cn(
        'flex min-h-[var(--goadmin-textarea-min-height,60px)] w-full rounded-[var(--goadmin-control-radius,var(--radius-md,0.375rem))] border border-[var(--goadmin-control-border,var(--color-input))] bg-[var(--goadmin-control-bg,transparent)] px-3 py-2 text-sm shadow-none placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50',
        props.class,
      )
    "
  />
</template>
