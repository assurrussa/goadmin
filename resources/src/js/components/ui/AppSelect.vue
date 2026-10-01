<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { computed } from 'vue'
import type { AcceptableValue } from 'reka-ui'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { cn } from '@/lib/utils'

interface Option {
  label: string
  value: AcceptableValue
  disabled?: boolean
  group?: string
}

const props = withDefaults(
  defineProps<{
    modelValue?: AcceptableValue
    options?: Option[]
    placeholder?: string
    disabled?: boolean
    error?: boolean
    size?: 'sm' | 'md' | 'lg'
    class?: HTMLAttributes['class']
  }>(),
  {
    options: () => [],
    placeholder: 'Выберите',
    disabled: false,
    error: false,
    size: 'md',
  },
)

const emit = defineEmits<{
  (e: 'update:modelValue', value: AcceptableValue): void
}>()
defineOptions({ inheritAttrs: false })

const sanitizedOptions = computed(() =>
  props.options.filter((option) => option.value !== '' && option.value !== null),
)

const groupedOptions = computed(() => {
  const groups = new Map<string, Option[]>()
  sanitizedOptions.value.forEach((option) => {
    const key = option.group || ''
    if (!groups.has(key)) {
      groups.set(key, [])
    }
    groups.get(key)!.push(option)
  })

  return Array.from(groups.entries()).map(([label, options]) => ({
    label,
    options,
  }))
})

const triggerClass = computed(() => {
  const sizeClass =
    props.size === 'sm' ? 'h-8 text-xs' : props.size === 'lg' ? 'h-10 text-sm' : 'h-9 text-sm'
  const errorClass = props.error ? 'border-destructive/70 focus:ring-destructive/40' : ''
  return cn(sizeClass, errorClass, props.class)
})
</script>

<template>
  <Select
    :model-value="modelValue"
    :disabled="disabled"
    @update:modelValue="emit('update:modelValue', $event as AcceptableValue)"
  >
    <SelectTrigger v-bind="$attrs" :class="cn('w-full', triggerClass)">
      <SelectValue :placeholder="placeholder" />
    </SelectTrigger>
    <SelectContent>
      <template v-for="group in groupedOptions" :key="group.label || 'default'">
        <SelectGroup>
          <SelectLabel v-if="group.label">{{ group.label }}</SelectLabel>
          <SelectItem
            v-for="option in group.options"
            :key="String(option.value)"
            :value="option.value"
            :disabled="option.disabled"
          >
            {{ option.label }}
          </SelectItem>
        </SelectGroup>
      </template>
    </SelectContent>
  </Select>
</template>
