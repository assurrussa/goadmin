<script setup lang="ts">
import type { AdminShellProps, AdminShellSlots } from './types'
defineProps<AdminShellProps>()
defineSlots<AdminShellSlots>()
</script>

<template>
  <div
    data-ui="admin-shell"
    data-part="sidebar"
    class="min-h-screen bg-background text-text-primary"
  >
    <aside
      id="admin-sidebar"
      class="fixed inset-y-0 left-0 z-30 hidden lg:block bg-card border-r border-border-primary shadow-[var(--goadmin-sidebar-shadow,var(--goadmin-default-sidebar-shadow))] transition-[width] duration-200 motion-reduce:transition-none"
      :class="
        collapsed
          ? 'w-[var(--goadmin-sidebar-collapsed-width,5rem)]'
          : 'w-[var(--goadmin-sidebar-width,16rem)]'
      "
    >
      <slot name="navigation" orientation="vertical" :collapsed="collapsed" />
    </aside>
    <slot name="mobileNavigation" />
    <div
      class="transition-[padding] duration-200 motion-reduce:transition-none"
      :class="
        collapsed
          ? 'lg:pl-[var(--goadmin-sidebar-collapsed-width,5rem)]'
          : 'lg:pl-[var(--goadmin-sidebar-width,16rem)]'
      "
    >
      <slot name="header" />
      <main class="py-6">
        <div
          class="max-w-[var(--goadmin-content-max-width,80rem)] mx-auto w-full px-4 sm:px-6 lg:px-8 space-y-6"
        >
          <slot name="breadcrumbs" />
          <slot />
        </div>
      </main>
    </div>
  </div>
</template>
