<script setup lang="ts">
import type { AdminShellProps, AdminShellSlots } from '@admin-core/presentation'

defineProps<AdminShellProps>()
defineSlots<AdminShellSlots>()
</script>

<template>
  <div
    data-ui="admin-shell"
    data-part="topbar"
    class="min-h-screen bg-background text-text-primary"
  >
    <header class="border-b border-border-primary bg-card">
      <div
        class="mx-auto flex min-h-16 max-w-[var(--goadmin-content-max-width,80rem)] items-center justify-between gap-4 px-4 sm:px-6 lg:px-8"
      >
        <div class="flex min-w-0 items-center gap-3">
          <slot name="navigationToggle" />
          <slot name="brand" />
        </div>
        <div class="flex shrink-0 items-center gap-3">
          <slot name="utilities" />
          <slot name="account" />
        </div>
      </div>
      <div class="hidden border-t border-border-secondary lg:block">
        <div class="mx-auto max-w-[var(--goadmin-content-max-width,80rem)] px-4 sm:px-6 lg:px-8">
          <slot name="navigation" orientation="horizontal" :collapsed="collapsed" />
        </div>
      </div>
    </header>
    <slot name="mobileNavigation" />
    <main class="py-6">
      <div
        class="mx-auto w-full max-w-[var(--goadmin-content-max-width,80rem)] space-y-6 px-4 sm:px-6 lg:px-8"
      >
        <slot name="breadcrumbs" />
        <slot />
      </div>
    </main>
  </div>
</template>
