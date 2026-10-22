<script setup lang="ts">
import type { SelectGroupProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { reactiveOmit } from '@vueuse/core'
import { SelectGroup } from 'reka-ui'
import { cn } from '@/lib/utils'

const props = defineProps<SelectGroupProps & { class?: HTMLAttributes['class'] }>()

const delegatedProps = reactiveOmit(props, 'class')
</script>

<template>
  <SelectGroup
    data-ui="select"
    data-part="group"
    :class="cn('p-1 w-full', props.class)"
    v-bind="delegatedProps"
  >
    <slot />
  </SelectGroup>
</template>
