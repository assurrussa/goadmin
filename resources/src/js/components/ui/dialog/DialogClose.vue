<script setup lang="ts">
import type { DialogCloseProps } from 'reka-ui'
import { DialogClose } from 'reka-ui'

const props = defineProps<DialogCloseProps>()
</script>

<template>
  <DialogClose data-ui="dialog" data-part="close" v-bind="props">
    <slot />
  </DialogClose>
</template>
