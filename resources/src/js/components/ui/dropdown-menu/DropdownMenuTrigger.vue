<script setup lang="ts">
import type { DropdownMenuTriggerProps } from 'reka-ui'
import { DropdownMenuTrigger, useForwardProps } from 'reka-ui'

const props = defineProps<DropdownMenuTriggerProps>()

const forwardedProps = useForwardProps(props)
</script>

<template>
  <DropdownMenuTrigger
    data-ui="dropdown-menu"
    data-part="trigger"
    class="outline-none"
    v-bind="forwardedProps"
  >
    <slot />
  </DropdownMenuTrigger>
</template>
