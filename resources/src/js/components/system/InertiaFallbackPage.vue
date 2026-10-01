<template>
  <main class="mx-auto max-w-xl p-6 text-center">
    <h1 class="mb-2 text-xl font-semibold text-text-primary">Страница недоступна</h1>
    <p class="mb-5 text-text-secondary">
      Не удалось открыть эту страницу. Попробуйте вернуться на главную.
    </p>
    <Link
      href="/"
      class="text-primary underline focus-visible:outline-2 focus-visible:outline-primary"
      >На главную</Link
    >
  </main>
</template>

<script setup lang="ts">
import { Link } from '@inertiajs/vue3'

defineProps<{ name: string }>()
</script>
