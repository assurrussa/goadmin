<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { computed } from 'vue'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'

const props = defineProps<{
  modelValue?: string | number | null
  defaultValue?: string | number
  type?: string
  placeholder?: string
  id?: string
  required?: boolean
  readonly?: boolean
  disabled?: boolean
  autocomplete?: string
  name?: string
  min?: string | number
  max?: string | number
  step?: string | number
  class?: HTMLAttributes['class']
}>()

const emits = defineEmits<{
  (e: 'update:modelValue', payload: string | number): void
}>()

const inputAttrs = computed(() => {
  const { class: _class, modelValue, defaultValue, ...rest } = props
  void _class
  void modelValue
  void defaultValue
  return rest
})
</script>

<template>
  <Input
    v-bind="inputAttrs"
    :model-value="props.modelValue ?? undefined"
    :class="cn('bg-card text-text-primary', props.class)"
    @update:modelValue="(value) => emits('update:modelValue', value)"
  />
</template>
