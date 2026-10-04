<template>
  <Link
    :href="href"
    :class="[
      'group relative flex w-full items-center px-3 py-2 text-sm font-medium rounded-lg border border-transparent transition duration-150 ease-out focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/40 focus-visible:ring-offset-2 focus-visible:ring-offset-background',
      active
        ? 'bg-primary/10 text-primary-dark dark:text-primary-light border-primary/70 shadow-[0_0_8px_rgba(21,107,233,0.32)]'
        : 'text-text-secondary hover:text-text-primary hover:bg-surface-variant hover:border-border-primary hover:shadow-sm',
    ]"
  >
    <component
      v-if="icon"
      :is="icon"
      :class="[
        'mr-3 flex-shrink-0 h-5 w-5 transition duration-150',
        active
          ? 'text-primary-dark dark:text-primary-light'
          : 'text-text-tertiary group-hover:text-text-primary',
      ]"
    />
    <slot />
  </Link>
</template>

<script setup lang="ts">
import { Link } from '@inertiajs/vue3'
import type { Component } from 'vue'

defineProps<{
  href: string
  icon?: Component
  active?: boolean
}>()
</script>
