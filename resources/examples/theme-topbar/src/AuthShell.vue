<script setup lang="ts">
import type { AuthShellProps, AuthShellSlots } from '@admin-core/presentation'

const props = defineProps<AuthShellProps>()
defineSlots<AuthShellSlots>()
</script>

<template>
  <div
    data-ui="auth-shell"
    data-part="split"
    :class="props.class"
    class="min-h-screen bg-background text-text-primary"
  >
    <div class="absolute right-4 top-4 flex items-center gap-2"><slot name="utilities" /></div>
    <div
      class="mx-auto grid min-h-screen max-w-[var(--goadmin-content-max-width,80rem)] items-center gap-8 px-4 py-20 sm:px-6 lg:grid-cols-2 lg:gap-16 lg:px-8"
    >
      <section
        class="relative overflow-hidden rounded-[var(--goadmin-card-radius,8px)] bg-primary/10 p-8 lg:min-h-96 lg:p-12"
      >
        <div
          aria-hidden="true"
          class="pointer-events-none absolute -bottom-20 -right-20 size-64 rounded-full border-[32px] border-primary/10"
        />
        <div class="relative space-y-6">
          <slot name="brand" />
          <h1 class="text-3xl font-semibold tracking-tight lg:text-4xl"><slot name="title" /></h1>
          <p v-if="$slots.subtitle" class="max-w-md text-sm leading-relaxed text-text-secondary">
            <slot name="subtitle" />
          </p>
        </div>
      </section>
      <div class="mx-auto w-full max-w-md">
        <div class="card p-6 sm:p-8"><slot /></div>
        <div v-if="$slots.footer" class="mt-6"><slot name="footer" /></div>
      </div>
    </div>
  </div>
</template>
