<script setup lang="ts">
import type { SelectValueProps } from 'reka-ui'
import { SelectValue } from 'reka-ui'

const props = defineProps<SelectValueProps>()
</script>

<template>
  <SelectValue data-ui="select" data-part="value" v-bind="props">
    <slot />
  </SelectValue>
</template>
