<script setup lang="ts">
import { Head } from '@inertiajs/vue3'

defineProps({ title: String })
</script>

<template>
  <Head :title="title ? `${title} — Админка` : 'Админка'">
    <slot />
  </Head>
</template>
