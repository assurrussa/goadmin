<script setup lang="ts">
import type { AuthShellProps, AuthShellSlots } from './types'
const props = defineProps<AuthShellProps>()
defineSlots<AuthShellSlots>()
</script>

<template>
  <div
    data-ui="auth-shell"
    :class="props.class"
    class="min-h-screen bg-background flex flex-col justify-center"
  >
    <div class="absolute top-4 right-4 z-50 flex gap-2"><slot name="utilities" /></div>
    <div class="w-full relative">
      <div class="flex pt-5 pb-2 justify-center w-full items-center"><slot name="brand" /></div>
      <h2 class="mt-6 text-center text-3xl font-semibold tracking-tight text-text-primary">
        <slot name="title" />
      </h2>
      <p v-if="$slots.subtitle" class="mt-2 text-center text-sm text-text-secondary">
        <slot name="subtitle" />
      </p>
    </div>
    <div class="mt-8 px-4 sm:mx-auto sm:w-full sm:max-w-md sm:px-0">
      <div class="card py-8 px-4 sm:px-10"><slot /></div>
      <div v-if="$slots.footer" class="mt-6"><slot name="footer" /></div>
    </div>
  </div>
</template>
